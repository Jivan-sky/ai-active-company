package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"anc/internal/hot"
)

type fakeTasks struct {
	m map[string]hot.State
}

func newFakeTasks() *fakeTasks { return &fakeTasks{m: map[string]hot.State{}} }

func (f *fakeTasks) EnqueueTask(spec hot.TaskSpec) (hot.State, error) {
	if _, ok := f.m[spec.ID]; ok {
		return hot.State{}, fmt.Errorf("duplicate %s", spec.ID)
	}
	s := hot.State{
		ID: spec.ID, To: spec.To, By: spec.By, Title: spec.Title, Note: spec.Note,
		Domain: spec.Domain, Project: spec.Project, Session: spec.Session,
		Intent: spec.Intent, Status: "queued", Version: 1,
		AsOf: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}
	f.m[s.ID] = s
	return s, nil
}

func (f *fakeTasks) PullTasks(to string, limit int, _ time.Duration, session string) ([]hot.State, error) {
	ids := f.ids()
	out := []hot.State{}
	for _, id := range ids {
		if len(out) >= limit {
			break
		}
		s := f.m[id]
		if s.To != to || s.Status != "queued" {
			continue
		}
		s.Status, s.By, s.Version = "running", to, s.Version+1
		if s.Session == "" {
			s.Session = session
		}
		f.m[id] = s
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeTasks) AckTask(id, who, status, note string) (hot.State, error) {
	s, ok := f.m[id]
	if !ok {
		return hot.State{}, hot.ErrNotFound
	}
	if s.To != who {
		return hot.State{}, fmt.Errorf("not assignee")
	}
	s.Status, s.Version = status, s.Version+1
	if note != "" {
		s.Note = note
	}
	f.m[id] = s
	return s, nil
}

func (f *fakeTasks) List() ([]hot.State, error) {
	out := make([]hot.State, 0, len(f.m))
	for _, id := range f.ids() {
		out = append(out, f.m[id])
	}
	return out, nil
}

func (f *fakeTasks) ids() []string {
	ids := make([]string, 0, len(f.m))
	for id := range f.m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func callTool(t *testing.T, h http.Handler, name string, args map[string]any) (string, bool) {
	t.Helper()
	out := mcpRoundTrip(t, h, "tools/call", map[string]any{"name": name, "arguments": args})
	if len(out.Result.Content) == 0 {
		t.Fatalf("回话里没有 content：%+v", out.Result)
	}
	return out.Result.Content[0].Text, out.Result.IsError
}

func taskHarness(t *testing.T) (http.Handler, *fakeTasks, *fakeBeatStore) {
	h, g, beats := beatHarness(t)
	ft := newFakeTasks()
	g.Tasks = ft
	return h, ft, beats
}

func TestTaskPullModelAndVersion(t *testing.T) {
	h, _, _ := taskHarness(t)
	text, isErr := callTool(t, h, "anc_task", map[string]any{
		"action": "enqueue", "id": "t-1", "to": "贸易助手", "title": "处理单据",
		"session": "s-1", "intent": "queue",
	})
	if isErr {
		t.Fatalf("入队失败：%s", text)
	}
	var enq map[string]any
	if err := json.Unmarshal([]byte(text), &enq); err != nil {
		t.Fatal(err)
	}
	task := enq["task"].(map[string]any)
	if task["status"] != "queued" || task["version"] != float64(1) {
		t.Fatalf("入队契约不对：%v", task)
	}

	text, isErr = callTool(t, h, "anc_task", map[string]any{"action": "pull", "who": "tradebot", "limit": 1})
	if isErr {
		t.Fatalf("pull 失败：%s", text)
	}
	var pulled map[string]any
	if err := json.Unmarshal([]byte(text), &pulled); err != nil {
		t.Fatal(err)
	}
	tasks := pulled["tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("应拉到一条：%v", pulled)
	}
	p := tasks[0].(map[string]any)
	if p["status"] != "running" || p["version"] != float64(2) || p["to"] != "tradebot" {
		t.Fatalf("pull 后应是 running / v2：%v", p)
	}
	if p["session"] != "s-1" {
		t.Fatalf("session 没带出来：%v", p)
	}

	text, isErr = callTool(t, h, "anc_task", map[string]any{
		"action": "ack", "id": "t-1", "who": "tradebot", "status": "done", "note": "完成",
	})
	if isErr {
		t.Fatalf("ack 失败：%s", text)
	}
	var ack map[string]any
	if err := json.Unmarshal([]byte(text), &ack); err != nil {
		t.Fatal(err)
	}
	at := ack["task"].(map[string]any)
	if at["status"] != "done" || at["version"] != float64(3) || at["note"] != "完成" {
		t.Fatalf("ack 契约不对：%v", at)
	}
}

func TestTaskEnqueueReportsRunningConflict(t *testing.T) {
	h, _, _ := taskHarness(t)
	callTool(t, h, "anc_task", map[string]any{"action": "enqueue", "id": "t-1", "to": "tradebot", "title": "先做"})
	callTool(t, h, "anc_task", map[string]any{"action": "pull", "who": "tradebot"})
	text, _ := callTool(t, h, "anc_task", map[string]any{
		"action": "enqueue", "id": "t-2", "to": "tradebot", "title": "再做一件", "intent": "queue",
	})
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	if out["active_running"] != float64(1) || out["queue_behind_running"] != true {
		t.Fatalf("有任务在跑时应如实报冲突：%v", out)
	}
	text, _ = callTool(t, h, "anc_task", map[string]any{
		"action": "enqueue", "id": "t-3", "to": "tradebot", "title": "打断一下", "intent": "interrupt",
	})
	out = map[string]any{}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	if out["interrupt_requested"] != true {
		t.Fatalf("intent=interrupt 应标成请求：%v", out)
	}
	if !strings.Contains(out["note"].(string), "agent 自己决定") {
		t.Fatalf("必须写清 ANC 不替 agent 决定：%v", out["note"])
	}
}

func TestTaskRejectsUnknownAgent(t *testing.T) {
	h, ft, _ := taskHarness(t)
	text, isErr := callTool(t, h, "anc_task", map[string]any{
		"action": "enqueue", "to": "ghostbot", "title": "幽灵任务",
	})
	if !isErr || !strings.Contains(text, "身份不许自报") {
		t.Fatalf("查无此人应被拒：isErr=%v %s", isErr, text)
	}
	if len(ft.m) != 0 {
		t.Fatalf("被拒的任务不该入队：%+v", ft.m)
	}
}

func TestSessionRotateAndGet(t *testing.T) {
	h, _, beats := taskHarness(t)
	text, isErr := callTool(t, h, "anc_session", map[string]any{
		"action": "rotate", "who": "tradebot", "session_id": "s-new",
	})
	if isErr {
		t.Fatalf("rotate 失败：%s", text)
	}
	if beats.got.Session != "s-new" {
		t.Fatalf("新会话没记在心跳上：%+v", beats.got)
	}
	beats.has = true
	text, isErr = callTool(t, h, "anc_session", map[string]any{"action": "get", "who": "tradebot"})
	if isErr {
		t.Fatalf("get 失败：%s", text)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	if out["session"] != "s-new" {
		t.Fatalf("get 没回当前会话：%v", out)
	}
}
