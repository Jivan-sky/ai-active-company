package hot

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ---------- 假连接：按剧本回话，同时把「我们发出去的命令」留下来 ----------

type fakeConn struct {
	writes  bytes.Buffer
	replies []string
	idx     int
	left    []byte
}

func (f *fakeConn) Write(p []byte) (int, error) { return f.writes.Write(p) }

func (f *fakeConn) Read(p []byte) (int, error) {
	for len(f.left) == 0 {
		if f.idx >= len(f.replies) {
			return 0, io.EOF
		}
		f.left = []byte(f.replies[f.idx])
		f.idx++
	}
	n := copy(p, f.left)
	f.left = f.left[n:]
	return n, nil
}

func (f *fakeConn) Close() error { return nil }

func fakeStore(t *testing.T, replies ...string) (*Store, *fakeConn) {
	t.Helper()
	f := &fakeConn{replies: replies}
	return newStore(f, Config{}), f
}

// ---------- RESP 编解码 ----------

func TestRESPWrite(t *testing.T) {
	f := &fakeConn{replies: []string{"+PONG\r\n"}}
	rc := newRespConn(f, time.Second)
	rep, err := rc.do("PING")
	if err != nil {
		t.Fatalf("PING 失败：%v", err)
	}
	if rep != "PONG" {
		t.Fatalf("PING 回了 %#v，想要 PONG", rep)
	}
	want := "*1\r\n$4\r\nPING\r\n"
	if got := f.writes.String(); got != want {
		t.Fatalf("发出去的不是这个：\n got %q\nwant %q", got, want)
	}
}

func TestRESPRead(t *testing.T) {
	cases := []struct {
		name string
		wire string
		want any
	}{
		{"状态码", "+OK\r\n", "OK"},
		{"整数", ":42\r\n", int64(42)},
		{"负整数", ":-1\r\n", int64(-1)},
		{"批量串", "$5\r\nhello\r\n", "hello"},
		{"空批量串", "$0\r\n\r\n", ""},
		{"nil 批量串", "$-1\r\n", nil},
		{"数组", "*2\r\n$1\r\na\r\n$1\r\nb\r\n", []any{"a", "b"}},
		{"空数组", "*0\r\n", []any{}},
		{"nil 数组", "*-1\r\n", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeConn{replies: []string{c.wire}}
			rc := newRespConn(f, time.Second)
			got, err := rc.do("X")
			if err != nil {
				t.Fatalf("读 %q 失败：%v", c.wire, err)
			}
			if !sameAny(got, c.want) {
				t.Fatalf("读 %q 得到 %#v，想要 %#v", c.wire, got, c.want)
			}
		})
	}
}

func sameAny(a, b any) bool {
	switch av := a.(type) {
	case nil:
		return b == nil
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case int64:
		bv, ok := b.(int64)
		return ok && av == bv
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !sameAny(av[i], bv[i]) {
				return false
			}
		}
		return true
	}
	return false
}

func TestRESPErrorReply(t *testing.T) {
	f := &fakeConn{replies: []string{"-ERR no such key\r\n"}}
	rc := newRespConn(f, time.Second)
	_, err := rc.do("GET", "x")
	if err == nil || !strings.Contains(err.Error(), "no such key") {
		t.Fatalf("错误回复没被读出来：%v", err)
	}
}

// ---------- 状态本身（纯逻辑） ----------

func TestFieldsRoundTrip(t *testing.T) {
	in := State{
		ID: "t-1", Status: "running", Stage: StageRunning, Fence: 7, Attempt: 2, MaxAttempt: 3,
		By: "alice", From: "lead", To: "bob", Domain: "ops", Project: "P",
		Title: "一句话", Note: "备注", AsOf: "2026-10-10T10:00:00+08:00",
		Session: "s-1", Intent: "interrupt", Version: 7,
		Root: "root-1", Parent: "p-1", Accept: "抽检 200 通", Hop: 2, MaxHop: 6,
		DueAt: "2026-10-20T18:00:00+08:00", Priority: "high",
		Refs:    []string{"doc-a", "doc-b"},
		Handoff: Handoff{From: "alice", To: "bob", Note: "换手", CrossDomain: true, At: "2026-10-10T09:00:00+08:00"},
		Review:  Review{By: "lead", Verdict: VerdictPending, Note: "待看", At: "2026-10-10T09:30:00+08:00", Self: true},
	}
	got := StateFromFields(in.Fields())
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("绕过一圈变了：\n got %#v\nwant %#v", got, in)
	}
}

func TestFieldsWriteEmptyOnesToo(t *testing.T) {
	// 空值也要写：只写非空的那些，「上次有值、这次清空」就会留下一个删不掉的旧值。
	m := State{ID: "t-1"}.Fields()
	if v, ok := m["note"]; !ok || v != "" {
		t.Fatalf("空字段没被写出去：%#v", m)
	}
	if _, ok := m["to"]; !ok {
		t.Fatalf("空字段没被写出去：%#v", m)
	}
	if len(m) != 27 {
		t.Fatalf("字段数变了（%d）—— 加字段时顺手把这里的数一起改", len(m))
	}
}

func TestOverdueIsDerivedNotStored(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		s    State
		want bool
	}{
		{"没定截止", State{Stage: StageRunning}, false},
		{"截止读不懂", State{Stage: StageRunning, DueAt: "明天下午"}, false},
		{"还没到", State{Stage: StageRunning, DueAt: now.Add(time.Hour).Format(time.RFC3339)}, false},
		{"已经过了", State{Stage: StageRunning, DueAt: now.Add(-time.Hour).Format(time.RFC3339)}, true},
		{"done 不算超期", State{Stage: StageDone, DueAt: now.Add(-time.Hour).Format(time.RFC3339)}, false},
		{"cancelled 不算超期", State{Stage: StageCancelled, DueAt: now.Add(-time.Hour).Format(time.RFC3339)}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.s.Overdue(now); got != c.want {
				t.Fatalf("Overdue=%v，想要 %v", got, c.want)
			}
		})
	}
	late := State{Stage: StageBlocked, DueAt: now.Add(-2 * time.Hour).Format(time.RFC3339)}
	if d, ok := late.OverdueFor(now); !ok || d != 2*time.Hour {
		t.Fatalf("超期时长应为 2h，实得 %v / %v", d, ok)
	}
}

func TestDeriveStage(t *testing.T) {
	cases := map[string]string{
		"queued": StageQueued, "running": StageRunning, "review": StageReview,
		"blocked": StageBlocked, "done": StageDone, "failed": StageDone,
		"cancelled": StageCancelled, "": "", "waiting_customer": "",
	}
	for status, want := range cases {
		if got := DeriveStage(status); got != want {
			t.Fatalf("DeriveStage(%q)=%q，想要 %q", status, got, want)
		}
	}
}

func TestLeaseNoExpiryIsHeld(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	if !(State{LeaseNoExpiry: true}).LeaseHeld(now) {
		t.Fatal("没有到期时间的租约应视为仍然持有")
	}
	if (State{LeaseTo: ""}).LeaseHeld(now) {
		t.Fatal("没有租约的条目不该被当成持有")
	}
}

func TestStale(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	fresh := State{ID: "a", AsOf: now.Add(-time.Hour).Format(time.RFC3339)}
	old := State{ID: "b", AsOf: now.Add(-48 * time.Hour).Format(time.RFC3339)}
	broken := State{ID: "c", AsOf: "昨天下午"}

	if fresh.Stale(now, 24*time.Hour) {
		t.Fatal("1 小时前动过的被判成不新鲜")
	}
	if !old.Stale(now, 24*time.Hour) {
		t.Fatal("48 小时前动过的没被判成不新鲜")
	}
	if !broken.Stale(now, 24*time.Hour) {
		t.Fatal("时间读不懂的该按不新鲜算（让人去看），不是假装它新鲜")
	}
	if broken.Stale(now, 0) {
		t.Fatal("阈值 <= 0 表示不判，不该报不新鲜")
	}
}

func TestLeaseHeld(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	held := State{LeaseTo: now.Add(time.Minute).Format(time.RFC3339)}
	gone := State{LeaseTo: now.Add(-time.Minute).Format(time.RFC3339)}
	if !held.LeaseHeld(now) {
		t.Fatal("还没到期的租约被判成没了")
	}
	if gone.LeaseHeld(now) {
		t.Fatal("已经过期的租约被判成还在")
	}
	if (State{}).LeaseHeld(now) {
		t.Fatal("空租约被判成还有效")
	}
}

func TestSortByAgeOldestFirst(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	list := []State{
		{ID: "new", AsOf: now.Add(-time.Minute).Format(time.RFC3339)},
		{ID: "broken", AsOf: "看不懂"},
		{ID: "old", AsOf: now.Add(-time.Hour).Format(time.RFC3339)},
	}
	SortByAge(list)
	got := []string{list[0].ID, list[1].ID, list[2].ID}
	want := []string{"old", "new", "broken"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("排序不对：got %v want %v（卡最久的要在最前，时间读不懂的排最后）", got, want)
		}
	}
}

func TestKeyNamespacesDoNotOverlap(t *testing.T) {
	if TaskKey("anc", "x") == LeaseKey("anc", "x") {
		t.Fatal("状态和租约撞了同一个 key")
	}
	if strings.HasPrefix(LeaseKey("anc", "x"), strings.TrimSuffix(scanPattern("anc"), "*")) {
		t.Fatal("租约 key 会被列举时的 SCAN 一起捞出来 —— 每处都得分一次「这条是不是租约」，迟早漏")
	}
}

// ---------- 与热层的往返（按剧本） ----------

func TestPutWritesHashAndKeepsNoTTL(t *testing.T) {
	st, f := fakeStore(t, ":9\r\n", ":1\r\n", "$-1\r\n")
	saved, err := st.Put(State{ID: "t-1", Status: "running", Title: "一句话"})
	if err != nil {
		t.Fatalf("put 失败：%v", err)
	}
	if saved.AsOf == "" {
		t.Fatal("as_of 没被推到「现在」—— 新鲜度就没了依据")
	}
	sent := f.writes.String()
	if !strings.Contains(sent, "HSET") || !strings.Contains(sent, "anc:task:t-1") {
		t.Fatalf("没往状态 key 写：%q", sent)
	}
	if !strings.Contains(sent, "PERSIST") {
		t.Fatalf("没配 TTL 时该显式 PERSIST（清掉上一次可能留下的过期时间），实发：%q", sent)
	}
}

func TestPutAppliesConfiguredTTL(t *testing.T) {
	f := &fakeConn{replies: []string{":9\r\n", ":1\r\n", "$-1\r\n"}}
	st := newStore(f, Config{TTL: 90 * time.Minute})
	if _, err := st.Put(State{ID: "t-1"}); err != nil {
		t.Fatalf("put 失败：%v", err)
	}
	if !strings.Contains(f.writes.String(), "PEXPIRE") || !strings.Contains(f.writes.String(), "5400000") {
		t.Fatalf("配了 TTL 没设过期：%q", f.writes.String())
	}
}

func TestClaimOccupiedSaysWho(t *testing.T) {
	st, _ := fakeStore(t,
		"$-1\r\n",         // SET NX 没抢到
		"$5\r\nalice\r\n", // GET 租约：现在是 alice 占着
		":60000\r\n",      // PTTL：还剩一分钟
	)
	_, err := st.Claim("t-1", "bob", time.Minute)
	var occ *ErrOccupied
	if !errors.As(err, &occ) {
		t.Fatalf("抢同一件没报占用：%v", err)
	}
	if occ.Owner != "alice" {
		t.Fatalf("占用者报错了：%q", occ.Owner)
	}
	if !strings.Contains(occ.Error(), "alice") {
		t.Fatalf("给不了人话：%s", occ.Error())
	}
}

func TestClaimTakesOwnership(t *testing.T) {
	st, f := fakeStore(t,
		"+OK\r\n",           // SET NX 抢到了
		":1\r\n",            // INCR fence
		"+OK\r\n",           // SET 租约 owner|fence
		"*0\r\n",            // HGETALL：热层里本来还没有这条
		":1\r\n",            // EVAL：CAS 写入成功
		"$7\r\nalice|1\r\n", // GET 租约
		":-1\r\n",           // PTTL：没设到期
	)
	got, err := st.Claim("t-1", "alice", 0)
	if err != nil {
		t.Fatalf("认领失败：%v", err)
	}
	if got.By != "alice" {
		t.Fatalf("认领人没落上：%#v", got)
	}
	if got.Status != "running" {
		t.Fatalf("认领后该是 running，实得 %q", got.Status)
	}
	if got.Stage != StageRunning {
		t.Fatalf("认领后该进 running 阶段，实得 %q", got.Stage)
	}
	if got.Fence != 1 {
		t.Fatalf("围栏号该是 1，实得 %d", got.Fence)
	}
	if !got.LeaseNoExpiry {
		t.Fatal("lease=0 该记成「无到期租约」")
	}
	if got.LeaseTo != "" {
		t.Fatalf("没设到期就不该报出到期时刻：%q", got.LeaseTo)
	}
	if !strings.Contains(f.writes.String(), "SET") {
		t.Fatalf("认领没走 SET NX：%q", f.writes.String())
	}
}

func TestClaimNeedsWho(t *testing.T) {
	st, f := fakeStore(t)
	if _, err := st.Claim("t-1", "", 0); err == nil {
		t.Fatal("没给 --by 也认领成功了 —— 认领必须落在具体的人 / agent 头上")
	}
	if f.writes.Len() != 0 {
		t.Fatalf("参数不全就不该碰热层，实发：%q", f.writes.String())
	}
}

func TestListScansAndReads(t *testing.T) {
	st, f := fakeStore(t,
		"*2\r\n$1\r\n0\r\n*1\r\n$10\r\nanc:task:a\r\n",                   // SCAN 一轮就完
		"*4\r\n$2\r\nid\r\n$1\r\na\r\n$6\r\nstatus\r\n$7\r\nrunning\r\n", // HGETALL
		"$-1\r\n", // GET 租约：没人认领
		":-2\r\n", // PTTL 租约：没有这条键
	)
	list, err := st.List()
	if err != nil {
		t.Fatalf("列举失败：%v", err)
	}
	if len(list) != 1 || list[0].ID != "a" || list[0].Status != "running" {
		t.Fatalf("列举结果不对：%#v", list)
	}
	if strings.Contains(f.writes.String(), "KEYS") {
		t.Fatalf("用了 KEYS —— 在生产上会把整个 redis 卡住：%q", f.writes.String())
	}
}

// 批量读要一次发完：先全部 HGETALL，再全部 GET，最后全部 PTTL。
// 顺序错了会把 B 的租约贴到 A 上 —— 这种错不会报错，只会让看板显示错的认领人。
func TestListPipelinesHashAndLease(t *testing.T) {
	st, f := fakeStore(t,
		"*2\r\n$1\r\n0\r\n*2\r\n$10\r\nanc:task:a\r\n$10\r\nanc:task:b\r\n",
		"*4\r\n$2\r\nid\r\n$1\r\na\r\n$6\r\nstatus\r\n$7\r\nrunning\r\n",
		"*4\r\n$2\r\nid\r\n$1\r\nb\r\n$6\r\nstatus\r\n$6\r\nqueued\r\n",
		"$7\r\nalice|7\r\n",
		"$-1\r\n",
		":60000\r\n",
		":-2\r\n",
	)
	list, err := st.List()
	if err != nil {
		t.Fatalf("列举失败：%v", err)
	}
	byID := map[string]State{}
	for _, s := range list {
		byID[s.ID] = s
	}
	if len(byID) != 2 {
		t.Fatalf("该有两条：%#v", list)
	}
	a := byID["a"]
	if a.Fence != 7 || a.LeaseTo == "" {
		t.Fatalf("a 的租约没折进来：%+v", a)
	}
	b := byID["b"]
	if b.Fence != 0 || b.LeaseTo != "" || b.LeaseNoExpiry {
		t.Fatalf("b 没有租约，不该借到 a 的：%+v", b)
	}
	w := f.writes.String()
	if got := strings.Count(w, "$7\r\nHGETALL\r\n"); got != 2 {
		t.Fatalf("HGETALL 该发 2 条，实发 %d：%q", got, w)
	}
	if got := strings.Count(w, "$3\r\nGET\r\n"); got != 2 {
		t.Fatalf("GET 租约该发 2 条，实发 %d：%q", got, w)
	}
	if got := strings.Count(w, "$4\r\nPTTL\r\n"); got != 2 {
		t.Fatalf("PTTL 该发 2 条，实发 %d：%q", got, w)
	}
	if strings.Index(w, "$3\r\nGET\r\n") < strings.LastIndex(w, "HGETALL") {
		t.Fatalf("租约读取不该插在 hash 读取中间：%q", w)
	}
}

// nil 的 Store 真被调到了，要回一句人话，不许当场 panic：调用方要是把「没接上」写成一个
// 「装着 nil 指针的接口」，`if x == nil` 判不出来 —— 那一下就会打到这儿。
// （读出口那条路真踩过：MCP 接入面在热层没接上时直接断连。）
func TestListOnNilStoreSaysSo(t *testing.T) {
	var st *Store
	_, err := st.List()
	if err == nil || !strings.Contains(err.Error(), "热层没开") {
		t.Fatalf("nil 的 Store 该回一句人话，实际：%v", err)
	}
}

func TestGetMissingIsNotAnError(t *testing.T) {
	st, _ := fakeStore(t, "*0\r\n")
	_, ok, err := st.Get("nope")
	if err != nil {
		t.Fatalf("「没有这一条」不该报错：%v", err)
	}
	if ok {
		t.Fatal("没有这一条却报有")
	}
}

func TestReleaseRefusesOthersLease(t *testing.T) {
	st, f := fakeStore(t, "$5\r\nalice\r\n", ":-1\r\n")
	if err := st.Release("t-1", "bob"); err == nil {
		t.Fatal("替别人放开认领居然成了")
	}
	if strings.Contains(f.writes.String(), "DEL") {
		t.Fatal("被拒的放开不该真的动热层")
	}
}

func TestReleaseNoLeaseIsFine(t *testing.T) {
	st, _ := fakeStore(t, "$-1\r\n")
	if err := st.Release("t-1", "alice"); err != nil {
		t.Fatalf("本来就没租约，该当成功：%v", err)
	}
}
