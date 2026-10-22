package board

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// searchFixture 造一份有知识正文的 vault：trade / logistics 各一篇，另有一篇公司级资料。
// 跨域收紧成 hidden，这样 bob（logistics）看不见 trade，yet 公司级资料仍然可见。
func searchFixture(t *testing.T) string {
	t.Helper()
	vault := t.TempDir()
	if err := os.CopyFS(vault, os.DirFS(fixturePath(t, "domains"))); err != nil {
		t.Fatal(err)
	}
	hideSearchCrossDomain(t, vault)
	writeSearchDoc(t, vault, "10-knowledge/trade.md", `---
title: 结算规则
domain: trade
---

合同结算要先核对到货与发票。
`)
	writeSearchDoc(t, vault, "10-knowledge/logistics.md", `---
title: 订舱流程
domain: logistics
---

订舱要先确认船期与舱位。
`)
	writeSearchDoc(t, vault, "10-knowledge/company.md", `---
title: 公司级结算口径
---

公司级资料：结算是所有业务共同使用的词。
`)
	return vault
}

func writeSearchDoc(t *testing.T, vault, rel, body string) {
	t.Helper()
	p := filepath.Join(vault, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hideSearchCrossDomain(t *testing.T, vault string) {
	t.Helper()
	p := filepath.Join(vault, "company", "company.md")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	end := strings.Index(s[4:], "\n---")
	if !strings.HasPrefix(s, "---\n") || end < 0 {
		t.Fatalf("company.md 的 frontmatter 不完整：\n%s", s)
	}
	s = s[:4+end] + "\naccess:\n  cross_domain: hidden" + s[4+end:]
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func searchOf(t *testing.T, h http.Handler, path string) (SearchView, string) {
	t.Helper()
	rec := get(t, h, path)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 %d，期望 200：%s", rec.Code, rec.Body.String())
	}
	var v SearchView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("不是合法 JSON：%v\n%s", err, rec.Body.String())
	}
	if v.Schema != SearchSchema {
		t.Fatalf("schema=%q，期望 %s", v.Schema, SearchSchema)
	}
	return v, rec.Body.String()
}

// 先过滤再打分：bob 看不见 trade，命中里不能出现 trade 那篇，计数也不能把它算进去。
// 公司级资料（domain 空）仍然可见 —— 与 Viewer.CanSeeDomain("") 同一口径。
func TestSearchFiltersBeforeScoring(t *testing.T) {
	vault := searchFixture(t)
	h := (&Server{Vault: vault, Now: func() time.Time { return fixedNow }}).Handler()

	bob, body := searchOf(t, h, "/api/search?q=结算&who=bob")
	if !bob.Wired || bob.Backend == "" {
		t.Fatalf("检索没接上：%s", body)
	}
	if bob.Refused != nil {
		t.Fatalf("bob 搜公司级词不该被拒：%+v", bob.Refused)
	}
	if len(bob.Hits) != 1 || bob.Hits[0].Path != "10-knowledge/company.md" {
		t.Fatalf("bob 只该命中公司级资料，实际 %+v", bob.Hits)
	}
	if bob.Count != len(bob.Hits) {
		t.Fatalf("count 与 hits 对不上：%d vs %d", bob.Count, len(bob.Hits))
	}

	all, body := searchOf(t, h, "/api/search?q=结算")
	if len(all.Hits) != 2 {
		t.Fatalf("本机操作者该看见 trade + 公司级两篇，实际 %+v\n%s", all.Hits, body)
	}
}

// 域收窄走同一把尺子：先判声明，再判可见；越域要给「找谁」，不能只说零结果。
func TestSearchDomainNarrowing(t *testing.T) {
	h := (&Server{Vault: searchFixture(t), Now: func() time.Time { return fixedNow }}).Handler()

	got, body := searchOf(t, h, "/api/search?q=结算&who=bob&domain=trade")
	if got.Refused == nil || got.Refused.Ask == "" {
		t.Fatalf("bob 越域搜 trade 该拒并指路：%s", body)
	}
	if len(got.Hits) != 0 {
		t.Fatalf("被拒的回话里不该带命中：%+v", got.Hits)
	}

	unknown, body := searchOf(t, h, "/api/search?q=结算&domain=no-such-domain")
	if unknown.Refused == nil || strings.Contains(unknown.Refused.Reason, "不在你的可见范围") {
		t.Fatalf("没声明的域要说「没声明」，不能说成越权：%s", body)
	}

	trade, body := searchOf(t, h, "/api/search?q=订舱&who=bob&domain=logistics")
	if trade.Refused != nil || len(trade.Hits) == 0 || trade.Hits[0].Path != "10-knowledge/logistics.md" {
		t.Fatalf("logistics 域内查询不对：%+v\n%s", trade, body)
	}
	for _, hit := range trade.Hits {
		if hit.Domain != "logistics" {
			t.Fatalf("收窄到 logistics 后不该出现别域命中：%+v", trade.Hits)
		}
	}
	if trade.ScopeNote == "" {
		t.Fatalf("收窄到一域时要说明公司级资料仍然包含：%+v", trade)
	}
}

func TestSearchInputAndBackendErrorsAreExplicit(t *testing.T) {
	h := (&Server{Vault: searchFixture(t), Now: func() time.Time { return fixedNow }}).Handler()
	for _, path := range []string{
		"/api/search",
		"/api/search?q=结算&limit=abc",
		"/api/search?q=" + strings.Repeat("词", maxSearchQueryRunes+1),
	} {
		v, body := searchOf(t, h, path)
		if v.Error == "" {
			t.Fatalf("%s 该明确报错：%s", path, body)
		}
	}

	t.Setenv("ANC_SEARCH_BACKEND", "weknora")
	v, body := searchOf(t, h, "/api/search?q=结算")
	if v.Error == "" || !strings.Contains(v.Error, "不认识的检索后端") {
		t.Fatalf("未知后端要明确报错，不能静默回落：%+v\n%s", v, body)
	}
}

func TestSearchLeaksNoLocalPaths(t *testing.T) {
	vault := searchFixture(t)
	h := (&Server{Vault: vault, Now: func() time.Time { return fixedNow }}).Handler()
	_, body := searchOf(t, h, "/api/search?q=结算&who=bob")
	if strings.Contains(body, vault) {
		t.Fatalf("回话里出现了本机路径：%s", body)
	}
}

// 只读面：搜索也必须是 GET；POST 不能落到处理函数上。
func TestSearchIsReadOnly(t *testing.T) {
	h := (&Server{Vault: searchFixture(t), Now: func() time.Time { return fixedNow }}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/search?q=结算", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/search 状态码 %d，期望 405", rec.Code)
	}
}
