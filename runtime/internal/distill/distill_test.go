package distill

import (
	"strings"
	"testing"

	"anc/internal/timeline"
)

func row(id, caseID, at, status, title, domain, detail string, refs ...string) timeline.Entry {
	return timeline.Entry{
		ID: id, Case: caseID, At: at, Status: status, By: "cs-owner",
		Title: title, Domain: domain, Detail: detail, Refs: refs,
	}
}

func firstLineOf(list []string, prefix string) string {
	for _, s := range list {
		if strings.HasPrefix(s, prefix) {
			return s
		}
	}
	return ""
}

// 候选只认「最后一条是绿档」：同一 case 先 done 后 running，就不算闭环。
// 排序、信号、同名统计都按现算事实走。
func TestCandidatesOnlyClosedCases(t *testing.T) {
	in := Input{
		TimelineWired: true, SkillsWired: true,
		Entries: []timeline.Entry{
			row("a1", "case-a", "2026-10-18T10:00:00+08:00", "running", "对账", "trade", "第一步"),
			row("a2", "case-a", "2026-10-18T11:00:00+08:00", "done", "对账", "trade", "第二步完成", "refs/a.md"),
			row("b1", "case-b", "2026-10-18T12:00:00+08:00", "blocked", "等 ETA", "trade", "卡在外部"),
			row("c1", "case-c", "2026-10-18T13:00:00+08:00", "done", "对账", "ops", "", "refs/b.md", "refs/c.md"),
			row("d1", "case-d", "2026-10-18T14:00:00+08:00", "done", "单独的事", "", ""),
			row("e1", "case-e", "2026-10-18T09:00:00+08:00", "done", "回滚过的判断", "trade", "先成了"),
			row("e2", "case-e", "2026-10-18T15:00:00+08:00", "running", "回滚过的判断", "trade", "又开了"),
		},
	}
	r := Build(in)
	if r.Cases != 5 || r.Green != 3 || r.CandidateTotal != 3 {
		t.Fatalf("cases=%d green=%d candidate_total=%d，期望 5/3/3", r.Cases, r.Green, r.CandidateTotal)
	}
	if len(r.Candidates) != 3 {
		t.Fatalf("候选 %d 条，期望 3", len(r.Candidates))
	}
	if got := []string{r.Candidates[0].Case, r.Candidates[1].Case, r.Candidates[2].Case}; strings.Join(got, ",") != "case-d,case-c,case-a" {
		t.Fatalf("候选顺序 %v，期望按最后活动倒序 case-d,case-c,case-a", got)
	}
	for _, c := range r.Candidates {
		if c.Case == "case-b" || c.Case == "case-e" {
			t.Fatalf("%s 不该进候选（最后一条不是绿档）", c.Case)
		}
	}
	a := r.Candidates[2]
	if a.Entries != 2 || a.DetailChars != 8 || a.Refs != 1 || a.SameTitle != 2 {
		t.Fatalf("case-a 事实不对：%+v", a)
	}
	if strings.Join(a.Signals, " | ") != "2 条推进 | detail 8 字 | refs 1 个 | 同名 2 次" {
		t.Fatalf("case-a signals=%v", a.Signals)
	}
}

// 技能覆盖：声明了但没正文 = missing；有正文但没人声明 = 孤儿。
func TestSkillCoverageAndOrphans(t *testing.T) {
	r := Build(Input{
		TimelineWired: true, SkillsWired: true,
		Roles: []RoleSkills{
			{Role: "manager", Skills: []string{"now", "im-send"}},
			{Role: "ops", Skills: []string{"now"}},
		},
		Bodies: []string{"now", "orphan"},
	})
	if len(r.Skills) != 2 {
		t.Fatalf("技能 %d 份，期望 2：%+v", len(r.Skills), r.Skills)
	}
	if r.Skills[0].Name != "im-send" || !r.Skills[0].Missing || strings.Join(r.Skills[0].Roles, ",") != "manager" {
		t.Fatalf("im-send 应缺正文且只由 manager 声明：%+v", r.Skills[0])
	}
	if r.Skills[1].Name != "now" || r.Skills[1].Missing || strings.Join(r.Skills[1].Roles, ",") != "manager,ops" {
		t.Fatalf("now 应有正文、由 manager+ops 声明：%+v", r.Skills[1])
	}
	if len(r.Orphans) != 1 || r.Orphans[0] != "orphan" {
		t.Fatalf("孤儿技能=%v，期望 [orphan]", r.Orphans)
	}
	if got := firstLineOf(r.Gaps, "技能正文没落：skills/im-send/SKILL.md"); got == "" {
		t.Fatalf("缺口里没说 im-send 正文没落：%v", r.Gaps)
	}
}

// 空 timeline 不是错；没有候选 ≠ 没有资产，这句话必须留在 gaps 里。
func TestEmptyTimelineIsNotAnError(t *testing.T) {
	r := Build(Input{TimelineWired: true, SkillsWired: true})
	if r.Error != "" {
		t.Fatalf("空 timeline 不该是错误：%s", r.Error)
	}
	if r.Candidates == nil || len(r.Candidates) != 0 {
		t.Fatalf("空时候选要是空数组，不是 null：%#v", r.Candidates)
	}
	if got := firstLineOf(r.Gaps, "timeline：还没有任何记录"); got == "" {
		t.Fatalf("空 timeline 的缺口没写清：%v", r.Gaps)
	}
}

// org 读不动时，技能那一段答不了 —— 但要与「没有技能」分开说。
func TestSkillsNotWiredIsReported(t *testing.T) {
	r := Build(Input{TimelineWired: true, SkillsWired: false})
	if r.SkillsWired {
		t.Fatal("SkillsWired 应为 false")
	}
	if got := firstLineOf(r.Gaps, "技能覆盖：org 真相源读不动"); got == "" {
		t.Fatalf("缺口没交代技能那一段：%v", r.Gaps)
	}
}
