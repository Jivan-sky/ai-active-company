package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"anc/internal/org"
)

const contextUsage = `anc context pack <vault> --who <编号> [--domain <slug>]

  一个 agent 的**工作上下文**：身份 / 业务域 / 数据源 / 可见范围 / 技能来源 / 在跟的项目。

  这份是**算出来的只读投影**，不是新的真相源 —— 真相源仍是 vault 里的 markdown。
  它与 harness 走 MCP 调的 anc_read_context **同一个装配点**（assembleContext）：两个出口，一份上下文。

  --who <编号>     成员名（members/<名>）或业务 agent 的 slug（agents.md）
  --domain <slug>  只要这一块业务；不写 = 它负责的全部域（外加「找谁」目录）

  只读：一个字不写回真相源，也**不落盘** —— 要一份冻结副本就自己重定向到文件
  （同 anc org export 的规矩：投影是纯函数产物，存下来就成第三处会漂移的源）。

  退出码：0 = 答出来了 · 1 = 拒了（这个人不存在 / 越域 / on_behalf_of 解不出），或真相源读不了 · 2 = 用法错
`

// cmdContext 按需生成一份工作上下文（议题 #53）。
//
// 为什么是「按需生成」而不是落盘常贮：真相源 markdown · 留痕 jsonl 已经两处了，
// 再把装配产物存一份，就是第三处会漂移的源。谁要一份冻结的，自己接着（重定向、进交付包、贴议题）。
func cmdContext(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, contextUsage)
		return 2
	}
	switch args[0] {
	case "pack":
		return cmdContextPack(args[1:])
	}
	fmt.Fprint(os.Stderr, contextUsage)
	return 2
}

// cmdContextPack 做投影，不做任何写操作 —— 也不留痕：
// 留痕是「一次行使」的账（谁向平台要了什么），那条账挂在 MCP 那个入口上；
// 这是操作者在自己的机器上打一份投影，跟 anc org export 一个性质。
func cmdContextPack(args []string) int {
	fs := flag.NewFlagSet("context pack", flag.ContinueOnError)
	who := fs.String("who", "", "编号：成员名或业务 agent 的 slug（必填）")
	domain := fs.String("domain", "", "只要这一块业务（slug）；不写 = 它负责的全部域")
	flagArgs, posArgs := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) != 1 || *who == "" {
		fmt.Fprint(os.Stderr, contextUsage)
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
	printIssuesTo(os.Stderr, o.Warnings)

	// domain 过一遍与 MCP 那条路同一个清洗：两条出口的入参口径要一样，免得只在一边生效。
	out, refused := assembleContext(o, abs, *who, "", safeToken(*domain), time.Now())
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
