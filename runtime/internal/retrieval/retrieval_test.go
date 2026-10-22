package retrieval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"anc/internal/org"
)

// 这些用例锁的是**业务契约**，不是分词 / 打分的内部细节：
//   - 中文查得到（这一层不要求分词的精确率，只要求「结算」能找到「Q3 结算差异」）；
//   - 看不见的域一篇都不出现，且不占用 limit；
//   - 公司级资料谁都能看；
//   - 路径 → 域、frontmatter → 域两条推断口径；
//   - Get 只认索引里、且可见的路径（目录穿越没有入口）。

func doc(path, domain, title, body string) Doc {
	return Doc{Path: path, Domain: domain, Title: title, Body: body}
}

func TestTokenizeChineseAndASCII(t *testing.T) {
	got := tokenize("ANC 网关 Q3结算")
	want := map[string]bool{"anc": true, "q3": true, "结算": true, "网关": true}
	for w := range want {
		if !contains(got, w) {
			t.Fatalf("分词缺 %q：%v", w, got)
		}
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func mustSearch(t *testing.T, ix *Index, q string, sc Scope, limit int) []Hit {
	t.Helper()
	hits, err := ix.Search(q, sc, limit)
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

func mustGet(t *testing.T, ix *Index, path string, sc Scope) (Doc, bool) {
	t.Helper()
	d, ok, err := ix.Get(path, sc)
	if err != nil {
		t.Fatal(err)
	}
	return d, ok
}

func TestSearchFindsChineseBigram(t *testing.T) {
	ix := New([]Doc{
		doc("projects/trade-q3.md", "trade", "Q3 结算差异", "合同执行与结算口径。"),
		doc("shipments/eta.md", "logistics", "到货跟踪", "船公司的 ETA 与 B/L。"),
	})
	hits := mustSearch(t, ix, "结算", Scope{All: true}, 8)
	if len(hits) == 0 {
		t.Fatal("中文二元组没命中「结算」")
	}
	if hits[0].Path != "projects/trade-q3.md" {
		t.Fatalf("命中了别的文档：%+v", hits)
	}
	if !strings.Contains(hits[0].Snippet, "结算") {
		t.Fatalf("片段没有命中上下文：%q", hits[0].Snippet)
	}
}

func TestSearchFiltersBeforeLimit(t *testing.T) {
	// 两篇都能命中，但 logistics 不在这次范围；limit=1 时也不能让不可见的挤掉可见的。
	ix := New([]Doc{
		doc("shipments/a.md", "logistics", "结算", "结算 结算 结算"),
		doc("projects/b.md", "trade", "结算", "结算"),
	})
	hits := mustSearch(t, ix, "结算", Scope{Domains: []string{"trade"}}, 1)
	if len(hits) != 1 || hits[0].Path != "projects/b.md" {
		t.Fatalf("先过滤再截断这条没成立：%+v", hits)
	}
}

// 分数也不能被看不见的文档影响：IDF / 平均长度都按**可见语料**算。
// 否则「分数偏了一点」就是一条侧信道（能反推出库里还有几篇同词文档）。
func TestScoresDoNotDependOnInvisibleDocs(t *testing.T) {
	visibleOnly := New([]Doc{doc("a.md", "trade", "结算", "结算")})
	withHidden := New([]Doc{
		doc("a.md", "trade", "结算", "结算"),
		doc("z.md", "logistics", "结算", "结算 结算 结算 结算 结算"),
	})
	sc := Scope{Domains: []string{"trade"}}
	a := mustSearch(t, visibleOnly, "结算", sc, 8)
	b := mustSearch(t, withHidden, "结算", sc, 8)
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("可见命中数该都是 1：%d vs %d", len(a), len(b))
	}
	if a[0].Score != b[0].Score {
		t.Fatalf("看不见的文档改变了分数：%f vs %f", a[0].Score, b[0].Score)
	}
}

func TestSearchCompanyWideDocsAlwaysVisible(t *testing.T) {
	ix := New([]Doc{doc("10-knowledge/company-rule.md", "", "公司级制度", "报销走飞书审批。")})
	hits := mustSearch(t, ix, "报销", Scope{Domains: []string{"logistics"}}, 8)
	if len(hits) != 1 {
		t.Fatalf("公司级资料该对所有人可见：%+v", hits)
	}
}

func TestSearchReturnsNothingForUnknownTerm(t *testing.T) {
	ix := New([]Doc{doc("a.md", "", "合同", "合同执行")})
	if hits := mustSearch(t, ix, "量子纠缠", Scope{All: true}, 8); len(hits) != 0 {
		t.Fatalf("没有依据的词不该编出命中：%+v", hits)
	}
}

func TestSearchLimitIsClamped(t *testing.T) {
	var docs []Doc
	for i := 0; i < 60; i++ {
		docs = append(docs, doc(strings.Repeat("a", 3)+"/"+string(rune('a'+i%26))+string(rune('a'+i/26))+".md", "", "结算", "结算"))
	}
	ix := New(docs)
	if got := len(mustSearch(t, ix, "结算", Scope{All: true}, 999)); got != MaxLimit {
		t.Fatalf("limit 上限该夹到 %d，实际 %d", MaxLimit, got)
	}
}

func TestSearchOrderIsDeterministicOnTies(t *testing.T) {
	ix := New([]Doc{
		doc("b.md", "", "结算", "结算"),
		doc("a.md", "", "结算", "结算"),
	})
	hits := mustSearch(t, ix, "结算", Scope{All: true}, 8)
	if len(hits) != 2 || hits[0].Path != "a.md" || hits[1].Path != "b.md" {
		t.Fatalf("同分该按路径排：%+v", hits)
	}
}

func TestGetRefusesInvisibleAndTraversal(t *testing.T) {
	ix := New([]Doc{doc("projects/a.md", "trade", "A", "正文")})
	if _, ok := mustGet(t, ix, "projects/a.md", Scope{All: true}); !ok {
		t.Fatal("可见文档该取得到")
	}
	if _, ok := mustGet(t, ix, "projects/a.md", Scope{Domains: []string{"logistics"}}); ok {
		t.Fatal("不可见的文档不该取得到")
	}
	if _, ok := mustGet(t, ix, "../secrets.md", Scope{All: true}); ok {
		t.Fatal("索引外的路径（目录穿越）不该取得到")
	}
}

func TestLoadMapsDomainByPathAndFrontmatter(t *testing.T) {
	vault := t.TempDir()
	write(t, vault, "projects/a.md", "---\ntitle: 结算\n---\n结算口径。")
	write(t, vault, "projects/b.md", "---\ndomain: logistics\ntitle: 到货\n---\nETA。")
	write(t, vault, "projects/c.md", "---\ndomain:\ntitle: 公司制度\n---\n报销。")
	write(t, vault, "projects/cards/d.md", "---\ntitle: 卡片\n---\n合同。")

	o := &org.Org{Domains: []org.Domain{
		{Slug: "trade", Data: "projects"},
		{Slug: "logistics", Data: "shipments"},
	}}
	ix, warns, err := Load(vault, o, LoadOptions{Roots: []string{"projects"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Fatalf("不该有警告：%v", warns)
	}
	want := map[string]string{
		"projects/a.md":       "trade",
		"projects/b.md":       "logistics",
		"projects/c.md":       "",
		"projects/cards/d.md": "trade",
	}
	for path, domain := range want {
		d, ok := mustGet(t, ix, path, Scope{All: true})
		if !ok {
			t.Fatalf("没索引到 %s", path)
		}
		if d.Domain != domain {
			t.Fatalf("%s 的域该是 %q，实际 %q", path, domain, d.Domain)
		}
	}
}

func TestLoadWarnsButDoesNotFailOnBadEntries(t *testing.T) {
	vault := t.TempDir()
	write(t, vault, "10-knowledge/ok.md", "---\ntitle: 制度\n---\n正文。")
	write(t, vault, "10-knowledge/large.md", strings.Repeat("x", 2048))
	write(t, vault, "10-knowledge/image.png", "not text")

	ix, warns, err := Load(vault, nil, LoadOptions{Roots: []string{"10-knowledge", "missing"}, MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if ix.Len() != 1 {
		t.Fatalf("只该索引到 ok.md，实际 %d 篇", ix.Len())
	}
	if len(warns) != 2 {
		t.Fatalf("该有「目录不存在」与「文件过大」两条警告，实际 %v", warns)
	}
}

func TestDefaultRootsSkipsInboxAndDedupes(t *testing.T) {
	vault := t.TempDir()
	for _, d := range []string{"10-knowledge", "knowledge", "shipments", "00-inbox"} {
		if err := os.MkdirAll(filepath.Join(vault, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	o := &org.Org{Domains: []org.Domain{
		{Slug: "trade", Data: "10-knowledge"},
		{Slug: "logistics", Data: "shipments"},
		{Slug: "ops", Data: "20-ops"},
	}}
	got := DefaultRoots(vault, o)
	want := []string{"10-knowledge", "knowledge", "shipments"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("出厂索引面不对：%v", got)
	}
}

func TestSplitFrontmatterDoesNotEatBody(t *testing.T) {
	fm, body := splitFrontmatter("---\ntitle: 甲\ndomain: trade\n---\n# 正文\n内容")
	if fm["title"] != "甲" || fm["domain"] != "trade" {
		t.Fatalf("frontmatter 解析不对：%+v", fm)
	}
	if !strings.Contains(body, "# 正文") {
		t.Fatalf("正文被吃掉了：%q", body)
	}
}

func TestQuotedFrontmatterValuesAreUnquoted(t *testing.T) {
	fm, _ := splitFrontmatter("---\ntitle: \"甲\"\ndomain: 'trade'\n---\n正文")
	if fm["title"] != "甲" || fm["domain"] != "trade" {
		t.Fatalf("带引号的值没去掉引号：%+v", fm)
	}
}

func TestOpenRejectsUnknownBackendInsteadOfSilentFallback(t *testing.T) {
	// 静默回落会让「装了自托管检索层却还在用本地索引」变成假绿 —— 宁可不启动。
	if _, _, err := Open(t.TempDir(), nil, Config{Backend: "weknora"}); err == nil {
		t.Fatal("不认识的后端应当明确报错")
	}
	s, _, err := Open(t.TempDir(), nil, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Name() != "bm25" {
		t.Fatalf("出厂后端该是 bm25，实际 %q", s.Name())
	}
}

func write(t *testing.T, vault, rel, content string) {
	t.Helper()
	p := filepath.Join(vault, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
