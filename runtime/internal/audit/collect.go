package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"anc/internal/harness"
	"anc/internal/trail"
)

// ToolRule 是「一个工具怎么读成一次行使」。
//
// **这是数据**（可以整份替换，同 judge 的判据表）：换 harness、加工具、改归类，
// 换的是这份表，不是代码。认不出的工具**不静默丢掉** —— 计入 Problems 报出来（见 Collect）。
type ToolRule struct {
	Tool   string   `json:"tool"`   // harness 里的工具名
	Action string   `json:"action"` // read | write | invoke
	Target []string `json:"target"` // input 里哪几个键是「对谁」；按顺序取第一个非空的
}

// BuiltinTools 是出厂工具表。覆盖 Claude Code 原生工具，以及实测过的
// Codex CLI / OpenClaw 工具名（2026-10-10）。别的 harness 加自己的条目即可 —— 加的是数据。
//
// 目标键留空的（Bash / Agent）：它们的 input 是自由文本，抽不出「对谁」。
// 这类行使照样落流水（Object 为空），但归集器会把次数报进 Problems ——
// 「有一条行使，但目标抽不出来」是必须让人看见的事实。
func BuiltinTools() []ToolRule {
	return []ToolRule{
		{Tool: "Read", Action: ActionRead, Target: []string{"file_path"}},
		{Tool: "Glob", Action: ActionRead, Target: []string{"path"}},
		{Tool: "Grep", Action: ActionRead, Target: []string{"path"}},
		{Tool: "NotebookRead", Action: ActionRead, Target: []string{"notebook_path"}},
		{Tool: "Write", Action: ActionWrite, Target: []string{"file_path"}},
		{Tool: "Edit", Action: ActionWrite, Target: []string{"file_path"}},
		{Tool: "MultiEdit", Action: ActionWrite, Target: []string{"file_path"}},
		{Tool: "NotebookEdit", Action: ActionWrite, Target: []string{"notebook_path"}},
		{Tool: "Bash", Action: ActionInvoke},
		{Tool: "BashOutput", Action: ActionInvoke},
		{Tool: "Agent", Action: ActionInvoke},
		{Tool: "Task", Action: ActionInvoke},

		// ---- Codex CLI（2026-10-10 在真记录上按出现次数排的，84 份 rollout）----
		// 入参的键是 `cmd`（**不是** command）—— 实测。
		{Tool: "exec_command", Action: ActionInvoke},
		{Tool: "shell_command", Action: ActionInvoke},
		{Tool: "write_stdin", Action: ActionInvoke},
		{Tool: "js", Action: ActionInvoke},
		{Tool: "view_image", Action: ActionRead, Target: []string{"path"}},
		{Tool: "send_input", Action: ActionInvoke},
		{Tool: "wait_agent", Action: ActionInvoke},
		{Tool: "spawn_agent", Action: ActionInvoke},
		{Tool: "request_user_input", Action: ActionInvoke},
		{Tool: "voice_pill_status", Action: ActionInvoke},
		{Tool: "open_in_codex", Action: ActionInvoke},
		{Tool: "list_mcp_resources", Action: ActionInvoke},
		{Tool: "list_mcp_resource_templates", Action: ActionInvoke},
		// ANC 自己的接入口（实测入参只有 who）：向 ANC 要上下文 = 读，
		// 递一条东西进来 = 写。对象都是「以谁的名义」。
		//
		// **同一个工具在各家的记录里名字不一样**：claude 是 mcp__anc__anc_xxx、
		// openclaw 是 anc__anc_xxx、codex 是光秃秃的 anc_xxx —— 名字里带不带
		// 命名空间前缀是那一家的事，归集只认记录里写的是什么，所以三个都登记。
		{Tool: "anc_read_context", Action: ActionRead, Target: []string{"who"}},
		// 检索也是一次「读」：客体优先取 domain（问哪块业务），没写就取 q（问什么）。
		{Tool: "anc_search", Action: ActionRead, Target: []string{"domain", "q"}},
		{Tool: "anc_get", Action: ActionRead, Target: []string{"path"}},
		{Tool: "anc_send_envelope", Action: ActionWrite, Target: []string{"who"}},
		{Tool: "mcp__anc__anc_read_context", Action: ActionRead, Target: []string{"who"}},
		{Tool: "mcp__anc__anc_search", Action: ActionRead, Target: []string{"domain", "q"}},
		{Tool: "mcp__anc__anc_get", Action: ActionRead, Target: []string{"path"}},
		{Tool: "mcp__anc__anc_send_envelope", Action: ActionWrite, Target: []string{"who"}},
		{Tool: "anc__anc_read_context", Action: ActionRead, Target: []string{"who"}},
		{Tool: "anc__anc_search", Action: ActionRead, Target: []string{"domain", "q"}},
		{Tool: "anc__anc_get", Action: ActionRead, Target: []string{"path"}},
		{Tool: "anc__anc_send_envelope", Action: ActionWrite, Target: []string{"who"}},

		// ---- OpenClaw（同一批实测；工具名带 MCP 命名空间前缀时原样搬）----
		{Tool: "read", Action: ActionRead, Target: []string{"path"}},
		{Tool: "memory_get", Action: ActionRead, Target: []string{"path"}},
		{Tool: "write", Action: ActionWrite, Target: []string{"path"}},
		{Tool: "edit", Action: ActionWrite, Target: []string{"path"}},
		{Tool: "web_fetch", Action: ActionRead, Target: []string{"url"}},
		// `gateway` 也有 path 这个键，但那是它自己的配置路径、不是被行使的文件 ——
		// 登记目标键会把它错归成「读了一个文件」，所以这里刻意不登记（那一条报「没有目标键」是对的）。
		{Tool: "gateway", Action: ActionInvoke},
		{Tool: "exec", Action: ActionInvoke},
		{Tool: "process", Action: ActionInvoke},
		{Tool: "message", Action: ActionInvoke},
		{Tool: "sessions_history", Action: ActionInvoke},
		{Tool: "session_status", Action: ActionInvoke},
		{Tool: "feishu_chat", Action: ActionInvoke},
	}
}

// CollectOptions 是一次归集的输入。
type CollectOptions struct {
	ClaudeHome string                      // harness 的项目记录根（`<ClaudeHome>/projects/<slug>/`）
	Family     harness.Family              // 口径（零值 = 内置默认那家）；记录布局从表里来
	Projects   map[string]string           // project 名 → work_dir（`render.WorkDirs` 给的）
	ActorOf    func(project string) string // project → 行使者名；nil = 用 project 名
	Tools      []ToolRule                  // 空 = BuiltinTools()
	Known      map[string]bool             // 已经记过的 id（Load 出来的 IDs）；命中的跳过
	Since      time.Time                   // 只要这个时刻之后的（零值 = 不限）
}

// CollectResult 是一次归集的结果。
type CollectResult struct {
	Records  []Record
	Problems []string // **记不到的**（目标抽不出 / 工具没登记 / 结果配不上）—— 调用方必须回显
	Shards   int      // 扫了几个分片（会话文件）
	Calls    int      // 认出的行使数（含没落进 Records 的）
}

// Collect 从 harness 原生记录里把「已经发生的行使」抽出来。
//
// 三条口径：
//   - **只读**：不改上游、不碰会话文件。
//   - **幂等**：Record.ID 由（会话 id, tool call id）拼成；重跑不会写第二条。
//     调用方传 Known（既有 id 集合）来跳过已记的。
//   - **记不到的显式报**：目标抽不出、工具没登记、结果配不上，一律进 Problems。
//
// 分工是硬的：**记录长什么样**由 trail 的读取器管（一家一份格式知识），
// **一次行使该怎么读**（read / write / invoke、对谁）由这里的工具表管（那是数据）。
// 归集自己不解析任何一家的记录 —— 混进来的话，读法会跟着记录格式一起漂。
//
// 「本可以行使但没行使」**这里永远不会出现** —— 它没发生，就没有记录可归集。
// 那一类只能靠显式补记（`anc audit add`）。这是这份流水的边界，页面要如实写。
func Collect(opt CollectOptions) CollectResult {
	var res CollectResult
	if opt.Family.ID == "" {
		// 没给口径 = 用内置默认那家。零值 Family 一路走下去会让 Source 变成空字符串，
		// 那种半截状态比直接报错更难查 —— 所以在这里就落定。
		opt.Family = harness.DefaultFamily()
	}
	rules := opt.Tools
	if len(rules) == 0 {
		rules = BuiltinTools()
	}
	byTool := make(map[string]ToolRule, len(rules))
	for _, r := range rules {
		byTool[r.Tool] = r
	}
	unregistered := map[string]int{}
	noTarget := map[string]int{}      // 工具表里没登记目标键（Bash 这类）
	missingTarget := map[string]int{} // 登记了目标键、但记录里那个字段是空的

	// note 记一问题，但**同一句话只说一次**。读取器对每一份记录都会追加同样的口径说明
	// （「codex 的记录里没有金额」这类），一份一句会把它刷成几十行，反而看不见真正的问题。
	// 带文件名的那种（「N 行解析不了」）本来就是一句一样，不会被吃掉。
	seenNote := map[string]bool{}
	// noteShard 把一份分片自己报的问题带上（去重键是**读取器的原话**）。
	// 读取器对每一份记录都会追加同样的口径说明（「codex 的记录里没有金额」这类），
	// 一份一句会把它刷成几十行、反而看不见真正的问题；而带文件名的那种
	// （「N 行解析不了」）各是各的，不会被吃掉。
	noteShard := func(path string, probs []string) {
		for _, x := range probs {
			if seenNote[x] {
				continue
			}
			seenNote[x] = true
			res.Problems = append(res.Problems, fmt.Sprintf("%s：%s", filepath.Base(path), x))
		}
	}
	note := func(x string) {
		if seenNote[x] {
			return
		}
		seenNote[x] = true
		res.Problems = append(res.Problems, x)
	}

	// readShard 读一份分片，把它自己报的问题一并交出来（**报不报由调用方定**：
	// 不是我们项目的记录，连它的口径说明都不该出现在我们的归集结果里）。
	readShard := func(path string) (trail.Session, []string) {
		s := trail.ReadShard(opt.Family, path)
		return s, s.Problems
	}

	// collectActs 把一份**已经读出来的**分片里的行使，按工具表分类落流水。
	// 分片读一次就够（codex 那一家一份记录能到几十兆）—— 认领归属也用同一次读出来的 cwd。
	collectActs := func(s trail.Session, id, project string) {
		actor := project
		if opt.ActorOf != nil {
			if a := strings.TrimSpace(opt.ActorOf(project)); a != "" {
				actor = a
			}
		}
		for _, a := range s.Acts {
			res.Calls++
			rule, ok := byTool[a.Tool]
			if !ok {
				// 没登记的工具**不静默丢掉**，但也不落流水：记下名字，扫完统一报一次。
				unregistered[a.Tool]++
				continue
			}
			r := Record{
				ID:     id + ":" + a.CallID,
				At:     a.At.Format(time.RFC3339),
				Actor:  actor,
				Action: rule.Action,
				Object: targetOf(a.Input, rule.Target),
				Tool:   a.Tool,
				Source: "collect:" + opt.Family.ID,
				Ref:    id,
				Detail: head(inputDigest(a.Tool, a.Input), 160),
				Why:    a.Why,
			}
			// 目标抽不出来分两种：工具表没登记目标键（Bash 这类）vs 登记了但记录里是空的。
			// 两个都得报，但**原因不同** —— 混成一句话会把人送去改错的地方。
			if r.Object == "" {
				if len(rule.Target) == 0 {
					noTarget[a.Tool]++
				} else {
					missingTarget[a.Tool]++
				}
			}
			if !opt.Since.IsZero() && a.At.Before(opt.Since) {
				continue
			}
			if opt.Known[r.ID] {
				continue
			}
			switch a.Result {
			case trail.ResultOK:
				r.Result = ResultOK
			case trail.ResultDenied:
				r.Result = ResultDenied
			case trail.ResultFailed:
				r.Result = ResultFailed
			default:
				// 没有结果：这一轮被中断了（会话断了 / 进程被杀了）。
				// **不许当成「没发生」**，也不许猜一个结果出来 —— 落 unknown，并在 Problems 里说清。
				r.Result = ResultUnknown
				r.Detail = strings.TrimSpace(r.Detail + "（没有配到结果，是这一轮被中断了）")
				res.Problems = append(res.Problems, fmt.Sprintf(
					"%s 的 %s 调用没有配到结果（结果落 unknown）—— 多半是那一轮被中断，不是它没发生",
					id, a.Tool))
			}
			res.Records = append(res.Records, r)
		}
	}

	// 分片从哪来，两种口径（都来自接入口径表）：
	//  ① 声明了 shard_dir 的家：分片堆在同一层里，目录从项目名推不出来 → 全扫出来，
	//     再按记录里的 cwd 认领到项目（codex 按 年/月/日 分层就是这样）。
	//  ② 别的家：按项目推目录（<root>/<projects_dir>/<slug(work_dir)>）—— 目录本身就是项目。
	shards, global, serr := opt.Family.EnumerateShards(opt.ClaudeHome)
	if serr != nil {
		res.Problems = append(res.Problems, serr.Error())
		return res
	}
	if global {
		// 归一化后的工作目录 → 项目：记录里的 cwd 与配置里的 work_dir 写法可能不一样
		// （分隔符、大小写），比的是归一化之后的值。
		byDir := map[string]string{}
		for _, project := range sortedKeys(opt.Projects) {
			byDir[normPath(opt.Projects[project])] = project
		}
		for _, path := range shards {
			res.Shards++
			s, probs := readShard(path)
			if !s.Found {
				// 读不到就认不出归属：它可能是我们的、也可能是别人的。不静默 ——
				// 「读不到」伪装成「没有」是这份流水最不能犯的错。
				noteShard(path, probs)
				continue
			}
			project, ok := byDir[normPath(s.Cwd)]
			if !ok {
				// 认不到 = 别的目录的会话（这台机器上不止我们这几个项目），跳过 ——
				// 连它自己的口径说明都不该混进我们的归集结果。
				// **但记录里没有 cwd 是另一回事**：那说明这份分片认不出归属，
				// 得报出来 —— 不能悄悄当成「没有」。
				if strings.TrimSpace(s.Cwd) == "" {
					noteShard(path, probs)
					note(fmt.Sprintf(
						"%s 里没有 cwd（%s 那一家认归属就是靠它）—— 这份分片里的 %d 次行使没有归集，这是**读不到**",
						filepath.Base(path), opt.Family.ID, len(s.Acts)))
				}
				continue
			}
			noteShard(path, probs)
			collectActs(s, shardID(path), project)
		}
	} else {
		for _, project := range sortedKeys(opt.Projects) {
			dir, derr := opt.Family.TranscriptDir(opt.ClaudeHome, opt.Projects[project])
			if derr != nil {
				res.Problems = append(res.Problems, derr.Error())
				continue
			}
			names, err := os.ReadDir(dir)
			if err != nil {
				// 记录目录不在 = 这个 bot 还没有过会话（或没跑在这台机器上）。
				// 不报 Problem：那是「还没发生」，不是「读不到」（区别见 ReadShard 的 Found）。
				continue
			}
			for _, n := range names {
				if n.IsDir() || !strings.HasSuffix(n.Name(), ".jsonl") {
					continue
				}
				res.Shards++
				path := filepath.Join(dir, n.Name())
				s, probs := readShard(path)
				noteShard(path, probs)
				if !s.Found {
					continue
				}
				collectActs(s, shardID(path), project)
			}
		}
	}
	for _, tool := range sortedKeys(unregistered) {
		res.Problems = append(res.Problems, fmt.Sprintf(
			"工具 %s 调用 %d 次，不在工具表里，没有落进审计 —— 加一条 ToolRule 就能收（工具表是数据）",
			tool, unregistered[tool]))
	}
	for _, tool := range sortedKeys(noTarget) {
		res.Problems = append(res.Problems, fmt.Sprintf(
			"工具 %s 调用 %d 次，工具表里没有它的目标键（object 为空）—— 这类行使「对谁」这一栏永远是空的，看板上按「未归属」显示",
			tool, noTarget[tool]))
	}
	for _, tool := range sortedKeys(missingTarget) {
		res.Problems = append(res.Problems, fmt.Sprintf(
			"工具 %s 调用 %d 次，工具表登记了目标键但记录里是空的（object 为空）—— 这是记录不全，不是「没有目标」",
			tool, missingTarget[tool]))
	}
	return res
}

// shardID 是分片的会话句柄：文件名去后缀（各家都是这一条，见接入口径表的 session_handle）。
func shardID(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

// normPath 把路径归一成「能比」的样子：分隔符统一、去掉 ./ 这类冗余。
//
// Windows 上同一个目录的大小写可以不一样（配置里写 D:\Foo，记录里写 d:\foo），
// 所以那一头不分大小写；别家分大小写，照原样比。
func normPath(p string) string {
	p = filepath.ToSlash(filepath.Clean(strings.TrimSpace(p)))
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}

// targetOf 从 input 里取「对谁」。键按顺序试，取第一个非空字符串。
func targetOf(input json.RawMessage, keys []string) string {
	if len(keys) == 0 || len(input) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(input, &m) != nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// inputDigest 给 Detail 一句可读的原话：目标键没登记的（Bash）拿 command / prompt，
// 别的工具拿第一个字符串值 —— 只在 object 抽不出来时才用得上。
func inputDigest(tool string, input json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(input, &m) != nil {
		return ""
	}
	for _, k := range []string{"command", "description", "prompt", "pattern", "query"} {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// reasonHead 取原因的第一句。上游在拒绝理由后面接了一长段给模型看的 IMPORTANT 提示，
// 那是给 agent 的指令、不是给人看的原因 —— 切掉。
//
// 与 trail.reasonHead 同源（同一份记录格式），刻意各留一份：
// 两个包对记录的依赖面不同，谁也不用为对方的重构买单。
func reasonHead(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "IMPORTANT:"); i > 0 {
		s = strings.TrimSpace(s[:i])
	}
	s = strings.ReplaceAll(s, "\n", " ")
	return head(s, 160)
}

// blockText 把 block 的 content 取成人话：可能是字符串，也可能是 [{type,text}]。
func blockText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Text)
	}
	return b.String()
}

// slugDir 是 harness 给项目目录起名的规则（同 trail.Slug）：每个非字母数字字符 → '-'。
//
// 这个映射是有损的、反推不回来，所以只做正向：从 work_dir 算出该去哪个目录找记录。
func slugDir(workDir string) string {
	s, err := harness.Slug(harness.RuleNonalnumDash, workDir)
	if err != nil {
		// 规则是编译期常量，走不到这里；真走到了宁可炸，也不要静默编一个目录名。
		panic("slugDir: " + err.Error())
	}
	return s
}

// head 按 **rune** 截断：按字节切会把一个中文字切成两半，拼出乱码。
func head(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
