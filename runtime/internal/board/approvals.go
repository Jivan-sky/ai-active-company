package board

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"anc/internal/approvals"
)

// WritePath 是看板的**唯一写口**：审批信号。
//
// 为什么它不算「第二条写入口」—— SPEC §7：CLI / 对话式 bot / 看板是**同一个变更管道的三个前端**。
// 在看板上点「同意」与在终端敲 `anc approvals decide`，走的是**同一个入口、同一份留痕**：
// 「收提案、送给该批的人、留痕」这三件事都在 `internal/approvals` 里，这里只把它接到 HTTP 上。
//
// 三道限制，都是**管道**的性质，不是「谁有权限」的规则（权限那层由 SPEC §6 的 `panel` 客体定，
// 现在**没落**）：
//
//  1. 只放行 `POST` + `application/json`：跨站表单发不出这个 Content-Type
//     （浏览器会先发预检，而我们不回 CORS 头）—— 所以没有 token 也挡得住「别的网页替你点头」；
//  2. 请求体限 64KB；
//  3. **不校「点的人是不是该批的人」**：`by` 原样留痕，判断交给 agent（2026-10-10 口径）。
const WritePath = "/api/approvals/decide"

// ApprovalView 是 `/api/approvals` 的形状（schema = anc.approvals/v1）。
//
// 与 `/api/runtime` 同一个规矩：**内容有问题也回 200**，把问题显示出来，不装成故障。
type ApprovalView struct {
	Schema string `json:"schema"`
	// Wired=false 表示热层没接上 —— 看板读不到待批就如实说，不拿一份空队列冒充「没人提」。
	Wired bool   `json:"wired"`
	Why   string `json:"why,omitempty"`
	Error string `json:"error,omitempty"`
	// WritePath 是「点头往哪发」—— **由服务端给**，前端不另写一份常量。
	// 与常量 WritePath 同值；放进视图是为了让前端只有一处可依赖（改了路由不会两边不同步）。
	WritePath string        `json:"write_path"`
	Signals   []SignalView  `json:"signals"`
	Count     int           `json:"count"`
	Pending   []PendingView `json:"pending"`
}

// BandInputRequired 是「判真」那根轴上的**第四档**：**等人给一句话**
// （A2A 任务状态机里的一等中断态 `input-required` —— 不是失败，也不是系统自己在走）。
//
// 为什么它住在这里、**不进 `timeline.Band` 的词表**：
// `timeline.Band` 分的是**真相源里 status 那些词**（绿 / 黄 / 红 / 认不出的词），
// 这一档根本不来自 status —— 它来自「这条提案正躺在待批队列里」这个事实本身。
// 两路数据共用一个词表，迟早会把「在跑」和「等你点头」读成一件事（#59 要修的就是这个）。
//
// 这个词只有一处：前端照它画符号（靛蓝 + 暂停形），不另抄一份。
const BandInputRequired = "input-required"

// PendingView 是一条待批的对外形状（snake_case —— 与其它几份契约同一套写法）。
//
// 为什么不直接出 approvals.Pending：那是**领域类型**（字段大写、给 Go 用），
// 端到 HTTP 上就成了「改个字段名会静默改契约」的隐式契约。这里显式写一遍，
// 多一列少一列都得在这儿点头。
type PendingView struct {
	ID       string `json:"id"`
	Who      string `json:"who"`      // 谁提的（bot 名）
	To       string `json:"to"`       // 该谁批：域表 who 那一列（岗位，不是人名）
	Domain   string `json:"domain"`   // 客体：哪块业务
	Project  string `json:"project"`  // 客体：哪个项目
	Title    string `json:"title"`    // 队列里的一句话（含「代谁」）
	Body     string `json:"body"`     // 提案原文：一字不改地端出来，看板不解析、不截断
	Enqueued string `json:"enqueued"` // RFC3339：什么时候进的队列
	Status   string `json:"status"`   // 当前态（从热层读回来是原样，不在这里改写）
	// Band 是判真那一档：**在队列里 = 有人在等你给一句话**（BandInputRequired）。
	// 不由 status 推 —— status 是提案自己写的词，而「在等人」是队列这个事实。
	Band string `json:"band"`
}

func pendingViews(list []approvals.Pending) []PendingView {
	out := make([]PendingView, 0, len(list))
	for _, p := range list {
		out = append(out, PendingView{
			ID: p.ID, Who: p.Who, To: p.To, Domain: p.Domain, Project: p.Project,
			Title: p.Title, Body: p.Body, Enqueued: p.Enqueued, Status: p.Status,
			Band: BandInputRequired,
		})
	}
	return out
}

// SignalView 是三个信号的对外形状：值 + 人话。
//
// **由服务端给**，前端照这份画按钮 —— 三个词只此一份，不两边各写一遍。
type SignalView struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func signalViews() []SignalView {
	out := make([]SignalView, 0, len(approvals.Signals))
	for _, s := range approvals.Signals {
		out = append(out, SignalView{Value: string(s), Label: s.Verdict()})
	}
	return out
}

// signalWords 是「只认哪三个字」那句话（提示文案只此一处）。
func signalWords() string {
	parts := make([]string, 0, len(approvals.Signals))
	for _, s := range approvals.Signals {
		parts = append(parts, string(s))
	}
	return strings.Join(parts, " / ")
}

// handleApprovals 出待批队列（只读）。
func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	view := ApprovalView{Schema: "anc.approvals/v1", WritePath: WritePath, Signals: signalViews(), Pending: []PendingView{}}
	if s.Approvals == nil {
		view.Why = "热层没接上 —— 起服务时给 --hot-addr（或设 ANC_HOT_ADDR）"
		writeJSON(w, http.StatusOK, view)
		return
	}
	list, err := approvals.ListPending(s.Approvals)
	view.Wired = true
	if err != nil {
		view.Error = "热层读不了：" + s.scrub(err.Error())
		writeJSON(w, http.StatusOK, view)
		return
	}
	view.Pending = pendingViews(list)
	view.Count = len(list)
	writeJSON(w, http.StatusOK, view)
}

// DecideRequest 是看板点头的请求体。
//
// `by` 是「谁点的」—— **看板没有免登，所以这一栏由页面出**（飞书卡片那条嘴不需要它：
// 长连接事件自带 `open_id`）。ANC 只把 `by` 原样留痕，不校。
type DecideRequest struct {
	ID     string `json:"id"`
	By     string `json:"by"`
	Signal string `json:"signal"`
	Why    string `json:"why"`
}

// DecideReply 是点头的回执。
// 两个落点都给出来（库 / 审计），并明说**真相源不由 ANC 动** —— 免得看板上点完以为权已经生效。
type DecideReply struct {
	OK       bool   `json:"ok"`
	Proposal string `json:"proposal"`
	Verdict  string `json:"verdict"`
	By       string `json:"by"`
	Timeline string `json:"timeline"`
	Audit    string `json:"audit"`
	Note     string `json:"note"`
}

// handleApprovalsDecide 是看板这条嘴：收一次点头，落到与 CLI 同一个入口上。
func (s *Server) handleApprovalsDecide(w http.ResponseWriter, r *http.Request) {
	if s.Approvals == nil {
		approvalFail(w, http.StatusServiceUnavailable, "热层没接上，点头没地方落",
			"起服务时给 --hot-addr；自检 `anc hot ping`")
		return
	}
	if ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))); !strings.HasPrefix(ct, "application/json") {
		approvalFail(w, http.StatusUnsupportedMediaType, "只收 application/json",
			"这一条同时挡掉跨站表单：「别的网页替你点头」发不出这个 Content-Type")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		approvalFail(w, http.StatusBadRequest, "请求体读不了："+err.Error(), "")
		return
	}
	var req DecideRequest
	if err := json.Unmarshal(body, &req); err != nil {
		approvalFail(w, http.StatusBadRequest, "请求体不是合法 JSON："+err.Error(), "")
		return
	}
	sig, ok := approvals.ParseSignal(req.Signal)
	if !ok {
		approvalFail(w, http.StatusBadRequest, "认不出的信号："+req.Signal,
			"只认 "+signalWords()+"（不猜）")
		return
	}
	d, err := approvals.New(req.ID, req.By, sig, req.Why, s.now())
	if err != nil {
		approvalFail(w, http.StatusBadRequest, err.Error(), "")
		return
	}
	rec, err := approvals.Settle(s.Approvals, s.Vault, d)
	if err != nil {
		if errors.Is(err, approvals.ErrNotQueued) {
			approvalFail(w, http.StatusConflict, s.scrub(err.Error()),
				"刷新一下这一页：队列里没有这条，多半已经结过账了")
			return
		}
		approvalFail(w, http.StatusServiceUnavailable, s.scrub(err.Error()),
			"落库不成，队列没动 —— 修好再点")
		return
	}
	writeJSON(w, http.StatusOK, DecideReply{
		OK:       true,
		Proposal: rec.Pending.ID,
		Verdict:  rec.Decision.Verdict(),
		By:       rec.Decision.By,
		Timeline: s.scrub(rec.Timeline),
		Audit:    s.scrub(rec.Audit),
		Note:     "真相源没动：grants/ 那个文件由人侧的管理者 bot 写（ANC 只留痕）",
	})
}

// approvalFail 出人话的错：错在哪 + 下一步做什么。看板是给人看的，错误也得是给人看的。
func approvalFail(w http.ResponseWriter, code int, msg, hint string) {
	body := map[string]any{"ok": false, "error": msg}
	if strings.TrimSpace(hint) != "" {
		body["hint"] = hint
	}
	writeJSON(w, code, body)
}
