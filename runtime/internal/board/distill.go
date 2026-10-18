package board

import (
	"net/http"
	"strconv"
	"strings"

	"anc/internal/distill"
)

// 每页默认 / 上限：同一份候选，CLI 想全看就加大，看板默认只摆最近 50 条。
// 上限不是门禁，是防「?limit=100000」把一次请求变成全库回显。
const (
	distillDefaultLimit = 50
	distillMaxLimit     = 500
)

// handleDistill 回「哪些事已闭环、技能覆盖缺在哪」。
//
// 这是**只读**视图：候选只是原料，写不写技能由人判断。
// 读不动 timeline 不是 500 —— 与 /api/assets 同一条纪律：看板要能显示「这一格答不了」，
// 而不是整页塌掉。
func (s *Server) handleDistill(w http.ResponseWriter, r *http.Request) {
	limit := distillDefaultLimit
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			rpt := distill.Report{Schema: distill.Schema, Candidates: []distill.Candidate{},
				Skills: []distill.Skill{}, Orphans: []string{}, Gaps: []string{},
				Error: "limit 要是个正整数"}
			writeJSON(w, http.StatusOK, rpt)
			return
		}
		limit = n
		if limit > distillMaxLimit {
			limit = distillMaxLimit
		}
	}
	rpt := distill.Load(s.Vault)
	if len(rpt.Candidates) > limit {
		rpt.Candidates = rpt.Candidates[:limit]
		rpt.Truncated = true
	}
	writeJSON(w, http.StatusOK, rpt)
}
