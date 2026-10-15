package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"anc/internal/ctxpack"
)

// ---------- 议题 #68：bot 侧那个「我被允许做什么」的视图 ----------
//
// 它与看板「授权」页（全量）是**两份产物**，但尺子是同一把（org.GrantHitsSubject）。
// 这组用例盯四件事：
//  1. 只出与我相关的几条；`total` 照样给全表数（不藏事实）；
//  2. 同一个库换个人问，出来的是**另一份** —— 「按人过滤」不是「大家看一样的东西」；
//  3. 认不出的身份：整份拒（同「我们公司没有这个人」那条口径），不是全量、也不是空表；
//  4. 单域问不带这一块（同 Todo / Doing / Gaps 那条规矩）；
//  5. 空表把「空就是不通」说清楚，且 rows 是 [] 不是 null。

// grantDoc 造一条授权文件（与 org 那侧的写法一致：七个字段 + 一段自由正文）。
func grantDoc(from, to, action, object, reason string) string {
	return "---\ngrant:\n" +
		"  from: " + from + "\n" +
		"  to: " + to + "\n" +
		"  action: " + action + "\n" +
		"  object: " + object + "\n" +
		"  reason: " + reason + "\n" +
		"---\n\n正文随便写，ANC 不解析它。\n"
}

func writeIntoFixture(t *testing.T, vault, rel, content string) {
	t.Helper()
	p := filepath.Join(vault, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// grantsFixture：三条权，覆盖两类主体 + 一条「to 是岗位」（那条**不**该命中任何人）。
//
//	alice-to-bob   member:alice -> member:bob     alice 与 bob 都相关
//	manager-to-bot role:manager -> agent:order-bot  只有 order-bot 相关
//	bare-name      bob -> alice                    alice 与 bob 都相关
func grantsFixture(t *testing.T) string {
	t.Helper()
	v := copyFixture(t, "domains")
	writeIntoFixture(t, v, "agents.md",
		"| slug | name | domain | harness | tools | model | role | app_id |\n"+
			"|---|---|---|---|---|---|---|---|\n"+
			"| order-bot | 订单处理 | trade | codex | read,bash | claude-sonnet-5 | manager | cli_demo_order |\n")
	writeIntoFixture(t, v, "grants/alice-to-bob.md",
		grantDoc("member:alice", "member:bob", "read", "domain:trade", "对读数"))
	writeIntoFixture(t, v, "grants/manager-to-bot.md",
		grantDoc("role:manager", "agent:order-bot", "invoke", "orders", "让业务 agent 接单"))
	writeIntoFixture(t, v, "grants/bare-name.md",
		grantDoc("bob", "alice", "write", "shipments", "裸名字也认"))
	return v
}

func packGrants(t *testing.T, vault string, extra ...string) (ctxpack.Out, string, int) {
	t.Helper()
	out, code := ctxPack(t, vault, extra...)
	var got ctxpack.Out
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("不是合法 JSON：%v\n%s", err, out)
	}
	return got, out, code
}

func TestContextPackGrantsOnlyMine(t *testing.T) {
	v := grantsFixture(t)

	got, _, code := packGrants(t, v, "--who", "alice")
	if code != 0 {
		t.Fatalf("退出码 %d", code)
	}
	if got.Grants == nil {
		t.Fatal("授权那一块没给：业务 agent 问「我被允许做什么」应该拿得到")
	}
	if got.Grants.Schema != ctxpack.GrantsSchema {
		t.Fatalf("schema=%q，期望 %s", got.Grants.Schema, ctxpack.GrantsSchema)
	}
	if got.Grants.Total != 3 || got.Grants.Shown != 2 {
		t.Fatalf("total / shown = %d / %d，期望 3 / 2", got.Grants.Total, got.Grants.Shown)
	}
	var paths []string
	for _, r := range got.Grants.Rows {
		paths = append(paths, r.Path)
	}
	if strings.Join(paths, ",") != "grants/alice-to-bob.md,grants/bare-name.md" {
		t.Fatalf("露出来的行不对（顺序也应确定）：%v", paths)
	}
	// 字段原样搬：这一行的 from / to 就是文件里写的那个写法，没有转述过。
	if got.Grants.Rows[0].From != "member:alice" || got.Grants.Rows[0].Reason != "对读数" {
		t.Fatalf("字段没原样搬：%+v", got.Grants.Rows[0])
	}

	// 换个人问：另一份，且那条「to 是岗位」的权**不**命中岗位上的人。
	bot, _, code := packGrants(t, v, "--who", "order-bot")
	if code != 0 {
		t.Fatalf("退出码 %d", code)
	}
	if bot.Grants == nil || bot.Grants.Total != 3 || bot.Grants.Shown != 1 {
		t.Fatalf("业务 agent 那一份不对：%+v", bot.Grants)
	}
	if bot.Grants.Rows[0].Path != "grants/manager-to-bot.md" {
		t.Fatalf("业务 agent 该只看到那一条：%+v", bot.Grants.Rows)
	}
}

// 认不出的身份：整份**拒**（同「我们公司没有这个人」），而不是退回全量、也不是给一份空表。
func TestContextPackGrantsUnknownWhoRefuses(t *testing.T) {
	got, raw, code := packGrants(t, grantsFixture(t), "--who", "ghost")
	if code != 1 {
		t.Fatalf("退出码 %d，期望 1（拒）", code)
	}
	if got.Refused == nil {
		t.Fatalf("认不出该拒：%+v", got)
	}
	if got.Grants != nil {
		t.Fatalf("拒的时候不该带授权那一块：%+v", got.Grants)
	}
	if strings.Contains(raw, "grants/") {
		t.Fatalf("拒话里不该出现任何一条授权：%s", raw)
	}
}

// 单域问：这一块不跟着进来（授权按**主体**算，切不出「这一块业务的那几条」）。
func TestContextPackGrantsSparsedOnSingleDomain(t *testing.T) {
	got, _, code := packGrants(t, grantsFixture(t), "--who", "alice", "--domain", "trade")
	if code != 0 {
		t.Fatalf("退出码 %d", code)
	}
	if got.Grants != nil {
		t.Fatalf("单域问不该带授权那一块：%+v", got.Grants)
	}
}

// 表是空的（存量客户还没写任何 grant）：`total=0` + 一句「空就是不通」，
// 而且 rows 必须是 `[]` 不是 `null` —— 前端与 harness 吃到 null 就炸。
func TestContextPackGrantsEmptyTableSaysWhat(t *testing.T) {
	got, raw, code := packGrants(t, copyFixture(t, "domains"), "--who", "alice")
	if code != 0 {
		t.Fatalf("退出码 %d：%s", code, raw)
	}
	if got.Grants == nil || got.Grants.Total != 0 || got.Grants.Shown != 0 {
		t.Fatalf("空表该如实报：%+v", got.Grants)
	}
	if !strings.Contains(got.Grants.Note, "不通") {
		t.Fatalf("空表要把「没有 grant 就是不通」说清：%q", got.Grants.Note)
	}
	if strings.Contains(raw, `"rows":null`) {
		t.Fatalf("rows 是 null：\n%s", raw)
	}
}
