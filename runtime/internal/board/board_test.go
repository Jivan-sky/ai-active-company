package board

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"anc/internal/org"
)

var fixedNow = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

func boardOrg(t *testing.T, name string) *org.Org {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", "testdata", "orgs", name))
	if err != nil {
		t.Fatal(err)
	}
	o, err := org.Load(abs)
	if err != nil {
		t.Fatalf("加载 %s: %v", name, err)
	}
	return o
}

// 纯函数：同一份真相源 + 同一个时刻 → 逐字节相同。前端据此可以按字节 diff / 缓存。
func TestOfIsDeterministic(t *testing.T) {
	o := boardOrg(t, "domains")
	a, err := Of(o, fixedNow).JSON()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Of(o, fixedNow).JSON()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("两次投影不一致：\n%s\n---\n%s", a, b)
	}
	if !strings.Contains(a, `"schema": "`+Schema+`"`) {
		t.Fatalf("投影里没有 schema 版本号：\n%s", a)
	}
}

// **这条是硬约束**：看板是观测面，不是凭据面。飞书标识符一个都不许漏进投影。
func TestViewLeaksNoCredentials(t *testing.T) {
	s, err := Of(boardOrg(t, "domains"), fixedNow).JSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		"ou_demo_alice", "cli_demo_alice", "ou_demo_bob", "cli_demo_dev",
		"open_id", "app_id", "extra_allow_from", "allow_chat",
	} {
		if strings.Contains(s, bad) {
			t.Fatalf("投影里出现了 %q —— 看板不该拿到凭据面字段：\n%s", bad, s)
		}
	}
}

// who 存的是岗位，投影里派生成人名 —— 与 persona 用的是同一处口径（org.WhoLabel）。
func TestViewDerivesWhoLabel(t *testing.T) {
	v := Of(boardOrg(t, "domains"), fixedNow)
	if len(v.Domains) != 2 {
		t.Fatalf("期望 2 个域，实际 %d", len(v.Domains))
	}
	if v.Domains[0].Who != "manager" || v.Domains[0].WhoLabel != "Alice Wang（经理）" {
		t.Fatalf("who / who_label 不对：%+v", v.Domains[0])
	}
}

// 成员的 model 是三级回退之后的生效值，不是原始字段（看板要能直接显示「他跑哪个模型」）。
func TestMemberModelIsResolved(t *testing.T) {
	v := Of(boardOrg(t, "domains"), fixedNow)
	for _, m := range v.Members {
		if m.Model == "" {
			t.Fatalf("成员 %s 的 model 没回退到 company.defaults：%+v", m.Name, m)
		}
	}
}

// 项目表进投影：owner 岗位派生成人名（与 persona / 域表同一处口径），period 原文照搬 ——
// ANC 不解读周期，看板也不该解读。
func TestViewProjectsProjected(t *testing.T) {
	v := Of(boardOrg(t, "domains"), fixedNow)
	if len(v.Projects) != 1 {
		t.Fatalf("期望 1 个项目，实际 %d：%+v", len(v.Projects), v.Projects)
	}
	pr := v.Projects[0]
	if pr.Slug != "trade-q3" || pr.Name != "Q3 结算改造" || pr.Domain != "trade" {
		t.Fatalf("项目字段投影不对：%+v", pr)
	}
	if pr.Owner != "manager" || pr.OwnerLabel != "Alice Wang（经理）" {
		t.Fatalf("owner / owner_label 不对：%+v", pr)
	}
	if pr.Period != "2026-07-01 → 2026-09-30" {
		t.Fatalf("period 被改动了：%q", pr.Period)
	}
	// 立项书副本的落点：看板要能显示「这个项目的材料进来了没」。
	if pr.Charter != "charters/trade-q3" {
		t.Fatalf("charter（副本落点）投影不对：%q", pr.Charter)
	}
}

// nil 切片一律归一成 []：JSON 里 null 会让前端多一层判空。
func TestEmptySlicesAreArraysNotNull(t *testing.T) {
	s, err := Of(boardOrg(t, "one"), fixedNow).JSON()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s, "null") {
		t.Fatalf("投影里出现了 null：\n%s", s)
	}
	// one 没有 domains.md，也没有任何成员划域 —— 两处都该是空数组。
	if !strings.Contains(s, `"domains": []`) {
		t.Fatalf("空域表没渲染成 []：\n%s", s)
	}
	if !strings.Contains(s, `"projects": []`) {
		t.Fatalf("空项目表没渲染成 []：\n%s", s)
	}
}

// boardOrgWithGrant 把 fixture 拷进临时目录、落一条 grant，再加载。
// 直接改 testdata 会把「有没有授权」烤进那份共享样本，别的用例会跟着变。
func boardOrgWithGrant(t *testing.T, name string) *org.Org {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "orgs", name)
	dst := filepath.Join(t.TempDir(), name)
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(dst, "grants", "pilot")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	text := "---\ngrant:\n  from: member:alice\n  to: role:ops\n  action: read\n" +
		"  object: shipments\n  ttl: 30d\n  reason: 看板要看得见这条\n---\n\n正文。\n"
	if err := os.WriteFile(filepath.Join(dir, "read-shipments.md"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	o, err := org.Load(dst)
	if err != nil {
		t.Fatalf("加载带授权的 %s: %v", name, err)
	}
	return o
}

// 看板投影是**全量**的：授权表原样进来（bot 侧那个「只出与我相关的几条」是另一个视图）。
func TestViewProjectsGrants(t *testing.T) {
	v := Of(boardOrgWithGrant(t, "domains"), fixedNow)
	if len(v.Grants) != 1 {
		t.Fatalf("授权没进投影：%+v", v.Grants)
	}
	g := v.Grants[0]
	if g.Slug != "read-shipments" || g.Path != "grants/pilot/read-shipments.md" ||
		g.From != "member:alice" || g.To != "role:ops" || g.Action != "read" ||
		g.Object != "shipments" || g.TTL != "30d" || g.Reason != "看板要看得见这条" {
		t.Fatalf("字段没搬全：%+v", g)
	}
}

// 字段恒在、不 omitempty：零条授权也要出一个 `[]`，前端少一个判空。
func TestViewGrantsAlwaysArray(t *testing.T) {
	v := Of(boardOrg(t, "domains"), fixedNow)
	if v.Grants == nil {
		t.Fatal("零条授权时 Grants 是 nil —— 恒在的契约要求空数组")
	}
	s, err := v.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `"grants": []`) {
		t.Fatalf("零条授权时投影里应当是 `\"grants\": []`：\n%s", s)
	}
}
