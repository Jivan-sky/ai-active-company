package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"anc/internal/board"
)

// captureStdout 抓住一次调用的 stdout。
func captureStdout(t *testing.T, f func() int) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	code := f()
	os.Stdout = old
	w.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	return string(b), code
}

// `org export` 的 stdout 必须是**纯 JSON**：提示只能走 stderr，
// 否则消费方（看板 / 管道）会拿到一份坏的 JSON。
func TestOrgExportStdoutIsPureJSON(t *testing.T) {
	out, code := captureStdout(t, func() int { return cmdOrgExport([]string{fixturePath(t, "domains")}) })
	if code != 0 {
		t.Fatalf("退出码 %d，期望 0", code)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("stdout 不是合法 JSON：%v\n%s", err, out)
	}
	if m["schema"] != board.Schema {
		t.Fatalf("schema = %v，期望 %s", m["schema"], board.Schema)
	}
}

// 有非红档发现时，仍要给一份**能解析的** JSON —— 发现走 stderr。
func TestOrgExportWarningsDoNotCorruptJSON(t *testing.T) {
	v := copyFixture(t, "domains")
	// 把 who 改成不存在的岗位：触发一条 warn。
	p := filepath.Join(v, "domains.md")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(string(b), "| manager | projects |", "| 不存在的岗 | projects |", 1)
	if broken == string(b) {
		t.Fatal("没改到 who 单元格，这条测试就白测了")
	}
	if err := os.WriteFile(p, []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	out, code := captureStdout(t, func() int { return cmdOrgExport([]string{v}) })
	if code != 0 {
		t.Fatalf("非红档发现不该改变退出码，实际 %d", code)
	}
	if !json.Valid([]byte(out)) {
		t.Fatalf("stdout 被提示污染了：\n%s", out)
	}
}

// 少给 vault 路径时只打用法，不报 panic。
func TestOrgExportNeedsVaultArg(t *testing.T) {
	_, code := captureStdout(t, func() int { return cmdOrgExport(nil) })
	if code != 2 {
		t.Fatalf("退出码 %d，期望 2", code)
	}
}
