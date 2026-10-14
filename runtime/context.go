package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"anc/internal/ctxpack"
	"anc/internal/hot"
	"anc/internal/org"
)

const contextUsage = `anc context pack <vault> --who <编号> [--domain <slug>]
anc context freeze <vault> --who <编号> [--domain <slug>] [--to <目录>]

  一个 agent 的**工作上下文**：身份 / 业务域 / 数据源 / 可见范围 / 技能来源 / 在跟的项目。

  这份是**算出来的只读投影**，不是新的真相源 —— 真相源仍是 vault 里的 markdown。
  它与 harness 走 MCP 调的 anc_read_context、看板的「上下文」页是**同一个装配点**
  （ctxpack.Assemble）：三个出口，一份上下文。

  还捎上「派给我的待办」（热层里 to / by 指向你的那些）—— **反向下发**就靠这一块：
  agent 在自己的会话里 pull，ANC 不去插话、不去打断。任务本体住热层（当前值），
  达标的落 timeline（历史）。**热层接不上不编**：报缺说「这一段答不了」，
  不拿空清单冒充「没人派给你」。

  --who <编号>        成员名（members/<名>）或业务 agent 的 slug（agents.md）
  --domain <slug>     只要这一块业务；不写 = 它负责的全部域（外加「找谁」目录）
  --hot-addr <地址>   热层地址（「派给我的待办」从这儿来）；默认 127.0.0.1:6379，也可用 ANC_HOT_ADDR
  --hot-prefix <前缀> 热层 key 前缀；默认 anc，也可用 ANC_HOT_PREFIX
  --to <目录>         冻结副本落到哪（只有 freeze 有这一项）；不写 = <vault>/context

  pack    只打 stdout，一个字不落盘 —— 投影是纯函数产物，存下来就成第三处会漂移的源。
  freeze  另冻一份**副本**到 <vault>/context/<编号>[-<slug>].json（进 git，给交付与审计取证）。
          它是快照、不是常贮：里面 at 就是算出来的时刻，同一个编号再冻一次会**覆盖**；
          内容与 pack 的 stdout 相同（文件末尾多一个换行，文件该以换行收尾）。
          **拒了就不冻**：这个人不存在 / 要的是别人的域 —— 那时候没有可交付的上下文，
          那条账记在 audit/（一次「想行使」），不在这里留个空壳。

  两个子命令都**不留痕**：audit 记的是「一次**行使**」（谁向平台要了什么），那条账挂在 MCP
  那个入口上；这是操作者在自己机器上打一份投影 / 冻一份存档，同 anc org export 一个性质。

  退出码：0 = 答出来了 · 1 = 拒了（这个人不存在 / 越域 / on_behalf_of 解不出），或真相源读不了 · 2 = 用法错
`

// cmdContext 生成一份工作上下文（议题 #53）。
//
// 为什么 pack 是「按需生成」而不是落盘常贮：真相源 markdown · 留痕 jsonl 已经两处了，
// 再把装配产物存一份，就是第三处会漂移的源。要**存下来**的那一份走 freeze ——
// 它写的是一个带时刻的快照，不是又一处「大家都去读」的源（两个动词分开，账才分得清）。
func cmdContext(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, contextUsage)
		return 2
	}
	switch args[0] {
	case "pack":
		return cmdContextPack(args[1:])
	case "freeze":
		return cmdContextFreeze(args[1:])
	}
	fmt.Fprint(os.Stderr, contextUsage)
	return 2
}

// contextArgs 是 pack 与 freeze 共用的那几项：vault 位置、who、domain、热层。
// 两个动词的入参口径必须一样 —— 各写一遍，迟早只有一边跟得上（本仓库付过学费的那类）。
type contextArgs struct {
	vault     string
	who       string
	domain    string
	hotAddr   string
	hotPrefix string
}

// parseContextArgs 解析 `<vault> --who --domain [--to]`。to 只在 freeze 用得上，
// 不给 to 的动词传一个 nil 进来即可（nil = 不认这一项，给了就算用法错）。
func parseContextArgs(name string, args []string, to *string) (contextArgs, int) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	who := fs.String("who", "", "编号：成员名或业务 agent 的 slug（必填）")
	domain := fs.String("domain", "", "只要这一块业务（slug）；不写 = 它负责的全部域")
	hotAddr := fs.String("hot-addr", "", "热层地址（「派给我的待办」用它；默认 "+hot.DefaultAddr+"；也可用 ANC_HOT_ADDR）")
	hotPrefix := fs.String("hot-prefix", "", "热层 key 前缀（默认 "+hot.DefaultPrefix+"；也可用 ANC_HOT_PREFIX）")
	var toFlag *string
	if to != nil {
		toFlag = fs.String("to", "", "冻结副本的落点目录（默认 <vault>/context）")
	}
	flagArgs, posArgs := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return contextArgs{}, 2
	}
	if len(posArgs) != 1 || *who == "" {
		fmt.Fprint(os.Stderr, contextUsage)
		return contextArgs{}, 2
	}
	if toFlag != nil {
		*to = *toFlag
	}
	abs, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return contextArgs{}, 2
	}
	return contextArgs{vault: abs, who: *who, domain: *domain, hotAddr: *hotAddr, hotPrefix: *hotPrefix}, 0
}

// readHotTasks 是给「派给我的待办」读热层的那一道口。
//
// 做成变量是为了用例能换掉它：热层那条路要验「接上了」与「没接上」两档，
// 而为了这个在用例里起一个假 redis 不值当（同 board.Server.Now 那个注入口的用意）。
// 也正因为换得掉，「三个出口逐字段相同」这条不变量才验得动 —— 两个出口喂同一份输入。
var readHotTasks = func(a contextArgs) (ctxpack.TasksIn, func()) {
	noop := func() {}
	st, err := hot.Open(hot.Config{
		Addr:   firstNonEmpty(a.hotAddr, os.Getenv("ANC_HOT_ADDR"), hot.DefaultAddr),
		Prefix: firstNonEmpty(a.hotPrefix, os.Getenv("ANC_HOT_PREFIX")),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "注意：热层没接上（%v）—— 「派给我的待办」这一段只能报缺（那不是「没人派给你」）。\n", err)
		return ctxpack.TasksIn{}, noop
	}
	states, err := st.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "注意：热层读不出来（%v）—— 「派给我的待办」这一段只能报缺。\n", err)
		_ = st.Close()
		return ctxpack.TasksIn{}, noop
	}
	return ctxpack.TasksIn{Rows: states, Wired: true}, func() { _ = st.Close() }
}

// loadContextOrg 读真相源。读不了 = 红档，退出码 1（与「拒了」同一个号：都是没办成）。
func loadContextOrg(vault string) (*org.Org, int) {
	o, err := org.Load(vault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return nil, 1
	}
	printIssuesTo(os.Stderr, o.Warnings)
	return o, 0
}

// cmdContextPack 做投影，不做任何写操作 —— 也不留痕：
// 留痕是「一次行使」的账（谁向平台要了什么），那条账挂在 MCP 那个入口上；
// 这是操作者在自己的机器上打一份投影，跟 anc org export 一个性质。
func cmdContextPack(args []string) int {
	a, code := parseContextArgs("context pack", args, nil)
	if code != 0 {
		return code
	}
	o, code := loadContextOrg(a.vault)
	if code != 0 {
		return code
	}
	// domain 不清洗：清洗就发生在装配那一层（ctxpack.SafeToken），三个入口共用同一处，
	// 免得「只在一边生效」。
	tasks, done := readHotTasks(a)
	defer done()
	out, refused := ctxpack.Assemble(o, a.vault, a.who, "", a.domain, time.Now(), tasks)
	fmt.Print(mustJSON(out))
	if refused {
		// 拒不是「工具坏了」，但确实没答出来 —— 给脚本一个看得见的信号（与 MCP 的 isError 对齐）。
		if out.Refused != nil {
			fmt.Fprintln(os.Stderr, "没答出工作上下文："+out.Refused.Reason)
		}
		return 1
	}
	return 0
}

// cmdContextFreeze 把这一份上下文**冻住**：落成一个文件（默认 <vault>/context/<编号>.json）。
//
// 与 pack 共用同一个装配点、同一份序列化 —— 所以「stdout 上那一份」与「冻下来的那份」除末尾换行
// 外逐字节相同（有用例钉着）。这不是顺手，是纪律：对不上号的冻结副本比没有更坏。
func cmdContextFreeze(args []string) int {
	var to string
	a, code := parseContextArgs("context freeze", args, &to)
	if code != 0 {
		return code
	}
	o, code := loadContextOrg(a.vault)
	if code != 0 {
		return code
	}
	tasks, done := readHotTasks(a)
	defer done()
	out, refused := ctxpack.Assemble(o, a.vault, a.who, "", a.domain, time.Now(), tasks)
	if refused {
		fmt.Fprintln(os.Stderr, "没冻：这一份没答出来 —— 没有可交付的上下文，就没有可冻的东西。")
		if out.Refused != nil {
			fmt.Fprintln(os.Stderr, "  "+out.Refused.Reason)
		}
		fmt.Fprintln(os.Stderr, "  （CLI 这条不打痕：留痕挂在 MCP 那个入口上 —— agent 来要上下文被拒的那次才有痕，")
		fmt.Fprintln(os.Stderr, "   被拒的那次同样是「一次想行使」）")
		return 1
	}
	dir := to
	if dir == "" {
		dir = filepath.Join(a.vault, org.ContextDir)
	} else if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 落点建不出来：%v\n", err)
		return 1
	}
	// 文件名用**解出来的编号**（out.Who），不是 --who 的原样：真相源里那个拼法才是唯一的。
	// 它已经过 member.name 那套校验（不含 / \ : * ? " < > | 与控制字符），落成文件名是安全的。
	name := out.Who
	if out.Scope != "" {
		name += "." + out.Scope
	}
	path := filepath.Join(dir, name+".json")
	_, statErr := os.Stat(path)
	body := mustJSON(out) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 写不进落点：%v\n", err)
		return 1
	}
	if statErr == nil {
		fmt.Printf("已冻结 → %s（%d 字节，覆盖了原来那一份）\n", path, len(body))
	} else {
		fmt.Printf("已冻结 → %s（%d 字节）\n", path, len(body))
	}
	return 0
}
