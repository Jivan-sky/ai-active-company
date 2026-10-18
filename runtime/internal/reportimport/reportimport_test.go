package reportimport

import (
	"strings"
	"testing"
)

const emojiReport = `---
title: "2026-10-01 每日记录"
created: 2026-10-01
tags:
  - type/daily
  - project/demo
---

## ✅ 完成事项

### 🛠️ 主线

- **A 事项**：做完了 A。

## 🧭 关键决策

- **选 B** — 因为 B 更便宜。

## ⏳ 遗留待办

- [ ] **C 事项**（等接口）
- [x] **D 事项**
- ~~E 事项~~
`

func TestParseEmojiReport(t *testing.T) {
	r := Parse("2026-10/2026-10-01.md", []byte(emojiReport))
	if r.Date != "2026-10-01" {
		t.Fatalf("日期读到 %q", r.Date)
	}
	if len(r.Items) != 5 {
		t.Fatalf("条目数 = %d，想要 5：%+v", len(r.Items), r.Items)
	}
	if r.Items[0].Section != SectionDone || r.Items[0].Title != "A 事项" || r.Items[0].Detail != "做完了 A。" {
		t.Fatalf("完成事项解析不对：%+v", r.Items[0])
	}
	if r.Items[0].Project != "主线" {
		t.Fatalf("项目子标题没接住：%+v", r.Items[0])
	}
	if r.Items[1].Section != SectionDecision || r.Items[1].Title != "选 B" {
		t.Fatalf("决策解析不对：%+v", r.Items[1])
	}
	if r.Items[2].State != StateOpen || r.Items[2].Title != "C 事项" || r.Items[2].Detail != "（等接口）" {
		t.Fatalf("未完成待办解析不对：%+v", r.Items[2])
	}
	if r.Items[3].State != StateDone {
		t.Fatalf("已完成待办解析不对：%+v", r.Items[3])
	}
	if r.Items[4].State != StateDropped {
		t.Fatalf("划掉的待办应 dropped：%+v", r.Items[4])
	}
}

const tableReport = `---
created: 2026-08-28
tags:
  - project/workorder
---

## 今日待办

| # | 任务 | 优先级 | 来源 | 状态 |
|---|------|--------|------|------|
| 1 | 写报告 | 🔴 高 | 承接昨日 | ⬜ |
| 2 | 修 hook | 🟢 低 | 新增 | ✅ |
`

func TestParseTodoTable(t *testing.T) {
	r := Parse("2026-08/2026-08-28.md", []byte(tableReport))
	if len(r.Items) != 2 {
		t.Fatalf("表格条目数 = %d，想要 2：%+v", len(r.Items), r.Items)
	}
	if r.Items[0].Title != "写报告" || r.Items[0].State != StateOpen {
		t.Fatalf("第一行解析不对：%+v", r.Items[0])
	}
	if !strings.Contains(r.Items[0].Detail, "优先级：🔴 高") {
		t.Fatalf("优先级没带进 detail：%q", r.Items[0].Detail)
	}
	if r.Items[1].State != StateDone {
		t.Fatalf("第二行应已完成：%+v", r.Items[1])
	}
}

func TestBuildLatestTodoWins(t *testing.T) {
	d1 := `---
created: 2026-10-01
---
## ⏳ 遗留待办

- [ ] **X 事项**（等接口）
- [ ] **Y 事项**
- ~~**Z 事项**~~
`
	d2 := `---
created: 2026-10-03
---
## ⏳ 遗留待办

- [x] **X 事项**（接口到了）
- [ ] **Y 事项**
`
	p := Build([]Report{
		Parse("2026-10/2026-10-01.md", []byte(d1)),
		Parse("2026-10/2026-10-03.md", []byte(d2)),
	})
	if len(p.Tasks) != 1 || p.Tasks[0].Title != "Y 事项" {
		t.Fatalf("应只剩 Y 一条任务：%+v", p.Tasks)
	}
	if p.Superseded != 2 {
		t.Fatalf("被覆盖条数 = %d，想要 2", p.Superseded)
	}
	var done int
	for _, e := range p.Entries {
		if e.Section == SectionTodo && e.State == StateDone {
			done++
		}
	}
	if done != 1 {
		t.Fatalf("X 应作为已完成进入 timeline 一次，实际 %d", done)
	}
	if len(p.Skipped) != 1 {
		t.Fatalf("划掉的应进 skipped：%+v", p.Skipped)
	}
}

func TestEntryIDStable(t *testing.T) {
	a := Item{Date: "2026-10-01", File: "2026-10/a.md", Line: 7, Section: SectionDone, Title: "A"}
	b := a
	if EntryID(a) != EntryID(b) {
		t.Fatal("同一行同一条应得到同一个 id")
	}
	b.Line = 8
	if EntryID(a) == EntryID(b) {
		t.Fatal("不同行不应撞 id")
	}
	if CaseID(a) != "report:2026-10-01" {
		t.Fatalf("没有项目时 case = %q", CaseID(a))
	}
	b.Project = "主线"
	if CaseID(b) != "report:2026-10-01#主线" {
		t.Fatalf("有项目时 case = %q", CaseID(b))
	}
}

func TestParseUnreadableFileReportsWarning(t *testing.T) {
	r := Parse("notes.md", []byte("hello\n"))
	if len(r.Warnings) == 0 {
		t.Fatal("读不出日期应给出 warning，而不是静默跳过")
	}
}

const subHeadingReport = `---
created: 2026-09-03
---
## 遗留待办

### 🔴 明日优先：诊断

- **现象**：窗口无限弹窗。
- **已确诊根因链**：A → B → C。

---

- 拓扑选错版本 standalone 报告（待拆）。
`

func TestSubHeadingBulletsAreNotTodos(t *testing.T) {
	r := Parse("2026-09/2026-09-03.md", []byte(subHeadingReport))
	if len(r.Items) != 1 {
		t.Fatalf("只该留下分隔线后的顶层待办，实际 %d 条：%+v", len(r.Items), r.Items)
	}
	if !strings.HasPrefix(r.Items[0].Title, "拓扑选错版本 standalone 报告") || r.Items[0].State != StateOpen {
		t.Fatalf("顶层待办解析不对：%+v", r.Items[0])
	}
}
