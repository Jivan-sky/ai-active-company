package hot

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// TaskSpec 是「派一件事」的输入。ID 不给就由调用方生成 —— 热层不猜业务编号。
type TaskSpec struct {
	ID       string
	To       string
	By       string
	Title    string
	Note     string
	Domain   string
	Project  string
	Session  string
	Intent   string
	Root     string   // 根 case；空 = ID
	Parent   string   // 父工作项；空 = 没有
	Accept   string   // 验收口径（自由文本）
	DueAt    string   // 截止时刻（RFC3339；空 = 没定截止）
	Priority string   // 优先级（自由文本；空 = 未知）
	Refs     []string // 证据 / 引用
	// MaxAttempt 是「失联几次后不再自动回队列」；0 = DefaultTaskMaxAttempt。
	MaxAttempt int
	// MaxHop 是「最多交接几次」；0 = DefaultTaskMaxHop。
	MaxHop int
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
	root := strings.TrimSpace(spec.Root)
	if root == "" {
		root = id
	}
	dueAt := strings.TrimSpace(spec.DueAt)
	if dueAt != "" {
		if _, derr := time.Parse(time.RFC3339, dueAt); derr != nil {
			return State{}, fmt.Errorf("hot: due_at 必须是 RFC3339（例如 2026-10-20T18:00:00+08:00）：%w", derr)
		}
	}
	s := State{
		ID: id, To: to, By: strings.TrimSpace(spec.By), From: strings.TrimSpace(spec.By),
		Title: strings.TrimSpace(spec.Title), Note: strings.TrimSpace(spec.Note),
		Domain: strings.TrimSpace(spec.Domain), Project: strings.TrimSpace(spec.Project),
		Session: strings.TrimSpace(spec.Session), Intent: intent,
		Status: "queued", Stage: StageQueued, Version: 1,
		Root: root, Parent: strings.TrimSpace(spec.Parent), Accept: strings.TrimSpace(spec.Accept),
		DueAt: dueAt, Priority: strings.TrimSpace(spec.Priority),
		Refs: cleanRefs(spec.Refs), MaxAttempt: spec.MaxAttempt, MaxHop: spec.MaxHop,
	}
	if s.MaxAttempt <= 0 {
		s.MaxAttempt = DefaultTaskMaxAttempt
	}
	if s.MaxHop <= 0 {
		s.MaxHop = DefaultTaskMaxHop
	}
	return st.PutCAS(s, 0)
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
	// 先把「租约已经过期、还挂在 running」的条收回队列，再发新的。
	// 不这么做，agent 一崩，这条就永远是 running —— 看板看得见，但没人能接手。
	if _, err := st.ReapExpired(st.now()); err != nil {
		return nil, err
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
		if s.To != to || s.Stage != StageQueued {
			continue
		}
		claimed, err := st.claimIf(s.ID, to, lease, session, StageQueued, s.Version)
		if err != nil {
			var occupied *ErrOccupied
			var moved *ErrVersion
			var stage *ErrStage
			if errors.As(err, &occupied) || errors.As(err, &moved) || errors.As(err, &stage) {
				continue // 别人先拉走了；不是错误，下一轮再看
			}
			return out, err
		}
		out = append(out, claimed)
	}
	return out, nil
}

// AckTask 记一次任务回执。**只有被派的人能回执** —— 谁在做这件事必须唯一。
//
// status 不在这里造词：done / blocked / failed / cancelled … 认不出的照收，
// 由业务与看板去解释。回执后放开租约；`done` 的条目仍留在热层，
// 等「落库」那一步把它写进 timeline 再清走（先落库、再清热层）。
func (st *Store) AckTask(id, who, status, note string) (State, error) {
	return st.AckTaskFenced(id, who, 0, status, note)
}

// AckTaskFenced 是带回执围栏的版本。fence=0 = 兼容旧调用方（只校认领人）；
// 新调用方应当把 pull 返回的 fence 原样带回来，旧持有者就写不进来了。
func (st *Store) AckTaskFenced(id, who string, fence int64, status, note string) (State, error) {
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
	if strings.TrimSpace(s.Stage) == "" {
		s.Stage = DeriveStage(s.Status)
	}
	owner, leaseFence, _, err := st.leaseParts(id)
	if err != nil {
		return State{}, err
	}
	if s.Stage == StageRunning {
		if strings.TrimSpace(owner) == "" {
			return State{}, fmt.Errorf("hot: 任务 %s 没有有效租约（已过期或还没 pull）—— 先 pull / reap，再回执", id)
		}
		if owner != who {
			return State{}, fmt.Errorf("hot: 任务 %s 的当前认领人是 %s，不是 %s", id, owner, who)
		}
	}
	if fence != 0 && leaseFence != fence {
		return State{}, fmt.Errorf("hot: 任务 %s 的围栏号对不上（当前 %d，调用方 %d）—— 这条已被重新认领", id, leaseFence, fence)
	}
	s.Status = status
	s.Stage = StageForStatus(status, s.Stage)
	if n := strings.TrimSpace(note); n != "" {
		s.Note = n
	}
	saved, err := st.PutCAS(s, s.Version)
	if err != nil {
		return State{}, err
	}
	if err := st.Release(id, who); err != nil {
		return saved, err
	}
	saved.LeaseTo = ""
	saved.LeaseNoExpiry = false
	return saved, nil
}

// ReapReport 是一次回收的结果：回了队列的、升级给人的、被别人抢着处理的。
type ReapReport struct {
	Requeued  []State
	Escalated []State
	Skipped   []string
}

// ReapExpired 把「还在 running、但租约已经不在」的条收回队列。
//
// 三条口径：
//   - 只处理 running；queued / done / blocked 不动。
//   - 没有到期时间的租约（lease <= 0）视为**仍然持有**，不回收。
//   - 回队列不是删除：attempt +1、fence +1；超过 max_attempt 才进 blocked，等人接手。
func (st *Store) ReapExpired(now time.Time) (ReapReport, error) {
	list, err := st.List()
	if err != nil {
		return ReapReport{}, err
	}
	report := ReapReport{}
	for _, s := range list {
		if s.Stage != StageRunning || s.LeaseHeld(now) {
			continue
		}
		lockKey := ReapLockKey(st.prefix, s.ID)
		rep, err := st.do("SET", lockKey, "1", "NX", "PX", "5000")
		if err != nil {
			return report, err
		}
		if rep == nil {
			report.Skipped = append(report.Skipped, s.ID)
			continue
		}
		cur, ok, err := st.Get(s.ID)
		if err != nil {
			_, _ = st.do("DEL", lockKey)
			return report, err
		}
		if !ok || cur.Stage != StageRunning || cur.LeaseHeld(now) {
			_, _ = st.do("DEL", lockKey)
			continue
		}
		if err := st.deleteLease(s.ID); err != nil {
			_, _ = st.do("DEL", lockKey)
			return report, err
		}
		fence, err := st.nextFence(s.ID)
		if err != nil {
			_, _ = st.do("DEL", lockKey)
			return report, err
		}
		cur.Fence = fence
		cur.Attempt++
		cur.AsOf = now.Format(time.RFC3339)
		max := cur.MaxAttempt
		if max <= 0 {
			max = DefaultTaskMaxAttempt
		}
		if cur.Attempt > max {
			cur.Stage = StageBlocked
			cur.Status = "blocked"
			cur.Note = appendNote(cur.Note, fmt.Sprintf("reap: 第 %d 次失联，超过上限 %d，等人接手", cur.Attempt, max))
		} else {
			cur.Stage = StageQueued
			cur.Status = "queued"
			cur.By = ""
			cur.Note = appendNote(cur.Note, fmt.Sprintf("reap: 租约过期，回队列（第 %d 次）", cur.Attempt))
		}
		saved, err := st.PutCAS(cur, cur.Version)
		_, _ = st.do("DEL", lockKey)
		if err != nil {
			var moved *ErrVersion
			if errors.As(err, &moved) {
				report.Skipped = append(report.Skipped, s.ID)
				continue
			}
			return report, err
		}
		if saved.Stage == StageBlocked {
			report.Escalated = append(report.Escalated, saved)
		} else {
			report.Requeued = append(report.Requeued, saved)
		}
	}
	return report, nil
}

func appendNote(old, add string) string {
	old, add = strings.TrimSpace(old), strings.TrimSpace(add)
	switch {
	case old == "":
		return add
	case add == "":
		return old
	}
	return old + "\n" + add
}

// HandoffTask 把一件进行中的事交给下一个人。
//
// **不是重新派单**：root / from / refs / 历史都保留，只改「下一手是谁」。
// 只有当前持有者能交接 —— 而且必须带着 pull 发的围栏号：过期持有者交不出去。
// 超过 max_hop 进 blocked：交接成环比交接失败更贵，停下等人看。
//
// crossDomain 由调用方按组织关系算好传进来；热层不读 org，也不据此拒绝 ——
// v1 的口径是「放行 + 留痕 + 告警」，硬拦留给业务自己决定。
func (st *Store) HandoffTask(id, who string, fence int64, to, note string, refs []string, crossDomain bool) (State, error) {
	id = strings.TrimSpace(id)
	who = strings.TrimSpace(who)
	to = strings.TrimSpace(to)
	note = strings.TrimSpace(note)
	if id == "" || who == "" || to == "" {
		return State{}, errors.New("hot: handoff 需要 id / who / to")
	}
	if note == "" {
		return State{}, errors.New("hot: handoff 要写清为什么交给下一个人 —— note 不能空")
	}
	s, ok, err := st.Get(id)
	if err != nil {
		return State{}, err
	}
	if !ok {
		return State{}, ErrNotFound
	}
	if s.Stage == "" {
		s.Stage = DeriveStage(s.Status)
	}
	if err := st.requireHeld(s, who, fence); err != nil {
		return State{}, err
	}
	max := s.MaxHop
	if max <= 0 {
		max = DefaultTaskMaxHop
	}
	now := st.now().Format(time.RFC3339)
	s.Hop++
	s.Handoff = Handoff{From: who, To: to, Note: note, Refs: cleanRefs(refs), CrossDomain: crossDomain, At: now}
	s.Refs = mergeRefs(s.Refs, refs)
	s.To = to
	s.By = ""
	s.AsOf = now
	if s.Hop > max {
		s.Stage = StageBlocked
		s.Status = "blocked"
		s.Note = appendNote(s.Note, fmt.Sprintf("handoff: 第 %d 次交接，超过上限 %d，停在等人接手（下一手 %s）", s.Hop, max, to))
	} else {
		s.Stage = StageQueued
		s.Status = "queued"
		s.Note = appendNote(s.Note, fmt.Sprintf("handoff: %s → %s（第 %d 次）：%s", who, to, s.Hop, note))
	}
	saved, err := st.PutCAS(s, s.Version)
	if err != nil {
		return State{}, err
	}
	if err := st.Release(id, who); err != nil {
		return saved, err
	}
	saved.LeaseTo = ""
	saved.LeaseNoExpiry = false
	return saved, nil
}

// SubmitTask 把一件做完的事交去验收。
//
// 提交必须有验收口径：没有「什么算做完」，review 就是走过场。
// 提交后放开租约 —— 验收人不做事，只是在看。
// 验收人就是提交人时允许，但记 Self=true 让看板与审计看得见。
func (st *Store) SubmitTask(id, who string, fence int64, reviewer, accept, note string, refs []string) (State, error) {
	id = strings.TrimSpace(id)
	who = strings.TrimSpace(who)
	reviewer = strings.TrimSpace(reviewer)
	if id == "" || who == "" || reviewer == "" {
		return State{}, errors.New("hot: submit 需要 id / who / reviewer")
	}
	s, ok, err := st.Get(id)
	if err != nil {
		return State{}, err
	}
	if !ok {
		return State{}, ErrNotFound
	}
	if s.Stage == "" {
		s.Stage = DeriveStage(s.Status)
	}
	if err := st.requireHeld(s, who, fence); err != nil {
		return State{}, err
	}
	accept = firstNonEmpty(accept, s.Accept)
	if accept == "" {
		return State{}, errors.New("hot: submit 需要验收口径 —— 说清楚什么算做完，否则验收无从谈起")
	}
	now := st.now().Format(time.RFC3339)
	self := strings.TrimSpace(s.By) == reviewer || s.Review.Self
	s.Accept = accept
	s.Refs = mergeRefs(s.Refs, refs)
	s.Review = Review{By: reviewer, Verdict: VerdictPending, Note: strings.TrimSpace(note), At: now, Self: self}
	s.Stage = StageReview
	s.Status = "review"
	s.To = reviewer
	s.By = who
	s.AsOf = now
	saved, err := st.PutCAS(s, s.Version)
	if err != nil {
		return State{}, err
	}
	if err := st.Release(id, who); err != nil {
		return saved, err
	}
	saved.LeaseTo = ""
	saved.LeaseNoExpiry = false
	return saved, nil
}

// VerifyTask 记一次验收裁定。
//
// 只有验收人能判；验收不要求租约 —— submit 那一步已经放开了。
// accepted → done；rejected → 回作者队列，理由必填（没有理由的驳回等于没说）。
func (st *Store) VerifyTask(id, who, verdict, note string) (State, error) {
	id = strings.TrimSpace(id)
	who = strings.TrimSpace(who)
	verdict = normalizeVerdict(verdict)
	note = strings.TrimSpace(note)
	if id == "" || who == "" {
		return State{}, errors.New("hot: verify 需要 id / who")
	}
	if verdict == "" {
		return State{}, errors.New("hot: verify 的结论只认 accepted / rejected")
	}
	s, ok, err := st.Get(id)
	if err != nil {
		return State{}, err
	}
	if !ok {
		return State{}, ErrNotFound
	}
	if s.Stage == "" {
		s.Stage = DeriveStage(s.Status)
	}
	if s.Stage != StageReview {
		return State{}, fmt.Errorf("hot: 任务 %s 现在在 %q 阶段，不在 review —— 没提交就验收不了", id, s.Stage)
	}
	reviewer := firstNonEmpty(s.Review.By, s.To)
	if reviewer == "" || reviewer != who {
		return State{}, fmt.Errorf("hot: 任务 %s 的验收人是 %s，不是 %s —— 不能替别人验收", id, dash(reviewer), who)
	}
	if verdict == VerdictRejected && note == "" {
		return State{}, errors.New("hot: 驳回必须写理由 —— 否则作者不知道要改什么")
	}
	now := st.now().Format(time.RFC3339)
	self := s.Review.Self || strings.TrimSpace(s.By) == who
	s.Review = Review{By: who, Verdict: verdict, Note: note, At: now, Self: self}
	s.AsOf = now
	if verdict == VerdictAccepted {
		s.Stage = StageDone
		s.Status = "done"
		s.Note = appendNote(s.Note, "verify: 验收通过")
	} else {
		author := firstNonEmpty(s.By, s.Handoff.From)
		if author == "" {
			author = strings.TrimSpace(s.From)
		}
		if author == "" {
			return State{}, errors.New("hot: 驳回找不到作者 —— 这条缺 by / from，先补人再驳回")
		}
		s.Stage = StageQueued
		s.Status = "queued"
		s.To = author
		s.By = ""
		s.Note = appendNote(s.Note, "verify: 驳回，回队列："+note)
	}
	return st.PutCAS(s, s.Version)
}

// EscalateTask 把卡点升级到具体的人。
//
// running 里升级要持有租约与围栏；review 里升级要是参与方（作者 / 验收人 / 上一手）。
// 结果一律进 blocked，并写明下一手 —— 「blocked 但不知道找谁」等于没升级。
func (st *Store) EscalateTask(id, who string, fence int64, to, note string) (State, error) {
	id = strings.TrimSpace(id)
	who = strings.TrimSpace(who)
	to = strings.TrimSpace(to)
	note = strings.TrimSpace(note)
	if id == "" || who == "" || to == "" {
		return State{}, errors.New("hot: escalate 需要 id / who / to")
	}
	if note == "" {
		return State{}, errors.New("hot: escalate 要把卡点写清楚 —— note 不能空")
	}
	s, ok, err := st.Get(id)
	if err != nil {
		return State{}, err
	}
	if !ok {
		return State{}, ErrNotFound
	}
	if s.Stage == "" {
		s.Stage = DeriveStage(s.Status)
	}
	wasRunning := s.Stage == StageRunning
	switch s.Stage {
	case StageRunning:
		if err := st.requireHeld(s, who, fence); err != nil {
			return State{}, err
		}
	case StageReview:
		if !isParticipant(s, who) {
			return State{}, fmt.Errorf("hot: %s 不是任务 %s 的参与者 —— 不能替别人升级", who, id)
		}
	default:
		return State{}, fmt.Errorf("hot: 任务 %s 在 %q 阶段，只有 running / review 能升级", id, s.Stage)
	}
	now := st.now().Format(time.RFC3339)
	s.Stage = StageBlocked
	s.Status = "blocked"
	s.To = to
	s.AsOf = now
	s.Note = appendNote(s.Note, fmt.Sprintf("escalate: %s → %s：%s", who, to, note))
	saved, err := st.PutCAS(s, s.Version)
	if err != nil {
		return State{}, err
	}
	if wasRunning {
		if err := st.Release(id, who); err != nil {
			return saved, err
		}
		saved.LeaseTo = ""
		saved.LeaseNoExpiry = false
	}
	return saved, nil
}

// requireHeld 校验「调用者现在确实是这条任务的持有者」。
// 围栏号必须带：不带就等于放弃防僵尸写，那还不如不给交接。
func (st *Store) requireHeld(s State, who string, fence int64) error {
	who = strings.TrimSpace(who)
	if who == "" {
		return errors.New("hot: 需要 who —— 交接 / 提交必须落在具体的人 / agent 头上")
	}
	if s.Stage != StageRunning {
		return fmt.Errorf("hot: 任务 %s 现在在 %q 阶段，不在 running —— 先 pull 再动", s.ID, s.Stage)
	}
	if strings.TrimSpace(s.To) != "" && s.To != who {
		return fmt.Errorf("hot: 任务 %s 是交给 %s 的，不是 %s —— 不能替别人交接 / 提交", s.ID, s.To, who)
	}
	owner, leaseFence, ms, err := st.leaseParts(s.ID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(owner) == "" {
		return fmt.Errorf("hot: 任务 %s 没有有效租约（已过期或还没 pull）—— 先 pull / reap，再动", s.ID)
	}
	if owner != who {
		return fmt.Errorf("hot: 任务 %s 的当前认领人是 %s，不是 %s", s.ID, owner, who)
	}
	if ms != -1 && ms <= 0 {
		return fmt.Errorf("hot: 任务 %s 的租约已经到期 —— 先 reap，再重新 pull", s.ID)
	}
	if fence <= 0 {
		return fmt.Errorf("hot: 任务 %s 需要带上 pull 返回的 fence —— 不带围栏号就无法挡住旧持有者", s.ID)
	}
	if leaseFence != fence {
		return fmt.Errorf("hot: 任务 %s 的围栏号对不上（当前 %d，调用方 %d）—— 这条已被重新认领", s.ID, leaseFence, fence)
	}
	return nil
}

func isParticipant(s State, who string) bool {
	who = strings.TrimSpace(who)
	if who == "" {
		return false
	}
	return who == strings.TrimSpace(s.Review.By) ||
		who == strings.TrimSpace(s.To) ||
		who == strings.TrimSpace(s.By) ||
		who == strings.TrimSpace(s.Handoff.From) ||
		who == strings.TrimSpace(s.From)
}

func normalizeVerdict(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case VerdictAccepted, "accept", "approve", "approved", "pass", "ok":
		return VerdictAccepted
	case VerdictRejected, "reject", "fail", "failed", "deny", "denied":
		return VerdictRejected
	}
	return ""
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(空)"
	}
	return s
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(b)
}

func cleanRefs(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, r := range in {
		r = strings.TrimSpace(r)
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeRefs(old, add []string) []string {
	return cleanRefs(append(append([]string{}, old...), add...))
}
