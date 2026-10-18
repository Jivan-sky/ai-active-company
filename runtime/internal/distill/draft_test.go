package distill

import (
	"errors"
	"strings"
	"testing"

	"anc/internal/timeline"
)

func TestBuildDraftOnlyClosedCases(t *testing.T) {
	entries := []timeline.Entry{
		row("d1", "case-open", "2026-10-18T10:00:00+08:00", "done", "开了又关", "trade", "先做完"),
		row("d2", "case-open", "2026-10-18T11:00:00+08:00", "running", "开了又关", "trade", "又开了"),
	}
	if _, err := BuildDraft(entries, "case-open"); !errors.Is(err, ErrCaseNotClosed) {
		t.Fatalf("最后一条是 running，应回 ErrCaseNotClosed，实际 %v", err)
	}
	if _, err := BuildDraft(entries, "case-missing"); !errors.Is(err, ErrCaseNotFound) {
		t.Fatalf("不存在的 case，应回 ErrCaseNotFound，实际 %v", err)
	}
	ok := append(entries, row("d3", "case-done", "2026-10-18T12:00:00+08:00", "done", "闭环", "", "做完了"))
	d, err := BuildDraft(ok, "case-done")
	if err != nil {
		t.Fatalf("绿档 case 应能出草稿：%v", err)
	}
	if d.Case.Case != "case-done" || len(d.Rows) != 1 {
		t.Fatalf("草稿不对：%+v", d)
	}
}

// 草稿只装事实：frontmatter、归属、证据、原始记录、引用都来自 timeline；
// 它必须自己说清「不是可执行技能」，且不同时提供正式技能的落点。
func TestDraftMarkdownCarriesFactsOnly(t *testing.T) {
	entries := []timeline.Entry{
		row("m1", "case-md", "2026-10-18T10:00:00+08:00", "running", "对账", "trade", "第一步"),
		row("m2", "case-md", "2026-10-18T11:00:00+08:00", "done", "对账", "trade", "第二步完成", "refs/a.md", "refs/b.md"),
	}
	d, err := BuildDraft(entries, "case-md")
	if err != nil {
		t.Fatal(err)
	}
	md := d.Markdown("reconcile", "2026-10-18T12:00:00+08:00")
	for _, want := range []string{
		"name: reconcile", "status: draft", "case: case-md", "generated_at: 2026-10-18T12:00:00+08:00",
		"# 对账", "业务域：trade", "推进记录：2 条",
		"2 条推进", "detail 8 字", "refs 2 个",
		"### 2026-10-18T10:00:00+08:00 · running", "第一步", "第二步完成",
		"- refs/a.md", "- refs/b.md",
		"不是可执行技能", "skills/reconcile/SKILL.md", "anc distill ls",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("草稿缺少 %q：\n%s", want, md)
		}
	}
	if strings.Contains(md, "TODO") || strings.Contains(md, "以后补") {
		t.Fatalf("草稿里不许出现占位符：\n%s", md)
	}
}

func TestSkillNameAndSlug(t *testing.T) {
	cases := map[string]string{
		"case-anc-20261018-a": "case-anc-20261018-a",
		"Case.ANC 2026/10":    "case-anc-2026-10",
		"":                    "",
		"中文":                  "",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q)=%q，期望 %q", in, got, want)
		}
	}
	for _, ok := range []string{"now", "im-send", "a", "post-sale-2"} {
		if !ValidSkillName(ok) {
			t.Errorf("%q 应合法", ok)
		}
	}
	for _, bad := range []string{"", "Now", "1abc", "_drafts", "中文", "a b", "-x", strings.Repeat("a", 65)} {
		if ValidSkillName(bad) {
			t.Errorf("%q 不该合法", bad)
		}
	}
}
