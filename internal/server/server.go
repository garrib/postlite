// Package server 提供 HTTP 路由：/api/* 与内嵌静态前端（接口契约见 docs/api.md）。
package server

import (
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/garrib/postlite/internal/history"
	"github.com/garrib/postlite/internal/proxy"
)

// 信封解码上限：单个文件 base64(14MB)≈19MB，留足余量。
const maxEnvelopeBytes = 64 << 20

const unbuiltHTML = `<!doctype html>
<html lang="zh"><head><meta charset="utf-8"><title>Postlite</title>
<style>body{font-family:-apple-system,sans-serif;background:#16181d;color:#d7dae0;display:flex;min-height:100vh;align-items:center;justify-content:center}div{max-width:34em;line-height:1.8}code{background:#22252c;padding:2px 6px;border-radius:4px}</style>
</head><body><div><h2>Postlite 前端尚未构建</h2>
<p>当前二进制只内嵌了占位文件。请在仓库目录执行：</p>
<p><code>make build</code></p>
<p>（或 <code>make build-dist</code> 仅构建前端产物后重新 <code>go build</code>。）</p>
</div></body></html>`

type Server struct {
	hist    *history.Store
	dist    fs.FS
	version string
}

// New 组装全部路由。dist 为前端产物目录（index.html 可缺，缺则回退内置提示页）。
func New(hist *history.Store, dist fs.FS, version string) http.Handler {
	s := &Server{hist: hist, dist: dist, version: version}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/proxy", s.handleProxy)
	mux.HandleFunc("GET /api/history", s.handleHistoryList)
	mux.HandleFunc("DELETE /api/history/{id}", s.handleHistoryDelete)
	mux.HandleFunc("DELETE /api/history", s.handleHistoryClear)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.Handle("/", http.HandlerFunc(s.handleStatic))
	return mux
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	var env proxy.RequestEnvelope
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxEnvelopeBytes))
	if err := dec.Decode(&env); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "请求信封不是合法 JSON: "+err.Error())
		return
	}

	resp, perr := proxy.Execute(env)
	if perr != nil {
		// 传输级失败（502/504）落历史（data.md §2 error 字段）；400 属请求构造错误，不落。
		if env.ShouldRecord() && perr.HTTPStatus != http.StatusBadRequest {
			s.recordHistory(env, nil, perr.Message)
		}
		writeError(w, perr.HTTPStatus, perr.Code, perr.Message)
		return
	}

	if env.ShouldRecord() {
		s.recordHistory(env, &resp, "")
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) recordHistory(env proxy.RequestEnvelope, resp *proxy.Response, sendErr string) {
	rec := history.NewRecord(timeNow(), env, resp, sendErr)
	// 落历史失败不阻塞响应（data.md §4）。
	if err := s.hist.Append(rec); err != nil {
		log.Printf("postlite: 历史落盘失败: %v", err)
	}
}

func (s *Server) handleHistoryList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": s.hist.List()})
}

func (s *Server) handleHistoryDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.hist.Delete(r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleHistoryClear(w http.ResponseWriter, _ *http.Request) {
	if err := s.hist.Clear(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": s.version})
}

// handleStatic 静态资源 + SPA 兜底（api.md §5）。
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	if _, err := fs.Stat(s.dist, name); err == nil {
		http.FileServerFS(s.dist).ServeHTTP(w, r)
		return
	}
	// SPA 兜底：未命中交给 index.html；产物缺失给内置提示页。
	index, err := fs.ReadFile(s.dist, "index.html")
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, unbuiltHTML)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(index)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("postlite: 应答序列化失败: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

// timeNow 单点抽时钟，便于测试注入。
var timeNow = time.Now
