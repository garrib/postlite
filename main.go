// Postlite —— 本机轻量 HTTP 调试工具（设计见 docs/design.md）。
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/garrib/postlite/internal/config"
	"github.com/garrib/postlite/internal/cookies"
	"github.com/garrib/postlite/internal/history"
	"github.com/garrib/postlite/internal/server"
)

// atomicWrite temp+rename 原子写（与 history 存储同策略，data.md §1）。
func atomicWrite(path string, raw []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) //nolint:errcheck // rename 成功后 Remove 是 no-op
	if err = tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err = tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("postlite: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ln, port, err := listenLoopback(cfg.Port)
	if err != nil {
		return err
	}

	// 原生壳专用：父进程退出（被 reparent 到 launchd）后自杀，避免孤儿服务。
	if os.Getenv("POSTLITE_WATCH_PARENT") == "1" {
		go watchParent()
	}

	store, err := history.Open(cfg.DataDir)
	if err != nil {
		return err
	}

	// Cookie 自动管理（design.md §3.3b）：进程级 Jar + cookies.json 持久化
	jar := cookies.New()
	if raw, err := os.ReadFile(filepath.Join(cfg.DataDir, "cookies.json")); err == nil {
		if err := jar.Load(raw); err != nil {
			log.Printf("postlite: cookies.json 损坏，忽略（%v）", err)
		}
	}
	persistCookies := func(raw []byte) error {
		return atomicWrite(filepath.Join(cfg.DataDir, "cookies.json"), raw)
	}

	sub, err := fs.Sub(distFS, "frontend/dist")
	if err != nil {
		return err
	}

	srv := &http.Server{Handler: server.New(store, sub, config.Version, jar, persistCookies)}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	addr := fmt.Sprintf("http://127.0.0.1:%d", port)
	log.Printf("Postlite v%s 已启动 → %s（数据目录 %s）", config.Version, addr, cfg.DataDir)

	if cfg.OpenBrowser {
		openBrowser(addr)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

// listenLoopback 仅绑定 127.0.0.1（免登录的安全边界），端口占用则 +1 重试。
func listenLoopback(start int) (net.Listener, int, error) {
	const tries = 50
	for i := 0; i < tries; i++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", start+i))
		if err == nil {
			return ln, start + i, nil
		}
		if i == tries-1 {
			return nil, 0, fmt.Errorf("127.0.0.1:%d 起共 %d 个端口均被占用: %w", start, tries, err)
		}
	}
	return nil, 0, nil // unreachable
}

func openBrowser(url string) {
	var bin string
	switch runtime.GOOS {
	case "darwin":
		bin = "open"
	case "windows":
		// cmd /c start 语义复杂，v0.1 先支持 darwin/linux。
		return
	default:
		bin = "xdg-open"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return
	}
	// 用户手动启动，浏览器打开失败只提示不报错。
	if err := exec.Command(bin, url).Start(); err != nil {
		log.Printf("自动打开浏览器失败（%v），请手动访问 %s", err, url)
	}
}

// watchParent 轮询 ppid：原生壳被强杀（SIGKILL）时本进程会被 reparent，探测到即退出。
func watchParent() {
	ppid := os.Getppid()
	for {
		time.Sleep(2 * time.Second)
		if os.Getppid() != ppid {
			log.Println("postlite: 父进程已退出，服务随之关闭")
			os.Exit(0)
		}
	}
}
