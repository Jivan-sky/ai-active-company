package board

import (
	"net/http"

	"anc/internal/ctxpack"
	"anc/internal/org"
)

// ContextSchema 是这一份视图的版本号（同 /api/hot 的理由）。
const ContextSchema = "anc.context/v1"

// ContextView 是 `/api/context` 的形状 —— 「一个 agent 的工作上下文是什么」。
//
// 与其余几份视图的差别，两条：
//
//   - 背后没有别的服务：读的就是真相源本身（热层那种才有 wired / why 那一对）——
//     加载不了的时候 error 里说清，前端照它显示。
//   - 名单（roster）与那一份上下文在同一份回话里：看板是**观测面**，一页该自己答完，
//     不让前端去别的端点拼。名单空 = 真相源里没有成员也没有业务 agent（不是「读不到」，
//     读不到走 error）。
//
// 「哪几个字段能出」由 ctxpack 那两份结构定死（它们是 agent 那条路上的契约）——
// 看板这一页**不另立一份**：同一份上下文，换一个摆法而已。
type ContextView struct {
	Schema string                `json:"schema"`
	Who    string                `json:"who"`
	Domain string                `json:"domain"`
	Roster []ctxpack.IdentityOut `json:"roster"`
	Pack   *ctxpack.Out          `json:"pack,omitempty"`
	Error  string                `json:"error,omitempty"`
	Note   string                `json:"note"`
}

// handleContext 答「这个 agent 的工作上下文是什么」。
//
// 只读：这一页一个字都不写回真相源（要一份冻住的副本是 CLI 那件事 —— `anc context freeze`
// 落到 vault 的 context/ 里，那里会进 git；看板只摆出来看）。
//
// 拒（这个人不存在 / 越域）**不是 HTTP 错误**：照回 200，拒话在 pack.refused 里 ——
// 看板要把它**显示**出来（同 /api/issues 的口径），而不是当故障处理。
func (s *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	view := ContextView{
		Schema: ContextSchema,
		Roster: []ctxpack.IdentityOut{},
		Who:    ctxpack.SafeToken(q.Get("who")),
		Domain: ctxpack.SafeToken(q.Get("domain")),
		Note: "这一页摆的是**算出来的只读投影**：身份 / 业务域 / 数据源 / 可见范围 / 技能来源 / 在跟的项目，" +
			"与 harness 走 MCP 要的那一份、`anc context pack` 打的那一份**同一个装配点**。" +
			"真相源仍是 vault 里的 markdown —— 改完刷新即见。" +
			"看板不落盘：要一份冻住的副本用 `anc context freeze`（落 vault 的 context/，进 git）。",
	}
	o, err := org.Load(s.Vault)
	if err != nil {
		view.Error = s.scrub(firstLine(err.Error()))
		writeJSON(w, http.StatusOK, view)
		return
	}
	// 名单：真相源里声明的成员与业务 agent —— 编号是它们的（不许自报那条纪律的延伸）。
	// 走 o.Identity 是**唯一**一条「编号 → 主体」的转换，看板不自己拼一份。
	for _, m := range o.Members {
		if idn, ok := o.Identity(m.Name); ok {
			view.Roster = append(view.Roster, *ctxpack.IdentityOf(o, idn))
		}
	}
	for _, a := range o.Agents {
		if idn, ok := o.Identity(a.Slug); ok {
			view.Roster = append(view.Roster, *ctxpack.IdentityOf(o, idn))
		}
	}
	if view.Who != "" {
		out, _ := ctxpack.Assemble(o, s.Vault, view.Who, "", view.Domain, s.now())
		view.Pack = &out
	}
	writeJSON(w, http.StatusOK, view)
}
