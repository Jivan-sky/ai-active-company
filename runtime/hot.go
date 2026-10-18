package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"anc/internal/hot"
	"anc/internal/timeline"
)

// anc hot —— 「进行中」的热层。
//
// 一句话：**状态是「当前值」，不是「历史」。** 当前值放热层（可覆写、可原子认领、
// 可有新鲜度），历史与结论落 timeline（append-only）。
//
// 「达标」的规矩是**先落库、再清热层** —— 反序就会丢东西。
const hotUsage = `anc hot —— 「进行中」的热层（达标落库，进行中放这里）

一句话：状态是「当前值」，不是「历史」。当前值放热层（可覆写、可原子认领、可判新鲜度），
历史与结论落 timeline（append-only）。**先落库、再清热层** —— 反序会丢东西。

用法：
  anc hot ping [--addr 127.0.0.1:6379] [--prefix anc]
  anc hot put <id> [-status <词>] [-by <谁>] [-to <谁>] [-domain <域>] [-project <项目>]
                   [-title <一句话>] [-note <备注>]
  anc hot ls [--stale 24h] [--json]
  anc hot claim <id> -by <谁> [--lease 30m]
  anc hot release <id> [-by <谁>]
  anc hot drop <id>
  anc hot done <vault> <id> -by <谁> [-title <一句话>] [--kind execution] [--status done]

配置（外置，代码里不留死值）：
  --addr    热层地址；默认 127.0.0.1:6379，也可用环境变量 ANC_HOT_ADDR
  --prefix  key 前缀；默认 anc，也可用环境变量 ANC_HOT_PREFIX
  --ttl     状态条目存活时长；0（默认）= 不失效

口径三条：
  · **TTL 不是新鲜度。** TTL 是删数据，只给「授权 / 租约」那种「到点就该失效」的东西用；
    新鲜度靠 as_of + --stale 打标记，**过期不删** —— 一件进行中的事凭空消失就是失真。
  · **已落库的不在这里。** 结论进 timeline，热层只留「还在做」的当前态。
  · **认领是原子的**（SET NX）：两个 agent 同时以为自己在做同一件事，是最贵的错。
`

func cmdHot(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, hotUsage)
		return 2
	}
	switch args[0] {
	case "ping":
		return cmdHotPing(args[1:])
	case "put":
		return cmdHotPut(args[1:])
	case "ls", "status":
		return cmdHotLs(args[1:])
	case "claim":
		return cmdHotClaim(args[1:])
	case "release":
		return cmdHotRelease(args[1:])
	case "drop":
		return cmdHotDrop(args[1:])
	case "done":
		return cmdHotDone(args[1:])
	}
	fmt.Fprintf(os.Stderr, "错误：不认识子命令 %q\n\n", args[0])
	fmt.Fprint(os.Stderr, hotUsage)
	return 2
}

// ---------- 配置：三个口子（flag / 环境变量 / 默认值），别处不许再出现地址与前缀 ----------

type hotOpts struct {
	addr   *string
	prefix *string
	ttl    *time.Duration
}

func addHotOpts(fs *flag.FlagSet) *hotOpts {
	return &hotOpts{
		addr:   fs.String("addr", "", "热层地址（默认 "+hot.DefaultAddr+"；也可用 ANC_HOT_ADDR）"),
		prefix: fs.String("prefix", "", "key 前缀（默认 "+hot.DefaultPrefix+"；也可用 ANC_HOT_PREFIX）"),
		ttl:    fs.Duration("ttl", hot.DefaultTTL, "状态条目存活时长；0 = 不失效"),
	}
}

func (o *hotOpts) shownAddr() string {
	return firstNonEmpty(*o.addr, os.Getenv("ANC_HOT_ADDR"), hot.DefaultAddr)
}

func (o *hotOpts) open() (*hot.Store, error) {
	return hot.Open(hot.Config{
		Addr:   o.shownAddr(),
		Prefix: firstNonEmpty(*o.prefix, os.Getenv("ANC_HOT_PREFIX")),
		TTL:    *o.ttl,
	})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// envDuration 读一个可选的环境变量时长；没设或读不懂都回 0（交给调用方落默认值）。
// 读不懂时不在这里报错：调用方有一处能说清「这个值没生效」，比散落各处的静默兜底强。
func envDuration(name string) time.Duration {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0
	}
	return d
}

// firstDuration 取第一个 > 0 的时长；全都 <= 0 就回 0（交给下层落默认值）。
func firstDuration(vals ...time.Duration) time.Duration {
	for _, d := range vals {
		if d > 0 {
			return d
		}
	}
	return 0
}

// ---------- 子命令 ----------

func cmdHotPing(args []string) int {
	fs := flag.NewFlagSet("hot ping", flag.ContinueOnError)
	o := addHotOpts(fs)
	flagArgs, _ := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()
	fmt.Println("anc hot —— 热层自检")
	fmt.Println("  状态     在（PING -> PONG）")
	fmt.Printf("  地址     %s\n", o.shownAddr())
	fmt.Printf("  前缀     %s\n", st.Prefix())
	fmt.Printf("  TTL      %s\n", ttlLine(st.TTL()))
	return 0
}

func cmdHotPut(args []string) int {
	fs := flag.NewFlagSet("hot put", flag.ContinueOnError)
	o := addHotOpts(fs)
	status := fs.String("status", "", "当前态：running / done / blocked / failed（认不出的词照收）")
	by := fs.String("by", "", "谁在做")
	to := fs.String("to", "", "下一手该交给谁")
	domain := fs.String("domain", "", "业务域")
	project := fs.String("project", "", "项目")
	title := fs.String("title", "", "一句话")
	note := fs.String("note", "", "备注")
	flagArgs, pos := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) == 0 {
		fmt.Fprintln(os.Stderr, "错误：缺少 <id>")
		return 2
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()
	saved, err := st.Put(hot.State{
		ID: pos[0], Status: *status, By: *by, To: *to,
		Domain: *domain, Project: *project, Title: *title, Note: *note,
	})
	if err != nil {
		return hotFail(err)
	}
	fmt.Println("anc hot put —— 记下当前态（覆写，不追加）")
	hotPrintState(saved, time.Now(), 0)
	return 0
}

func cmdHotLs(args []string) int {
	fs := flag.NewFlagSet("hot ls", flag.ContinueOnError)
	o := addHotOpts(fs)
	stale := fs.Duration("stale", hot.DefaultStale, "多久没动就算不新鲜（只画标记，不拦不删）")
	asJSON := fs.Bool("json", false, "打 JSON")
	flagArgs, _ := splitArgs(args, map[string]bool{"json": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()
	list, err := st.List()
	if err != nil {
		return hotFail(err)
	}
	now := time.Now()

	if *asJSON {
		out := map[string]any{
			"schema":    "anc.hot/v1",
			"addr":      o.shownAddr(),
			"prefix":    st.Prefix(),
			"stale_for": stale.String(),
			"count":     len(list),
			"tasks":     list,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}

	fmt.Println("anc hot —— 进行中的（只读）")
	fmt.Printf("  地址     %s\n", o.shownAddr())
	fmt.Printf("  前缀     %s\n", st.Prefix())
	fmt.Printf("  不新鲜   %s 没动过（只画标记，不拦不删）\n", staleLine(*stale))
	fmt.Println()
	if len(list) == 0 {
		fmt.Println("  现在没有进行中的事。")
		fmt.Println("  记一条：anc hot put <id> -status running -by <谁> -title \"…\"")
		return 0
	}
	staleN, unreadN := 0, 0
	for _, s := range list {
		if s.Stale(now, *stale) {
			staleN++
		}
		if _, ok := s.Time(); !ok {
			unreadN++
		}
		hotPrintState(s, now, *stale)
	}
	fmt.Printf("  共 %d 件进行中", len(list))
	if staleN > 0 {
		fmt.Printf(" · %d 件不新鲜（该去看一眼）", staleN)
	}
	if unreadN > 0 {
		fmt.Printf(" · %d 件时间读不懂", unreadN)
	}
	fmt.Println()
	return 0
}

func cmdHotClaim(args []string) int {
	fs := flag.NewFlagSet("hot claim", flag.ContinueOnError)
	o := addHotOpts(fs)
	by := fs.String("by", "", "谁在认领（必填）")
	lease := fs.Duration("lease", 0, "租约时长；0 = 不到期（人 / agent 断了，事情不该被永远锁住，所以留了口子）")
	flagArgs, pos := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) == 0 {
		fmt.Fprintln(os.Stderr, "错误：缺少 <id>")
		return 2
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()
	saved, err := st.Claim(pos[0], *by, *lease)
	var occupied *hot.ErrOccupied
	if errors.As(err, &occupied) {
		fmt.Fprintf(os.Stderr, "%v\n", occupied)
		fmt.Fprintln(os.Stderr, "  · 想接手：等租约到期，或让认领人自己跑 anc hot release")
		return 1
	}
	if err != nil {
		return hotFail(err)
	}
	fmt.Println("anc hot claim —— 认领到手（SET NX，抢不到就报）")
	hotPrintState(saved, time.Now(), 0)
	return 0
}

func cmdHotRelease(args []string) int {
	fs := flag.NewFlagSet("hot release", flag.ContinueOnError)
	o := addHotOpts(fs)
	by := fs.String("by", "", "谁在放开（只能放自己的）")
	flagArgs, pos := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) == 0 {
		fmt.Fprintln(os.Stderr, "错误：缺少 <id>")
		return 2
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()
	if err := st.Release(pos[0], *by); err != nil {
		return hotFail(err)
	}
	fmt.Printf("anc hot release —— %s 的认领放开了（状态还在，只是没人占着了）\n", pos[0])
	return 0
}

func cmdHotDrop(args []string) int {
	fs := flag.NewFlagSet("hot drop", flag.ContinueOnError)
	o := addHotOpts(fs)
	flagArgs, pos := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) == 0 {
		fmt.Fprintln(os.Stderr, "错误：缺少 <id>")
		return 2
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()
	if err := st.Drop(pos[0]); err != nil {
		return hotFail(err)
	}
	fmt.Printf("anc hot drop —— %s 从热层清走了（状态 + 租约；真相源不归它管）\n", pos[0])
	return 0
}

// done 是「达标」那一步：**先落库，再清热层**。顺序不能反 —— 反了就丢东西。
func cmdHotDone(args []string) int {
	fs := flag.NewFlagSet("hot done", flag.ContinueOnError)
	o := addHotOpts(fs)
	by := fs.String("by", "", "谁交的")
	title := fs.String("title", "", "一句话（热层里没有的话就靠这一条）")
	kind := fs.String("kind", "execution", "这条是什么：execution / decision / note")
	status := fs.String("status", "done", "终态词：done / failed / blocked")
	detail := fs.String("detail", "", "展开")
	refs := fs.String("refs", "", "引用，逗号分隔")
	flagArgs, pos := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) < 2 {
		fmt.Fprintln(os.Stderr, "错误：用法 anc hot done <vault> <id> -by <谁> -title \"…\"")
		return 2
	}
	abs, err := filepath.Abs(pos[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()

	id := strings.TrimSpace(pos[1])
	prev, found, err := st.Get(id)
	if err != nil {
		return hotFail(err)
	}

	ent := timeline.Entry{
		Case:    id,
		At:      time.Now().Format(time.RFC3339),
		Kind:    strings.TrimSpace(*kind),
		Status:  strings.TrimSpace(*status),
		By:      firstNonEmpty(*by, prev.By),
		Title:   firstNonEmpty(*title, prev.Title),
		Detail:  strings.TrimSpace(*detail),
		To:      prev.To,
		Domain:  prev.Domain,
		Project: prev.Project,
	}
	for _, r := range strings.Split(*refs, ",") {
		if s := strings.TrimSpace(r); s != "" {
			ent.Refs = append(ent.Refs, s)
		}
	}
	if ent.Title == "" {
		fmt.Fprintln(os.Stderr, "错误：这条没有标题 —— 用 -title 给一句。")
		fmt.Fprintln(os.Stderr, "      （落库的那条是以后别人唯一的依据，得能被人看懂）")
		return 2
	}
	ent.ID = autoID(ent)

	path, err := timeline.Append(abs, ent)
	if err != nil {
		// 落库失败：热层一个字都不许动 —— 这时候它还是唯一的记录。
		fmt.Fprintf(os.Stderr, "错误：往真相源写失败，热层原样留着（没丢东西）：%v\n", err)
		return 1
	}
	fmt.Println("anc hot done —— 先落库，再清热层")
	fmt.Printf("  %s %s\n", bandDot(timeline.Band(ent.Status)), titleLine(ent))
	fmt.Printf("  落库     %s\n", relTo(abs, path))
	fmt.Printf("  这一条   id=%s kind=%s status=%s by=%s\n",
		ent.ID, dashIfEmpty(ent.Kind), dashIfEmpty(ent.Status), dashIfEmpty(ent.By))
	if !found {
		fmt.Printf("  注意     热层里本来就没有 %s（只落了库）\n", id)
		return 0
	}
	if err := st.Drop(id); err != nil {
		fmt.Fprintf(os.Stderr, "  已落库，但热层没清干净：%v\n", err)
		fmt.Fprintf(os.Stderr, "  · 真相源里已经有了（上面那条）；热层这条下次 anc hot ls 还会出现 —— 再跑一次 anc hot drop %s\n", id)
		return 1
	}
	fmt.Printf("  热层     %s 已清走（状态 + 租约）\n", id)
	return 0
}

// ---------- 回显 ----------

func hotPrintState(s hot.State, now time.Time, staleAfter time.Duration) {
	fmt.Printf("  %s   %s\n", s.ID, dashIfEmpty(s.Title))
	line := "       " + dashIfEmpty(s.Status)
	if s.By != "" {
		line += " · 在做 " + s.By
	}
	if s.To != "" {
		line += " · 交给 " + s.To
	}
	if s.Domain != "" {
		line += " · 域 " + s.Domain
	}
	if s.Project != "" {
		line += " · 项目 " + s.Project
	}
	fmt.Println(line)
	mark := "新鲜"
	if staleAfter > 0 {
		if s.Stale(now, staleAfter) {
			mark = "不新鲜（该去看一眼）"
		}
	}
	if age, ok := s.Age(now); ok {
		fmt.Printf("       %s 前动过 · %s\n", shortDur(age), mark)
	} else {
		fmt.Printf("       时间读不懂（as_of=%q）· 记的时候格式不对\n", s.AsOf)
	}
	// 只有「真占着」才多打一行：没租约时上面那句「在做 X」已经说明了，再打一遍是废话。
	if s.By != "" && s.LeaseHeld(now) {
		fmt.Printf("       认领 %s（租约到 %s）\n", s.By, s.LeaseTo)
	}
	if s.Note != "" {
		fmt.Printf("       备注 %s\n", s.Note)
	}
}

// staleLine 把阈值说成人话：「24h0m0s」这种没人愿意读。
func staleLine(d time.Duration) string {
	switch {
	case d <= 0:
		return "不判（--stale 0 = 关掉这个标记）"
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%d 天", int(d/(24*time.Hour)))
	case d%time.Hour == 0:
		return fmt.Sprintf("%d 小时", int(d/time.Hour))
	case d%time.Minute == 0:
		return fmt.Sprintf("%d 分钟", int(d/time.Minute))
	}
	return d.String()
}

func ttlLine(d time.Duration) string {
	if d <= 0 {
		return "不失效（--ttl 可设；按客户的项目周期收口也是这里）"
	}
	return d.String() + "（到点自动没 —— 只该给授权 / 租约那种东西用）"
}

// 热层连不上时说人话，并给下一步 —— 不是把 err 一喷了事。
func hotUnreachable(err error) int {
	fmt.Fprintf(os.Stderr, "热层没连上：%v\n", err)
	fmt.Fprintln(os.Stderr, "  · 热层是一个独立小进程（默认 Redis），先确认它起来了")
	fmt.Fprintf(os.Stderr, "  · 地址不对就加 --addr，或设 ANC_HOT_ADDR（默认 %s）\n", hot.DefaultAddr)
	fmt.Fprintln(os.Stderr, "  · 自检：anc hot ping")
	return 1
}

func hotFail(err error) int {
	fmt.Fprintf(os.Stderr, "热层这次操作没成：%v\n", err)
	return 1
}
