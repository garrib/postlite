package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/garrib/postlite/internal/history"
	"github.com/garrib/postlite/internal/proxy"
)

func newTestServer(t *testing.T, files map[string]string) (http.Handler, *history.Store) {
	t.Helper()
	store, err := history.Open(t.TempDir())
	if err != nil {
		t.Fatalf("history.Open: %v", err)
	}
	dist := fstest.MapFS{}
	for k, v := range files {
		dist[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return New(store, dist, "test"), store
}

// TestIntegrationFlow 发送 → 落历史 → 列表 → 删除 → 清空 闭环（docs/test-cases.md §1 集成）。
func TestIntegrationFlow(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("pong"))
	}))
	defer up.Close()

	h, store := newTestServer(t, map[string]string{"index.html": "<html>app</html>"})
	ts := httptest.NewServer(h)
	defer ts.Close()

	doJSON := func(method, path string, body any) *http.Response {
		t.Helper()
		var rd io.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			rd = bytes.NewReader(raw)
		}
		req, err := http.NewRequest(method, ts.URL+path, rd)
		if err != nil {
			t.Fatalf("newreq: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		return resp
	}

	// 1. health。
	resp := doJSON("GET", "/api/health", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("health=%d", resp.StatusCode)
	}
	var health struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&health)
	resp.Body.Close()
	if !health.OK || health.Version != "test" {
		t.Fatalf("health=%+v", health)
	}

	// 2. 发送并落历史。
	env := proxy.RequestEnvelope{
		Method: "POST",
		URL:    up.URL + "/ping",
		Body:   proxy.Body{Type: "json", Text: `{"x":1}`},
	}
	resp = doJSON("POST", "/api/proxy", env)
	if resp.StatusCode != 200 {
		t.Fatalf("proxy=%d", resp.StatusCode)
	}
	var pres proxy.Response
	_ = json.NewDecoder(resp.Body).Decode(&pres)
	resp.Body.Close()
	if pres.Status != 200 || pres.BodyText != "pong" {
		t.Fatalf("pres=%+v", pres)
	}

	// 3. 历史里应有 1 条。
	resp = doJSON("GET", "/api/history", nil)
	var list struct {
		Items []history.Record `json:"items"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if len(list.Items) != 1 {
		t.Fatalf("应有 1 条历史，得到 %d", len(list.Items))
	}
	rec := list.Items[0]
	if rec.Request.Method != "POST" || rec.Request.URL != up.URL+"/ping" {
		t.Fatalf("rec=%+v", rec.Request)
	}
	if rec.Response == nil || rec.Response.Status != 200 || rec.Response.PreviewText != "pong" {
		t.Fatalf("rec.Response=%+v", rec.Response)
	}
	if rec.Request.Record != nil {
		t.Fatalf("落库信封不应再携带 record 开关")
	}

	// 4. 删除该条（幂等再删一次）。
	if resp = doJSON("DELETE", "/api/history/"+rec.ID, nil); resp.StatusCode != 204 {
		t.Fatalf("delete=%d", resp.StatusCode)
	}
	resp.Body.Close()
	if resp = doJSON("DELETE", "/api/history/nope", nil); resp.StatusCode != 204 {
		t.Fatalf("幂等删除=%d", resp.StatusCode)
	}
	resp.Body.Close()
	if got := store.List(); len(got) != 0 {
		t.Fatalf("应已删除，剩 %d", len(got))
	}

	// 5. 再发一条 record=false，不落历史。
	noRec := env
	f := false
	noRec.Record = &f
	resp = doJSON("POST", "/api/proxy", noRec)
	if resp.StatusCode != 200 {
		t.Fatalf("proxy=%d", resp.StatusCode)
	}
	resp.Body.Close()
	if got := store.List(); len(got) != 0 {
		t.Fatalf("record=false 不应落历史，得到 %d", len(got))
	}
}

// TestProxyErrorEnvelope 错误信封与错误码映射。
func TestProxyErrorEnvelope(t *testing.T) {
	h, store := newTestServer(t, nil)
	ts := httptest.NewServer(h)
	defer ts.Close()

	// 非 JSON 信封 → 400，且不落历史。
	resp, err := http.Post(ts.URL+"/api/proxy", "application/json", strings.NewReader("not-json"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&e)
	if e.Error.Code != "invalid_request" {
		t.Fatalf("code=%q", e.Error.Code)
	}
	if got := store.List(); len(got) != 0 {
		t.Fatalf("400 不应落历史，得到 %d", len(got))
	}

	// 上游拒连 → 502，落历史（error 字段，与 response 互斥）。
	env := proxy.RequestEnvelope{Method: "GET", URL: "http://127.0.0.1:1/x"}
	raw, _ := json.Marshal(env)
	resp2, err := http.Post(ts.URL+"/api/proxy", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 502 {
		t.Fatalf("status=%d", resp2.StatusCode)
	}
	var e2 struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp2.Body).Decode(&e2)
	if e2.Error.Code != "upstream_unreachable" {
		t.Fatalf("code=%q", e2.Error.Code)
	}
	got := store.List()
	if len(got) != 1 {
		t.Fatalf("502 应落 1 条历史，得到 %d", len(got))
	}
	if got[0].Error == "" || got[0].Response != nil {
		t.Fatalf("失败条目应带 error 且无 response: %+v", got[0])
	}
}

// TestStaticAndSPA 静态资源与 SPA 兜底（api.md §5）。
func TestStaticAndSPA(t *testing.T) {
	h, _ := newTestServer(t, map[string]string{
		"index.html":  "<html>app</html>",
		"assets/a.js": "console.log(1)",
	})
	ts := httptest.NewServer(h)
	defer ts.Close()

	for _, tc := range []struct{ path, want string }{
		{"/", "<html>app</html>"},
		{"/assets/a.js", "console.log(1)"},
		{"/some/spa/route", "<html>app</html>"}, // SPA 兜底
	} {
		resp, err := http.Get(ts.URL + tc.path)
		if err != nil {
			t.Fatalf("get %s: %v", tc.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(string(body), tc.want) {
			t.Fatalf("%s → %d %q", tc.path, resp.StatusCode, body)
		}
	}
}

// TestStaticUnbuilt 前端未构建时给内置提示页。
func TestStaticUnbuilt(t *testing.T) {
	h, _ := newTestServer(t, nil)
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "尚未构建") {
		t.Fatalf("未构建提示页不符: %d %q", resp.StatusCode, body)
	}
}

// TestTimeNowInjection timeNow 可注入（编译期类型核对）。
func TestTimeNowInjection(t *testing.T) {
	orig := timeNow
	defer func() { timeNow = orig }()
	timeNow = func() time.Time { return time.UnixMilli(1732531200000) }
	if timeNow().UnixMilli() != 1732531200000 {
		t.Fatalf("注入失败")
	}
}
