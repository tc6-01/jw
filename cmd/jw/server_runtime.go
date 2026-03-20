package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	autoImportInterval = 10 * time.Minute
	autoImportBatch    = 300
)

type appConfig struct {
	AutoImportHistory bool  `json:"auto_import_history"`
	LastChromeVisitUS int64 `json:"last_chrome_visit_us,omitempty"`
}

type serverState struct {
	PID       int    `json:"pid"`
	Addr      string `json:"addr"`
	StartedAt int64  `json:"started_at"`
}

type historyRow struct {
	URL       string
	Title     string
	LastVisit int64
}

func handleConfig(args []string) {
	switch {
	case len(args) == 0:
		cfg, _, err := loadAppConfig()
		if err != nil {
			fmt.Printf("读取配置失败: %v\n", err)
			os.Exit(1)
		}
		status := "off"
		if cfg.AutoImportHistory {
			status = "on"
		}
		fmt.Printf("auto-import-history=%s\n", status)
	case len(args) == 2 && args[0] == "auto-import-history":
		enabled, ok := parseOnOff(args[1])
		if !ok {
			fmt.Println("用法: jw config auto-import-history <on|off>")
			os.Exit(1)
		}

		cfg, cfgPath, err := loadAppConfig()
		if err != nil {
			fmt.Printf("读取配置失败: %v\n", err)
			os.Exit(1)
		}
		cfg.AutoImportHistory = enabled
		if err := saveAppConfig(cfgPath, cfg); err != nil {
			fmt.Printf("写入配置失败: %v\n", err)
			os.Exit(1)
		}
		if enabled {
			fmt.Println("已开启自动导入浏览器记录")
		} else {
			fmt.Println("已关闭自动导入浏览器记录")
		}
	default:
		fmt.Println("用法:")
		fmt.Println("  jw config                              查看当前配置")
		fmt.Println("  jw config auto-import-history on|off   配置自动导入浏览器记录")
		os.Exit(1)
	}
}

func loadAppConfig() (appConfig, string, error) {
	cfgPath, err := appConfigPath()
	if err != nil {
		return appConfig{}, "", err
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return appConfig{}, cfgPath, nil
		}
		return appConfig{}, "", err
	}

	if len(data) == 0 {
		return appConfig{}, cfgPath, nil
	}

	var cfg appConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return appConfig{}, "", err
	}
	return cfg, cfgPath, nil
}

func saveAppConfig(path string, cfg appConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func parseOnOff(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "true", "1", "enable", "enabled", "yes":
		return true, true
	case "off", "false", "0", "disable", "disabled", "no":
		return false, true
	default:
		return false, false
	}
}

func runServerForeground(addr string, internalChild bool) {
	if addr == "" {
		autoAddr, err := pickFreeLocalAddr()
		if err != nil {
			fmt.Printf("分配空闲端口失败: %v\n", err)
			os.Exit(1)
		}
		addr = autoAddr
	}

	cfg, cfgPath, err := loadAppConfig()
	if err != nil {
		fmt.Printf("读取配置失败: %v\n", err)
		os.Exit(1)
	}

	if !internalChild {
		fmt.Printf("jw server 已启动: http://%s\n", addr)
		fmt.Println("可用接口: GET /health, POST /record, GET /jump?q=<keyword>")
		if cfg.AutoImportHistory {
			fmt.Println("自动导入浏览器记录: 已启用")
		} else {
			fmt.Println("自动导入浏览器记录: 未启用（jw config auto-import-history on）")
		}
	}

	if cfg.AutoImportHistory {
		go runAutoImportLoop(cfgPath)
	}

	if err := http.ListenAndServe(addr, newServerMux()); err != nil {
		fmt.Printf("server 退出: %v\n", err)
		os.Exit(1)
	}
}

func runAutoImportLoop(cfgPath string) {
	if imported, err := importChromeHistoryOnce(cfgPath); err != nil {
		fmt.Printf("自动导入失败: %v\n", err)
	} else if imported > 0 {
		fmt.Printf("自动导入完成: +%d 条记录\n", imported)
	}

	ticker := time.NewTicker(autoImportInterval)
	defer ticker.Stop()

	for range ticker.C {
		if imported, err := importChromeHistoryOnce(cfgPath); err != nil {
			fmt.Printf("自动导入失败: %v\n", err)
		} else if imported > 0 {
			fmt.Printf("自动导入完成: +%d 条记录\n", imported)
		}
	}
}

func importChromeHistoryOnce(cfgPath string) (int, error) {
	cfg, _, err := loadAppConfig()
	if err != nil {
		return 0, err
	}

	rows, latestVisit, err := readChromeHistoryRows(cfg.LastChromeVisitUS, autoImportBatch)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}

	serverStoreMu.Lock()
	defer serverStoreMu.Unlock()

	path, db, err := loadDB()
	if err != nil {
		return 0, err
	}

	imported := 0
	for _, row := range rows {
		visitedAt := chromeVisitTimeToUnix(row.LastVisit)
		if _, err := db.AddAuto(row.URL, row.Title, visitedAt); err != nil {
			continue
		}
		imported++
	}

	if imported > 0 {
		if err := saveDB(path, db); err != nil {
			return 0, err
		}
	}

	if latestVisit > cfg.LastChromeVisitUS {
		cfg.LastChromeVisitUS = latestVisit
		if err := saveAppConfig(cfgPath, cfg); err != nil {
			return imported, err
		}
	}

	return imported, nil
}

func readChromeHistoryRows(sinceVisitUS int64, limit int) ([]historyRow, int64, error) {
	historyPath, err := detectChromeHistoryPath()
	if err != nil {
		return nil, 0, err
	}
	if _, err := exec.LookPath("sqlite3"); err != nil {
		return nil, 0, errors.New("未找到 sqlite3，无法读取浏览器历史")
	}

	tmp, err := os.CreateTemp("", "jw-history-*.db")
	if err != nil {
		return nil, 0, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	if err := copyFile(historyPath, tmpPath); err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = autoImportBatch
	}

	separator := string(rune(31))
	query := fmt.Sprintf(
		"SELECT url, COALESCE(title, ''), last_visit_time FROM urls WHERE url LIKE 'http%%' AND last_visit_time > %d ORDER BY last_visit_time ASC LIMIT %d;",
		sinceVisitUS,
		limit,
	)
	out, err := exec.Command("sqlite3", "-readonly", "-separator", separator, tmpPath, query).Output()
	if err != nil {
		return nil, 0, err
	}

	rows := make([]historyRow, 0, limit)
	latestVisit := sinceVisitUS
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, separator, 3)
		if len(parts) != 3 {
			continue
		}
		visit, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			continue
		}
		row := historyRow{
			URL:       strings.TrimSpace(parts[0]),
			Title:     strings.TrimSpace(parts[1]),
			LastVisit: visit,
		}
		if row.URL == "" {
			continue
		}
		rows = append(rows, row)
		if visit > latestVisit {
			latestVisit = visit
		}
	}
	return rows, latestVisit, nil
}

func detectChromeHistoryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default", "History"),
			filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Profile 1", "History"),
		}
	default:
		candidates = []string{
			filepath.Join(home, ".config", "google-chrome", "Default", "History"),
			filepath.Join(home, ".config", "chromium", "Default", "History"),
		}
	}

	for _, p := range candidates {
		info, err := os.Stat(p)
		if err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("未找到 Chrome History 文件")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func chromeVisitTimeToUnix(chromeVisitUS int64) int64 {
	// Chrome stores visit time as microseconds since 1601-01-01 UTC.
	const chromeToUnixOffsetSec = int64(11644473600)
	if chromeVisitUS <= 0 {
		return time.Now().Unix()
	}
	unixSec := chromeVisitUS/1_000_000 - chromeToUnixOffsetSec
	if unixSec <= 0 {
		return time.Now().Unix()
	}
	return unixSec
}

func startServerInBackground(addr string) {
	state, running, err := readServerState()
	if err != nil {
		fmt.Printf("读取后台状态失败: %v\n", err)
		os.Exit(1)
	}
	if running && !isServerHealthy(state.Addr) {
		_ = os.Remove(mustServerStatePath())
		running = false
	}
	if running {
		fmt.Printf("后台 server 已在运行: pid=%d addr=http://%s\n", state.PID, state.Addr)
		return
	}

	if addr == "" {
		autoAddr, err := pickFreeLocalAddr()
		if err != nil {
			fmt.Printf("分配空闲端口失败: %v\n", err)
			os.Exit(1)
		}
		addr = autoAddr
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("获取可执行文件路径失败: %v\n", err)
		os.Exit(1)
	}

	logPath, err := serverLogPath()
	if err != nil {
		fmt.Printf("生成日志路径失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		fmt.Printf("创建日志目录失败: %v\n", err)
		os.Exit(1)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Printf("打开日志文件失败: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "server", "_run", addr)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("后台启动失败: %v\n", err)
		os.Exit(1)
	}

	state = serverState{
		PID:       cmd.Process.Pid,
		Addr:      addr,
		StartedAt: time.Now().Unix(),
	}
	if err := writeServerState(state); err != nil {
		fmt.Printf("写入后台状态失败: %v\n", err)
		os.Exit(1)
	}

	if err := waitForServerReady(state); err != nil {
		_ = os.Remove(mustServerStatePath())
		_ = stopProcess(state.PID)
		fmt.Printf("后台启动失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("后台 server 已启动: pid=%d addr=http://%s\n", state.PID, state.Addr)
	fmt.Printf("日志文件: %s\n", logPath)
}

func stopServerInBackground() {
	state, running, err := readServerState()
	if err != nil {
		fmt.Printf("读取后台状态失败: %v\n", err)
		os.Exit(1)
	}
	if !running {
		fmt.Println("后台 server 未运行")
		return
	}
	if !isServerHealthy(state.Addr) {
		_ = os.Remove(mustServerStatePath())
		fmt.Println("后台状态已过期，未发现可用 jw server")
		return
	}

	if err := stopProcess(state.PID); err != nil {
		fmt.Printf("停止后台 server 失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.Remove(mustServerStatePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Printf("清理后台状态失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("后台 server 已停止")
}

func printServerStatus() {
	state, running, err := readServerState()
	if err != nil {
		fmt.Printf("读取后台状态失败: %v\n", err)
		os.Exit(1)
	}
	if !running {
		fmt.Println("后台 server 未运行")
		return
	}

	health := "unhealthy"
	if isServerHealthy(state.Addr) {
		health = "healthy"
	}
	startedAt := time.Unix(state.StartedAt, 0).Format(time.RFC3339)
	fmt.Printf("后台 server 运行中: pid=%d addr=http://%s status=%s started_at=%s\n", state.PID, state.Addr, health, startedAt)
}

func readServerState() (serverState, bool, error) {
	path := mustServerStatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return serverState{}, false, nil
		}
		return serverState{}, false, err
	}

	var state serverState
	if err := json.Unmarshal(data, &state); err != nil {
		return serverState{}, false, err
	}
	if state.PID <= 0 || strings.TrimSpace(state.Addr) == "" {
		return serverState{}, false, errors.New("后台状态文件损坏")
	}
	if !isProcessAlive(state.PID) {
		_ = os.Remove(path)
		return serverState{}, false, nil
	}
	return state, true, nil
}

func writeServerState(state serverState) error {
	path := mustServerStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func waitForServerReady(state serverState) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !isProcessAlive(state.PID) {
			return errors.New("子进程提前退出")
		}
		if isServerHealthy(state.Addr) {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return errors.New("健康检查超时")
}

func isServerHealthy(addr string) bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get("http://" + addr + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func stopProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if !isProcessAlive(pid) {
		return nil
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := proc.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrProcessDone) {
		return false
	}
	return !errors.Is(err, syscall.ESRCH)
}

func appConfigPath() (string, error) {
	base, err := jwBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "config.json"), nil
}

func serverStatePath() (string, error) {
	base, err := jwBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "server-state.json"), nil
}

func serverLogPath() (string, error) {
	base, err := jwBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "server.log"), nil
}

func jwBaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".jw"), nil
}

func mustServerStatePath() string {
	path, err := serverStatePath()
	if err == nil {
		return path
	}
	return filepath.Join(".jw", "server-state.json")
}
