package board

import (
	"net/http"
	"sort"
	"time"

	"anc/internal/hot"
)

// HotSchema 是这一份视图的版本号（同 /api/assets 的理由）。
// v2 起条目带上 domain / project —— 没有它们，项目页只能看到「有任务」，
// 看不出「是哪一摊的」。
const HotSchema = "anc.hot/v2"

// hotKeep 是最多回显几条条目 —— 看板要的是「这一层现在什么情况」，
// 不是把整个队列搬进浏览器。总数照报 Total，不因为截断就装作只有这些。
const hotKeep = 20

// HotLayer 是热层的**整体**视角，区别于 approvals.Queue 的队列视角：
// 那个问「哪几件事在等人点头」，这个问「这一层现在住着几条、几条不新鲜、多久自己走」。
// 同一份底层数据、两个问题、两张视图 —— `anc hot ls` 与 `anc approvals ls` 也是这么分的。
// `*hot.Store` 满足它。
type HotLayer interface {
	List() ([]hot.State, error)
	TTL() time.Duration
}

// HotEntryView 是一条热层条目的对外形状（snake_case —— 与其它几份契约同一套写法）。
//
// AgeSecs 是指针：**没写「这是什么时候的」就不出这个键**。
// 出个 0 会读成「刚发生」—— 那是在编，不是缺。
type HotEntryView struct {
	ID            string       `json:"id"`
	Status        string       `json:"status"`
	Stage         string       `json:"stage,omitempty"`
	Domain        string       `json:"domain,omitempty"`
	Project       string       `json:"project,omitempty"`
	Fence         int64        `json:"fence,omitempty"`
	Attempt       int          `json:"attempt,omitempty"`
	MaxAttempt    int          `json:"max_attempt,omitempty"`
	Title         string       `json:"title"`
	By            string       `json:"by,omitempty"`
	From          string       `json:"from,omitempty"`
	To            string       `json:"to,omitempty"`
	Root          string       `json:"root,omitempty"`
	Parent        string       `json:"parent,omitempty"`
	Accept        string       `json:"accept,omitempty"`
	DueAt         string       `json:"due_at,omitempty"`
	Priority      string       `json:"priority,omitempty"`
	Overdue       bool         `json:"overdue"`
	OverdueSecs   *int64       `json:"overdue_secs,omitempty"`
	Hop           int          `json:"hop,omitempty"`
	MaxHop        int          `json:"max_hop,omitempty"`
	Refs          []string     `json:"refs,omitempty"`
	Handoff       *hot.Handoff `json:"handoff,omitempty"`
	Review        *hot.Review  `json:"review,omitempty"`
	AsOf          string       `json:"as_of,omitempty"`
	Session       string       `json:"session,omitempty"`
	Intent        string       `json:"intent,omitempty"`
	Version       int64        `json:"version,omitempty"`
	LeaseTo       string       `json:"lease_to,omitempty"`
	LeaseNoExpiry bool         `json:"lease_no_expiry,omitempty"`
	AgeSecs       *int64       `json:"age_secs,omitempty"`
	Stale         bool         `json:"stale"`
}

// HotView 是 `/api/hot` 的形状（schema = anc.hot/v2）—— 「热层里现在住着什么」。
//
// 三条与其它视图一致的规矩：
//   - 热层接不上就如实回 wired=false + why —— **「读不到」和「里面是空的」是两件事**，
//     拿一份空列表冒充「没人有事」，比报错更坏（同 /api/approvals 的口径）。
//   - 阈值由**服务端给**（`stale_after_secs` / `ttl_secs`），前端照抄：口径只此一份。
//   - 「不新鲜」只是个**标记**：不拦、不删、不改任何一条（同 hot.DefaultStale 的口径）。
type HotView struct {
	Schema         string         `json:"schema"`
	Wired          bool           `json:"wired"`
	Why            string         `json:"why,omitempty"`
	Error          string         `json:"error,omitempty"`
	TTLSecs        int64          `json:"ttl_secs"`         // 0 = 不失效
	StaleAfterSecs int64          `json:"stale_after_secs"` // 多久没动算「不新鲜」
	Total          int            `json:"total"`
	Stale          int            `json:"stale"`
	Overdue        int            `json:"overdue"`
	Blocked        int            `json:"blocked"`
	Review         int            `json:"review"`
	Actionable     int            `json:"actionable"`
	Entries        []HotEntryView `json:"entries"`
	// Projects 按项目汇总**全量**热层条目（不受 hotKeep 截断影响）。
	// 项目页要回答「这一摊还有几件在办」，不能拿被截断的 entries 去数 —— 那会少报。
	Projects []HotProjectView `json:"projects"`
	// SettlePath 是「归档往哪发」—— 与审批同一口径：由服务端给，前端不另写一份常量。
	SettlePath string `json:"settle_path"`
	Note       string `json:"note"`
}

// HotProjectView 是一个项目的热层汇总。只数事实，不算完成率：
// 「做了几件」不等于「交付到哪」，后者仍没有真相源。
type HotProjectView struct {
	Slug    string `json:"slug"`
	Total   int    `json:"total"`
	Blocked int    `json:"blocked"`
	Review  int    `json:"review"`
	Overdue int    `json:"overdue"`
}

// hotRow 是排序用的一行：视图 + 「这条有没有写时间」。
type hotRow struct {
	e       HotEntryView
	hasTime bool
}

// handleHot 读一遍热层。读不动**不拦看板**：看不到热层不等于真相源坏了，别的页照样该看得到。
func (s *Server) handleHot(w http.ResponseWriter, r *http.Request) {
	view := HotView{
		Schema: HotSchema, Entries: []HotEntryView{}, Projects: []HotProjectView{},
		SettlePath:     SettlePath,
		StaleAfterSecs: int64(hot.DefaultStale / time.Second),
		Note: "热层是「还在发生的事」待的地方：带保鲜期，到期自己走 —— 所以它便宜。" +
			"只在终态条目上开一个归档口（先落 timeline、再清热层）；不认领、不改状态。",
	}
	if s.Hot == nil {
		view.Why = "热层没接上 —— 起服务时给 --hot-addr（或设 ANC_HOT_ADDR）"
		writeJSON(w, http.StatusOK, view)
		return
	}
	view.Wired = true
	view.TTLSecs = int64(s.Hot.TTL() / time.Second)
	list, err := s.Hot.List()
	if err != nil {
		view.Error = "热层读不了：" + s.scrub(firstLine(err.Error()))
		writeJSON(w, http.StatusOK, view)
		return
	}
	now := s.now()
	view.Total = len(list)
	rows := make([]hotRow, 0, len(list))
	actionable := map[string]bool{}
	projectCounts := map[string]*HotProjectView{}
	projectOf := func(slug string) *HotProjectView {
		if slug == "" {
			return nil
		}
		p := projectCounts[slug]
		if p == nil {
			p = &HotProjectView{Slug: slug}
			projectCounts[slug] = p
		}
		return p
	}
	for _, st := range list {
		e := HotEntryView{
			ID: st.ID, Status: st.Status, Stage: st.Stage, Fence: st.Fence,
			Domain: st.Domain, Project: st.Project,
			Attempt: st.Attempt, MaxAttempt: st.MaxAttempt, Title: st.Title, By: st.By, From: st.From, To: st.To,
			Root: st.Root, Parent: st.Parent, Accept: st.Accept, DueAt: st.DueAt, Priority: st.Priority,
			Hop: st.Hop, MaxHop: st.MaxHop, Refs: st.Refs,
			AsOf: st.AsOf, Session: st.Session, Intent: st.Intent, Version: st.Version,
			LeaseTo: st.LeaseTo, LeaseNoExpiry: st.LeaseNoExpiry,
		}
		e.Overdue = st.Overdue(now)
		if p := projectOf(st.Project); p != nil {
			p.Total++
			if e.Overdue {
				p.Overdue++
			}
			if st.Stage == hot.StageBlocked {
				p.Blocked++
			}
			if st.Stage == hot.StageReview && st.Review.Verdict == hot.VerdictPending {
				p.Review++
			}
		}
		if d, ok := st.OverdueFor(now); ok {
			secs := int64(d / time.Second)
			e.OverdueSecs = &secs
		}
		if e.Overdue {
			view.Overdue++
			actionable[st.ID] = true
		}
		if st.Stage == hot.StageBlocked {
			view.Blocked++
			actionable[st.ID] = true
		}
		if st.Stage == hot.StageReview && st.Review.Verdict == hot.VerdictPending {
			view.Review++
			actionable[st.ID] = true
		}
		if st.Handoff.From != "" || st.Handoff.To != "" {
			h := st.Handoff
			e.Handoff = &h
		}
		if st.Review.By != "" || st.Review.Verdict != "" {
			r := st.Review
			e.Review = &r
		}
		age, ok := st.Age(now)
		// 没写「这是什么时候的」→ 没法说它新鲜。照灰，不当成刚发生。
		e.Stale = !ok || st.Stale(now, hot.DefaultStale)
		if ok {
			secs := int64(age / time.Second)
			e.AgeSecs = &secs
		}
		if e.Stale {
			view.Stale++
		}
		rows = append(rows, hotRow{e: e, hasTime: ok})
	}
	view.Actionable = len(actionable)
	for _, p := range projectCounts {
		view.Projects = append(view.Projects, *p)
	}
	sort.Slice(view.Projects, func(i, j int) bool { return view.Projects[i].Slug < view.Projects[j].Slug })
	// 有时间的排前面，其中最新的在前；**没时间的沉到最后**（它没有「多新」可比）；
	// 同一时刻按 id 排 —— 同一份现场两次刷新要逐字节相同。
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.hasTime != b.hasTime {
			return a.hasTime
		}
		ai, bi := int64(0), int64(0)
		if a.e.AgeSecs != nil {
			ai = *a.e.AgeSecs
		}
		if b.e.AgeSecs != nil {
			bi = *b.e.AgeSecs
		}
		if ai != bi {
			return ai < bi
		}
		return a.e.ID < b.e.ID
	})
	for _, r := range rows {
		if len(view.Entries) >= hotKeep {
			break
		}
		view.Entries = append(view.Entries, r.e)
	}
	writeJSON(w, http.StatusOK, view)
}
