package trail

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ReaderCodex 是 OpenAI Codex CLI 的读取器名（写在接入口径表的 reader 字段里）。
const ReaderCodex = "codex-jsonl"

// codexLine 是一行 rollout 记录的外壳：`type` 决定 payload 是哪一种。
//
// 这家一行一个事件，事件的种类写在**外层** type 上
// （session_meta / turn_context / event_msg / response_item / token_usage_record / world_state），
// 内层 payload 再各自带一个二级 type（UserMessage / CommandExecution / function_call …）。
type codexLine struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"` // RFC3339 字符串（毫秒精度）
	Payload   json.RawMessage `json:"payload"`
}

// codexMeta 是 session_meta 的 payload。
type codexMeta struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
}

// codexEvent 是 event_msg 的 payload。
type codexEvent struct {
	Type   string        `json:"type"`
	TurnID string        `json:"turn_id"`
	Item   codexItemDone `json:"item"`
}

// codexItemDone 是 event_msg/item_completed 里那一项。种类是 Item.Type：
// UserMessage / CommandExecution / Reasoning / AgentMessage / McpToolCall / ImageView。
//
// 实测（2026-10-10，本机 84 份 rollout）：**命令与 MCP 调用都带 status**
// （CommandExecution / McpToolCall），ImageView 不带。所以「成没成」的显式判据用 status，
// 没有 status 的那几种退到「有没有 function_call_output」。
type codexItemDone struct {
	Type    string `json:"type"`
	ID      string `json:"id"` // 就是那次 function_call 的 call_id
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Server           string   `json:"server"` // McpToolCall
	Tool             string   `json:"tool"`   // McpToolCall
	Status           string   `json:"status"` // completed / failed / …
	Stderr           string   `json:"stderr"`
	AggregatedOutput string   `json:"aggregated_output"`
	Command          []string `json:"command"`
	Result           struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"result"`
}

// whyOf 取这次调用「没成功」的原话。**按记录里的优先级取，不转述**：
// 命令的 stderr → 聚合输出 → MCP / 别的工具的 result.content 文本。都没有就留空。
func (it codexItemDone) whyOf() string {
	for _, s := range []string{it.Stderr, it.AggregatedOutput} {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	var parts []string
	for _, c := range it.Result.Content {
		if c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// codexItem 是 response_item 的 payload。
type codexItem struct {
	Type   string `json:"type"` // message / reasoning / function_call / function_call_output
	Name   string `json:"name"`
	CallID string `json:"call_id"`
	Role   string `json:"role"`
	// Arguments 是**字符串**（不是对象）—— 实测：`"arguments":"{\"cmd\": \"ls\"}"`。
	Arguments string `json:"arguments"`
	Meta      struct {
		TurnID string `json:"turn_id"`
	} `json:"internal_chat_message_metadata_passthrough"`
}

// codexUsageRec 是 token_usage_record 的 payload。
type codexUsageRec struct {
	TurnID     string `json:"turn_id"`
	ResponseID string `json:"response_id"`
	Usage      struct {
		InputTokens       int `json:"input_tokens"`
		CachedInputTokens int `json:"cached_input_tokens"`
		CacheWriteTokens  int `json:"cache_write_input_tokens"`
		OutputTokens      int `json:"output_tokens"`
	} `json:"usage"`
}

// usageOf 取这一次调用的账。
//
// **缓存读含在输入里**（实测：input 101911 + output 274 = total 102185，cached 10368 是其中一部分）
// —— 所以 CacheRead 是 In 的子集，不能与 In 相加。
func (r codexUsageRec) usageOf() Usage {
	return Usage{
		In:         r.Usage.InputTokens,
		Out:        r.Usage.OutputTokens,
		CacheRead:  r.Usage.CachedInputTokens,
		CacheWrite: r.Usage.CacheWriteTokens,
	}
}

// codexScan 是一次 rollout 扫描的中间状态。
//
// 拆成结构体而不是一长串闭包，是为了让「一行记录怎么落进账」按种类各归各位：
// 记录格式变了（实测已经变过几次），只需要改对应那一个方法，别的种类不受牵连。
// 状态本身仍然是**一遍扫完**：没有随机访问、没有二次读盘。
type codexScan struct {
	path string
	ses  *Session

	turns    []Turn
	ends     []time.Time
	turnIdx  map[string]int
	callName map[string]string
	byCall   map[string]*Usage
	callTurn map[string]int
	tools    map[string]int
	uses     map[string]int
	acts     []Act
	actIdx   map[string]int
	itemRes  map[string]codexItemResult
	hasOut   map[string]bool
	failures []Failure

	badLines int
	metaID   string
	asked    int
}

// codexItemResult 是 item_completed 给出的显式成败（只有带 status 的项才有）。
type codexItemResult struct{ res, why string }

func newCodexScan(path string, s *Session) *codexScan {
	return &codexScan{
		path:     path,
		ses:      s,
		turnIdx:  map[string]int{},
		callName: map[string]string{},
		byCall:   map[string]*Usage{},
		callTurn: map[string]int{},
		tools:    map[string]int{},
		uses:     map[string]int{},
		actIdx:   map[string]int{},
		itemRes:  map[string]codexItemResult{},
		hasOut:   map[string]bool{},
	}
}

func (c *codexScan) observe(ts time.Time) {
	if ts.IsZero() {
		return
	}
	if c.ses.Started.IsZero() || ts.Before(c.ses.Started) {
		c.ses.Started = ts
	}
	if ts.After(c.ses.Ended) {
		c.ses.Ended = ts
	}
}

func (c *codexScan) mark(i int, ts time.Time) {
	if i >= 0 && i < len(c.ends) && ts.After(c.ends[i]) {
		c.ends[i] = ts
	}
}

// ensureTurn 按 turn_id 认轮次：同一个 turn_id 的第一条记录就把这一轮立起来。
// 用「先到先立」而不是「等 UserMessage 才立」，是为了让**没有提问记录的那一轮**
// （续跑 / 系统触发）的账也有地方落，而不是被悄悄丢掉。
func (c *codexScan) ensureTurn(turnID string, ts time.Time) int {
	if turnID == "" {
		return -1
	}
	if i, ok := c.turnIdx[turnID]; ok {
		if c.turns[i].Prompt == "" && !ts.IsZero() && ts.Before(c.turns[i].At) {
			c.turns[i].At = ts
		}
		return i
	}
	c.turns = append(c.turns, Turn{At: ts})
	c.ends = append(c.ends, ts)
	c.turnIdx[turnID] = len(c.turns) - 1
	return len(c.turns) - 1
}

// line 处理一行记录。解析不了的行只计数，不中断整份扫描 ——
// 半截记录不该让整段会话的账消失。
func (c *codexScan) line(raw []byte) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return
	}
	var l codexLine
	if err := json.Unmarshal(raw, &l); err != nil {
		c.badLines++
		return
	}
	var ts time.Time
	if l.Timestamp != "" {
		if parsed, perr := time.Parse(time.RFC3339Nano, l.Timestamp); perr == nil {
			ts = parsed
		}
	}
	switch l.Type {
	case "session_meta":
		c.onMeta(l.Payload, ts)
	case "event_msg":
		c.onEvent(l.Payload, ts)
	case "response_item":
		c.onItem(l.Payload, ts)
	case "token_usage_record":
		c.onUsage(l.Payload, ts)
	default:
		c.observe(ts)
	}
}

func (c *codexScan) onMeta(payload json.RawMessage, ts time.Time) {
	var m codexMeta
	if json.Unmarshal(payload, &m) == nil {
		c.metaID = m.SessionID
		if m.Cwd != "" {
			c.ses.Cwd = m.Cwd
		}
		if c.ses.ID == "" {
			// 调用方没有句柄（直接读一份分片）：按记录自报的 session_id 补上，
			// 免得下面那条「session_id 对不上」误报。
			c.ses.ID = m.SessionID
		}
	}
	c.observe(ts)
}

func (c *codexScan) onEvent(payload json.RawMessage, ts time.Time) {
	var e codexEvent
	if json.Unmarshal(payload, &e) != nil {
		c.badLines++
		return
	}
	i := c.ensureTurn(e.TurnID, ts)
	if e.Type == "item_completed" {
		switch e.Item.Type {
		case "UserMessage":
			// 这一段整段可能挂着环境上下文（实测：`<in-app-browser-context …>` + `## My request:`
			// + 真人那句），所以先 promptOf 挑出人问的那句，再截。
			if i >= 0 && c.turns[i].Prompt == "" {
				var parts []string
				for _, part := range e.Item.Content {
					if part.Text != "" {
						parts = append(parts, part.Text)
					}
				}
				c.turns[i].Prompt = promptOf(strings.Join(parts, " "))
				c.asked++
			}
		case "CommandExecution":
			// 没成功的动作：status 不是 completed。原因照抄记录原话（截断），不转述。
			if e.Item.Status != "" && e.Item.Status != "completed" {
				why := e.Item.Stderr
				if strings.TrimSpace(why) == "" {
					why = e.Item.AggregatedOutput
				}
				fa := Failure{Kind: "failed", Tool: c.callName[e.Item.ID], Why: reasonHead(why), At: ts}
				c.failures = append(c.failures, fa)
				if i >= 0 {
					c.turns[i].Failures = append(c.turns[i].Failures, fa)
				}
			}
		}
		// 显式成败：**只认带了 status 的那些**（实测 CommandExecution 与 McpToolCall 有，
		// ImageView 没有）。先记下来，扫完再配回对应的行使 —— 免得依赖「哪一行先到」。
		if e.Item.ID != "" && e.Item.Status != "" {
			res := ResultOK
			if e.Item.Status != "completed" || e.Item.Result.IsError {
				res = ResultFailed
			}
			c.itemRes[e.Item.ID] = codexItemResult{res, reasonHead(e.Item.whyOf())}
		}
	}
	c.mark(i, ts)
	c.observe(ts)
}

func (c *codexScan) onItem(payload json.RawMessage, ts time.Time) {
	var it codexItem
	if json.Unmarshal(payload, &it) != nil {
		c.badLines++
		return
	}
	i := c.ensureTurn(it.Meta.TurnID, ts)
	if it.Type == "function_call" && it.Name != "" {
		c.tools[it.Name]++
		if i >= 0 {
			c.turns[i].Tools = bumpBy(c.turns[i].Tools, it.Name, 1)
		}
		if it.CallID != "" {
			c.callName[it.CallID] = it.Name
			if i >= 0 {
				c.uses[it.CallID] = i
			}
		}
		c.acts = putAct(c.acts, c.actIdx, Act{
			CallID: it.CallID, At: ts, Tool: it.Name, Input: rawJSON(it.Arguments),
		})
	}
	if it.Type == "function_call_output" && it.CallID != "" {
		c.hasOut[it.CallID] = true
	}
	c.mark(i, ts)
	c.observe(ts)
}

func (c *codexScan) onUsage(payload json.RawMessage, ts time.Time) {
	var r codexUsageRec
	if json.Unmarshal(payload, &r) != nil {
		c.badLines++
		return
	}
	i := c.ensureTurn(r.TurnID, ts)
	key := r.ResponseID
	u := r.usageOf()
	if key == "" {
		// 没有 response_id 就没法去重（记录格式变了吗）——宁可少并（可能多算）
		// 也要把它算进来，并在 Problems 里说清楚。
		key = "noid:" + ts.Format(time.RFC3339Nano)
	}
	if prev, ok := c.byCall[key]; ok {
		*prev = maxOf(*prev, u)
	} else {
		cp := u
		c.byCall[key] = &cp
	}
	c.callTurn[key] = i
	c.mark(i, ts)
	c.observe(ts)
}

// finish 把中间状态折进会话账，并把「这份账仍然说不准」的地方写进 Problems。
func (c *codexScan) finish() map[string]int {
	s := c.ses
	if c.badLines > 0 {
		s.Problems = append(s.Problems, fmt.Sprintf(
			"%s 里有 %d 行解析不了 —— 记录格式可能变了，这份账不全", filepath.Base(c.path), c.badLines))
	}
	if c.metaID != "" && s.ID != "" && c.metaID != s.ID {
		s.Problems = append(s.Problems, fmt.Sprintf(
			"这份记录的 session_id 是 %s，而要读的是 %s —— 可能找错了记录", c.metaID, s.ID))
	}

	var orphan Usage
	for key, u := range c.byCall {
		if i := c.callTurn[key]; i >= 0 && i < len(c.turns) {
			c.turns[i].Usage = c.turns[i].Usage.Add(*u)
		} else {
			orphan = orphan.Add(*u)
		}
	}
	for i := range c.turns {
		c.turns[i].Usage = clampUsage(c.turns[i].Usage)
		s.Usage = s.Usage.Add(c.turns[i].Usage)
		sortCounts(c.turns[i].Tools)
		if i < len(c.ends) && c.ends[i].After(c.turns[i].At) {
			c.turns[i].Duration = c.ends[i].Sub(c.turns[i].At)
		}
	}
	s.Turns = append(s.Turns, c.turns...)
	s.Failures = append(s.Failures, c.failures...)
	for name, n := range c.tools {
		s.Tools = bumpBy(s.Tools, name, n)
	}
	sortCounts(s.Tools)

	// 行使的结果：① 带 status 的那项说了算（命令 / MCP 调用）；
	// ② 没有 status 的（ImageView 这类）退到「有没有输出」——有输出 = 跑完了；
	// ③ 两样都没有 = unknown（settleActs 落定），**不许当成没发生**。
	for i := range c.acts {
		if r, ok := c.itemRes[c.acts[i].CallID]; ok {
			c.acts[i].Result, c.acts[i].Why = r.res, r.why
			continue
		}
		if c.hasOut[c.acts[i].CallID] {
			c.acts[i].Result = ResultOK
		}
	}
	s.Acts = append(s.Acts, settleActs(c.acts)...)

	if orphan.Total() > 0 || orphan.CacheRead > 0 {
		s.Problems = append(s.Problems, fmt.Sprintf(
			"有 %d 个 token 记录归不到某一轮（没有 turn_id）：in %d / out %d —— 只在会话总账上",
			len(c.byCall)-len(c.callTurn), orphan.In, orphan.Out))
	}
	if n := len(c.turns) - c.asked; n > 0 {
		s.Problems = append(s.Problems, fmt.Sprintf(
			"有 %d 轮没有用户提问记录（续跑 / 系统触发的那种）：这些轮的账在，提问不在", n))
	}

	// 三处「这份账仍然说不准」的地方，明说，别让人以为读到的是全的。
	s.Problems = append(s.Problems,
		"codex 的记录里**没有金额**（实测：全文只有推理正文里出现过 cost 这个词）—— cost_usd 记 0 是**读不到**，不是花了 0 元",
		"本段会话的账按每轮逐次调用求和（实测与末条 turn_token_usage 逐字段相等）；"+
			"记录里的 thread_token_usage 是**线程累计**，fork 出来的会话把前史一起带着 —— 别拿它当本会话的账",
		"缓存读（cached_input_tokens）**含在输入里**（实测 total = input + output）—— 别把 in 与 cache_read 相加")
	return c.uses
}

// scanCodex 读一份 Codex rollout 记录，折进 s。
//
// 形态是 2026-10-09 在本机真记录上**逐字段对拍**出来的（一次 fork 出来的会话，5 轮 / 541 行），
// 不是照文档猜的。四条口径都有实测支撑：
//
//  1. **一轮的账 = 该轮各次调用的 `usage` 求和**，按 `response_id` 去重（65 条无重复）。
//     实测这一条与末条 `turn_token_usage` **逐字段相等**（5/5 轮），两条路互为对拍。
//  2. **`thread_token_usage` 不是本段会话的账** —— 它是线程累计，fork 出来的会话把前史一起带进来
//     （本样本 6.4 亿 vs 本段 1.04 千万，差 60 倍）。拿它当本会话的账就是**严重多报**。
//  3. **提问在 `event_msg/item_completed` 的 `UserMessage` 里**，不在 `response_item` 的
//     `message/role=user` 里 —— 后者是环境上下文那种合成注入（`content_item_kinds` 会标出来）。
//  4. **失败判据在 `CommandExecution.status`**（completed / failed），不在
//     `function_call_output` 上 —— 后者只有输出文本，不带成败。
//
// 记录里**没有金额字段**（实测：全文只有推理正文里出现过 cost 这个英文词）：成本记 0 并在
// Problems 里说明 —— 「读不到」不能当成「花了 0 元」。
func scanCodex(path string, s *Session) (map[string]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	c := newCodexScan(path, s)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	for sc.Scan() {
		c.line(sc.Bytes())
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return c.finish(), nil
}
