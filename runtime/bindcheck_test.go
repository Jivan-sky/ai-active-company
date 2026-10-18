package main

import (
	"net"
	"strings"
	"testing"
)

type fakeAddr struct{}

func (fakeAddr) Network() string { return "unix" }
func (fakeAddr) String() string  { return "/tmp/anc.sock" }

// 绑定边界：回环放行、非回环默认拒绝、显式 --insecure-bind 才放行并留一条警告。
// 非 TCP 地址拿不到 IP，不在这一层猜。
func TestCheckBindAddr(t *testing.T) {
	cases := []struct {
		name     string
		addr     net.Addr
		insecure bool
		allow    bool
		contains string
	}{
		{"回环放行", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8787}, false, true, ""},
		{"IPv6 回环放行", &net.TCPAddr{IP: net.ParseIP("::1"), Port: 8787}, false, true, ""},
		{"通配地址拒绝", &net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 8787}, false, false, "--insecure-bind"},
		{"非回环单播拒绝", &net.TCPAddr{IP: net.ParseIP("203.0.113.10"), Port: 8787}, false, false, "--insecure-bind"},
		{"显式放开", &net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 8787}, true, true, "--insecure-bind"},
		{"非 TCP 地址不猜", fakeAddr{}, false, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			allow, reason := checkBindAddr(c.addr, c.insecure)
			if allow != c.allow {
				t.Fatalf("allow=%v，期望 %v（reason=%q）", allow, c.allow, reason)
			}
			if c.contains != "" && !strings.Contains(reason, c.contains) {
				t.Fatalf("reason 里该有 %q：%q", c.contains, reason)
			}
		})
	}
}
