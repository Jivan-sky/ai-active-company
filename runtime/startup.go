package main

import (
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"unicode/utf16"

	"anc/internal/gateway"
)

// ---------- 断电兜底：断电 / 重启之后，这东西会不会自己回来 ----------
//
// 与 anc probe 的分工：probe 看**现在**通不通话（socket + 会话事实）；
// 这一组看的是**下一次机器自己起来**时谁把它拉回来 —— 只读，不替客户改上游默认，
// 也不设门禁（doctor 的结论行本来就写着「⚠️ 不一定是阻断项」）。
//
// Linux 三条前提缺一条都不成立：单元存在 + 已 enable + Linger=yes；
// Windows 看触发器里有没有「开机就起」那一条，再看两条电源设置。
// 口径与三层实测见 runtime/DESIGN.md §7.1.11。

// 计划任务名取自网关形态表（SPEC §4.7 G3）—— 不再写死一家。
var startupTaskName = gateway.CCConnect.ServiceName

// taskFacts 是计划任务定义里与「起得来吗」有关的那几样。
type taskFacts struct {
	Triggers      []string
	NoStartOnBatt bool
	StopOnBatt    bool
	Enabled       string // 定义里 <Enabled> 的原样文本："" = 没写（默认启用）/ "true" / "false"
}

// Disabled 认「这份定义把任务停用了」。
//
// 为什么读**定义**不读运行时状态：`Get-ScheduledTask` 的 State 跟着系统语言翻译
// （中文机上叫「已禁用」），而 XML 里这一格永远是 true / false —— 判据不许被系统语言带走。
// 2026-10-16 本机实测：这台开发机上的 cc-connect 任务就是 `Enabled=false`，
// 而在此之前 doctor 只看触发器与电源条件，**从来没说过这一条**（一个真因一直没人报）。
func (f taskFacts) Disabled() bool { return strings.EqualFold(strings.TrimSpace(f.Enabled), "false") }

// parseTaskXML 只取三样东西：触发器种类、两条电源设置。
func parseTaskXML(b []byte) (taskFacts, error) {
	var f taskFacts
	var doc struct {
		Settings struct {
			DisallowStartIfOnBatteries bool   `xml:"DisallowStartIfOnBatteries"`
			StopIfGoingOnBatteries     bool   `xml:"StopIfGoingOnBatteries"`
			Enabled                    string `xml:"Enabled"`
		} `xml:"Settings"`
		Triggers struct {
			Any []struct {
				XMLName xml.Name
			} `xml:",any"`
		} `xml:"Triggers"`
	}
	dec := xml.NewDecoder(strings.NewReader(toUTF8(b)))
	// 编码声明在 toUTF8 里已经改掉了；这个 CharsetReader 什么都不做，只是兜一层：
	// 哪天声明又变回别的名字，也不至于整条判据哑成「读不懂」。
	dec.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	if err := dec.Decode(&doc); err != nil {
		return f, err
	}
	for _, t := range doc.Triggers.Any {
		f.Triggers = append(f.Triggers, t.XMLName.Local)
	}
	f.NoStartOnBatt = doc.Settings.DisallowStartIfOnBatteries
	f.StopOnBatt = doc.Settings.StopIfGoingOnBatteries
	f.Enabled = doc.Settings.Enabled
	return f, nil
}

// toUTF8 把 schtasks 交出来的 UTF-16 转成 UTF-8。顺手把 XML 头里的编码声明改成
// UTF-8：声明留着不改，encoding/xml 会因为「不认这个编码又没有 CharsetReader」直接报错。
func toUTF8(b []byte) string {
	s := string(b)
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		u := make([]uint16, 0, len(b)/2)
		for i := 2; i+1 < len(b); i += 2 {
			u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
		}
		s = string(utf16.Decode(u))
	}
	// 声明一定要改成 UTF-8 —— 留在那儿不改，encoding/xml 会因为「不认这个编码」直接报错。
	// 无 BOM 那条路（schtasks 真换成输出 UTF-8 时）同样要改：实测漏掉它就整条读不懂。
	return strings.Replace(s, `encoding="UTF-16"`, `encoding="UTF-8"`, 1)
}

// hasBootTrigger：触发器里有没有一条「机器起来就起、不用人登录」的。
// 只认 BootTrigger —— 定时 / 事件触发器也能不靠人，但那是另一回事，不许捎带算过。
func hasBootTrigger(ts []string) bool {
	for _, t := range ts {
		if t == "BootTrigger" {
			return true
		}
	}
	return false
}

// parseLinger 认 loginctl -p Linger 那一行（`Linger=yes`）。
func parseLinger(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok && k == "Linger" {
			return strings.EqualFold(strings.TrimSpace(v), "yes")
		}
	}
	return false
}

// startupChecks 是「断电兜底」这一组；没覆盖的平台返回 nil（不许空手报错）。
func startupChecks() []check {
	switch runtime.GOOS {
	case "windows":
		return windowsStartupChecks()
	case "linux":
		return linuxStartupChecks()
	}
	return nil
}

// windowsTaskFacts 读一次计划任务定义。第二个返回值是「装没装」：
// false = 任务不在（还没 apply），此时 taskFacts 是零值 —— 与「装了但读不懂」（err 非空）分开，
// 那是两件事：前者该跳过，后者该报红。
func windowsTaskFacts() (taskFacts, bool, error) {
	out, err := exec.Command("schtasks", "/query", "/tn", startupTaskName, "/xml", "ONE").Output()
	if err != nil {
		return taskFacts{}, false, nil
	}
	f, perr := parseTaskXML(out)
	return f, true, perr
}

// windowsStartupChecks：任务还没装就明确跳过 —— 不给一台刚 init 的机器平添红字。
func windowsStartupChecks() []check {
	f, installed, err := windowsTaskFacts()
	if !installed {
		return []check{{"自启兜底", true, "计划任务 " + startupTaskName + " 不在（还没 apply）—— 跳过"}}
	}
	if err != nil {
		return []check{{"自启兜底", false, "读不懂任务定义：" + err.Error()}}
	}
	triggers := strings.Join(f.Triggers, " + ")
	var cs []check
	// 先看最硬的一条：定义里写着停用，触发器再全也没人拉得起来（2026-10-16 本机命中的正是它）。
	if f.Disabled() {
		cs = append(cs, check{"任务启用", false, "定义里 Enabled=false —— **任务被停用了**，谁也拉不起来。" +
			"启用：schtasks /change /tn " + startupTaskName + " /enable"})
	} else {
		cs = append(cs, check{"任务启用", true, "定义里没写「停用」"})
	}
	if hasBootTrigger(f.Triggers) {
		cs = append(cs, check{"自启兜底", true, "触发器 " + triggers + " —— 断电重启后不等人登录"})
	} else {
		cs = append(cs, check{"自启兜底", false, "触发器只有「" + triggers + "」—— 断电重启后没人登录就不会起（议题 #42）" +
			"；补一条开机触发器 = 改上游默认（要么以 SYSTEM 跑、要么存下口令），要先拍，见 DESIGN §7.1.11"})
	}
	if f.NoStartOnBatt || f.StopOnBatt {
		cs = append(cs, check{"电源条件", false, "电池供电不启动 / 拔电就停 —— 笔记本上它一直起不来（议题 #42；服务器没有电池，这条不适用）"})
	} else {
		cs = append(cs, check{"电源条件", true, "没设「电池时不启动 / 拔电就停」"})
	}
	return cs
}

// findSystemdUnit 找现成的那份单元（两个落点按 euid 排过序，取第一个存在的）。
// 「兜底」与「现在为什么没起来」两支都要找它，就只留这一处 —— 各找一遍，迟早两处认的不是同一份。
func findSystemdUnit() (systemdUnit, bool, error) {
	units, err := systemdUnits()
	if err != nil {
		return systemdUnit{}, false, err
	}
	for _, c := range units {
		if _, err := os.Stat(c.Path); err == nil {
			return c, true, nil
		}
	}
	return systemdUnit{}, false, nil
}

// linuxStartupChecks：装着哪份单元、它会不会自己起。
func linuxStartupChecks() []check {
	u, found, err := findSystemdUnit()
	if err != nil {
		return []check{{"自启兜底", false, err.Error()}}
	}
	if !found {
		return []check{{"自启兜底", true, "systemd 单元 " + startupTaskName + " 不在（还没 apply）—— 跳过"}}
	}
	cs := []check{{"单元", true, u.Path}}
	args := []string{"is-enabled", startupTaskName}
	if u.User {
		args = append([]string{"--user"}, args...)
	}
	// disabled 会带退出码 1，而且提示语在 stderr：看字，不看码。
	out, _ := exec.Command("systemctl", args...).CombinedOutput()
	st := strings.TrimSpace(string(out))
	if st == "" {
		st = "(空)"
	}
	if strings.HasPrefix(st, "enabled") {
		cs = append(cs, check{"开机自启", true, "is-enabled: " + st})
	} else {
		cs = append(cs, check{"开机自启", false, "is-enabled: " + st + " —— 开机不会自己起"})
	}
	if !u.User {
		return cs // 系统单元不归 linger 管
	}
	cu, err := user.Current()
	if err != nil {
		return cs
	}
	lout, err := exec.Command("loginctl", "show-user", cu.Username, "-p", "Linger").Output()
	if err != nil {
		cs = append(cs, check{"Linger", false, "查不到（" + err.Error() + "）—— 没人登录时用户级 systemd 不会起"})
		return cs
	}
	if parseLinger(string(lout)) {
		cs = append(cs, check{"Linger", true, cu.Username + " = yes（不等人登录）"})
	} else {
		cs = append(cs, check{"Linger", false, "Linger=no —— 没人登录时用户级 systemd 不会起。" +
			"补：sudo loginctl enable-linger " + cu.Username})
	}
	return cs
}

// ---------- 「现在为什么没起来」（议题 #42 的另一半）----------
//
// 与上面那一组的分别：上面看的是**下次断电**会不会自己回来（兜底）；这一组看的是**此刻**为什么没起来。
// 两批事实有重叠，但结论必须分开说 —— 把「下一次的风险」当成「这一次的原因」，与把「起没起来」
// 当成「凭据缺」是同一种错：都把人指到一个不是真因的地方。议题 #42 要的正是这一条：
// 「回读要能认出来并明说」（没装 / 被条件挡 / 起来了又死 / 凭据缺）。
//
// 全部只读：不改任务、不改配置、不动上游默认。

// liveFacts 是「此刻这台机器」的两样读数 —— 与任务定义无关，每次现问。
type liveFacts struct {
	PowerKnown  bool   // 供电读到了没
	OnBattery   bool   // 现在靠电池跑（PowerKnown 为假时这一格无意义）
	ProcKnown   bool   // 进程问到了没
	ProcRunning bool   // daemon 进程在不在
	Why         string // 读不到时的原因（说给人听，不编）
}

// liveNow 是包级缝：用例换掉它，不必真去问电源与进程（同 readHotTasks 那条做法）。
var liveNow = func() liveFacts { return readLiveFacts(runtime.GOOS) }

// readLiveFacts 问一次「此刻」。读不到的**不猜** —— PowerKnown / ProcKnown 就是给这件事留的：
// 猜一个「在用外接电源」，会把「被电源条件挡住」判成「不是它」，正好把真因盖掉。
func readLiveFacts(goos string) liveFacts {
	switch goos {
	case "windows":
		// 一次 PowerShell 问两样：供电（`PowerLineStatus` = Online / Offline，与系统语言无关）
		// 与进程。进程那条要排掉**自己**：这条命令的正文里就含 `cc-connect-daemon.ps1`，
		// 不排 $PID 就会把自己数进去 —— 2026-10-16 实测踩到过，结果是 `PROC=1` 的假绿。
		const script = `Add-Type -AssemblyName System.Windows.Forms; ` +
			`'POWER=' + [System.Windows.Forms.SystemInformation]::PowerStatus.PowerLineStatus; ` +
			`$me=$PID; $p=Get-CimInstance Win32_Process -ErrorAction SilentlyContinue | Where-Object { ` +
			`$_.ProcessId -ne $me -and ( $_.Name -like 'cc-connect*' -or $_.CommandLine -like '*cc-connect-daemon.ps1*' ) }; ` +
			`if($p){'PROC=1'}else{'PROC=0'}`
		// 重定向出来的可能是 UTF-16（schtasks 就是这样，见 toUTF8）—— 同一支解码，不另写一份。
		out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
		if err != nil {
			return liveFacts{Why: "问不到：" + err.Error()}
		}
		var lf liveFacts
		for _, line := range strings.Split(toUTF8(out), "\n") {
			line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
			switch {
			case line == "POWER=Online":
				lf.PowerKnown, lf.OnBattery = true, false
			case line == "POWER=Offline":
				lf.PowerKnown, lf.OnBattery = true, true
			case line == "PROC=1":
				lf.ProcKnown, lf.ProcRunning = true, true
			case line == "PROC=0":
				lf.ProcKnown, lf.ProcRunning = true, false
			}
		}
		if !lf.PowerKnown && lf.Why == "" {
			lf.Why = "这台机器没报「接不接外电」（台式机 / 虚拟机可能没有这一格）"
		}
		return lf
	case "linux":
		// 供电这一格 Linux 上不查：笔记本要看 acpi、容器里根本没有 —— 与其猜，不如说「没这一格」
		// （systemd 也没有 Windows 那两条电源条件，这一格在 Linux 上本来就用不上）。
		// 进程：`pgrep -f` 只认「命令行里有这个串」的进程，且它自己不算在里面。
		lf := liveFacts{Why: "Linux 上不查供电（没有 Windows 那两条电源条件，也不需要这一格）"}
		out, err := exec.Command("pgrep", "-f", "cc-connect").Output()
		if err == nil || len(out) > 0 {
			lf.ProcKnown, lf.ProcRunning = true, strings.TrimSpace(string(out)) != ""
		}
		return lf
	}
	return liveFacts{Why: "这个平台还没写「此刻」那一路读数"}
}

// whyDaemonDown 给 `anc apply` 第 7 步用：gateway 拨不通时，把「为什么没起来」问到这一步。
// 它只做「取事实」，判据在 daemonDownLines 里（纯函数，用例直接喂）。
func whyDaemonDown(goos string) []string {
	lf := liveNow()
	switch goos {
	case "windows":
		f, installed, _ := windowsTaskFacts()
		return daemonDownLines(goos, installed, f, lf, "")
	case "linux":
		u, found, _ := findSystemdUnit()
		state := ""
		if found {
			args := []string{"is-active", startupTaskName}
			if u.User {
				args = append([]string{"--user"}, args...)
			}
			// is-active 对 inactive / failed 也给退出码非 0，状态字在 stdout：看字，不看码。
			if out, _ := exec.Command("systemctl", args...).Output(); len(out) > 0 {
				state = strings.TrimSpace(string(out))
			}
		}
		return daemonDownLines(goos, found, taskFacts{}, lf, state)
	}
	return daemonDownLines(goos, false, taskFacts{}, lf, "")
}

// daemonDownLines 组「现在为什么没起来」那几行 —— 纯函数：事实都由调用方喂，用例直接给。
//
// 三组分开说，因为**后果不同**：
//  1. 现在就在挡住它的（这一组带 FIX）；
//  2. 现在不挡、下次断电才咬人的 —— 那是兜底缺口，**不是这一次的原因**（混在一起就是 #42 那种指错方向）；
//  3. 核过的事实（含「这一格没读到」，说清而不是猜）。
func daemonDownLines(goos string, installed bool, f taskFacts, lf liveFacts, unitState string) []string {
	var now, later, facts []string
	switch goos {
	case "windows":
		if !installed {
			now = append(now, "计划任务 "+startupTaskName+" **不在** —— daemon 根本没装过（还没 apply）")
		} else {
			if f.Disabled() {
				now = append(now, "计划任务被**停用**（定义里 Enabled=false）—— 谁也拉不起来。"+
					"FIX：schtasks /change /tn "+startupTaskName+" /enable")
			}
			if f.NoStartOnBatt || f.StopOnBatt {
				switch {
				case lf.PowerKnown && lf.OnBattery:
					now = append(now, "任务带「电池供电不启动 / 拔电就停」，而这台机器**正在靠电池跑** —— 挡它的就是这两条。"+
						"FIX：插上电源再跑一次（要改成「电池也跑」= 替客户改上游默认，得先拍，见议题 #42）")
				case lf.PowerKnown && !lf.OnBattery:
					facts = append(facts, "任务带「电池供电不启动 / 拔电就停」，但现在用的是外接电源 —— 这一次挡它的不是这两条")
				default:
					facts = append(facts, "任务带「电池供电不启动 / 拔电就停」，但"+unread(lf.Why)+" —— 是不是它在挡，判不了")
				}
			} else {
				facts = append(facts, "任务没设「电池时不启动 / 拔电就停」")
			}
			if !hasBootTrigger(f.Triggers) {
				later = append(later, "触发器只有「"+triggerList(f.Triggers)+"」—— 断电重启后没人登录就不会起"+
					"（兜底缺口，不是这一次的原因；补一条开机触发器 = 改上游默认，要先拍）")
			}
		}
	case "linux":
		if !installed {
			now = append(now, "systemd 单元 "+startupTaskName+" **不在** —— daemon 根本没装过（还没 apply）")
		} else if unitState != "" && unitState != "active" && unitState != "activating" {
			now = append(now, "单元 is-active = "+unitState+" —— 现在没在跑（failed 看它自己的日志；"+
				"inactive 就是没人拉它：`systemctl --user status "+startupTaskName+"`）")
		}
	}
	// 进程这一格两个平台共用一条判据：**进程在、socket 却拨不通**是另一种病（起来了又死），
	// 不是「没起来」—— 这两件事要查的地方完全不同。
	if lf.ProcKnown {
		if lf.ProcRunning {
			now = append(now, "daemon 进程**在**、socket 却拨不通 —— 病在「起来了又死」，去看它自己的日志，不是凭据")
		} else {
			facts = append(facts, "名字或命令行里带 cc-connect 的进程一个都没有 —— 它确实没在跑")
		}
	} else {
		facts = append(facts, "进程没问到："+unread(lf.Why))
	}
	if lf.PowerKnown {
		if lf.OnBattery {
			facts = append(facts, "此刻供电：靠电池")
		} else {
			facts = append(facts, "此刻供电：外接电源")
		}
	}
	out := []string{}
	if len(now) > 0 {
		out = append(out, "现在挡着它的：")
		for _, s := range now {
			out = append(out, "  · "+s)
		}
	} else {
		out = append(out, "没找到「现在就在挡着它」的那一条 —— 日志最后几行里它自己该说话了")
	}
	if len(later) > 0 {
		out = append(out, "只在下次断电时才咬人的（**不是**这一次的原因）：")
		for _, s := range later {
			out = append(out, "  · "+s)
		}
	}
	if len(facts) > 0 {
		out = append(out, "核过的事实：")
		for _, s := range facts {
			out = append(out, "  · "+s)
		}
	}
	return out
}

// unread 把「没读到」和「读不到的**原因**」接成一句 —— 原因可以为空（那时明说没说）。
func unread(why string) string {
	if strings.TrimSpace(why) == "" {
		return "没读到（也没说为什么）"
	}
	return "没读到（" + why + "）"
}

func triggerList(ts []string) string {
	if len(ts) == 0 {
		return "一条触发器都没有"
	}
	return strings.Join(ts, " + ")
}
