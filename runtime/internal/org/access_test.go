package org

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// addCompanyLines 往 company.md 的 frontmatter 末尾插几行原始 YAML。
// 和 addPolicy 分开：那个往 policy 段里塞规则，这个只用于顶层键（access.*）。
func addCompanyLines(t *testing.T, vault string, lines ...string) {
	t.Helper()
	path := filepath.Join(vault, "company", "company.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	i := strings.Index(text, "\n---")
	if i < 0 {
		t.Fatalf("%s 里找不到 frontmatter 结束符", path)
	}
	head := text[:i]
	for _, l := range lines {
		head += "\n" + l
	}
	if err := os.WriteFile(path, []byte(head+text[i:]), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveViewerRejectsUnknownOpenID(t *testing.T) {
	o, err := Load(fixture(t, "domains"))
	if err != nil {
		t.Fatal(err)
	}
	// 默认拒绝（2026-10-19 拍板）：白名单对不上就进不来，没有游客档。
	for _, id := range []string{"", "   ", "ou_nobody"} {
		if v, ok := o.ResolveViewer(id); ok {
			t.Fatalf("open_id=%q 不该被放行，实际解出 %+v", id, v)
		}
	}
}

func TestResolveViewerRejectsDisabled(t *testing.T) {
	o, err := Load(fixture(t, "disabled"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := o.ResolveViewer("ou_demo_frank"); ok {
		t.Fatal("已停用的成员不该能登录")
	}
}

func TestResolveViewerAdminFromCompany(t *testing.T) {
	o, err := Load(fixture(t, "domains"))
	if err != nil {
		t.Fatal(err)
	}
	v, ok := o.ResolveViewer("ou_demo_alice")
	if !ok {
		t.Fatal("alice 应当在白名单里")
	}
	if !v.Admin {
		t.Fatal("company.admins 里的成员应当是管理员")
	}
	if !v.AllDomains || !v.CanSeeDomain("logistics") {
		t.Fatal("管理员应当看见所有域")
	}
}

// 编号入口与 open_id 入口必须给出同一份判定 —— 检索 / CLI 用编号，登录用 open_id。
func TestResolveMemberViewerMatchesOpenIDViewer(t *testing.T) {
	o, err := Load(fixture(t, "domains"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alice", "bob"} {
		m, ok := o.Member(name)
		if !ok {
			t.Fatalf("夹具里没有 %s", name)
		}
		byID, ok := o.ResolveViewer(m.Feishu.OpenID)
		if !ok {
			t.Fatalf("%s 的 open_id 解不出来", name)
		}
		byName, ok := o.ResolveMemberViewer(name)
		if !ok {
			t.Fatalf("%s 的编号解不出来", name)
		}
		if byID.Admin != byName.Admin || byID.AllDomains != byName.AllDomains {
			t.Fatalf("%s 两处判定不一致：open_id=%+v 编号=%+v", name, byID, byName)
		}
		if strings.Join(byID.Domains, ",") != strings.Join(byName.Domains, ",") {
			t.Fatalf("%s 的域集合不一致：%v vs %v", name, byID.Domains, byName.Domains)
		}
	}
	if _, ok := o.ResolveMemberViewer("ghost"); ok {
		t.Fatal("不存在的编号不该解出访问者")
	}
}

func TestResolveViewerCrossDomainDefaultVisible(t *testing.T) {
	o, err := Load(fixture(t, "domains"))
	if err != nil {
		t.Fatal(err)
	}
	v, ok := o.ResolveViewer("ou_demo_bob")
	if !ok {
		t.Fatal("bob 应当在白名单里")
	}
	if v.Admin {
		t.Fatal("bob 不是管理员")
	}
	if !v.CanSeeDomain("trade") {
		t.Fatal("没写 access.cross_domain 时，跨域默认可见")
	}
}

func TestResolveViewerCrossDomainHidden(t *testing.T) {
	vault := copyVault(t, "domains")
	addCompanyLines(t, vault, "access:", "  cross_domain: hidden")
	o, err := Load(vault)
	if err != nil {
		t.Fatal(err)
	}

	bob, ok := o.ResolveViewer("ou_demo_bob")
	if !ok {
		t.Fatal("bob 应当在白名单里")
	}
	if bob.AllDomains {
		t.Fatal("hidden 时普通成员不该看见所有域")
	}
	if !bob.CanSeeDomain("logistics") {
		t.Fatal("bob 应当能看自己所属的 logistics")
	}
	if bob.CanSeeDomain("trade") {
		t.Fatal("hidden 时 bob 不该能看 trade")
	}
	if !bob.CanSeeDomain("") {
		t.Fatal("不属于任何域的公司级资料应当可见")
	}

	alice, ok := o.ResolveViewer("ou_demo_alice")
	if !ok {
		t.Fatal("alice 应当在白名单里")
	}
	if !alice.CanSeeDomain("trade") || !alice.CanSeeDomain("logistics") {
		t.Fatal("管理员不受跨域开关限制")
	}
}

// 「随时把某人提成最高权限」的人身写法：只改 persona.md 的 admin 位。
func TestResolveViewerAdminFromMemberFlag(t *testing.T) {
	vault := copyVault(t, "domains")
	addCompanyLines(t, vault, "access:", "  cross_domain: hidden")
	path := filepath.Join(vault, "members", "bob", "persona.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(b), "admin: false", "admin: true", 1)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}

	o, err := Load(vault)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := o.ResolveViewer("ou_demo_bob")
	if !ok {
		t.Fatal("bob 应当在白名单里")
	}
	if !v.Admin || !v.AllDomains {
		t.Fatal("admin: true 的成员应当直接拿到最高权限")
	}
}

func TestCrossDomainInvalidValueIsFatal(t *testing.T) {
	vault := copyVault(t, "domains")
	addCompanyLines(t, vault, "access:", "  cross_domain: friends-only")
	_, err := Load(vault)
	if err == nil {
		t.Fatal("写错 access.cross_domain 应当拦住，不能静默按默认值走")
	}
	le, ok := err.(*LoadError)
	if !ok {
		t.Fatalf("应当是 LoadError，实际 %T: %v", err, err)
	}
	ids := ruleIDs(le.Report.Fatal())
	for _, id := range ids {
		if id == "company.access.cross_domain.invalid" {
			return
		}
	}
	t.Fatalf("应当报 company.access.cross_domain.invalid，实际 %v", ids)
}
