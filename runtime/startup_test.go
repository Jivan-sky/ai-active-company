package main

import (
	"runtime"
	"strings"
	"testing"
	"unicode/utf16"
)

// 用例是真机抓的：Windows 计划任务 cc-connect 的定义（2026-10-08，笔记本，未接电源）。
// 抓下来是 UTF-16、只有登录触发器、两条电源设置都是 true —— 三样都进了下面的用例。
const taskXML = `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.3" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Principals>
    <Principal id="Author">
      <UserId>S-1-5-21-2766059460-2847692454-1436359435-1001</UserId>
      <LogonType>InteractiveToken</LogonType>
    </Principal>
  </Principals>
  <Settings>
    <DisallowStartIfOnBatteries>true</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>true</StopIfGoingOnBatteries>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
  </Settings>
  <Triggers>
    <LogonTrigger>
      <UserId>LAPTOP-XXXXXXXX\dev</UserId>
    </LogonTrigger>
  </Triggers>
</Task>`

// utf16LE 把字面量编成 schtasks 那种「BOM + UTF-16LE」的字节，走一遍真解码路径。
func utf16LE(s string) []byte {
	b := []byte{0xFF, 0xFE}
	for _, r := range utf16.Encode([]rune(s)) {
		b = append(b, byte(r), byte(r>>8))
	}
	return b
}

func TestParseTaskXMLUTF16(t *testing.T) {
	f, err := parseTaskXML(utf16LE(taskXML))
	if err != nil {
		t.Fatalf("解析真机抓下来的 UTF-16 任务定义报错：%v", err)
	}
	if len(f.Triggers) != 1 || f.Triggers[0] != "LogonTrigger" {
		t.Errorf("触发器 = %v，想要 [LogonTrigger]", f.Triggers)
	}
	if !f.NoStartOnBatt || !f.StopOnBatt {
		t.Errorf("两条电源设置该都是 true，实际 NoStartOnBatt=%v StopOnBatt=%v", f.NoStartOnBatt, f.StopOnBatt)
	}
}

func TestParseTaskXMLUTF8PassThrough(t *testing.T) {
	// 没有 BOM 的那条路也要能走（schtasks 换成输出 UTF-8 时不许哑掉）
	f, err := parseTaskXML([]byte(taskXML))
	if err != nil {
		t.Fatalf("无 BOM 的 UTF-8 任务定义解析报错：%v", err)
	}
	if len(f.Triggers) != 1 {
		t.Errorf("触发器 = %v，想要 1 条", f.Triggers)
	}
}

// 判据只认 BootTrigger：登录触发器和开机触发器长得很像，不能混为一谈。
func TestHasBootTrigger(t *testing.T) {
	if hasBootTrigger([]string{"LogonTrigger"}) {
		t.Error("只有登录触发器不该算「开机就起」")
	}
	if !hasBootTrigger([]string{"LogonTrigger", "BootTrigger"}) {
		t.Error("带开机触发器就该算过")
	}
	if hasBootTrigger(nil) {
		t.Error("没有触发器不许算过")
	}
}

func TestParseLinger(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{
		{"Linger=yes\n", true},
		{"Linger=no\n", false},
		{"", false},
		{"Linger=yes", true},
	} {
		if got := parseLinger(c.in); got != c.want {
			t.Errorf("parseLinger(%q) = %v，想要 %v", c.in, got, c.want)
		}
	}
}

// 没覆盖的平台返回 nil：doctor 不许因为「这个平台不知道查什么」而报出假红。
func TestStartupChecksOnThisPlatform(t *testing.T) {
	cs := startupChecks()
	switch runtime.GOOS {
	case "windows", "linux":
		if cs == nil {
			t.Fatalf("%s 上该有自检项", runtime.GOOS)
		}
		for _, c := range cs {
			if c.Name == "" {
				t.Errorf("自检项缺名字：%+v", c)
			}
		}
	default:
		if cs != nil {
			t.Errorf("%s 上该返回 nil，实际 %v", runtime.GOOS, cs)
		}
	}
}

// ---------- 议题 #42：任务被停用 / 电源条件 / 「起来又死」各说各的 ----------
//
// 2026-10-16 本机实测（真机抓的定义）：cc-connect 任务的 `<Settings>` 里写着
// `<Enabled>false</Enabled>` —— 也就是说这台开发机上它**一直是被停用的**，
// 而在此之前 doctor 只看触发器与电源条件，从来没报过这一条。
const taskXMLDisabled = `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.3" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Settings>
    <DisallowStartIfOnBatteries>true</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>true</StopIfGoingOnBatteries>
    <Enabled>false</Enabled>
  </Settings>
  <Triggers>
    <LogonTrigger><UserId>LAPTOP-XXXXXXXX\dev</UserId></LogonTrigger>
  </Triggers>
</Task>`

// 停用这一格读的是**定义**（locale-free 的 true/false），不是 Get-ScheduledTask 那个会被
// 系统语言翻译过去的 State —— 判据不许被系统语言带走。
func TestParseTaskXMLReadsEnabled(t *testing.T) {
	f, err := parseTaskXML([]byte(taskXMLDisabled))
	if err != nil {
		t.Fatal(err)
	}
	if !f.Disabled() {
		t.Errorf("定义里写着 Enabled=false，该认停用；Enabled=%q", f.Enabled)
	}
	// 老定义里没这一格 = 默认启用，不许被读成停用（读错方向就是另一种假红）。
	old, err := parseTaskXML([]byte(taskXML))
	if err != nil {
		t.Fatal(err)
	}
	if old.Disabled() {
		t.Error("没写 <Enabled> 是「默认启用」，不是「停用」")
	}
	tru := strings.Replace(taskXMLDisabled, "<Enabled>false</Enabled>", "<Enabled>true</Enabled>", 1)
	if f2, err := parseTaskXML([]byte(tru)); err != nil || f2.Disabled() {
		t.Errorf("Enabled=true 不该算停用（err=%v）", err)
	}
}

// liveFacts 全给定：这批用例只验判据，不去问真机器的电源与进程（那随机器而变，会变成假绿）。
func withLive(t *testing.T, lf liveFacts) {
	t.Helper()
	old := liveNow
	liveNow = func() liveFacts { return lf }
	t.Cleanup(func() { liveNow = old })
}

// containsLine：这几行里有没有哪一行同时含这几个词（报缺的文案即契约，按关键词断，不钉整句）。
func hasLine(lines []string, wants ...string) bool {
	for _, l := range lines {
		hit := true
		for _, w := range wants {
			if !strings.Contains(l, w) {
				hit = false
				break
			}
		}
		if hit {
			return true
		}
	}
	return false
}

// 本机今天就是这一种：任务在、定义里停用了、现在接着外电、进程不在。
// 之前那句 FIX 只会把人指向凭据桥 —— 现在必须**第一句就点名「停用」**。
func TestDaemonDownSaysDisabled(t *testing.T) {
	lines := daemonDownLines("windows", true, taskFacts{Triggers: []string{"LogonTrigger"}, Enabled: "false"},
		liveFacts{PowerKnown: true, OnBattery: false, ProcKnown: true, ProcRunning: false}, "")
	if !hasLine(lines, "停用", "/enable") {
		t.Fatalf("任务停用了却没点名停用：%v", lines)
	}
	if !hasLine(lines, "外接电源") {
		t.Errorf("核过的事实里该有「此刻供电」：%v", lines)
	}
	if !hasLine(lines, "没在跑") {
		t.Errorf("进程那一格该说它没在跑：%v", lines)
	}
	// 没开机触发器是**下一次**的事：不许混进「现在挡着它的」。
	if hasLine(lines[:1], "断电重启") {
		t.Errorf("兜底缺口混进了「现在挡着它」：%v", lines)
	}
}

// 被电源条件挡住的**确定因**：两条设置都在 + 此刻真的靠电池。
func TestDaemonDownSaysOnBatteryIsTheBlocker(t *testing.T) {
	f := taskFacts{Triggers: []string{"LogonTrigger", "BootTrigger"}, NoStartOnBatt: true, StopOnBatt: true}
	lines := daemonDownLines("windows", true, f, liveFacts{PowerKnown: true, OnBattery: true, ProcKnown: true}, "")
	if !hasLine(lines, "正在靠电池跑", "插上电源") {
		t.Fatalf("在用电池 + 电源条件在，该点名这两条：%v", lines)
	}
}

// **读不到就不许猜**：供电这一格读不到时，那两条设置只能进「事实」，绝不能报成「挡它的就是它」。
// 这条正是议题 #42 的病根：把一个不是真因的东西说成真因。
func TestDaemonDownDoesNotBlamePowerWhenUnknown(t *testing.T) {
	f := taskFacts{Triggers: []string{"LogonTrigger", "BootTrigger"}, NoStartOnBatt: true, StopOnBatt: true}
	lines := daemonDownLines("windows", true, f, liveFacts{PowerKnown: false, Why: "问不到：powershell 不在", ProcKnown: false}, "")
	if hasLine(lines, "正在靠电池跑") {
		t.Fatalf("供电都没读到，凭什么说它在挡：%v", lines)
	}
	if !hasLine(lines, "判不了") {
		t.Errorf("读不到就该说「判不了」：%v", lines)
	}
	if !hasLine(lines, "没找到「现在就在挡着它」的那一条") {
		t.Errorf("既然没有确定因，就该明说没找到（而不是留一句空话）：%v", lines)
	}
}

// 进程**在**、socket 却拨不通 = 另一种病（起来了又死）：要查的是它自己的日志，不是凭据。
func TestDaemonDownSaysProcessAliveButSocketDead(t *testing.T) {
	lines := daemonDownLines("windows", true, taskFacts{Triggers: []string{"BootTrigger"}, Enabled: "true"},
		liveFacts{PowerKnown: true, OnBattery: false, ProcKnown: true, ProcRunning: true}, "")
	if !hasLine(lines, "起来了又死") {
		t.Fatalf("进程在、socket 拨不通该单独说：%v", lines)
	}
}

// 没装 = 第一种因；Linux 的「现在没在跑」看 is-active 的状态字（failed 与 inactive 要指的地方不同）。
func TestDaemonDownNotInstalledAndLinuxStates(t *testing.T) {
	win := daemonDownLines("windows", false, taskFacts{}, liveFacts{ProcKnown: true}, "")
	if !hasLine(win, "不在", "还没 apply") {
		t.Errorf("没装该明说没装：%v", win)
	}
	lin := daemonDownLines("linux", true, taskFacts{}, liveFacts{ProcKnown: true}, "failed")
	if !hasLine(lin, "failed") {
		t.Errorf("Linux 该把 is-active 的状态字报出来：%v", lin)
	}
	ok := daemonDownLines("linux", true, taskFacts{Triggers: nil}, liveFacts{ProcKnown: true, ProcRunning: true}, "active")
	if !hasLine(ok, "起来了又死") {
		t.Errorf("Linux 上进程在、socket 不通也是「起来又死」：%v", ok)
	}
}

// 真机烟测：这一支要 spawn powershell / schtasks，不许在任何机器上崩、也不许空手（行数 ≥ 1）。
// 它**不**断言这台机器的具体状态（那会随机器变）；-v 时把行打出来，人一眼能看出判得对不对。
func TestWhyDaemonDownRunsForReal(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("只有这两个平台有这一路读数")
	}
	lines := whyDaemonDown(runtime.GOOS)
	if len(lines) == 0 {
		t.Fatal("一行都没有 —— 这条回读就是空手")
	}
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			t.Errorf("空行混进来了：%v", lines)
		}
		t.Logf("真机读数：%s", l)
	}
}
