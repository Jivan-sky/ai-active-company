package board

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"anc/internal/hot"
	"anc/internal/org"
	"anc/internal/tasksettle"
	"anc/internal/timeline"
)

// SettlePath 是看板的第二个写口：任务归档（落 timeline、清热层）。
//
// 为什么它不算「又开一条写入口」：与 CLI `anc task settle`、MCP `anc_task action=settle`
// 走的是**同一个**领域入口 `tasksettle.Settle` —— 「先落库、再清热层、幂等」这三条规则
// 只写在那一处。看板这里只做 HTTP 接线，不另写一套结账逻辑。
//
// 三道限制与审批信号同一套：
//  1. 只收 POST + application/json（跨站表单发不出这个 Content-Type）；
//  2. 请求体限 64KB；
//  3. **不校「点的人是不是该归档的人」**：`by` 原样留痕，判断交给 agent（同审批口径）。
const SettlePath = "/api/tasks/settle"

// TaskSettler 是结账要的两件事（同 tasksettle.Store）。`*hot.Store` 满足它。
type TaskSettler interface {
	Get(id string) (hot.State, bool, error)
	Drop(id string) error
}

// SettleRequest 是看板归档的请求体。
//
// `by` 是「谁点的」—— 看板没有免登，所以这一栏由页面出；ANC 只留痕，不校。
// status 不填 = 用热层里的当前态；要给 cancelled / failed 时必须显式写，理由留在 note 里。
type SettleRequest struct {
	ID     string `json:"id"`
	By     string `json:"by"`
	Status string `json:"status"`
	Title  string `json:"title"`
	Note   string `json:"note"`
}

// SettleReply 是归档回执：既说「落了哪条、写到哪个文件」，也说「是不是早就落过」。
type SettleReply struct {
	OK       bool   `json:"ok"`
	ID       string `json:"id"`
	By       string `json:"by"`
	Status   string `json:"status"`
	Already  bool   `json:"already"`
	Timeline string `json:"timeline,omitempty"`
	Note     string `json:"note"`
}

// handleTaskSettle 是看板这条嘴：收一次归档，落到与 CLI 同一个入口上。
func (s *Server) handleTaskSettle(w http.ResponseWriter, r *http.Request) {
	if s.Tasks == nil {
		approvalFail(w, http.StatusServiceUnavailable, "热层没接上，归档没地方读、也没地方清",
			"起服务时给 --hot-addr；自检 `anc hot ping`")
		return
	}
	if ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))); !strings.HasPrefix(ct, "application/json") {
		approvalFail(w, http.StatusUnsupportedMediaType, "只收 application/json",
			"这一条同时挡掉跨站表单：「别的网页替你归档」发不出这个 Content-Type")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		approvalFail(w, http.StatusBadRequest, "请求体读不了："+err.Error(), "")
		return
	}
	var req SettleRequest
	if err := json.Unmarshal(body, &req); err != nil {
		approvalFail(w, http.StatusBadRequest, "请求体不是合法 JSON："+err.Error(), "")
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	req.By = strings.TrimSpace(req.By)
	if req.ID == "" {
		approvalFail(w, http.StatusBadRequest, "缺少任务 id", "")
		return
	}
	if req.By == "" {
		approvalFail(w, http.StatusBadRequest, "先填「谁在归档」：匿名归档等于没归档",
			"这一栏会原样写进 timeline；看板没有免登，由填的人负责")
		return
	}

	res, err := tasksettle.Settle(s.Tasks, s.Vault, req.ID, tasksettle.Opts{
		By: s.authorSlug(req.By), Title: req.Title, Status: req.Status, Note: req.Note, At: s.now(),
	})
	if err != nil {
		switch {
		case errors.Is(err, timeline.ErrAuthor):
			approvalFail(w, http.StatusBadRequest, s.scrub(err.Error()),
				"用成员 slug（例如 alice）或英文名；中文名会尽量对到已登记的成员 slug")
		case errors.Is(err, tasksettle.ErrOpen):
			approvalFail(w, http.StatusConflict, s.scrub(err.Error()),
				"先让执行的人回执；要终止就带 status（例如 cancelled）并在 note 里写清理由")
		case errors.Is(err, hot.ErrNotFound):
			approvalFail(w, http.StatusConflict, s.scrub(err.Error()),
				"刷新一下这一页：热层里没有这条，时间线里也没有")
		default:
			approvalFail(w, http.StatusServiceUnavailable, s.scrub(err.Error()),
				"落库不成，热层没动 —— 修好再点")
		}
		return
	}
	note := "已先落 timeline、再清热层；真相源里从此有这一条。"
	if res.Already {
		note = "这条早已沉淀，本次没重复写；热层还在的话已顺手清掉。"
	}
	writeJSON(w, http.StatusOK, SettleReply{
		OK: true, ID: res.Entry.Case, By: res.Entry.By, Status: res.Entry.Status,
		Already: res.Already, Timeline: s.scrub(res.Path), Note: note,
	})
}

// authorSlug 把「点的人」对齐到已登记的成员：中文显示名 → slug。
//
// 为什么只对成员：timeline 的作者要能落成文件名（ASCII），而看板上的「谁在点」
// 是自由文本。填的是张三、成员表里有张三（zhangsan），就署 zhangsan；
// 认不出就原样用 —— 不替人编一个身份。org 读不出来也原样用，后面的 timeline
// 会如实报「写不成文件名」，那是填错了，不是服务坏了。
func (s *Server) authorSlug(by string) string {
	o, err := org.Load(s.Vault)
	if err != nil {
		return by
	}
	for _, m := range o.Members {
		if strings.EqualFold(by, m.Name) || strings.EqualFold(by, m.DisplayName) {
			return m.Name
		}
	}
	for _, a := range o.Agents {
		if strings.EqualFold(by, a.Slug) || strings.EqualFold(by, a.Name) {
			return a.Slug
		}
	}
	return by
}
