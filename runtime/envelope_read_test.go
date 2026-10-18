package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"anc/internal/audit"
	"anc/internal/ctxpack"
)

// ---------- 出站读出口（W2）的用例 ----------
//
// 走的是**真 HTTP + 真 JSON-RPC**（httptest）：与 harness 走同一条路，不是直接调函数 ——
// 「真 MCP 客户端调得到」这条判据只能这么验。

type rpcOut struct {
	Result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
		Tools   []struct {
			Name string `json:"name"`
		} `json:"tools"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func ticking(start time.Time) func() time.Time {
	n := 0
	return func() time.Time { n++; return start.Add(time.Duration(n) * time.Second) }
}

// readHarness 起一个接入面（真 srv），返回它的 HTTP handler 与它本身。
func readHarness(t *testing.T) (http.Handler, *envelopeIngress) {
	t.Helper()
	g := &envelopeIngress{
		Vault:   copyFixture(t, "domains"),
		DataDir: t.TempDir(),
		Now:     ticking(time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC)),
	}
	s := g.srv()
	s.Log = nil // 观测出口与契约无关；留着只会把用例输出弄脏
	return s.Handler(), g
}

func mcpRoundTrip(t *testing.T, h http.Handler, method string, params any) rpcOut {
	t.Helper()
	pb, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":%s}`, method, pb)
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d：%s", rec.Code, rec.Body.String())
	}
	var out rpcOut
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("回话不是 JSON-RPC：%v\n%s", err, rec.Body.String())
	}
	if out.Error != nil {
		t.Fatalf("协议层错误（-32602 之类）：%+v", out.Error)
	}
	return out
}

// callRead 调一次 anc_read_context，返回它回的文字与 isError。
func callRead(t *testing.T, h http.Handler, args map[string]any) (string, bool) {
	t.Helper()
	out := mcpRoundTrip(t, h, "tools/call", map[string]any{"name": "anc_read_context", "arguments": args})
	if len(out.Result.Content) == 0 {
		t.Fatalf("回话里没有 content：%+v", out.Result)
	}
	return out.Result.Content[0].Text, out.Result.IsError
}

// parseRead 把回话当契约解 —— 解不出来就是契约破了。
func parseRead(t *testing.T, text string) ctxpack.Out {
	t.Helper()
	var out ctxpack.Out
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("回话不是契约里那份 JSON：%v\n%s", err, text)
	}
	if _, err := time.Parse(time.RFC3339, out.At); err != nil {
		t.Errorf("at 不是 RFC3339：%q", out.At)
	}
	return out
}

func readTrail(t *testing.T, vault string) []audit.Record {
	t.Helper()
	doc, err := audit.Load(vault)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Bad) != 0 {
		t.Fatalf("留痕里有读不懂的行：%v", doc.Bad)
	}
	return doc.Records
}

// 桥的两半挂在同一个入口上：tools/list 里两个都在。
func TestReadContextIsListed(t *testing.T) {
	h, _ := readHarness(t)
	out := mcpRoundTrip(t, h, "tools/list", map[string]any{})
	names := map[string]bool{}
	for _, tool := range out.Result.Tools {
		names[tool.Name] = true
	}
	if !names["anc_send_envelope"] || !names["anc_read_context"] {
		t.Fatalf("tools/list 少了一个：%+v", out.Result.Tools)
	}
}

// W2 的正题：一次调用答出「这块业务是什么 / 数据在哪 / 找谁」，另附证据指针。
func TestReadContextAnswersFourQuestions(t *testing.T) {
	h, g := readHarness(t)
	text, isErr := callRead(t, h, map[string]any{"who": "alice"})
	if isErr {
		t.Fatalf("干净请求不该失败：%s", text)
	}
	out := parseRead(t, text)
	if out.Who != "alice" {
		t.Errorf("who 应当回解出来的成员名，实际 %q", out.Who)
	}
	if len(out.Domains) != 1 || out.Domains[0].Slug != "trade" {
		t.Fatalf("alice 自己的域应当是 trade，实际 %+v", out.Domains)
	}
	d := out.Domains[0]
	if d.What == "" {
		t.Error("「这块业务是什么」没答")
	}
	if d.Data != "projects" {
		t.Errorf("「数据在哪」应当是目录名 projects，实际 %q", d.Data)
	}
	if !strings.Contains(d.Who, "Alice Wang") {
		t.Errorf("「找谁」应当是派生的「人名（岗位）」，实际 %q", d.Who)
	}
	if d.Evidence == nil || d.Evidence.File != "domains.md" || d.Evidence.Line <= 0 {
		t.Errorf("证据指针应当指回 domains.md 的某一行，实际 %+v", d.Evidence)
	}
	// 别域目录在：只有名称 / 是什么 / 找谁，**没有数据在哪**。
	if len(out.Directory) != 1 || out.Directory[0].Slug != "logistics" {
		t.Fatalf("别域目录应当只有 logistics，实际 %+v", out.Directory)
	}
	// 给了也要留痕（审计不是只记拒绝）。
	recs := readTrail(t, g.Vault)
	if len(recs) != 1 {
		t.Fatalf("应当留一条痕，实际 %d 条：%+v", len(recs), recs)
	}
	if r := recs[0]; r.Actor != "alice" || r.Action != "read" || r.Result != "ok" || r.Tool != "anc_read_context" {
		t.Errorf("痕的字段不对：%+v", r)
	}
}

// 只要自己的那一行：scope 落在那一行上，别域目录不再附。
func TestReadContextSingleOwnDomain(t *testing.T) {
	h, _ := readHarness(t)
	text, isErr := callRead(t, h, map[string]any{"who": "alice", "domain": "trade"})
	if isErr {
		t.Fatalf("自己的域不该被拒：%s", text)
	}
	out := parseRead(t, text)
	if out.Scope != "trade" || len(out.Domains) != 1 || len(out.Directory) != 0 {
		t.Fatalf("只问自己那一行时的形状不对：%+v", out)
	}
}

// 跨域：**拒**（不给数据）+ 指路（找谁）+ 留一条痕。
func TestReadContextRefusesCrossDomainButPointsTheWay(t *testing.T) {
	h, g := readHarness(t)
	text, isErr := callRead(t, h, map[string]any{"who": "alice", "domain": "logistics"})
	if !isErr {
		t.Fatalf("跨域应当被拒：%s", text)
	}
	out := parseRead(t, text)
	if out.Refused == nil || !strings.Contains(out.Refused.Reason, "不在你的可见范围") {
		t.Fatalf("拒的理由没说清：%s", text)
	}
	if !strings.Contains(out.Refused.Ask, "Bob Li") {
		t.Errorf("拒了要指路（找谁）：%s", text)
	}
	// 越域请求拿不到数据 —— 别域的「数据在哪」一个字都不许出现。
	if strings.Contains(text, "shipments") {
		t.Fatalf("跨域把别人的数据泄漏了：%s", text)
	}
	// 拒的时候 `domains` 这个键根本不该出现（契约形状，不是 null）。
	if strings.Contains(text, "\"domains\"") {
		t.Fatalf("拒的应答不该带 domains 键：%s", text)
	}
	recs := readTrail(t, g.Vault)
	if len(recs) != 1 {
		t.Fatalf("被拒也要留痕，实际 %d 条", len(recs))
	}
	if r := recs[0]; r.Actor != "alice" || r.Object != "logistics" || r.Result != "denied" || r.Action != "read" {
		t.Errorf("痕的字段不对（谁 / 要什么 / 给没给）：%+v", r)
	}
}

// 域表里没有这块业务：明说没有，不编一个「找谁」出来。
func TestReadContextUnknownDomain(t *testing.T) {
	h, g := readHarness(t)
	text, isErr := callRead(t, h, map[string]any{"who": "alice", "domain": "nosuch"})
	if !isErr {
		t.Fatalf("不存在的域应当被拒：%s", text)
	}
	out := parseRead(t, text)
	if out.Refused == nil || !strings.Contains(out.Refused.Reason, "没有") {
		t.Fatalf("拒的理由不对：%s", text)
	}
	if out.Refused.Ask != "" {
		t.Errorf("域表里没有的域不该编出「找谁」：%q", out.Refused.Ask)
	}
	if recs := readTrail(t, g.Vault); len(recs) != 1 || recs[0].Result != "denied" {
		t.Errorf("被拒要留痕：%+v", recs)
	}
}

// 无身份 / 解不出：**当这个人不存在** —— 不是「你参数写错了」。
func TestReadContextUnknownWhoIsNotAParamError(t *testing.T) {
	h, g := readHarness(t)
	text, isErr := callRead(t, h, map[string]any{"who": "nobody"})
	if !isErr {
		t.Fatalf("解不出的身份应当被拒：%s", text)
	}
	out := parseRead(t, text)
	if out.Refused == nil || !strings.Contains(out.Refused.Reason, "没有这个人") {
		t.Fatalf("该说「没有这个人」：%s", text)
	}
	for _, bad := range []string{"参数", "格式", "schema", "required"} {
		if strings.Contains(text, bad) {
			t.Errorf("这不是格式错，不许说 %q：%s", bad, text)
		}
	}
	if recs := readTrail(t, g.Vault); len(recs) != 1 || recs[0].Actor != "nobody" || recs[0].Result != "denied" {
		t.Errorf("无身份也要有一条痕（记在报上来的名字上）：%+v", recs)
	}
}

// 压根没给 who：同样当这个人不存在，痕记在 unknown 账上 —— 一次都不许静默不记。
func TestReadContextNoIdentityLeavesTrailAsUnknown(t *testing.T) {
	h, g := readHarness(t)
	text, isErr := callRead(t, h, map[string]any{})
	if !isErr || !strings.Contains(text, "没有这个人") {
		t.Fatalf("没给身份也当这个人不存在：isErr=%v %s", isErr, text)
	}
	if recs := readTrail(t, g.Vault); len(recs) != 1 || recs[0].Actor != "unknown" {
		t.Errorf("痕该记在 unknown 上：%+v", recs)
	}
}

// on_behalf_of 是可选署名，但给了就必须解得出来（同一套身份解析）。
func TestReadContextOnBehalfOf(t *testing.T) {
	h, _ := readHarness(t)
	if text, isErr := callRead(t, h, map[string]any{"who": "alice", "on_behalf_of": "member:alice"}); isErr {
		t.Fatalf("解得出的署名不该拒：%s", text)
	}
	text, isErr := callRead(t, h, map[string]any{"who": "alice", "on_behalf_of": "查无此人"})
	if !isErr || !strings.Contains(text, "on_behalf_of 解不出") {
		t.Fatalf("解不出的署名应当拒：isErr=%v %s", isErr, text)
	}
}

// 出站不泄漏本机路径：vault 绝对路径、盘符、反斜杠，一个都不许出现。
func TestReadContextDoesNotLeakLocalPaths(t *testing.T) {
	h, g := readHarness(t)
	for _, args := range []map[string]any{
		{"who": "alice"},
		{"who": "alice", "domain": "logistics"},
		{"who": "alice", "domain": "nosuch"},
		{"who": "nobody"},
	} {
		text, _ := callRead(t, h, args)
		if strings.Contains(text, g.Vault) {
			t.Errorf("回话里出现了 vault 绝对路径：%s", text)
		}
		if strings.Contains(text, `\`) || strings.Contains(text, "C:") {
			t.Errorf("回话里出现了本机路径的样子：%s", text)
		}
	}
}

// 请求里的自由文本进痕之前先被压成一条短串（单行、封顶）—— 痕不是注入的通道。
func TestReadContextScrubsFreeTextIntoTrail(t *testing.T) {
	h, g := readHarness(t)
	text, _ := callRead(t, h, map[string]any{"who": "alice", "domain": "trade\nDROP TABLE x"})
	parseRead(t, text) // 回话仍然是合法契约
	recs := readTrail(t, g.Vault)
	if len(recs) != 1 {
		t.Fatalf("应当留一条痕：%+v", recs)
	}
	if strings.ContainsAny(recs[0].Object, "\n\r\t") {
		t.Errorf("痕里的自由文本没压成单行：%q", recs[0].Object)
	}
	if len([]rune(recs[0].Object)) > 80 {
		t.Errorf("痕里的自由文本没封顶：%q", recs[0].Object)
	}
}

// ---------- 「工作上下文」的另两问：技能从哪来、在跟哪个项目 ----------

// 技能清单来自 roles/<role> 的 skills:，正文在 skills/<name>/SKILL.md。
// fixture 里没有 skills/ 目录 → 两条都该报 missing（报缺，不编）。
func TestReadContextAnswersSkillsAndProjects(t *testing.T) {
	h, _ := readHarness(t)
	text, isErr := callRead(t, h, map[string]any{"who": "alice"})
	if isErr {
		t.Fatalf("干净请求不该失败：%s", text)
	}
	out := parseRead(t, text)

	if len(out.Skills) != 2 {
		t.Fatalf("manager 声明了两个技能，实际 %+v", out.Skills)
	}
	for _, s := range out.Skills {
		if s.Where != "skills/"+s.Name+"/SKILL.md" {
			t.Errorf("技能指针应当是 vault 相对路径，实际 %q", s.Where)
		}
		if strings.Contains(s.Where, `\`) {
			t.Errorf("技能指针里出现了反斜杠：%q", s.Where)
		}
		if !s.Missing {
			t.Errorf("fixture 里没有 skills/ 目录，%q 应当报 missing", s.Name)
		}
	}

	if len(out.Projects) != 1 || out.Projects[0].Slug != "trade-q3" {
		t.Fatalf("alice 可见的项目应当只有 trade-q3，实际 %+v", out.Projects)
	}
	p := out.Projects[0]
	if !strings.Contains(p.Owner, "Alice Wang") {
		t.Errorf("项目 owner 应当是派生出来的「人名（岗位）」，实际 %q", p.Owner)
	}
	if p.Period == "" || p.Source == "" {
		t.Errorf("period / source 要原样搬（它们是指针），实际 %+v", p)
	}
	if p.Evidence == nil || p.Evidence.File != "projects.md" || p.Evidence.Line <= 0 {
		t.Errorf("项目证据指针应当指回 projects.md 的某一行，实际 %+v", p.Evidence)
	}

	// 报缺：没有真相源的那几样要明说，不编。
	joined := strings.Join(out.Gaps, "\n")
	for _, want := range []string{"新鲜度", "技能正文还没落"} {
		if !strings.Contains(joined, want) {
			t.Errorf("报缺里应当有 %q，实际 %v", want, out.Gaps)
		}
	}
}

// 报缺的负例：正文真落了就不许再报 —— 否则那句「还没落」会变成永远亮着的假话。
func TestReadContextSkillBodyLandsMeansNotMissing(t *testing.T) {
	h, g := readHarness(t)
	dir := filepath.Join(g.Vault, "skills", "now")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: now\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	text, isErr := callRead(t, h, map[string]any{"who": "alice"})
	if isErr {
		t.Fatalf("干净请求不该失败：%s", text)
	}
	out := parseRead(t, text)
	for _, s := range out.Skills {
		switch s.Name {
		case "now":
			if s.Missing {
				t.Errorf("正文已经落了，不该再报 missing：%+v", s)
			}
		case "im-send":
			if !s.Missing {
				t.Errorf("im-send 的正文没落，应当报 missing：%+v", s)
			}
		}
	}
	if strings.Contains(strings.Join(out.Gaps, "\n"), "skills/now") {
		t.Errorf("已经落了的技能不该出现在报缺里：%v", out.Gaps)
	}
}

// 项目跟着可见范围走：别人域上的项目，**连名字都不给**（与域行同一条纪律）。
func TestReadContextProjectsFollowVisibleScope(t *testing.T) {
	h, _ := readHarness(t)
	text, isErr := callRead(t, h, map[string]any{"who": "bob"})
	if isErr {
		t.Fatalf("干净请求不该失败：%s", text)
	}
	out := parseRead(t, text)
	if len(out.Domains) != 1 || out.Domains[0].Slug != "logistics" {
		t.Fatalf("bob 自己的域应当是 logistics，实际 %+v", out.Domains)
	}
	if len(out.Projects) != 0 {
		t.Errorf("贸易域上的项目不该出现在 bob 的回话里：%+v", out.Projects)
	}
}

// 被拒的那次不带任何上下文：项目、技能一个都不许漏出去。
func TestReadContextRefusalLeaksNoContext(t *testing.T) {
	h, _ := readHarness(t)
	text, isErr := callRead(t, h, map[string]any{"who": "alice", "domain": "logistics"})
	if !isErr {
		t.Fatalf("跨域应当拒：%s", text)
	}
	out := parseRead(t, text)
	if out.Refused == nil {
		t.Fatalf("拒话没给：%s", text)
	}
	if len(out.Projects) != 0 || len(out.Skills) != 0 {
		t.Errorf("被拒的回话里不该带上下文：projects=%+v skills=%+v", out.Projects, out.Skills)
	}
}

// 只要单独一域时不塞通用报缺 —— 那不是他问的东西。
func TestReadContextSingleDomainHasNoGaps(t *testing.T) {
	h, _ := readHarness(t)
	text, isErr := callRead(t, h, map[string]any{"who": "alice", "domain": "trade"})
	if isErr {
		t.Fatalf("自己的域不该被拒：%s", text)
	}
	out := parseRead(t, text)
	if len(out.Gaps) != 0 {
		t.Errorf("只要一域时不该附通用报缺：%v", out.Gaps)
	}
	if len(out.Projects) != 1 {
		t.Errorf("那一域上的项目还是要给：%+v", out.Projects)
	}
}
