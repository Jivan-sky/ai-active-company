package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"anc/internal/distill"
)

func distillCLIFixture(t *testing.T) string {
	t.Helper()
	vault := t.TempDir()
	if err := os.CopyFS(vault, os.DirFS(fixturePath(t, "domains"))); err != nil {
		t.Fatal(err)
	}
	lines := `{"id":"c1","case":"case-9","at":"2026-10-18T10:00:00+08:00","status":"done","by":"cs-owner","title":"已闭环的事","detail":"做完了","refs":["a.md"]}
{"id":"c2","case":"case-8","at":"2026-10-18T11:00:00+08:00","status":"blocked","by":"cs-owner","title":"还卡着的事"}
`
	if err := os.MkdirAll(filepath.Join(vault, "timeline"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, "timeline", "2026-10.jsonl"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(vault, "skills", "orphan")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# orphan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return vault
}

// --json 与 /api/distill 是同一份契约；人读输出要把「缺正文 / 孤儿」写清楚。
func TestDistillCLIJSON(t *testing.T) {
	vault := distillCLIFixture(t)
	out, code := captureStdout(t, func() int { return cmdDistillLs([]string{vault, "--json"}) })
	if code != 0 {
		t.Fatalf("退出码 %d：%s", code, out)
	}
	var r distill.Report
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("不是合法 JSON：%v\n%s", err, out)
	}
	if r.Schema != distill.Schema || r.CandidateTotal != 1 || r.Green != 1 {
		t.Fatalf("报告不对：schema=%s green=%d candidates=%d", r.Schema, r.Green, r.CandidateTotal)
	}
	if r.Candidates[0].Case != "case-9" || r.Candidates[0].Signals[0] != "1 条推进" {
		t.Fatalf("候选不对：%+v", r.Candidates[0])
	}
}

func TestDistillCLIHumanMentionsGaps(t *testing.T) {
	vault := distillCLIFixture(t)
	out, code := captureStdout(t, func() int { return cmdDistillLs([]string{vault}) })
	if code != 0 {
		t.Fatalf("退出码 %d：%s", code, out)
	}
	for _, want := range []string{"可沉淀候选", "case-9", "技能正文没落：skills/im-send/SKILL.md", "孤儿技能 1 份", "写入点是 skills/"} {
		if !strings.Contains(out, want) {
			t.Fatalf("输出缺少 %q：\n%s", want, out)
		}
	}
}

// draft：只写 skills/_drafts/，不碰正式技能；重复写要显式 --force；
// 没闭环的 case 拒绝出草稿；名字不合法直接用法错。
func TestDistillDraftCLI(t *testing.T) {
	vault := distillCLIFixture(t)
	out, code := captureStdout(t, func() int { return cmdDistillDraft([]string{vault, "case-9"}) })
	if code != 0 {
		t.Fatalf("退出码 %d：%s", code, out)
	}
	draftPath := filepath.Join(vault, "skills", "_drafts", "case-9.md")
	raw, err := os.ReadFile(draftPath)
	if err != nil {
		t.Fatalf("草稿没写出来：%v", err)
	}
	md := string(raw)
	for _, want := range []string{"name: case-9", "status: draft", "case: case-9", "做完了", "不是可执行技能"} {
		if !strings.Contains(md, want) {
			t.Fatalf("草稿缺少 %q：\n%s", want, md)
		}
	}
	if _, err := os.Stat(filepath.Join(vault, "skills", "case-9", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("draft 不许碰正式技能 skills/case-9/SKILL.md")
	}
	if _, code := captureStdout(t, func() int { return cmdDistillDraft([]string{vault, "case-9"}) }); code != 1 {
		t.Fatalf("重复出草稿应退出 1，实际 %d", code)
	}
	if _, code := captureStdout(t, func() int { return cmdDistillDraft([]string{vault, "case-9", "--force"}) }); code != 0 {
		t.Fatalf("--force 应覆盖，退出码 %d", code)
	}
	if _, code := captureStdout(t, func() int { return cmdDistillDraft([]string{vault, "case-8"}) }); code != 1 {
		t.Fatalf("没闭环的 case 应退出 1，实际 %d", code)
	}
	if _, code := captureStdout(t, func() int { return cmdDistillDraft([]string{vault, "case-9", "--name", "Bad Name"}) }); code != 2 {
		t.Fatalf("非法名字应退出 2，实际 %d", code)
	}

	// _drafts 不是技能：它不该出现在候选表的孤儿清单里。
	jsonOut, code := captureStdout(t, func() int { return cmdDistillLs([]string{vault, "--json"}) })
	if code != 0 {
		t.Fatalf("ls 退出码 %d", code)
	}
	var r distill.Report
	if err := json.Unmarshal([]byte(jsonOut), &r); err != nil {
		t.Fatal(err)
	}
	for _, o := range r.Orphans {
		if o == "_drafts" {
			t.Fatal("_drafts 不该被当成孤立技能")
		}
	}
}

// check：生成的草稿四段没写，先拦；补完四段就放行（没有岗位声明只提醒）；
// 正式落点被占用时再拦一次。全程只读。
func TestDistillCheckCLI(t *testing.T) {
	vault := distillCLIFixture(t)
	if _, code := captureStdout(t, func() int { return cmdDistillDraft([]string{vault, "case-9"}) }); code != 0 {
		t.Fatalf("草稿生成失败，退出码 %d", code)
	}
	out, code := captureStdout(t, func() int { return cmdDistillCheck([]string{vault, "case-9"}) })
	if code != 1 {
		t.Fatalf("四段没写应退出 1，实际 %d：\n%s", code, out)
	}
	for _, want := range []string{"「何时用」缺内容", "不能发布"} {
		if !strings.Contains(out, want) {
			t.Fatalf("输出缺少 %q：\n%s", want, out)
		}
	}

	draftPath := filepath.Join(vault, "skills", "_drafts", "case-9.md")
	raw, err := os.ReadFile(draftPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw) + "\n## 何时用\n月底对账时。\n\n## 输入\n账单 CSV。\n\n## 步骤\n1. 对齐单号。\n\n## 验证\n差异为零。\n"
	if err := os.WriteFile(draftPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code = captureStdout(t, func() int { return cmdDistillCheck([]string{vault, "case-9"}) })
	if code != 0 {
		t.Fatalf("补完四段应退出 0，实际 %d：\n%s", code, out)
	}
	for _, want := range []string{"可以发布", "没有任何岗位声明"} {
		if !strings.Contains(out, want) {
			t.Fatalf("输出缺少 %q：\n%s", want, out)
		}
	}

	official := filepath.Join(vault, "skills", "case-9")
	if err := os.MkdirAll(official, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(official, "SKILL.md"), []byte("# 正式\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code = captureStdout(t, func() int { return cmdDistillCheck([]string{vault, "case-9"}) })
	if code != 1 || !strings.Contains(out, "正式落点已被占用") {
		t.Fatalf("正式落点被占用应拦：code=%d\n%s", code, out)
	}
}
