package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"anc/internal/audit"
)

// ---------- 知识检索（D13）的用例 ----------
//
// 锁三件事：
//  1 看不见的域不出现 —— 检索不是「先全给再遮住」；
//  2 公司级资料谁都能看（同 Viewer.CanSeeDomain 的空域口径）；
//  3 每次 MCP 调用都留痕，被拒的那次也留。

func seedSearchDocs(t *testing.T, vault string) {
	t.Helper()
	writeSearchDoc(t, vault, "10-knowledge/trade.md", "---\ndomain: trade\ntitle: Q3 结算差异\n---\n结算口径与合同执行。")
	writeSearchDoc(t, vault, "10-knowledge/logi.md", "---\ndomain: logistics\ntitle: 到货跟踪\n---\n结算与 ETA。")
	writeSearchDoc(t, vault, "10-knowledge/shared.md", "---\ntitle: 公司报销制度\n---\n结算与报销审批。")
}

func writeSearchDoc(t *testing.T, vault, rel, content string) {
	t.Helper()
	p := filepath.Join(vault, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// hideCrossDomain 把 cross_domain=hidden 插进 company.md 的 **frontmatter** 里
// （追加到文件末尾只会变成正文，解析层看不到）。
func hideCrossDomain(t *testing.T, vault string) {
	t.Helper()
	p := filepath.Join(vault, "company", "company.md")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		t.Fatalf("company.md 没有 frontmatter：\n%s", s)
	}
	end := strings.Index(s[4:], "\n---")
	if end < 0 {
		t.Fatalf("company.md 的 frontmatter 没闭合：\n%s", s)
	}
	s = s[:4+end] + "\naccess:\n  cross_domain: hidden" + s[4+end:]
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

type cliSearch struct {
	Who     string   `json:"who"`
	Scope   []string `json:"scope"`
	Indexed int      `json:"indexed"`
	Visible int      `json:"visible"`
	Count   int      `json:"count"`
	Hits    []struct {
		Path   string `json:"path"`
		Domain string `json:"domain"`
	} `json:"hits"`
	Doc *struct {
		Path string `json:"path"`
		Body string `json:"body"`
	} `json:"doc"`
	Refused *struct {
		Reason string `json:"reason"`
		Ask    string `json:"ask"`
	} `json:"refused"`
}

func runSearchCLI(t *testing.T, vault string, extra ...string) (cliSearch, int) {
	t.Helper()
	args := append(append([]string{}, extra...), vault, "--json")
	out, code := captureStdout(t, func() int { return cmdSearch(args) })
	var got cliSearch
	if !json.Valid([]byte(out)) {
		t.Fatalf("stdout 不是合法 JSON：\n%s", out)
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	return got, code
}

func TestSearchCLIFiltersByMemberDomains(t *testing.T) {
	vault := copyFixture(t, "domains")
	seedSearchDocs(t, vault)
	hideCrossDomain(t, vault)

	bob, code := runSearchCLI(t, vault, "--q", "结算", "--who", "bob", "--roots", "10-knowledge")
	if code != 0 {
		t.Fatalf("退出码 %d", code)
	}
	if bob.Visible != 2 { // logi.md + shared.md；trade.md 看不见
		t.Fatalf("bob 可见篇数该是 2，实际 %d", bob.Visible)
	}
	for _, h := range bob.Hits {
		if h.Domain == "trade" {
			t.Fatalf("看不见的域漏出来了：%+v", h)
		}
	}

	alice, code := runSearchCLI(t, vault, "--q", "结算", "--who", "alice", "--roots", "10-knowledge")
	if code != 0 {
		t.Fatalf("退出码 %d", code)
	}
	if alice.Visible != 3 {
		t.Fatalf("管理员该看见 3 篇，实际 %d", alice.Visible)
	}
}

func TestSearchCLIRefusesForeignDomainAndPointsToOwner(t *testing.T) {
	vault := copyFixture(t, "domains")
	seedSearchDocs(t, vault)
	hideCrossDomain(t, vault)

	got, code := runSearchCLI(t, vault, "--q", "结算", "--who", "bob", "--domain", "trade", "--roots", "10-knowledge")
	if code != 1 || got.Refused == nil {
		t.Fatalf("跨域提问该被拒：code=%d out=%+v", code, got)
	}
	if !strings.Contains(got.Refused.Reason, "不在你的可见范围") {
		t.Fatalf("拒话不对：%q", got.Refused.Reason)
	}
	if got.Refused.Ask == "" {
		t.Fatal("拒了要指路（找谁），否则 agent 只能转头去问人")
	}
}

func TestSearchCLIRefusesUnknownWho(t *testing.T) {
	vault := copyFixture(t, "domains")
	seedSearchDocs(t, vault)
	got, code := runSearchCLI(t, vault, "--q", "结算", "--who", "ghost", "--roots", "10-knowledge")
	if code != 1 || got.Refused == nil || !strings.Contains(got.Refused.Reason, "没有这个人") {
		t.Fatalf("不存在的身份该拒：code=%d out=%+v", code, got)
	}
}

func TestSearchCLIRefusesDisabledMember(t *testing.T) {
	vault := copyFixture(t, "disabled")
	got, code := runSearchCLI(t, vault, "--q", "结算", "--who", "frank")
	if code != 1 || got.Refused == nil || !strings.Contains(got.Refused.Reason, "停用") {
		t.Fatalf("已停用成员该被拒：code=%d out=%+v", code, got)
	}
}

func TestSearchCLIRefusesUndeclaredDomain(t *testing.T) {
	vault := copyFixture(t, "domains")
	got, code := runSearchCLI(t, vault, "--q", "结算", "--domain", "nope")
	if code != 1 || got.Refused == nil || !strings.Contains(got.Refused.Reason, "domains.md 里没有") {
		t.Fatalf("没声明的域该明确报出来：code=%d out=%+v", code, got)
	}
}

func TestSearchCLIGetOnlyVisible(t *testing.T) {
	vault := copyFixture(t, "domains")
	seedSearchDocs(t, vault)
	hideCrossDomain(t, vault)

	if _, code := runSearchCLI(t, vault, "--get", "10-knowledge/trade.md", "--who", "bob", "--roots", "10-knowledge"); code != 1 {
		t.Fatalf("bob 不该取得到 trade 的正文，退出码 %d", code)
	}
	got, code := runSearchCLI(t, vault, "--get", "10-knowledge/trade.md", "--who", "alice", "--roots", "10-knowledge")
	if code != 0 || got.Doc == nil || !strings.Contains(got.Doc.Body, "结算口径") {
		t.Fatalf("alice 该取得到正文：code=%d out=%+v", code, got)
	}
}

func TestSearchMCPIsRegisteredAndAudited(t *testing.T) {
	h, g := readHarness(t)
	seedSearchDocs(t, g.Vault)

	tools := mcpRoundTrip(t, h, "tools/list", map[string]any{})
	found := false
	for _, tl := range tools.Result.Tools {
		if tl.Name == "anc_search" {
			found = true
		}
	}
	if !found {
		t.Fatal("tools/list 里没有 anc_search")
	}

	text, isErr := callTool(t, h, "anc_search", map[string]any{"who": "alice", "q": "结算"})
	if isErr {
		t.Fatalf("该成功：%s", text)
	}
	if !strings.Contains(text, "10-knowledge/trade.md") {
		t.Fatalf("没搜到 trade 的文档：%s", text)
	}
	doc, err := audit.Load(g.Vault)
	if err != nil {
		t.Fatal(err)
	}
	hit := false
	for _, r := range doc.Records {
		if r.Tool == "anc_search" && r.Actor == "alice" && r.Result == audit.ResultOK {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("检索没留痕：%+v", doc.Records)
	}
}

func TestSearchMCPDeniesForeignDomainAndAudits(t *testing.T) {
	h, g := readHarness(t)
	seedSearchDocs(t, g.Vault)
	hideCrossDomain(t, g.Vault)

	text, isErr := callTool(t, h, "anc_search", map[string]any{"who": "bob", "q": "结算", "domain": "trade"})
	if !isErr {
		t.Fatalf("跨域检索该被拒：%s", text)
	}
	if !strings.Contains(text, "不在你的可见范围") {
		t.Fatalf("拒话不对：%s", text)
	}
	doc, err := audit.Load(g.Vault)
	if err != nil {
		t.Fatal(err)
	}
	hit := false
	for _, r := range doc.Records {
		if r.Tool == "anc_search" && r.Result == audit.ResultDenied {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("被拒的那次也要留痕：%+v", doc.Records)
	}
}

func TestGetMCPReturnsVisibleBodyAndAudits(t *testing.T) {
	h, g := readHarness(t)
	seedSearchDocs(t, g.Vault)

	text, isErr := callTool(t, h, "anc_get", map[string]any{"who": "alice", "path": "10-knowledge/trade.md"})
	if isErr {
		t.Fatalf("该取得到：%s", text)
	}
	if !strings.Contains(text, "结算口径") || !strings.Contains(text, "Q3 结算差异") {
		t.Fatalf("正文 / 标题没回来：%s", text)
	}
	doc, err := audit.Load(g.Vault)
	if err != nil {
		t.Fatal(err)
	}
	hit := false
	for _, r := range doc.Records {
		if r.Tool == "anc_get" && r.Actor == "alice" && r.Result == audit.ResultOK {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("取正文没留痕：%+v", doc.Records)
	}
}

func TestGetMCPDeniesInvisibleBody(t *testing.T) {
	h, g := readHarness(t)
	seedSearchDocs(t, g.Vault)
	hideCrossDomain(t, g.Vault)

	text, isErr := callTool(t, h, "anc_get", map[string]any{"who": "bob", "path": "10-knowledge/trade.md"})
	if !isErr || !strings.Contains(text, "取不到这篇") {
		t.Fatalf("看不见的正文该拒：isErr=%v %s", isErr, text)
	}
}

func TestGetMCPTruncatesLargeBodyExplicitly(t *testing.T) {
	h, g := readHarness(t)
	writeSearchDoc(t, g.Vault, "10-knowledge/big.md", "---\ntitle: 大文档\n---\n"+strings.Repeat("字", maxDocReplyBytes))

	text, isErr := callTool(t, h, "anc_get", map[string]any{"who": "alice", "path": "10-knowledge/big.md"})
	if isErr {
		t.Fatalf("该取得到：%s", text)
	}
	var got struct {
		Body       string `json:"body"`
		TotalBytes int    `json:"total_bytes"`
		Truncated  bool   `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || got.TotalBytes <= maxDocReplyBytes ||
		len(got.Body) > maxDocReplyBytes || len(got.Body) < maxDocReplyBytes-4 || !utf8.ValidString(got.Body) {
		t.Fatalf("截断口径不对：truncated=%v total=%d bodylen=%d", got.Truncated, got.TotalBytes, len(got.Body))
	}
}
