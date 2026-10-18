package hot

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// TaskSpec 是「派一件事」的输入。ID 不给就由调用方生成 —— 热层不猜业务编号。
type TaskSpec struct {
	ID      string
	To      string
	By      string
	Title   string
	Note    string
	Domain  string
	Project string
	Session string
	Intent  string
}

// EnqueueTask 把一件事放进「待拉取」：status=queued、version=1。
//
// **这不是下发，也不是打断**：ANC 只把它写进队列，agent 自己 pull。
// `Intent=interrupt` 只是随任务带过去的一句请求，真正的「打断 / 等这一轮跑完」由 agent 自己决定。
func (st *Store) EnqueueTask(spec TaskSpec) (State, error) {
	id := strings.TrimSpace(spec.ID)
	to := strings.TrimSpace(spec.To)
	if id == "" {
		return State{}, errors.New("hot: 入队需要任务 id")
	}
	if to == "" {
		return State{}, errors.New("hot: 入队需要 to —— 这件事交给哪个 agent")
	}
	if _, exists, err := st.Get(id); err != nil {
		return State{}, err
	} else if exists {
		return State{}, fmt.Errorf("hot: 任务 %s 已经存在 —— 用新 id，别覆盖别人手上的事", id)
	}
	intent := strings.TrimSpace(spec.Intent)
	if intent == "" {
		intent = "queue"
	}
	s := State{
		ID: id, To: to, By: strings.TrimSpace(spec.By), From: strings.TrimSpace(spec.By),
		Title: strings.TrimSpace(spec.Title), Note: strings.TrimSpace(spec.Note),
		Domain: strings.TrimSpace(spec.Domain), Project: strings.TrimSpace(spec.Project),
		Session: strings.TrimSpace(spec.Session), Intent: intent,
		Status: "queued", Version: 1,
	}
	return st.Put(s)
}

// PullTasks 把「交给这个 agent、还没被拉走」的任务原子认领成 running。
//
// 认领用 SET NX（见 Claim）：两个同名 agent 同时 pull，同一条只会落到一个手里。
// lease <= 0 = 不设到期（客户自己配）；看板上的过期只打标，不抢。
func (st *Store) PullTasks(to string, limit int, lease time.Duration, session string) ([]State, error) {
	to = strings.TrimSpace(to)
	if to == "" {
		return nil, errors.New("hot: pull 需要 to")
	}
	if limit <= 0 {
		limit = 1
	}
	list, err := st.List()
	if err != nil {
		return nil, err
	}
	out := make([]State, 0, limit)
	for _, s := range list {
		if len(out) >= limit {
			break
		}
		if s.To != to || s.Status != "queued" {
			continue
		}
		claimed, err := st.Claim(s.ID, to, lease)
		if err != nil {
			var occupied *ErrOccupied
			if errors.As(err, &occupied) {
				continue // 别人先拉走了；不是错误，下一轮再看
			}
			return out, err
		}
		claimed.Status = "running"
		// 入队时没钉会话（老任务 / 直接写热层的任务）：pull 时按 agent 当前会话补上，
		// 让后续 ack / list 看得到同一条归属，而不是到回执时又变成空。
		if strings.TrimSpace(claimed.Session) == "" {
			claimed.Session = strings.TrimSpace(session)
		}
		claimed.Version++
		saved, err := st.Put(claimed)
		if err != nil {
			_ = st.Release(s.ID, to)
			return out, err
		}
		out = append(out, saved)
	}
	return out, nil
}

// AckTask 记一次任务回执。**只有被派的人能回执** —— 谁在做这件事必须唯一。
//
// status 不在这里造词：done / blocked / failed / cancelled … 认不出的照收，
// 由业务与看板去解释。回执后放开租约；`done` 的条目仍留在热层，
// 等「落库」那一步把它写进 timeline 再清走（先落库、再清热层）。
func (st *Store) AckTask(id, who, status, note string) (State, error) {
	id = strings.TrimSpace(id)
	who = strings.TrimSpace(who)
	status = strings.TrimSpace(status)
	if id == "" || who == "" || status == "" {
		return State{}, errors.New("hot: ack 需要 id / who / status")
	}
	s, ok, err := st.Get(id)
	if err != nil {
		return State{}, err
	}
	if !ok {
		return State{}, ErrNotFound
	}
	if strings.TrimSpace(s.To) != "" && s.To != who {
		return State{}, fmt.Errorf("hot: 任务 %s 是交给 %s 的，不是 %s —— 不能替别人回执", id, s.To, who)
	}
	s.Status = status
	if n := strings.TrimSpace(note); n != "" {
		s.Note = n
	}
	s.Version++
	saved, err := st.Put(s)
	if err != nil {
		return State{}, err
	}
	if err := st.Release(id, who); err != nil {
		return saved, err
	}
	saved.LeaseTo = ""
	return saved, nil
}
