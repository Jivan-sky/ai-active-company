package board

import (
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"net/http"

	"anc/internal/org"
	"anc/internal/retrieval"
)

// SearchSchema 是这一份视图的版本号（同 /api/assets 的理由：字段增删一律改版本号）。
const SearchSchema = "anc.search/v1"

// maxSearchQueryRunes 是单次查询的长度上限。检索不是写入，但也不该拿一整篇文档
// 当 query 来拖垮索引；超了直接说清楚，不悄悄截断后假装搜过。
const maxSearchQueryRunes = 512

// SearchView 是 `/api/search` 的形状 —— 「按当前可见范围，知识库里哪几篇相关」。
//
// 它和 `/api/assets` 的分工：assets 数的是目录与文件，这一页答的是**内容命中**；
// 两者都不把「有文件」说成「有资产」。
//
// 权限口径不在这里发明：`who` 与 CLI / MCP 共用 retrieval.ScopeFor，域收窄共用
// retrieval.NarrowToDomain。看板现在没有登录态，`who` 是**观察视角**，不是登录凭证；
// 绑非回环地址仍需显式 `--insecure-bind`（同看板其余端点）。
type SearchView struct {
	Schema    string             `json:"schema"`
	Wired     bool               `json:"wired"`
	Query     string             `json:"query,omitempty"`
	Who       string             `json:"who,omitempty"`
	Domain    string             `json:"domain,omitempty"`
	Scope     []string           `json:"scope"`
	ScopeNote string             `json:"scope_note,omitempty"`
	Backend   string             `json:"backend,omitempty"`
	Indexed   int                `json:"indexed"`
	Visible   int                `json:"visible"`
	Count     int                `json:"count"`
	Hits      []retrieval.Hit    `json:"hits"`
	Warnings  []string           `json:"warnings,omitempty"`
	Refused   *retrieval.Refusal `json:"refused,omitempty"`
	Note      string             `json:"note"`
	Error     string             `json:"error,omitempty"`
	At        string             `json:"at"`
}

// handleSearch 答「按域可见范围搜知识」。只读；拒与空结果都不是 HTTP 错误，
// 照回 200 让前端把原因显示出来（同 /api/context /api/issues 的口径）。
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	who := strings.TrimSpace(r.URL.Query().Get("who"))
	domain := strings.TrimSpace(r.URL.Query().Get("domain"))
	view := SearchView{
		Schema: SearchSchema, Query: q, Who: who, Domain: domain,
		Scope: []string{}, Hits: []retrieval.Hit{},
		At: time.Now().Format(time.RFC3339),
		Note: "检索给的是**指针 + 片段**，不是正文结论；查不到就如实说没入库，不按记忆补。" +
			"看板没有登录态，who 是观察视角，不是登录凭证。",
	}
	if utf8.RuneCountInString(q) > maxSearchQueryRunes {
		view.Error = "查询太长（上限 " + strconv.Itoa(maxSearchQueryRunes) + " 个字符）—— 收回关键词再搜，不截断后假装搜过"
		writeJSON(w, http.StatusOK, view)
		return
	}
	if q == "" {
		view.Error = "q 为空 —— 要查什么？"
		writeJSON(w, http.StatusOK, view)
		return
	}
	limit, err := searchLimit(r.URL.Query().Get("limit"))
	if err != nil {
		view.Error = err.Error()
		writeJSON(w, http.StatusOK, view)
		return
	}

	o, err := org.Load(s.Vault)
	if err != nil {
		view.Error = "真相源读不动，搜不了：" + s.scrub(firstLine(err.Error())) + "（先修 org：anc org check <vault>）"
		writeJSON(w, http.StatusOK, view)
		return
	}
	view.Wired = true

	sc, refused := retrieval.ScopeFor(o, who)
	if refused != nil {
		view.Refused = refused
		writeJSON(w, http.StatusOK, view)
		return
	}
	view.Scope = retrieval.ScopeLabels(sc)
	if domain != "" {
		sc, refused = retrieval.NarrowToDomain(o, sc, domain)
		if refused != nil {
			view.Refused = refused
			writeJSON(w, http.StatusOK, view)
			return
		}
		view.Scope = []string{domain}
		view.ScopeNote = "含公司级资料"
	}

	ix, warns, err := retrieval.Open(s.Vault, o, retrieval.Config{
		Backend: os.Getenv("ANC_SEARCH_BACKEND"),
		Roots:   org.SplitList(os.Getenv("ANC_SEARCH_ROOTS")),
	})
	view.Warnings = warns
	if err != nil {
		view.Error = s.scrub("索引建不起来：" + err.Error())
		writeJSON(w, http.StatusOK, view)
		return
	}
	view.Backend = ix.Name()
	view.Indexed, view.Visible = ix.Len(), ix.Visible(sc)

	hits, err := ix.Search(q, sc, limit)
	if err != nil {
		view.Error = s.scrub("检索失败：" + err.Error())
		writeJSON(w, http.StatusOK, view)
		return
	}
	view.Hits = hits
	view.Count = len(hits)
	if view.Count == 0 {
		view.Note = "没有命中 —— 可能是还没入库；不要按记忆补（查不到就说查不到）"
	}
	writeJSON(w, http.StatusOK, view)
}

// searchLimit 解析并夹取 limit：空 = 默认值，非法 = 明确报错，
// 大于上限由检索层夹到 MaxLimit（同一个上限，不在看板另立一份）。
func searchLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return retrieval.DefaultLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, &searchBadLimit{raw: raw}
	}
	return n, nil
}

type searchBadLimit struct{ raw string }

func (e *searchBadLimit) Error() string {
	return "limit=「" + e.raw + "」不是正整数 —— 用 1–" + strconv.Itoa(retrieval.MaxLimit) + "，不写就用 " + strconv.Itoa(retrieval.DefaultLimit)
}
