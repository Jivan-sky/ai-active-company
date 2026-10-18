package board

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"anc/internal/distill"
)

func distillFixture(t *testing.T) string {
	t.Helper()
	vault := t.TempDir()
	if err := os.CopyFS(vault, os.DirFS(fixturePath(t, "domains"))); err != nil {
		t.Fatal(err)
	}
	lines := `{"id":"t1","case":"case-1","at":"2026-10-18T10:00:00+08:00","status":"running","by":"cs-owner","title":"在跑的事"}
{"id":"t2","case":"case-1","at":"2026-10-18T11:00:00+08:00","status":"done","by":"cs-owner","title":"在跑的事","detail":"做完了"}
{"id":"t3","case":"case-2","at":"2026-10-18T12:00:00+08:00","status":"done","by":"cs-owner","title":"另一件闭环"}
`
	if err := os.MkdirAll(filepath.Join(vault, "timeline"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, "timeline", "2026-10.jsonl"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"now", "orphan"} {
		dir := filepath.Join(vault, "skills", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return vault
}

func distillOf(t *testing.T, h http.Handler, path string) distill.Report {
	t.Helper()
	rec := get(t, h, path)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 %d，期望 200：%s", rec.Code, rec.Body.String())
	}
	var v distill.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("不是合法 JSON：%v", err)
	}
	if v.Schema != distill.Schema {
		t.Fatalf("schema=%q，期望 %s", v.Schema, distill.Schema)
	}
	return v
}

// 看板这一口与 CLI 同一份报告：绿档才算候选，技能缺失与孤儿技能都要看得见。
func TestDistillReportsCandidatesAndSkillGaps(t *testing.T) {
	v := distillOf(t, (&Server{Vault: distillFixture(t)}).Handler(), "/api/distill")
	if !v.Wired || !v.SkillsWired {
		t.Fatalf("两份真相源都读得动，却报 wired=%v skills_wired=%v（%s / %s）", v.Wired, v.SkillsWired, v.Error, v.SkillsError)
	}
	if v.Green != 2 || v.CandidateTotal != 2 || len(v.Candidates) != 2 {
		t.Fatalf("闭环 %d / 候选 %d，期望 2/2：%+v", v.Green, v.CandidateTotal, v.Candidates)
	}
	for _, c := range v.Candidates {
		if c.Case != "case-1" && c.Case != "case-2" {
			t.Fatalf("混进了不该有的候选：%+v", c)
		}
	}
	for _, s := range v.Skills {
		if s.Name == "now" && s.Missing {
			t.Fatalf("now 的正文在，不该报 missing：%+v", s)
		}
		if s.Name == "im-send" && !s.Missing {
			t.Fatalf("im-send 的正文不在，必须报 missing：%+v", s)
		}
	}
	if len(v.Orphans) != 1 || v.Orphans[0] != "orphan" {
		t.Fatalf("孤儿技能=%v，期望 [orphan]", v.Orphans)
	}
}

// limit 只截回显，不截 candidate_total；截了就明说 truncated。
func TestDistillLimitTruncatesHonestly(t *testing.T) {
	v := distillOf(t, (&Server{Vault: distillFixture(t)}).Handler(), "/api/distill?limit=1")
	if len(v.Candidates) != 1 || !v.Truncated || v.CandidateTotal != 2 {
		t.Fatalf("limit=1 应回 1 条 + truncated + total=2：len=%d truncated=%v total=%d",
			len(v.Candidates), v.Truncated, v.CandidateTotal)
	}
}

func TestDistillBadLimit(t *testing.T) {
	v := distillOf(t, (&Server{Vault: distillFixture(t)}).Handler(), "/api/distill?limit=abc")
	if v.Error != "limit 要是个正整数" {
		t.Fatalf("错误=%q", v.Error)
	}
}
