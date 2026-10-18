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
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
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
	// DefaultTaskLease 是 pull 之后的默认认领租约。
	// 它防的是「agent 拿到任务后失联，任务永远锁在它手里」；到期不抢、不删，
	// 只是下一次 pull 可以再认领（同 Claim 的口径）。
	DefaultTaskLease = 10 * time.Minute
	// DefaultTaskMaxAttempt 是「同一件事失联几次后不再自动回队列」。
	// 超过这个次数就进 blocked，等人接手；不是删除，也不是判失败。
	DefaultTaskMaxAttempt = 3
	// DefaultTaskMaxHop 是「一件事最多交接几次」的默认上限。
	// 超过就进 blocked 等人接手 —— 交接成环比交接失败更贵。
	DefaultTaskMaxHop = 6
)

// stage 是**协议阶段**，不是业务状态。业务词仍放 Status：
// stage 只回答「这件事在流程的哪一步」，status 回答「业务上现在什么样」。
// 两者拆开，机制才不会因为客户换了一个业务词就失灵。
const (
	StageQueued    = "queued"
	StageRunning   = "running"
	StageReview    = "review"
	StageBlocked   = "blocked"
	StageDone      = "done"
	StageCancelled = "cancelled"
)

// 验收结论只认三种：还没判、接受、驳回。
const (
	VerdictPending  = "pending"
	VerdictAccepted = "accepted"
	VerdictRejected = "rejected"
)

// Handoff 是一次交接的当前值（历史仍在 timeline / audit 里，不在这里堆）。
type Handoff struct {
	From        string   `json:"from"`
	To          string   `json:"to"`
	Note        string   `json:"note,omitempty"`
	Refs        []string `json:"refs,omitempty"`
	CrossDomain bool     `json:"cross_domain,omitempty"`
	At          string   `json:"at"`
}

// Review 是验收的当前值。By 是验收人；Self 表示验收人就是提交人 ——
// 只告警，不拦：这是 v1 拍板的口径。
type Review struct {
	By      string `json:"by"`
	Verdict string `json:"verdict"`
	Note    string `json:"note,omitempty"`
	At      string `json:"at"`
	Self    bool   `json:"self,omitempty"`
}

// State 是「一件进行中的事」的当前态。
//
// 字段全是数据：加字段不用迁移，老条目没这一项就是空。
type State struct {
	ID      string // 用的是 timeline 的 case id —— 一份 id 贯穿热层与真相源
	Status  string // 业务状态词：done / running / blocked / failed / waiting_customer …（认不出的照收）
	Stage   string // 协议阶段：queued / running / review / blocked / done / cancelled
	Fence   int64  // 当前认领的围栏号；每次 claim / reap +1，旧持有者的写入会被拒
	Attempt int    // 失联回收次数（只由 reap 增加）
	// MaxAttempt 是「失联几次后不再自动回队列」；0 = 用 DefaultTaskMaxAttempt。
	MaxAttempt int
	By         string // 谁在做（pull 时改成本人）
	From       string // 谁派的（入队时钉住，之后不再改）
	To         string // 下一手该交给谁
	Domain     string
	Project    string
	Title      string
	Note       string
	Root       string // 根 case；入队时默认 = ID，交接不换根
	Parent     string // 父工作项（可选；空 = 没有父项）
	Accept     string // 验收口径（自由文本；由业务决定，不做结构化字段）
	DueAt      string // 截止时刻（RFC3339；空 = 没定截止）
	Priority   string // 优先级（自由文本；空 = 未知；热层不猜大小）
	Hop        int    // 交接次数（只由 handoff 增加）
	// MaxHop 是「最多交接几次」；0 = DefaultTaskMaxHop。
	MaxHop  int
	Refs    []string // 证据 / 引用（自由文本）
	Handoff Handoff  // 最近一次交接；零值 = 没交接过
	Review  Review   // 最近一次验收；零值 = 没提交过验收
	AsOf    string   // RFC3339：这个当前值是「什么时候的」—— 新鲜度靠它，不靠 TTL
	Session string   // 这条任务属于哪个会话（空 = 沿用 agent 当前会话）
	Intent  string   // queue / interrupt / new_session …（认不出的照收；ANC 只转达，不执行）
	Version int64    // 每次任务状态变化 +1；客户端据此判断「拿到的是不是最新的一条」
	LeaseTo string   // RFC3339：认领租约到期时刻（**派生值，不落库**；空 = 没租约）
	// LeaseNoExpiry 是派生值：租约没有到期时间（claim 时 lease <= 0）。
	// 没有它，`LeaseTo` 为空会被误读成「租约过期」，reap 就会把一条永不过期的任务抢回来。
	LeaseNoExpiry bool
}

// Fields 转成热层里那份 hash。
// **空值也照写**（写成空串）：只写非空的那些，会让「上一次有值、这一次清空」变成
// 一个删不掉的旧值 —— 那种脏数据最难查。
func (s State) Fields() map[string]string {
	return map[string]string{
		"id":          s.ID,
		"status":      s.Status,
		"stage":       s.Stage,
		"fence":       strconv.FormatInt(s.Fence, 10),
		"attempt":     strconv.Itoa(s.Attempt),
		"max_attempt": strconv.Itoa(s.MaxAttempt),
		"by":          s.By,
		"from":        s.From,
		"to":          s.To,
		"domain":      s.Domain,
		"project":     s.Project,
		"title":       s.Title,
		"note":        s.Note,
		"root":        s.Root,
		"parent":      s.Parent,
		"accept":      s.Accept,
		"due_at":      s.DueAt,
		"priority":    s.Priority,
		"hop":         strconv.Itoa(s.Hop),
		"max_hop":     strconv.Itoa(s.MaxHop),
		"refs":        refsJSON(s.Refs),
		"handoff":     handoffJSON(s.Handoff),
		"review":      reviewJSON(s.Review),
		"as_of":       s.AsOf,
		"session":     s.Session,
		"intent":      s.Intent,
		"version":     strconv.FormatInt(s.Version, 10),
	}
}

// StateFromFields 是 Fields 的逆。认不出的字段**丢掉不算错** ——
// 热层里多一个别人写的字段，不该让这里读不出东西。
func StateFromFields(m map[string]string) State {
	version, _ := strconv.ParseInt(strings.TrimSpace(m["version"]), 10, 64)
	fence, _ := strconv.ParseInt(strings.TrimSpace(m["fence"]), 10, 64)
	attempt, _ := strconv.Atoi(strings.TrimSpace(m["attempt"]))
	maxAttempt, _ := strconv.Atoi(strings.TrimSpace(m["max_attempt"]))
	hop, _ := strconv.Atoi(strings.TrimSpace(m["hop"]))
	maxHop, _ := strconv.Atoi(strings.TrimSpace(m["max_hop"]))
	stage := strings.TrimSpace(m["stage"])
	if stage == "" {
		stage = DeriveStage(m["status"])
	}
	root := strings.TrimSpace(m["root"])
	if root == "" {
		root = m["id"]
	}
	return State{
		ID:         m["id"],
		Status:     m["status"],
		Stage:      stage,
		Fence:      fence,
		Attempt:    attempt,
		MaxAttempt: maxAttempt,
		By:         m["by"],
		From:       m["from"],
		To:         m["to"],
		Domain:     m["domain"],
		Project:    m["project"],
		Title:      m["title"],
		Note:       m["note"],
		Root:       root,
		Parent:     m["parent"],
		Accept:     m["accept"],
		DueAt:      m["due_at"],
		Priority:   m["priority"],
		Hop:        hop,
		MaxHop:     maxHop,
		Refs:       refsFromJSON(m["refs"]),
		Handoff:    handoffFromJSON(m["handoff"]),
		Review:     reviewFromJSON(m["review"]),
		AsOf:       m["as_of"],
		Session:    m["session"],
		Intent:     m["intent"],
		Version:    version,
	}
}

func refsJSON(refs []string) string {
	if len(refs) == 0 {
		return ""
	}
	b, _ := json.Marshal(refs)
	return string(b)
}

func refsFromJSON(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

func handoffJSON(h Handoff) string {
	if h.From == "" && h.To == "" && h.Note == "" && h.At == "" && len(h.Refs) == 0 && !h.CrossDomain {
		return ""
	}
	b, _ := json.Marshal(h)
	return string(b)
}

func handoffFromJSON(s string) Handoff {
	var h Handoff
	if strings.TrimSpace(s) == "" {
		return h
	}
	_ = json.Unmarshal([]byte(s), &h)
	return h
}

func reviewJSON(r Review) string {
	if r.By == "" && r.Verdict == "" && r.Note == "" && r.At == "" && !r.Self {
		return ""
	}
	b, _ := json.Marshal(r)
	return string(b)
}

func reviewFromJSON(s string) Review {
	var r Review
	if strings.TrimSpace(s) == "" {
		return r
	}
	_ = json.Unmarshal([]byte(s), &r)
	return r
}

// DeriveStage 把旧数据 / 只在 status 里写词的调用方，折成协议阶段。
// 认不出的业务词回空 —— 空不等于 queued，调用方必须自己决定要不要处理。
func DeriveStage(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case StageQueued:
		return StageQueued
	case StageRunning:
		return StageRunning
	case StageReview:
		return StageReview
	case StageBlocked:
		return StageBlocked
	case StageDone, "failed":
		return StageDone
	case StageCancelled:
		return StageCancelled
	}
	return ""
}

// StageForStatus 是「回执写了一个业务词之后，协议阶段该走到哪」。
// 认不出的词不动阶段：业务可以加词，机制不跟着猜。
func StageForStatus(status, current string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case StageQueued:
		return StageQueued
	case StageRunning:
		return StageRunning
	case StageReview:
		return StageReview
	case StageBlocked:
		return StageBlocked
	case StageDone, "failed":
		return StageDone
	case StageCancelled:
		return StageCancelled
	}
	return current
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

// Due 解析截止时刻。空、或者读不懂，都返回 false —— 调用方据此说「没定截止」
// 或「截止时间读不懂」，而不是把它当成「已经到期」。
func (s State) Due() (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s.DueAt))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// Overdue 只回答「现在是不是该催了」。终态（done / cancelled）不算超期：
// 那是「有没有晚交」的另一个问题，不该混进异常条。
// **只打标，不改 stage、不升级、不通知** —— 通知是 outbox 那一刀的显式动作。
func (s State) Overdue(now time.Time) bool {
	if s.Stage == StageDone || s.Stage == StageCancelled {
		return false
	}
	due, ok := s.Due()
	return ok && now.After(due)
}

// OverdueFor 给出「超了多久」。没超期 / 没定截止 / 读不懂都返回 false。
func (s State) OverdueFor(now time.Time) (time.Duration, bool) {
	if !s.Overdue(now) {
		return 0, false
	}
	due, _ := s.Due()
	return now.Sub(due), true
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
	if s.LeaseNoExpiry {
		return true
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s.LeaseTo))
	return err == nil && t.After(now)
}

// TaskKey 是状态条目的 key：<prefix>:task:<id>。
// 前缀是配置（换一套 ANC 换一个前缀），id 用 timeline 的 case id。
func TaskKey(prefix, id string) string { return prefix + ":task:" + id }

// LeaseKey 单独一个命名空间 —— 放进 task: 下面会被列举时的 SCAN 一起捞出来，
// 每处都得分一次「这条是不是租约」，迟早漏一处。
func LeaseKey(prefix, id string) string { return prefix + ":lease:" + id }

// FenceKey 是认领围栏的单调计数器。每次 claim / reap 都 INCR，
// 旧持有者拿着旧 fence 回来写，会被直接拒掉。
func FenceKey(prefix, id string) string { return prefix + ":fence:" + id }

// ReapLockKey 是回收那一刀的短锁：两个进程同时 reap，同一条只该被处理一次。
func ReapLockKey(prefix, id string) string { return prefix + ":lock:reap:" + id }

func scanPattern(prefix string) string { return prefix + ":task:*" }

// ErrNotFound：热层里没有这件事。它**不是错误状态**：可能只是已经达标清走了。
var ErrNotFound = errors.New("hot: 热层里没有这一条")

// ErrOccupied：这条已经被别人认领了，租约还没到期。
type ErrOccupied struct {
	ID    string
	Owner string
}

// ErrVersion：写之前读到的 version 已经不是当前值 —— 有人先写了一刀。
// 这不是「坏数据」，是两个写者撞上了；调用方该重读，而不是盲写。
type ErrVersion struct {
	ID   string
	Want int64
	Got  int64
}

func (e *ErrVersion) Error() string {
	return fmt.Sprintf("hot: %s 的版本对不上（读到 %d，当前 %d）—— 有人先写过，重读再决定", e.ID, e.Want, e.Got)
}

// ErrStage：这条已经不在调用方以为的阶段了（典型：pull 拿着旧列表去认领，
// 期间任务已经 done）。不是错误状态，是「这一手不该再接」。
type ErrStage struct {
	ID   string
	Want string
	Got  string
}

func (e *ErrStage) Error() string {
	return fmt.Sprintf("hot: %s 现在在 %q 阶段，不在 %q —— 不再认领", e.ID, e.Got, e.Want)
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
	Slug    string // business agent 的 slug（agents.md 的 slug）—— 身份的锚点
	Status  string // 自报状态：running / idle / blocked / failed …（认不出的照收，不在这里造词）
	TaskID  string // 当前在做的 task id（可选；不校验存不存在，看板能对上标题就显示）
	Note    string // 一句话说明
	AsOf    string // RFC3339：最后一次心跳的时刻
	Session string // 当前会话 id；空 = 尚未绑定（ANC 只用它做归属，不控制系统会话）
}

// Fields 转成热层里那份 hash。空值也照写（同 State.Fields 的理由）。
func (h AgentHeartbeat) Fields() map[string]string {
	return map[string]string{
		"slug":    h.Slug,
		"status":  h.Status,
		"task_id": h.TaskID,
		"note":    h.Note,
		"as_of":   h.AsOf,
		"session": h.Session,
	}
}

// AgentHeartbeatFromFields 是 Fields 的逆；认不出的字段丢掉不算错。
func AgentHeartbeatFromFields(m map[string]string) AgentHeartbeat {
	return AgentHeartbeat{
		Slug:    m["slug"],
		Status:  m["status"],
		TaskID:  m["task_id"],
		Note:    m["note"],
		AsOf:    m["as_of"],
		Session: m["session"],
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
