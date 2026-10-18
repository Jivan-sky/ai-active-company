package hot

import (
	"bufio"
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
}

func newMemRedis() *memRedis {
	return &memRedis{hashes: map[string]map[string]string{}, lease: map[string]string{}, ttl: map[string]int64{}}
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
	if saved.Status != "queued" || saved.Version != 1 || saved.Intent != "interrupt" || saved.Session != "s-1" {
		t.Fatalf("入队后的状态不对：%+v", saved)
	}
	if _, err := st.EnqueueTask(TaskSpec{ID: "t-1", To: "receptionbot", Title: "重复"}); err == nil {
		t.Fatal("同一个 id 重复入队居然成了 —— 会覆盖别人手上的事")
	}

	pulled, err := st.PullTasks("receptionbot", 1, time.Minute, "s-2")
	if err != nil {
		t.Fatalf("pull 失败：%v", err)
	}
	if len(pulled) != 1 || pulled[0].Status != "running" || pulled[0].Version != 2 {
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
	if acked.Status != "done" || acked.Version != 3 || acked.Note != "抽检完成" {
		t.Fatalf("回执后的状态不对：%+v", acked)
	}
	if acked.LeaseTo != "" {
		t.Fatalf("回执后应放开租约：%+v", acked)
	}
	if _, err := st.AckTask("t-1", "workorderbot", "done", ""); err == nil {
		t.Fatal("没被派的人也能替别人回执 —— 这件事必须唯一")
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
