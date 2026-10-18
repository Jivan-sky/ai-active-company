package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decodeAccess(t *testing.T, out string) accessOut {
	t.Helper()
	var got accessOut
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout 不是合法 JSON：%v\n%s", err, out)
	}
	return got
}

// 普通成员：白名单放行、不是管理员、默认跨域可见。
func TestOrgAccessCLIAllowed(t *testing.T) {
	out, code := captureStdout(t, func() int {
		return cmdOrgAccess([]string{fixturePath(t, "domains"), "--open-id", "ou_demo_bob",
			"--domain", "trade", "--json"})
	})
	if code != 0 {
		t.Fatalf("退出码 %d，期望 0", code)
	}
	got := decodeAccess(t, out)
	if !got.Allowed || got.Admin {
		t.Fatalf("bob 应当放行且不是管理员：%+v", got)
	}
	if got.Member != "bob" || got.Role != "ops" {
		t.Fatalf("解出的人不对：%+v", got)
	}
	if !got.AllDomains || got.CrossDomain != "visible" {
		t.Fatalf("没写 access.cross_domain 时应当跨域可见：%+v", got)
	}
	if got.CanSee == nil || !*got.CanSee {
		t.Fatalf("默认跨域可见时 bob 应当能看 trade：%+v", got)
	}
}

// 对不上 open_id：拒绝，且说清理由；退出码仍是 0（判定不是命令失败）。
func TestOrgAccessCLIDenied(t *testing.T) {
	out, code := captureStdout(t, func() int {
		return cmdOrgAccess([]string{fixturePath(t, "domains"), "--open-id", "ou_nobody", "--json"})
	})
	if code != 0 {
		t.Fatalf("退出码 %d，期望 0", code)
	}
	got := decodeAccess(t, out)
	if got.Allowed {
		t.Fatalf("没人认领的 open_id 不该放行：%+v", got)
	}
	if got.Why == "" {
		t.Fatal("拒绝必须写清为什么")
	}
}

// 客户把跨域关掉后：只看自己域 + 公司级资料。
func TestOrgAccessCLIHiddenDomain(t *testing.T) {
	vault := filepath.Join(t.TempDir(), "domains")
	if err := os.CopyFS(vault, os.DirFS(fixturePath(t, "domains"))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(vault, "company", "company.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	i := strings.Index(text, "\n---")
	if i < 0 {
		t.Fatal("company.md 里找不到 frontmatter 结束符")
	}
	text = text[:i] + "\naccess:\n  cross_domain: hidden" + text[i:]
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}

	out, _ := captureStdout(t, func() int {
		return cmdOrgAccess([]string{vault, "--open-id", "ou_demo_bob", "--domain", "trade", "--json"})
	})
	got := decodeAccess(t, out)
	if got.CrossDomain != "hidden" || got.AllDomains {
		t.Fatalf("hidden 时不该跨域可见：%+v", got)
	}
	if got.CanSee == nil || *got.CanSee {
		t.Fatalf("hidden 时 bob 不该能看 trade：%+v", got)
	}
}

func TestOrgAccessCLINeedsOpenID(t *testing.T) {
	_, code := captureStdout(t, func() int {
		return cmdOrgAccess([]string{fixturePath(t, "domains"), "--json"})
	})
	if code != 2 {
		t.Fatalf("缺 --open-id 应当返回 2，实际 %d", code)
	}
}
