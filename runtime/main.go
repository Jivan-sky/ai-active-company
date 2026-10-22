// anc —— ANC 驻场装配器。
//
// 一句话：把一台空机器（或一个空目录）在几分钟内变成「客户 ANC 运行层」——
// 树干目录 + 路由表 + 判层卡 + 计划书骨架 + 连接件 + 操作日志与可逆性分级。
//
// 为什么是 Go：装配器要在客户的 Mac mini / Windows / Linux 上跑，同一份源码三平台，
// 且交付物不许自带运行时。模板全部外置成纯文件，现场改模板不用重编译。
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// 模板与资产清单**编进 exe**（议题 #19）：装机第一步就是 `anc init`，而它原来只认
// 「exe 同级 templates/」与「cwd 上级 templates/」两处 —— 拷一个 exe 过去就挂，
// 「发一个 exe 出去」这个交付形态当场是假的。
//
// 三处磁盘都找不到时才落到这一份：**拷一个 exe 就能 init**。磁盘那三处仍然优先 ——
// 现场改模板不用重编（那是当初把模板外置的理由；两条不冲突：一条管交付形态，一条管改起来快）。
//
//go:embed all:templates all:inventory
var embeddedRuntime embed.FS

const version = "0.1.0"

// ---------- 九宫格：三层客户 ----------

type tierInfo struct {
	Code int
	Name string
	Size string
	Want string
	Play []string
	Note string
}

var tiers = map[int]tierInfo{
	1: {1, "降本增效型", "约 30 人", "省钱",
		[]string{"⑦ ROI 切单（商务）——聚焦高频痛点、固定范围与报价、验收标准清晰", "⑧ 核心工作流梳理（产品）——1~3 条人力密集的高频流程，输入输出明确", "⑨ 轻量工具化（研发）——插件/脚本/IM 机器人，必须快、稳、易用"},
		"别接大而全。切一个能快速回本的小口。"},
	2: {2, "进步型", "约 200 人", "进步（不是省钱）",
		[]string{"④ 共同定义试点（商务）——锁一个试点部门、定义成功标准、给分阶段路线图", "⑤ 复杂工作流编排（产品）——5~10 条工作流、数据流归一、异常处理、系统集成", "⑥ 智能体工作系统（研发）——上 Agent、自研 Skill、统一知识库当业务大脑"},
		"老板不为降本买单；主打降本反而脱靶，讲愿景与整体流程改造。"},
	3: {3, "战略型", "大型国央企", "战略变革 + 安全合规可审计",
		[]string{"① 战略立项（商务）——多意图识别、对齐一把手/二把手、写进公司战略", "② 业务蓝图（产品）——Ontology 级建模、跨系统蓝图、组织与岗位调整路径", "③ 安全合规底座（研发）——私有化/信创、权限与审计、数据不出域、等保"},
		"抬头要够大；一个人接不住，要 FDE 团队。"},
}

// ---------- 交付形态 ----------

var forms = map[string]string{
	"F1": "48h 薄切片：可跑的薄切片。前提＝有真实单据 + 有一线可当场验证。",
	"F2": "一页纸真伪结论：判断 + 证据 + 建议。三种情况交这个——真伪是「伪」、经济账算不过、这活之前有人做过且说不清为什么失败。",
	"F3": "现场工具：一线持续要用的脚本/工具。从 F1 验证通过、一线还要求接着用，才固化它。",
	"F4": "报告/复盘：对方明确只要工具之外的结论时才交。少做，防退化回咨询。",
}

// ---------- 客户工作区骨架 ----------

var skeletonDirs = []string{
	"00-inbox/_originals",
	"10-knowledge",
	"20-ops",
	"30-delivery",
	"roles",
	"50-skills",
	"company",
}

type tplEntry struct{ Src, Dst string }

var tplMap = []tplEntry{
	{"README.md.tmpl", "README.md"},
	{"CONTRIBUTING.md.tmpl", "CONTRIBUTING.md"},
	{"CLAUDE.md.tmpl", "CLAUDE.md"},
	{"CLAUDE.md.tmpl", "AGENTS.md"},
	{"tier-card.md.tmpl", "30-delivery/00-判层卡.md"},
	{"plan.md.tmpl", "30-delivery/01-计划书.md"},
	{"connectors.md.tmpl", "20-ops/00-七个连接件.md"},
	{"ops-log.md.tmpl", "20-ops/01-操作日志与可逆性分级.md"},
}

// ---------- 入口 ----------

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	// 脱敏表先落定（进程一次）—— 审计的**落盘即脱敏**必须对五条写口同时生效，
	// 散在各子命令里落定早晚漏一条。见 redact.go。
	armRedactor()

	cmd := os.Args[1]
	args := os.Args[2:]
	switch cmd {
	case "init":
		os.Exit(cmdInit(args))
	case "doctor":
		os.Exit(cmdDoctor(args))
	case "assets":
		os.Exit(cmdAssets(args))
	case "render":
		os.Exit(cmdRender(args))
	case "apply":
		os.Exit(cmdApply(args))
	case "probe":
		os.Exit(cmdProbe(args))
	case "notify":
		os.Exit(cmdNotify(args))
	case "trail":
		os.Exit(cmdTrail(args))
	case "audit":
		os.Exit(cmdAudit(args))
	case "timeline":
		os.Exit(cmdTimeline(args))
	case "hot":
		os.Exit(cmdHot(args))
	case "agent":
		os.Exit(cmdAgent(args))
	case "task":
		os.Exit(cmdTask(args))
	case "distill":
		os.Exit(cmdDistill(args))
	case "approvals":
		os.Exit(cmdApprovals(args))
	case "envelope":
		os.Exit(cmdEnvelope(args))
	case "gate":
		os.Exit(cmdGate(args))
	case "gateway":
		os.Exit(cmdGateway(args))
	case "context":
		os.Exit(cmdContext(args))
	case "search":
		os.Exit(cmdSearch(args))
	case "ingest":
		os.Exit(cmdIngest(args))
	case "import":
		os.Exit(cmdImport(args))
	case "org":
		if len(args) > 0 && args[0] == "check" {
			os.Exit(cmdOrgCheck(args[1:]))
		}
		if len(args) > 0 && args[0] == "init" {
			os.Exit(cmdOrgInit(args[1:]))
		}
		if len(args) > 0 && args[0] == "export" {
			os.Exit(cmdOrgExport(args[1:]))
		}
		if len(args) > 0 && args[0] == "who" {
			os.Exit(cmdOrgWho(args[1:]))
		}
		if len(args) > 0 && args[0] == "access" {
			os.Exit(cmdOrgAccess(args[1:]))
		}
		fmt.Fprint(os.Stderr, renderUsage)
		fmt.Fprint(os.Stderr, boardUsage)
		fmt.Fprint(os.Stderr, orgWhoUsage)
		fmt.Fprint(os.Stderr, orgAccessUsage)
		os.Exit(2)
	case "board":
		if len(args) > 0 && args[0] == "serve" {
			os.Exit(cmdBoardServe(args[1:]))
		}
		fmt.Fprint(os.Stderr, boardServeUsage)
		os.Exit(2)
	case "version", "--version", "-v":
		fmt.Println("anc " + version)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "未知子命令：%s\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`anc —— ANC 驻场装配器

用法：
  anc init <目录> --client <客户名> [--tier 1|2|3] [--form F1|F2|F3|F4] [--date YYYY-MM-DD] [--templates <模板目录>] [--git] [--force]
  anc doctor [目录]                 环境与网络自检（客户机器上跑）
  anc assets [--templates <模板目录>]  列出可复用资产台账
  anc org check <vault>             只加载 + 校验 org 真相源
  anc org init <目录> --client <客户名> --id <ascii-id>
                                    生成 org 真相源骨架（vault 模板）
  anc org export <vault>            投影成看板消费的只读 JSON（打到 stdout）
  anc ingest <vault> --from <提案.json|-> [--dry-run] [--patch]
  anc ingest --dump-shape            出厂提案形状（数据）；给 harness 那一面照它说话
                                    外部材料进真相源的唯一机器入口：先校验（同一张规则表）再整份替换落盘，不 commit
  anc import reports <vault> <日报目录> [--to <agent>] [--since YYYY-MM-DD] [--apply]
                                    日报批量导入：完成/决策进 timeline，未完成待办入队（默认 dry-run，稳定 id 去重）
  anc board serve <vault> [--addr 127.0.0.1:8787]
                                    起只读看板（前端已嵌在二进制里；默认只绑本机）
   anc render <vault> [选项]         渲染 gateway config（默认 dry-run）
   anc apply <vault> [选项]          渲染 → 装载上游 daemon → 凭据桥 → 回读（默认 dry-run）
   anc probe <vault> [选项]          运行态探针：每个 bot 到底能不能回话（只读，不烧 token）
   anc notify <vault> [选项]         把报红推到负责人面前（默认 dry-run，--send 才真发）
   anc trail <vault> [选项]          只读聚合：谁问了什么、agent 干了什么、花了多少
   anc timeline add|list <vault>      决策与执行的留存记录（红 / 黄 / 绿三档，append-only）
   anc hot ping|put|ls|claim|release|drop|done
                                     「进行中」的热层：状态是当前值，不是历史（达标落库，进行中放这里）
   anc agent beat|ls <vault>          业务 agent 心跳与在线态（在线 ≠ 最近调用）
   anc task add|pull|ack|handoff|submit|verify|escalate|ls <vault>
                                      拉取式任务队列（ANC 只入队，agent 自己 pull）
   anc distill ls|draft|check <vault> 可沉淀候选 / 生成草稿 / 发布前校验（只读；不碰正式技能）
   anc approvals add|ls|decide|card|card-action <vault>
                                      提案的待批队列、卡片与「点头」（审批的落点：看板 / 飞书卡片走同一个入口）
                                      卡片长什么样是数据（--frame），值是现算的（谁提的 / 属于谁 / 该谁批）
   anc audit log|add|collect <vault>  行使的流水：谁在什么时候、对谁、行使了什么（含被拒的）
   anc envelope check <vault> <信封.json>  接入面的信封：解析 + 绑真相源（只读）
   anc envelope serve <vault> [--addr 127.0.0.1:8791]
                                    接入面：harness 走 MCP 把信封递进来（无状态，默认只绑本机）
   anc gate <vault> [--rules <文件>] [--show-rules] [--json]
                                    交付链路的门：五道门的证据落痕了没有（只出结论，不设门禁）
   anc gateway relay <vault> [--app-id <id>] [--patch print|off|cmd:<命令>]
                                    第二条网关的收口环：吃一行一条的事件流（stdin），落痕 + 收口
  anc context pack <vault> --who <编号> [--domain <slug>]
                                    一个 agent 的工作上下文（只读投影：身份 / 业务域 / 数据源 / 可见范围 / 技能 / 项目）
  anc context freeze <vault> --who <编号> [--domain <slug>] [--to <目录>]
                                    把这一份冻成副本落 vault 的 context/（快照；给交付与审计取证）
  anc search <vault> --q "要查什么" [--who <编号>] [--domain <slug>] [--limit 8]
                                    按域可见范围搜知识（默认 BM25；给指针 + 片段，查不到就说没入库）
  anc version

三层客户（九宫格判层，先判层再定打法，答错层＝后面全错）：
  1 = 降本增效型（约 30 人，要省钱）
  2 = 进步型（约 200 人，要进步）
  3 = 战略型（大型国央企，要战略变革 + 安全合规）

例：
  anc init ./climax --client 某车队 --tier 1 --form F1 --git
`)
}

// ---------- init ----------

func cmdInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	client := fs.String("client", "", "客户名（必填）")
	tierNo := fs.Int("tier", 1, "客户层级 1|2|3")
	form := fs.String("form", "F1", "交付形态 F1|F2|F3|F4")
	date := fs.String("date", time.Now().Format("2006-01-02"), "日期 YYYY-MM-DD")
	tplDir := fs.String("templates", "", "模板目录（默认：exe 同级 templates/，其次 ./templates）")
	doGit := fs.Bool("git", false, "生成后执行 git init + 首次提交")
	force := fs.Bool("force", false, "目标目录已存在且非空时仍继续")
	flagArgs, posArgs := splitArgs(args, map[string]bool{"git": true, "force": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if *client == "" {
		fmt.Fprintln(os.Stderr, "错误：--client 必填")
		return 2
	}
	ti, ok := tiers[*tierNo]
	if !ok {
		fmt.Fprintf(os.Stderr, "错误：--tier 只能是 1 / 2 / 3，收到 %d\n", *tierNo)
		return 2
	}
	formKey := strings.ToUpper(*form)
	formMean, ok := forms[formKey]
	if !ok {
		fmt.Fprintf(os.Stderr, "错误：--form 只能是 F1 / F2 / F3 / F4，收到 %s\n", *form)
		return 2
	}
	if _, err := time.Parse("2006-01-02", *date); err != nil {
		fmt.Fprintf(os.Stderr, "错误：--date 需为 YYYY-MM-DD，收到 %s\n", *date)
		return 2
	}
	dst := ""
	if len(posArgs) > 0 {
		dst = posArgs[0]
	}
	if dst == "" {
		fmt.Fprintln(os.Stderr, "错误：缺少目标目录")
		return 2
	}
	abs, err := filepath.Abs(dst)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	tpl, err := resolveTemplates(*tplDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	if !*force {
		if entries, err := os.ReadDir(abs); err == nil && len(entries) > 0 {
			fmt.Fprintf(os.Stderr, "错误：%s 已存在且非空（要覆盖请加 --force）\n", abs)
			return 1
		}
	}

	vars := map[string]string{
		"{{CLIENT}}":      *client,
		"{{DATE}}":        *date,
		"{{TIER_N}}":      fmt.Sprintf("%d", ti.Code),
		"{{TIER_NAME}}":   ti.Name,
		"{{TIER_SIZE}}":   ti.Size,
		"{{TIER_WANT}}":   ti.Want,
		"{{TIER_PLAY}}":   "- " + strings.Join(ti.Play, "\n- "),
		"{{TIER_NOTE}}":   ti.Note,
		"{{FORM}}":        formKey,
		"{{FORM_MEAN}}":   formMean,
		"{{DIR}}":         abs,
		"{{ANC_VERSION}}": version,
	}

	for _, d := range skeletonDirs {
		if err := os.MkdirAll(filepath.Join(abs, d), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "错误：建目录 %s 失败：%v\n", d, err)
			return 1
		}
	}
	written := 0
	for _, t := range tplMap {
		raw, err := tpl.Read(t.Src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误：读模板 %s 失败：%v\n", t.Src, err)
			return 1
		}
		out := render(string(raw), vars)
		out = strings.TrimRight(out, "\n") + "\n"
		p := filepath.Join(abs, filepath.FromSlash(t.Dst))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "错误：%v\n", err)
			return 1
		}
		if err := os.WriteFile(p, []byte(out), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "错误：写 %s 失败：%v\n", p, err)
			return 1
		}
		written++
	}
	// 空目录 git 不跟踪，给每个骨架目录落 .gitkeep，否则 clone / 换机后骨架就散架。
	for _, d := range skeletonDirs {
		_ = os.WriteFile(filepath.Join(abs, d, ".gitkeep"), []byte(""), 0o644)
	}

	fmt.Printf("✅ 已生成客户 ANC 运行层：%s\n\n", abs)
	fmt.Printf("   客户：%s\n   判层：第 %d 层 · %s（%s）→ 老板要「%s」\n   形态：%s —— %s\n",
		*client, ti.Code, ti.Name, ti.Size, ti.Want, formKey, formMean)
	fmt.Printf("   模板：%d 个（%s）→ 写出 %d 个文件\n\n", countTemplates(tpl), tpl.Kind, written)
	fmt.Println("   下一步：")
	fmt.Println("   1) 填 30-delivery/00-判层卡.md —— 没填完不要开跑")
	fmt.Println("   2) 填 30-delivery/01-计划书.md 第 9 节与「停止条件」，且必须让客户也认可")
	fmt.Println("   3) 把客户的真实单据、群文件丢进 00-inbox/，原件放 00-inbox/_originals/")

	if *doGit {
		if _, err := exec.LookPath("git"); err != nil {
			fmt.Println("\n⚠️  未找到 git，跳过 --git。")
		} else {
			steps := [][]string{
				{"init", "-q"},
				{"add", "-A"},
				{"-c", "user.name=anc", "-c", "user.email=anc@local", "commit", "-q", "-m",
					fmt.Sprintf("anc init: %s (T%d/%s, %s)", *client, ti.Code, ti.Name, formKey)},
			}
			for _, s := range steps {
				c := exec.Command("git", s...)
				c.Dir = abs
				if out, err := c.CombinedOutput(); err != nil {
					fmt.Printf("\n⚠️  git %s 失败：%v %s\n", strings.Join(s, " "), err, strings.TrimSpace(string(out)))
					return 0
				}
			}
			fmt.Println("\n   git：已 init 并完成首次提交（用局部身份 anc/anc@local，未改全局配置）")
		}
	}
	return 0
}

func countTemplates(src templateSource) int {
	n := 0
	for _, t := range tplMap {
		if _, err := src.Read(t.Src); err == nil {
			n++
		}
	}
	return n
}

func render(s string, vars map[string]string) string {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s = strings.ReplaceAll(s, k, vars[k])
	}
	return s
}

// splitArgs：Go 标准库 flag 遇到第一个非 flag 参数就停止解析，
// 而驻场时人很自然会写 `anc init ./climax --client 某车队`（目录在前）。
// 所以先把位置参数摘出来，让 flag 排在前后都能认。
// boolFlags 列出「不带值」的开关，其余 `-x` 一律吃掉下一个参数当值。
// kvFlag 是可重复的 `--agent-home <slug>=<目录>`：**本机事实**，记「这台机器上这个业务 agent 的 cwd」。
//
// 为什么用旗标、不写进 org 真相源：路径是本机事实，进了 git 就会在跨机同步时只对一台机器成立
// （SPEC §4.5：git = org 真相，`~/.anc` = 部署参数）。为什么可重复、不另开一种文件格式：
// 部署参数现在本来就是旗标这一族（--vault / --homes / --data），加一种文件格式等于多一样要解释的东西。
type kvFlag struct{ m map[string]string }

func (f *kvFlag) String() string {
	if f == nil || len(f.m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(f.m))
	for k := range f.m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+f.m[k])
	}
	return strings.Join(parts, ",")
}

func (f *kvFlag) Set(v string) error {
	i := strings.Index(v, "=")
	if i <= 0 || strings.TrimSpace(v[i+1:]) == "" {
		return fmt.Errorf("要写成 <slug>=<目录>：%q", v)
	}
	k := strings.TrimSpace(v[:i])
	if !reSlugLike.MatchString(k) {
		return fmt.Errorf("左边要是 ASCII 小写标识（%s）：%q", reSlugLike, k)
	}
	if f.m == nil {
		f.m = map[string]string{}
	}
	f.m[k] = strings.TrimSpace(v[i+1:])
	return nil
}

// reSlugLike 与 org 层的 slug 形状对齐（`[a-z][a-z0-9-]*`）—— 这里是给人当场纠错的，
// 真正的判据仍在 org 规则表里，两处不共用一支代码但共用同一个形状。
var reSlugLike = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func splitArgs(args []string, boolFlags map[string]bool) (flags []string, positional []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' {
			flags = append(flags, a)
			name := strings.TrimLeft(a, "-")
			if strings.Contains(name, "=") || boolFlags[name] {
				continue
			}
			if i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, a)
	}
	return flags, positional
}

// padRight：按「显示宽度」右补空格（中文按 2 列算），让 doctor 的输出对齐。
func padRight(s string, width int) string {
	w := 0
	for _, r := range s {
		if r < 0x1100 {
			w++
			continue
		}
		wide := r == 0x2329 || r == 0x232A ||
			(r >= 0x1100 && r <= 0x115F) ||
			(r >= 0x2E80 && r <= 0xA4CF && r != 0x303F) ||
			(r >= 0xAC00 && r <= 0xD7A3) ||
			(r >= 0xF900 && r <= 0xFAFF) ||
			(r >= 0xFE30 && r <= 0xFE6F) ||
			(r >= 0xFF00 && r <= 0xFF60) ||
			(r >= 0xFFE0 && r <= 0xFFE6) ||
			(r >= 0x1F300 && r <= 0x1FAFF) ||
			(r >= 0x20000 && r <= 0x3FFFD)
		if wide {
			w += 2
		} else {
			w++
		}
	}
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// templateSource 是「模板从哪来」。它只做两件事：读一份模板、说清自己是谁（报错与 doctor 都回显认的那个来处）。
//
// 为什么要这一层：模板有两个来处 —— 磁盘（可以现场改）与内嵌（拷一个 exe 就够，议题 #19）。
// 两个来处的读法不同，但**判据只该有一份**：调用方只管 Read，不再各写一遍 os.ReadFile。
type templateSource struct {
	Kind   string // 说给人听的来处
	Dir    string // 磁盘目录；内嵌时是空串
	fsys   fs.FS  // 读的根
	prefix string // 模板在 fsys 里的前缀：磁盘目录 = ""，内嵌 = "templates/"
}

func (s templateSource) Read(name string) ([]byte, error) {
	return fs.ReadFile(s.fsys, s.prefix+filepath.ToSlash(name))
}

// Open 读一份**兄弟**路径上的文件（`inventory/assets.md` 这种，不在模板目录里）：
// 磁盘上是模板目录的上一级；内嵌时同一条相对路径直接在内嵌树里取（内嵌树以运行层的包目录为根）。
func (s templateSource) Open(rel string) ([]byte, error) {
	if s.Dir == "" {
		return fs.ReadFile(s.fsys, filepath.ToSlash(rel))
	}
	return os.ReadFile(filepath.Join(filepath.Dir(s.Dir), filepath.FromSlash(rel)))
}

// Sub 取模板目录下的一棵子树（`org/` 骨架走它）—— 两个来处都要能取。
func (s templateSource) Sub(dir string) (templateSource, error) {
	if s.Dir == "" {
		return templateSource{Kind: s.Kind, fsys: s.fsys, prefix: s.prefix + dir + "/"}, nil
	}
	p := filepath.Join(s.Dir, filepath.FromSlash(dir))
	if _, err := fs.Stat(os.DirFS(p), "."); err != nil {
		return templateSource{}, fmt.Errorf("模板里没有 %s/ 这一棵：%v", dir, err)
	}
	return templateSource{Kind: "目录 " + p, Dir: p, fsys: os.DirFS(p)}, nil
}

// resolveTemplates：优先 --templates，其次 exe 同级 templates/，最后 ./templates；
// **三处都没有就落到内嵌那一份**（编在 exe 里，见 embeddedRuntime）。
// 磁盘优先是有意的：现场改模板不必重编。
func resolveTemplates(explicit string) (templateSource, error) {
	var cands []string
	if explicit != "" {
		cands = append(cands, explicit)
	}
	if exe, err := os.Executable(); err == nil {
		cands = append(cands, filepath.Join(filepath.Dir(exe), "templates"))
	}
	cands = append(cands, "templates")
	var tried []string
	for _, c := range cands {
		abs, _ := filepath.Abs(c)
		tried = append(tried, abs)
		if st, err := os.Stat(filepath.Join(abs, "README.md.tmpl")); err == nil && !st.IsDir() {
			return templateSource{Kind: "目录 " + abs, Dir: abs, fsys: os.DirFS(abs)}, nil
		}
	}
	// 内嵌那一份必须在：它是编译期就进二进制的，取不到等于这个 exe 是坏的 —— 照实报，别装作只是「没找到目录」。
	if _, err := fs.Stat(embeddedRuntime, "templates/README.md.tmpl"); err != nil {
		return templateSource{}, fmt.Errorf("找不到模板目录（需含 README.md.tmpl），exe 里那一份也读不了：%v。已试：\n  %s",
			err, strings.Join(tried, "\n  "))
	}
	return templateSource{Kind: "内嵌（编在 exe 里）", fsys: embeddedRuntime, prefix: "templates/"}, nil
}

// ---------- doctor ----------

type check struct {
	Name string
	OK   bool
	Info string
}

func cmdDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	tplDir := fs.String("templates", "", "模板目录")
	skipNet := fs.Bool("skip-net", false, "跳过网络连通性检查")
	flagArgs, posArgs := splitArgs(args, map[string]bool{"skip-net": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	target := ""
	if len(posArgs) > 0 {
		target = posArgs[0]
	}
	var cs []check

	// 模板
	if tpl, err := resolveTemplates(*tplDir); err != nil {
		cs = append(cs, check{"模板目录", false, err.Error()})
	} else {
		cs = append(cs, check{"模板目录", true, fmt.Sprintf("%s（%d 个已登记模板）", tpl.Kind, countTemplates(tpl))})
	}
	// git
	if p, err := exec.LookPath("git"); err != nil {
		cs = append(cs, check{"git", false, "未找到 —— 没有 git 就没有「落盘即生效」和可回溯"})
	} else {
		cs = append(cs, check{"git", true, p})
	}
	// 目标目录可写
	if target != "" {
		abs, _ := filepath.Abs(target)
		if st, err := os.Stat(abs); err != nil {
			cs = append(cs, check{"目标目录", false, "不存在：" + abs})
		} else if !st.IsDir() {
			cs = append(cs, check{"目标目录", false, "不是目录：" + abs})
		} else {
			f, err := os.CreateTemp(abs, ".anc-write-*")
			if err != nil {
				cs = append(cs, check{"目标目录可写", false, err.Error()})
			} else {
				name := f.Name()
				f.Close()
				os.Remove(name)
				cs = append(cs, check{"目标目录可写", true, abs})
			}
		}
	} else {
		cs = append(cs, check{"目标目录可写", true, "未指定（跳过）"})
	}
	// 断电兜底：装机形态上的三条前提 —— 起得来吗（没装就明确跳过，见 startup.go）
	cs = append(cs, startupChecks()...)
	// 代理环境（本地部署最常见的坑：全局代理把国内 API 绕到境外）
	proxied := false
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		if v := os.Getenv(k); v != "" {
			cs = append(cs, check{"代理环境 " + k, false, v + " —— 确认它不会把国内 IM 接口绕到境外节点"})
			proxied = true
			break
		}
	}
	if !proxied {
		cs = append(cs, check{"代理环境", true, "未设置"})
	}
	// 网络
	if !*skipNet {
		for _, h := range []struct{ name, addr string }{
			{"飞书 open API", "open.feishu.cn:443"},
			{"钉钉 API", "api.dingtalk.com:443"},
			{"企业微信 API", "qyapi.weixin.qq.com:443"},
			{"Go 模块代理（备选）", "goproxy.cn:443"},
		} {
			conn, err := net.DialTimeout("tcp", h.addr, 4*time.Second)
			if err != nil {
				cs = append(cs, check{"连通 " + h.name, false, h.addr + " —— " + err.Error()})
			} else {
				conn.Close()
				cs = append(cs, check{"连通 " + h.name, true, h.addr})
			}
		}
	}

	bad := 0
	fmt.Println("anc doctor —— 驻场环境自检")
	fmt.Println("（IM 平台都提供免公网 IP 的长连接，所以客户机器不需要公网 IP、不需要开端口、不需要备案）")
	fmt.Println()
	for _, c := range cs {
		mark := "✅"
		if !c.OK {
			mark = "⚠️"
			bad++
		}
		fmt.Printf("  %s %s %s\n", mark, padRight(c.Name, 22), c.Info)
	}
	fmt.Println()
	if bad == 0 {
		fmt.Println("结论：全部通过，可以 anc init。")
		return 0
	}
	fmt.Printf("结论：%d 项需要处理（⚠️ 不一定是阻断项，但要知道它存在）。\n", bad)
	return 1
}

// ---------- assets ----------

func cmdAssets(args []string) int {
	fs := flag.NewFlagSet("assets", flag.ContinueOnError)
	tplDir := fs.String("templates", "", "模板目录")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	tpl, err := resolveTemplates(*tplDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	b, err := tpl.Open("inventory/assets.md")
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：找不到 inventory/assets.md：%v\n", err)
		return 1
	}
	fmt.Print(string(b))
	return 0
}
