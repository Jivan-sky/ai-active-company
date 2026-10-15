package board

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"anc/internal/org"
	"anc/internal/render"
)

// DataflowSchema 是这一份视图的版本号。与 anc.board/v1 / anc.runtime/v1 各管各的 ——
// 三份契约来源不同、变化节奏不同，谁也不许捎带升别人的版本。
const DataflowSchema = "anc.dataflow/v1"

// DataflowView 是 `/api/dataflow` 的形状 —— 「谁可以做什么、通道开着吗」的**策略面**。
//
// 为什么不并进 anc.board/v1：那份投影只读真相源（改组织才动）；这一份要看**执行面** ——
// gateway 实际吃的那份 config.toml 里到底写着什么。两个来源、两个变化节奏（同 /api/runtime）。
//
// 为什么观测面可以 import 渲染器：方向是对的（看板读渲染器的判据），反过来才越界 ——
// render 依赖 board 会让「生成配置」这条路径够得着看板的形状。
//
// 刻意**不带** open_id / app_id / 密钥：看板是观测面，不是凭据面（SPEC §2.3 / §6-3）。
// 入站授权只出**人数**，不出标识符。
//
// 这一页现在**只有策略面**。事件面（谁尝试连了哪、有没有越权尝试）没有数据源 ——
// SPEC §6 要求「必须记尝试」，审计还没实现，所以这里如实写缺，不画通道图充数。
type DataflowView struct {
	Schema  string     `json:"schema"`
	Wired   bool       `json:"wired"` // false = 真相源读不动，策略面算不出来
	Relay   RelayView  `json:"relay"`
	Exec    ExecView   `json:"exec"`
	Bots    []BotGrant `json:"bots"`
	Gaps    []string   `json:"gaps"`    // 结构性风险：渲染器既有判据（render.SecurityGaps）
	Missing []string   `json:"missing"` // 这一页现在缺什么数据源 —— 如实列，不编
	Error   string     `json:"error,omitempty"`
}

// RelayView 是 bot 之间的通道。上游默认是**开着**的（timeout_secs=120），
// 所以「执行面里没写这一段」本身就是要说出来的一件事，不是一个空值。
type RelayView struct {
	Declared    bool   `json:"declared"`
	TimeoutSecs int    `json:"timeout_secs"`
	Note        string `json:"note"`
}

// ExecView 是执行面的身份：这份 config 是渲染产物吗、按哪份真相源生成、里头有哪些 project。
type ExecView struct {
	ConfigPresent  bool     `json:"config_present"`
	HasFingerprint bool     `json:"has_fingerprint"`
	Version        string   `json:"version,omitempty"`
	Inputs         string   `json:"inputs,omitempty"`
	GeneratedAt    string   `json:"generated_at,omitempty"`
	Projects       []string `json:"projects"` // 执行面里带 anc persona 块的 project
	Note           string   `json:"note,omitempty"`
}

// BotGrant 是「这个 bot 手里有什么」—— 全部来自真相源（渲染器的输入）。
// Tools 就是 harness 白名单：空数组是一种确定的档（没预授权），不是「未知」。
type BotGrant struct {
	Project      string   `json:"project"`
	Role         string   `json:"role"`
	Mode         string   `json:"mode"`
	Model        string   `json:"model"`
	Tools        []string `json:"tools"`
	Inbound      int      `json:"inbound"`       // 入站授权人数（本人那条 open_id 算 1）
	InboundExtra int      `json:"inbound_extra"` // 额外放行的人数（只出个数）
}

// handleDataflow 出数据流的策略面。用 200 而不是 5xx：读不到执行面不是「请求错了」，
// 而是**它就是来报这件事的**（同 /api/runtime）。
func (s *Server) handleDataflow(w http.ResponseWriter, r *http.Request) {
	view := DataflowView{
		Schema: DataflowSchema,
		Bots:   []BotGrant{},
		Gaps:   []string{},
		Missing: []string{
			"事件面：谁尝试连了哪、有没有越权尝试 —— SPEC §6 要求「必须记尝试」，审计还没实现",
			"授权表不在这页（它有自己的「授权」页）：这一页只管通道与工具白名单；grant 的**执行语义**（期限到点、降级后还剩什么）仍未落地 —— 议题 #32–#35",
		},
	}
	o, err := org.Load(s.Vault)
	if err != nil {
		view.Error = "真相源读不动，策略面算不出来（不猜）：" + s.scrub(firstLine(err.Error()))
		writeJSON(w, http.StatusOK, view)
		return
	}
	view.Wired = true
	view.Bots = botGrants(o)

	text, rerr := os.ReadFile(filepath.Join(filepath.Dir(s.Vault), "gateway", "config.toml"))
	if rerr != nil {
		view.Exec.Note = "读不到执行面（gateway config 不在这）：" + s.scrub(rerr.Error()) +
			" —— 下面是真相源算出来的策略；它有没有真的落到 gateway 上，现在答不了。"
		writeJSON(w, http.StatusOK, view)
		return
	}
	view.Exec.ConfigPresent = true
	view.Exec.Projects = projectsIn(string(text))
	view.Gaps = render.SecurityGaps(string(text))
	if view.Gaps == nil {
		view.Gaps = []string{} // 空档也要是 []：前端直接 .map/.length，null 会把它打崩
	}
	if n, ok := render.RelayIn(string(text)); ok {
		view.Relay.Declared = true
		view.Relay.TimeoutSecs = n
	}
	switch {
	case !view.Relay.Declared:
		view.Relay.Note = "执行面里没有 [relay] 段 —— 上游默认 timeout_secs=120，等于通道开着，且没人显式同意过。"
	case view.Relay.TimeoutSecs == 0:
		view.Relay.Note = "通道关着（v1 口径：机制保留、默认零绑定）。要通得走授权模型（SPEC §6）。"
	default:
		view.Relay.Note = "通道开着，且不是 v1 口径的 0 —— 谁开的、谁同意的，这一页答不了。"
	}
	if v, inputs, at, ok := render.Fingerprint(string(text)); ok {
		view.Exec.HasFingerprint = true
		view.Exec.Version, view.Exec.Inputs, view.Exec.GeneratedAt = v, inputs, at
	} else {
		view.Exec.Note = "这份 config 没有 anc 指纹：手写或他源配置 —— 策略面与执行面对不对得上，无从判断。"
	}
	writeJSON(w, http.StatusOK, view)
}

// botGrants 逐 bot 算策略面。顺序按成员名排，让两次刷新之间可以逐字节 diff。
func botGrants(o *org.Org) []BotGrant {
	members := o.Enabled()
	sort.SliceStable(members, func(i, j int) bool { return members[i].Name < members[j].Name })
	out := make([]BotGrant, 0, len(members))
	for _, m := range members {
		role := o.Roles[m.Role]
		tools := append([]string{}, role.AllowedTools...)
		sort.Strings(tools)
		inbound := 0
		if strings.TrimSpace(m.Feishu.OpenID) != "" {
			inbound = 1
		}
		out = append(out, BotGrant{
			Project: render.ProjectName(o.Company.ID, m.Name),
			Role:    m.Role, Mode: role.Mode, Model: o.ModelFor(m),
			Tools:   tools,
			Inbound: inbound, InboundExtra: len(m.Feishu.ExtraAllowFrom),
		})
	}
	return out
}

// projectsIn 数出执行面里的 project —— 用渲染器自己那个扫法（render.ProjectsIn），
// 不另写一套：两套扫法迟早各说各话。排序只为让两次刷新可以逐字节 diff。
func projectsIn(text string) []string {
	out := render.ProjectsIn(text)
	sort.Strings(out)
	return out
}
