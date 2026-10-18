package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"anc/internal/org"
)

// lintLayer 只负责发现事实，档次由规则表定。这条测试锁的就是这个分工：
// 实现里一旦再硬编码档次，这里就会红。
func TestLintLevelsComeFromPolicy(t *testing.T) {
	pol := org.DefaultPolicy()
	cases := []struct {
		name  string
		text  string
		rule  string
		fatal bool
	}{
		{"三引号会破 TOML", "正常一行\n这行有 ''' 三引号", "persona.toml.quote_break", true},
		{"美元大括号会被替换", "含 ${ENV} 的行", "persona.env.substitution", true},
		{"双花括号槽位", "含 {{slot}} 的行", "persona.slot.unreplaced", false},
		{"绝对路径", "见 /Users/dev/vault/x.md", "persona.path.absolute", false},
		{"冻结事实", "当前仅有 3 个客户", "persona.fact.frozen", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := lintLayer(pol, "x.md", c.text)
			var hit *org.Issue
			for i := range got {
				if got[i].Rule == c.rule {
					hit = &got[i]
				}
			}
			if hit == nil {
				t.Fatalf("没发现 %s，实际 %v", c.rule, org.IssueStrings(got))
			}
			want := org.LevelWarn
			if c.fatal {
				want = org.LevelFatal
			}
			if hit.Level != want {
				t.Fatalf("%s 的档次是 %s，期望 %s", c.rule, hit.Level, want)
			}
		})
	}

	if got := lintLayer(pol, "x.md", "一句干净的话。"); len(got) != 0 {
		t.Fatalf("干净文本不该有发现，实际 %v", org.IssueStrings(got))
	}
}

// 降级即可放行：红线之外，规则表说 off 就 off。
func TestPolicyCanSilenceWarnLevel(t *testing.T) {
	pol := org.DefaultPolicy()
	// DefaultPolicy 是出厂表；用一个临时副本改档，别污染别的用例。
	v := copyFixture(t, "broken-section")
	addOrgPolicy(t, v, "role.persona.section_unknown: off")
	if _, err := org.Load(v); err != nil {
		t.Fatalf("降级后应当通过：%v", err)
	}
	if pol.Level("persona.path.absolute") != org.LevelWarn {
		t.Fatal("出厂表被动过了")
	}
}

// 会破坏 TOML 的 persona 绝不允许产出配置 —— 这条是 SPEC §171 的生产事故防线。
func TestFatalLintBlocksRender(t *testing.T) {
	v := copyFixture(t, "one")
	path := filepath.Join(v, "members", "alice", "persona.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, []byte("\n- 这行有 ''' 三引号\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	o, err := org.Load(v)
	if err != nil {
		t.Fatalf("校验期不该拦这个（它是 persona 文本问题）：%v", err)
	}
	if _, err := Build(o, Options{Host: org.Host{VaultRoot: "/vault", HomesRoot: "/homes", DataDir: "/data"}, Version: "test", Now: fixedNow}); err == nil {
		t.Fatal("含三引号的 persona 应当拒绝渲染")
	} else if !strings.Contains(err.Error(), "persona.toml.quote_break") {
		t.Fatalf("拒绝理由应当带上规则 id，实际 %v", err)
	}
}

// copyFixture 把 testdata/orgs 下的 vault 拷进临时目录。
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("..", "..", "testdata", "orgs", name))
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), name)
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	return dst
}

// addOrgPolicy 往 company.md 的 frontmatter 末尾插 policy 段。
func addOrgPolicy(t *testing.T, vault string, lines ...string) {
	t.Helper()
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
	head := text[:i] + "\npolicy:"
	for _, l := range lines {
		head += "\n  " + l
	}
	if err := os.WriteFile(path, []byte(head+text[i:]), 0o600); err != nil {
		t.Fatal(err)
	}
}

// 段 8「业务域」：自己的域给整行（含 data），别人的域只给目录（它的 terms 不外泄）。
func TestDomainSegmentScoping(t *testing.T) {
	o, host := fixtureOrg(t, "domains")
	alice, ok := o.Member("alice")
	if !ok {
		t.Fatal("取不到 alice")
	}
	res, err := Persona(o, o.Roles[alice.Role], alice, host)
	if err != nil {
		t.Fatal(err)
	}
	txt := res.Text
	if !strings.Contains(txt, "## 8 业务域") {
		t.Fatalf("没渲染段 8：\n%s", txt)
	}
	// 自己的域：整行，data 在
	if !strings.Contains(txt, "| trade | 大宗贸易 |") || !strings.Contains(txt, "| projects |") {
		t.Fatalf("自己的域没给全列：\n%s", txt)
	}
	// who 填的是岗位，渲染时才解成人名 —— 名单只有一份，不会漂移
	if !strings.Contains(txt, "Alice Wang（经理）") {
		t.Fatalf("who 没派生成「人名（岗位）」：\n%s", txt)
	}
	// 别人的域：只在目录里，且它的术语不许进 alice 的上下文
	if !strings.Contains(txt, "| logistics | 物流 |") {
		t.Fatalf("全公司域目录里没有 logistics：\n%s", txt)
	}
	if strings.Contains(txt, "ETA") {
		t.Fatalf("别人域的术语进了 alice 的上下文：\n%s", txt)
	}
}

// 没有 domains.md 就不出段 8：存量 vault 的 persona 逐字不变。
func TestNoDomainSegmentWithoutTable(t *testing.T) {
	o, host := fixtureOrg(t, "one")
	alice, ok := o.Member("alice")
	if !ok {
		t.Fatal("取不到 alice")
	}
	res, err := Persona(o, o.Roles[alice.Role], alice, host)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Text, "业务域") {
		t.Fatalf("没有 domains.md 不该出段 8：\n%s", res.Text)
	}
}

// 域表里的字也会进 persona —— 三引号红线不因字来自表格而豁免（SPEC §171 事故防线）。
func TestDomainCellLintBlocksRender(t *testing.T) {
	v := copyFixture(t, "domains")
	path := filepath.Join(v, "domains.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(string(b), "进出口合同的签订", "进出口合同的'''签订", 1)
	if broken == string(b) {
		t.Fatal("没改到 what 单元格，这条测试就白测了")
	}
	if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	o, err := org.Load(v)
	if err != nil {
		t.Fatalf("校验期不该拦这个（它是 persona 文本问题）：%v", err)
	}
	if _, err := Build(o, Options{Host: org.Host{VaultRoot: "/vault", HomesRoot: "/homes", DataDir: "/data"}, Version: "test", Now: fixedNow}); err == nil {
		t.Fatal("域表里的三引号应当拒绝渲染")
	} else if !strings.Contains(err.Error(), "persona.toml.quote_break") {
		t.Fatalf("拒绝理由应当带上规则 id，实际 %v", err)
	}
}
