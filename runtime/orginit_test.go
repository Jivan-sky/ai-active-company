package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"anc/internal/org"
)

// 生成的骨架要自带两条性质：
//  1. 除了「故意留白」的那条红档，骨架自身零问题 —— 模板 bug 必须在这里就炸，不能等驻场那天；
//  2. 它**不能**被直接当成成品上线：allowed_tools 空着就拦住渲染
//     （实测：dontAsk 下空白名单 = 工具全被自动拒绝，bot 连得上却干不了活）。
func TestOrgInitProducesUsableVault(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "vault")
	if code := quiet(t, func() int {
		return cmdOrgInit([]string{dir, "--client", "某车队", "--id", "demo", "--members", "alice,bob"})
	}); code != 0 {
		t.Fatalf("生成退出码 %d", code)
	}

	for _, p := range []string{
		"company/company.md",
		"domains.md",
		"projects.md",
		"charters/CLAUDE.md",
		"context/CLAUDE.md",
		"roles/manager/persona.md",
		"roles/devbot/persona.md",
		"members/alice/persona.md",
		"members/bob/persona.md",
		"members/devbot/persona.md",
		"10-knowledge/CLAUDE.md",
		"20-ops/CLAUDE.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); err != nil {
			t.Fatalf("缺文件 %s：%v", p, err)
		}
	}

	_, loadErr := org.Load(dir)
	le, ok := loadErr.(*org.LoadError)
	if !ok {
		t.Fatalf("骨架应当只因「故意留白」被拦，实际 loadErr=%v", loadErr)
	}
	var fatal []string
	for _, i := range le.Report.Fatal() {
		fatal = append(fatal, i.Rule)
	}
	if len(fatal) != 1 || fatal[0] != "role.allowed_tools.empty" {
		t.Fatalf("骨架的红档应当只有「故意留白」那一条，实际 %v —— 多出来的就是模板 bug", fatal)
	}
	// 这条红档得自己说清「去哪儿改」—— 只说「没配好」不顶用。
	if w := le.Report.Fatal()[0].Where; w != "roles/manager/persona.md" {
		t.Fatalf("红档应当指向要改的角色文件，实际 %q", w)
	}

	// 生成完只是一半：空白名单必须拦住渲染，逼人把角色配成真能干活的样子。
	cfg := filepath.Join(t.TempDir(), "gateway", "config.toml")
	if code := quiet(t, func() int {
		return cmdRender([]string{dir, "--config", cfg, "--apply"})
	}); code != 1 {
		t.Fatalf("空白名单的骨架应当被拒绝渲染（退出码 1），实际 %d", code)
	}
	if _, err := os.Stat(cfg); !os.IsNotExist(err) {
		t.Fatal("被拦的那次不该落盘")
	}

	// 填上白名单就应当放行 —— 这道门得是可解的，不能是个死结。
	role := filepath.Join(dir, "roles", "manager", "persona.md")
	b, err := os.ReadFile(role)
	if err != nil {
		t.Fatal(err)
	}
	fixed := strings.Replace(string(b), "allowed_tools: []", "allowed_tools: [Read, Grep, Glob]", 1)
	if fixed == string(b) {
		t.Fatal("模板里没找到 allowed_tools: [] —— 这条断言跟着模板一起漂了")
	}
	if err := os.WriteFile(role, []byte(fixed), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := quiet(t, func() int {
		return cmdRender([]string{dir, "--config", cfg, "--apply"})
	}); code != 0 {
		t.Fatalf("填完白名单应当能渲染，退出码 %d", code)
	}
}

func TestOrgInitRefusesBadInput(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"id 不是 ascii", []string{"--client", "某车队", "--id", "Demo Co"}, 2},
		{"id 大写", []string{"--client", "某车队", "--id", "Demo"}, 2},
		{"缺 id", []string{"--client", "某车队"}, 2},
		{"devbot 不该手写", []string{"--client", "某车队", "--id", "demo", "--members", "alice,devbot"}, 2},
		{"admin 不在名单里", []string{"--client", "某车队", "--id", "demo", "--admin", "carol"}, 2},
		{"日期格式不对", []string{"--client", "某车队", "--id", "demo", "--date", "2026/10/06"}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args := append([]string{filepath.Join(t.TempDir(), "v")}, c.args...)
			if code := quiet(t, func() int { return cmdOrgInit(args) }); code != c.want {
				t.Fatalf("退出码 %d，期望 %d", code, c.want)
			}
		})
	}
}

func TestOrgInitProtectsNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "已有文件.txt"), []byte("别踩我"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := []string{dir, "--client", "某车队", "--id", "demo"}
	if code := quiet(t, func() int { return cmdOrgInit(base) }); code != 1 {
		t.Fatalf("非空目录应当拒绝，退出码 %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "company", "company.md")); !os.IsNotExist(err) {
		t.Fatal("拒绝时不该写入任何东西")
	}
	if code := quiet(t, func() int { return cmdOrgInit(append(base, "--force")) }); code != 0 {
		t.Fatal("--force 后应当生成")
	}
}

func TestSplitListAndDisplayName(t *testing.T) {
	if got := splitList(" a, b ,,c "); len(got) != 3 || got[2] != "c" {
		t.Fatalf("切分结果不对：%v", got)
	}
	if got := displayName("alice"); got != "Alice" {
		t.Fatalf("displayName(alice) = %q", got)
	}
	if got := displayName("张伟"); got != "张伟" {
		t.Fatalf("中文名不该被动：%q", got)
	}
}
