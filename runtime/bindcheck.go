package main

// 监听地址的对外边界。
//
// SPEC §5 的口径：默认只绑回环；要对外必须显式打开并带鉴权。
// 鉴权还没接上时，「显式打开」就是 --insecure-bind —— 人不写出来，就拒绝启动，
// 不给「先跑起来再说」的机会。这不是多一道审批，是把「手滑开 0.0.0.0」变成不可能。

import (
	"fmt"
	"net"
)

// checkBindAddr 判断一个已监听的地址能不能继续对外服务。
//
// allowed=false 时 reason 是拒绝原因；allowed=true 时 reason 可能是警告（显式放开那条路）。
// 非 TCP 地址不猜：拿不到 IP 就不在这里拦。
func checkBindAddr(addr net.Addr, insecure bool) (allowed bool, reason string) {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok {
		return true, ""
	}
	if tcp.IP.IsLoopback() {
		return true, ""
	}
	if insecure {
		return true, fmt.Sprintf("绑到了 %s：--insecure-bind 已开，组织与项目信息会暴露在网络上。", addr)
	}
	return false, fmt.Sprintf("绑到了 %s（非回环）：默认不对外。\n    要么改用 ssh 端口转发（推荐），要么显式加 --insecure-bind 自行承担暴露风险。", addr)
}
