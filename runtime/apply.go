package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"anc/internal/apply"
	"anc/internal/gateway"
	"anc/internal/org"
	"anc/internal/probe"
	renderpkg "anc/internal/render"
)

const applyUsage = `anc apply —— 把 org 变成一台机器上真在跑的东西（阶段 C）

用法：
  anc apply <vault 目录> [选项]

七步，顺序固定，失败即停：
  1 校验   org 真相源（红档即停，一个文件都不动）
  2 渲染   org → gateway config（与 anc render 同一套门禁）
  3 体检   产物引用的 ${ENV}，secrets.env 里齐不齐
  4 装载   cc-connect daemon install --no-capture-secrets --force
  5 凭据桥 把 secrets.env 读进 daemon 进程环境（上游没有 dotenv，这一步不能省）
  6 重启   cc-connect daemon restart --force
  7 回读   日志里 platform ready 到齐没有 + 只读探针

选项：
  --config <文件>   gateway config（默认 <vault>/../gateway/config.toml）
  --homes <目录>    每 bot 的家目录根（默认 <vault>/../homes）
  --data <目录>     gateway 的 data_dir（默认 <vault>/../data）
  --agent-home <slug>=<目录>   业务 agent 在本机的 cwd（可重复）—— **本机事实**，不进 git
  --secrets <文件>  凭据文件（默认 ~/.anc/secrets.env）
  --daemon <文件>   cc-connect 可执行文件（默认 ~/.anc/bin/ 或 PATH）
  --apply           真做（默认只打印这七步要看的东西）
  --adopt           接管没有 anc 指纹的现有 config（同 render）
  --allow-scale     允许本次新增 / 删除 project（同 render）
  --no-restart      装完不重启（改动留到下次重启生效）
  --wait <时长>     重启后等 platform ready 的窗口（默认 30s）

退出码：0 装载并起来了；1 校验 / 体检不过，或 daemon 没起来；2 用法错误

为什么「凭据桥」要单独一步：上游 daemon 的装载体没有 dotenv（实测 v1.3.4），
${ENV} 只从**进程环境**解析 —— 不注入它，daemon 会以「env var placeholder references
unset variable」「app_id and app_secret are required」起不来，而表面上引擎是绿的。

平台：装载这一步落了 Windows 腿（计划任务 + 接管上游生成的 ps1）与 Linux 腿
       （systemd 单元 + 我们自己那份 drop-in 里写 EnvironmentFile）。
       macOS 零覆盖（议题 #23）—— **未实现就明确报错，不假装成功**。
`

// applySteps 是七步的总数，只用于回显编号。
const applySteps = 7

func cmdApply(args []string) int {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	cfg := fs.String("config", "", "gateway config")
	homes := fs.String("homes", "", "每 bot 的家目录根")
	data := fs.String("data", "", "gateway data_dir")
	secrets := fs.String("secrets", "", "凭据文件")
	daemon := fs.String("daemon", "", "cc-connect 可执行文件")
	doApply := fs.Bool("apply", false, "真做")
	adopt := fs.Bool("adopt", false, "接管没有指纹的现有 config")
	allowScale := fs.Bool("allow-scale", false, "允许新增/删除 project")
	agentHome := &kvFlag{}
	fs.Var(agentHome, "agent-home", "业务 agent 的本机 cwd：<slug>=<目录>（可重复）")
	noRestart := fs.Bool("no-restart", false, "装完不重启")
	wait := fs.Duration("wait", 30*time.Second, "等 platform ready 的窗口")
	vault := fs.String("vault", "", "vault 目录（也可用位置参数）")
	flagArgs, posArgs := splitArgs(args, map[string]bool{
		"apply": true, "adopt": true, "allow-scale": true, "no-restart": true,
	})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	root := strings.TrimSpace(*vault)
	if root == "" && len(posArgs) > 0 {
		root = posArgs[0]
	}
	if root == "" {
		fmt.Fprint(os.Stderr, applyUsage)
		return 2
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 2
	}
	// 平台先卡：凭据桥是本命令唯一与平台绑死的一步，别的平台把前四步做完
	// 只会得到一个读不到凭据的 daemon —— 与其做一半，不如先说清楚。
	switch runtime.GOOS {
	case "windows", "linux":
	default:
		fmt.Fprintf(os.Stderr, "本平台（%s）的凭据桥没实现 —— 装载只落了 Windows 与 Linux 两腿。\n", runtime.GOOS)
		fmt.Fprintf(os.Stderr, "macOS 按议题 #23 零覆盖：不给没验过的平台发一张「看起来能跑」的票。\n")
		fmt.Fprintf(os.Stderr, "FIX: 在 Windows / Linux 上跑装载；macOS 的包装脚本见 runtime/DESIGN.md 阶段 C。\n")
		return 1
	}

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
	secretsPath := *secrets
	if secretsPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			return 2
		}
		secretsPath = filepath.Join(home, ".anc", "secrets.env")
	}
	ddir, err := daemonDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 2
	}

	mode := "dry-run（什么都不动；要真做加 --apply）"
	if *doApply {
		mode = "真做（--apply）"
	}
	fmt.Printf("anc apply —— org → 渲染 → 装载上游 daemon → 凭据桥 → 回读\n")
	fmt.Printf("  vault      %s\n", abs)
	fmt.Printf("  gateway    %s\n", cfgPath)
	fmt.Printf("  凭据       %s\n", secretsPath)
	fmt.Printf("  模式       %s\n", mode)

	step := func(n int, name string) {
		fmt.Printf("\n[%d/%d] %s\n", n, applySteps, name)
	}

	// ---- 1 校验 ----
	step(1, "校验    org 真相源")
	o, err := org.Load(abs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%v\n", err)
		return 1
	}
	fmt.Printf("  ✅ %s（启用 %d / 共 %d）\n", o.Company.Name, len(o.Enabled()), len(o.Members))
	printIssues(o.Warnings)

	// ---- 2 渲染 ----
	step(2, "渲染    org → gateway config")
	host := org.Host{VaultRoot: abs, HomesRoot: homesPath, DataDir: dataPath, AgentHomes: agentHome.m}
	plan, err := renderpkg.Build(o, renderpkg.Options{Host: host, Version: version, Now: time.Now()})
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n错误: %v\n", err)
		return 1
	}
	st := readState(cfgPath)
	fmt.Printf("  inputs     %s\n", plan.InputsHash)
	fmt.Printf("  project    %d 个：%s\n", len(plan.Projects), strings.Join(plan.Projects, ", "))
	fmt.Printf("  %s\n", driftLine(st, plan))
	printIssues(plan.Issues)
	for _, w := range plan.Warns {
		fmt.Printf("  ⚠️  %s\n", w)
	}
	if *doApply {
		if err := commitConfig(st, plan, o, host, renderGates{Adopt: *adopt, AllowScale: *allowScale}); err != nil {
			fmt.Fprintf(os.Stderr, "\n%v\n", err)
			return 1
		}
	} else {
		fmt.Printf("  （dry-run 不写）\n")
	}

	// ---- 3 体检：产物要哪些键、这份凭据文件里有没有 ----
	// 键名从 plan 自己的账上取，不从产物文本回读：2026-10-08 之前键名就是「成员名大写」，
	// 中文名拼出的键回读看不见，会把「这个 bot 要不到凭据」报成「✅ 齐」。
	// 现在键名是全函数派生（中文名也写得出合法键），账与回读一致 —— 但账仍是唯一出处。
	step(3, "体检    产物要求的 ${ENV}")
	refs := plan.SecretKeys
	rawSecrets, readErr := os.ReadFile(secretsPath)
	if readErr != nil {
		fmt.Printf("  🔴 读不到凭据文件：%v\n", readErr)
		fmt.Printf("FIX: 建这一份文件，一行一个 KEY=VALUE（权限收紧到只有本人 + SYSTEM）。\n")
		return 1
	}
	sf := apply.ParseSecrets(secretsPath, string(rawSecrets))
	fmt.Printf("  引用       %d 个：%s\n", len(refs), orNone(refs))
	fmt.Printf("  文件       %d 个键：%s\n", len(sf.Keys), orNone(sf.Keys))
	if len(sf.Bad) > 0 {
		fmt.Printf("  ⚠️  读不懂的行 %d 条（这几行里的键都等于没写）\n", len(sf.Bad))
		for _, b := range sf.Bad {
			fmt.Printf("    - %s\n", b)
		}
	}
	missing, blank := apply.Missing(refs, sf), sf.Blank()
	if len(missing)+len(blank) > 0 {
		// 这是本命令唯一的一处「拦」：装载一个必然起不来的 daemon，
		// 代价是**这台机器上原来在跑的 bot 全部下线**（install/restart 都会先杀掉旧实例）。
		fmt.Fprintf(os.Stderr, "  🔴 缺键 / 空值键：%s\n", strings.Join(append(append([]string{}, missing...), blank...), "、"))
		var unwritable []string
		for _, k := range missing {
			if !apply.LegalKey(k) {
				unwritable = append(unwritable, k)
			}
		}
		if len(unwritable) > 0 {
			// 「这个键只是没写」和「这个键根本写不出来」处置都是拦，但修法完全不同 ——
			// 不说清，现场会照着下面那条 FIX 去补一个永远补不上的键。
			// 2026-10-08 起「写不出来」在正常渲染里已经不可达（键名全函数派生，中文名也写得出键），
			// 走到这里只可能是产物被手改过、或派生逻辑坏了。
			fmt.Fprintf(os.Stderr, "  🔴 其中有 %s：不是合法的环境变量名，写不进凭据文件\n", strings.Join(unwritable, "、"))
			fmt.Fprintf(os.Stderr, "      键名的写法只有一处：render.FeishuSecretKey（ANC_FEISHU_SECRET_ + 名字的 ASCII 转义），只认 [A-Za-z_][A-Za-z0-9_]*。\n")
			fmt.Fprintf(os.Stderr, "FIX: 别手改产物 —— 重跑 `anc render`（或 `anc apply --apply`）按真相源重新生成。\n")
		}
		fmt.Fprintf(os.Stderr, "FIX: 往 %s 补齐这些键（明文只住这份文件，不要写进 config）。\n", secretsPath)
		return 1
	}
	fmt.Printf("  ✅ 齐\n")

	// ---- 4 装载上游 daemon ----
	step(4, "装载    上游 daemon（--no-capture-secrets）")
	// 装之前先看一眼装载体：上游 install（--force）每次都会把它整份重写回自己那份，
	// 所以「这次是首次注入 / 重注 / 内容变了」只能靠**装之前**的状态判断。
	// Linux 腿注入的是**我们自己**那份 drop-in，上游不碰它，所以那边不需要这一眼。
	var preLoader apply.Loader
	if runtime.GOOS == "windows" {
		if b, err := os.ReadFile(filepath.Join(ddir, gateway.CCConnect.LoaderScriptName())); err == nil {
			preLoader = apply.InspectLoader(string(b))
		}
	}
	daemonExe, err := resolveDaemonExe(*daemon)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n错误: %v\n", err)
		return 1
	}
	installArgs := gateway.CCConnect.InstallArgs(cfgPath)
	fmt.Printf("  执行       %s %s\n", daemonExe, strings.Join(installArgs, " "))
	if *doApply {
		if err := runDaemon(daemonExe, installArgs); err != nil {
			fmt.Fprintf(os.Stderr, "  🔴 装载失败：%v\n", err)
			return 1
		}
	}

	// ---- 5 凭据桥 ----
	step(5, "凭据桥  secrets.env → daemon 进程环境")
	if code := credentialBridge(secretsPath, version, *doApply, preLoader); code != 0 {
		return code
	}

	// ---- 6 重启 ----
	logPath := daemonLogFile(ddir)
	from := fileSize(logPath)
	step(6, "重启    上游 daemon（--force）")
	if *noRestart {
		fmt.Printf("  跳过       --no-restart（改动留到下次重启生效 —— 现在这份 daemon 还是旧的）\n")
		return 0
	}
	restartArgs := gateway.CCConnect.RestartArgs()
	fmt.Printf("  执行       %s %s\n", daemonExe, strings.Join(restartArgs, " "))
	if *doApply {
		if err := runDaemon(daemonExe, restartArgs); err != nil {
			fmt.Fprintf(os.Stderr, "  🔴 重启失败：%v\n", err)
			return 1
		}
	}

	// ---- 7 回读 ----
	step(7, "回读    platform ready + 只读探针")
	if !*doApply {
		fmt.Printf("  （dry-run 没重启，也就没得回读）\n")
		return 0
	}
	// 期望值只取**接了平台凭据**的 project（见 readyExpectation）：声明未接线的那些不该被等。
	expect := readyExpectation(plan, o)
	want := len(expect)
	ready, tail := waitReady(logPath, from, expect, *wait)
	if d := diagnose(tail); d != "" {
		fmt.Printf("  %s\n", d)
	}
	probeOpts := probe.Options{Vault: abs, Config: cfgPath, DataDir: dataPath, Admins: o.AdminLabels(), Unwired: renderpkg.UnwiredProjects(o)}
	if ready < want {
		fmt.Fprintf(os.Stderr, "  🔴 platform ready %d/%d（等了 %s）—— 日志：%s\n", ready, want, *wait, logPath)
		// 「没到 ready」至少两种病，光看日志分不出来：daemon 压根没跑，还是跑起来了但没到 ready。
		// 先拨一次 socket 再给 FIX —— 否则会把「没起来」误报成「凭据桥没生效」，
		// 照那句话去查是白花时间（2026-10-08 实测：Windows 上真因是任务被电源条件挡住，见议题 #42）。
		if rep, err := probe.Run(probeOpts); err == nil && rep.Gateway == "down" {
			fmt.Fprintf(os.Stderr, "  🔎 探针：gateway 没在跑（socket 拨不通）—— 病在「起没起来」，不在凭据。\n")
			// 「没起来」不止一种：问到「现在挡着它的那条」为止（议题 #42 要的那一步）。
			// 只读、不改上游任务；问不到就照实说问不到。
			for _, line := range whyDaemonDown(runtime.GOOS) {
				fmt.Fprintf(os.Stderr, "  %s\n", line)
			}
			fmt.Fprintf(os.Stderr, "FIX: %s\n", startupHint(runtime.GOOS))
			return 1
		}
		fmt.Fprintf(os.Stderr, "  🔎 探针：gateway 在跑，但 platform ready 没到齐 —— 这才该往凭据 / 配置方向查。\n")
		fmt.Fprintf(os.Stderr, "FIX: 确认第 5 步那个文件的托管区还在，再看日志最后几行说了什么。\n")
		return 1
	}
	if want == 0 {
		// 一个可期待的都没有：这时「到齐了」是空真 —— 空集上的全绿是最纯的那种假绿（同探针那条口径）。
		fmt.Printf("  ⚪ 这份产物里没有接了平台凭据的 project（都是声明未接线）—— 没有可期待的就绪，别把这当绿\n")
	} else {
		fmt.Printf("  🟢 platform ready %d/%d\n", ready, want)
	}
	// 探针只回显，不参与退出码：刚装完、还没人来过话的机器本来就该是黄档，
	// 把它算成失败等于让「刚装好」永远报错。
	rep, err := probe.Run(probeOpts)
	if err != nil {
		fmt.Printf("  ⚠️  探针读不动：%v\n", err)
	} else {
		mark := map[probe.State]string{probe.StateOK: "🟢", probe.StateWarn: "🟡", probe.StateFail: "🔴", probe.StateUnwired: "⚪"}
		for _, b := range rep.Bots {
			fmt.Printf("  %s %-18s %s\n", mark[b.State], b.Project, b.Why)
		}
		if !rep.AllGreen() {
			fmt.Printf("  详情：anc probe %s\n", abs)
		}
	}
	fmt.Printf("\n结论：装载完成，daemon 在跑。要不要人介入，看上面探针那一行。\n")
	return 0
}

// credentialBridge 是第 5 步的全部。两个平台的落点与载体不同，要做的事相同：
// 让**拉起 gateway 的那个进程**在启动前就拿到 secrets.env 里的键。
// 两边都写同一对边界标记、同一套指纹，回显与判据因此是同一个口径。
func credentialBridge(secretsPath, ver string, doApply bool, pre apply.Loader) int {
	if runtime.GOOS == "linux" {
		return bridgeLinux(secretsPath, ver, doApply)
	}
	return bridgeWindows(secretsPath, ver, doApply, pre)
}

// bridgeWindows 注入的是上游生成的 ~/.cc-connect/cc-connect-daemon.ps1
// （计划任务的 Action 就是它）。上游 install --force 会把它整份重写，所以注入必须幂等可重入。
func bridgeWindows(secretsPath, ver string, doApply bool, pre apply.Loader) int {
	ddir, err := daemonDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  🔴 %v\n", err)
		return 1
	}
	loaderPath := filepath.Join(ddir, gateway.CCConnect.LoaderScriptName())
	block := apply.LoaderBlock(secretsPath, ver)
	fmt.Printf("  目标       %s\n", loaderPath)
	if !doApply {
		// dry-run 不许依赖「只有 apply 之后才存在」的东西：第 4 步还没真跑，
		// 上游的装载体可能压根还没生成。所以这里只报落点与指纹，不去读它。
		fmt.Printf("  将注入     fp %s（dry-run 不读也不写）\n", apply.BlockFingerprint(block))
		return 0
	}
	orig, err := os.ReadFile(loaderPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  🔴 读不到上游装载体 %s：%v\n", loaderPath, err)
		fmt.Fprintf(os.Stderr, "FIX: 先让第 4 步真的装上一次（上游 install 会生成这个文件）。\n")
		return 1
	}
	out, _ := apply.InjectLoader(string(orig), block)
	if l := apply.InspectLoader(out); !l.Intact() {
		fmt.Fprintf(os.Stderr, "  🔴 注入后回读不一致：%+v\n", l)
		return 1
	}
	switch {
	case !pre.Found:
		fmt.Printf("  状态       首次注入\n")
	case pre.Intact() && pre.Actual == apply.BlockFingerprint(block):
		fmt.Printf("  状态       重注（上游把装载体重写回原样，托管区放回原位，内容与上次一致）\n")
	default:
		fmt.Printf("  状态       更新（上一次那段与这次不一致，或已被改过）\n")
	}
	fmt.Printf("  fp         %s\n", apply.BlockFingerprint(block))
	fmt.Printf("  说明       上游重装会整份重写这个文件 —— `anc apply` 会把它放回去（幂等）。\n")
	if err := os.WriteFile(loaderPath, []byte(out), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "  🔴 写装载体失败：%v\n", err)
		return 1
	}
	return 0
}

// bridgeLinux 注入的是**我们自己**那份 systemd drop-in，里面只写一行 EnvironmentFile
// 指向 secrets.env。上游生成的单元文件我们一个字节都不碰：它重装多少次都不影响这一份。
// reload 必须我们自己来：实测（Ubuntu 24.04 / systemd 255）上游 `daemon restart --force`
// **不会**重新读新写的 drop-in —— 不 reload 的话它在 systemd 眼里压根不存在，
// 表现是 daemon 起来了却报「引用了没定义的环境变量」。
func bridgeLinux(secretsPath, ver string, doApply bool) int {
	cands, err := systemdUnits()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  🔴 %v\n", err)
		return 1
	}
	unit := cands[0]
	if doApply {
		// 装完才去看它到底落在哪 —— 不猜。上游按 euid 选落点，我们只是把它找出来。
		found := false
		for _, c := range cands {
			if fi, err := os.Stat(c.Path); err == nil && !fi.IsDir() {
				unit, found = c, true
				break
			}
		}
		if !found {
			var tried []string
			for _, c := range cands {
				tried = append(tried, c.Path)
			}
			fmt.Fprintf(os.Stderr, "  🔴 找不到上游装的 systemd 单元。已试：\n  %s\n", strings.Join(tried, "\n  "))
			fmt.Fprintf(os.Stderr, "FIX: 第 4 步必须真的装上一次。上游在 Linux 上只走 systemd —— 没有用户会话它会直接拒绝。\n")
			return 1
		}
	}
	scope := "系统单元"
	if unit.User {
		scope = "用户单元"
	}
	target := apply.UnitDropInFile(unit.Path)
	block := apply.UnitBlock(secretsPath, ver)
	if doApply {
		fmt.Printf("  单元       %s（%s）\n", unit.Path, scope)
	} else {
		fmt.Printf("  单元       %s（%s，预计落点 —— 上游装完才存在）\n", unit.Path, scope)
	}
	fmt.Printf("  目标       %s\n", target)
	if !doApply {
		fmt.Printf("  将注入     fp %s（dry-run 不读也不写）\n", apply.BlockFingerprint(block))
		return 0
	}
	existing := ""
	var pre apply.Loader
	if b, err := os.ReadFile(target); err == nil {
		existing = string(b)
		pre = apply.InspectUnit(existing)
	}
	out, _ := apply.InjectLoader(existing, block)
	if l := apply.InspectUnit(out); !l.Intact() {
		fmt.Fprintf(os.Stderr, "  🔴 注入后回读不一致：%+v\n", l)
		return 1
	}
	switch {
	case !pre.Found:
		fmt.Printf("  状态       首次写入\n")
	case pre.Intact() && pre.Actual == apply.BlockFingerprint(block):
		fmt.Printf("  状态       重写（内容与上次一致）\n")
	default:
		fmt.Printf("  状态       更新（上一次那份与这次不一致，或已被改过）\n")
	}
	fmt.Printf("  fp         %s\n", apply.BlockFingerprint(block))
	fmt.Printf("  说明       drop-in 是 `anc apply` 自己的文件，上游重装不会碰它；systemd 按它把凭据读进服务环境。\n")
	if err := os.MkdirAll(apply.UnitDropInDir(unit.Path), 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "  🔴 建 drop-in 目录失败：%v\n", err)
		return 1
	}
	if err := os.WriteFile(target, []byte(out), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "  🔴 写 drop-in 失败：%v\n", err)
		return 1
	}
	if err := systemctlReload(unit.User); err != nil {
		fmt.Fprintf(os.Stderr, "  🔴 让 systemd 认这份 drop-in 失败：%v\n", err)
		fmt.Fprintf(os.Stderr, "FIX: 手工跑 `%s` 再重试。\n", reloadCmd(unit.User))
		return 1
	}
	fmt.Printf("  生效       %s（上游 restart 不认新 drop-in，必须自己 reload）\n", reloadCmd(unit.User))
	return 0
}

// reloadCmd 是让 systemd 认这份 drop-in 的那条命令。
func reloadCmd(user bool) string {
	if user {
		return "systemctl --user daemon-reload"
	}
	return "systemctl daemon-reload"
}

// systemctlReload 执行 daemon-reload。为什么不能省：见 bridgeLinux 上面那段。
func systemctlReload(user bool) error {
	args := []string{"daemon-reload"}
	if user {
		args = append([]string{"--user"}, args...)
	}
	cmd := exec.Command("systemctl", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// systemdUnit 是上游可能落单元的位置之一。
type systemdUnit struct {
	Path string
	User bool // true = 用户单元（~/.config/systemd/user），false = 系统单元（/etc/systemd/system）
}

// systemdUnits 列出两个可能落点，并按 euid 排序：上游就是这么选的
// （README：Linux root → systemd system service，Linux non-root → systemd user service），
// 所以把自己这一侧排前面 —— dry-run 就按第一个报「预计落点」。
func systemdUnits() ([]systemdUnit, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	unit := gateway.CCConnect.ServiceUnitName()
	user := systemdUnit{Path: filepath.Join(home, ".config", "systemd", "user", unit), User: true}
	sys := systemdUnit{Path: filepath.Join("/etc/systemd/system", unit)}
	if os.Geteuid() == 0 {
		return []systemdUnit{sys, user}, nil
	}
	return []systemdUnit{user, sys}, nil
}

// driftLine 一句话说清「这次渲染与现有配置差在哪」。与 `render --check` 同口径
// （比的就是同一份产物），只是不重复它那套回显。
func driftLine(st renderState, plan *renderpkg.Plan) string {
	if !st.HasFile {
		return "现有       目标不存在，将新建"
	}
	if !st.HasFP {
		return "现有       没有 anc 指纹（手写或他源配置）—— 要接管得加 --adopt"
	}
	oldHashes := renderpkg.PersonaHashesIn(string(st.Existing))
	var added, changed, removed []string
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
	if len(added)+len(changed)+len(removed) == 0 {
		return "现有       与上一轮一致（0 处漂移）"
	}
	var parts []string
	if len(added) > 0 {
		parts = append(parts, fmt.Sprintf("新增 %v", added))
	}
	if len(changed) > 0 {
		parts = append(parts, fmt.Sprintf("persona 有变 %v", changed))
	}
	if len(removed) > 0 {
		parts = append(parts, fmt.Sprintf("将删除 %v", removed))
	}
	return "现有       漂移：" + strings.Join(parts, "；")
}

// resolveDaemonExe 装载与重启都要它。找不到就明说试过哪些 —— 别让人猜。
func resolveDaemonExe(flagPath string) (string, error) {
	var cands []string
	if flagPath != "" {
		cands = append(cands, flagPath)
	}
	if home, err := os.UserHomeDir(); err == nil {
		cands = append(cands, gateway.CCConnect.BinCandidates(home)...)
	}
	for _, name := range gateway.CCConnect.BinNames {
		if p, err := exec.LookPath(name); err == nil {
			cands = append(cands, p)
		}
	}
	var tried []string
	for _, c := range cands {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		tried = append(tried, abs)
		if fi, err := os.Stat(abs); err == nil && !fi.IsDir() {
			return abs, nil
		}
	}
	return "", fmt.Errorf("找不到 cc-connect 可执行文件。已试：\n  %s\nFIX: 加 --daemon <路径>，或把它放进 ~/.anc/bin/",
		strings.Join(tried, "\n  "))
}

// daemonDir 是上游 daemon 的落点（一个账号一份）。
func daemonDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return gateway.CCConnect.HomeDir(home), nil
}

// daemonLogFile 从上游自己写的 daemon.json 里读日志落点 —— 不猜路径。
func daemonLogFile(dir string) string {
	def := filepath.Join(dir, gateway.CCConnect.LogSubdir, gateway.CCConnect.LogFileName)
	b, err := os.ReadFile(filepath.Join(dir, gateway.CCConnect.DaemonManifestName))
	if err != nil {
		return def
	}
	var m struct {
		LogFile string `json:"log_file"`
	}
	if json.Unmarshal(b, &m) != nil || m.LogFile == "" {
		return def
	}
	return m.LogFile
}

// runDaemon 跑上游命令并把它的输出原样放过去 —— 装载失败的原因全在它自己的输出里，不许吞。
func runDaemon(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// readyExpectation 是「这次重启**该**起来的那几个 project」—— 回读只拿它们当期望值。
//
// 真相源里声明了还没接线（`unwired`，含业务 agent 没写 `app_id`）的那些**不算期望**：
// 上游对它们的两种表现都实测过（2026-10-10，一次真 apply 里两种都撞到）——
//
//	要么 `platform ready` 紧跟着 `websocket error: app_id is invalid`（先绿后死）；
//	要么 `failed to create platform` 干脆不打 `platform ready`。
//
// 把这两种放进期望值，同一份部署会一次假绿、一次假红 —— 两次都不是证据。
// 与灰档共用 render.UnwiredProjects 这一支算：各算各的，就会出现「探针说灰、apply 回读说绿」。
func readyExpectation(plan *renderpkg.Plan, o *org.Org) map[string]bool {
	unwired := map[string]bool{}
	for _, n := range renderpkg.UnwiredProjects(o) {
		unwired[n] = true
	}
	expect := map[string]bool{}
	for _, n := range plan.Projects {
		if !unwired[n] {
			expect[n] = true
		}
	}
	return expect
}

// rePlatformReady 从一行上游日志里取出 `platform ready` 那个 project 名（logfmt）。
//
// 值带空格时上游会给它加引号（project 名 = `<公司 id>-<成员名>`，而成员名可以带空格），
// 所以两种写法都要认 —— 只认一种，带空格的名字就会静默数不到，回读又变回「按行数」。
var rePlatformReady = regexp.MustCompile(`msg="platform ready" project=(?:"([^"]*)"|(\S+))`)

// waitReady 盯着日志等 platform ready 到齐（每隔 0.5s 看一次新增的那段）。
func waitReady(logPath string, from int64, expect map[string]bool, timeout time.Duration) (int, string) {
	want := len(expect)
	deadline := time.Now().Add(timeout)
	for {
		ready, tail := countReady(logPath, from, expect)
		if ready >= want || !time.Now().Before(deadline) {
			return ready, tail
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// countReady 数的是**期望里的**那几个 project 里，日志报了几个 platform ready。
//
// 两件事一起管：
//
//	只数**重启之后新增**的那段 —— 旧的成功记录不算数，否则「这次没起来」会被上一次的绿字盖住；
//	只数**期望里的**那些名字 —— 见 readyExpectation，不是按 `platform ready` 的行数数。
//
// 注意这一行本身的分量：它只说明那个 project 的适配器**初始化完了**，不说明它真连上了
// （实测：同一条日志里先 `platform ready`、后 `websocket error`）。所以它是**必要不充分**的读数，
// 别把它当连通性证明 —— 真要判「这一秒能不能回话」，按拍板由人来判（同 probe 的口径）。
func countReady(logPath string, from int64, expect map[string]bool) (int, string) {
	f, err := os.Open(logPath)
	if err != nil {
		return 0, ""
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return 0, ""
	}
	if fi.Size() < from {
		from = 0 // 日志轮转 / 被截断
	}
	if _, err := f.Seek(from, io.SeekStart); err != nil {
		return 0, ""
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return 0, ""
	}
	tail := string(b)
	seen := map[string]bool{}
	for _, m := range rePlatformReady.FindAllStringSubmatch(tail, -1) {
		name := m[1]
		if name == "" {
			name = m[2]
		}
		if expect[name] {
			seen[name] = true
		}
	}
	return len(seen), tail
}

// diagnose 把上游日志里那两类「引擎绿但起不来」的指纹翻成人话。
func diagnose(tail string) string {
	switch {
	case strings.Contains(tail, "references unset variable"):
		return "⛔ 上游报「引用了没定义的环境变量」—— 凭据桥没生效（secrets.env 缺键，或托管区被上游重装冲掉了）"
	case strings.Contains(tail, "app_id and app_secret are required"):
		return "⛔ 上游报 app_id / app_secret 缺失 —— 同上，先看第 5 步那个文件还在不在"
	}
	return ""
}

// startupHint 给「daemon 没起来」这件事一句平台相关的下一步。
// 与 diagnose() 分工不同：diagnose 认的是**日志指纹**（引擎起来了、但凭据缺），
// 这里认的是**进程根本没起**（socket 拨不通）—— 两者要查的地方完全不同。
func startupHint(goos string) string {
	switch goos {
	case "windows":
		return "看计划任务状态 `schtasks /query /tn cc-connect /v /fo LIST`（定义里读得到的那几样，" +
			"上面那一组已经核过一遍）；也可以手工跑一次装载体 ~/.cc-connect/cc-connect-daemon.ps1，" +
			"看它自己报什么。"
	case "linux":
		return "看单元状态 `systemctl --user status cc-connect`。" +
			"刚写过 drop-in 时最容易少一次 daemon-reload；也可以手工跑一次 ~/.anc/bin/cc-connect，看它自己报什么。"
	}
	return "先确认 daemon 进程起没起来。"
}

func fileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

func orNone(list []string) string {
	if len(list) == 0 {
		return "（无）"
	}
	return strings.Join(list, ", ")
}
