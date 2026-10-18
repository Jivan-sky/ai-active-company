package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"anc/internal/hot"
	"anc/internal/org"
	"anc/internal/timeline"
)

// anc task —— 拉取式任务队列。ANC 只入队；agent 用 MCP 的 anc_task pull 自己拉。
const taskUsage = `anc task —— 拉取式任务队列（ANC 只入队，agent 自己 pull）

用法：
  anc task add <vault> --to <agent> --title <一句话> [--id <id>] [--by <谁>]
                       [--note <正文>] [--domain <域>] [--project <项目>]
                       [--session <会话>] [--intent queue|interrupt|new_session]
  anc task pull <vault> --who <agent> [--limit 1] [--lease 10m] [--json]
  anc task ack <vault> <id> --who <agent> --status done [--note <回执>]
  anc task settle <vault> <id> [--by <谁>] [--status <终态>] [--title <一句话>]
                                [--note <结账说明>] [--kind execution] [--refs a,b]
  anc task ls <vault> [--who <agent>] [--status queued|running|done] [--limit 50] [--json]

配置：
  --addr / --prefix   热层地址与 key 前缀（也可用 ANC_HOT_ADDR / ANC_HOT_PREFIX）
  --lease             pull 之后的认领租约；默认 10m，也可用 ANC_TASK_LEASE

口径：
  · **ANC 不 push、不杀会话**：入队只是把事写进队列；agent 下次 pull 才看到。
  · intent=interrupt 是**请求**，是否打断当前回合由 agent 自己决定。
  · pull 原子认领（SET NX）；两个 agent 同时拉，同一条只落到一个手里。
  · 回执只有被派的人能写；done 先留在热层，等 settle 那一步再清走。
  · settle = 先落库、再清热层；queued / running 默认不结账（要终止就显式给 status）。
  · settle 幂等：timeline 里已有同 case 的行就不再追加。
`

func cmdTask(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, taskUsage)
		return 2
	}
	switch args[0] {
	case "add":
		return cmdTaskAdd(args[1:])
	case "pull":
		return cmdTaskPull(args[1:])
	case "ack":
		return cmdTaskAck(args[1:])
	case "settle":
		return cmdTaskSettle(args[1:])
	case "ls", "list":
		return cmdTaskLs(args[1:])
	}
	fmt.Fprintf(os.Stderr, "错误：不认识子命令 %q\n\n", args[0])
	fmt.Fprint(os.Stderr, taskUsage)
	return 2
}

type taskOpts struct {
	addr   *string
	prefix *string
	lease  *time.Duration
}

func addTaskOpts(fs *flag.FlagSet) *taskOpts {
	return &taskOpts{
		addr:   fs.String("addr", "", "热层地址（默认 "+hot.DefaultAddr+"；也可用 ANC_HOT_ADDR）"),
		prefix: fs.String("prefix", "", "key 前缀（默认 "+hot.DefaultPrefix+"；也可用 ANC_HOT_PREFIX）"),
		lease:  fs.Duration("lease", 0, "pull 之后的认领租约（默认 "+hot.DefaultTaskLease.String()+"；也可用 ANC_TASK_LEASE）"),
	}
}

func (o *taskOpts) open() (*hot.Store, error) {
	return hot.Open(hot.Config{
		Addr:   firstNonEmpty(*o.addr, os.Getenv("ANC_HOT_ADDR"), hot.DefaultAddr),
		Prefix: firstNonEmpty(*o.prefix, os.Getenv("ANC_HOT_PREFIX")),
	})
}

func (o *taskOpts) leaseOf() time.Duration {
	return firstDuration(*o.lease, envDuration("ANC_TASK_LEASE"), hot.DefaultTaskLease)
}

func cmdTaskAdd(args []string) int {
	fs := flag.NewFlagSet("task add", flag.ContinueOnError)
	o := addTaskOpts(fs)
	id := fs.String("id", "", "任务 id（默认自动生成）")
	to := fs.String("to", "", "交给哪个业务 agent（slug 或 name）")
	by := fs.String("by", "anc", "谁派的")
	title := fs.String("title", "", "一句话任务")
	note := fs.String("note", "", "正文")
	domain := fs.String("domain", "", "业务域 slug")
	project := fs.String("project", "", "项目 slug")
	session := fs.String("session", "", "会话 id（可选）")
	intent := fs.String("intent", "queue", "queue / interrupt / new_session")
	flagArgs, posArgs := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) != 1 || strings.TrimSpace(*to) == "" || strings.TrimSpace(*title) == "" {
		fmt.Fprint(os.Stderr, taskUsage)
		return 2
	}
	vault, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	oa, err := org.Load(vault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "真相源加载失败：%v\n", err)
		return 1
	}
	a, why := findAgent(oa, *to)
	if why != "" {
		fmt.Fprintln(os.Stderr, "错误："+why)
		return 1
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()
	tid := strings.TrimSpace(*id)
	if tid == "" {
		tid = fmt.Sprintf("t-%d", time.Now().UnixNano())
	}
	saved, err := st.EnqueueTask(hot.TaskSpec{
		ID: tid, To: a.Slug, By: *by, Title: *title, Note: *note,
		Domain: *domain, Project: *project, Session: *session, Intent: *intent,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "任务没入队：%v\n", err)
		return 1
	}
	printJSON(map[string]any{"ok": true, "task": taskMap(saved, ""), "note": "已入队；agent 下次 pull 才会看到。"})
	return 0
}

func cmdTaskPull(args []string) int {
	fs := flag.NewFlagSet("task pull", flag.ContinueOnError)
	o := addTaskOpts(fs)
	who := fs.String("who", "", "拉取者（业务 agent slug 或 name）")
	limit := fs.Int("limit", 1, "最多拉几条")
	asJSON := fs.Bool("json", false, "出 JSON（默认人读）")
	flagArgs, posArgs := splitArgs(args, map[string]bool{"json": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) != 1 || strings.TrimSpace(*who) == "" {
		fmt.Fprint(os.Stderr, taskUsage)
		return 2
	}
	vault, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	oa, err := org.Load(vault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "真相源加载失败：%v\n", err)
		return 1
	}
	a, why := findAgent(oa, *who)
	if why != "" {
		fmt.Fprintln(os.Stderr, "错误："+why)
		return 1
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()
	current := ""
	if h, ok, herr := st.GetAgent(a.Slug); herr == nil && ok {
		current = h.Session
	}
	tasks, err := st.PullTasks(a.Slug, *limit, o.leaseOf(), current)
	if err != nil {
		fmt.Fprintf(os.Stderr, "任务拉不了：%v\n", err)
		return 1
	}
	if *asJSON {
		out := make([]map[string]any, 0, len(tasks))
		for _, t := range tasks {
			out = append(out, taskMap(t, ""))
		}
		printJSON(map[string]any{"ok": true, "who": a.Slug, "count": len(out), "tasks": out})
		return 0
	}
	if len(tasks) == 0 {
		fmt.Println("没有交给你、还没被拉走的任务。")
		return 0
	}
	fmt.Printf("%s 拉走 %d 条：\n", a.Slug, len(tasks))
	for _, t := range tasks {
		fmt.Printf("  %-20s %s  v%d\n", t.ID, t.Title, t.Version)
		if t.Note != "" {
			fmt.Printf("    %s\n", t.Note)
		}
	}
	return 0
}

func cmdTaskAck(args []string) int {
	fs := flag.NewFlagSet("task ack", flag.ContinueOnError)
	o := addTaskOpts(fs)
	who := fs.String("who", "", "回执者（必须是被派的 agent）")
	status := fs.String("status", "", "done / blocked / failed / cancelled …（认不出的照收）")
	note := fs.String("note", "", "回执说明")
	flagArgs, posArgs := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) != 2 || strings.TrimSpace(*who) == "" || strings.TrimSpace(*status) == "" {
		fmt.Fprint(os.Stderr, taskUsage)
		return 2
	}
	vault, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	oa, err := org.Load(vault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "真相源加载失败：%v\n", err)
		return 1
	}
	a, why := findAgent(oa, *who)
	if why != "" {
		fmt.Fprintln(os.Stderr, "错误："+why)
		return 1
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()
	saved, err := st.AckTask(posArgs[1], a.Slug, *status, *note)
	if err != nil {
		fmt.Fprintf(os.Stderr, "回执没记上：%v\n", err)
		return 1
	}
	printJSON(map[string]any{"ok": true, "task": taskMap(saved, "")})
	return 0
}

func cmdTaskLs(args []string) int {
	fs := flag.NewFlagSet("task ls", flag.ContinueOnError)
	o := addTaskOpts(fs)
	who := fs.String("who", "", "只看这个 agent 的任务")
	status := fs.String("status", "", "只看这个状态")
	limit := fs.Int("limit", 50, "最多几条")
	asJSON := fs.Bool("json", false, "出 JSON（默认人读）")
	flagArgs, posArgs := splitArgs(args, map[string]bool{"json": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) != 1 {
		fmt.Fprint(os.Stderr, taskUsage)
		return 2
	}
	vault, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	oa, err := org.Load(vault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "真相源加载失败：%v\n", err)
		return 1
	}
	want := strings.TrimSpace(*who)
	if want != "" {
		a, why := findAgent(oa, want)
		if why != "" {
			fmt.Fprintln(os.Stderr, "错误："+why)
			return 1
		}
		want = a.Slug
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()
	list, err := st.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "任务读不了：%v\n", err)
		return 1
	}
	out := make([]hot.State, 0, len(list))
	for _, t := range list {
		if len(out) >= *limit {
			break
		}
		if want != "" && t.To != want && t.By != want && t.From != want {
			continue
		}
		if strings.TrimSpace(*status) != "" && t.Status != *status {
			continue
		}
		out = append(out, t)
	}
	if *asJSON {
		rows := make([]map[string]any, 0, len(out))
		for _, t := range out {
			rows = append(rows, taskMap(t, ""))
		}
		printJSON(map[string]any{"ok": true, "count": len(rows), "tasks": rows})
		return 0
	}
	if len(out) == 0 {
		fmt.Println("没有符合条件的任务。")
		return 0
	}
	for _, t := range out {
		fmt.Printf("  %-20s %-9s v%-3d →%-14s %s\n", t.ID, t.Status, t.Version, t.To, t.Title)
	}
	return 0
}

func cmdTaskSettle(args []string) int {
	fs := flag.NewFlagSet("task settle", flag.ContinueOnError)
	o := addTaskOpts(fs)
	by := fs.String("by", "", "谁交的（默认 = 任务的执行者）")
	status := fs.String("status", "", "这次结账的终态；不给 = 用热层里的当前态")
	title := fs.String("title", "", "一句话（默认用任务标题）")
	note := fs.String("note", "", "结账说明")
	kind := fs.String("kind", "execution", "decision / execution / note")
	refs := fs.String("refs", "", "引用，逗号分隔")
	flagArgs, pos := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) != 2 {
		fmt.Fprint(os.Stderr, taskUsage)
		return 2
	}
	abs, err := filepath.Abs(pos[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()

	var refList []string
	for _, r := range strings.Split(*refs, ",") {
		if s := strings.TrimSpace(r); s != "" {
			refList = append(refList, s)
		}
	}
	res, err := settleTask(st, abs, pos[1], taskSettleOpts{
		By: *by, Title: *title, Status: *status, Kind: *kind, Note: *note, Refs: refList,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "结账没成：%v\n", err)
		return 1
	}
	if res.Already {
		fmt.Println("anc task settle —— 已经在 timeline 里，本次不重复写")
		fmt.Printf("  case %s · status=%s · at=%s\n", res.Entry.Case, dashIfEmpty(res.Entry.Status), dashIfEmpty(res.Entry.At))
		if res.Task.ID != "" {
			fmt.Printf("  热层     %s 已清走\n", res.Task.ID)
		}
		return 0
	}
	fmt.Println("anc task settle —— 先落库，再清热层")
	fmt.Printf("  %s %s\n", bandDot(timeline.Band(res.Entry.Status)), titleLine(res.Entry))
	fmt.Printf("  落库     %s\n", relTo(abs, res.Path))
	fmt.Printf("  这一条   id=%s kind=%s status=%s by=%s\n",
		res.Entry.ID, dashIfEmpty(res.Entry.Kind), dashIfEmpty(res.Entry.Status), dashIfEmpty(res.Entry.By))
	fmt.Printf("  热层     %s 已清走（进行中 → 历史）\n", res.Task.ID)
	return 0
}

func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}
