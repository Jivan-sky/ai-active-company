package board

import (
	"net/http"
	"path/filepath"

	"anc/internal/org"
	"anc/internal/probe"
	renderpkg "anc/internal/render"
)

// RuntimeView 是 `/api/runtime` 的形状 —— 运行态观测，不是真相源投影。
//
// 刻意**不**并进 `anc.board/v2`：那个契约回答「真相源现在长什么样」，这个回答
// 「运行态现在健不健康」—— 一份读 vault、一份读 gateway 的 data 目录，来源不同，
// 变化节奏也不同（改组织是手动动作，gateway 每分钟都在动）。混进同一个 schema，
// 第一个消费方就得跟着一起升版本。（同 `/api/issues` 的理由，见 server.go。）
//
// 也刻意**不带** vault / config / data 的绝对路径：看板是观测面，那些字段对读者
// 没有信息量，只会泄漏本机布局（SPEC §2.3）。要看路径用 `anc probe`。
type RuntimeView struct {
	Schema string `json:"schema"`
	// Wired=false 表示看板没接 data 目录 —— 前端如实写「未接入」，不画假的绿灯。
	Wired      bool            `json:"wired"`
	Gateway    string          `json:"gateway"` // up / down；Wired=false 时是空串
	GatewayWhy string          `json:"gateway_why,omitempty"`
	Bots       []probe.Finding `json:"bots"`
	Extras     []string        `json:"extras,omitempty"`
	// Handlers = 出问题该交给谁（company.admins，解成「人名（岗位）」）。
	// 探针只观测、不做自动处置，所以报红必须连带说清「交给谁」，否则等于把问题丢在地上。
	Handlers   []string `json:"handlers"`
	HandlerWhy string   `json:"handler_why,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// handleRuntime 出运行态观测。用 200 而不是 5xx/4xx：gateway 挂着不是「请求错了」，
// 而是**它就是来报这件事的** —— 前端要把它显示出来。
func (s *Server) handleRuntime(w http.ResponseWriter, r *http.Request) {
	if s.DataDir == "" {
		writeJSON(w, http.StatusOK, RuntimeView{
			Schema: probe.Schema, Wired: false, Bots: []probe.Finding{}, Handlers: []string{},
			Error: "看板没带 data 目录：用 `anc board serve <vault> --data <data_dir>` 起，运行态才接得上",
		})
		return
	}
	opt := probe.Options{
		Vault:   s.Vault,
		Config:  filepath.Join(filepath.Dir(s.Vault), "gateway", "config.toml"),
		DataDir: s.DataDir,
	}
	handlerWhy := ""
	if o, err := org.Load(s.Vault); err == nil {
		opt.Admins = o.AdminLabels()
		// 「还没接凭据」也是真相源里的**声明** —— 看板必须跟 `anc probe` 用同一支算，
		// 否则会出现「CLI 说灰、看板说黄」两张互相打脸的报告。
		opt.Unwired = renderpkg.UnwiredProjects(o)
		if len(opt.Admins) == 0 {
			handlerWhy = "真相源里 company.admins 是空的 —— 现在没人可交"
		}
	} else {
		handlerWhy = "真相源现在读不动，算不出该交给谁（不猜）：" + s.scrub(firstLine(err.Error()))
	}

	rep, err := probe.Run(opt)
	if err != nil {
		writeJSON(w, http.StatusOK, RuntimeView{
			Schema: probe.Schema, Wired: true, Bots: []probe.Finding{}, Handlers: opt.Admins, HandlerWhy: handlerWhy,
			Error: "探针跑不动（多半是 gateway config 不在这）：" + s.scrub(err.Error()),
		})
		return
	}
	view := RuntimeView{
		Schema:     rep.Schema,
		Wired:      true,
		Gateway:    rep.Gateway,
		GatewayWhy: s.scrub(rep.GatewayWhy),
		Bots:       scrubBots(rep.Bots, s.scrub),
		Extras:     rep.Extras,
		Handlers:   rep.Handlers,
		HandlerWhy: handlerWhy,
	}
	writeJSON(w, http.StatusOK, view)
}

// scrubBots 逐条摘掉本机路径 —— 判据文案里可能带 socket / 会话文件的落点。
func scrubBots(list []probe.Finding, scrub func(string) string) []probe.Finding {
	out := make([]probe.Finding, 0, len(list))
	for _, f := range list {
		f.Why = scrub(f.Why)
		out = append(out, f)
	}
	return out
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}
