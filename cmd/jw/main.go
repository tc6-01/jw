package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"jw/internal/app/localstore"
	"jw/internal/domain/ranker"
	"jw/internal/domain/urlnorm"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "help", "-h", "--help":
		printHelp()
	case "normalize":
		handleNormalize(args)
	case "score-demo":
		handleScoreDemo(args)
	case "tutorial":
		handleTutorial()
	case "about":
		handleAbout()
	case "config":
		handleConfig(args)
	case "server":
		handleServer(args)
	case "add":
		handleAdd(args)
	case "query":
		handleQuery(args)
	case "jump":
		handleJump(args)
	case "list":
		handleList()
	case "rm":
		handleRemove(args)
	default:
		// 默认把首个参数当作关键词，支持 `jw github docs` 这种快速跳转。
		handleJump(append([]string{cmd}, args...))
	}
}

func printHelp() {
	fmt.Println("jw - 终端网页快速跳转工具")
	fmt.Println("")
	fmt.Println("产品介绍: jw about")
	fmt.Println("快速上手: jw tutorial")
	fmt.Println("")
	fmt.Println("命令入口:")
	fmt.Println("  jw server [addr]         前台启动本地记录服务（自动选空闲端口）")
	fmt.Println("  jw server start [addr]   后台启动本地记录服务")
	fmt.Println("  jw server stop|status    停止/查看后台服务")
	fmt.Println("  jw config                查看当前配置")
	fmt.Println("  jw config auto-import-history on|off")
	fmt.Println("  jw add <url> [title]     手动添加或更新网址记录")
	fmt.Println("  jw query <keyword>       查看候选结果")
	fmt.Println("  jw jump <keyword>        跳转最佳匹配")
	fmt.Println("  jw <keyword>             关键词快速跳转（等价于 jw jump <keyword>）")
	fmt.Println("  jw list                  查看本地记录")
	fmt.Println("  jw rm <url|title>        删除一条记录")
	fmt.Println("  jw tutorial              运行 30 秒可执行教程")
	fmt.Println("  jw about                 查看产品介绍与上手路径")
}

func handleNormalize(args []string) {
	if len(args) != 1 {
		fmt.Println("用法: jw normalize <url>")
		os.Exit(1)
	}

	normalized, err := urlnorm.NormalizeAndRedact(args[0])
	if err != nil {
		fmt.Printf("normalize 失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(normalized)
}

func handleScoreDemo(args []string) {
	if len(args) != 2 {
		fmt.Println("用法: jw score-demo <daysAgo> <keywordScore0to1>")
		os.Exit(1)
	}

	daysAgo, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		fmt.Printf("daysAgo 非法: %v\n", err)
		os.Exit(1)
	}
	kw, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		fmt.Printf("keywordScore 非法: %v\n", err)
		os.Exit(1)
	}

	params := ranker.DefaultParams()
	events := []ranker.Event{{DaysAgo: daysAgo, AliasWeight: 1.0}}
	ctx := ranker.Context{DirMatches: 1, KeywordScore: kw, TimeBucketMatches: 1, AliasBoost: 1.0}
	fmt.Printf("score=%.6f\n", ranker.Score(events, ctx, params))
}

func handleTutorial() {
	fmt.Println("jw 可执行教程")
	fmt.Println("")
	fmt.Println("第 1 步：启动本地记录服务")
	fmt.Println("  jw server")
	fmt.Println("")
	fmt.Println("第 2 步：录入常用网址")
	fmt.Println("  jw add https://github.com GitHub")
	fmt.Println("  jw add https://docs.github.com GitHubDocs")
	fmt.Println("")
	fmt.Println("第 3 步：快速跳转")
	fmt.Println("  jw github")
	fmt.Println("")
	fmt.Println("第 4 步：查看与清理")
	fmt.Println("  jw list")
	fmt.Println("  jw rm GitHubDocs")
}

func handleAbout() {
	fmt.Println("jw - 终端网页快速跳转工具")
	fmt.Println("zoxide-like for web：把常用网页记在本地，用关键词快速跳转。")
	fmt.Println("")
	fmt.Println("30 秒上手:")
	fmt.Println("  jw tutorial")
	fmt.Println("  jw add https://github.com GitHub")
	fmt.Println("  jw github")
	fmt.Println("")
	fmt.Println("需要完整命令入口请运行: jw help")
	fmt.Println("本地数据路径: ~/.jw/store.json")
	fmt.Println("更多说明: README.md")
	fmt.Println("Repo: https://github.com/tc6-01/jw")
	fmt.Println("License: MIT")
}

func loadDB() (string, *localstore.DB, error) {
	path, err := localstore.DefaultPath()
	if err != nil {
		return "", nil, err
	}
	db, err := localstore.Load(path)
	if err != nil {
		return "", nil, err
	}
	return path, db, nil
}

func saveDB(path string, db *localstore.DB) error {
	return db.Save(path)
}

func handleAdd(args []string) {
	if len(args) < 1 {
		fmt.Println("用法: jw add <url> [title]")
		os.Exit(1)
	}

	title := ""
	if len(args) > 1 {
		title = strings.Join(args[1:], " ")
	}

	path, db, err := loadDB()
	if err != nil {
		fmt.Printf("初始化存储失败: %v\n", err)
		os.Exit(1)
	}
	entry, err := db.Add(args[0], title)
	if err != nil {
		fmt.Printf("add 失败: %v\n", err)
		os.Exit(1)
	}
	if err := saveDB(path, db); err != nil {
		fmt.Printf("写入存储失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("已记录: %s\n", entry.URL)
}

func handleQuery(args []string) {
	if len(args) < 1 {
		fmt.Println("用法: jw query <keyword>")
		os.Exit(1)
	}

	keyword := strings.Join(args, " ")
	_, db, err := loadDB()
	if err != nil {
		fmt.Printf("初始化存储失败: %v\n", err)
		os.Exit(1)
	}
	matches := db.Query(keyword, 5)
	if len(matches) == 0 {
		fmt.Println("没有匹配结果")
		return
	}

	for i, m := range matches {
		title := m.Entry.Title
		if title == "" {
			title = "(无标题)"
		}
		fmt.Printf("%d) %s\n   %s\n   score=%.4f count=%d\n", i+1, title, m.Entry.URL, m.Score, m.Entry.Count)
	}
}

func handleJump(args []string) {
	if len(args) < 1 {
		fmt.Println("用法: jw jump <keyword> 或 jw <keyword>")
		os.Exit(1)
	}

	keyword := strings.Join(args, " ")
	path, db, err := loadDB()
	if err != nil {
		fmt.Printf("初始化存储失败: %v\n", err)
		os.Exit(1)
	}
	best, err := db.Best(keyword)
	if err != nil {
		if err == localstore.ErrNoMatch {
			fmt.Println("没有匹配结果")
			os.Exit(1)
		}
		fmt.Printf("jump 失败: %v\n", err)
		os.Exit(1)
	}

	if _, err := db.Add(best.Entry.URL, best.Entry.Title); err == nil {
		if err := saveDB(path, db); err != nil {
			fmt.Printf("写入存储失败: %v\n", err)
		}
	}

	if err := openURL(best.Entry.URL); err != nil {
		fmt.Printf("打开失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("已跳转: %s\n", best.Entry.URL)
}

func handleList() {
	_, db, err := loadDB()
	if err != nil {
		fmt.Printf("初始化存储失败: %v\n", err)
		os.Exit(1)
	}
	if len(db.Entries) == 0 {
		fmt.Println("暂无记录")
		return
	}
	for i, e := range db.Entries {
		title := e.Title
		if title == "" {
			title = "(无标题)"
		}
		fmt.Printf("%d) %s\n   %s\n   count=%d\n", i+1, title, e.URL, e.Count)
	}
}

func handleRemove(args []string) {
	if len(args) < 1 {
		fmt.Println("用法: jw rm <url|title>")
		os.Exit(1)
	}

	target := strings.Join(args, " ")
	path, db, err := loadDB()
	if err != nil {
		fmt.Printf("初始化存储失败: %v\n", err)
		os.Exit(1)
	}
	if !db.Remove(target) {
		fmt.Println("未找到要删除的记录")
		os.Exit(1)
	}
	if err := saveDB(path, db); err != nil {
		fmt.Printf("写入存储失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("删除成功")
}

func handleServer(args []string) {
	if len(args) == 0 {
		runServerForeground("", false)
		return
	}

	switch args[0] {
	case "start":
		if len(args) > 2 {
			fmt.Println("用法: jw server start [addr]")
			os.Exit(1)
		}
		addr := ""
		if len(args) == 2 {
			addr = args[1]
		}
		startServerInBackground(addr)
	case "stop":
		if len(args) != 1 {
			fmt.Println("用法: jw server stop")
			os.Exit(1)
		}
		stopServerInBackground()
	case "status":
		if len(args) != 1 {
			fmt.Println("用法: jw server status")
			os.Exit(1)
		}
		printServerStatus()
	case "_run":
		if len(args) > 2 {
			fmt.Println("用法: jw server _run [addr]")
			os.Exit(1)
		}
		addr := ""
		if len(args) == 2 {
			addr = args[1]
		}
		runServerForeground(addr, true)
	default:
		if len(args) > 1 {
			fmt.Println("用法: jw server [addr]")
			fmt.Println("或:  jw server start [addr]")
			os.Exit(1)
		}
		runServerForeground(args[0], false)
	}
}

func pickFreeLocalAddr() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer ln.Close()
	return ln.Addr().String(), nil
}

var serverStoreMu sync.Mutex

func newServerMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})

	mux.HandleFunc("/record", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var body struct {
			URL   string `json:"url"`
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		serverStoreMu.Lock()
		defer serverStoreMu.Unlock()

		path, db, err := loadDB()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		entry, err := db.Add(body.URL, body.Title)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := saveDB(path, db); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "url": entry.URL})
	})

	mux.HandleFunc("/jump", func(w http.ResponseWriter, r *http.Request) {
		keyword := strings.TrimSpace(r.URL.Query().Get("q"))
		if keyword == "" {
			http.Error(w, "missing q", http.StatusBadRequest)
			return
		}

		serverStoreMu.Lock()
		defer serverStoreMu.Unlock()

		path, db, err := loadDB()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		best, err := db.Best(keyword)
		if err != nil {
			http.Error(w, "no match", http.StatusNotFound)
			return
		}

		if _, err := db.Add(best.Entry.URL, best.Entry.Title); err == nil {
			_ = saveDB(path, db)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "url": best.Entry.URL, "score": best.Score})
	})
	return mux
}

func openURL(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}
