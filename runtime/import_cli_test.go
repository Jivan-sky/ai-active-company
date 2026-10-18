package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"anc/internal/hot"
	"anc/internal/timeline"
)

type fakeImportQueue struct{ m map[string]hot.State }

func (f *fakeImportQueue) EnqueueTask(spec hot.TaskSpec) (hot.State, error) {
	if _, ok := f.m[spec.ID]; ok {
		return hot.State{}, fmt.Errorf("duplicate %s", spec.ID)
	}
	s := hot.State{
		ID: spec.ID, To: spec.To, By: spec.By, Title: spec.Title, Note: spec.Note,
		Project: spec.Project, Priority: spec.Priority, Root: spec.Root,
		Stage: hot.StageQueued, Status: "queued", Version: 1, Refs: spec.Refs,
	}
	f.m[spec.ID] = s
	return s, nil
}

func (f *fakeImportQueue) List() ([]hot.State, error) {
	out := make([]hot.State, 0, len(f.m))
	for _, s := range f.m {
		out = append(out, s)
	}
	return out, nil
}

func writeReport(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, "2026-10", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestImportReportsTimelineAndTasksIdempotent(t *testing.T) {
	vault := t.TempDir()
	reports := t.TempDir()
	writeReport(t, reports, "2026-10-01.md", `---
created: 2026-10-01
tags:
  - project/demo
---
## ✅ 完成事项
- **A 事项**：做完了 A。
## ⏳ 遗留待办
- [ ] **B 事项**（等接口）
- [x] **C 事项**
`)
	writeReport(t, reports, "2026-10-02.md", `---
created: 2026-10-02
---
## ⏳ 遗留待办
- [ ] **B 事项**（还在等）
`)

	q := &fakeImportQueue{m: map[string]hot.State{}}
	old := openImportQueue
	openImportQueue = func(*taskOpts) (importQueue, error) { return q, nil }
	t.Cleanup(func() { openImportQueue = old })

	args := []string{vault, reports, "--apply", "--to", "demo-bot", "--by", "test-import", "--json"}
	if code := cmdImportReports(args); code != 0 {
		t.Fatalf("第一次导入退出码 %d", code)
	}
	doc, err := timeline.Load(vault)
	if err != nil {
		t.Fatal(err)
	}
	// A 完成 + C 已完成待办 = 2 条；B 只有一条任务（10-02 覆盖 10-01）。
	if len(doc.Entries) != 2 {
		t.Fatalf("timeline 条数 = %d，想要 2：%+v", len(doc.Entries), doc.Entries)
	}
	if len(q.m) != 1 {
		t.Fatalf("任务数 = %d，想要 1", len(q.m))
	}
	for _, s := range q.m {
		if s.To != "demo-bot" || s.Root != "report:2026-10-02" {
			t.Fatalf("任务落点不对：%+v", s)
		}
	}

	if code := cmdImportReports(args); code != 0 {
		t.Fatalf("第二次导入退出码 %d", code)
	}
	doc2, _ := timeline.Load(vault)
	if len(doc2.Entries) != 2 {
		t.Fatalf("重导后 timeline 条数 = %d，想要仍是 2", len(doc2.Entries))
	}
	if len(q.m) != 1 {
		t.Fatalf("重导后任务数 = %d，想要仍是 1", len(q.m))
	}
}

func TestImportReportsNoTasksDoesNotOpenQueue(t *testing.T) {
	vault := t.TempDir()
	reports := t.TempDir()
	writeReport(t, reports, "2026-10-01.md", `---
created: 2026-10-01
---
## ⏳ 遗留待办
- [ ] **只有待办**
`)
	old := openImportQueue
	openImportQueue = func(*taskOpts) (importQueue, error) { return nil, fmt.Errorf("不该连热层") }
	t.Cleanup(func() { openImportQueue = old })

	if code := cmdImportReports([]string{vault, reports, "--apply", "--no-tasks"}); code != 0 {
		t.Fatalf("--no-tasks 不应连热层，退出码 %d", code)
	}
}
