package board

import (
	"net/http"
	"sort"
	"time"

	"anc/internal/hot"
)

// HotSchema 是这一份视图的版本号（同 /api/assets 的理由）。
const HotSchema = "anc.hot/v1"

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
	ID      string `json:"id"`
	Status  string `json:"status"`
	Title   string `json:"title"`
	By      string `json:"by,omitempty"`
	To      string `json:"to,omitempty"`
	AsOf    string `json:"as_of,omitempty"`
	AgeSecs *int64 `json:"age_secs,omitempty"`
	Stale   bool   `json:"stale"`
}

// HotView 是 `/api/hot` 的形状（schema = anc.hot/v1）—— 「热层里现在住着什么」。
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
	Entries        []HotEntryView `json:"entries"`
	Note           string         `json:"note"`
}

// hotRow 是排序用的一行：视图 + 「这条有没有写时间」。
type hotRow struct {
	e       HotEntryView
	hasTime bool
}

// handleHot 读一遍热层。读不动**不拦看板**：看不到热层不等于真相源坏了，别的页照样该看得到。
func (s *Server) handleHot(w http.ResponseWriter, r *http.Request) {
	view := HotView{
		Schema: HotSchema, Entries: []HotEntryView{},
		StaleAfterSecs: int64(hot.DefaultStale / time.Second),
		Note: "热层是「还在发生的事」待的地方：带保鲜期，到期自己走 —— 所以它便宜。" +
			"这一页只读：不认领、不清走、不改任何一条。",
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
	for _, st := range list {
		e := HotEntryView{ID: st.ID, Status: st.Status, Title: st.Title, By: st.By, To: st.To, AsOf: st.AsOf}
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
