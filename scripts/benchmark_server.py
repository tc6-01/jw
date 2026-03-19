#!/usr/bin/env python3
import csv
import json
import math
import os
import statistics
import subprocess
import threading
import time
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

REPO = Path(__file__).resolve().parents[1]
OUT_DIR = REPO / "docs" / "perf"
DATE = "2026-03-19"
CSV_PATH = OUT_DIR / f"server-benchmark-{DATE}.csv"
SVG_PATH = OUT_DIR / f"server-benchmark-{DATE}.svg"
BIN_PATH = Path("/tmp/jw-bench")
PORT = 20091
ADDR = f"127.0.0.1:{PORT}"
BASE_URL = f"http://{ADDR}"
HOME_DIR = Path("/tmp/jw-bench-home")
LOG_PATH = Path("/tmp/jw-bench.log")

SCENARIOS = [
    ("idle_30s", 0, 1),
    ("health_seq_10000", 10000, 1),
    ("health_conc_20_x20000", 20000, 20),
    ("record_seq_5000", 5000, 1),
    ("jump_conc_20_x10000", 10000, 20),
]


def run(cmd, **kwargs):
    return subprocess.run(cmd, check=True, text=True, **kwargs)


def build_binary():
    run(["go", "build", "-o", str(BIN_PATH), "./cmd/jw"], cwd=str(REPO))


def clear_proxy_env(env):
    for key in [
        "http_proxy",
        "https_proxy",
        "HTTP_PROXY",
        "HTTPS_PROXY",
        "ALL_PROXY",
        "all_proxy",
        "NO_PROXY",
        "no_proxy",
    ]:
        env.pop(key, None)


def start_server():
    HOME_DIR.mkdir(parents=True, exist_ok=True)
    env = os.environ.copy()
    clear_proxy_env(env)
    env["HOME"] = str(HOME_DIR)
    logf = LOG_PATH.open("w", encoding="utf-8")
    proc = subprocess.Popen(
        [str(BIN_PATH), "server", ADDR],
        stdout=logf,
        stderr=logf,
        env=env,
    )
    return proc, logf


def stop_server(proc, logf):
    try:
        proc.terminate()
        proc.wait(timeout=5)
    except Exception:
        proc.kill()
    logf.close()


def wait_health(timeout=8.0):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            body = http_get("/health")
            if body.get("ok") is True:
                return
        except Exception:
            pass
        time.sleep(0.1)
    raise RuntimeError("server health check timeout")


def opener():
    return urllib.request.build_opener(urllib.request.ProxyHandler({}))


def http_get(path):
    req = urllib.request.Request(BASE_URL + path, method="GET")
    with opener().open(req, timeout=3) as resp:
        data = resp.read().decode("utf-8")
    return json.loads(data)


def http_post(path, payload):
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        BASE_URL + path,
        data=data,
        method="POST",
        headers={"Content-Type": "application/json"},
    )
    with opener().open(req, timeout=3) as resp:
        body = resp.read().decode("utf-8")
    return json.loads(body)


def percentile(sorted_values, p):
    if not sorted_values:
        return 0.0
    if len(sorted_values) == 1:
        return float(sorted_values[0])
    idx = (len(sorted_values) - 1) * p
    lo = math.floor(idx)
    hi = math.ceil(idx)
    if lo == hi:
        return float(sorted_values[lo])
    frac = idx - lo
    return float(sorted_values[lo] * (1.0 - frac) + sorted_values[hi] * frac)


class ResourceSampler:
    def __init__(self, pid):
        self.pid = pid
        self._stop = threading.Event()
        self._lock = threading.Lock()
        self._active = False
        self._samples = []
        self._thread = threading.Thread(target=self._loop, daemon=True)

    def start(self):
        self._thread.start()

    def _loop(self):
        while not self._stop.is_set():
            if self._active:
                sample = self._sample_once()
                if sample is not None:
                    with self._lock:
                        self._samples.append(sample)
            time.sleep(0.2)

    def _sample_once(self):
        try:
            out = subprocess.check_output(
                ["ps", "-p", str(self.pid), "-o", "%cpu=,rss="], text=True
            ).strip()
        except Exception:
            return None
        if not out:
            return None
        parts = out.split()
        if len(parts) < 2:
            return None
        try:
            cpu = float(parts[0])
            rss_kb = int(parts[1])
        except ValueError:
            return None
        return cpu, rss_kb

    def begin_window(self):
        with self._lock:
            self._samples = []
        self._active = True

    def end_window(self):
        self._active = False
        with self._lock:
            samples = list(self._samples)
        if not samples:
            return {
                "peak_cpu": 0.0,
                "avg_cpu": 0.0,
                "peak_rss_mb": 0.0,
                "avg_rss_mb": 0.0,
            }
        cpus = [s[0] for s in samples]
        rss = [s[1] / 1024.0 for s in samples]
        return {
            "peak_cpu": max(cpus),
            "avg_cpu": sum(cpus) / len(cpus),
            "peak_rss_mb": max(rss),
            "avg_rss_mb": sum(rss) / len(rss),
        }

    def stop(self):
        self._stop.set()
        self._thread.join(timeout=2)


def run_idle(seconds):
    start = time.perf_counter()
    time.sleep(seconds)
    duration = time.perf_counter() - start
    return {
        "requests": 0,
        "success": 0,
        "duration_sec": duration,
        "throughput_rps": 0.0,
        "avg_ms": 0.0,
        "p50_ms": 0.0,
        "p95_ms": 0.0,
    }


def run_health(count, concurrency):
    def one(_):
        t0 = time.perf_counter()
        http_get("/health")
        return (time.perf_counter() - t0) * 1000.0

    return run_requests(count, concurrency, one)


def run_record(count, concurrency):
    def one(i):
        payload = {
            "url": f"https://example{i % 100}.com/site/{i}",
            "title": f"Example {i}",
        }
        t0 = time.perf_counter()
        http_post("/record", payload)
        return (time.perf_counter() - t0) * 1000.0

    return run_requests(count, concurrency, one)


def run_jump(count, concurrency):
    def one(_):
        t0 = time.perf_counter()
        http_get("/jump?q=site")
        return (time.perf_counter() - t0) * 1000.0

    return run_requests(count, concurrency, one)


def run_requests(count, concurrency, fn):
    latencies = []
    start = time.perf_counter()
    success = 0
    with ThreadPoolExecutor(max_workers=concurrency) as ex:
        futures = [ex.submit(fn, i) for i in range(count)]
        for fut in as_completed(futures):
            lat = fut.result()
            latencies.append(lat)
            success += 1
    duration = time.perf_counter() - start
    latencies.sort()
    return {
        "requests": count,
        "success": success,
        "duration_sec": duration,
        "throughput_rps": success / duration if duration > 0 else 0.0,
        "avg_ms": statistics.mean(latencies) if latencies else 0.0,
        "p50_ms": percentile(latencies, 0.5),
        "p95_ms": percentile(latencies, 0.95),
    }


def write_csv(rows):
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    fields = [
        "scenario",
        "requests",
        "concurrency",
        "success",
        "duration_sec",
        "throughput_rps",
        "avg_ms",
        "p50_ms",
        "p95_ms",
        "avg_cpu_pct",
        "peak_cpu_pct",
        "avg_rss_mb",
        "peak_rss_mb",
    ]
    with CSV_PATH.open("w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=fields)
        w.writeheader()
        for r in rows:
            w.writerow(r)


def _bar_width(value, max_value, width):
    if max_value <= 0:
        return 0.0
    return (value / max_value) * width


def write_svg(rows):
    # filter non-idle for latency/throughput chart
    perf_rows = [r for r in rows if r["requests"] > 0]
    labels = [r["scenario"] for r in perf_rows]

    max_p95 = max((r["p95_ms"] for r in perf_rows), default=1.0)
    max_tps = max((r["throughput_rps"] for r in perf_rows), default=1.0)
    max_rss = max((r["peak_rss_mb"] for r in rows), default=1.0)

    width = 1200
    height = 760
    chart_w = 460
    left_margin = 220

    y0 = 90
    row_h = 38

    lines = []
    add = lines.append
    add('<?xml version="1.0" encoding="UTF-8"?>')
    add(f'<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" viewBox="0 0 {width} {height}">')
    add('<rect width="100%" height="100%" fill="#ffffff"/>')
    add('<style>')
    add('text{font-family:Menlo,Consolas,monospace;fill:#1f2937;font-size:14px;}')
    add('.title{font-size:22px;font-weight:700;}')
    add('.subtitle{font-size:13px;fill:#4b5563;}')
    add('</style>')
    add(f'<text x="40" y="44" class="title">jw Server Benchmark ({DATE})</text>')
    add('<text x="40" y="66" class="subtitle">Machine: macOS arm64 | Build: release binary | auto-import-history=off</text>')

    # Chart 1: P95 latency
    add('<text x="40" y="96" style="font-size:16px;font-weight:700;">P95 Latency (ms)</text>')
    for idx, r in enumerate(perf_rows):
        y = y0 + 26 + idx * row_h
        bw = _bar_width(r["p95_ms"], max_p95, chart_w)
        add(f'<text x="40" y="{y+14}">{r["scenario"]}</text>')
        add(f'<rect x="{left_margin}" y="{y}" width="{bw:.1f}" height="18" fill="#2563eb"/>')
        add(f'<text x="{left_margin + bw + 8:.1f}" y="{y+14}">{r["p95_ms"]:.2f}</text>')

    # Chart 2: Throughput
    y2 = 300
    add(f'<text x="40" y="{y2-20}" style="font-size:16px;font-weight:700;">Throughput (req/s)</text>')
    for idx, r in enumerate(perf_rows):
        y = y2 + 26 + idx * row_h
        bw = _bar_width(r["throughput_rps"], max_tps, chart_w)
        add(f'<text x="40" y="{y+14}">{r["scenario"]}</text>')
        add(f'<rect x="{left_margin}" y="{y}" width="{bw:.1f}" height="18" fill="#16a34a"/>')
        add(f'<text x="{left_margin + bw + 8:.1f}" y="{y+14}">{r["throughput_rps"]:.1f}</text>')

    # Chart 3: Peak RSS (all scenarios)
    y3 = 510
    add(f'<text x="40" y="{y3-20}" style="font-size:16px;font-weight:700;">Peak Memory RSS (MB)</text>')
    for idx, r in enumerate(rows):
        y = y3 + 26 + idx * 28
        bw = _bar_width(r["peak_rss_mb"], max_rss, chart_w)
        add(f'<text x="40" y="{y+13}">{r["scenario"]}</text>')
        add(f'<rect x="{left_margin}" y="{y}" width="{bw:.1f}" height="16" fill="#f59e0b"/>')
        add(f'<text x="{left_margin + bw + 8:.1f}" y="{y+13}">{r["peak_rss_mb"]:.2f}</text>')

    add('</svg>')
    SVG_PATH.write_text("\n".join(lines), encoding="utf-8")


def main():
    build_binary()
    proc, logf = start_server()
    try:
        wait_health()
        sampler = ResourceSampler(proc.pid)
        sampler.start()

        rows = []

        for scenario, count, conc in SCENARIOS:
            sampler.begin_window()
            if scenario.startswith("idle"):
                res = run_idle(30)
            elif scenario.startswith("health"):
                res = run_health(count, conc)
            elif scenario.startswith("record"):
                res = run_record(count, conc)
            elif scenario.startswith("jump"):
                res = run_jump(count, conc)
            else:
                raise RuntimeError(f"unknown scenario: {scenario}")
            stats = sampler.end_window()
            row = {
                "scenario": scenario,
                "requests": res["requests"],
                "concurrency": conc,
                "success": res["success"],
                "duration_sec": round(res["duration_sec"], 4),
                "throughput_rps": round(res["throughput_rps"], 2),
                "avg_ms": round(res["avg_ms"], 3),
                "p50_ms": round(res["p50_ms"], 3),
                "p95_ms": round(res["p95_ms"], 3),
                "avg_cpu_pct": round(stats["avg_cpu"], 2),
                "peak_cpu_pct": round(stats["peak_cpu"], 2),
                "avg_rss_mb": round(stats["avg_rss_mb"], 2),
                "peak_rss_mb": round(stats["peak_rss_mb"], 2),
            }
            rows.append(row)
            print(json.dumps(row, ensure_ascii=False))

        sampler.stop()

        write_csv(rows)
        write_svg(rows)
        print(f"CSV: {CSV_PATH}")
        print(f"SVG: {SVG_PATH}")
    finally:
        stop_server(proc, logf)


if __name__ == "__main__":
    main()
