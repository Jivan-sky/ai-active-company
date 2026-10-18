package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"anc/internal/hot"
)

type fakeBeatStore struct {
	got   hot.AgentHeartbeat
	err   error
	stale time.Duration
	has   bool
}

func (f *fakeBeatStore) BeatAgent(h hot.AgentHeartbeat) (hot.AgentHeartbeat, error) {
	f.got = h
	if h.AsOf == "" {
		h.AsOf = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	}
	return h, f.err
}
func (f *fakeBeatStore) ListAgents() ([]hot.AgentHeartbeat, error) { return nil, f.err }
func (f *fakeBeatStore) AgentStale() time.Duration                 { return f.stale }
func (f *fakeBeatStore) GetAgent(string) (hot.AgentHeartbeat, bool, error) {
	return f.got, f.has, f.err
}

func beatHarness(t *testing.T) (http.Handler, *envelopeIngress, *fakeBeatStore) {
	t.Helper()
	vault := copyFixture(t, "domains")
	writeIntoFixture(t, vault, "agents.md",
		"| slug | name | domain | role | tools | harness | model | description | app_id |\n"+
			"|---|---|---|---|---|---|---|---|---|\n"+
			"| tradebot | 贸易助手 | trade | manager | anc_read_context | codex | | 处理贸易单据 | |\n")
	g := &envelopeIngress{
		Vault: vault, DataDir: t.TempDir(),
		Now: ticking(time.Date(2026, 10, 18, 3, 0, 0, 0, time.UTC)),
	}
	fs := &fakeBeatStore{stale: 2 * time.Minute}
	g.Agents = fs
	s := g.srv()
	s.Log = nil
	return s.Handler(), g, fs
}

func callBeat(t *testing.T, h http.Handler, args map[string]any) (string, bool) {
	t.Helper()
	out := mcpRoundTrip(t, h, "tools/call", map[string]any{"name": "anc_agent_heartbeat", "arguments": args})
	if len(out.Result.Content) == 0 {
		t.Fatalf("回话里没有 content：%+v", out.Result)
	}
	return out.Result.Content[0].Text, out.Result.IsError
}

func TestAgentHeartbeatIsListed(t *testing.T) {
	h, _, _ := beatHarness(t)
	out := mcpRoundTrip(t, h, "tools/list", map[string]any{})
	for _, tool := range out.Result.Tools {
		if tool.Name == "anc_agent_heartbeat" {
			return
		}
	}
	t.Fatalf("tools/list 里没有 anc_agent_heartbeat：%+v", out.Result.Tools)
}

func TestAgentHeartbeatRecordsRegisteredAgent(t *testing.T) {
	h, _, fs := beatHarness(t)
	text, isErr := callBeat(t, h, map[string]any{
		"who": "贸易助手", "status": "running", "task_id": "t-1", "note": "在处理单据",
	})
	if isErr {
		t.Fatalf("已登记的 agent 报心跳被拒：%s", text)
	}
	if fs.got.Slug != "tradebot" || fs.got.Status != "running" || fs.got.TaskID != "t-1" {
		t.Fatalf("心跳没按登记身份落下来：%+v", fs.got)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("回话不是 JSON：%v\n%s", err, text)
	}
	if out["ok"] != true || out["slug"] != "tradebot" {
		t.Fatalf("回话契约不对：%v", out)
	}
}

// 身份不许自报：agents.md 里没有的名字，心跳不收 —— 否则看板能被点亮一个幽灵。
func TestAgentHeartbeatRejectsUnknownAgent(t *testing.T) {
	h, _, fs := beatHarness(t)
	text, isErr := callBeat(t, h, map[string]any{"who": "ghostbot"})
	if !isErr {
		t.Fatalf("查无此人的心跳居然收了：%s", text)
	}
	if !strings.Contains(text, "身份不许自报") || !strings.Contains(text, "tradebot") {
		t.Fatalf("拒绝要说清原因与现有 slug：%s", text)
	}
	if fs.got.Slug != "" {
		t.Fatalf("被拒的心跳不该写下去：%+v", fs.got)
	}
}

// 热层没接上：如实报错，不假装记下了。
func TestAgentHeartbeatUnwiredFails(t *testing.T) {
	h, g, _ := beatHarness(t)
	g.Agents = nil
	text, isErr := callBeat(t, h, map[string]any{"who": "tradebot"})
	if !isErr || !strings.Contains(text, "热层没接上") {
		t.Fatalf("没接热层却像成功了一样：isErr=%v %s", isErr, text)
	}
}
