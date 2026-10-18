package hot

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config 是热层的**外置配置**（地址、前缀、TTL、超时全在这里，代码里不留死值）。
type Config struct {
	Addr       string        // 默认 DefaultAddr
	Prefix     string        // 默认 DefaultPrefix
	TTL        time.Duration // 状态条目存活时长；0 = 不失效（默认）
	Timeout    time.Duration // 连/读兜底超时；0 = DefaultTimeout
	AgentStale time.Duration // 多久没心跳不算在线；0 = DefaultAgentStale（只打标，不删）
}

// Store 是热层的一个介质实现。它**不是**真相源（见包注释 1）。
type Store struct {
	mu   sync.Mutex // 只护 conn 这一个字段 —— 换连接时，别的调用不该拿到半截的它
	conn *respConn
	// dial 是「再拨一条」的办法；nil = 这条 Store 不自己拨号（测试注入连接时就是这种）。
	dial       func() (io.ReadWriteCloser, error)
	timeout    time.Duration
	prefix     string
	ttl        time.Duration
	agentStale time.Duration
	now        func() time.Time
}

// errNoDial 是「这条 Store 不会自己拨号」：newStore 直接接了条连接时（测试注入）就是这种。
// 这种情况下坏掉的流换不掉 —— 与其硬往上写，不如如实报错。
var errNoDial = errors.New("hot: 这条 Store 不自己拨号，换不了连接")

// Open 连上一个热层，并**当场 PING 一次**：热层没起来要立刻报出来，
// 不许静默降级成「假装没有这回事」。
func Open(cfg Config) (*Store, error) {
	addr := strings.TrimSpace(cfg.Addr)
	if addr == "" {
		addr = DefaultAddr
	}
	conn, err := dialResp(addr, cfg.Timeout)
	if err != nil {
		return nil, err
	}
	st := newStore(conn, cfg)
	// 常驻进程（看板）一活就是好几天，而热层随时可能被重启 —— 连接坏了得能自己换一条。
	// 少了这一步，一次 redis 重启就等于「从此永久失明，只能重启进程」（议题 #66）。
	st.dial = func() (io.ReadWriteCloser, error) { return dialResp(addr, cfg.Timeout) }
	if err := st.Ping(); err != nil {
		_ = st.Close() // 关「现在手里这条」：Ping 失败时它可能已经换过了
		return nil, err
	}
	return st, nil
}

func newStore(rw io.ReadWriteCloser, cfg Config) *Store {
	prefix := strings.TrimSpace(cfg.Prefix)
	if prefix == "" {
		prefix = DefaultPrefix
	}
	return &Store{
		conn:       newRespConn(rw, cfg.Timeout),
		timeout:    cfg.Timeout,
		prefix:     prefix,
		ttl:        cfg.TTL,
		agentStale: cfg.AgentStale,
		now:        time.Now,
	}
}

// Prefix / TTL 回显实际生效的配置值 —— 别让人猜它连的是哪儿、TTL 是不是生效了。
func (st *Store) Prefix() string     { return st.prefix }
func (st *Store) TTL() time.Duration { return st.ttl }

// AgentStale 回显实际生效的心跳新鲜度阈值（0 = 不判）。
func (st *Store) AgentStale() time.Duration {
	if st.agentStale > 0 {
		return st.agentStale
	}
	return DefaultAgentStale
}

// Close 关掉现在手里的那条连接。
func (st *Store) Close() error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.conn == nil {
		return nil
	}
	return st.conn.Close()
}

// do 是本包所有命令的**唯一出口**。连接坏了就换一条新的，但**这条命令不重发**。
//
// 为什么不顺手重发：「重发安不安全」按命令而异 —— 读（HGETALL / GET / SCAN）重发无害，
// 但 `SET NX` 在「服务端执行了、回包没回来」这个窗口里，重发会看到 key 已存在，
// 「被别人抢了」和「自己上次其实成功了」长得一模一样。那个判据要人来拍（议题 #66），
// 这里只治「断了不会接」：下一次调用会拿到一条好连接。
//
// 另一条规矩：**标了死的流就不再写**。上一次读失败时，那条命令的回复可能还半截在路上，
// 这时候再往上写，两条命令的答复会串在一起 —— 读回一份错的值，比报个错坏得多。
func (st *Store) do(args ...string) (any, error) {
	c := st.currentConn()
	if c.isDead() {
		nc, err := st.replaceConn(c)
		if err != nil {
			return nil, fmt.Errorf("hot: 手里这条连接已经失效，且换不上新的：%w", err)
		}
		c = nc
	}
	rep, err := c.do(args...)
	if err == nil || !c.isDead() {
		return rep, err
	}
	// 这一条照实报错、不重发；顺手记下「该换了」—— 换得成，下一次调用走上面的分支直接就通。
	_, _ = st.replaceConn(c)
	return nil, err
}

func (st *Store) currentConn() *respConn {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.conn
}

// replaceConn 把坏掉的那条换掉，回「换完之后手里这条」。
// 拨号失败就留着旧的、把错回给调用方 —— 热层回来之后，谁都不用重启进程。
func (st *Store) replaceConn(bad *respConn) (*respConn, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.conn != bad {
		return st.conn, nil // 别人已经换过了 —— 用换好的那条
	}
	if st.dial == nil {
		return st.conn, errNoDial
	}
	rw, err := st.dial()
	if err != nil {
		return st.conn, err // 热层还没回来：留着旧的，下一次调用再试
	}
	st.conn = newRespConn(rw, st.timeout)
	_ = bad.Close()
	return st.conn, nil
}

// Ping 报「热层在不在」。
func (st *Store) Ping() error {
	rep, err := st.do("PING")
	if err != nil {
		return err
	}
	s, ok := rep.(string)
	if !ok || !strings.EqualFold(strings.TrimSpace(s), "PONG") {
		return fmt.Errorf("hot: PING 的回复读不懂（%v）", rep)
	}
	return nil
}

// Now 是注入的时钟（测试用），生产就是 time.Now。
func (st *Store) Now() time.Time { return st.now() }

// Put 覆写一条当前态，并把 as_of 推到「现在」。
// ttl 从 Config 来：> 0 才设过期，否则**显式**清掉上一次可能留下的过期时间 ——
// 不然会出现「我明明没配 TTL，它几小时后自己没了」这种最难查的失真。
func (st *Store) Put(s State) (State, error) {
	return st.put(s, false, 0)
}

// PutCAS 是带版本比较的写入：只有热层里的 version 仍等于 expect 时才落。
// 写入成功后 version = expect + 1 —— 版本由这里推进，调用方不再自己 ++。
//
// 为什么必须原子：ack 与 reap 会同时发生。两边都先 Get 再 Put 的话，
// 后写的那一刀会把前一刀悄悄盖掉 —— 任务明明已经回队列，却又被标成 done。
func (st *Store) PutCAS(s State, expect int64) (State, error) {
	return st.put(s, true, expect)
}

func (st *Store) put(s State, cas bool, expect int64) (State, error) {
	id := strings.TrimSpace(s.ID)
	if id == "" {
		return State{}, errors.New("hot: put 需要 id")
	}
	if cas && expect < 0 {
		return State{}, errors.New("hot: PutCAS 的 expect 不能是负数")
	}
	s.ID = id
	s.LeaseTo = ""
	s.LeaseNoExpiry = false
	if cas {
		s.Version = expect + 1
	}
	if strings.TrimSpace(s.AsOf) == "" {
		s.AsOf = st.now().Format(time.RFC3339)
	}
	key := TaskKey(st.prefix, id)
	if cas {
		return st.putCAS(s, key, expect)
	}
	args := []string{"HSET", key}
	for k, v := range s.Fields() {
		args = append(args, k, v)
	}
	if _, err := st.do(args...); err != nil {
		return State{}, err
	}
	if err := st.applyTTL(key); err != nil {
		return State{}, err
	}
	return st.withLease(s)
}

// casScript 把「比对 version + 写字段 + 处理 TTL」压成一次 EVAL：
// 三条命令分开发，中间就能插进另一个写者；EVAL 在 Redis 里是原子的。
// ARGV[1] = expect，ARGV[2] = ttl 毫秒（0 = 不失效），其余是 field/value。
const casScript = `
local cur = redis.call('HGET', KEYS[1], 'version') or '0'
if cur ~= ARGV[1] then return 0 end
local i = 3
while i < #ARGV do
  redis.call('HSET', KEYS[1], ARGV[i], ARGV[i + 1])
  i = i + 2
end
if ARGV[2] == '0' then
  redis.call('PERSIST', KEYS[1])
else
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
end
return 1
`

func (st *Store) putCAS(s State, key string, expect int64) (State, error) {
	ttl := "0"
	if st.ttl > 0 {
		ms := st.ttl.Milliseconds()
		if ms < 1 {
			ms = 1
		}
		ttl = strconv.FormatInt(ms, 10)
	}
	args := []string{"EVAL", casScript, "1", key, strconv.FormatInt(expect, 10), ttl}
	for k, v := range s.Fields() {
		args = append(args, k, v)
	}
	rep, err := st.do(args...)
	if err != nil {
		return State{}, err
	}
	n, _ := rep.(int64)
	if n != 1 {
		got, _ := st.versionOf(s.ID)
		return State{}, &ErrVersion{ID: s.ID, Want: expect, Got: got}
	}
	return st.withLease(s)
}

func (st *Store) versionOf(id string) (int64, error) {
	rep, err := st.do("HGET", TaskKey(st.prefix, id), "version")
	if err != nil {
		return 0, err
	}
	s, ok := rep.(string)
	if !ok {
		return 0, nil
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n, nil
}

// Get 读一条。第二个返回值是「有没有」—— 没有不算错：多半是已经达标清走了。
func (st *Store) Get(id string) (State, bool, error) {
	m, err := st.hash(TaskKey(st.prefix, id))
	if err != nil {
		return State{}, false, err
	}
	if len(m) == 0 {
		return State{}, false, nil
	}
	s, err := st.withLease(StateFromFields(m))
	if err != nil {
		return State{}, false, err
	}
	return s, true, nil
}

// List 列举所有「进行中」的条目（最久没动的在前）。
// 用 SCAN 而不是 KEYS：KEYS 在生产上会把整个 redis 卡住。
func (st *Store) List() ([]State, error) {
	// nil 接收者：正常写法不该走到这儿（没接上就该让那口一直空着）。但真被调到了，
	// 这里回一句人话，别当场 panic —— 调用方`var st *hot.Store`直接塞进接口时，
	// `== nil` 判不出来，读出口那一整条路会连回话都发不出去（真踩过）。
	if st == nil {
		return nil, errors.New("热层没开：Store 是 nil —— 调用方把「没接上」写成了「接上了」")
	}
	keys, err := st.scan(scanPattern(st.prefix))
	if err != nil {
		return nil, err
	}
	out := make([]State, 0, len(keys))
	for _, k := range keys {
		id := strings.TrimPrefix(k, st.prefix+":task:")
		if id == "" || id == k {
			continue
		}
		s, ok, err := st.Get(id)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, s)
		}
	}
	SortByAge(out)
	return out, nil
}

// BeatAgent 记一次心跳：覆写该 agent 的当前态，并把 as_of 推到「现在」。
//
// 心跳**不跟任务 TTL 走**：任务 TTL 是「这件事到点该走」，心跳是「最后见到它的证据」。
// 所以这里始终 PERSIST —— 过期与否由 AgentStale 打标，不由 Redis 删记录。
func (st *Store) BeatAgent(h AgentHeartbeat) (AgentHeartbeat, error) {
	slug := strings.TrimSpace(h.Slug)
	if slug == "" {
		return AgentHeartbeat{}, errors.New("hot: 心跳需要 agent slug")
	}
	h.Slug = slug
	if strings.TrimSpace(h.AsOf) == "" {
		h.AsOf = st.now().Format(time.RFC3339)
	}
	key := AgentKey(st.prefix, slug)
	args := []string{"HSET", key}
	for k, v := range h.Fields() {
		args = append(args, k, v)
	}
	if _, err := st.do(args...); err != nil {
		return AgentHeartbeat{}, err
	}
	if _, err := st.do("PERSIST", key); err != nil {
		return AgentHeartbeat{}, err
	}
	return h, nil
}

// GetAgent 读一个 agent 的最后心跳。第二个返回值是「有没有」—— 没有不算错：
// 它可能只是还没接上心跳，不能据此说它不存在。
func (st *Store) GetAgent(slug string) (AgentHeartbeat, bool, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return AgentHeartbeat{}, false, errors.New("hot: 读心跳需要 agent slug")
	}
	m, err := st.hash(AgentKey(st.prefix, slug))
	if err != nil {
		return AgentHeartbeat{}, false, err
	}
	if len(m) == 0 {
		return AgentHeartbeat{}, false, nil
	}
	return AgentHeartbeatFromFields(m), true, nil
}

// ListAgents 列举所有心跳（SCAN <prefix>:agent:*，不碰 task / lease）。
// 排序交给调用方：看板按在线、最近心跳排；CLI 按 slug 排。
func (st *Store) ListAgents() ([]AgentHeartbeat, error) {
	if st == nil {
		return nil, errors.New("热层没开：Store 是 nil —— 调用方把「没接上」写成了「接上了」")
	}
	keys, err := st.scan(agentScanPattern(st.prefix))
	if err != nil {
		return nil, err
	}
	out := make([]AgentHeartbeat, 0, len(keys))
	for _, k := range keys {
		slug := strings.TrimPrefix(k, st.prefix+":agent:")
		if slug == "" || slug == k {
			continue
		}
		h, ok, err := st.GetAgent(slug)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, h)
		}
	}
	return out, nil
}

// Claim 是原子认领（SET NX）：抢不到就报 ErrOccupied，并说出现在是谁。
// 「谁在做这件事」必须唯一 —— 两个 agent 同时以为自己在做同一件事，是最贵的错。
//
// lease > 0 = 租约到期自动放开（人或 agent 断了，事情不该被永远锁住）；
// lease <= 0 = 不设到期（默认不纠缠，客户自己配）。
func (st *Store) Claim(id, who string, lease time.Duration) (State, error) {
	return st.claim(id, who, lease, "")
}

// claim 是 Claim 的内部版本：任务 pull 时顺手把会话钉在同一个原子写里，
// 免得「认领」和「补会话」变成两次写、多一个版本。
func (st *Store) claim(id, who string, lease time.Duration, session string) (State, error) {
	return st.claimIf(id, who, lease, session, "", 0)
}

// claimIf 在认领的同时钉住两个前提：阶段必须还是 needStage，版本必须还是
// expectVersion（0 = 不校版本）。pull 用的是列表快照，快照到手之后任务可能已经
// done / 被重新回收 —— 这一层把「拿旧列表复活旧任务」挡掉。
func (st *Store) claimIf(id, who string, lease time.Duration, session, needStage string, expectVersion int64) (State, error) {
	id = strings.TrimSpace(id)
	who = strings.TrimSpace(who)
	if id == "" {
		return State{}, errors.New("hot: claim 需要 id")
	}
	if who == "" {
		return State{}, errors.New("hot: claim 需要 --by —— 认领必须落在具体的人 / agent 头上")
	}
	key := LeaseKey(st.prefix, id)
	args := []string{"SET", key, who, "NX"}
	if lease > 0 {
		args = append(args, "PX", strconv.FormatInt(lease.Milliseconds(), 10))
	}
	rep, err := st.do(args...)
	if err != nil {
		return State{}, err
	}
	if rep == nil { // NX 没抢到
		owner, _, _, lerr := st.leaseOf(id)
		if lerr != nil {
			return State{}, lerr
		}
		return State{}, &ErrOccupied{ID: id, Owner: owner}
	}
	fence, err := st.nextFence(id)
	if err != nil {
		_ = st.deleteLease(id)
		return State{}, err
	}
	// 锁已经是我们的了：把 fence 写进租约值，之后所有写都要对得上它。
	// 不写进 hash 的 version，是因为 fence 属于「谁现在有权写」，不是「内容第几版」。
	setArgs := []string{"SET", key, who + "|" + strconv.FormatInt(fence, 10)}
	if lease > 0 {
		setArgs = append(setArgs, "PX", strconv.FormatInt(lease.Milliseconds(), 10))
	}
	if _, err := st.do(setArgs...); err != nil {
		_ = st.deleteLease(id)
		return State{}, err
	}
	s, ok, err := st.Get(id)
	if err != nil {
		return State{}, err
	}
	if !ok {
		s = State{ID: id}
	}
	if strings.TrimSpace(s.Stage) == "" {
		s.Stage = DeriveStage(s.Status)
	}
	if needStage != "" && s.Stage != needStage {
		_ = st.deleteLease(id)
		return State{}, &ErrStage{ID: id, Want: needStage, Got: s.Stage}
	}
	if expectVersion > 0 && s.Version != expectVersion {
		_ = st.deleteLease(id)
		return State{}, &ErrVersion{ID: id, Want: expectVersion, Got: s.Version}
	}
	s.By = who
	if strings.TrimSpace(s.Status) == "" || strings.EqualFold(strings.TrimSpace(s.Status), StageQueued) {
		s.Status = "running"
	}
	s.Stage = StageRunning
	s.Fence = fence
	if strings.TrimSpace(s.Session) == "" {
		s.Session = strings.TrimSpace(session)
	}
	s.AsOf = st.now().Format(time.RFC3339)
	saved, err := st.PutCAS(s, s.Version)
	if err != nil {
		_ = st.deleteLease(id)
		return State{}, err
	}
	return saved, nil
}

// nextFence 给这一条任务发一个新的围栏号。INCR 是原子的：两个进程同时 claim，
// 也只会有一个拿到当前值；另一个即使拿到了更大的数，也抢不到锁。
func (st *Store) nextFence(id string) (int64, error) {
	rep, err := st.do("INCR", FenceKey(st.prefix, id))
	if err != nil {
		return 0, err
	}
	n, _ := rep.(int64)
	return n, nil
}

// Release 放开认领。**只允许认领人自己放** —— 抢别人的锁这件事，不该由热层顺手允许。
func (st *Store) Release(id, who string) error {
	raw, err := st.rawLease(id)
	if err != nil {
		return err
	}
	owner, _ := splitLease(raw)
	if owner == "" {
		return nil // 本来就没租约：当成功，别让人为了「已经放开」再跑一遍
	}
	if w := strings.TrimSpace(who); w != "" && w != owner {
		return fmt.Errorf("hot: %s 的认领人是 %s，不是 %s —— 不能替别人放开", id, owner, w)
	}
	// 比对与删除必须是一刀：比对完再 DEL，中间可能已经换了新认领人，
	// 那一 DEL 删掉的就是别人的租约。
	_, err = st.do("EVAL", releaseScript, "1", LeaseKey(st.prefix, id), raw)
	return err
}

const releaseScript = `
local cur = redis.call('GET', KEYS[1])
if not cur then return 1 end
if cur ~= ARGV[1] then return 0 end
return redis.call('DEL', KEYS[1])
`

func (st *Store) rawLease(id string) (string, error) {
	rep, err := st.do("GET", LeaseKey(st.prefix, id))
	if err != nil {
		return "", err
	}
	s, _ := rep.(string)
	return strings.TrimSpace(s), nil
}

func (st *Store) deleteLease(id string) error {
	_, err := st.do("DEL", LeaseKey(st.prefix, id))
	return err
}

// Drop 把一条进行中的状态从热层清走（状态 + 租约）。
// **只该在「已经落库」之后用**：清之前，真相源里没有的东西就真没了。
func (st *Store) Drop(id string) error {
	if _, err := st.do("DEL", TaskKey(st.prefix, id)); err != nil {
		return err
	}
	// 围栏号跟着这一条走：任务清了还留着计数器，同一个 id 再出现时会从旧号继续涨，
	// 虽然不影响正确性，但会留下再也用不到的 key。
	_, err := st.do("DEL", LeaseKey(st.prefix, id), FenceKey(st.prefix, id))
	return err
}

// ---------- 内部 ----------

func (st *Store) applyTTL(key string) error {
	if st.ttl > 0 {
		ms := st.ttl.Milliseconds()
		if ms < 1 {
			ms = 1
		}
		_, err := st.do("PEXPIRE", key, strconv.FormatInt(ms, 10))
		return err
	}
	_, err := st.do("PERSIST", key)
	return err
}

func (st *Store) hash(key string) (map[string]string, error) {
	rep, err := st.do("HGETALL", key)
	if err != nil {
		return nil, err
	}
	items, ok := rep.([]any)
	if !ok {
		if rep == nil {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("hot: HGETALL 的回复读不懂（%v）", rep)
	}
	if len(items)%2 != 0 {
		return nil, fmt.Errorf("hot: HGETALL 回了奇数个字段（%d）", len(items))
	}
	m := make(map[string]string, len(items)/2)
	for i := 0; i < len(items); i += 2 {
		k, _ := items[i].(string)
		v, _ := items[i+1].(string)
		m[k] = v
	}
	return m, nil
}

// leaseOf 回 (认领人, 围栏号, 剩余毫秒)。剩余 -1 = 没有过期时间（不到期），-2 = 没人认领。
func (st *Store) leaseOf(id string) (string, int64, int64, error) {
	key := LeaseKey(st.prefix, id)
	rep, err := st.do("GET", key)
	if err != nil {
		return "", 0, 0, err
	}
	raw, _ := rep.(string)
	owner, fence := splitLease(raw)
	if owner == "" {
		return "", 0, -2, nil
	}
	repTTL, err := st.do("PTTL", key)
	if err != nil {
		return "", 0, 0, err
	}
	ms, _ := repTTL.(int64)
	return owner, fence, ms, nil
}

// splitLease 认 `owner|fence`；旧数据只有 owner，fence 当 0。
func splitLease(v string) (string, int64) {
	parts := strings.SplitN(strings.TrimSpace(v), "|", 2)
	owner := strings.TrimSpace(parts[0])
	if len(parts) != 2 {
		return owner, 0
	}
	fence, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	return owner, fence
}

// withLease 把租约折成「到期时刻」塞进返回值 —— 租约是「还有多久」，
// 存下来立刻就成了旧值，不如回读一次算出来。
func (st *Store) withLease(s State) (State, error) {
	owner, fence, ms, err := st.leaseOf(s.ID)
	if err != nil {
		return State{}, err
	}
	s.LeaseTo = ""
	s.LeaseNoExpiry = false
	if strings.TrimSpace(owner) == "" {
		return s, nil
	}
	s.Fence = fence
	if ms == -1 {
		s.LeaseNoExpiry = true
		return s, nil
	}
	if ms > 0 {
		s.LeaseTo = st.now().Add(time.Duration(ms) * time.Millisecond).Format(time.RFC3339)
	}
	return s, nil
}

func (st *Store) scan(pattern string) ([]string, error) {
	var keys []string
	cursor := "0"
	for {
		rep, err := st.do("SCAN", cursor, "MATCH", pattern, "COUNT", "200")
		if err != nil {
			return nil, err
		}
		items, ok := rep.([]any)
		if !ok || len(items) != 2 {
			return nil, fmt.Errorf("hot: SCAN 的回复读不懂（%v）", rep)
		}
		next, _ := items[0].(string)
		page, ok := items[1].([]any)
		if !ok {
			return nil, errors.New("hot: SCAN 的 keys 读不懂")
		}
		for _, it := range page {
			if s, ok := it.(string); ok {
				keys = append(keys, s)
			}
		}
		if next == "" || next == "0" {
			return keys, nil
		}
		cursor = next
	}
}
