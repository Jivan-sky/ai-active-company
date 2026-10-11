package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	iofs "io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	"anc/internal/board"
	"anc/internal/hot"
	"anc/internal/org"
)

const boardServeUsage = `anc board serve —— 起 ANC 看板（默认只读；唯一的写口是审批信号）

用法：
  anc board serve <vault 目录> [--addr 127.0.0.1:8787] [--data <data 目录>]
                                 [--hot-addr 127.0.0.1:6379] [--hot-prefix anc]
                                 [--ui-dir <前端目录>]

看板读的是真相源（vault）的**实时投影**：改完 vault 刷新即见，不缓存。
前端（手写的 html + 构建期打出的 js）默认已经嵌在这个二进制里 —— 不需要 node、不需要外网。
现场要换一份前端（改样式、补一页、拿真机产物核界面）用 --ui-dir 指过去，不必重编译。

选项：
  --addr <host:port>   监听地址（默认只绑本机 127.0.0.1；绑到别处会打警告）
  --data <目录>        gateway 的 data 目录（默认 <vault>/../data）—— 给了它，「运行态」
                       页才接得上；默认口径同 anc probe 命令，两边看的是同一份现场。
  --hot-addr <地址>    热层地址（「待批」页用）；默认 127.0.0.1:6379，也可用 ANC_HOT_ADDR。
                       接不上**不拦看板**：那页会如实报「没接上」，不拿空队列冒充「没人提」。
  --hot-prefix <前缀>  热层 key 前缀；默认 anc，也可用 ANC_HOT_PREFIX。
  --ui-dir <目录>     前端静态资源目录；默认用**内嵌的那份**。指到磁盘上的一份就覆盖它 ——
                       这一项只换「前端文件从哪来」，不动路由与只读口径。目录写错**直接退**。

安全口径：
  - **默认只读**：只放行 GET / HEAD。唯一的写口是审批信号 POST /api/approvals/decide ——
    SPEC §7：CLI / 对话式 bot / 看板是**同一条变更管道的三个前端**；那条路只留痕，不写真相源。
  - 那条写口只收 POST + application/json（跨站表单发不出这个 Content-Type，浏览器会先发预检，
    而我们不回 CORS 头 —— 所以没有 token 也挡得住「别的网页替你点头」）；请求体限 64KB。
  - **不校「点的人是不是该批的人」**：请求里带的 by 原样留痕，判断交给 agent。
  - 默认只绑本机 —— 看板里有组织与项目信息，要给别人看请走 ssh 端口转发，
    别把 0.0.0.0 直接开出去；
  - 投影里不含凭据面字段（feishu app_id / open_id 等一律不进）。

退出：Ctrl-C。
`

// cmdBoardServe 起看板（默认只读）。它不写真相源、不装载服务 —— 与阶段 C 的 `anc serve` 不是同一件事。
// 唯一的写口是审批信号：那条路与 CLI 同一个入口，只留痕（见 internal/board/approvals.go）。
func cmdBoardServe(args []string) int {
	fs := flag.NewFlagSet("board serve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8787", "监听地址")
	data := fs.String("data", "", "gateway data 目录（运行态页用）")
	hotAddr := fs.String("hot-addr", "", "热层地址（待批页用；默认 "+hot.DefaultAddr+"；也可用 ANC_HOT_ADDR）")
	hotPrefix := fs.String("hot-prefix", "", "热层 key 前缀（默认 "+hot.DefaultPrefix+"；也可用 ANC_HOT_PREFIX）")
	uiDir := fs.String("ui-dir", "", "前端静态资源目录（默认用内嵌的那份）")
	flagArgs, posArgs := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) != 1 {
		fmt.Fprint(os.Stderr, boardServeUsage)
		return 2
	}
	abs, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	// 路径写错就别起服务（起一个只会 500 的服务是骗人）；内容有问题则照起 ——
	// 看板正是用来看「哪里坏了」的，前端会把红档摆在最上面。
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		fmt.Fprintf(os.Stderr, "错误：vault 目录不存在或不是目录：%s\n", abs)
		return 1
	}
	// 运行态目录：默认与 `anc probe` 同一口径（<vault>/../data）。
	// 目录不在**不拦**看板 —— 真相源投影仍然要看得到；运行态页会自己报「没接上」。
	dataDir := *data
	if dataDir == "" {
		dataDir = filepath.Join(filepath.Dir(abs), "data")
	} else if a, err := filepath.Abs(dataDir); err == nil {
		dataDir = a
	}

	// 前端：默认用内嵌的那份（一个 exe 就起来）；--ui-dir 指到磁盘上的一份就**覆盖**它。
	// 目录写错**直接退**：起一个打开只会 404 前端的服务，看到的是「界面坏了」，
	// 真原因是路径写错 —— 这种假故障最费人（口径同上面的 vault）。
	var uiFS iofs.FS
	if *uiDir != "" {
		dir, err := filepath.Abs(*uiDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误：%v\n", err)
			return 2
		}
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			fmt.Fprintf(os.Stderr, "错误：--ui-dir 不是目录：%s\n", dir)
			return 1
		}
		uiFS = os.DirFS(dir)
		*uiDir = dir
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：监听 %s 失败：%v\n", *addr, err)
		return 1
	}
	defer func() { _ = ln.Close() }()

	if o, err := org.Load(abs); err == nil {
		printIssues(o.Warnings)
	} else {
		fmt.Fprintf(os.Stderr, "⚠️  真相源现在有红档发现（看板会把它摆在最上面）：\n%v\n", err)
	}

	urlHost := ln.Addr().String()
	if tcp, ok := ln.Addr().(*net.TCPAddr); ok {
		if tcp.IP.IsUnspecified() {
			urlHost = net.JoinHostPort("127.0.0.1", strconv.Itoa(tcp.Port))
		}
		if !tcp.IP.IsLoopback() {
			fmt.Fprintf(os.Stderr,
				"⚠️  绑到了 %s：看板里有组织与项目信息，只该给本机、或经 ssh 端口转发的人看。\n", ln.Addr())
		}
	}

	// 热层：接得上就把「待批」页接上；接不上**不拦看板**（同 data 目录的口径）——
	// 看不到待批不等于真相源坏了，投影照样该看得到。
	bs := &board.Server{Vault: abs, DataDir: dataDir, Now: time.Now, UIFS: uiFS}
	if st, err := hot.Open(hot.Config{
		Addr:   firstNonEmpty(*hotAddr, os.Getenv("ANC_HOT_ADDR"), hot.DefaultAddr),
		Prefix: firstNonEmpty(*hotPrefix, os.Getenv("ANC_HOT_PREFIX")),
	}); err != nil {
		fmt.Fprintf(os.Stderr, "注意：热层没接上（%v）—— 「待批」与「沉淀」两页都会如实报没接上。\n", err)
	} else {
		defer st.Close()
		bs.Approvals = st
		bs.Hot = st
	}

	fmt.Printf("ANC 看板已起（默认只读）：\n")
	fmt.Printf("  地址   http://%s\n", urlHost)
	fmt.Printf("  vault  %s\n", abs)
	if uiFS == nil {
		fmt.Printf("  前端   内嵌（--ui-dir 可换成磁盘上的一份）\n")
	} else {
		fmt.Printf("  前端   %s（来自 --ui-dir，覆盖内嵌）\n", *uiDir)
	}
	if st, err := os.Stat(dataDir); err == nil && st.IsDir() {
		fmt.Printf("  data   %s（运行态页已接入）\n", dataDir)
	} else {
		fmt.Printf("  data   %s（没有这个目录 —— 运行态页会报「没接上」）\n", dataDir)
	}
	if bs.Approvals == nil {
		fmt.Printf("  待批   没接上（--hot-addr 或 ANC_HOT_ADDR）\n")
	} else {
		fmt.Printf("  待批   http://%s/#/approvals（热层已接上）\n", urlHost)
	}
	fmt.Printf("  写口   只有一处：POST http://%s%s（审批信号；只留痕，不写真相源）\n", urlHost, board.WritePath)
	fmt.Printf("  退出   Ctrl-C\n\n")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	srv := &http.Server{
		Handler:           bs.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case <-ctx.Done():
		fmt.Println("\n收到中断，正在关闭看板…")
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			fmt.Fprintf(os.Stderr, "关闭时出错：%v\n", err)
			return 1
		}
		return 0
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
}
