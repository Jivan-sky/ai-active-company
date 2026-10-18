package main

import (
	"path/filepath"
	"testing"
)

// `anc board serve` 的用法与前置检查（不起服务：起服务要靠人开浏览器看，不适合放进测试）。
//
// 口径：路径写错就别起服务 —— 起一个只会 500 的服务是骗人；
// 而**内容**有问题照起，看板正是用来看「哪里坏了」的。
func TestBoardServeArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"没给 vault", []string{}, 2},
		{"位置参数多了", []string{"a", "b"}, 2},
		{"addr 缺值", []string{"--addr"}, 2},
		{"vault 不存在", []string{filepath.Join(t.TempDir(), "没有这个目录")}, 1},
		// vault 是好的，只有 --ui-dir 写错：照样别起服务（起一个打开只会 404 前端的服务是骗人）。
		{"ui-dir 不是目录", []string{
			filepath.Join("testdata", "orgs", "domains"),
			"--ui-dir", filepath.Join(t.TempDir(), "没有这个目录"),
		}, 1},
		// 非回环地址不给 --insecure-bind：先 bind 到临时端口，再拒绝启动（端口随即关掉）。
		{"非回环没给 insecure-bind", []string{
			filepath.Join("testdata", "orgs", "domains"),
			"--addr", "0.0.0.0:0",
		}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if code := quiet(t, func() int { return cmdBoardServe(c.args) }); code != c.want {
				t.Fatalf("退出码 %d，期望 %d", code, c.want)
			}
		})
	}
}
