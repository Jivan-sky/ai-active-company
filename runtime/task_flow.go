package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"anc/internal/audit"
	"anc/internal/org"
)

// taskActor 是任务链路上的一个身份：业务 agent 或成员。
// ID 用登记表里的规范名 —— 交接与验收都按它比对，避免 name / slug 混着写。
type taskActor struct {
	ID      string
	Domain  string
	IsAgent bool
}

// resolveTaskActor 先认业务 agent，再认成员。认不出就回一句「表里有什么」。
func resolveTaskActor(oa *org.Org, who string) (taskActor, string) {
	who = strings.TrimSpace(who)
	if who == "" {
		return taskActor{}, "身份不能为空"
	}
	if a, why := findAgent(oa, who); why == "" {
		return taskActor{ID: a.Slug, Domain: a.Domain, IsAgent: true}, ""
	}
	for _, m := range oa.Members {
		if m.Name == who || (m.DisplayName != "" && m.DisplayName == who) {
			d := ""
			if len(m.Domains) > 0 {
				d = m.Domains[0]
			}
			return taskActor{ID: m.Name, Domain: d}, ""
		}
	}
	_, why := findAgent(oa, who)
	return taskActor{}, why + "（成员表里也没有这个人）"
}

// taskCrossDomain 只回答「能不能证明同域」。证明不了按跨域告警 ——
// v1 不拒，但看板与审计要看得见。
func taskCrossDomain(from, to taskActor) bool {
	if strings.TrimSpace(from.Domain) == "" || strings.TrimSpace(to.Domain) == "" {
		return true
	}
	return from.Domain != to.Domain
}

// taskAudit 是交接 / 升级的统一留痕口。CLI 与 MCP 都走这里，判断只维护一份。
func taskAudit(vault, actor, tool, object string, result string, detail string) {
	now := time.Now()
	rec := audit.Record{
		ID: fmt.Sprintf("%s-%d-%s", tool, now.UnixNano(), audit.ShardName(actor)),
		At: now.Format(time.RFC3339), Actor: actor, Action: audit.ActionInvoke,
		Object: object, Result: result, Tool: tool, Source: "task", Detail: detail,
	}
	if _, err := audit.Append(vault, rec); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  交接 / 升级没留上痕（audit）：%v\n", err)
	}
}

func cmdTaskHandoff(args []string) int {
	fs := flag.NewFlagSet("task handoff", flag.ContinueOnError)
	o := addTaskOpts(fs)
	who := fs.String("who", "", "当前持有者（业务 agent）")
	to := fs.String("to", "", "下一手（业务 agent）")
	fence := fs.Int64("fence", 0, "pull 返回的围栏号（必填）")
	note := fs.String("note", "", "为什么交接（必填）")
	refs := fs.String("refs", "", "随交接带过去的证据，逗号分隔")
	flagArgs, pos := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) != 2 || strings.TrimSpace(*who) == "" || strings.TrimSpace(*to) == "" || strings.TrimSpace(*note) == "" {
		fmt.Fprint(os.Stderr, taskUsage)
		return 2
	}
	vault, err := filepath.Abs(pos[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	oa, err := org.Load(vault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "真相源加载失败：%v\n", err)
		return 1
	}
	from, why := resolveTaskActor(oa, *who)
	if why != "" {
		fmt.Fprintln(os.Stderr, "错误："+why)
		return 1
	}
	target, why := resolveTaskActor(oa, *to)
	if why != "" {
		fmt.Fprintln(os.Stderr, "错误："+why)
		return 1
	}
	if !from.IsAgent || !target.IsAgent {
		fmt.Fprintln(os.Stderr, "错误：handoff 的两端都必须是 agents.md 里的业务 agent —— 任务只能交给会 pull 的 agent")
		return 1
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()
	cross := taskCrossDomain(from, target)
	saved, err := st.HandoffTask(pos[1], from.ID, *fence, target.ID, *note, splitList(*refs), cross)
	if err != nil {
		fmt.Fprintf(os.Stderr, "交接没成：%v\n", err)
		return 1
	}
	taskAudit(vault, from.ID, "anc_task_handoff", "task:"+pos[1], audit.ResultOK,
		fmt.Sprintf("to=%s hop=%d cross_domain=%t note=%s", target.ID, saved.Hop, cross, *note))
	printJSON(map[string]any{"ok": true, "task": taskMap(saved, ""), "cross_domain": cross})
	return 0
}

func cmdTaskSubmit(args []string) int {
	fs := flag.NewFlagSet("task submit", flag.ContinueOnError)
	o := addTaskOpts(fs)
	who := fs.String("who", "", "提交人（当前持有者）")
	reviewer := fs.String("reviewer", "", "验收人（业务 agent 或成员）")
	accept := fs.String("accept", "", "验收口径（任务已有可省略）")
	fence := fs.Int64("fence", 0, "pull 返回的围栏号（必填）")
	note := fs.String("note", "", "提交说明")
	refs := fs.String("refs", "", "证据，逗号分隔")
	flagArgs, pos := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) != 2 || strings.TrimSpace(*who) == "" || strings.TrimSpace(*reviewer) == "" {
		fmt.Fprint(os.Stderr, taskUsage)
		return 2
	}
	vault, err := filepath.Abs(pos[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	oa, err := org.Load(vault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "真相源加载失败：%v\n", err)
		return 1
	}
	author, why := resolveTaskActor(oa, *who)
	if why != "" {
		fmt.Fprintln(os.Stderr, "错误："+why)
		return 1
	}
	checker, why := resolveTaskActor(oa, *reviewer)
	if why != "" {
		fmt.Fprintln(os.Stderr, "错误："+why)
		return 1
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()
	saved, err := st.SubmitTask(pos[1], author.ID, *fence, checker.ID, *accept, *note, splitList(*refs))
	if err != nil {
		fmt.Fprintf(os.Stderr, "提交没成：%v\n", err)
		return 1
	}
	warn := ""
	if saved.Review.Self {
		warn = "验收人就是提交人 —— 已放行并标记 self，请在看板 / 审计里重点看"
		fmt.Fprintln(os.Stderr, "⚠️  "+warn)
	}
	printJSON(map[string]any{"ok": true, "task": taskMap(saved, ""), "warning": warn})
	return 0
}

func cmdTaskVerify(args []string) int {
	fs := flag.NewFlagSet("task verify", flag.ContinueOnError)
	o := addTaskOpts(fs)
	who := fs.String("who", "", "验收人（必须是提交时写的那一个）")
	verdict := fs.String("verdict", "", "accepted / rejected")
	note := fs.String("note", "", "验收意见（rejected 必填）")
	flagArgs, pos := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) != 2 || strings.TrimSpace(*who) == "" || strings.TrimSpace(*verdict) == "" {
		fmt.Fprint(os.Stderr, taskUsage)
		return 2
	}
	vault, err := filepath.Abs(pos[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	oa, err := org.Load(vault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "真相源加载失败：%v\n", err)
		return 1
	}
	checker, why := resolveTaskActor(oa, *who)
	if why != "" {
		fmt.Fprintln(os.Stderr, "错误："+why)
		return 1
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()
	saved, err := st.VerifyTask(pos[1], checker.ID, *verdict, *note)
	if err != nil {
		fmt.Fprintf(os.Stderr, "验收没成：%v\n", err)
		return 1
	}
	warn := ""
	if saved.Review.Self {
		warn = "验收人就是提交人 —— 已放行并标记 self，不代表独立复核"
	}
	printJSON(map[string]any{"ok": true, "task": taskMap(saved, ""), "warning": warn})
	return 0
}

func cmdTaskEscalate(args []string) int {
	fs := flag.NewFlagSet("task escalate", flag.ContinueOnError)
	o := addTaskOpts(fs)
	who := fs.String("who", "", "谁在升级（持有者 / 验收参与方）")
	to := fs.String("to", "", "卡点交给谁（agent 或成员）")
	fence := fs.Int64("fence", 0, "running 中升级必填：pull 返回的围栏号")
	note := fs.String("note", "", "卡在哪（必填）")
	flagArgs, pos := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) != 2 || strings.TrimSpace(*who) == "" || strings.TrimSpace(*to) == "" || strings.TrimSpace(*note) == "" {
		fmt.Fprint(os.Stderr, taskUsage)
		return 2
	}
	vault, err := filepath.Abs(pos[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	oa, err := org.Load(vault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "真相源加载失败：%v\n", err)
		return 1
	}
	from, why := resolveTaskActor(oa, *who)
	if why != "" {
		fmt.Fprintln(os.Stderr, "错误："+why)
		return 1
	}
	target, why := resolveTaskActor(oa, *to)
	if why != "" {
		fmt.Fprintln(os.Stderr, "错误："+why)
		return 1
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()
	cross := taskCrossDomain(from, target)
	saved, err := st.EscalateTask(pos[1], from.ID, *fence, target.ID, *note)
	if err != nil {
		fmt.Fprintf(os.Stderr, "升级没成：%v\n", err)
		return 1
	}
	taskAudit(vault, from.ID, "anc_task_escalate", "task:"+pos[1], audit.ResultOK,
		fmt.Sprintf("to=%s cross_domain=%t note=%s", target.ID, cross, *note))
	printJSON(map[string]any{"ok": true, "task": taskMap(saved, ""), "cross_domain": cross})
	return 0
}
