package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"anc/internal/hot"
	"anc/internal/timeline"
)

type fakeSettleStore struct {
	m       map[string]hot.State
	getErr  error
	dropErr error
	dropped []string
}

func (f *fakeSettleStore) Get(id string) (hot.State, bool, error) {
	if f.getErr != nil {
		return hot.State{}, false, f.getErr
	}
	s, ok := f.m[id]
	return s, ok, nil
}

func (f *fakeSettleStore) Drop(id string) error {
	if f.dropErr != nil {
		return f.dropErr
	}
	delete(f.m, id)
	f.dropped = append(f.dropped, id)
	return nil
}

var settleAt = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func settledTask() hot.State {
	return hot.State{
		ID: "t-1", Status: "done", By: "anc", From: "anc", To: "tradebot",
		Title: "处理单据", Note: "已答复客户", Domain: "trade", Project: "p-1",
		Session: "s-1", Intent: "queue", Version: 3,
		AsOf: "2026-10-18T11:00:00Z",
	}
}

func TestSettleTaskWritesEntryAndDrops(t *testing.T) {
	vault := t.TempDir()
	st := &fakeSettleStore{m: map[string]hot.State{"t-1": settledTask()}}

	res, err := settleTask(st, vault, "t-1", taskSettleOpts{At: settleAt})
	if err != nil {
		t.Fatalf("结账失败：%v", err)
	}
	if res.Already || res.Path == "" {
		t.Fatalf("应是新写一条：%+v", res)
	}
	e := res.Entry
	if e.Case != "t-1" || e.ID != "task-t-1" || e.Status != "done" || e.By != "tradebot" {
		t.Fatalf("落库字段不对：%+v", e)
	}
	if e.Title != "处理单据" || e.Domain != "trade" || e.Project != "p-1" || e.To != "tradebot" {
		t.Fatalf("任务字段没带过去：%+v", e)
	}
	if !strings.Contains(e.Detail, "已答复客户") || !strings.Contains(e.Detail, "会话 s-1") ||
		!strings.Contains(e.Detail, "v3") || !strings.Contains(e.Detail, "派单 anc") {
		t.Fatalf("detail 应带上正文 / 会话 / 版本：%q", e.Detail)
	}
	if len(st.dropped) != 1 || st.dropped[0] != "t-1" {
		t.Fatalf("落库后热层应清走：%v", st.dropped)
	}
	doc, err := timeline.Load(vault)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Entries) != 1 || doc.Entries[0].Case != "t-1" {
		t.Fatalf("timeline 应有一条：%+v", doc.Entries)
	}
	if timeline.Band(doc.Entries[0].Status) != timeline.BandGreen {
		t.Fatalf("done 应落绿档：%s", doc.Entries[0].Status)
	}
}

func TestSettleTaskIsIdempotent(t *testing.T) {
	vault := t.TempDir()
	st := &fakeSettleStore{m: map[string]hot.State{"t-1": settledTask()}}
	if _, err := settleTask(st, vault, "t-1", taskSettleOpts{At: settleAt}); err != nil {
		t.Fatal(err)
	}
	res, err := settleTask(st, vault, "t-1", taskSettleOpts{At: settleAt.Add(time.Hour)})
	if err != nil {
		t.Fatalf("重复结账不该报错：%v", err)
	}
	if !res.Already || res.Path != "" {
		t.Fatalf("重复结账应只报已沉淀：%+v", res)
	}
	doc, err := timeline.Load(vault)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Entries) != 1 {
		t.Fatalf("重复结账不该追加第二条：%d", len(doc.Entries))
	}
}

func TestSettleTaskRefusesOpenTask(t *testing.T) {
	vault := t.TempDir()
	queued := settledTask()
	queued.Status = "queued"
	st := &fakeSettleStore{m: map[string]hot.State{"t-1": queued}}
	if _, err := settleTask(st, vault, "t-1", taskSettleOpts{At: settleAt}); err == nil ||
		!strings.Contains(err.Error(), "还在队列里") {
		t.Fatalf("queued 默认不该结账：%v", err)
	}
	if len(st.dropped) != 0 {
		t.Fatalf("被拒时热层不许动：%v", st.dropped)
	}

	running := settledTask()
	running.Status = "running"
	st = &fakeSettleStore{m: map[string]hot.State{"t-1": running}}
	if _, err := settleTask(st, vault, "t-1", taskSettleOpts{At: settleAt}); err == nil ||
		!strings.Contains(err.Error(), "还在进行中") {
		t.Fatalf("running 默认不该结账：%v", err)
	}

	res, err := settleTask(st, vault, "t-1", taskSettleOpts{At: settleAt, Status: "cancelled", Note: "客户撤回"})
	if err != nil {
		t.Fatalf("显式给终态应能结账：%v", err)
	}
	if res.Entry.Status != "cancelled" || !strings.Contains(res.Entry.Detail, "客户撤回") {
		t.Fatalf("显式终态没落上：%+v", res.Entry)
	}
}

func TestSettleTaskKeepsHotWhenTimelineFails(t *testing.T) {
	vault := t.TempDir()
	dir := filepath.Join(vault, timeline.DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "2026-10.human.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	st := &fakeSettleStore{m: map[string]hot.State{"t-1": settledTask()}}
	_, err := settleTask(st, vault, "t-1", taskSettleOpts{At: settleAt, By: "human"})
	if err == nil || !strings.Contains(err.Error(), "落库失败") {
		t.Fatalf("落库失败应报出来：%v", err)
	}
	if len(st.dropped) != 0 {
		t.Fatalf("落库失败时热层一个字都不许动：%v", st.dropped)
	}
}

func TestSettleTaskReportsDropFailure(t *testing.T) {
	vault := t.TempDir()
	st := &fakeSettleStore{m: map[string]hot.State{"t-1": settledTask()}, dropErr: errors.New("redis down")}
	res, err := settleTask(st, vault, "t-1", taskSettleOpts{At: settleAt})
	if err == nil || !strings.Contains(err.Error(), "库已落") {
		t.Fatalf("清热层失败要说清已落库：%v", err)
	}
	if res.Path == "" {
		t.Fatalf("应把落库路径带回来：%+v", res)
	}
	doc, _ := timeline.Load(vault)
	if len(doc.Entries) != 1 {
		t.Fatalf("库应已落一条：%d", len(doc.Entries))
	}
}
