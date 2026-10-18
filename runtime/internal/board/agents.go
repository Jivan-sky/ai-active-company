package board

import (
	"net/http"
	"sort"
	"time"

	"anc/internal/hot"
	"anc/internal/org"
)

// AgentsSchema 是业务 agent 在线态的契约版本。
// 它与 anc.board/v2 分开：board 讲**真相源里登记了什么**，这里讲**运行到现在谁还在线**。
const AgentsSchema = "anc.agents/v1"

// AgentLayer 是心跳视角。`*hot.Store` 满足它；nil = 没接上。
// 单开一个接口而不是复用 HotLayer：这里问的是 agent，不是任务条目。
type AgentLayer interface {
	ListAgents() ([]hot.AgentHeartbeat, error)
	AgentStale() time.Duration
}

// AgentView 是一个业务 agent 的对外形状。
//
// 三件事必须分开，不能拿一个「绿点」糊过去：
//   - credentialed：agents.md 有没有 app_id（接没接凭据面）
//   - online / stale / last_seen：心跳说他现在还在不在
//   - status / task_id / task_title / note：他自报在干什么
type AgentView struct {
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Domain       string   `json:"domain,omitempty"`
	DomainName   string   `json:"domain_name,omitempty"`
	Role         string   `json:"role,omitempty"`
	Harness      string   `json:"harness,omitempty"`
	Model        string   `json:"model,omitempty"`
	Tools        []string `json:"tools,omitempty"`
	Credentialed bool     `json:"credentialed"`
	Online       bool     `json:"online"`
	Stale        bool     `json:"stale"`
	LastSeen     string   `json:"last_seen,omitempty"`
	AgeSecs      *int64   `json:"age_secs,omitempty"`
	Status       string   `json:"status,omitempty"`
	TaskID       string   `json:"task_id,omitempty"`
	TaskTitle    string   `json:"task_title,omitempty"`
	Note         string   `json:"note,omitempty"`
	Session      string   `json:"session,omitempty"`
	RunningTasks int      `json:"running_tasks"`
	QueuedTasks  int      `json:"queued_tasks"`
}

// AgentsView 是 `/api/agents` 的形状（schema = anc.agents/v1）。
//
// 失败口径与其它视图一致：
//   - 热层没接上：wired=false + why —— 不许拿「全是离线」冒充「都掉线了」；
//   - 热层读不了：wired=true + error，entries 仍出注册表（在线态未知）；
//   - 已登记但从未心跳：online=false、last_seen 空 —— 这是「从未心跳」，不是「离线很久」。
type AgentsView struct {
	Schema         string      `json:"schema"`
	Wired          bool        `json:"wired"`
	Why            string      `json:"why,omitempty"`
	Error          string      `json:"error,omitempty"`
	StaleAfterSecs int64       `json:"stale_after_secs"`
	Total          int         `json:"total"`
	Online         int         `json:"online"`
	Entries        []AgentView `json:"entries"`
	Note           string      `json:"note"`
}

// handleAgents 把「登记了谁」与「谁还在心跳」合成一张视图。
// 只读：不改任何一条心跳（心跳只由 agent 自己报）。
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	view := AgentsView{
		Schema: AgentsSchema, Entries: []AgentView{},
		StaleAfterSecs: int64(hot.DefaultAgentStale / time.Second),
		Note: "在线 = 最近一次心跳在阈值内；「最近调用」是审计里的历史，不是此刻在线。" +
			"已登记但从未心跳的 agent 会显示为未知，不会凭空点亮。",
	}
	if s.Agents == nil {
		view.Why = "热层没接上 —— 起服务时给 --hot-addr（或设 ANC_HOT_ADDR）"
		writeJSON(w, http.StatusOK, view)
		return
	}
	view.Wired = true
	view.StaleAfterSecs = int64(s.Agents.AgentStale() / time.Second)

	o, err := org.Load(s.Vault)
	if err != nil {
		view.Error = "真相源加载失败，出不了注册表：" + s.scrub(firstLine(err.Error()))
		writeJSON(w, http.StatusOK, view)
		return
	}

	beats, err := s.Agents.ListAgents()
	if err != nil {
		view.Error = "心跳读不了：" + s.scrub(firstLine(err.Error()))
	}
	bySlug := make(map[string]hot.AgentHeartbeat, len(beats))
	for _, h := range beats {
		bySlug[h.Slug] = h
	}

	// 任务标题：只在热层接上时补。补不到就留空，不拿项目名冒充「正在做」。
	tasks := map[string]hot.State{}
	type taskCount struct{ running, queued int }
	counts := map[string]taskCount{}
	if s.Hot != nil {
		if list, terr := s.Hot.List(); terr == nil {
			for _, t := range list {
				tasks[t.ID] = t
				c := counts[t.To]
				switch t.Status {
				case "running":
					c.running++
				case "queued":
					c.queued++
				}
				counts[t.To] = c
			}
		}
	}

	now := s.now()
	domainName := map[string]string{}
	for _, d := range o.Domains {
		domainName[d.Slug] = d.Name
	}
	view.Total = len(o.Agents)
	view.Entries = make([]AgentView, 0, len(o.Agents))
	for _, a := range o.Agents {
		e := AgentView{
			Slug: a.Slug, Name: a.Name, Description: a.Description,
			Domain: a.Domain, DomainName: domainName[a.Domain], Role: a.Role,
			Harness: a.Harness, Model: a.Model, Tools: org.SplitList(a.Tools),
			Credentialed: a.AppID != "",
		}
		h, ok := bySlug[a.Slug]
		if ok {
			e.LastSeen = h.AsOf
			e.Status = h.Status
			e.TaskID = h.TaskID
			e.Note = h.Note
			e.Session = h.Session
			if age, has := h.Age(now); has {
				secs := int64(age / time.Second)
				e.AgeSecs = &secs
				e.Stale = h.Stale(now, s.Agents.AgentStale())
				e.Online = !e.Stale
			} else {
				// 心跳时间读不懂：不能算在线，也不能当成「刚刚」。
				e.Stale = true
			}
			if t, found := tasks[h.TaskID]; found {
				e.TaskTitle = t.Title
			}
		}
		c := counts[a.Slug]
		e.RunningTasks, e.QueuedTasks = c.running, c.queued
		if e.Online {
			view.Online++
		}
		view.Entries = append(view.Entries, e)
	}

	// 在线在前，其次最近心跳，最后按 slug —— 同一份现场两次刷新逐字节相同。
	sort.SliceStable(view.Entries, func(i, j int) bool {
		a, b := view.Entries[i], view.Entries[j]
		if a.Online != b.Online {
			return a.Online
		}
		ai, bi := int64(1<<62), int64(1<<62)
		if a.AgeSecs != nil {
			ai = *a.AgeSecs
		}
		if b.AgeSecs != nil {
			bi = *b.AgeSecs
		}
		if ai != bi {
			return ai < bi
		}
		return a.Slug < b.Slug
	})
	writeJSON(w, http.StatusOK, view)
}
