package board

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", "testdata", "orgs", name))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newHandler(t *testing.T, name string) http.Handler {
	t.Helper()
	return (&Server{Vault: fixturePath(t, name), Now: func() time.Time { return fixedNow }}).Handler()
}

// /api/board 出的必须是**完整可吃**的投影：schema 在、项目在、副本落点在。
func TestServerServesBoardJSON(t *testing.T) {
	rec := get(t, newHandler(t, "domains"), "/api/board")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 %d，期望 200：%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type=%q，期望 application/json", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control=%q，看板不该被缓存", cc)
	}
	var v View
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("不是合法 JSON：%v", err)
	}
	if v.Schema != Schema {
		t.Fatalf("schema=%q，期望 %q", v.Schema, Schema)
	}
	if len(v.Projects) != 1 || v.Projects[0].Charter != "charters/trade-q3" {
		t.Fatalf("投影里没有带副本落点的项目：%+v", v.Projects)
	}
}

// 校验发现走 /api/issues，且**不并进投影契约**：anc.board/v2 只在成功时出现。
func TestServerIssues(t *testing.T) {
	t.Run("干净 vault", func(t *testing.T) {
		rec := get(t, newHandler(t, "domains"), "/api/issues")
		if rec.Code != http.StatusOK {
			t.Fatalf("状态码 %d", rec.Code)
		}
		var i Issues
		if err := json.Unmarshal(rec.Body.Bytes(), &i); err != nil {
			t.Fatal(err)
		}
		if !i.OK || len(i.Fatal) != 0 || len(i.Warn) != 0 || i.Error != "" {
			t.Fatalf("干净 vault 不该有任何发现：%+v", i)
		}
	})

	t.Run("红档 vault", func(t *testing.T) {
		rec := get(t, newHandler(t, "broken-validate"), "/api/issues")
		var i Issues
		if err := json.Unmarshal(rec.Body.Bytes(), &i); err != nil {
			t.Fatal(err)
		}
		if i.OK || len(i.Fatal) == 0 {
			t.Fatalf("红档 vault 要把 fatal 端出来：%+v", i)
		}
		for _, f := range i.Fatal {
			if f.Rule == "" || f.Msg == "" {
				t.Fatalf("发现缺规则 id 或文案：%+v", f)
			}
		}
	})

	t.Run("红档 vault 不给投影", func(t *testing.T) {
		rec := get(t, newHandler(t, "broken-validate"), "/api/board")
		if rec.Code == http.StatusOK {
			t.Fatalf("红档时不该给「看起来正常」的视图：%s", rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "ok") {
			t.Fatalf("错误响应应当是 JSON：%s", rec.Body.String())
		}
	})
}

// 只读是硬约束：写方法一律 405，且带 Allow 头 —— 没有写入口，就不存在越权写入口。
func TestServerIsReadOnly(t *testing.T) {
	h := newHandler(t, "domains")
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		for _, p := range []string{"/api/board", "/api/issues", "/"} {
			req := httptest.NewRequest(m, p, strings.NewReader("别动我"))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s 状态码 %d，期望 405", m, p, rec.Code)
			}
			if allow := rec.Header().Get("Allow"); allow != "GET, HEAD" {
				t.Fatalf("%s %s 的 Allow=%q", m, p, allow)
			}
		}
	}
}

// 前端产物必须**真的嵌进去了** —— 这条挡的是「改了 TS 忘了 build / 忘了提交产物」。
func TestServerServesUI(t *testing.T) {
	h := newHandler(t, "domains")

	rec := get(t, h, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("首页状态码 %d", rec.Code)
	}
	html := rec.Body.String()
	if !strings.Contains(html, "<!doctype html") || !strings.Contains(html, `src="/app.js"`) {
		t.Fatalf("首页不像看板外壳：%s", html[:min(200, len(html))])
	}

	rec = get(t, h, "/app.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("app.js 状态码 %d —— 多半是没跑 npm run build", rec.Code)
	}
	if rec.Body.Len() < 1000 {
		t.Fatalf("app.js 只有 %d 字节，像是空产物", rec.Body.Len())
	}
	if !strings.Contains(rec.Body.String(), "/api/board") {
		t.Fatalf("app.js 里没有 /api/board —— 产物不对")
	}
	if !strings.Contains(rec.Body.String(), "/api/search") {
		t.Fatalf("app.js 里没有 /api/search —— 检索页没进产物（改了 TS 要 npm run build）")
	}
}

// 前端默认走内嵌；`--ui-dir` 那份能**盖过**内嵌 —— 但默认没被拆掉（默认那条见 TestServerServesUI）。
//
// 覆盖是**替换**不是叠加：外面目录里没有的文件，不该从内嵌那份里漏出来。
func TestServerUIOverride(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// 记号用「外部前端」：断言赢的是外面这份，而不是「内容恰好一样」。
	write("index.html", "<!doctype html><html><body>外部前端</body></html>")
	write("app.js", "// 外部前端\n"+strings.Repeat("// 占位\n", 400)+"// /api/board\n")

	h := (&Server{
		Vault: fixturePath(t, "domains"),
		Now:   func() time.Time { return fixedNow },
		UIFS:  os.DirFS(dir),
	}).Handler()

	if rec := get(t, h, "/"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "外部前端") {
		t.Fatalf("首页没走外部那份（状态码 %d）：%s", rec.Code, rec.Body.String())
	}
	if rec := get(t, h, "/app.js"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "外部前端") {
		t.Fatalf("/app.js 没走外部那份（状态码 %d）", rec.Code)
	}
	// 内嵌里有、外面没有：不该漏出来。
	if rec := get(t, h, "/favicon.svg"); rec.Code == http.StatusOK {
		t.Fatalf("/favicon.svg 居然 200 —— 覆盖是替换，不该叠加内嵌那份")
	}
	// 换前端不动 HTTP 面的契约：投影端点照旧。
	if rec := get(t, h, "/api/board"); rec.Code != http.StatusOK {
		t.Fatalf("/api/board 状态码 %d —— 换前端不该动端点", rec.Code)
	}
}

// 内嵌文件系统不外泄：包外的路径取不到。
func TestServerDoesNotServeOutsideAssets(t *testing.T) {
	for _, p := range []string{"/../go.mod", "/../../README.md", "/ui/../server.go"} {
		if rec := get(t, newHandler(t, "domains"), p); rec.Code == http.StatusOK {
			t.Fatalf("%s 居然 200：内嵌资源外泄", p)
		}
	}
}

// 到不了报告的错误（文件缺失等）也要把本机路径摘掉再给前端。
func TestIssuesHideVaultPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "空vault")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Vault: dir, Now: func() time.Time { return fixedNow }}).Handler()
	rec := get(t, h, "/api/issues")
	// 按**解码后**的字段断言：消费方看到的是解码结果，不是 JSON 里的转义写法。
	var i Issues
	if err := json.Unmarshal(rec.Body.Bytes(), &i); err != nil {
		t.Fatalf("不是合法 JSON：%v", err)
	}
	if !strings.Contains(i.Error, "<vault>") {
		t.Fatalf("没有把 vault 路径替换成 <vault>：%q", i.Error)
	}
	if strings.Contains(i.Error, dir) {
		t.Fatalf("泄漏了本机绝对路径：%q", i.Error)
	}
	if strings.Contains(rec.Body.String(), dir) {
		t.Fatalf("原始响应里出现了本机绝对路径：%s", rec.Body.String())
	}
}

// 真起一个监听（不是 httptest 的内存往返）：验证真实网络路径，并**优雅关闭** ——
// 这条挡的是「httptest 过得了、真起服务却不对」那类问题。
func TestServerOnRealListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: (&Server{Vault: searchFixture(t)}).Handler()}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	base := "http://" + ln.Addr().String()
	for _, c := range []struct{ path, want string }{
		{"/", "<!doctype html"},
		{"/app.js", "/api/board"},
		{"/api/board", `"schema": "` + Schema + `"`},
		{"/api/search?q=%E7%BB%93%E7%AE%97", `"schema": "` + SearchSchema + `"`},
	} {
		res, err := http.Get(base + c.path)
		if err != nil {
			t.Fatalf("GET %s: %v", c.path, err)
		}
		b, err := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if err != nil {
			t.Fatalf("读 %s 响应: %v", c.path, err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s 状态码 %d", c.path, res.StatusCode)
		}
		if !strings.Contains(string(b), c.want) {
			t.Fatalf("GET %s 的响应里没有 %q（前 200 字节：%.200s）", c.path, c.want, string(b))
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("优雅关闭失败：%v", err)
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("Serve 返回了非预期错误：%v", err)
	}
}
