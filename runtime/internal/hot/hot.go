// Package hot —— 「进行中」的热层。
//
// 一句话：**达标的事落库，还在做的事实放这里。**
//
// 为什么不是文件（2026-10-10 human 拍板）：任务状态是「当前值」，不是「历史」。
// 拿行文本承载当前值，写一百次就留一百行，最后没人说得出「现在到底谁在做、卡在哪」——
// 那是垃圾堆积，不是管理。历史归 timeline（append-only、可 diff、可回滚，见 internal/timeline）；
// 当前态归热层（可覆写、可过期、可原子认领）。
//
// 三条口径：
//
//  1. **热层不是真相源。** 真相源是 vault 里的 markdown 与 timeline 行。热层丢掉只该丢
//     「进行中的即时状态」，不该丢任何结论 —— 所以「达标」的规矩是**先落库、再清热层**，
//     反序就会丢东西。
//
//  2. **TTL 不是新鲜度。** TTL 是删数据，只给「授权 / 租约」这种「到点就该失效」的东西用；
//     新鲜度是**打标记**（as_of + 阈值），**过期不删**。一件进行中的事凭空消失，
//     是这个系统最怕的失真。
//
//  3. **介质不写死。** 默认实现是 Redis；地址 / key 前缀 / TTL / 超时全是配置，
//     代码里不留死值。换介质只动 Open 一处。
package hot

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 默认值：都只是「没人配的时候先用这个」，不是规矩。
const (
	DefaultAddr   = "127.0.0.1:6379"
	DefaultPrefix = "anc"
	// DefaultTimeout 是连与读的兜底超时：热层挂了要立刻报出来，不许把调用方挂死。
	DefaultTimeout = 5 * time.Second
	// DefaultStale 是「多久没动就算不新鲜」的默认阈值。**它只画标记，不拦任何东西** ——
	// 和 anc probe 的 --stale 24h 同一把尺子。
	DefaultStale = 24 * time.Hour
	// DefaultTTL 是状态条目的默认存活时长：0 = 不失效。
	// 理由：一件事还没做完就过期消失，等于把「进行中」变成「不存在」。
	// 真要按客户的项目周期收口，那是配置项，不是这里的死值。
	DefaultTTL        = time.Duration(0)
	DefaultAgentStale = 2 * time.Minute
)

// State 是「一件进行中的事」的当前态。
//
// 字段全是数据：加字段不用迁移，老条目没这一项就是空。
type State struct {
	ID      string // 用的是 timeline 的 case id —— 一份 id 贯穿热层与真相源
	Status  string // 沿用 timeline 的档位词：done / running / blocked / failed（认不出的照收）
	By      string // 谁在做
	To      string // 下一手该交给谁
	Domain  string
	Project string
	Title   string
	Note    string
	AsOf    string // RFC3339：这个当前值是「什么时候的」—— 新鲜度靠它，不靠 TTL
	LeaseTo string // RFC3339：认领租约到期时刻（**派生值，不落库**；空 = 没租约）
}

// Fields 转成热层里那份 hash。
// **空值也照写**（写成空串）：只写非空的那些，会让「上一次有值、这一次清空」变成
// 一个删不掉的旧值 —— 那种脏数据最难查。
func (s State) Fields() map[string]string {
	return map[string]string{
		"id":      s.ID,
		"status":  s.Status,
		"by":      s.By,
		"to":      s.To,
		"domain":  s.Domain,
		"project": s.Project,
		"title":   s.Title,
		"note":    s.Note,
		"as_of":   s.AsOf,
	}
}

// StateFromFields 是 Fields 的逆。认不出的字段**丢掉不算错** ——
// 热层里多一个别人写的字段，不该让这里读不出东西。
func StateFromFields(m map[string]string) State {
	return State{
		ID:      m["id"],
		Status:  m["status"],
		By:      m["by"],
		To:      m["to"],
		Domain:  m["domain"],
		Project: m["project"],
		Title:   m["title"],
		Note:    m["note"],
		AsOf:    m["as_of"],
	}
}

// Time 解析 AsOf。读不懂返回 false —— 调用方据此说「时间读不懂」，
// 而不是把它当成「刚刚更新过」。
func (s State) Time() (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s.AsOf))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// Stale 是新鲜度判据：超过 after 没动过 = 不新鲜。after <= 0 = 不判（永远算新鲜）。
// **只画标记，不拦、不删** —— 判的是「要不要去看一眼」，不是「能不能用」。
func (s State) Stale(now time.Time, after time.Duration) bool {
	if after <= 0 {
		return false
	}
	t, ok := s.Time()
	if !ok {
		return true // 时间读不懂：按不新鲜算，让人去看，别假装它新鲜
	}
	return now.Sub(t) > after
}

// LeaseHeld 报租约还在不在。空、或者读不懂，都算没有有效租约。
func (s State) LeaseHeld(now time.Time) bool {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s.LeaseTo))
	return err == nil && t.After(now)
}

// TaskKey 是状态条目的 key：<prefix>:task:<id>。
// 前缀是配置（换一套 ANC 换一个前缀），id 用 timeline 的 case id。
func TaskKey(prefix, id string) string { return prefix + ":task:" + id }

// LeaseKey 单独一个命名空间 —— 放进 task: 下面会被列举时的 SCAN 一起捞出来，
// 每处都得分一次「这条是不是租约」，迟早漏一处。
func LeaseKey(prefix, id string) string { return prefix + ":lease:" + id }

func scanPattern(prefix string) string { return prefix + ":task:*" }

// ErrNotFound：热层里没有这件事。它**不是错误状态**：可能只是已经达标清走了。
var ErrNotFound = errors.New("hot: 热层里没有这一条")

// ErrOccupied：这条已经被别人认领了，租约还没到期。
type ErrOccupied struct {
	ID    string
	Owner string
}

func (e *ErrOccupied) Error() string {
	if strings.TrimSpace(e.Owner) == "" {
		return fmt.Sprintf("hot: %s 已经被别人认领（租约还没到期）", e.ID)
	}
	return fmt.Sprintf("hot: %s 已经被 %s 认领（租约还没到期）", e.ID, e.Owner)
}

// SortByAge 把「最久没动的」排前面 —— 中控台先该看见的是卡最久的那几件。
// 时间读不懂的排最后（它们另有标记，见 Stale）。
func SortByAge(list []State) {
	sort.SliceStable(list, func(i, j int) bool {
		ti, oki := list[i].Time()
		tj, okj := list[j].Time()
		switch {
		case oki && okj:
			if ti.Equal(tj) {
				return list[i].ID < list[j].ID
			}
			return ti.Before(tj)
		case oki != okj:
			return oki
		}
		return list[i].ID < list[j].ID
	})
}

// Age 报「这个当前值多久没动过了」。
func (s State) Age(now time.Time) (time.Duration, bool) {
	t, ok := s.Time()
	if !ok {
		return 0, false
	}
	return now.Sub(t), true
}

// AgentHeartbeat 是业务 agent 的「当前在线态」。
//
// 它和 State 是两件事：State 讲「一件进行中的事」，心跳讲「一个 agent 现在还在不在」。
// 所以它单占一个命名空间（<prefix>:agent:<slug>），不进任务列表的 SCAN。
//
// 一条心跳只在「最后一次见到它」这个意义上是当前值：过期不删，只按 AgentStale 打标，
// 让看板能说「最后心跳在 X 分钟前」，而不是把记录抹成「从未存在」。
type AgentHeartbeat struct {
	Slug   string // business agent 的 slug（agents.md 的 slug）—— 身份的锚点
	Status string // 自报状态：running / idle / blocked / failed …（认不出的照收，不在这里造词）
	TaskID string // 当前在做的 task id（可选；不校验存不存在，看板能对上标题就显示）
	Note   string // 一句话说明
	AsOf   string // RFC3339：最后一次心跳的时刻
}

// Fields 转成热层里那份 hash。空值也照写（同 State.Fields 的理由）。
func (h AgentHeartbeat) Fields() map[string]string {
	return map[string]string{
		"slug":    h.Slug,
		"status":  h.Status,
		"task_id": h.TaskID,
		"note":    h.Note,
		"as_of":   h.AsOf,
	}
}

// AgentHeartbeatFromFields 是 Fields 的逆；认不出的字段丢掉不算错。
func AgentHeartbeatFromFields(m map[string]string) AgentHeartbeat {
	return AgentHeartbeat{
		Slug:   m["slug"],
		Status: m["status"],
		TaskID: m["task_id"],
		Note:   m["note"],
		AsOf:   m["as_of"],
	}
}

// Time 解析 AsOf。读不懂返回 false —— 调用方据此说「时间读不懂」，不把它当成刚刚心跳。
func (h AgentHeartbeat) Time() (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(h.AsOf))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// Age 报「最后一次心跳离现在多久」。
func (h AgentHeartbeat) Age(now time.Time) (time.Duration, bool) {
	t, ok := h.Time()
	if !ok {
		return 0, false
	}
	return now.Sub(t), true
}

// Stale 是新鲜度判据：超过 after 没心跳 = 不算在线。after <= 0 = 不判。
// **只画标记，不拦、不删。**
func (h AgentHeartbeat) Stale(now time.Time, after time.Duration) bool {
	if after <= 0 {
		return false
	}
	t, ok := h.Time()
	if !ok {
		return true // 时间读不懂：按不新鲜算，让人去看
	}
	return now.Sub(t) > after
}

// AgentKey 是心跳的 key：<prefix>:agent:<slug>。
// 与 task / lease 各占一个命名空间 —— 列举任务时不会把心跳一起捞出来。
func AgentKey(prefix, slug string) string { return prefix + ":agent:" + slug }

func agentScanPattern(prefix string) string { return prefix + ":agent:*" }
