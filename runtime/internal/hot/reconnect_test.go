package hot

import (
	"bufio"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------- 常驻侧的「断了不会接」（议题 #66） ----------
//
// 真事（2026-10-11）：Store 在 Open 时拨一次号，此后的命令全走那一条。热层重启一次、
// 网络抖一下，从那以后每条命令都报错，**没有任何重拨** —— 看板那一格永远「读不到」，
// 只有重启看板才会好。
//
// 现在的规矩：连接坏了就换一条新的，但**坏掉的那条命令不重发**
// （「重发安不安全」按命令而异，那个判据待拍）。下面两条用例盯的就是这个。

// flakyRedis 是个会掐线的假热层：每条连接上答完 dropAfter 条，就把连接关掉。
type flakyRedis struct {
	mu        sync.Mutex
	perConn   [][]string // 每条连接上收到的命令，按顺序
	dropAfter int
	reply     string // 非空时用它代替正常回话（用来造「热层自己回的错」）
}

func (s *flakyRedis) serve(c net.Conn) {
	defer c.Close()
	s.mu.Lock()
	idx := len(s.perConn)
	s.perConn = append(s.perConn, nil)
	s.mu.Unlock()

	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	for {
		args, err := readCmdArgs(r)
		if err != nil {
			return
		}
		s.mu.Lock()
		s.perConn[idx] = append(s.perConn[idx], strings.Join(args, " "))
		n := len(s.perConn[idx])
		s.mu.Unlock()
		if n > s.dropAfter {
			return // 不答了，直接断 —— 模拟 redis 重启 / 对端 RST
		}
		line := "+ok\r\n"
		if s.reply != "" {
			line = s.reply
		}
		if _, err := w.WriteString(line); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}
	}
}

func (s *flakyRedis) listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("起假热层失败：%v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return ln
}

func (s *flakyRedis) conns() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]string, len(s.perConn))
	for i, c := range s.perConn {
		out[i] = append([]string(nil), c...)
	}
	return out
}

// openStoreOn 起一个「和 Open 一样会自己拨号」的 Store，但接到假热层上。
func openStoreOn(t *testing.T, ln net.Listener) *Store {
	t.Helper()
	addr := ln.Addr().String()
	rw, err := dialResp(addr, 2*time.Second)
	if err != nil {
		t.Fatalf("连不上假热层：%v", err)
	}
	st := newStore(rw, Config{Timeout: 2 * time.Second})
	st.dial = func() (io.ReadWriteCloser, error) { return dialResp(addr, 2*time.Second) }
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// readCmdArgs 读一条 RESP 命令（`*n` + `$len` 列表）—— 只认请求方向这一种形状。
func readCmdArgs(r *bufio.Reader) ([]string, error) {
	head, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	head = strings.TrimRight(head, "\r\n")
	if !strings.HasPrefix(head, "*") {
		return nil, errors.New("不是数组命令：" + head)
	}
	n, err := strconv.Atoi(head[1:])
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		lenLine, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		lenLine = strings.TrimRight(lenLine, "\r\n")
		if !strings.HasPrefix(lenLine, "$") {
			return nil, errors.New("不是批量串：" + lenLine)
		}
		size, err := strconv.Atoi(lenLine[1:])
		if err != nil {
			return nil, err
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		out = append(out, string(buf[:size]))
	}
	return out, nil
}

func TestStoreRedialsAfterConnDies(t *testing.T) {
	srv := &flakyRedis{dropAfter: 1}
	st := openStoreOn(t, srv.listen(t))

	if rep, err := st.do("ECHO", "一"); err != nil || rep != "ok" {
		t.Fatalf("第 1 条该通：err=%v rep=%#v", err, rep)
	}
	if _, err := st.do("ECHO", "二"); err == nil {
		t.Fatalf("对端已经把连接掐了，这一条必须报错 —— 不许吞")
	}
	if rep, err := st.do("ECHO", "三"); err != nil || rep != "ok" {
		t.Fatalf("换连接之后没有自愈：err=%v rep=%#v", err, rep)
	}

	conns := srv.conns()
	if len(conns) != 2 {
		t.Fatalf("该换一条连接（共 2 条），实际 %d 条：%v", len(conns), conns)
	}
	if len(conns[0]) != 2 {
		t.Fatalf("第 1 条连接上该有 2 条命令（1 条答了、1 条被掐），实际 %v", conns[0])
	}
	// 关键一条：坏掉的那条**没有被重发** —— 新连接上第一条命令就是「三」。
	if len(conns[1]) == 0 || conns[1][0] != "ECHO 三" {
		t.Fatalf("新连接上第一条命令是 %v，想要 [ECHO 三] —— 说明坏掉的那条被重发了", conns[1])
	}
}

func TestStoreKeepsConnWhenServerRepliesError(t *testing.T) {
	srv := &flakyRedis{dropAfter: 99, reply: "-ERR 命令不认\r\n"}
	st := openStoreOn(t, srv.listen(t))

	for i := 0; i < 3; i++ {
		if _, err := st.do("NOPE"); err == nil {
			t.Fatalf("热层回了错，第 %d 次 do 不该报成功", i+1)
		}
	}
	if n := len(srv.conns()); n != 1 {
		t.Fatalf("对端只是回了个错、连接是好的 —— 不该换连接，实际换了 %d 条", n)
	}
}

// dialer 是可控的「再拨一条」：数着被叫了几次，前 failFirst 次当作拨不通。
type dialer struct {
	mu        sync.Mutex
	addr      string
	failFirst int
	calls     int
}

func (d *dialer) dial() (io.ReadWriteCloser, error) {
	d.mu.Lock()
	d.calls++
	n := d.calls
	d.mu.Unlock()
	if n <= d.failFirst {
		return nil, errors.New("热层还没回来")
	}
	return dialResp(d.addr, 2*time.Second)
}

// 热层还没回来时，坏掉的那条流**不许再被写**：上一条命令的回复可能还半截在路上，
// 再写上去，两条命令的答复会串在一起 —— 读回一份错的值，比报个错坏得多。
// 等热层回来，下一条命令就该直接通，而不是先白报一次错。
func TestStoreNeverWritesOnDeadConn(t *testing.T) {
	srv := &flakyRedis{dropAfter: 1}
	ln := srv.listen(t)
	addr := ln.Addr().String()

	rw, err := dialResp(addr, 2*time.Second)
	if err != nil {
		t.Fatalf("连不上假热层：%v", err)
	}
	st := newStore(rw, Config{Timeout: 2 * time.Second})
	// 前两次「再拨」都拨不通 —— 模拟热层还没回来。
	st.dial = (&dialer{addr: addr, failFirst: 2}).dial
	t.Cleanup(func() { _ = st.Close() })

	if rep, err := st.do("ECHO", "一"); err != nil || rep != "ok" {
		t.Fatalf("第 1 条该通：err=%v rep=%#v", err, rep)
	}
	if _, err := st.do("ECHO", "二"); err == nil {
		t.Fatalf("被掐线了，这一条必须报错 —— 不许吞")
	}
	if _, err := st.do("ECHO", "三"); err == nil {
		t.Fatalf("换不上新连接时，不该在死流上再写一条还报成功")
	}
	if rep, err := st.do("ECHO", "四"); err != nil || rep != "ok" {
		t.Fatalf("热层回来之后这一条就该直接通：err=%v rep=%#v", err, rep)
	}

	conns := srv.conns()
	if len(conns) != 2 {
		t.Fatalf("该换一条连接（共 2 条），实际 %d 条：%v", len(conns), conns)
	}
	// 关键：死流上的条数停在 2 —— 「三」压根没写出去。
	if len(conns[0]) != 2 {
		t.Fatalf("死流上又被写了东西：%v", conns[0])
	}
	if len(conns[1]) == 0 || conns[1][0] != "ECHO 四" {
		t.Fatalf("新连接上第一条命令是 %v，想要 [ECHO 四]", conns[1])
	}
}
