package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"anc/internal/hot"
	"anc/internal/org"
)

// anc agent —— 业务 agent 的心跳与在线态。
//
// 它不是「启动 / 停止 agent」：ANC 不接管 harness 的执行策略。这里只做两件事：
// 写一次「我还活着、我在做什么」，以及读回「最后见到它是什么时候」。
// 看板上的在线判定与这条 CLI 用的是同一个阈值（--agent-stale / ANC_AGENT_STALE）。
const agentUsage = `anc agent —— 业务 agent 的心跳与在线态

用法：
  anc agent beat <vault> <slug> [--status running] [--task <id>] [--note <一句话>]
  anc agent ls <vault> [--agent-stale 2m] [--json]

配置（与看板 / 接入面共用）：
  --addr          热层地址；默认 127.0.0.1:6379，也可用 ANC_HOT_ADDR
  --prefix        key 前缀；默认 anc，也可用 ANC_HOT_PREFIX
  --agent-stale   多久没心跳不算在线；默认 2m，也可用 ANC_AGENT_STALE

口径：
  · 心跳身份必须已经在 agents.md 登记 —— 查无此人的心跳不收，免得看板点亮幽灵。
  · 心跳过期不删：只打「不新鲜」标记，最后心跳时间仍留着。
  · 「在线」与「最近调用」是两件事：后者来自审计历史，不能冒充此刻在线。
`

func cmdAgent(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, agentUsage)
		return 2
	}
	switch args[0] {
	case "beat":
		return cmdAgentBeat(args[1:])
	case "ls", "list":
		return cmdAgentLs(args[1:])
	}
	fmt.Fprintf(os.Stderr, "错误：不认识子命令 %q\n\n", args[0])
	fmt.Fprint(os.Stderr, agentUsage)
	return 2
}

type agentOpts struct {
	addr   *string
	prefix *string
	stale  *time.Duration
}

func addAgentOpts(fs *flag.FlagSet) *agentOpts {
	return &agentOpts{
		addr:   fs.String("addr", "", "热层地址（默认 "+hot.DefaultAddr+"；也可用 ANC_HOT_ADDR）"),
		prefix: fs.String("prefix", "", "key 前缀（默认 "+hot.DefaultPrefix+"；也可用 ANC_HOT_PREFIX）"),
		stale:  fs.Duration("agent-stale", 0, "多久没心跳不算在线（默认 "+hot.DefaultAgentStale.String()+"；也可用 ANC_AGENT_STALE）"),
	}
}

func (o *agentOpts) open() (*hot.Store, error) {
	return hot.Open(hot.Config{
		Addr:       firstNonEmpty(*o.addr, os.Getenv("ANC_HOT_ADDR"), hot.DefaultAddr),
		Prefix:     firstNonEmpty(*o.prefix, os.Getenv("ANC_HOT_PREFIX")),
		AgentStale: firstDuration(*o.stale, envDuration("ANC_AGENT_STALE")),
	})
}

func cmdAgentBeat(args []string) int {
	fs := flag.NewFlagSet("agent beat", flag.ContinueOnError)
	o := addAgentOpts(fs)
	status := fs.String("status", "", "自报状态（running / idle / blocked / failed；认不出的照收）")
	task := fs.String("task", "", "当前在做的 task id")
	note := fs.String("note", "", "一句话说明")
	flagArgs, posArgs := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) != 2 {
		fmt.Fprint(os.Stderr, agentUsage)
		return 2
	}
	vault, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	who := strings.TrimSpace(posArgs[1])
	if who == "" {
		fmt.Fprintln(os.Stderr, "错误：需要一个 agents.md 里的 slug 或 name")
		return 2
	}
	oa, err := org.Load(vault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "真相源加载失败，认不出这个 agent：%v\n（先修 org：anc org check %s）\n", err, vault)
		return 1
	}
	a, why := findAgent(oa, who)
	if why != "" {
		fmt.Fprintln(os.Stderr, "错误："+why)
		return 1
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()
	h, err := st.BeatAgent(hot.AgentHeartbeat{
		Slug: a.Slug, Status: strings.TrimSpace(*status),
		TaskID: strings.TrimSpace(*task), Note: strings.TrimSpace(*note),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "心跳没记上：%v\n", err)
		return 1
	}
	out := map[string]any{
		"schema": "anc.agent-heartbeat/v1", "ok": true,
		"slug": h.Slug, "name": a.Name, "status": h.Status,
		"task_id": h.TaskID, "note": h.Note, "as_of": h.AsOf,
		"stale_after_secs": int64(st.AgentStale() / time.Second),
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
	return 0
}

func cmdAgentLs(args []string) int {
	fs := flag.NewFlagSet("agent ls", flag.ContinueOnError)
	o := addAgentOpts(fs)
	asJSON := fs.Bool("json", false, "出 JSON（默认人读）")
	flagArgs, posArgs := splitArgs(args, map[string]bool{"json": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) != 1 {
		fmt.Fprint(os.Stderr, agentUsage)
		return 2
	}
	vault, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	oa, err := org.Load(vault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "真相源加载失败，出不了注册表：%v\n", err)
		return 1
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()
	beats, err := st.ListAgents()
	if err != nil {
		fmt.Fprintf(os.Stderr, "心跳读不了：%v\n", err)
		return 1
	}
	bySlug := make(map[string]hot.AgentHeartbeat, len(beats))
	for _, h := range beats {
		bySlug[h.Slug] = h
	}
	now := st.Now()
	threshold := st.AgentStale()

	type row struct {
		Slug     string `json:"slug"`
		Name     string `json:"name"`
		Online   bool   `json:"online"`
		Stale    bool   `json:"stale"`
		Status   string `json:"status,omitempty"`
		TaskID   string `json:"task_id,omitempty"`
		Note     string `json:"note,omitempty"`
		LastSeen string `json:"last_seen,omitempty"`
		AgeSecs  *int64 `json:"age_secs,omitempty"`
	}
	rows := make([]row, 0, len(oa.Agents))
	for _, a := range oa.Agents {
		r := row{Slug: a.Slug, Name: a.Name}
		if h, ok := bySlug[a.Slug]; ok {
			r.LastSeen, r.Status, r.TaskID, r.Note = h.AsOf, h.Status, h.TaskID, h.Note
			if age, has := h.Age(now); has {
				secs := int64(age / time.Second)
				r.AgeSecs = &secs
				r.Stale = h.Stale(now, threshold)
				r.Online = !r.Stale
			} else {
				r.Stale = true
			}
		}
		rows = append(rows, r)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Slug < rows[j].Slug })
	if *asJSON {
		out := map[string]any{
			"schema": "anc.agents/v1", "stale_after_secs": int64(threshold / time.Second),
			"total": len(rows), "entries": rows,
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return 0
	}
	fmt.Printf("业务 Agent 心跳（%s 没心跳不算在线）\n\n", threshold)
	online := 0
	for _, r := range rows {
		state := "从未心跳"
		if r.LastSeen != "" {
			if r.AgeSecs == nil {
				state = "时间读不懂"
			} else if r.Online {
				state = "在线"
				online++
			} else {
				state = "离线（" + sinceTextCLI(time.Duration(*r.AgeSecs)*time.Second) + "前）"
			}
		}
		extra := ""
		if r.Status != "" {
			extra += " · " + r.Status
		}
		if r.TaskID != "" {
			extra += " · task " + r.TaskID
		}
		if r.Note != "" {
			extra += " · " + r.Note
		}
		fmt.Printf("  %-16s %-12s %-20s%s\n", r.Slug, r.Name, state, extra)
	}
	fmt.Printf("\n在线 %d / 共 %d\n", online, len(rows))
	return 0
}

// sinceTextCLI 是 CLI 的短时长写法：分钟级足够，不引前端那一套。
func sinceTextCLI(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d 秒", int(d/time.Second))
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟", int(d/time.Minute))
	default:
		return fmt.Sprintf("%.1f 小时", d.Hours())
	}
}
