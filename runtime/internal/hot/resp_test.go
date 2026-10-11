package hot

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------- 连接层：一条连接上「一问一答」必须是原子的 ----------
//
// 真事（2026-10-12，看板）：一屏会并发打好几个接口，而「待批」与「沉淀」两页共用
// 同一个 *hot.Store —— 也就是同一条 TCP 连接。respConn 每次 do 要往 bufio.Writer 里
// 写好几段（`*n` / `$len` / 正文），bufio 的锁只护得住**每一段**，护不住**一段序列**：
// 两个 do 交错写出去，对端读到的是「一条不存在的命令」→ 真 redis 回协议错误并断连，
// 看板从此读不到热层（症状就是 `short write`，且再也自愈不了）。
//
// 所以这一层的规矩是：一条连接上一次只许有一个来回。这条用例盯的就是这个。

// echoRedis 是个照妖镜：只认「*1 + 一个参数」这一种命令，收到就把参数原样回成批量串。
// 读到认不出的字节就把现场留下来 —— 那正是串包。
type echoRedis struct {
	mu     sync.Mutex
	broken string
	served int
}

func (e *echoRedis) note(msg string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.broken == "" {
		e.broken = msg
	}
}

func (e *echoRedis) bump() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.served++
}

func (e *echoRedis) state() (string, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.broken, e.served
}

func (e *echoRedis) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	for {
		head, err := r.ReadString('\n')
		if err != nil {
			return
		}
		if head != "*1\r\n" {
			e.note("命令头不是 *1：" + strconv.Quote(head))
			return
		}
		lenLine, err := r.ReadString('\n')
		if err != nil {
			e.note("缺长度行")
			return
		}
		if !strings.HasPrefix(lenLine, "$") {
			e.note("长度行不是 $ 开头：" + strconv.Quote(lenLine))
			return
		}
		n, err := strconv.Atoi(strings.TrimSuffix(lenLine[1:], "\r\n"))
		if err != nil {
			e.note("长度读不动：" + strconv.Quote(lenLine))
			return
		}
		body := make([]byte, n+2)
		if _, err := io.ReadFull(r, body); err != nil {
			e.note("正文没读全")
			return
		}
		if string(body[n:]) != "\r\n" {
			e.note("正文后面不是 CRLF：" + strconv.Quote(string(body)))
			return
		}
		arg := string(body[:n])
		e.bump()
		_, _ = w.WriteString("$" + strconv.Itoa(len(arg)) + "\r\n" + arg + "\r\n")
		if err := w.Flush(); err != nil {
			return
		}
	}
}

func TestRespDoIsAtomicOnOneConn(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("起假 redis 失败：%v", err)
	}
	defer ln.Close()
	srv := &echoRedis{}
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		srv.serve(c)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("连不上假 redis：%v", err)
	}
	rc := newRespConn(conn, 5*time.Second)

	const workers = 64
	const rounds = 20
	var wg sync.WaitGroup
	bad := make(chan string, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				want := fmt.Sprintf("v-%d-%d", i, r)
				rep, err := rc.do(want)
				if err != nil {
					bad <- fmt.Sprintf("第 %d 轮 do 报错：%v", r, err)
					return
				}
				if got, _ := rep.(string); got != want {
					bad <- fmt.Sprintf("回话串了：要 %q 拿到 %#v", want, rep)
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(bad)

	broken, served := srv.state()
	if broken != "" {
		t.Fatalf("命令流被打断了（对端读到不存在的命令）：%s", broken)
	}
	for msg := range bad {
		t.Fatalf("%s", msg)
	}
	if want := workers * rounds; served != want {
		t.Fatalf("对端只数到 %d 条完整命令，想要 %d 条", served, want)
	}
}
