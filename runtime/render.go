package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"anc/internal/org"
	renderpkg "anc/internal/render"
)

const renderUsage = `anc render / anc org check —— org 真相源 → gateway config

用法：
  anc org check <vault 目录> [--rules]           只加载 + 校验 org，不渲染
  anc render <vault 目录> [选项]                 渲染 gateway config（默认 dry-run）

选项：
  --config <文件>    输出的 config.toml（默认 <vault>/../gateway/config.toml）
  --homes <目录>     每 bot 的家目录根（默认 <vault>/../homes）
  --data <目录>      gateway 的 data_dir（默认 <homes>/../data）
  --agent-home <slug>=<目录>   业务 agent 在本机的 cwd（可重复）—— **本机事实**，不进 git
  --apply            真落盘（时间戳备份 + 原子写）
  --check            与现有配置比对，有漂移就退出码 1
  --adopt            接管一份没有 anc 指纹的现有配置（否则拒绝覆盖）
  --allow-scale      允许这次渲染新增或删除 project（防一次误操作静默上线/下线 bot）
  --rules            （org check）打印生效规则表

退出码：0 一致/成功；1 校验或漂移；2 用法错误

规则档次在 company.md 的 policy 段逐条可覆盖（fatal / warn / off），
但标了红线的规则不许降级 —— 见 anc org check <vault> --rules。
`

func cmdRender(args []string) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	cfg := fs.String("config", "", "输出的 config.toml")
	homes := fs.String("homes", "", "每 bot 的家目录根")
	data := fs.String("data", "", "gateway 的 data_dir")
	apply := fs.Bool("apply", false, "真落盘")
	check := fs.Bool("check", false, "比对现有配置")
	adopt := fs.Bool("adopt", false, "接管没有指纹的现有配置")
	allowScale := fs.Bool("allow-scale", false, "允许新增/删除 project")
	agentHome := &kvFlag{}
	fs.Var(agentHome, "agent-home", "业务 agent 的本机 cwd：<slug>=<目录>（可重复）")
	vault := fs.String("vault", "", "vault 目录（也可用位置参数）")
	// Go 的 flag 包遇到第一个位置参数就停止解析，所以先按「哪些是布尔开关」把位置参数摘出来。
	flagArgs, posArgs := splitArgs(args, map[string]bool{"apply": true, "check": true, "adopt": true, "allow-scale": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	root := strings.TrimSpace(*vault)
	if root == "" && len(posArgs) > 0 {
		root = posArgs[0]
	}
	if root == "" {
		fmt.Fprint(os.Stderr, renderUsage)
		return 2
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 2
	}
	// 默认路径：vault 的兄弟目录，绝不写进 vault（git 只放 org 真相）。
	cfgPath := *cfg
	if cfgPath == "" {
		cfgPath = filepath.Join(filepath.Dir(abs), "gateway", "config.toml")
	}
	homesPath := *homes
	if homesPath == "" {
		homesPath = filepath.Join(filepath.Dir(abs), "homes")
	}
	dataPath := *data
	if dataPath == "" {
		dataPath = filepath.Join(filepath.Dir(abs), "data")
	}

	o, err := org.Load(abs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 1
	}
	host := org.Host{VaultRoot: abs, HomesRoot: homesPath, DataDir: dataPath, AgentHomes: agentHome.m}
	plan, err := renderpkg.Build(o, renderpkg.Options{Host: host, Version: version, Now: time.Now()})
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 1
	}

	existing, readErr := os.ReadFile(cfgPath)
	hasExisting := readErr == nil
	st := readState(cfgPath)
	existing, hasExisting, oldFP := st.Existing, st.HasFile, st.HasFP
	oldInputs := ""
	if hasExisting {
		_, oldInputs, _, _ = renderpkg.Fingerprint(string(existing))
	}

	fmt.Printf("anc render —— org → gateway config\n")
	fmt.Printf("  vault      %s\n", abs)
	fmt.Printf("  成员       启用 %d / 共 %d\n", len(o.Enabled()), len(o.Members))
	fmt.Printf("  project    %d 个：%s\n", len(plan.Projects), strings.Join(plan.Projects, ", "))
	fmt.Printf("  inputs     %s\n", plan.InputsHash)
	fmt.Printf("  目标       %s（%s）\n", cfgPath, map[bool]string{true: "已存在", false: "不存在"}[hasExisting])
	// 非红档发现：规则表产物 + org 层发现，一起回显。
	printIssues(append(append([]org.Issue{}, o.Warnings...), plan.Issues...))
	for _, w := range plan.Warns {
		fmt.Printf("  ⚠️  %s\n", w)
	}

	// 现有配置的安全缺口：产物必须完全显式。缺了就是静默沿用上游默认值 —— 必须说出来，不许吞。
	var secGaps []string
	if hasExisting {
		if secGaps = renderpkg.SecurityGaps(string(existing)); len(secGaps) > 0 {
			fmt.Printf("\n⛔ 现有配置有安全缺口 %d 项（渲染产物必须完全显式）\n", len(secGaps))
			for _, g := range secGaps {
				fmt.Printf("  - %s\n", g)
			}
		}
	}

	if *check {
		if !hasExisting {
			fmt.Printf("\n结论：目标配置不存在 —— 需要 `anc render --apply`。\n")
			return 1
		}
		if len(secGaps) > 0 {
			fmt.Printf("\n结论：先补安全缺口 —— 现有配置缺 %d 项显式声明，漂移比对留着补齐后再看。\n", len(secGaps))
			fmt.Printf("FIX: 跑 `anc render --apply`\n")
			return 1
		}
		oldHashes := renderpkg.PersonaHashesIn(string(existing))
		var changed, added, removed []string
		for _, p := range plan.Projects {
			oh, ok := oldHashes[p]
			switch {
			case !ok:
				added = append(added, p)
			case oh != plan.PersonaHash[p]:
				changed = append(changed, p)
			}
		}
		for p := range oldHashes {
			if !contains(plan.Projects, p) {
				removed = append(removed, p)
			}
		}
		sort.Strings(removed)
		if len(added)+len(changed)+len(removed) == 0 && oldFP && oldInputs == plan.InputsHash {
			fmt.Printf("\n结论：与上一轮渲染一致，0 处漂移。\n")
			fmt.Printf("口径：这只是「现在这份 == anc 上一轮生成的」。它**不等于** gateway 已接受 ——\n")
			fmt.Printf("      真实拉起 + 功能探针（kickstart + 90s 窗口）在阶段 B 的服务单元里才落地。\n")
			return 0
		}
		fmt.Printf("\n结论：检测到漂移 ——\n")
		for _, p := range added {
			fmt.Printf("  新增 project   %s\n", p)
		}
		for _, p := range changed {
			fmt.Printf("  persona 有变   %s\n", p)
		}
		for _, p := range removed {
			fmt.Printf("  将删除 project %s\n", p)
		}
		if !oldFP {
			fmt.Printf("  现有配置没有 anc 指纹（手写或他源）——要接管需显式 `--adopt`\n")
		} else if oldInputs != plan.InputsHash {
			fmt.Printf("  inputs 变化    %s → %s\n", short(oldInputs), short(plan.InputsHash))
		}
		fmt.Printf("FIX: 跑 `anc render --apply`%s%s\n",
			map[bool]string{true: " --adopt", false: ""}[!oldFP],
			map[bool]string{true: " --allow-scale", false: ""}[len(added)+len(removed) > 0])
		return 1
	}

	if !*apply {
		fmt.Printf("\n（dry-run，未落盘；要落盘加 --apply）\n")
		return 0
	}

	if err := commitConfig(st, plan, o, host, renderGates{Adopt: *adopt, AllowScale: *allowScale}); err != nil {
		fmt.Fprintf(os.Stderr, "\n%v\n", err)
		return 1
	}
	fmt.Printf("\n结论：已落盘。装载与回读（daemon install + 凭据桥 + 重启 + 探针）走 `anc apply`。\n")
	return 0
}

// renderState 是「目标配置现在长什么样」—— 落盘的门禁全靠它判断。
type renderState struct {
	Path     string
	Existing []byte
	HasFile  bool // 目标文件在
	HasFP    bool // 且带 anc 指纹（= 是我们上一轮生成的）
}

// readState 读一次现有配置。读不动（不存在）不是错 —— 首次落盘就是这样。
func readState(cfgPath string) renderState {
	st := renderState{Path: cfgPath}
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return st
	}
	st.Existing, st.HasFile = b, true
	_, _, _, st.HasFP = renderpkg.Fingerprint(string(b))
	return st
}

// renderGates 是落盘时的两条门禁。
type renderGates struct {
	Adopt      bool // 接管没有 anc 指纹的现有配置
	AllowScale bool // 允许本次新增 / 删除 project
}

// commitConfig 是「落盘」这一步的唯一实现 —— `anc render --apply` 与 `anc apply` 共用。
// 门禁与措辞只留在这里：别处再抄一遍，两边迟早会漂。
//
// 两条门都不是新设计的：adopt 拦「别人的文件」（没有 anc 指纹的配置不许静默覆盖），
// allow-scale 拦「一次误操作静默上线 / 下线 bot」。
func commitConfig(st renderState, plan *renderpkg.Plan, o *org.Org, host org.Host, gates renderGates) error {
	if st.HasFile && !st.HasFP && !gates.Adopt {
		return refusal{fmt.Sprintf("拒绝覆盖：%s 没有 anc 指纹（手写或他源配置）。确认要接管再加 --adopt。", st.Path)}
	}
	if st.HasFile {
		added, removed := diffSets(plan.Projects, renderpkg.ProjectsIn(string(st.Existing)))
		if (len(added) > 0 || len(removed) > 0) && !gates.AllowScale {
			return refusal{fmt.Sprintf("拒绝落盘：本次会新增 %v / 删除 %v 个 project。确认无误再加 --allow-scale。", added, removed)}
		}
	}
	if st.HasFile {
		backup := fmt.Sprintf("%s.bak-%s", st.Path, time.Now().Format("20060102-150405"))
		if err := os.WriteFile(backup, st.Existing, 0o600); err != nil {
			return fmt.Errorf("备份失败: %w", err)
		}
		fmt.Printf("\n  备份       %s\n", backup)
	}
	// work_dir 就是 agent 进程的 cwd：目录不存在时配置本身没错，
	// 失败会推迟到第一条消息才炸（实测 Windows：fork/exec <agent>: The directory name is invalid）。
	// 这个路径是渲染器的产物，就由渲染器保证它落地 —— 先建目录，再写指向它的配置。
	var newHomes []string
	for _, m := range o.Enabled() {
		dir := renderpkg.WorkDir(m, host)
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			continue
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("建 work_dir 失败 %s: %w", dir, err)
		}
		newHomes = append(newHomes, dir)
	}
	if err := writeAtomic(st.Path, plan.Text); err != nil {
		return fmt.Errorf("落盘失败: %w", err)
	}
	fmt.Printf("  写入       %s（%d 个 project）\n", st.Path, len(plan.Projects))
	if len(newHomes) > 0 {
		fmt.Printf("  家目录     新建 %d 个：%s\n", len(newHomes), strings.Join(newHomes, ", "))
	}
	return nil
}

// refusal 是「被门禁拦下」：调用方打印后退出 1。
// 与 IO 出错分开，是因为两者要现场做的事完全不同 —— 一个要人拍板，一个要修环境。
type refusal struct{ msg string }

func (r refusal) Error() string { return r.msg }

// cmdOrgCheck 只加载 + 校验，不渲染。
func cmdOrgCheck(args []string) int {
	fs := flag.NewFlagSet("org check", flag.ContinueOnError)
	rules := fs.Bool("rules", false, "打印生效规则表")
	flagArgs, posArgs := splitArgs(args, map[string]bool{"rules": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) != 1 {
		fmt.Fprint(os.Stderr, renderUsage)
		return 2
	}
	abs, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 2
	}
	o, err := org.Load(abs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 1
	}
	fmt.Printf("OK —— org 校验通过\n")
	fmt.Printf("  公司    %s（id=%s，平台=%s）\n", o.Company.Name, o.Company.ID, o.Company.Platform)
	fmt.Printf("  角色    %s\n", strings.Join(roleNames(o), ", "))
	fmt.Printf("  成员    启用 %d / 共 %d\n", len(o.Enabled()), len(o.Members))
	fmt.Printf("  路由    %s\n", routingDirs(o))
	fmt.Printf("  授权    %d 条\n", len(o.Grants))
	if over := o.Policy.OverrideIDs(); len(over) > 0 {
		fmt.Printf("  规则覆盖 %s\n", strings.Join(over, ", "))
	}
	if *rules {
		printRuleTable(o)
	}
	printIssues(o.Warnings)
	return 0
}

// printRuleTable 回显生效规则表 —— 规则是数据，得能一眼看全，而不是读实现。
func printRuleTable(o *org.Org) {
	eff := o.Policy.Effective()
	over := map[string]bool{}
	for _, id := range o.Policy.OverrideIDs() {
		over[id] = true
	}
	fmt.Printf("\n生效规则表（%d 条；★ = 被 company.md 覆盖过）\n", len(eff))
	for _, r := range eff {
		mark := " "
		if over[r.ID] {
			mark = "★"
		}
		lock := ""
		if r.Locked {
			lock = " [红线]"
		}
		fmt.Printf("  %s %-32s %-5s%s  %s\n", mark, r.ID, r.Level, lock, r.Why)
	}
}

// printIssues 回显非红档发现（走 stdout）。
func printIssues(list []org.Issue) { printIssuesTo(os.Stdout, list) }

// printIssuesTo 回显非红档发现。门禁不许静默：不拦的，必须让人看见。
// 带 writer 是因为 `org export` 的 stdout 必须是纯 JSON —— 提示只能走 stderr。
func printIssuesTo(w io.Writer, list []org.Issue) {
	if len(list) == 0 {
		return
	}
	fmt.Fprintf(w, "\n⚠️  非红档发现 %d 条（不拦，但请过目）\n", len(list))
	for _, i := range list {
		fmt.Fprintf(w, "  - %s\n", i.String())
	}
}

func roleNames(o *org.Org) []string { return o.RoleKeys() }

func routingDirs(o *org.Org) string {
	if len(o.Routing) == 0 {
		return "（无数据目录）"
	}
	out := make([]string, 0, len(o.Routing))
	for _, r := range o.Routing {
		out = append(out, r.Dir)
	}
	return strings.Join(out, ", ")
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func diffSets(newList, oldList []string) (added, removed []string) {
	for _, p := range newList {
		if !contains(oldList, p) {
			added = append(added, p)
		}
	}
	for _, p := range oldList {
		if !contains(newList, p) {
			removed = append(removed, p)
		}
	}
	return added, removed
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// writeAtomic：同目录 temp + fsync + rename，权限 600（配置里只有 ${ENV} 引用，但仍收紧）。
func writeAtomic(path, text string) error {
	return writeFileAtomic(path, []byte(text), 0o600, 0o700)
}

// writeFileAtomic 是同目录 temp + fsync + rename 的通用写口。
//
// 权限由调用方给：config 那份是 600 / 700（里面只有 ${ENV} 引用，但仍收紧）；
// 真相源里的 markdown 是 644 / 755 —— 与 org init 生成的那批同档，
// 不然一次 `anc ingest` 之后 vault 里就多出一批只有属主读得了的文件。
func writeFileAtomic(path string, data []byte, filePerm, dirPerm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".anc-write-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, filePerm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
