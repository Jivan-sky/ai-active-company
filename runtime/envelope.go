package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"anc/internal/audit"
	"anc/internal/envelope"
	"anc/internal/hot"
	"anc/internal/mcp"
	"anc/internal/org"
)

const envelopeUsage = `anc envelope —— 接入面：递进来（信封）+ 要出去（读出口）

出站读出口的用法在 serve 里 —— 它挂在接入面的 MCP 服务端上，工具名 anc_read_context：
只读、按可见范围过滤（自己的域全行 / 别人的域只给「找谁」）、每次调用留一条痕进 audit。

用法：
  anc envelope check <vault 目录> <信封.json> [--json]
  anc envelope serve <vault 目录> [--addr 127.0.0.1:8791] [--data <data 目录>]
                    起接入面：harness 通过 MCP（streamable HTTP）调 anc_send_envelope 把信封递进来

它答三件事：
  1 结构   读不读得懂（缺 id / ts 不是 RFC3339 / 缺 who → 拒收，退出码 1）
  2 绑定   who / on_behalf_of / scope 指向的东西在真相源里**到底存不存在**
           这一组发现走生效规则表的档位：默认 warn（只回显，不拦）
  3 授权   这封信的（主体，客体）有没有一条 grant 覆盖 —— **只判跨域**
           （没写 scope / 写的是自己那一摊，本来就不通 grant）
           这一条**出厂就是 fatal**（SPEC §6 不变量 1「默认拒」），可以关：
           company.md 的 policy 段写 grant.match.missing = warn|off

三组的档位都在数据里 —— 门禁是数据，不是代码里的 if。看生效档位：anc org check <vault> --rules。

顺带把判据那六问摊出来：谁 / 代谁 / 哪块业务 / 要什么 / 证据在哪 / 要不要人拍。

它**不**做：不发、不写。想看待生效的档位：anc org check <vault> --rules。
`

// envelopeFinding 是发现对外的形状（与 board.Finding 同形）：规则 id / 档位 / 定位 / 文案。
type envelopeFinding struct {
	Rule  string `json:"rule"`
	Level string `json:"level"`
	Where string `json:"where"`
	Msg   string `json:"msg"`
}

type envelopeCheckOut struct {
	OK       bool              `json:"ok"`
	Envelope envelope.Envelope `json:"envelope"`
	Brief    []envelope.Line   `json:"brief"`
	Fatal    []envelopeFinding `json:"fatal"`
	Warn     []envelopeFinding `json:"warn"`
}

func cmdEnvelope(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, envelopeUsage)
		return 2
	}
	switch args[0] {
	case "check":
		return cmdEnvelopeCheck(args[1:])
	case "serve":
		return cmdEnvelopeServe(args[1:])
	}
	fmt.Fprintf(os.Stderr, "错误：不认识子命令 %q\n\n", args[0])
	fmt.Fprint(os.Stderr, envelopeUsage)
	return 2
}

func cmdEnvelopeCheck(args []string) int {
	fs := flag.NewFlagSet("envelope check", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "出 JSON（默认人读的六问六答）")
	flagArgs, posArgs := splitArgs(args, map[string]bool{"json": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) != 2 {
		fmt.Fprint(os.Stderr, envelopeUsage)
		return 2
	}
	vault, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	raw, err := os.ReadFile(posArgs[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：读不了信封文件：%v\n", err)
		return 1
	}
	e, err := envelope.Parse(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "信封读不懂：%v\n", err)
		return 1
	}
	o, err := org.Load(vault)
	if err != nil {
		// 真相源本身就是红的：绑不了。不编一份绿灯，直接说清去哪儿修。
		fmt.Fprintf(os.Stderr, "真相源加载失败：%v\n先修 org：anc org check %s\n", err, vault)
		return 1
	}
	issues := envelope.Bind(o, e, o.Policy)
	// 授权那一组与绑定同源：同一个 vault、同一个信封、同一份规则表。
	// 两个子命令（check / serve）走的是同一支 —— 不存在「查得出、拦不住」那种两套判法。
	gated, _ := envelope.Gate(o, e, o.Policy)
	issues = append(issues, gated...)
	fatal, warn := splitIssues(issues)

	if *asJSON {
		out := envelopeCheckOut{
			OK: len(fatal) == 0, Envelope: e, Brief: e.Brief(),
			Fatal: toEnvelopeFindings(fatal), Warn: toEnvelopeFindings(warn),
		}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误：序列化失败：%v\n", err)
			return 1
		}
		fmt.Println(string(b))
	} else {
		printEnvelopeCheck(e, fatal, warn)
	}
	if len(fatal) > 0 {
		return 1
	}
	return 0
}

func printEnvelopeCheck(e envelope.Envelope, fatal, warn []org.Issue) {
	fmt.Printf("信封 %s  ·  %s\n\n", e.ID, e.TS)
	for _, l := range e.Brief() {
		fmt.Printf("  %s %s\n", padRight(l.Q, 14), l.A)
	}
	fmt.Println()
	if len(fatal) == 0 && len(warn) == 0 {
		fmt.Println("判定：0 项发现（生效档位看 `anc org check <vault> --rules`）")
		return
	}
	fmt.Printf("判定：%d 项红 / %d 项非红档发现\n", len(fatal), len(warn))
	for _, i := range fatal {
		fmt.Printf("  🔴 [%s] %s\n", i.Rule, i.Msg)
	}
	for _, i := range warn {
		fmt.Printf("  🟡 [%s] %s\n", i.Rule, i.Msg)
	}
	if len(fatal) == 0 {
		fmt.Println("  （全是非红档：不拦，但请过目 —— 想提红就写进 company.md 的 policy 段）")
	}
}

func splitIssues(issues []org.Issue) (fatal, warn []org.Issue) {
	for _, i := range issues {
		if i.Level == org.LevelFatal {
			fatal = append(fatal, i)
		} else {
			warn = append(warn, i)
		}
	}
	return fatal, warn
}

func toEnvelopeFindings(in []org.Issue) []envelopeFinding {
	out := make([]envelopeFinding, 0, len(in))
	for _, i := range in {
		out = append(out, envelopeFinding{Rule: i.Rule, Level: string(i.Level), Where: i.Where, Msg: i.Msg})
	}
	return out
}

// ---------- 接入面：MCP 入口（anc envelope serve） ----------

const envelopeServeUsage = `anc envelope serve —— 起 ANC 的接入面（MCP，streamable HTTP）

用法：
  anc envelope serve <vault 目录> [--addr 127.0.0.1:8791] [--data <data 目录>]
                                     [--hot-addr 127.0.0.1:6379] [--hot-prefix anc]
                                     [--agent-stale 2m]

harness（Claude Code / Codex / …）通过 MCP 调工具。桥的两半都在这一个入口上：

  入站（agent 递给 ANC）  anc_send_envelope(who, on_behalf_of?, kind?, body, scope_domain?, scope_project?, refs?, needs?)
  出站（agent 向 ANC 要）  anc_read_context(who, on_behalf_of?, domain?)
  心跳（agent 报在线）    anc_agent_heartbeat(who, status?, task_id?, note?)

入站服务端做的事，一件不多：
  1 读工具参数 → 拼成一份信封 JSON → **走和线上同一条 Parse**（不给工具路径单开一套校验）
  2 绑真相源（Bind）：who / on_behalf_of / scope 指的东西存不存在
  3 落运行态日志：<data>/envelope/<YYYY-MM>.<who>.jsonl（按 who 分片，append-only）
  4 把「六问六答 + 发现」原样回给 harness

出站（W2 · 只读，一个字都不写回真相源）：
  问「这块业务是什么 / 数据在哪 / 卡住找谁 / 证据在哪」，回一份 JSON。
  可见范围 = render.VisibleDomains（与 persona 段 8 同一把尺子）：自己的域全行，
  别人的域只给「找谁」；要别人的域**拒**，但指路。身份不许自报：解不出 = 这个人不存在。
  每次调用留一条痕进 audit（谁 / 要什么 / 给没给 / 为什么拒）。
  还捎上「派给我的待办」（热层里 to / by 指向你的那些）—— **反向下发**就靠这一块：
  agent 在自己的会话里 pull，ANC 不去插话、不去打断（插话就是打断：上一条还没跑完，
  第二条已经在改前提了）。任务本体住热层（当前值），达标的落 timeline（历史）。
  **热层接不上不编**：报缺说「这一段答不了」，不拿空清单冒充「没人派给你」。

门禁口径（与 envelope check 完全一致，因为走的是同一份规则表、同一支判法）：
  绑定那一组发现默认 warn = **照收**（落盘 + 回话）；被 company.md 的 policy 段提成 fatal 的，
  按 fatal 的定义（阻止落盘）**拒收**：不写信封日志、回 isError=true。
  **授权那一组**（grant.match.missing）出厂就是 fatal：跨域信封没有 grant 覆盖 → 拒收。
  想开就开、想关就关，开关在数据里，不在这段代码里。
  拒也留一条痕：被拒的信封进不了信封日志，所以另落一条 audit（result=denied，SPEC §6 不变量 4）。
  跨域且被授权的那次同样留痕（result=ok）—— 审计不是只记拒绝。

无状态：不发 Mcp-Session-Id、不记会话、不提供 SSE 流 —— 状态在真相源与日志里，不在连接里。

安全口径：默认只绑本机（127.0.0.1）。要给别人用请走 ssh 端口转发，别把 0.0.0.0 开出去。

退出：Ctrl-C。
`

// hotLister 只取装配要的那一件事（读一摞当前值）—— 出口不关心 TTL / 认领那一套。
type hotLister interface {
	List() ([]hot.State, error)
}

// agentHeartbeats 是业务 agent 的在线态：写一次心跳、读最后心跳、回新鲜度阈值。
// 与 hotLister 分开：读出口只关心任务，心跳是另一条路径 —— 混成一个接口，
// 会逼着只想要读出口的实现也去做写口。
type agentHeartbeats interface {
	BeatAgent(hot.AgentHeartbeat) (hot.AgentHeartbeat, error)
	ListAgents() ([]hot.AgentHeartbeat, error)
	AgentStale() time.Duration
}

// envelopeIngress 是接入面本身：一个 vault + 一个 data 目录 + 一个热层 + 一个时钟。
type envelopeIngress struct {
	Vault   string
	DataDir string
	Now     func() time.Time
	// Hot 是热层（「派给我的待办」从这儿来）。nil = 这一块没接上：读出口如实报缺，
	// 不拿空清单冒充「没人派给你」（同 `anc board serve --hot-addr` 的口径）。
	Hot hotLister
	// Agents 是同一个热层的心跳视角。nil = 没接上：心跳工具如实报错，不假装记下了。
	Agents agentHeartbeats
}

func (g *envelopeIngress) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

// srv 装配 MCP 服务端：**桥的两半挂在这一个入口上** —— 递进来（信封）与要出去（读出口）。
// 两个工具都是「一次调用 = 一条痕」，所以 #44「入口唯一性」没被破：多的不是入口，
// 是同一个入口上的第二个动作。
func (g *envelopeIngress) srv() *mcp.Server {
	return &mcp.Server{
		Name: "anc-envelope", Version: version,
		Tools: []mcp.Tool{{
			Name: "anc_send_envelope",
			Description: "把一个信封递给 ANC（接入面）。who 必须是 vault 里真实存在的成员/bot 名 —— " +
				"身份不许自报：解不出来 ANC 会如实报出来（默认只告警、不拦）。" +
				"scope_domain 写**别人的**域 = 跨域：没有 grant 覆盖会**被拒**（数据默认不通，SPEC §6-12）。" +
				"on_behalf_of 写你代谁：member:名字 | role:岗位 | domain:slug，或直接写名字。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"who":           map[string]any{"type": "string", "description": "发件人（vault 里真实存在的成员/bot 名）"},
					"on_behalf_of":  map[string]any{"type": "string", "description": "代谁：member:名字 | role:岗位 | domain:slug，或裸名字"},
					"kind":          map[string]any{"type": "string", "description": "ask / report / notify / ingest / proposal（认不出的照收）"},
					"body":          map[string]any{"type": "string", "description": "正文"},
					"scope_domain":  map[string]any{"type": "string", "description": "哪块业务（domains.md 的 slug）"},
					"scope_project": map[string]any{"type": "string", "description": "哪个项目（projects.md 的 slug）"},
					"refs":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "证据在哪"},
					"needs":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "要不要人拍（自由文本）"},
				},
				"required": []string{"who", "body"},
			},
			Call: g.sendEnvelope,
		}, {
			Name: "anc_read_context",
			Description: "向 ANC 要上下文（**只读**）：这块业务是什么、数据在哪、卡住找谁。" +
				"who 必须是 vault 里真实存在的成员名 —— 身份不许自报，解不出 ANC 会说没有这个人。" +
				"domain 可选（domains.md 的 slug）：不写 = 你负责的域；写别人的域会被拒，" +
				"但会告诉你要找谁（数据默认不通，跨域按「找谁」接头）。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"who":          map[string]any{"type": "string", "description": "你是谁（vault 里真实存在的成员名）"},
					"on_behalf_of": map[string]any{"type": "string", "description": "可选：你代谁（member:名字 | role:岗位 | domain:slug，或裸名字）"},
					"domain":       map[string]any{"type": "string", "description": "可选：只问这一块业务（domains.md 的 slug）；不写 = 你负责的域"},
				},
				"required": []string{"who"},
			},
			Call: g.readContext,
		}, {
			Name: "anc_agent_heartbeat",
			Description: "业务 agent 报一次心跳（**写当前态**，不是历史）：谁还在线、现在在干什么。" +
				"who 必须是 agents.md 里真实存在的业务 agent（slug 或 name）—— 不存在的身份不收，" +
				"免得看板上多出一个查无此人的幽灵。" +
				"心跳过期不删，按服务端阈值打「不新鲜」标记；看板据此区分「在线」与「最后心跳」。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"who":     map[string]any{"type": "string", "description": "业务 agent 的 slug 或 name（agents.md 里必须存在）"},
					"status":  map[string]any{"type": "string", "description": "可选：running / idle / blocked / failed（认不出的词照收）"},
					"task_id": map[string]any{"type": "string", "description": "可选：当前在做的 task id"},
					"note":    map[string]any{"type": "string", "description": "可选：一句话说明"},
				},
				"required": []string{"who"},
			},
			Call: g.agentHeartbeat,
		}},
		Log: func(format string, a ...any) { fmt.Printf("  · "+format+"\n", a...) },
	}
}

// agentHeartbeat 收一次业务 agent 的心跳。
//
// 身份先过 agents.md：只有**已登记**的业务 agent 才能报心跳。理由很直接 ——
// 看板上的「在线」若允许自报，任何人都能把一个查无此人的名字点亮；那一条绿灯就是假的。
func (g *envelopeIngress) agentHeartbeat(args map[string]any) (string, bool) {
	who := strings.TrimSpace(strArg(args, "who"))
	if who == "" {
		return "心跳需要 who（agents.md 里的 slug 或 name）", true
	}
	if g.Agents == nil {
		return "热层没接上 —— 心跳无处可写（起接入面时给 --hot-addr 或设 ANC_HOT_ADDR）", true
	}
	o, err := org.Load(g.Vault)
	if err != nil {
		return "真相源加载失败，认不出这个 agent：" + err.Error() + "\n（先修 org：anc org check <vault>）", true
	}
	a, why := findAgent(o, who)
	if why != "" {
		return why, true
	}
	h, err := g.Agents.BeatAgent(hot.AgentHeartbeat{
		Slug:   a.Slug,
		Status: strings.TrimSpace(strArg(args, "status")),
		TaskID: strings.TrimSpace(strArg(args, "task_id")),
		Note:   strings.TrimSpace(strArg(args, "note")),
	})
	if err != nil {
		return "心跳没记上：" + err.Error(), true
	}
	return mustJSON(map[string]any{
		"ok":               true,
		"slug":             h.Slug,
		"name":             a.Name,
		"domain":           a.Domain,
		"role":             a.Role,
		"status":           h.Status,
		"task_id":          h.TaskID,
		"note":             h.Note,
		"as_of":            h.AsOf,
		"stale_after_secs": int64(g.Agents.AgentStale() / time.Second),
	}), false
}

// findAgent 把调用方写的身份解成一个已登记的业务 agent。
// 先认 slug，再认 name；都认不出就回一句「有哪些」—— 不让它去猜。
func findAgent(o *org.Org, who string) (org.Agent, string) {
	for _, a := range o.Agents {
		if a.Slug == who {
			return a, ""
		}
	}
	for _, a := range o.Agents {
		if a.Name == who {
			return a, ""
		}
	}
	for _, a := range o.Agents {
		if strings.EqualFold(a.Slug, who) || strings.EqualFold(a.Name, who) {
			return a, ""
		}
	}
	slugs := make([]string, 0, len(o.Agents))
	for _, a := range o.Agents {
		slugs = append(slugs, a.Slug)
	}
	if len(slugs) == 0 {
		return org.Agent{}, "agents.md 里没有业务 agent —— 这个身份查无此人"
	}
	return org.Agent{}, fmt.Sprintf("agents.md 里没有 %q —— 身份不许自报。现有：%s", who, strings.Join(slugs, "、"))
}

// sendEnvelope 收一封信：拼信封 → Parse → Bind → 落盘 → 回六问六答。
func (g *envelopeIngress) sendEnvelope(args map[string]any) (string, bool) {
	now := g.now()
	id := strArg(args, "id")
	if id == "" {
		// 服务端补 id 与 ts：harness 不该为「这封信叫什么」操心。
		id = fmt.Sprintf("e-%d", now.UnixNano())
	}
	wire := map[string]any{"id": id, "ts": now.Format(time.RFC3339)}
	for _, k := range []string{"who", "on_behalf_of", "kind", "body"} {
		if v := strArg(args, k); v != "" {
			wire[k] = v
		}
	}
	scope := map[string]any{}
	if v := strArg(args, "scope_domain"); v != "" {
		scope["domain"] = v
	}
	if v := strArg(args, "scope_project"); v != "" {
		scope["project"] = v
	}
	if len(scope) > 0 {
		wire["scope"] = scope
	}
	if v := listArg(args, "refs"); len(v) > 0 {
		wire["refs"] = v
	}
	if v := listArg(args, "needs"); len(v) > 0 {
		wire["needs"] = v
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		return "拼信封失败：" + err.Error(), true
	}
	e, err := envelope.Parse(raw)
	if err != nil {
		return "这封信读不懂，没收：" + err.Error(), true
	}
	o, err := org.Load(g.Vault)
	if err != nil {
		return "真相源加载失败，绑不了：" + err.Error() + "\n（先修 org：anc org check <vault>）", true
	}
	issues := envelope.Bind(o, e, o.Policy)
	// 授权那一组：主体（on_behalf_of 优先）＋客体（scope.domain）有没有 grant 覆盖。
	// verdict 不是装饰 —— 「行使必留痕」要有痕可留（下面两个审计落点全靠它）。
	gated, verdict := envelope.Gate(o, e, o.Policy)
	issues = append(issues, gated...)
	fatal, warn := splitIssues(issues)

	var b strings.Builder
	if len(fatal) > 0 {
		// fatal 在这个库里的定义就是「阻止落盘」（org/rules.go）—— 所以拒收：不写信封日志。
		// **但拒也要留一条痕**（SPEC §6 不变量 4「行使必留痕」，含被拒的那一次）：
		// 被拒的信封进不了信封日志，所以那一条痕必须落在这儿 —— 否则「谁想递什么、
		// 为什么被拒」一个字都不剩，而「想行使但没被授权」恰恰是审计最该答的那一问。
		fmt.Fprintf(&b, "拒收（%d 项红档；信封日志没写）：\n", len(fatal))
		for _, i := range fatal {
			fmt.Fprintf(&b, "  🔴 [%s] %s\n", i.Rule, i.Msg)
		}
		g.auditEnvelope(verdict, audit.ResultDenied, denyWhy(fatal), now)
		return b.String(), true
	}
	// 跨域且进了门 = 一次真行使 → 留一条 ok 的痕。域内信封不通 grant，也就不进 audit
	// （它不是一次「跨域 / 跨通道」行使，信封日志已经记着它）。
	if verdict.CrossDomain {
		g.auditEnvelope(verdict, audit.ResultOK, gateNote(verdict), now)
	}
	rec := envelope.Record{Envelope: e, At: now.Format(time.RFC3339), Findings: envelope.ToFindings(issues)}
	path, err := envelope.Append(g.DataDir, rec)
	if err != nil {
		return "收了，但落不了盘（这次不算数）：" + err.Error(), true
	}
	fmt.Fprintf(&b, "收了，已落 %s\n\n", path)
	for _, l := range e.Brief() {
		fmt.Fprintf(&b, "  %s：%s\n", l.Q, l.A)
	}
	if len(warn) == 0 {
		b.WriteString("\n绑定：0 项发现。")
		return b.String(), false
	}
	fmt.Fprintf(&b, "\n绑定：%d 项非红档发现（照收，只是记下来）：\n", len(warn))
	for _, i := range warn {
		fmt.Fprintf(&b, "  🟡 [%s] %s\n", i.Rule, i.Msg)
	}
	return b.String(), false
}

func strArg(args map[string]any, k string) string {
	if v, ok := args[k].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func listArg(args map[string]any, k string) []string {
	raw, ok := args[k].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

// auditEnvelope 给一次**跨域**入站行使留一条痕（SPEC §6 不变量 4）。
//
// 什么时候记：只有跨域才记。域内信封不通 grant，也就不是一次「跨域 / 跨通道」行使。
//
// 结果怎么填（这里有一条**如实记**的边界）：
//   - 跨域 + 命中一条 grant → ok，`Detail` 写明是哪一条；
//   - 跨域 + 没有覆盖 + 拒收  → denied（想行使，权没给）；
//   - 跨域 + 没有覆盖 + 规则是 warn 档照收 → **ok**，`Why` 写明「照收」。
//     它确实进来了，写成 denied 是假话；audit 的四档（ok/denied/failed/unknown）答不了
//     「照收但未授权」这一档，所以那件事由**信封日志的 findings** 记着（两条腿互补）。
//
// **类别先记 invoke**：信封的 action 这一维**还没定**（信封里就没有这个字段，见 gate.go 注释）——
// 这里记的是「行使了**接入面这一个调用**」，不是「这封信的业务动作」。等 kind → action 那张表
// 拍出来，改的就是这一行（届时它应该变成数据）。
func (g *envelopeIngress) auditEnvelope(v envelope.GateOutcome, result, why string, at time.Time) {
	actor := v.Subject
	if actor == "" {
		actor = "unknown"
	}
	object := v.Domain
	if object != "" {
		object = "domain:" + object
	}
	rec := audit.Record{
		ID:         fmt.Sprintf("send-%d-%s", at.UnixNano(), audit.ShardName(actor)),
		At:         at.Format(time.RFC3339),
		Actor:      actor,
		Action:     audit.ActionInvoke,
		Object:     object,
		Result:     result,
		Why:        clipRunes(why, 200),
		OnBehalfOf: v.OnBehalfOf,
		Tool:       "anc_send_envelope",
		Source:     "serve",
		Detail:     grantDetail(v),
	}
	if _, err := audit.Append(g.Vault, rec); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  入站信封没留上痕（audit）：%v\n", err)
	}
}

// denyWhy 把拒收的理由折成一句（取第一条红档的原文）—— 审计要的是当时那句话，不是转述。
func denyWhy(fatal []org.Issue) string {
	if len(fatal) == 0 {
		return ""
	}
	return fatal[0].Rule + "：" + fatal[0].Msg
}

// gateNote 是「进了门」时的补充说明：命中 grant 就不写；靠 warn 档进来的要写明。
func gateNote(v envelope.GateOutcome) string {
	if v.Requesting {
		return "提案：请求授权 —— 不判授权（提案不是行使；否则还没拿到权的人永远提不出请求）"
	}
	if v.Grant == "" {
		return "跨域未见 grant 覆盖：grant.match.missing 是 warn 档，照收（这条发现记在信封日志里）"
	}
	return ""
}

func grantDetail(v envelope.GateOutcome) string {
	if v.Grant == "" {
		return ""
	}
	return "grant " + v.Grant
}

// clipRunes 按 rune 截断（不是按字节）—— 按字节切会把一个汉字劈成两半，落盘就是坏码。
func clipRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func cmdEnvelopeServe(args []string) int {
	fs := flag.NewFlagSet("envelope serve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8791", "监听地址")
	data := fs.String("data", "", "运行态 data 目录（信封日志落这儿）")
	hotAddr := fs.String("hot-addr", "", "热层地址（读出口的「派给我的待办」用它；默认 "+hot.DefaultAddr+"；也可用 ANC_HOT_ADDR）")
	hotPrefix := fs.String("hot-prefix", "", "热层 key 前缀（默认 "+hot.DefaultPrefix+"；也可用 ANC_HOT_PREFIX）")
	agentStale := fs.Duration("agent-stale", 0, "多久没心跳不算在线（默认 "+hot.DefaultAgentStale.String()+"；也可用 ANC_AGENT_STALE）")
	flagArgs, posArgs := splitArgs(args, map[string]bool{})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) != 1 {
		fmt.Fprint(os.Stderr, envelopeServeUsage)
		return 2
	}
	vault, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	if st, err := os.Stat(vault); err != nil || !st.IsDir() {
		fmt.Fprintf(os.Stderr, "错误：vault 目录不存在或不是目录：%s\n", vault)
		return 1
	}
	dataDir := *data
	if dataDir == "" {
		dataDir = filepath.Join(filepath.Dir(vault), "data")
	} else if a, err := filepath.Abs(dataDir); err == nil {
		dataDir = a
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "错误：建不了 data 目录：%v\n", err)
		return 1
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：监听 %s 失败：%v\n", *addr, err)
		return 1
	}
	defer func() { _ = ln.Close() }()

	urlHost := ln.Addr().String()
	if tcp, ok := ln.Addr().(*net.TCPAddr); ok {
		if tcp.IP.IsUnspecified() {
			urlHost = net.JoinHostPort("127.0.0.1", strconv.Itoa(tcp.Port))
		}
		if !tcp.IP.IsLoopback() {
			fmt.Fprintf(os.Stderr,
				"⚠️  绑到了 %s：接入面只该给本机（或经 ssh 端口转发的人）用。\n", ln.Addr())
		}
	}

	// 起之前先看一眼真相源：红的照样起（接入面正是用来看「哪里坏了」的），但要说出来。
	if o, err := org.Load(vault); err == nil {
		printIssues(o.Warnings)
	} else {
		fmt.Fprintf(os.Stderr, "⚠️  真相源现在有红档发现（绑定会带上它们）：\n%v\n", err)
	}

	// 热层：「派给我的待办」从这儿来。接不上**不拦接入面** —— 递信封、要上下文照样能用，
	// 读出口会把「这一段答不了」如实写进 gaps（同 board serve 那条口径）。
	// 只在接上了才往接口里塞：`var st *hot.Store` 直接塞进去会变成一个
	// 「非 nil 的接口、里面是个 nil 指针」—— `g.Hot == nil` 于是判不出来，
	// 读出口会走进 (*hot.Store)(nil).List() 当场 panic。没接上就让它一直是 nil。
	var hotStore *hot.Store
	hotAddrShown := firstNonEmpty(*hotAddr, os.Getenv("ANC_HOT_ADDR"), hot.DefaultAddr)
	if st, err := hot.Open(hot.Config{
		Addr:       hotAddrShown,
		Prefix:     firstNonEmpty(*hotPrefix, os.Getenv("ANC_HOT_PREFIX")),
		AgentStale: firstDuration(*agentStale, envDuration("ANC_AGENT_STALE")),
	}); err != nil {
		fmt.Fprintf(os.Stderr, "注意：热层没接上（%v）—— 读出口里「派给我的待办」那一段会如实报缺。\n", err)
	} else {
		defer st.Close()
		hotStore = st
	}

	g := &envelopeIngress{Vault: vault, DataDir: dataDir, Now: time.Now}
	if hotStore != nil {
		// 同一个底层对象、两条视角：任务（读出口）与心跳（agent 在线态）。
		// 只在接上时才塞接口 —— 否则会变成「非 nil 的接口里装着 nil 指针」（见上面那段注释）。
		g.Hot = hotStore
		g.Agents = hotStore
	}
	fmt.Printf("ANC 接入面已起（MCP · streamable HTTP · 无状态）：\n")
	fmt.Printf("  地址   http://%s/mcp\n", urlHost)
	fmt.Printf("  vault  %s\n", vault)
	fmt.Printf("  日志   %s\n", filepath.Join(dataDir, envelope.DirName))
	fmt.Printf("  工具   anc_send_envelope（递进来）· anc_read_context（要出去）· anc_agent_heartbeat（报在线）\n")
	fmt.Printf("  热层   %s（「派给我的待办」走它；没接上就如实报缺，不编）\n", hotAddrShown)
	fmt.Printf("  接入   claude mcp add --transport http anc http://%s/mcp\n", urlHost)
	fmt.Printf("  退出   Ctrl-C\n\n")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	srv := &http.Server{Handler: g.srv().Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case <-ctx.Done():
		fmt.Println("\n收到中断，正在关闭接入面…")
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
