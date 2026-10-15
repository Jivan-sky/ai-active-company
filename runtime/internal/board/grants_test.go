package board

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 授权表那十条规则的 id 一律以 grant. 开头（rules.go 的出厂表）—— 这条用例钉住它，
// 免得以后有人给授权表加一条不按前缀起名的规则，页面上就静默少一条发现。
const grantRulePrefix = "grant."

// grantText 造一个授权文件。字段空着就整行不写 —— 缺字段的发现由 org 那侧发，这里只造现场。
func grantText(from, to, action, object, ttl, reason string) string {
	var b strings.Builder
	b.WriteString("---\ngrant:\n")
	for _, kv := range [][2]string{
		{"from", from}, {"to", to}, {"action", action},
		{"object", object}, {"ttl", ttl}, {"reason", reason},
	} {
		if kv[1] != "" {
			b.WriteString("  " + kv[0] + ": " + kv[1] + "\n")
		}
	}
	b.WriteString("---\n\n正文随便写。\n")
	return b.String()
}

// grantsFixture 造一份「真相源 + 业务 agent 表 + 授权表」的现场。
//
// 为什么要业务 agent：授权表的主体有两类（成员 / 业务 agent），只验一类等于没验。
// 表只有 agents.md 一处，用例里也只在那一处写。
func grantsFixture(t *testing.T) string {
	t.Helper()
	vault := t.TempDir()
	if err := os.CopyFS(vault, os.DirFS(fixturePath(t, "domains"))); err != nil {
		t.Fatal(err)
	}
	writeInto(t, vault, "agents.md",
		"| slug | name | domain | harness | tools | model | role | app_id |\n"+
			"|---|---|---|---|---|---|---|---|\n"+
			"| order-bot | 订单处理 | trade | codex | read,bash | claude-sonnet-5 | manager | cli_demo_order |\n")
	writeInto(t, vault, "grants/alice-to-bob.md",
		grantText("member:alice", "member:bob", "read", "domain:trade", "30d", "对读数"))
	writeInto(t, vault, "grants/manager-to-bot.md",
		grantText("role:manager", "agent:order-bot", "invoke", "orders", "", "让业务 agent 接单"))
	writeInto(t, vault, "grants/bare-name.md",
		grantText("bob", "alice", "write", "shipments", "", "裸名字也认"))
	return vault
}

func writeInto(t *testing.T, vault, rel, content string) {
	t.Helper()
	p := filepath.Join(vault, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func grantsOf(t *testing.T, s *Server) (GrantsView, string) {
	t.Helper()
	return grantsAt(t, s, "/api/grants")
}

func grantsAt(t *testing.T, s *Server, path string) (GrantsView, string) {
	t.Helper()
	rec := get(t, s.Handler(), path)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 %d，期望 200：%s", rec.Code, rec.Body.String())
	}
	var v GrantsView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("不是合法 JSON：%v", err)
	}
	if v.Schema != GrantsSchema {
		t.Fatalf("schema=%q，期望 %s", v.Schema, GrantsSchema)
	}
	return v, rec.Body.String()
}

// 表里有什么就画什么：字段原样搬（不转述），顺序按路径定（两次刷新逐字节相同）。
func TestGrantsDrawsTheWholeTable(t *testing.T) {
	v, _ := grantsOf(t, &Server{Vault: grantsFixture(t)})
	if !v.Wired {
		t.Fatalf("真相源读得动却报 wired=false：%s", v.Error)
	}
	if v.Total != 3 || v.Shown != 3 {
		t.Fatalf("期望 3 条全露出来，实际 total=%d shown=%d", v.Total, v.Shown)
	}
	var got []string
	for _, r := range v.Rows {
		got = append(got, r.Path)
	}
	want := []string{"grants/alice-to-bob.md", "grants/bare-name.md", "grants/manager-to-bot.md"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("顺序或路径不对：%v，期望 %v", got, want)
	}
	// 原样搬：一个字都不许改（这一页是给人核对的，转述一遍 = 多一处会漂的副本）。
	row := v.Rows[0]
	if row.From != "member:alice" || row.To != "member:bob" || row.Action != "read" ||
		row.Object != "domain:trade" || row.TTL != "30d" || row.Reason != "对读数" ||
		row.Slug != "alice-to-bob" {
		t.Fatalf("字段没原样搬：%+v", row)
	}
}

// 引用要解成「是谁」：成员、业务 agent、岗位各有各的解 —— 解不出来的照原样留着并说清。
func TestGrantsResolvesWhoInEverySlot(t *testing.T) {
	v, _ := grantsOf(t, &Server{Vault: grantsFixture(t)})
	bySlug := map[string]GrantsRow{}
	for _, r := range v.Rows {
		bySlug[r.Slug] = r
	}
	// 成员 → 成员：两边都解得出来，且是 member。
	a := bySlug["alice-to-bob"]
	if len(a.Who) != 2 {
		t.Fatalf("期望 2 个引用（from / to），实际 %d：%+v", len(a.Who), a.Who)
	}
	if a.Who[0].Kind != "member" || a.Who[0].Identity == nil || a.Who[0].Identity.Ref != "alice" {
		t.Fatalf("from=member:alice 该解成 alice 这个成员：%+v", a.Who[0])
	}
	if a.Who[1].Identity == nil || a.Who[1].Identity.Ref != "bob" {
		t.Fatalf("to=member:bob 该解成 bob：%+v", a.Who[1])
	}
	// 岗位 → 业务 agent：岗位**解得出岗位**（它能授权，但不是「一个人」）。
	m := bySlug["manager-to-bot"]
	if len(m.Who) != 2 {
		t.Fatalf("期望 2 个引用，实际 %d：%+v", len(m.Who), m.Who)
	}
	if m.Who[0].Slot != "from" || m.Who[0].Kind != "role" || m.Who[0].Name != "manager" {
		t.Fatalf("岗位该解成 role，实际 %+v", m.Who[0])
	}
	if m.Who[0].Identity != nil {
		t.Fatalf("岗位不是主体（成员 / 业务 agent 才是），不该给 Identity：%+v", m.Who[0])
	}
	if m.Who[1].Kind != "agent" || m.Who[1].Identity == nil {
		t.Fatalf("业务 agent 该解出主体：%+v", m.Who[1])
	}
	if m.Who[1].Identity.Domain != "trade" {
		t.Fatalf("业务 agent 的归属该带出来（domain=trade），实际 %+v", m.Who[1].Identity)
	}
	// 裸名字：按「先成员、后业务 agent」解（顺序写死，同一份输入永远同一个答案）。
	b := bySlug["bare-name"]
	if b.Who[0].Kind != "member" || b.Who[0].Name == "" {
		t.Fatalf("裸名字该解成成员：%+v", b.Who[0])
	}
}

// 视角只筛、不改写：命中它的行露出来，其余行不露 —— 但 Total 仍是全表的数
// （「它名下只有这些」与「表里一共多少」是两件事，页面要能分开说）。
func TestGrantsScopeFiltersButKeepsTheTotal(t *testing.T) {
	vault := grantsFixture(t)
	s := &Server{Vault: vault}

	all, _ := grantsOf(t, s)
	if all.Shown != 3 || all.Total != 3 {
		t.Fatalf("不带 who 该给全量，实际 total=%d shown=%d", all.Total, all.Shown)
	}
	if all.Scope.Ref != "" || !all.Scope.Known {
		t.Fatalf("不带 who 时 scope 该是空的且 Known=true（没有认不出的东西），实际 %+v", all.Scope)
	}

	// 成员视角：只有跟他有关的行。
	bob, _ := grantsAt(t, s, "/api/grants?who=bob")
	if bob.Total != 3 {
		t.Fatalf("Total 是表里的总数，不该被视角改掉：%d", bob.Total)
	}
	if bob.Shown != 2 {
		t.Fatalf("bob 该命中 2 条（alice-to-bob 的 to + bare-name 的 from），实际 %d：%+v", bob.Shown, bob.Rows)
	}
	if !bob.Scope.Known || bob.Scope.Who == nil || bob.Scope.Who.Ref != "bob" {
		t.Fatalf("视角该解成 bob，实际 %+v", bob.Scope)
	}
	for _, r := range bob.Rows {
		if !r.Hit {
			t.Fatalf("露出来的行该标 hit：%+v", r)
		}
	}

	// 业务 agent 视角：同样按编号身份命中（不是靠名字像）。
	bot, _ := grantsAt(t, s, "/api/grants?who=order-bot")
	if bot.Shown != 1 || bot.Scope.Who == nil || bot.Scope.Who.Kind != "agent" {
		t.Fatalf("业务 agent 视角该命中 1 条并标出 kind=agent，实际 shown=%d scope=%+v", bot.Shown, bot.Scope)
	}
}

// 视角选择器不能被视角自己缩水：筛过的 rows 只剩他相关的，选择器要从**全表**取 ——
// 否则「换个视角看看」这条路就走不通（换不回全量、也换不到第二个人）。
// 另外只收**主体**：岗位与域不是主体（域是客体，SPEC §6），摆进选择器等于暗示
// 「可以站在一个域的位置上看」—— 那是另一件事，还没有定义处。
func TestGrantsSubjectsAreNotNarrowedByScope(t *testing.T) {
	s := &Server{Vault: grantsFixture(t)}
	all, _ := grantsOf(t, s)
	if len(all.Subjects) != 3 {
		t.Fatalf("表里出现过 3 个主体（alice / bob / order-bot），实际 %d：%+v", len(all.Subjects), all.Subjects)
	}
	for _, sub := range all.Subjects {
		if sub.Kind != "member" && sub.Kind != "agent" {
			t.Fatalf("选择器里出现了非主体（kind=%q）：%+v", sub.Kind, sub)
		}
	}
	bob, _ := grantsAt(t, s, "/api/grants?who=bob")
	if len(bob.Subjects) != len(all.Subjects) {
		t.Fatalf("视角把选择器缩水了：全量 %d 个，bob 视角下只剩 %d 个", len(all.Subjects), len(bob.Subjects))
	}
	if bob.Shown != 2 {
		t.Fatalf("bob 该命中 2 条，实际 %d", bob.Shown)
	}
}

// 认不出的视角：**不给全量表**（否则会被读成「他的权限是全部」），也不给一份空表冒充
// 「他什么都没有」—— 照实说认不出，并说清怎么写才对。
func TestGrantsUnknownScopeIsNotEverything(t *testing.T) {
	s := &Server{Vault: grantsFixture(t)}
	for _, bad := range []string{"ghost", "role:manager", "domain:trade"} {
		v, _ := grantsAt(t, s, "/api/grants?who="+bad)
		if v.Scope.Known {
			t.Fatalf("%s 不是主体，不该判成认得出来：%+v", bad, v.Scope)
		}
		if v.Scope.Why == "" {
			t.Fatalf("%s 认不出却没给理由：%+v", bad, v.Scope)
		}
		if v.Scope.Who != nil {
			t.Fatalf("%s 认不出却给了身份：%+v", bad, v.Scope.Who)
		}
		if len(v.Rows) != 0 {
			t.Fatalf("%s 认不出却露出 %d 行 —— 那会被读成「他的权限是全部」", bad, len(v.Rows))
		}
		if v.Total != 3 {
			t.Fatalf("认不出的视角也要如实说表里有多少条：%d", v.Total)
		}
	}
}

// 一条授权都没有是合法状态，不是故障：表默认零条（SPEC §6 不变量 1：没有 grant 就是不通）。
func TestGrantsZeroIsNotAnError(t *testing.T) {
	vault := t.TempDir()
	if err := os.CopyFS(vault, os.DirFS(fixturePath(t, "domains"))); err != nil {
		t.Fatal(err)
	}
	v, body := grantsOf(t, &Server{Vault: vault})
	if !v.Wired || v.Error != "" {
		t.Fatalf("没有 grants/ 目录不算错，实际 wired=%v error=%q", v.Wired, v.Error)
	}
	if v.Total != 0 || len(v.Rows) != 0 {
		t.Fatalf("零条时该是空表：%+v", v.Rows)
	}
	// 空数组不是 null：前端直接 .length / .map。
	for _, want := range []string{`"rows": []`, `"findings": []`} {
		if !strings.Contains(body, want) {
			t.Fatalf("响应里该出现 %s：\n%s", want, body)
		}
	}
}

// 发现照抄 org 那套：规则 id + **生效档次**（档次由规则表定，这一层不许硬编码）。
func TestGrantsCarriesFindingsFromTheRuleTable(t *testing.T) {
	vault := t.TempDir()
	if err := os.CopyFS(vault, os.DirFS(fixturePath(t, "domains"))); err != nil {
		t.Fatal(err)
	}
	// 域当授权者：域是客体不是主体（SPEC §6）—— 该报 grant.from.unknown。
	writeInto(t, vault, "grants/domain-as-from.md",
		grantText("domain:trade", "member:bob", "read", "shipments", "", "写错了"))
	v, _ := grantsOf(t, &Server{Vault: vault})
	found := false
	for _, f := range v.Findings {
		if !strings.HasPrefix(f.Rule, grantRulePrefix) {
			t.Fatalf("授权表这一页只出 grant.* 的发现，出了 %q", f.Rule)
		}
		if f.Rule == "grant.from.unknown" {
			found = true
			if f.Level != "warn" {
				t.Fatalf("这一条出厂档是 warn（新能力先看见不先拦），实际 %q", f.Level)
			}
			if f.Where != "grants/domain-as-from.md" {
				t.Fatalf("定位串该指回那个文件，实际 %q", f.Where)
			}
		}
	}
	if !found {
		t.Fatalf("域当 from 该报 grant.from.unknown：%+v", v.Findings)
	}
}

// 这一页答不了的要如实列 —— 缺的部分不许用「图好看」补上。
// 每一条都是当前口径里明确挂起或未做的，改口径要连带改它。
func TestGrantsListsWhatItCannotAnswer(t *testing.T) {
	v, _ := grantsOf(t, &Server{Vault: grantsFixture(t)})
	if len(v.Missing) == 0 {
		t.Fatal("missing 为空 —— 这一页答不了的那些事必须写出来")
	}
	joined := strings.Join(v.Missing, "\n")
	for _, want := range []string{"挂起", "级联", "该不该给"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing 里该提到 %q：\n%s", want, joined)
		}
	}
	// 判断归人这条口径必须写在页面上（否则读的人会以为这一页在替人拍板）。
	if !strings.Contains(v.Note, "不是执行层") {
		t.Fatalf("note 该说清这是控制面真相源、不是执行层：%q", v.Note)
	}
}

// 看板只读：这一页也没有写口。
func TestGrantsIsReadOnly(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/grants", nil)
	rec := httptest.NewRecorder()
	(&Server{Vault: grantsFixture(t)}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("状态码 %d，期望 405", rec.Code)
	}
}
