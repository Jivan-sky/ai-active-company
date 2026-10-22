package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"anc/internal/audit"
	"anc/internal/org"
	renderpkg "anc/internal/render"
	"anc/internal/retrieval"
)

// maxDocReplyBytes 是 MCP 取正文的上限：再大的文档塞进 harness 上下文只会把真正的问题挤掉。
// 截断**明说**（truncated / total_bytes），不静默给半篇冒充全文。
const maxDocReplyBytes = 64 << 10

// ---------- 知识检索（D13：接口抽象 + 默认 BM25 + 权限以 org 为准） ----------
//
// 这里只做三件事：解「这次能看哪些域」、调检索包、如实回话。索引怎么建、怎么打分
// 在 internal/retrieval；谁能看哪块在 org（这里不另造一套 ACL）。

const searchUsage = `anc search —— 按域可见范围搜 vault 里的知识

用法：
  anc search <vault 目录> --q "要查什么" [选项]
  anc search <vault 目录> --get <vault 相对路径> [选项]

选项：
  --q <词>         要查的问题 / 关键词（--get 时可省）
  --get <路径>     取一篇正文（只认索引里且可见的路径）
  --who <编号>     以谁的身份搜：成员名 / 业务 agent 的 slug；不写 = 本机操作者（全部域）
  --domain <slug>  只问这一块业务（domains.md 的 slug）
  --roots <目录>   这次索引哪些 vault 相对目录，逗号分隔；不写 = 10-knowledge / knowledge + 各域 data
  --backend <名>   检索后端：bm25（默认，零依赖）；其他名字明确报错，不静默回落
  --limit <n>      返回几条（默认 8，上限 50）
  --json           机器读的出口

口径：
  - 默认件是本地 BM25：索引每次从 markdown 重建，**不是真相源**，删掉不影响答案。
  - 权限只认 org：成员看自己 domains；管理员或 company.md 的 access.cross_domain=visible
    看全部；业务 agent 只按 agents.md 的 domain 看。看不见的文档不参与打分、不计数。
  - 公司级资料（frontmatter 没写 domain、或显式写空）谁都能看 —— 同看板首页。
  - 检索给的是**指针 + 片段**，不是正文结论；查不到就如实说没入库，不按记忆补。
`

type searchOut struct {
	Vault     string          `json:"vault"`
	Query     string          `json:"query,omitempty"`
	Who       string          `json:"who,omitempty"`
	Scope     []string        `json:"scope"` // ["*"] = 全部域
	ScopeNote string          `json:"scope_note,omitempty"`
	Backend   string          `json:"backend,omitempty"`
	Indexed   int             `json:"indexed"`
	Visible   int             `json:"visible"`
	Count     int             `json:"count"`
	Hits      []retrieval.Hit `json:"hits"`
	Doc       *searchDoc      `json:"doc,omitempty"`
	Warnings  []string        `json:"warnings,omitempty"`
	Note      string          `json:"note,omitempty"`
	Refused   *searchRefusal  `json:"refused,omitempty"`
	At        string          `json:"at"`
}

// searchDoc 是 --get 的回话形状。不直接序列化 retrieval.Doc：正文这一格要显式命名，
// 免得以后给 Doc 加内部字段时悄悄漏进回话。
type searchDoc struct {
	Path    string `json:"path"`
	Domain  string `json:"domain,omitempty"`
	Title   string `json:"title,omitempty"`
	Body    string `json:"body"`
	Updated string `json:"updated,omitempty"`
}

type searchRefusal struct {
	Reason string `json:"reason"`
	Ask    string `json:"ask,omitempty"`
}

func cmdSearch(args []string) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	q := fs.String("q", "", "要查什么")
	get := fs.String("get", "", "取一篇正文（vault 相对路径）")
	who := fs.String("who", "", "以谁的身份搜")
	domain := fs.String("domain", "", "只问这一块业务")
	roots := fs.String("roots", "", "这次索引哪些 vault 相对目录，逗号分隔")
	backend := fs.String("backend", envOr("ANC_SEARCH_BACKEND", "bm25"), "检索后端（默认 bm25）")
	limit := fs.Int("limit", retrieval.DefaultLimit, "返回几条")
	asJSON := fs.Bool("json", false, "机器读的出口")
	flagArgs, posArgs := splitArgs(args, map[string]bool{"json": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) != 1 {
		fmt.Fprint(os.Stderr, searchUsage)
		return 2
	}
	if strings.TrimSpace(*q) == "" && strings.TrimSpace(*get) == "" {
		fmt.Fprint(os.Stderr, "错误：--q 与 --get 至少给一个\n\n")
		fmt.Fprint(os.Stderr, searchUsage)
		return 2
	}
	abs, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 2
	}
	o, err := org.Load(abs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n（先修 org：anc org check <vault>）\n", err)
		return 1
	}

	out := searchOut{
		Vault: abs, Query: strings.TrimSpace(*q), Who: strings.TrimSpace(*who),
		At: time.Now().Format(time.RFC3339),
	}
	sc, refuse, ok := resolveSearchScope(o, out.Who)
	if !ok {
		out.Refused = refuse
		return printSearch(out, *asJSON, true)
	}
	out.Scope = scopeLabels(sc)

	if d := strings.TrimSpace(*domain); d != "" {
		if _, declared := o.Domain(d); !declared {
			out.Refused = &searchRefusal{Reason: "domains.md 里没有「" + d + "」这块业务 —— 要么拼错了，要么这一域还没抽成表行"}
			return printSearch(out, *asJSON, true)
		}
		if !sc.Allows(d) {
			reason, helper := refuseDomainText(o, d)
			out.Refused = &searchRefusal{Reason: reason, Ask: helper}
			return printSearch(out, *asJSON, true)
		}
		// 「只问这一块业务」也包含公司级资料 —— 与 Viewer.CanSeeDomain("") 同一口径。
		sc = retrieval.Scope{Domains: []string{d}}
		out.Scope = []string{d}
		out.ScopeNote = "含公司级资料"
	}

	cfg := retrieval.Config{Backend: *backend}
	if r := strings.TrimSpace(*roots); r != "" {
		cfg.Roots = org.SplitList(r)
	}
	ix, warns, err := retrieval.Open(abs, o, cfg)
	out.Warnings = warns
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：索引建不起来：%v\n", err)
		return 1
	}
	out.Backend = ix.Name()
	out.Indexed, out.Visible = ix.Len(), ix.Visible(sc)

	if p := strings.TrimSpace(*get); p != "" {
		d, found, err := ix.Get(p, sc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误：%v\n", err)
			return 1
		}
		if !found {
			out.Refused = &searchRefusal{Reason: "取不到这篇：要么不在索引面里，要么不在你的可见范围"}
			return printSearch(out, *asJSON, true)
		}
		out.Doc = &searchDoc{
			Path: d.Path, Domain: d.Domain, Title: d.Title, Body: d.Body,
			Updated: d.ModTime.UTC().Format(time.RFC3339),
		}
		return printSearch(out, *asJSON, false)
	}

	hits, err := ix.Search(out.Query, sc, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	out.Hits = hits
	out.Count = len(out.Hits)
	if out.Count == 0 {
		out.Note = "没有命中 —— 可能是还没入库；不要按记忆补（查不到就说查不到）"
	}
	return printSearch(out, *asJSON, false)
}

// resolveSearchScope 解「这个主体能搜哪些域」。who 为空 = 本机操作者（全部域）。
//
// 成员跨域口径与看板进门判定同源（admin 或 access.cross_domain=visible）；业务 agent
// **不跟着放宽** —— 它代的是岗位 / 流程，agents.md 的 domain 就是它的数据视野。
func resolveSearchScope(o *org.Org, who string) (retrieval.Scope, *searchRefusal, bool) {
	if who == "" {
		return retrieval.Scope{All: true}, nil, true
	}
	id, ok := o.Identity(who)
	if !ok {
		return retrieval.Scope{}, &searchRefusal{
			Reason: "我们公司没有这个人 —— 身份不许自报：who 必须能在 members/ 或 agents.md 里解出来",
		}, false
	}
	sc := retrieval.Scope{Domains: id.Domains}
	if id.Kind == org.KindMember {
		v, found := o.ResolveMemberViewer(id.Ref)
		if !found {
			return retrieval.Scope{}, &searchRefusal{Reason: "这个成员不在白名单里或已停用 —— 拒绝（停用是离职回收的第一道闸）"}, false
		}
		sc.All, sc.Domains = v.AllDomains, v.Domains
	}
	return sc, nil, true
}

func scopeLabels(sc retrieval.Scope) []string {
	if sc.All {
		return []string{"*"}
	}
	return append([]string(nil), sc.Domains...)
}

func refuseDomainText(o *org.Org, slug string) (reason, helper string) {
	if d, ok := o.Domain(slug); ok {
		who := o.WhoLabel(d.Who)
		if who == "—" {
			who = ""
		}
		return renderpkg.RefuseDomainText(slug, who), who
	}
	return "domains.md 里没有「" + slug + "」这块业务 —— 要么拼错了，要么这一域还没抽成表行", ""
}

func printSearch(out searchOut, asJSON bool, refused bool) int {
	if asJSON {
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			return 1
		}
		fmt.Println(string(b))
		if refused {
			return 1
		}
		return 0
	}
	for _, w := range out.Warnings {
		fmt.Fprintf(os.Stderr, "注意：%s\n", w)
	}
	if out.Refused != nil {
		fmt.Printf("anc search —— 拒绝\n")
		fmt.Printf("  为什么  %s\n", out.Refused.Reason)
		if out.Refused.Ask != "" {
			fmt.Printf("  找谁    %s\n", out.Refused.Ask)
		}
		return 1
	}
	fmt.Printf("anc search —— 按域可见范围搜知识\n")
	fmt.Printf("  vault    %s\n", out.Vault)
	if out.Who != "" {
		scope := strings.Join(out.Scope, "、")
		if out.ScopeNote != "" {
			scope += "（" + out.ScopeNote + "）"
		}
		fmt.Printf("  谁       %s（可见：%s）\n", out.Who, scope)
	} else {
		fmt.Printf("  谁       本机操作者（全部域）\n")
	}
	fmt.Printf("  后端     %s\n", out.Backend)
	fmt.Printf("  索引     %d 篇（可见 %d 篇）\n", out.Indexed, out.Visible)
	if out.Doc != nil {
		fmt.Printf("  文档     %s  [%s]\n", out.Doc.Path, dashOr(out.Doc.Domain))
		fmt.Printf("  标题     %s\n", dashOr(out.Doc.Title))
		fmt.Printf("  正文     \n%s\n", out.Doc.Body)
		return 0
	}
	fmt.Printf("  问       %s\n", out.Query)
	fmt.Printf("  命中     %d 条\n", out.Count)
	for i, h := range out.Hits {
		fmt.Printf("  %d) %s  [%s]  score %.4f  %s\n", i+1, h.Path, dashOr(h.Domain), h.Score, h.Updated)
		if h.Title != "" {
			fmt.Printf("     %s\n", h.Title)
		}
		fmt.Printf("     %s\n", h.Snippet)
	}
	if out.Note != "" {
		fmt.Printf("  说明     %s\n", out.Note)
	}
	return 0
}

func dashOr(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// ---------- MCP 工具：anc_search ----------

// searchKnowledge 是 anc_search 的实现：与 CLI 同一套口径（resolveSearchScope / retrieval.Load），
// 每次调用留一条痕。看不见的域不是「零结果」，是**拒**并指路。
func (g *envelopeIngress) searchKnowledge(args map[string]any) (string, bool) {
	now := g.now()
	who := strArg(args, "who")
	q := strings.TrimSpace(strArg(args, "q"))
	ask := strings.TrimSpace(strArg(args, "domain"))
	limit := intArg(args, "limit", retrieval.DefaultLimit)

	o, err := org.Load(g.Vault)
	if err != nil {
		g.auditTool("anc_search", readActor(who), ask, audit.ResultFailed, "真相源加载失败", now)
		return "真相源加载失败，搜不了：" + err.Error() + "\n（先修 org：anc org check <vault>）", true
	}
	sc, refuse, ok := resolveSearchScope(o, who)
	if !ok {
		g.auditTool("anc_search", readActor(who), ask, audit.ResultDenied, refuse.Reason, now)
		return mustJSON(map[string]any{"refused": refuse, "at": now.Format(time.RFC3339)}), true
	}
	if q == "" {
		g.auditTool("anc_search", who, ask, audit.ResultFailed, "q 为空", now)
		return "q 为空 —— 要查什么？", true
	}
	if ask != "" {
		if _, declared := o.Domain(ask); !declared {
			reason := "domains.md 里没有「" + ask + "」这块业务 —— 要么拼错了，要么这一域还没抽成表行"
			g.auditTool("anc_search", who, "domain:"+ask, audit.ResultDenied, reason, now)
			return mustJSON(map[string]any{"refused": map[string]any{"reason": reason}, "at": now.Format(time.RFC3339)}), true
		}
		if !sc.Allows(ask) {
			reason, helper := refuseDomainText(o, ask)
			g.auditTool("anc_search", who, ask, audit.ResultDenied, reason, now)
			return mustJSON(map[string]any{
				"refused": map[string]any{"reason": reason, "ask": helper},
				"at":      now.Format(time.RFC3339),
			}), true
		}
		sc = retrieval.Scope{Domains: []string{ask}}
	}

	ix, warns, err := retrieval.Open(g.Vault, o, retrieval.Config{
		Backend: os.Getenv("ANC_SEARCH_BACKEND"),
		Roots:   org.SplitList(os.Getenv("ANC_SEARCH_ROOTS")),
	})
	if err != nil {
		g.auditTool("anc_search", who, ask, audit.ResultFailed, "索引建不起来", now)
		return "索引建不起来：" + err.Error(), true
	}
	object := ask
	if object == "" {
		object = "knowledge"
	}
	hits, err := ix.Search(q, sc, limit)
	if err != nil {
		g.auditTool("anc_search", who, object, audit.ResultFailed, "检索失败", now)
		return "检索失败：" + err.Error(), true
	}
	g.auditTool("anc_search", who, object, audit.ResultOK, "", now)

	out := map[string]any{
		"who": who, "scope": scopeLabels(sc), "backend": ix.Name(),
		"indexed": ix.Len(), "visible": ix.Visible(sc),
		"query": q, "count": len(hits), "hits": hits, "at": now.Format(time.RFC3339),
	}
	if len(hits) == 0 {
		out["note"] = "没有命中 —— 可能是还没入库；不要按记忆补（查不到就说查不到）"
	}
	if len(warns) > 0 {
		out["warnings"] = warns
	}
	return mustJSON(out), false
}

// getDocument 是 anc_get 的实现：按 anc_search 给的 path 取正文。
// path 只认索引里、且可见的（目录穿越没有入口）；看不见 = 拒，并说清为什么。
func (g *envelopeIngress) getDocument(args map[string]any) (string, bool) {
	now := g.now()
	who := strArg(args, "who")
	path := strings.TrimSpace(strArg(args, "path"))

	o, err := org.Load(g.Vault)
	if err != nil {
		g.auditTool("anc_get", readActor(who), path, audit.ResultFailed, "真相源加载失败", now)
		return "真相源加载失败，取不了：" + err.Error() + "\n（先修 org：anc org check <vault>）", true
	}
	sc, refuse, ok := resolveSearchScope(o, who)
	if !ok {
		g.auditTool("anc_get", readActor(who), path, audit.ResultDenied, refuse.Reason, now)
		return mustJSON(map[string]any{"refused": refuse, "at": now.Format(time.RFC3339)}), true
	}
	if path == "" {
		g.auditTool("anc_get", who, path, audit.ResultFailed, "path 为空", now)
		return "path 为空 —— 要取哪一篇？（用 anc_search 回话里的 path）", true
	}
	ix, warns, err := retrieval.Open(g.Vault, o, retrieval.Config{
		Backend: os.Getenv("ANC_SEARCH_BACKEND"),
		Roots:   org.SplitList(os.Getenv("ANC_SEARCH_ROOTS")),
	})
	if err != nil {
		g.auditTool("anc_get", who, path, audit.ResultFailed, "索引建不起来", now)
		return "索引建不起来：" + err.Error(), true
	}
	d, found, err := ix.Get(path, sc)
	if err != nil {
		g.auditTool("anc_get", who, path, audit.ResultFailed, "取正文失败", now)
		return "取正文失败：" + err.Error(), true
	}
	if !found {
		reason := "取不到这篇：要么不在索引面里，要么不在你的可见范围"
		g.auditTool("anc_get", who, path, audit.ResultDenied, reason, now)
		return mustJSON(map[string]any{
			"refused": map[string]any{"reason": reason},
			"at":      now.Format(time.RFC3339),
		}), true
	}
	g.auditTool("anc_get", who, path, audit.ResultOK, "", now)
	body, truncated := clipUTF8(d.Body, maxDocReplyBytes)
	out := map[string]any{
		"path": d.Path, "title": d.Title, "domain": d.Domain, "body": body,
		"total_bytes": len(d.Body), "truncated": truncated, "at": now.Format(time.RFC3339),
	}
	if len(warns) > 0 {
		out["warnings"] = warns
	}
	return mustJSON(out), false
}

// clipUTF8 按字节上限截断，但**不切半个字**（中文一个字 3 字节；切一半会变成非法 UTF-8）。
func clipUTF8(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
