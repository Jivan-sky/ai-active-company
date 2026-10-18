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
	"anc/internal/timeline"
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
		ID: spec.ID, To: spec.To, By: spec.By, From: spec.By, Title: spec.Title, Note: spec.Note,
		Domain: spec.Domain, Project: spec.Project, Session: spec.Session,
		Intent: spec.Intent, Status: "queued", Stage: hot.StageQueued, Version: 1,
		Root: spec.Root, Parent: spec.Parent, Accept: spec.Accept, DueAt: spec.DueAt, Priority: spec.Priority,
		Refs:   spec.Refs,
		MaxHop: spec.MaxHop,
		AsOf:   time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}
	if s.MaxAttempt <= 0 {
		s.MaxAttempt = hot.DefaultTaskMaxAttempt
	}
	if s.MaxHop <= 0 {
		s.MaxHop = hot.DefaultTaskMaxHop
	}
	if s.Root == "" {
		s.Root = s.ID
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
		if s.To != to || s.Stage != hot.StageQueued {
			continue
		}
		s.Status, s.Stage, s.By, s.Fence, s.Version = "running", hot.StageRunning, to, s.Fence+1, s.Version+1
		if s.Session == "" {
			s.Session = session
		}
		f.m[id] = s
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeTasks) AckTask(id, who, status, note string) (hot.State, error) {
	return f.ack(id, who, 0, status, note)
}

func (f *fakeTasks) AckTaskFenced(id, who string, fence int64, status, note string) (hot.State, error) {
	return f.ack(id, who, fence, status, note)
}

func (f *fakeTasks) ack(id, who string, fence int64, status, note string) (hot.State, error) {
	s, ok := f.m[id]
	if !ok {
		return hot.State{}, hot.ErrNotFound
	}
	if s.To != who {
		return hot.State{}, fmt.Errorf("not assignee")
	}
	if fence != 0 && s.Fence != 0 && fence != s.Fence {
		return hot.State{}, fmt.Errorf("stale fence")
	}
	s.Status, s.Version = status, s.Version+1
	s.Stage = hot.StageForStatus(status, s.Stage)
	if note != "" {
		s.Note = note
	}
	f.m[id] = s
	return s, nil
}

func (f *fakeTasks) HandoffTask(id, who string, fence int64, to, note string, refs []string, crossDomain bool) (hot.State, error) {
	s, ok := f.m[id]
	if !ok {
		return hot.State{}, hot.ErrNotFound
	}
	if s.Stage != hot.StageRunning || s.To != who {
		return hot.State{}, fmt.Errorf("not holder")
	}
	if fence == 0 || s.Fence != fence {
		return hot.State{}, fmt.Errorf("stale fence")
	}
	if strings.TrimSpace(note) == "" {
		return hot.State{}, fmt.Errorf("note required")
	}
	s.Hop++
	s.Handoff = hot.Handoff{From: who, To: to, Note: note, Refs: refs, CrossDomain: crossDomain, At: s.AsOf}
	if len(refs) > 0 {
		s.Refs = append(s.Refs, refs...)
	}
	if s.Hop > s.MaxHop {
		s.Stage, s.Status = hot.StageBlocked, "blocked"
	} else {
		s.Stage, s.Status = hot.StageQueued, "queued"
	}
	s.To, s.By, s.Version = to, "", s.Version+1
	f.m[id] = s
	return s, nil
}

func (f *fakeTasks) SubmitTask(id, who string, fence int64, reviewer, accept, note string, refs []string) (hot.State, error) {
	s, ok := f.m[id]
	if !ok {
		return hot.State{}, hot.ErrNotFound
	}
	if s.Stage != hot.StageRunning || s.To != who {
		return hot.State{}, fmt.Errorf("not holder")
	}
	if fence == 0 || s.Fence != fence {
		return hot.State{}, fmt.Errorf("stale fence")
	}
	if accept == "" {
		accept = s.Accept
	}
	if accept == "" {
		return hot.State{}, fmt.Errorf("accept required")
	}
	s.Accept = accept
	if len(refs) > 0 {
		s.Refs = append(s.Refs, refs...)
	}
	s.Review = hot.Review{By: reviewer, Verdict: hot.VerdictPending, Note: note, At: s.AsOf, Self: s.By == reviewer}
	s.Stage, s.Status, s.To, s.By = hot.StageReview, "review", reviewer, who
	s.Version++
	f.m[id] = s
	return s, nil
}

func (f *fakeTasks) VerifyTask(id, who, verdict, note string) (hot.State, error) {
	s, ok := f.m[id]
	if !ok {
		return hot.State{}, hot.ErrNotFound
	}
	if s.Stage != hot.StageReview || s.Review.By != who {
		return hot.State{}, fmt.Errorf("not reviewer")
	}
	switch strings.ToLower(strings.TrimSpace(verdict)) {
	case "accepted", "accept":
		verdict = hot.VerdictAccepted
	case "rejected", "reject":
		verdict = hot.VerdictRejected
	default:
		return hot.State{}, fmt.Errorf("bad verdict")
	}
	if verdict == hot.VerdictRejected && strings.TrimSpace(note) == "" {
		return hot.State{}, fmt.Errorf("note required")
	}
	s.Review = hot.Review{By: who, Verdict: verdict, Note: note, At: s.AsOf, Self: s.Review.Self}
	if verdict == hot.VerdictAccepted {
		s.Stage, s.Status = hot.StageDone, "done"
	} else {
		s.Stage, s.Status, s.To, s.By = hot.StageQueued, "queued", s.By, ""
	}
	s.Version++
	f.m[id] = s
	return s, nil
}

func (f *fakeTasks) EscalateTask(id, who string, fence int64, to, note string) (hot.State, error) {
	s, ok := f.m[id]
	if !ok {
		return hot.State{}, hot.ErrNotFound
	}
	if strings.TrimSpace(note) == "" {
		return hot.State{}, fmt.Errorf("note required")
	}
	if s.Stage == hot.StageRunning {
		if s.To != who || fence == 0 || s.Fence != fence {
			return hot.State{}, fmt.Errorf("not holder")
		}
	} else if s.Stage != hot.StageReview {
		return hot.State{}, fmt.Errorf("bad stage")
	}
	s.Stage, s.Status, s.To = hot.StageBlocked, "blocked", to
	s.Version++
	f.m[id] = s
	return s, nil
}

func (f *fakeTasks) Get(id string) (hot.State, bool, error) {
	s, ok := f.m[id]
	return s, ok, nil
}

func (f *fakeTasks) Drop(id string) error {
	delete(f.m, id)
	return nil
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

func TestTaskHandoffSubmitVerifyEscalateViaMCP(t *testing.T) {
	h, _, _ := taskHarness(t)
	text, isErr := callTool(t, h, "anc_task", map[string]any{
		"action": "enqueue", "id": "t-flow", "to": "tradebot", "title": "跑一遍交接",
		"accept": "抽检 200 通且零漏检", "refs": []string{"doc-1"},
	})
	if isErr {
		t.Fatalf("入队失败：%s", text)
	}
	text, isErr = callTool(t, h, "anc_task", map[string]any{"action": "pull", "who": "tradebot"})
	if isErr {
		t.Fatalf("pull 失败：%s", text)
	}
	var pulled map[string]any
	if err := json.Unmarshal([]byte(text), &pulled); err != nil {
		t.Fatal(err)
	}
	fence := int64(pulled["tasks"].([]any)[0].(map[string]any)["fence"].(float64))

	text, isErr = callTool(t, h, "anc_task", map[string]any{
		"action": "handoff", "id": "t-flow", "who": "tradebot", "to": "shipbot",
		"fence": fence, "note": "这活该物流接", "refs": []string{"doc-2"},
	})
	if isErr {
		t.Fatalf("handoff 失败：%s", text)
	}
	var ho map[string]any
	if err := json.Unmarshal([]byte(text), &ho); err != nil {
		t.Fatal(err)
	}
	if ho["cross_domain"] != true {
		t.Fatalf("不同域交接应标成跨域：%v", ho)
	}
	ht := ho["task"].(map[string]any)
	if ht["stage"] != "queued" || ht["hop"] != float64(1) || ht["to"] != "shipbot" || ht["by"] != "" {
		t.Fatalf("handoff 契约不对：%v", ht)
	}
	if ht["accept"] != "抽检 200 通且零漏检" || ht["root"] != "t-flow" {
		t.Fatalf("handoff 不该丢验收口径 / 根：%v", ht)
	}

	text, isErr = callTool(t, h, "anc_task", map[string]any{"action": "pull", "who": "shipbot"})
	if isErr {
		t.Fatalf("shipbot pull 失败：%s", text)
	}
	var pulled2 map[string]any
	if err := json.Unmarshal([]byte(text), &pulled2); err != nil {
		t.Fatal(err)
	}
	fence2 := int64(pulled2["tasks"].([]any)[0].(map[string]any)["fence"].(float64))

	text, isErr = callTool(t, h, "anc_task", map[string]any{
		"action": "submit", "id": "t-flow", "who": "shipbot", "reviewer": "alice",
		"fence": fence2, "note": "做完了",
	})
	if isErr {
		t.Fatalf("submit 失败：%s", text)
	}
	var sub map[string]any
	if err := json.Unmarshal([]byte(text), &sub); err != nil {
		t.Fatal(err)
	}
	st := sub["task"].(map[string]any)
	review := st["review"].(map[string]any)
	if st["stage"] != "review" || st["to"] != "alice" || review["verdict"] != "pending" {
		t.Fatalf("submit 契约不对：%v", st)
	}

	text, isErr = callTool(t, h, "anc_task", map[string]any{
		"action": "verify", "id": "t-flow", "who": "alice", "verdict": "rejected", "note": "漏了 3 通",
	})
	if isErr {
		t.Fatalf("reject 失败：%s", text)
	}
	var rej map[string]any
	if err := json.Unmarshal([]byte(text), &rej); err != nil {
		t.Fatal(err)
	}
	rt := rej["task"].(map[string]any)
	if rt["stage"] != "queued" || rt["to"] != "shipbot" {
		t.Fatalf("驳回后应回作者队列：%v", rt)
	}

	text, isErr = callTool(t, h, "anc_task", map[string]any{"action": "pull", "who": "shipbot"})
	if isErr {
		t.Fatalf("二次 pull 失败：%s", text)
	}
	var pulled3 map[string]any
	if err := json.Unmarshal([]byte(text), &pulled3); err != nil {
		t.Fatal(err)
	}
	fence3 := int64(pulled3["tasks"].([]any)[0].(map[string]any)["fence"].(float64))
	callTool(t, h, "anc_task", map[string]any{
		"action": "submit", "id": "t-flow", "who": "shipbot", "reviewer": "alice", "fence": fence3,
	})
	text, isErr = callTool(t, h, "anc_task", map[string]any{
		"action": "verify", "id": "t-flow", "who": "alice", "verdict": "accepted",
	})
	if isErr {
		t.Fatalf("验收通过失败：%s", text)
	}
	var acc map[string]any
	if err := json.Unmarshal([]byte(text), &acc); err != nil {
		t.Fatal(err)
	}
	if acc["task"].(map[string]any)["stage"] != "done" {
		t.Fatalf("验收通过后应是 done：%v", acc)
	}

	callTool(t, h, "anc_task", map[string]any{"action": "enqueue", "id": "t-esc", "to": "tradebot", "title": "卡点"})
	text, _ = callTool(t, h, "anc_task", map[string]any{"action": "pull", "who": "tradebot"})
	var escPull map[string]any
	if err := json.Unmarshal([]byte(text), &escPull); err != nil {
		t.Fatal(err)
	}
	escFence := int64(escPull["tasks"].([]any)[0].(map[string]any)["fence"].(float64))
	text, isErr = callTool(t, h, "anc_task", map[string]any{
		"action": "escalate", "id": "t-esc", "who": "tradebot", "to": "bob",
		"fence": escFence, "note": "等客户确认口径",
	})
	if isErr {
		t.Fatalf("escalate 失败：%s", text)
	}
	var esc map[string]any
	if err := json.Unmarshal([]byte(text), &esc); err != nil {
		t.Fatal(err)
	}
	et := esc["task"].(map[string]any)
	if et["stage"] != "blocked" || et["to"] != "bob" {
		t.Fatalf("升级后应是 blocked 且带下一手：%v", et)
	}
}

func TestTaskSettleThroughMCP(t *testing.T) {
	h, g, _ := beatHarness(t)
	ft := newFakeTasks()
	g.Tasks = ft

	callTool(t, h, "anc_task", map[string]any{"action": "enqueue", "id": "t-9", "to": "tradebot", "title": "归档测试", "session": "s-1"})
	callTool(t, h, "anc_task", map[string]any{"action": "pull", "who": "tradebot"})
	callTool(t, h, "anc_task", map[string]any{"action": "ack", "id": "t-9", "who": "tradebot", "status": "done", "note": "做完"})

	text, isErr := callTool(t, h, "anc_task", map[string]any{"action": "settle", "id": "t-9", "who": "tradebot"})
	if isErr {
		t.Fatalf("settle 失败：%s", text)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	if out["already"] != false || out["timeline"] == nil {
		t.Fatalf("settle 契约不对：%v", out)
	}
	entry := out["entry"].(map[string]any)
	if entry["case"] != "t-9" || entry["status"] != "done" || entry["by"] != "tradebot" {
		t.Fatalf("落库字段不对：%v", entry)
	}
	if _, ok := ft.m["t-9"]; ok {
		t.Fatal("settle 后热层应清走")
	}

	text, isErr = callTool(t, h, "anc_task", map[string]any{"action": "settle", "id": "t-9"})
	if isErr || !strings.Contains(text, "早已沉淀") {
		t.Fatalf("重复 settle 应幂等：isErr=%v %s", isErr, text)
	}
	doc, err := timeline.Load(g.Vault)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Entries) != 1 {
		t.Fatalf("timeline 只该有一条：%d", len(doc.Entries))
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
