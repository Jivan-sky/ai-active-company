package board

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"anc/internal/hot"
	"anc/internal/org"
)

type fakeAgents struct {
	list  []hot.AgentHeartbeat
	err   error
	stale time.Duration
}

func (f *fakeAgents) ListAgents() ([]hot.AgentHeartbeat, error) { return f.list, f.err }
func (f *fakeAgents) AgentStale() time.Duration                 { return f.stale }

func demoVault(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", "testdata", "demo-public", "vault"))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func agentsOfView(t *testing.T, s *Server) AgentsView {
	t.Helper()
	rec := get(t, s.Handler(), "/api/agents")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 %d，期望 200：%s", rec.Code, rec.Body.String())
	}
	var v AgentsView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("不是合法 JSON：%v", err)
	}
	if v.Schema != AgentsSchema {
		t.Fatalf("schema=%q，期望 %s", v.Schema, AgentsSchema)
	}
	return v
}

// 心跳层没接上：必须说「没接上」，不能拿「全是离线」冒充真实在线态。
func TestAgentsUnwiredSaysWhy(t *testing.T) {
	v := agentsOfView(t, &Server{Vault: demoVault(t), Now: func() time.Time { return fixedNow }})
	if v.Wired {
		t.Fatal("没接心跳层却报 wired=true")
	}
	if v.Why == "" {
		t.Fatal("wired=false 必须带 why（怎么接上）")
	}
}

// 已登记但从未心跳：total 照出注册表，online=0 且 last_seen 为空 ——
// 「从未心跳」与「离线很久」不是同一句话。
func TestAgentsRegisteredButNeverBeat(t *testing.T) {
	vault := demoVault(t)
	o, err := org.Load(vault)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		Vault: vault, Now: func() time.Time { return fixedNow },
		Agents: &fakeAgents{stale: 2 * time.Minute},
	}
	v := agentsOfView(t, s)
	if !v.Wired || v.Error != "" {
		t.Fatalf("接上了却报错：%+v", v)
	}
	if v.Total != len(o.Agents) || len(v.Entries) != len(o.Agents) {
		t.Fatalf("注册表没出全：total=%d entries=%d want=%d", v.Total, len(v.Entries), len(o.Agents))
	}
	if v.Online != 0 {
		t.Fatalf("没有心跳却报了 %d 个在线", v.Online)
	}
	for _, e := range v.Entries {
		if e.Online || e.LastSeen != "" {
			t.Fatalf("从未心跳的 %s 不该有在线/最后心跳：%+v", e.Slug, e)
		}
	}
}

// 心跳在阈值内 = 在线；超过阈值 = 离线但保留最后心跳。不能再拿「最近调用」冒充在线。
func TestAgentsOnlineAndStaleAreSeparated(t *testing.T) {
	vault := demoVault(t)
	s := &Server{
		Vault: vault, Now: func() time.Time { return fixedNow },
		Agents: &fakeAgents{stale: 2 * time.Minute, list: []hot.AgentHeartbeat{
			{Slug: "receptionbot", Status: "running", TaskID: "t-1", AsOf: fixedNow.Add(-10 * time.Second).Format(time.RFC3339)},
			{Slug: "shopinitbot", Status: "idle", AsOf: fixedNow.Add(-10 * time.Minute).Format(time.RFC3339)},
		}},
	}
	v := agentsOfView(t, s)
	if v.Online != 1 {
		t.Fatalf("在线数应为 1，实际 %d：%+v", v.Online, v.Entries)
	}
	bySlug := map[string]AgentView{}
	for _, e := range v.Entries {
		bySlug[e.Slug] = e
	}
	live := bySlug["receptionbot"]
	if !live.Online || live.Stale || live.LastSeen == "" || live.AgeSecs == nil {
		t.Fatalf("阈值内的心跳应算在线：%+v", live)
	}
	if live.TaskID != "t-1" {
		t.Fatalf("当前任务没带上：%+v", live)
	}
	old := bySlug["shopinitbot"]
	if old.Online || !old.Stale {
		t.Fatalf("过期心跳应算离线且打 stale 标记：%+v", old)
	}
	if old.LastSeen == "" {
		t.Fatal("过期心跳不许把「最后心跳时间」抹掉")
	}
}

// 心跳读不了：wired=true + error，注册表照出；不拿空列表冒充「没有 agent」。
func TestAgentsReadErrorStillShowsRegistry(t *testing.T) {
	vault := demoVault(t)
	o, err := org.Load(vault)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		Vault: vault, Now: func() time.Time { return fixedNow },
		Agents: &fakeAgents{err: errors.New("redis down"), stale: 2 * time.Minute},
	}
	v := agentsOfView(t, s)
	if !v.Wired || v.Error == "" {
		t.Fatalf("读失败要说清楚：%+v", v)
	}
	if v.Total != len(o.Agents) {
		t.Fatalf("读失败时仍应出注册表：total=%d want=%d", v.Total, len(o.Agents))
	}
}
