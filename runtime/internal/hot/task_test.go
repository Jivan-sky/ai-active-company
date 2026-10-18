package hot

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// memRedis 是任务用例的内存热层：只实现任务路径真正用到的命令。
// 它不模拟过期时间，只记录 PTTL —— 用例关心的是「谁拿到、版本走到哪」，
// 不是 Redis 的定时器语义。
type memRedis struct {
	mu     sync.Mutex
	hashes map[string]map[string]string
	lease  map[string]string
	ttl    map[string]int64
	nums   map[string]int64
}

func newMemRedis() *memRedis {
	return &memRedis{
		hashes: map[string]map[string]string{}, lease: map[string]string{},
		ttl: map[string]int64{}, nums: map[string]int64{},
	}
}

func (m *memRedis) listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go m.serve(c)
		}
	}()
	return ln
}

func (m *memRedis) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	for {
		args, err := readCmdArgs(r)
		if err != nil {
			return
		}
		if err := m.reply(w, args); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}
	}
}

func (m *memRedis) reply(w *bufio.Writer, args []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cmd := strings.ToUpper(args[0])
	switch cmd {
	case "PING":
		_, err := w.WriteString("+PONG\r\n")
		return err
	case "HGETALL":
		h := m.hashes[args[1]]
		keys := make([]string, 0, len(h))
		for k := range h {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Fprintf(w, "*%d\r\n", len(keys)*2)
		for _, k := range keys {
			writeBulk(w, k)
			writeBulk(w, h[k])
		}
		return nil
	case "HGET":
		h := m.hashes[args[1]]
		if v, ok := h[args[2]]; ok {
			writeBulk(w, v)
		} else {
			_, err := w.WriteString("$-1\r\n")
			return err
		}
		return nil
	case "HSET":
		h := m.hashes[args[1]]
		if h == nil {
			h = map[string]string{}
			m.hashes[args[1]] = h
		}
		n := 0
		for i := 2; i+1 < len(args); i += 2 {
			if _, ok := h[args[i]]; !ok {
				n++
			}
			h[args[i]] = args[i+1]
		}
		fmt.Fprintf(w, ":%d\r\n", n)
		return nil
	case "GET":
		if v, ok := m.lease[args[1]]; ok {
			writeBulk(w, v)
		} else {
			_, err := w.WriteString("$-1\r\n")
			return err
		}
		return nil
	case "SET":
		key, val := args[1], args[2]
		nx := len(args) > 3 && strings.EqualFold(args[3], "NX")
		if nx {
			if _, ok := m.lease[key]; ok {
				_, err := w.WriteString("$-1\r\n")
				return err
			}
		}
		m.lease[key] = val
		if len(args) >= 6 && strings.EqualFold(args[4], "PX") {
			ms, _ := strconv.ParseInt(args[5], 10, 64)
			m.ttl[key] = ms
		}
		_, err := w.WriteString("+OK\r\n")
		return err
	case "PTTL":
		if ms, ok := m.ttl[args[1]]; ok {
			fmt.Fprintf(w, ":%d\r\n", ms)
		} else if _, ok := m.lease[args[1]]; ok {
			_, err := w.WriteString(":-1\r\n")
			return err
		} else {
			_, err := w.WriteString(":-2\r\n")
			return err
		}
		return nil
	case "PERSIST", "PEXPIRE":
		_, err := w.WriteString(":1\r\n")
		return err
	case "INCR":
		m.nums[args[1]]++
		fmt.Fprintf(w, ":%d\r\n", m.nums[args[1]])
		return nil
	case "EVAL":
		if len(args) == 5 { // releaseScript：EVAL script 1 key 期望值
			key, want := args[3], args[4]
			if m.lease[key] == want {
				delete(m.lease, key)
				delete(m.ttl, key)
				_, err := w.WriteString(":1\r\n")
				return err
			}
			_, err := w.WriteString(":0\r\n")
			return err
		}
		// casScript：EVAL script 1 key expect ttl field value ...
		if len(args) < 7 || args[2] != "1" {
			return fmt.Errorf("memRedis 只认识单 key 的 CAS script")
		}
		key, expect := args[3], args[4]
		h := m.hashes[key]
		got := "0"
		if h != nil {
			if v, ok := h["version"]; ok {
				got = v
			}
		}
		if got != expect {
			_, err := w.WriteString(":0\r\n")
			return err
		}
		if h == nil {
			h = map[string]string{}
			m.hashes[key] = h
		}
		for i := 6; i+1 < len(args); i += 2 {
			h[args[i]] = args[i+1]
		}
		_, err := w.WriteString(":1\r\n")
		return err
	case "DEL":
		n := 0
		for _, k := range args[1:] {
			if _, ok := m.hashes[k]; ok {
				delete(m.hashes, k)
				n++
			}
			if _, ok := m.lease[k]; ok {
				delete(m.lease, k)
				delete(m.ttl, k)
				n++
			}
			if _, ok := m.nums[k]; ok {
				delete(m.nums, k)
				n++
			}
		}
		fmt.Fprintf(w, ":%d\r\n", n)
		return nil
	case "SCAN":
		pattern := ""
		for i := 1; i+1 < len(args); i++ {
			if strings.EqualFold(args[i], "MATCH") {
				pattern = args[i+1]
			}
		}
		var keys []string
		for k := range m.hashes {
			if pattern == "" {
				keys = append(keys, k)
				continue
			}
			if ok, _ := path.Match(pattern, k); ok {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		fmt.Fprintf(w, "*2\r\n$1\r\n0\r\n*%d\r\n", len(keys))
		for _, k := range keys {
			writeBulk(w, k)
		}
		return nil
	}
	return fmt.Errorf("memRedis 不认识的命令：%s", cmd)
}

func writeBulk(w *bufio.Writer, s string) {
	fmt.Fprintf(w, "$%d\r\n%s\r\n", len(s), s)
}

func openMemStore(t *testing.T) *Store {
	t.Helper()
	ln := newMemRedis().listen(t)
	addr := ln.Addr().String()
	rw, err := dialResp(addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	st := newStore(rw, Config{Timeout: 2 * time.Second})
	st.dial = func() (io.ReadWriteCloser, error) { return dialResp(addr, 2*time.Second) }
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestTaskQueueEnqueuePullAck(t *testing.T) {
	st := openMemStore(t)
	saved, err := st.EnqueueTask(TaskSpec{
		ID: "t-1", To: "receptionbot", By: "cs-owner", Title: "抽检会话",
		Note: "200 通", Domain: "cs-ops", Project: "reception", Session: "s-1", Intent: "interrupt",
	})
	if err != nil {
		t.Fatalf("入队失败：%v", err)
	}
	if saved.Status != "queued" || saved.Stage != StageQueued || saved.Version != 1 || saved.Intent != "interrupt" || saved.Session != "s-1" {
		t.Fatalf("入队后的状态不对：%+v", saved)
	}
	if saved.MaxAttempt != DefaultTaskMaxAttempt {
		t.Fatalf("默认重试上限不对：%+v", saved)
	}
	if _, err := st.EnqueueTask(TaskSpec{ID: "t-1", To: "receptionbot", Title: "重复"}); err == nil {
		t.Fatal("同一个 id 重复入队居然成了 —— 会覆盖别人手上的事")
	}

	pulled, err := st.PullTasks("receptionbot", 1, time.Minute, "s-2")
	if err != nil {
		t.Fatalf("pull 失败：%v", err)
	}
	if len(pulled) != 1 || pulled[0].Status != "running" || pulled[0].Stage != StageRunning || pulled[0].Version != 2 || pulled[0].Fence == 0 {
		t.Fatalf("pull 后应是 running / v2：%+v", pulled)
	}
	if pulled[0].Intent != "interrupt" || pulled[0].Session != "s-1" {
		t.Fatalf("pull 把 intent / session 弄丢了：%+v", pulled[0])
	}
	if again, _ := st.PullTasks("receptionbot", 1, time.Minute, "s-2"); len(again) != 0 {
		t.Fatalf("同一条被拉了两次：%+v", again)
	}
	if other, _ := st.PullTasks("workorderbot", 1, time.Minute, "s-2"); len(other) != 0 {
		t.Fatalf("别的 agent 拉到了不属于它的任务：%+v", other)
	}

	acked, err := st.AckTask("t-1", "receptionbot", "done", "抽检完成")
	if err != nil {
		t.Fatalf("回执失败：%v", err)
	}
	if acked.Status != "done" || acked.Stage != StageDone || acked.Version != 3 || acked.Note != "抽检完成" {
		t.Fatalf("回执后的状态不对：%+v", acked)
	}
	if acked.LeaseTo != "" {
		t.Fatalf("回执后应放开租约：%+v", acked)
	}
	if _, err := st.AckTask("t-1", "workorderbot", "done", ""); err == nil {
		t.Fatal("没被派的人也能替别人回执 —— 这件事必须唯一")
	}
}

func TestCASRejectsStaleVersion(t *testing.T) {
	st := openMemStore(t)
	if _, err := st.EnqueueTask(TaskSpec{ID: "t-1", To: "alice", Title: "一件事"}); err != nil {
		t.Fatalf("入队失败：%v", err)
	}
	cur, ok, err := st.Get("t-1")
	if err != nil || !ok {
		t.Fatalf("读不到刚入队的任务：ok=%v err=%v", ok, err)
	}
	cur.Status = "review"
	cur.Stage = StageReview
	first, err := st.PutCAS(cur, cur.Version)
	if err != nil {
		t.Fatalf("第一次 CAS 该成功：%v", err)
	}
	if first.Version != cur.Version+1 {
		t.Fatalf("CAS 该把版本推到 %d，实得 %d", cur.Version+1, first.Version)
	}
	cur.Status = "done"
	cur.Stage = StageDone
	_, err = st.PutCAS(cur, first.Version-1)
	var stale *ErrVersion
	if !errors.As(err, &stale) {
		t.Fatalf("拿着旧版本写该被拒，实得 %v", err)
	}
	if stale.Got != first.Version {
		t.Fatalf("冲突里该报当前版本 %d，实得 %d", first.Version, stale.Got)
	}
	now, _, _ := st.Get("t-1")
	if now.Status != "review" || now.Stage != StageReview {
		t.Fatalf("被拒的写不该动热层：%+v", now)
	}
}

func TestPullSnapshotCannotResurrectDoneTask(t *testing.T) {
	st := openMemStore(t)
	if _, err := st.EnqueueTask(TaskSpec{ID: "t-1", To: "alice", Title: "一件事"}); err != nil {
		t.Fatalf("入队失败：%v", err)
	}
	snapshot, err := st.List()
	if err != nil || len(snapshot) != 1 {
		t.Fatalf("拿不到快照：%v %+v", err, snapshot)
	}
	pulled, err := st.PullTasks("alice", 1, time.Minute, "")
	if err != nil || len(pulled) != 1 {
		t.Fatalf("pull 失败：%v %+v", err, pulled)
	}
	if _, err := st.AckTaskFenced("t-1", "alice", pulled[0].Fence, "done", ""); err != nil {
		t.Fatalf("回执失败：%v", err)
	}
	// 模拟「pull 拿着旧列表、迟一步才来认领」：这时任务已经 done。
	_, err = st.claimIf("t-1", "alice", time.Minute, "", StageQueued, snapshot[0].Version)
	var stage *ErrStage
	if !errors.As(err, &stage) {
		t.Fatalf("拿旧快照认领已完成任务该被拒，实得 %v", err)
	}
	now, ok, err := st.Get("t-1")
	if err != nil || !ok {
		t.Fatalf("读不到任务：ok=%v err=%v", ok, err)
	}
	if now.Stage != StageDone || now.LeaseTo != "" || now.LeaseNoExpiry {
		t.Fatalf("被拒的认领不该复活任务或留下租约：%+v", now)
	}
}

func TestTaskQueuePullHonoursLimitAndOrder(t *testing.T) {
	st := openMemStore(t)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	st.now = func() time.Time { return now }
	for _, id := range []string{"t-old", "t-new"} {
		_, err := st.EnqueueTask(TaskSpec{ID: id, To: "receptionbot", Title: id})
		if err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Second) // 让两条的 as_of 拉开，pull 应先拿最老的
	}
	got, err := st.PullTasks("receptionbot", 1, time.Minute, "s-current")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "t-old" {
		t.Fatalf("limit=1 应先拿最老的那条：%+v", got)
	}
}

func TestTaskReapRequeuesExpiredLease(t *testing.T) {
	st := openMemStore(t)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	st.now = func() time.Time { return now }
	if _, err := st.EnqueueTask(TaskSpec{ID: "t-reap", To: "receptionbot", Title: "会过期", MaxAttempt: 2}); err != nil {
		t.Fatal(err)
	}
	first, err := st.PullTasks("receptionbot", 1, time.Minute, "s-1")
	if err != nil || len(first) != 1 {
		t.Fatalf("pull：%v %+v", err, first)
	}
	oldFence := first[0].Fence
	if err := st.deleteLease("t-reap"); err != nil { // 模拟 Redis 到点删掉租约
		t.Fatal(err)
	}
	rep, err := st.ReapExpired(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Requeued) != 1 || len(rep.Escalated) != 0 {
		t.Fatalf("回收结果不对：%+v", rep)
	}
	got := rep.Requeued[0]
	if got.Stage != StageQueued || got.Status != "queued" || got.Attempt != 1 || got.Fence <= oldFence {
		t.Fatalf("回收后的状态不对：%+v", got)
	}
	second, err := st.PullTasks("receptionbot", 1, time.Minute, "s-1")
	if err != nil || len(second) != 1 {
		t.Fatalf("二次 pull：%v %+v", err, second)
	}
	if second[0].Fence <= got.Fence {
		t.Fatalf("二次认领没发新 fence：%+v", second[0])
	}
}

func TestTaskReapEscalatesAfterMaxAttempts(t *testing.T) {
	st := openMemStore(t)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	st.now = func() time.Time { return now }
	if _, err := st.EnqueueTask(TaskSpec{ID: "t-escalate", To: "receptionbot", Title: "会升级", MaxAttempt: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PullTasks("receptionbot", 1, time.Minute, "s-1"); err != nil {
		t.Fatal(err)
	}
	if err := st.deleteLease("t-escalate"); err != nil {
		t.Fatal(err)
	}
	first, err := st.ReapExpired(now)
	if err != nil || len(first.Requeued) != 1 {
		t.Fatalf("第一次回收：%v %+v", err, first)
	}
	if _, err := st.PullTasks("receptionbot", 1, time.Minute, "s-1"); err != nil {
		t.Fatal(err)
	}
	if err := st.deleteLease("t-escalate"); err != nil {
		t.Fatal(err)
	}
	second, err := st.ReapExpired(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Escalated) != 1 || len(second.Requeued) != 0 {
		t.Fatalf("超过上限应升级：%+v", second)
	}
	if got := second.Escalated[0]; got.Stage != StageBlocked || got.Status != "blocked" || got.Attempt != 2 {
		t.Fatalf("升级后的状态不对：%+v", got)
	}
}

func TestAckRejectsStaleFence(t *testing.T) {
	st := openMemStore(t)
	if _, err := st.EnqueueTask(TaskSpec{ID: "t-fence", To: "receptionbot", Title: "围栏"}); err != nil {
		t.Fatal(err)
	}
	first, err := st.PullTasks("receptionbot", 1, time.Minute, "s-1")
	if err != nil || len(first) != 1 {
		t.Fatalf("pull：%v %+v", err, first)
	}
	if err := st.deleteLease("t-fence"); err != nil {
		t.Fatal(err)
	}
	second, err := st.Claim("t-fence", "receptionbot", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if second.Fence <= first[0].Fence {
		t.Fatalf("重新认领没发更大的 fence：%+v", second)
	}
	if _, err := st.AckTaskFenced("t-fence", "receptionbot", first[0].Fence, "done", ""); err == nil {
		t.Fatal("旧 fence 居然写进去了 —— 僵尸写没被拦住")
	}
	if _, err := st.AckTaskFenced("t-fence", "receptionbot", second.Fence, "done", ""); err != nil {
		t.Fatalf("新 fence 应能回执：%v", err)
	}
}

func TestAckRejectsRunningWithoutLease(t *testing.T) {
	st := openMemStore(t)
	if _, err := st.EnqueueTask(TaskSpec{ID: "t-no-lease", To: "receptionbot", Title: "没租约"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PullTasks("receptionbot", 1, time.Minute, "s-1"); err != nil {
		t.Fatal(err)
	}
	if err := st.deleteLease("t-no-lease"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AckTask("t-no-lease", "receptionbot", "done", ""); err == nil {
		t.Fatal("租约已经没了还能回执 —— 旧持有者应被拒绝")
	}
}

func TestTaskHandoffMovesToNextAgent(t *testing.T) {
	st := openMemStore(t)
	if _, err := st.EnqueueTask(TaskSpec{ID: "t-hop", To: "alice", By: "lead", Title: "交接", Refs: []string{"doc-1"}}); err != nil {
		t.Fatal(err)
	}
	first, err := st.PullTasks("alice", 1, time.Minute, "s-1")
	if err != nil || len(first) != 1 {
		t.Fatalf("pull：%v %+v", err, first)
	}
	got, err := st.HandoffTask("t-hop", "alice", first[0].Fence, "bob", "这活该 bob 接", []string{"doc-2"}, false)
	if err != nil {
		t.Fatalf("交接失败：%v", err)
	}
	if got.Stage != StageQueued || got.Status != "queued" || got.To != "bob" || got.By != "" {
		t.Fatalf("交接后的状态不对：%+v", got)
	}
	if got.Hop != 1 || got.Handoff.From != "alice" || got.Handoff.To != "bob" || got.Handoff.Note == "" {
		t.Fatalf("交接记录不对：%+v", got.Handoff)
	}
	if got.Root != "t-hop" || got.From != "lead" {
		t.Fatalf("交接不该换根 / 换派出人：%+v", got)
	}
	if len(got.Refs) != 2 {
		t.Fatalf("交接的 refs 应合并进任务：%+v", got.Refs)
	}
	if got.LeaseTo != "" || got.LeaseNoExpiry {
		t.Fatalf("交接后不该留租约：%+v", got)
	}
	second, err := st.PullTasks("bob", 1, time.Minute, "s-1")
	if err != nil || len(second) != 1 || second[0].ID != "t-hop" {
		t.Fatalf("下一手拉不到：%v %+v", err, second)
	}
}

func TestTaskHandoffRejectsStaleFence(t *testing.T) {
	st := openMemStore(t)
	if _, err := st.EnqueueTask(TaskSpec{ID: "t-stale-hop", To: "alice", Title: "旧围栏"}); err != nil {
		t.Fatal(err)
	}
	first, err := st.PullTasks("alice", 1, time.Minute, "s-1")
	if err != nil || len(first) != 1 {
		t.Fatalf("pull：%v %+v", err, first)
	}
	if err := st.deleteLease("t-stale-hop"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Claim("t-stale-hop", "alice", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := st.HandoffTask("t-stale-hop", "alice", first[0].Fence, "bob", "过期持有者", nil, false); err == nil {
		t.Fatal("旧 fence 居然交出去了 —— 僵尸交接没被拦住")
	}
}

func TestTaskHandoffBlocksAfterMaxHop(t *testing.T) {
	st := openMemStore(t)
	if _, err := st.EnqueueTask(TaskSpec{ID: "t-max-hop", To: "alice", Title: "交接成环", MaxHop: 1}); err != nil {
		t.Fatal(err)
	}
	a, err := st.PullTasks("alice", 1, time.Minute, "s-1")
	if err != nil || len(a) != 1 {
		t.Fatalf("pull alice：%v %+v", err, a)
	}
	if _, err := st.HandoffTask("t-max-hop", "alice", a[0].Fence, "bob", "第一手", nil, false); err != nil {
		t.Fatal(err)
	}
	b, err := st.PullTasks("bob", 1, time.Minute, "s-1")
	if err != nil || len(b) != 1 {
		t.Fatalf("pull bob：%v %+v", err, b)
	}
	got, err := st.HandoffTask("t-max-hop", "bob", b[0].Fence, "carol", "第二手", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Stage != StageBlocked || got.Status != "blocked" || got.To != "carol" {
		t.Fatalf("超过 max_hop 应停在 blocked 并留着下一手：%+v", got)
	}
	next, err := st.PullTasks("carol", 1, time.Minute, "s-1")
	if err != nil || len(next) != 0 {
		t.Fatalf("blocked 不该被 pull 走：%v %+v", err, next)
	}
}

func TestTaskSubmitReviewRejectAndAccept(t *testing.T) {
	st := openMemStore(t)
	if _, err := st.EnqueueTask(TaskSpec{ID: "t-review", To: "alice", By: "lead", Title: "交验收", Accept: "抽检 200 通"}); err != nil {
		t.Fatal(err)
	}
	run, err := st.PullTasks("alice", 1, time.Minute, "s-1")
	if err != nil || len(run) != 1 {
		t.Fatalf("pull：%v %+v", err, run)
	}
	submitted, err := st.SubmitTask("t-review", "alice", run[0].Fence, "lead", "抽检 200 通且零漏检", "做完了", []string{"report-1"})
	if err != nil {
		t.Fatalf("提交失败：%v", err)
	}
	if submitted.Stage != StageReview || submitted.To != "lead" || submitted.By != "alice" {
		t.Fatalf("提交后的状态不对：%+v", submitted)
	}
	if submitted.Review.Verdict != VerdictPending || submitted.Review.By != "lead" || submitted.Accept == "" {
		t.Fatalf("验收记录不对：%+v", submitted.Review)
	}
	if submitted.LeaseTo != "" || submitted.LeaseNoExpiry {
		t.Fatalf("提交后应放开租约：%+v", submitted)
	}
	if next, err := st.PullTasks("alice", 1, time.Minute, "s-1"); err != nil || len(next) != 0 {
		t.Fatalf("review 阶段不该被 pull：%v %+v", err, next)
	}
	if _, err := st.VerifyTask("t-review", "bob", VerdictAccepted, ""); err == nil {
		t.Fatal("非验收人居然能验收")
	}
	if _, err := st.VerifyTask("t-review", "lead", VerdictRejected, ""); err == nil {
		t.Fatal("驳回没写理由居然成了")
	}
	rejected, err := st.VerifyTask("t-review", "lead", VerdictRejected, "漏了 3 通")
	if err != nil {
		t.Fatalf("驳回失败：%v", err)
	}
	if rejected.Stage != StageQueued || rejected.To != "alice" || rejected.By != "" {
		t.Fatalf("驳回后应回作者队列：%+v", rejected)
	}
	again, err := st.PullTasks("alice", 1, time.Minute, "s-1")
	if err != nil || len(again) != 1 {
		t.Fatalf("驳回后作者拉不到：%v %+v", err, again)
	}
	if _, err := st.SubmitTask("t-review", "alice", again[0].Fence, "lead", "", "补完了", nil); err != nil {
		t.Fatalf("二次提交应沿用已有验收口径：%v", err)
	}
	accepted, err := st.VerifyTask("t-review", "lead", "accept", "通过")
	if err != nil {
		t.Fatalf("验收通过失败：%v", err)
	}
	if accepted.Stage != StageDone || accepted.Status != "done" || accepted.Review.Verdict != VerdictAccepted {
		t.Fatalf("验收通过后的状态不对：%+v", accepted)
	}
}

func TestTaskSubmitAllowsSelfReviewButMarksIt(t *testing.T) {
	st := openMemStore(t)
	if _, err := st.EnqueueTask(TaskSpec{ID: "t-self", To: "alice", Title: "自验", Accept: "自己看"}); err != nil {
		t.Fatal(err)
	}
	run, err := st.PullTasks("alice", 1, time.Minute, "s-1")
	if err != nil || len(run) != 1 {
		t.Fatalf("pull：%v %+v", err, run)
	}
	submitted, err := st.SubmitTask("t-self", "alice", run[0].Fence, "alice", "", "自己交给自己", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !submitted.Review.Self {
		t.Fatalf("自验没有被标出来：%+v", submitted.Review)
	}
	done, err := st.VerifyTask("t-self", "alice", VerdictAccepted, "看过")
	if err != nil {
		t.Fatal(err)
	}
	if !done.Review.Self || done.Stage != StageDone {
		t.Fatalf("自验结果不对：%+v", done)
	}
}

func TestTaskEscalateBlocksWithNextOwner(t *testing.T) {
	st := openMemStore(t)
	if _, err := st.EnqueueTask(TaskSpec{ID: "t-block", To: "alice", Title: "卡点"}); err != nil {
		t.Fatal(err)
	}
	run, err := st.PullTasks("alice", 1, time.Minute, "s-1")
	if err != nil || len(run) != 1 {
		t.Fatalf("pull：%v %+v", err, run)
	}
	if _, err := st.EscalateTask("t-block", "alice", run[0].Fence, "lead", ""); err == nil {
		t.Fatal("卡点没写说明居然能升级")
	}
	got, err := st.EscalateTask("t-block", "alice", run[0].Fence, "lead", "等客户确认口径")
	if err != nil {
		t.Fatal(err)
	}
	if got.Stage != StageBlocked || got.Status != "blocked" || got.To != "lead" {
		t.Fatalf("升级后的状态不对：%+v", got)
	}
	if got.LeaseTo != "" || got.LeaseNoExpiry {
		t.Fatalf("升级后不该留租约：%+v", got)
	}
}

func TestTaskDueAndPrioritySurviveHandoff(t *testing.T) {
	st := openMemStore(t)
	due := "2026-10-20T18:00:00+08:00"
	if _, err := st.EnqueueTask(TaskSpec{
		ID: "t-due", To: "alice", Title: "有截止", DueAt: due, Priority: "high",
	}); err != nil {
		t.Fatal(err)
	}
	run, err := st.PullTasks("alice", 1, time.Minute, "s-1")
	if err != nil || len(run) != 1 {
		t.Fatalf("pull：%v %+v", err, run)
	}
	if run[0].DueAt != due || run[0].Priority != "high" {
		t.Fatalf("due / priority 没落上：%+v", run[0])
	}
	ho, err := st.HandoffTask("t-due", "alice", run[0].Fence, "bob", "换手", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if ho.DueAt != due || ho.Priority != "high" {
		t.Fatalf("交接后 due / priority 丢了：%+v", ho)
	}
}

func TestTaskEnqueueRejectsBadDue(t *testing.T) {
	st := openMemStore(t)
	if _, err := st.EnqueueTask(TaskSpec{ID: "t-bad-due", To: "alice", Title: "坏截止", DueAt: "明天下午"}); err == nil {
		t.Fatal("due_at 读不懂也入队了 —— 那会让超期判断静默失效")
	}
}
