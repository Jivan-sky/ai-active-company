package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeEnvelope(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "e.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const envelopeGood = `{"id":"e1","ts":"2026-10-08T02:41:00+08:00","who":"alice",
  "on_behalf_of":"member:alice","scope":{"domain":"trade","project":"trade-q3"},"kind":"ask"}`

const envelopeBadWho = `{"id":"e2","ts":"2026-10-08T02:41:00+08:00","who":"nobody",
  "on_behalf_of":"member:alice","kind":"ask"}`

// 判据走完一圈：六问都打在输出里，且干净信封不报警。
func TestEnvelopeCLIAnswersSix(t *testing.T) {
	vault := copyFixture(t, "domains")
	file := writeEnvelope(t, envelopeGood)
	out, code := captureStdout(t, func() int { return cmdEnvelopeCheck([]string{vault, file}) })
	if code != 0 {
		t.Fatalf("干净信封应当 exit 0，实际 %d\n%s", code, out)
	}
	for _, want := range []string{"谁", "代谁", "哪块业务", "要什么", "证据在哪", "要不要人拍", "alice", "trade / trade-q3", "0 项发现"} {
		if !strings.Contains(out, want) {
			t.Errorf("输出里没有 %q：\n%s", want, out)
		}
	}
}

// 接入面同一条绑定边界：非回环不给 --insecure-bind 就拒绝启动。
// data 指到临时目录，避免往 testdata 里落东西。
func TestEnvelopeServeRefusesNonLoopback(t *testing.T) {
	code := quiet(t, func() int {
		return cmdEnvelopeServe([]string{
			filepath.Join("testdata", "orgs", "domains"),
			"--data", t.TempDir(),
			"--addr", "0.0.0.0:0",
		})
	})
	if code != 1 {
		t.Fatalf("退出码 %d，期望 1", code)
	}
}

// 缺 who = 读不懂 = 拒收（退出码 1），不是「一条 warn 然后放行」。
func TestEnvelopeCLIRejectsIncomplete(t *testing.T) {
	vault := copyFixture(t, "domains")
	file := writeEnvelope(t, `{"id":"e3","ts":"2026-10-08T02:41:00+08:00"}`)
	if code := quiet(t, func() int { return cmdEnvelopeCheck([]string{vault, file}) }); code != 1 {
		t.Fatalf("缺 who 应当 exit 1，实际 %d", code)
	}
}

// 身份对不上：报 warn 但**不拦**（退出码 0）—— 默认档就是「先看见，不先拦」。
func TestEnvelopeCLIWarnsButDoesNotBlock(t *testing.T) {
	vault := copyFixture(t, "domains")
	file := writeEnvelope(t, envelopeBadWho)
	out, code := captureStdout(t, func() int { return cmdEnvelopeCheck([]string{vault, file}) })
	if code != 0 {
		t.Fatalf("非红档不该拦，实际 exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "envelope.who.unknown") {
		t.Errorf("应当报出 envelope.who.unknown：\n%s", out)
	}
}

// --json 是给机器吃的：同一份输入，结构化出口里的 fatal/warn 与人类可读版一致。
func TestEnvelopeCLIJSON(t *testing.T) {
	vault := copyFixture(t, "domains")
	file := writeEnvelope(t, envelopeBadWho)
	out, code := captureStdout(t, func() int { return cmdEnvelopeCheck([]string{vault, file, "--json"}) })
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	for _, want := range []string{`"ok": true`, `"envelope.who.unknown"`, `"q": "谁"`} {
		if !strings.Contains(out, want) {
			t.Errorf("JSON 输出里没有 %s：\n%s", want, out)
		}
	}
}

// ---------- 接入面（serve）的本地测试 ----------

// addPolicyToCompany 往 company.md 的 frontmatter 末尾插一段 policy（同 org 包测试的做法）。
func addPolicyToCompany(t *testing.T, vault string, lines ...string) {
	t.Helper()
	path := filepath.Join(vault, "company", "company.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	i := strings.Index(text, "\n---")
	if i < 0 {
		t.Fatalf("%s 里找不到 frontmatter 结束符", path)
	}
	head := text[:i] + "\npolicy:"
	for _, l := range lines {
		head += "\n  " + l
	}
	if err := os.WriteFile(path, []byte(head+text[i:]), 0o600); err != nil {
		t.Fatal(err)
	}
}

func logFiles(t *testing.T, dataDir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dataDir, "envelope"))
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// 递一封信：回六问六答、落一行日志。工具参数 → 信封 JSON → Parse，与线上同一条路。
func TestIngressAnswersSixAndLogs(t *testing.T) {
	vault := copyFixture(t, "domains")
	data := t.TempDir()
	g := &envelopeIngress{Vault: vault, DataDir: data}
	text, isErr := g.sendEnvelope(map[string]any{
		"who": "alice", "on_behalf_of": "member:alice", "kind": "ask",
		"body": "帮我看下", "scope_domain": "trade", "needs": []any{"human-approval"},
	})
	if isErr {
		t.Fatalf("干净信封不该失败：%s", text)
	}
	for _, want := range []string{"谁：alice", "代谁：member:alice", "哪块业务：trade", "要不要人拍：human-approval", "0 项发现"} {
		if !strings.Contains(text, want) {
			t.Errorf("回话里没有 %q：\n%s", want, text)
		}
	}
	if names := logFiles(t, data); len(names) != 1 || names[0] != "2026-10.alice.jsonl" {
		t.Fatalf("日志分片不对：%v", names)
	}
}

// 默认档是 warn：身份对不上**照收**（落盘 + 回话），只是把发现一并回给递信的人。
func TestIngressWarnsButStillLogs(t *testing.T) {
	vault := copyFixture(t, "domains")
	data := t.TempDir()
	g := &envelopeIngress{Vault: vault, DataDir: data}
	text, isErr := g.sendEnvelope(map[string]any{"who": "nobody", "body": "身份对不上"})
	if isErr {
		t.Fatalf("非红档不该拒收：%s", text)
	}
	if !strings.Contains(text, "envelope.who.unknown") {
		t.Errorf("应当把发现回给递信的人：\n%s", text)
	}
	if names := logFiles(t, data); len(names) != 1 {
		t.Fatalf("照收就该落盘，实际 %v", names)
	}
}

// 开关翻到 fatal：**拒收**（按 fatal 在这库里的定义 = 阻止落盘），日志不写。
func TestIngressFatalRejectsAndDoesNotLog(t *testing.T) {
	vault := copyFixture(t, "domains")
	addPolicyToCompany(t, vault, "envelope.who.unknown: fatal")
	data := t.TempDir()
	g := &envelopeIngress{Vault: vault, DataDir: data}
	text, isErr := g.sendEnvelope(map[string]any{"who": "nobody", "body": "该被拒"})
	if !isErr {
		t.Fatalf("提成 fatal 后应当拒收：%s", text)
	}
	if !strings.Contains(text, "拒收") || !strings.Contains(text, "envelope.who.unknown") {
		t.Errorf("拒收回话要说清为什么：\n%s", text)
	}
	if names := logFiles(t, data); len(names) != 0 {
		t.Fatalf("拒收的信不该落盘，实际 %v", names)
	}
}

// 读不懂的信封（缺 who）在**进绑定之前**就被挡下 —— 也是拒收，也不落盘。
func TestIngressUnreadableEnvelope(t *testing.T) {
	g := &envelopeIngress{Vault: copyFixture(t, "domains"), DataDir: t.TempDir()}
	text, isErr := g.sendEnvelope(map[string]any{"body": "没有 who"})
	if !isErr || !strings.Contains(text, "读不懂") {
		t.Fatalf("缺 who 应当拒收并说清，实际 isErr=%v：%s", isErr, text)
	}
}
