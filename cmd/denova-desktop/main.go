package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/api"
	"denova/internal/app"
	"denova/internal/buildinfo"
	"denova/internal/observability"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func main() {
	var (
		workspace string
		devMode   bool
	)
	if hasVersionArg(os.Args[1:]) {
		fmt.Println(buildinfo.Version)
		return
	}

	cfg := config.Load()
	port := defaultPort(cfg)
	flag.StringVar(&workspace, "workspace", "", "作品工作目录 (默认恢复上次打开的书籍)")
	flag.StringVar(&port, "port", port, "内部 HTTP 服务端口")
	flag.BoolVar(&devMode, "dev-mode", false, "开发启动模式：开启开发诊断能力")
	flag.Parse()

	cfg.DevMode = devMode
	agent.SetModelInputLoggingEnabled(cfg.DevMode && cfg.LLMInputLogEnabled)
	agent.SetTraceRuntimeConfig(cfg.TraceCaptureLevel, cfg.TraceExporter, cfg.TraceRetentionRuns)

	logPath, closeLog := setupLogging("./log")
	defer closeLog()
	observability.ConfigureStructuredLogging()
	log.Printf("[desktop] 日志输出已启用 dir=./log current_file=%s", logPath)

	// 桌面模式强制使用本地回环，不暴露到局域网
	port = selectDesktopPort(port)
	if runtimeWebPort, err := strconv.Atoi(port); err == nil {
		cfg.RuntimeWebPort = runtimeWebPort
	}

	if workspace != "" {
		cfg.Workspace = workspace
		cfg.ResumeLastWorkspace = false
	} else if workspaceEnv := os.Getenv("DENOVA_WORKSPACE"); workspaceEnv != "" {
		cfg.Workspace = workspaceEnv
		cfg.ResumeLastWorkspace = false
	}

	cfg.SkillsDir = resolveSkillsDir(cfg.SkillsDir)

	// 桌面模式使用前端生产构建产物（web/dist），而非 Vite 开发入口
	resolveWebDistDir()

	ctx := context.Background()

	// 初始化应用运行时
	application_, err := app.New(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化应用失败: %v\n", err)
		os.Exit(1)
	}
	defer application_.Close()

	// 启动内部 HTTP 服务（仅监听 127.0.0.1）
	srv := api.NewServer(application_, port)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[desktop] HTTP 服务 panic: %v", r)
			}
		}()
		srv.Run()
	}()

	// 等待 HTTP 服务就绪
	localURL := fmt.Sprintf("http://127.0.0.1:%s", port)
	if !waitForServer(localURL, 10*time.Second) {
		fmt.Fprintf(os.Stderr, "HTTP 服务启动超时: %s\n", localURL)
		os.Exit(1)
	}

	log.Printf("[desktop] 内部 HTTP 服务就绪: %s", localURL)
	fmt.Printf("\n  Denova AI 创作工具 (桌面版)\n")
	fmt.Printf("  ─────────────────────────\n")
	fmt.Printf("  内部服务: %s\n", localURL)
	fmt.Printf("  作品目录: %s\n", application_.Workspace())
	fmt.Printf("  关闭窗口即退出应用\n\n")

	// 创建 Wails 桌面应用
	// 注意：Wails v3 的 WebviewWindowOptions.URL 指向外部 http:// 地址时页面不会加载
	// （见 wailsapp/wails#3810）。因此改用 Assets.Handler 反向代理方案：
	// WebView 通过 wails:// 协议加载，所有请求（静态资源 + /api + SSE）
	// 都代理到内部 Hertz 服务器，保持同源、零侵入复用全部现有 HTTP API。
	wailsApp := application.New(application.Options{
		Name:        "Denova",
		Description: "Denova AI 创作工具 - 桌面版",
		Assets: application.AssetOptions{
			Handler: newReverseProxy(localURL),
		},
		Linux: application.LinuxOptions{
			ProgramName: "denova",
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		OnShutdown: func() {
			log.Printf("[desktop] 应用关闭，清理资源...")
			application_.Close()
		},
	})

	// 创建主窗口：无边框（Frameless）模式，由前端自定义标题栏提供
	// 窗口控件与拖拽（见 web/src/components/desktop/DesktopTitleBar.tsx）。
	// 页面通过 Wails 资产服务器（wails:// 协议）加载，请求由 newReverseProxy
	// 转发到内部 Hertz 服务。
	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "Denova",
		URL:       "/",
		Width:     1280,
		Height:    860,
		MinWidth:  900,
		MinHeight: 600,
		Frameless: true,
		Mac: application.MacWindow{
			Appearance: application.NSAppearanceNameDarkAqua,
			TitleBar:   application.MacTitleBarDefault,
		},
	})

	// 运行桌面应用（阻塞直到窗口关闭）
	if err := wailsApp.Run(); err != nil {
		log.Printf("[desktop] 应用运行错误: %v", err)
		os.Exit(1)
	}
}

// waitForServer 轮询等待 HTTP 服务可用
func waitForServer(url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url + "/api/status")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// newReverseProxy 构建一个反向代理 http.Handler，将 Wails 资产服务器
// 收到的所有请求转发到内部 Hertz 服务（target 形如 http://127.0.0.1:8080）。
// FlushInterval=-1 保证 SSE 流式响应逐块即时刷新，不被缓冲。
func newReverseProxy(target string) http.Handler {
	targetURL, err := url.Parse(target)
	if err != nil {
		log.Printf("[desktop] 反向代理目标 URL 解析失败: %v", err)
		return http.NotFoundHandler()
	}
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.FlushInterval = -1
	// Wails/WebKitGTK 传入的 RemoteAddr 可能不是回环地址，默认 director 会将其
	// 写入 X-Forwarded-For，导致 Hertz 的 remoteAccessMiddleware 误判为局域网客户端
	// 而返回“当前未开启局域网访问”。桌面版本就是本地单用户应用，强制将
	// X-Forwarded-For 设为回环地址，确保始终被判定为本地请求。
	defaultDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		defaultDirector(req)
		req.Header.Set("X-Forwarded-For", "127.0.0.1")
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[desktop] 代理请求失败 path=%s err=%v", r.URL.Path, err)
		http.Error(w, "proxy error: "+err.Error(), http.StatusBadGateway)
	}
	// WebView 传入的请求体是未知长度流（ContentLength==0），ReverseProxy 在
	// Director 运行前就会丢弃这类 body（outreq.Body = nil），因此必须在
	// 进入代理前先规范化请求体。
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ensureRequestBody(r)
		proxy.ServeHTTP(w, r)
	})
}

// ensureRequestBody 修复 WebView 传入的“未知长度”请求体。
// Wails 资产服务器用平台流（Windows IStream / Linux GInputStream）构造请求，
// body 是泛型 io.ReadCloser，http.NewRequestWithContext 无法推断长度，
// ContentLength 保持 0 且请求头不含 Content-Length；而 httputil.ReverseProxy
// 将 ContentLength==0 的请求视为无请求体（outreq.Body = nil，见
// net/http/httputil/reverseproxy.go，且发生在 Director 之前），导致后端
// 收到空 POST/PUT、BindJSON 失败并返回“请求参数无效”。这里在请求进入
// 代理前将其完整读出并重写为定长缓冲，保证 body 完整转发
// （桌面应用请求体均有上限，如封面上传 16MB，全量缓存可接受）。
func ensureRequestBody(req *http.Request) {
	if req.Body == nil || req.Body == http.NoBody || req.ContentLength > 0 {
		return
	}
	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		log.Printf("[desktop] 读取 WebView 请求体失败 path=%s err=%v", req.URL.Path, err)
		return
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))
}

func hasVersionArg(args []string) bool {
	for _, arg := range args {
		if arg == "--version" || arg == "-version" {
			return true
		}
	}
	return false
}

func defaultPort(cfg *config.Config) string {
	if cfg != nil && cfg.BackendPort > 0 {
		return strconv.Itoa(cfg.BackendPort)
	}
	return "8080"
}

// selectDesktopPort 确保桌面模式使用可用端口
func selectDesktopPort(preferred string) string {
	if portAvailable(preferred) {
		return preferred
	}
	next, err := findAvailablePort(preferred, 20)
	if err != nil {
		log.Printf("[desktop] 端口 %s 不可用且自动选择失败: %v", preferred, err)
		return preferred
	}
	fmt.Fprintf(os.Stderr, "提示: 端口 %s 已被占用，已自动改用 %s\n", preferred, next)
	return next
}

func findAvailablePort(preferred string, attempts int) (string, error) {
	start, err := strconv.Atoi(preferred)
	if err != nil || start <= 0 || start > 65535 {
		return "", fmt.Errorf("端口号无效: %s", preferred)
	}
	for p := start + 1; p <= 65535 && p <= start+attempts; p++ {
		candidate := strconv.Itoa(p)
		if portAvailable(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("未找到可用端口: %d-%d", start+1, start+attempts)
}

func portAvailable(port string) bool {
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func resolveSkillsDir(configured string) string {
	if dir := existingDir(configured); dir != "" {
		return dir
	}
	candidates := []string{
		"./skills",
		bundledDir("skills"),
		bundledOneUpDir("skills"),
		bundledParentDir("skills"),
	}
	for _, c := range candidates {
		if dir := existingDir(c); dir != "" {
			return dir
		}
	}
	return configured
}

// bundledDir 返回可执行文件同级的 name 目录（发布包布局: <bundle>/denova-desktop + <bundle>/skills）。
// 必须用 filepath.Dir 而不是手写分隔符扫描：Windows 上 os.Executable() 返回反斜杠路径。
func bundledDir(name string) string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), name)
	}
	return ""
}

// bundledOneUpDir 返回可执行文件上一级的 name 目录
// （build-desktop.sh 布局: exe 在 <root>/dist/，资源在 <root>/skills、<root>/web/dist）。
func bundledOneUpDir(name string) string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "..", name)
	}
	return ""
}

func bundledParentDir(name string) string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "..", "..", name)
	}
	return ""
}

func existingDir(path string) string {
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	fi, err := os.Stat(clean)
	if err == nil && fi.IsDir() {
		if abs, err := filepath.Abs(clean); err == nil {
			return abs
		}
		return clean
	}
	return ""
}

// resolveWebDistDir 确保 DENOVA_WEB_DIR 指向前端生产构建产物目录。
// 桌面模式不使用 Vite dev server，必须加载 web/dist 中的编译后静态资源。
func resolveWebDistDir() {
	// 如果用户已显式设置，尊重用户配置
	if os.Getenv("DENOVA_WEB_DIR") != "" {
		return
	}
	candidates := []string{
		"web/dist",
		bundledDir("web/dist"),
		bundledOneUpDir("web/dist"),
		bundledParentDir("web/dist"),
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		indexPath := filepath.Join(c, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			abs, _ := filepath.Abs(c)
			os.Setenv("DENOVA_WEB_DIR", abs)
			log.Printf("[desktop] 前端静态资源目录: %s", abs)
			return
		}
	}
	log.Printf("[desktop] 警告: 未找到 web/dist 前端构建产物，请先运行 cd web && pnpm build")
}
