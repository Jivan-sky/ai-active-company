package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"anc/internal/org"
	"anc/internal/probe"
	renderpkg "anc/internal/render"
)

const probeUsage = `anc probe —— 运行态探针（防假绿，只读）

用法：
  anc probe <vault 目录> [选项]

选项：
  --config <文件>   gateway config（默认 <vault>/../gateway/config.toml）
  --data <目录>     gateway data_dir（默认 <vault>/../data）
  --stale <时长>    多久没有成功交互就算「不新鲜」（默认 24h）
  --stall <时长>    「最后一轮没人回」超过多久算卡住（默认 10m）
  --json            给看板 / watchdog 消费的 JSON（打到 stdout）

为什么不是「看进程在不在」：
  2026-10-07 一天踩了两次 —— 进程活着、engine started 打着绿字、连接也建着，
  但每条消息都起不来 agent。所以判据只能是「真回话过没有」。

只读两样东西，都在磁盘上、跨平台、不烧 token：
  1. <data>/run/api.sock —— **真拨一次**，不看文件在不在
     （残留的 socket 文件会把「已经挂了」看成「在跑」）。
  2. <data>/sessions/*.json —— 会话事实（谁说过话、agent 起没起来、最后谁回的）。

四档：
  绿  窗口内有过**真实回复**（assistant 真回了话且内容非空），不是「引擎起来了」。
      空回复不算回过话：只调了工具、或异常返回空，都证明不了它能说话（议题 #51）。
  黄  没观测到 / 久无成功交互。「没消息 = 没事」是反模式，所以这一档不许报绿。
  红  有消息但 agent 起不来，或最后一轮迟迟不回。
  灰  **还没接平台凭据**（真相源里声明 unwired: true）—— 本来就不该期待它回话，
      不计绿也不计黄。绿的口径因此是「**已接凭据的都绿了**」，不是「所有 bot 都绿了」。

裁判权留给人工：探针只观测、只报警，**不做任何自动处置**。
报红时会连带说清「该交给谁」（真相源里的 company.admins）。

退出码：0 已接凭据的全绿（且至少有一个已接的）；1 有黄 / 红 / 残留，或**一个绿的都没有**；
       2 用法错误。灰不参与计数 —— 没接凭据的不是事故，但「全都没接」也不是绿。
`

func cmdProbe(args []string) int {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	cfg := fs.String("config", "", "gateway config")
	data := fs.String("data", "", "gateway data_dir")
	stale := fs.Duration("stale", probe.DefaultStale, "多久没有成功交互算不新鲜")
	stall := fs.Duration("stall", probe.DefaultStall, "最后一轮没人回多久算卡住")
	asJSON := fs.Bool("json", false, "输出 JSON")
	vault := fs.String("vault", "", "vault 目录（也可用位置参数）")
	flagArgs, posArgs := splitArgs(args, map[string]bool{"json": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	root := strings.TrimSpace(*vault)
	if root == "" && len(posArgs) > 0 {
		root = posArgs[0]
	}
	if root == "" {
		fmt.Fprint(os.Stderr, probeUsage)
		return 2
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 2
	}
	cfgPath := *cfg
	if cfgPath == "" {
		cfgPath = filepath.Join(filepath.Dir(abs), "gateway", "config.toml")
	}
	dataPath := *data
	if dataPath == "" {
		dataPath = filepath.Join(filepath.Dir(abs), "data")
	}

	opt := probe.Options{Vault: abs, Config: cfgPath, DataDir: dataPath, Stale: *stale, Stall: *stall}
	// 「该交给谁」来自真相源，是组织知识 —— 留在 CLI 侧，不进 probe 包（包不认识 org）。
	// 读不动就**不猜** —— 一份红报告配上猜错的责任人更糟。
	handlerWhy := ""
	if o, err := org.Load(abs); err == nil {
		opt.Admins = o.AdminLabels()
		// 「还没接凭据」也是真相源里的**声明**（成员上的 `unwired: true`），同样不猜。
		opt.Unwired = renderpkg.UnwiredProjects(o)
		if len(opt.Admins) == 0 {
			handlerWhy = "真相源里 company.admins 是空的 —— 现在没人可交"
		}
	} else {
		handlerWhy = "真相源现在读不动，算不出该交给谁（不猜）：" + firstLine(err.Error())
	}

	rep, err := probe.Run(opt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		fmt.Fprintf(os.Stderr, "FIX: 先 `anc render %s --apply`\n", abs)
		return 1
	}
	rep.HandlerWhy = handlerWhy
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
	} else {
		printProbe(rep)
	}
	if rep.AllGreen() {
		return 0
	}
	return 1
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func printProbe(rep probe.Report) {
	mark := map[probe.State]string{probe.StateOK: "🟢", probe.StateWarn: "🟡", probe.StateFail: "🔴", probe.StateUnwired: "⚪"}
	fmt.Printf("anc probe —— 运行态探针（只读）\n")
	fmt.Printf("  vault    %s\n", rep.Vault)
	fmt.Printf("  config   %s\n", rep.Config)
	fmt.Printf("  data     %s\n", rep.DataDir)
	if rep.Gateway == "up" {
		fmt.Printf("  gateway  🟢 在跑（socket 拨得通）\n")
	} else {
		fmt.Printf("  gateway  🔴 %s\n", rep.GatewayWhy)
	}
	if len(rep.Handlers) > 0 {
		fmt.Printf("  该交给    %s\n", strings.Join(rep.Handlers, "、"))
	} else if rep.HandlerWhy != "" {
		fmt.Printf("  该交给    ⚠️ %s\n", rep.HandlerWhy)
	}
	fmt.Println()
	if len(rep.Bots) == 0 {
		fmt.Println("  （config 里一个 project 都没有）")
	}
	for _, f := range rep.Bots {
		fmt.Printf("  %s %-18s %s\n", mark[f.State], f.Project, f.Why)
	}
	ok, warn, fail, unwired := rep.Tally()
	if unwired > 0 {
		fmt.Printf("\n  口径：已接凭据的 %d 个 —— %d 绿 / %d 黄 / %d 红；另 %d 个声明了未接凭据（不计绿也不计黄）。\n",
			ok+warn+fail, ok, warn, fail, unwired)
	}
	if ok == 0 {
		fmt.Println("\n⚠️  一个绿的都没有 —— 这不是全绿。什么都没验过（全被声明成未接凭据，或 config 里没有 project），")
		fmt.Println("    所以退出码给 1：空集上的「都绿了」是空真，把它当健康是最纯的那种假绿。")
	}
	if len(rep.Extras) > 0 {
		fmt.Println("\n⚠️  配置外的残留（不是运行态问题，但该清）")
		for _, e := range rep.Extras {
			fmt.Printf("  - %s\n", e)
		}
	}
	fmt.Println()
	fmt.Println("口径：这里只读磁盘事实（socket 拨通 + 会话记录），不烧 token、不做任何自动处置。")
	fmt.Println("      真的「现在这一秒能不能回话」，按拍板由**人工**判定 —— 探针不代劳。")
}
