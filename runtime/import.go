package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"anc/internal/hot"
	"anc/internal/reportimport"
	"anc/internal/timeline"
)

// anc import —— 把外部材料里的**事实**搬进 ANC。
//
// 这里只做日报：日报是已经在发生的记录，不是「让模型再想一遍」的原料。
// 不写 vault 的 markdown（真相源仍归人 / ingest），只写两处：
//
//	· timeline/  —— 完成与决策，作为证据留痕；
//	· 热层任务队列 —— 最后一版仍是 [ ] 的待办，交给指定 agent 自己 pull。
const importUsage = `anc import —— 把外部材料里的既成事实搬进 ANC（只导入，不改执行策略）

用法：
  anc import reports <vault> <日报目录> [--to <agent>] [--since YYYY-MM-DD]
                                      [--by <谁>] [--limit N]
                                      [--no-timeline] [--no-tasks]
                                      [--apply] [--json]

默认是 **dry-run**：只统计与列出，不写 timeline、不入队。加 --apply 才落。

读什么（按日报自己的结构，不猜）：
  · 「完成事项」/「关键决策」章节的 bullet → timeline（kind=execution / decision）
  · 「遗留待办」/「今日待办」的 checkbox 与表格 → 待办
  · 待办按「项目 + 标题」归并，以**最后一次出现**为准：
    最后一次仍是 [ ] 才入队；后面已 [x] 的不再派单；~~划线~~ 直接跳过

落点：
  · timeline/<YYYY-MM>.<by>.jsonl —— append-only；稳定 id，重导不会重复
  · 热层任务队列 —— id 同样稳定；已存在的任务不覆盖

选项：
  --to <agent>       待办入队给谁（业务 agent slug 或成员名）；不给就只列出、不入队
  --since YYYY-MM-DD 只看这一天及之后的日报（含）
  --by <谁>          timeline 的署名；必须是能当文件名的英文/数字（默认 anc-import）
  --limit N          这一轮最多入队多少条（0 = 不限）
  --no-timeline      只导任务
  --no-tasks         只导 timeline
  --apply            真写；不加就是 dry-run
  --json             机器读的出口

口径：
  · 不根据日报**自动关闭**任何任务：日报说完成了，只进 timeline；
    任务状态仍由 agent 回执 / 人验收决定。ANC 不替执行者改状态。
  · 同一条待办今天 [ ]、明天 [x]，重导只留「已完成」这一条 timeline，
    不会把昨天那条未完成的再派一次。
  · 解析不了的文件逐份列出来（没有可识别章节 / 读不出日期），不静默跳过。
`

// importQueue 只取导入需要的两个动作 —— 不把整条任务协议塞进来。
type importQueue interface {
	EnqueueTask(hot.TaskSpec) (hot.State, error)
	List() ([]hot.State, error)
}

// openImportQueue 是测试注入口：单测不需要真 Redis。
var openImportQueue = func(o *taskOpts) (importQueue, error) { return o.open() }

type importOut struct {
	Vault   string `json:"vault"`
	Dir     string `json:"dir"`
	Since   string `json:"since,omitempty"`
	Apply   bool   `json:"apply"`
	To      string `json:"to,omitempty"`
	Files   int    `json:"files"`
	NoShape int    `json:"files_without_shape"`
	Items   int    `json:"items"`

	Entries        int `json:"entries"`
	EntriesWritten int `json:"entries_written"`
	EntriesExist   int `json:"entries_existing"`
	Superseded     int `json:"superseded_todos"`

	Tasks        int `json:"tasks"`
	TasksWritten int `json:"tasks_written"`
	TasksExist   int `json:"tasks_existing"`
	TasksLeft    int `json:"tasks_left"`

	Warnings []string                `json:"warnings,omitempty"`
	FilesOut []reportimport.FileStat `json:"file_stats,omitempty"`
	Sample   []reportimport.Item     `json:"sample,omitempty"`
	TaskList []reportimport.Item     `json:"task_list,omitempty"`
	SkipList []reportimport.Item     `json:"skipped,omitempty"`
}

func cmdImport(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, importUsage)
		return 2
	}
	switch args[0] {
	case "reports":
		return cmdImportReports(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "不认识的导入类型：%s\n\n", args[0])
		fmt.Fprint(os.Stderr, importUsage)
		return 2
	}
}

func cmdImportReports(args []string) int {
	fsx := flag.NewFlagSet("import reports", flag.ContinueOnError)
	o := addTaskOpts(fsx)
	to := fsx.String("to", "", "待办入队给谁（不给就只列出）")
	since := fsx.String("since", "", "只看这一天及之后的日报（YYYY-MM-DD）")
	by := fsx.String("by", "anc-import", "timeline 署名")
	limit := fsx.Int("limit", 0, "这一轮最多入队多少条（0 = 不限）")
	noTimeline := fsx.Bool("no-timeline", false, "只导任务")
	noTasks := fsx.Bool("no-tasks", false, "只导 timeline")
	apply := fsx.Bool("apply", false, "真写；不加就是 dry-run")
	asJSON := fsx.Bool("json", false, "出 JSON")

	flagArgs, pos := splitArgs(args, map[string]bool{
		"apply": true, "json": true, "no-timeline": true, "no-tasks": true,
	})
	if err := fsx.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) != 2 {
		fmt.Fprint(os.Stderr, importUsage)
		return 2
	}
	if *since != "" {
		if _, err := time.Parse("2006-01-02", *since); err != nil {
			fmt.Fprintf(os.Stderr, "错误：--since 要 YYYY-MM-DD（收到 %q）\n", *since)
			return 2
		}
	}

	vault, err := filepath.Abs(pos[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	dir, err := filepath.Abs(pos[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		fmt.Fprintf(os.Stderr, "错误：日报目录读不了：%s\n", dir)
		return 2
	}

	reports, other, err := readReports(dir, *since)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	plan := reportimport.Build(reports)

	out := importOut{
		Vault: vault, Dir: dir, Since: *since, Apply: *apply, To: *to,
		Files: len(reports), Items: 0,
		Entries: len(plan.Entries), Superseded: plan.Superseded,
		Tasks: len(plan.Tasks), FilesOut: plan.Files, Warnings: plan.Warnings,
	}
	for _, r := range reports {
		out.Items += len(r.Items)
		if len(r.Items) == 0 {
			out.NoShape++
		}
	}
	if other > 0 {
		out.Warnings = append(out.Warnings, fmt.Sprintf("跳过 %d 个不是 YYYY-MM-DD.md 的文件（索引等）", other))
	}

	// 只列前 8 条，避免把 400 条全打到终端。
	out.Sample = head(plan.Entries, 8)
	out.TaskList = plan.Tasks
	out.SkipList = head(plan.Skipped, 5)

	notes := []string{}
	if *apply {
		if !*noTimeline {
			exist, err := existingTimelineIDs(vault)
			if err != nil {
				fmt.Fprintf(os.Stderr, "错误：timeline 读不了：%v\n", err)
				return 1
			}
			for _, it := range plan.Entries {
				id := reportimport.EntryID(it)
				if exist[id] {
					out.EntriesExist++
					continue
				}
				if err := appendReportEntry(vault, *by, it); err != nil {
					fmt.Fprintf(os.Stderr, "错误：timeline 写不了：%v\n", err)
					return 1
				}
				exist[id] = true
				out.EntriesWritten++
			}
		}

		if !*noTasks && len(plan.Tasks) > 0 {
			if strings.TrimSpace(*to) == "" {
				notes = append(notes, fmt.Sprintf("没有 --to：%d 条待办只列出，没有入队", len(plan.Tasks)))
			} else {
				q, err := openImportQueue(o)
				if err != nil {
					fmt.Fprintf(os.Stderr, "错误：热层连不上：%v\n", err)
					return 1
				}
				exist, err := existingTaskIDs(q)
				if err != nil {
					fmt.Fprintf(os.Stderr, "错误：任务读不了：%v\n", err)
					return 1
				}
				for _, it := range plan.Tasks {
					if *limit > 0 && out.TasksWritten >= *limit {
						out.TasksLeft = len(plan.Tasks) - out.TasksWritten - out.TasksExist
						break
					}
					id := reportimport.EntryID(it)
					if exist[id] {
						out.TasksExist++
						continue
					}
					spec := hot.TaskSpec{
						ID: id, To: *to, By: *by, Title: it.Title,
						Note: taskNote(it), Project: it.Project,
						Priority: it.Priority, Root: reportimport.CaseID(it),
						Refs: []string{it.File},
					}
					if _, err := q.EnqueueTask(spec); err != nil {
						fmt.Fprintf(os.Stderr, "错误：任务入队失败（%s）：%v\n", id, err)
						return 1
					}
					exist[id] = true
					out.TasksWritten++
				}
			}
		}
	}

	out.Warnings = append(out.Warnings, notes...)
	if *asJSON {
		printJSON(out)
	} else {
		printImport(out)
	}
	return 0
}

func readReports(dir, since string) ([]reportimport.Report, int, error) {
	var out []reportimport.Report
	other := 0
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			return nil
		}
		if !isReportName(name) {
			other++
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			rel = name
		}
		r := reportimport.Parse(filepath.ToSlash(rel), raw)
		if since != "" && (r.Date == "" || r.Date < since) {
			return nil
		}
		out = append(out, r)
		return nil
	})
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].Name < out[j].Name
	})
	return out, other, err
}

func isReportName(name string) bool {
	if len(name) != len("2006-01-02.md") {
		return false
	}
	for i, r := range "2006-01-02" {
		switch r {
		case '-':
			if name[i] != '-' {
				return false
			}
		default:
			if name[i] < '0' || name[i] > '9' {
				return false
			}
		}
	}
	return strings.HasSuffix(name, ".md")
}

func existingTimelineIDs(vault string) (map[string]bool, error) {
	doc, err := timeline.Load(vault)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]bool, len(doc.Entries))
	for _, e := range doc.Entries {
		ids[e.ID] = true
	}
	return ids, nil
}

func existingTaskIDs(q importQueue) (map[string]bool, error) {
	list, err := q.List()
	if err != nil {
		return nil, err
	}
	ids := make(map[string]bool, len(list))
	for _, s := range list {
		ids[s.ID] = true
	}
	return ids, nil
}

func appendReportEntry(vault, by string, it reportimport.Item) error {
	_, err := timeline.Append(vault, timeline.Entry{
		ID: reportimport.EntryID(it), Case: reportimport.CaseID(it),
		At: atForDate(it.Date), Kind: reportimport.Kind(it), Status: "done",
		By: by, Project: it.Project, Title: it.Title, Detail: it.Detail,
		Refs: []string{it.File},
	})
	return err
}

// atForDate 把日报的日期放成当天 23:59（本地时区）—— 日报记的是这一天结束时的样子。
func atForDate(date string) string {
	t, err := time.ParseInLocation("2006-01-02 15:04", date+" 23:59", time.Local)
	if err != nil {
		return date + "T00:00:00Z"
	}
	return t.Format(time.RFC3339)
}

func taskNote(it reportimport.Item) string {
	n := strings.TrimSpace(it.Detail)
	src := "来源：" + it.File
	if n == "" {
		return src
	}
	return n + "\n" + src
}

func head(items []reportimport.Item, n int) []reportimport.Item {
	if len(items) <= n {
		return items
	}
	return items[:n]
}

func printImport(o importOut) {
	mode := "只读预览（加 --apply 才落盘）"
	if o.Apply {
		mode = "已落盘"
	}
	fmt.Printf("anc import reports —— %s\n", mode)
	fmt.Printf("  vault    %s\n", o.Vault)
	fmt.Printf("  日报     %s\n", o.Dir)
	if o.Since != "" {
		fmt.Printf("  起算     %s（含）\n", o.Since)
	}
	fmt.Printf("  读到     %d 份日报，%d 条记录；其中 %d 份没有可识别章节\n",
		o.Files, o.Items, o.NoShape)
	fmt.Printf("  timeline %d 条（已存在 %d，写入 %d）；被后文覆盖/收口的待办 %d 条\n",
		o.Entries, o.EntriesExist, o.EntriesWritten, o.Superseded)
	if o.Tasks == 0 {
		fmt.Println("  任务     没有待入队的待办")
	} else if strings.TrimSpace(o.To) == "" {
		fmt.Printf("  任务     %d 条待办只列出（没有 --to）\n", o.Tasks)
	} else {
		fmt.Printf("  任务     %d 条 → %s（已存在 %d，写入 %d，留待下一轮 %d）\n",
			o.Tasks, o.To, o.TasksExist, o.TasksWritten, o.TasksLeft)
	}

	if len(o.Sample) > 0 && !o.Apply {
		fmt.Println("  预览（前几条 timeline）：")
		for _, it := range o.Sample {
			fmt.Printf("    [%s] %s  %s\n", it.Date, it.Section, it.Title)
		}
	}
	if len(o.TaskList) > 0 && strings.TrimSpace(o.To) == "" {
		fmt.Println("  待办（前几条）：")
		for i, it := range o.TaskList {
			if i >= 5 {
				fmt.Printf("    …… 还有 %d 条\n", len(o.TaskList)-i)
				break
			}
			fmt.Printf("    [%s] %s\n", it.Date, it.Title)
		}
	}
	if len(o.Warnings) > 0 {
		fmt.Println("  ⚠️  提醒：")
		for i, w := range o.Warnings {
			if i >= 8 {
				fmt.Printf("    …… 还有 %d 条\n", len(o.Warnings)-i)
				break
			}
			fmt.Printf("    %s\n", w)
		}
	}
	fmt.Println("  口径：只导事实与待办；不自动关闭任务，不改 agent 的执行策略。")
}
