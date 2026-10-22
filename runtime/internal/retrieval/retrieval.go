// Package retrieval —— 知识检索的唯一入口。
//
// 一句话：**默认件是本地 BM25，索引随时可以从 markdown 重建**；自托管检索层
// （WeKnora 那类）只在数据量大到文件 + grep 撑不住、且资料不许放飞书时才切后端。
// 索引不是真相源 —— 删掉重建，答案不变（SPEC §4.2「默认件不是升级件」）。
//
// 三条边界：
//
//	1 **权限不在这个包里发明**。调用方给 Scope（可见域），这里只回答「这份文档在不在
//	   可见范围」。域从哪来、谁能看哪块，答案永远在 org 真相源（D13）。
//	2 **不返回看不见的东西**，连计数都不返回。先过滤、再排序、后截断 —— 不能先打分
//	   再过滤，否则「命中几条」会变成一条侧信道。
//	3 **零第三方依赖**。分词、BM25、frontmatter 都是标准库；交付物仍是单 exe。
package retrieval

import (
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"anc/internal/org"
)

// 默认参数：出厂值，不是业务规则。调用方可以改，但改的是「这次检索」而非真相源。
const (
	DefaultLimit    = 8
	MaxLimit        = 50
	DefaultMaxBytes = 2 << 20 // 单篇 2 MiB：再大的多半是误放进来的原件，跳过并报出来
	bm25K1          = 1.2
	bm25B           = 0.75
	snippetRunes    = 180
)

// Doc 是索引里的一篇文档 —— **vault 相对路径**是唯一标识，不是本机绝对路径。
type Doc struct {
	Path    string
	Domain  string // domains.md 的 slug；空 = 公司级资料，登录进来的人都能看
	Title   string
	Body    string
	ModTime time.Time
}

// Hit 是回给调用方的一条命中。Body 故意不在这里：检索给指针与片段，正文用 Get 取。
type Hit struct {
	Path    string  `json:"path"`
	Title   string  `json:"title,omitempty"`
	Domain  string  `json:"domain,omitempty"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet,omitempty"`
	Updated string  `json:"updated,omitempty"`
}

// Scope 是「这次能看哪些域」。它是**调用方算好的结果**，检索层不重算。
//
// 空 domain（公司级资料）永远可见 —— 同 org.Viewer.CanSeeDomain：它本来就渲染在每个人的
// 上下文里，拒绝它只会让人连首页都打不开。
type Scope struct {
	All     bool
	Domains []string
}

// Allows 判一份文档的域在不在这次范围里。空域恒真。
func (s Scope) Allows(domain string) bool {
	if domain == "" || s.All {
		return true
	}
	for _, d := range s.Domains {
		if d == domain {
			return true
		}
	}
	return false
}

// Index 是一份只读的内存索引。并发读安全（构建后不再写）。
type Index struct {
	docs     []Doc
	postings map[string][]posting
	lens     []int
}

// Searcher 是检索后端的唯一接口。
//
// 现在只有 BM25 一个实现；装自托管检索层（WeKnora 那类）时新增一个实现即可，
// CLI / MCP / 看板的调用方式不变。**权限参数（Scope）在接口里**，任何一个后端
// 都没有「绕过过滤」的入口 —— 这是 D13 那句「权限永远以 ANC 的 org 为准」的代码形状。
type Searcher interface {
	Name() string
	Len() int
	Visible(sc Scope) int
	Search(query string, sc Scope, limit int) ([]Hit, error)
	Get(path string, sc Scope) (Doc, bool, error)
}

// 编译期钉住：默认件就是 BM25。
var _ Searcher = (*Index)(nil)

// Config 是「这次用哪个后端、索引哪些目录」的部署参数（不是 org 真相）。
// 空值 = 出厂默认：BM25 + DefaultRoots。
type Config struct {
	Backend  string
	Roots    []string
	MaxBytes int64
}

// Open 按配置选后端。**不认识的后端明确报错，不静默回落 BM25** ——
// 静默回落会让「装了 WeKnora 却还在用本地索引」变成假绿。
func Open(vault string, o *org.Org, cfg Config) (Searcher, []string, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Backend)) {
	case "", "bm25":
		return Load(vault, o, LoadOptions{Roots: cfg.Roots, MaxBytes: cfg.MaxBytes})
	default:
		return nil, nil, fmt.Errorf(
			"不认识的检索后端 %q：目前只实现 bm25；自托管检索层要作为 Searcher 适配器接入（权限仍由 Scope 把关）",
			cfg.Backend)
	}
}

// Name 是后端名（审计 / 回话里用来区分「这次是谁答的」）。
func (ix *Index) Name() string { return "bm25" }

type posting struct {
	doc int
	tf  int
}

// New 从内存里的文档建索引。docs 会被排序后接管；调用方不要再改切片元素。
func New(docs []Doc) *Index {
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	ix := &Index{docs: docs, postings: map[string][]posting{}, lens: make([]int, len(docs))}
	for i, d := range docs {
		tf := map[string]int{}
		for _, t := range tokenize(d.Title) {
			tf[t]++
			// 标题命中比正文命中更值钱：同一篇里标题的词再记一次，不改 BM25 公式。
			tf[t]++
		}
		for _, t := range tokenize(d.Body) {
			tf[t]++
		}
		n := 0
		for t, c := range tf {
			ix.postings[t] = append(ix.postings[t], posting{doc: i, tf: c})
			n += c
		}
		ix.lens[i] = n
	}
	return ix
}

// Len 是索引里的文档总数（**不按可见性过滤**；给人看的是 Visible）。
func (ix *Index) Len() int { return len(ix.docs) }

// Visible 数这次范围内能看见几篇。
func (ix *Index) Visible(sc Scope) int {
	n := 0
	for _, d := range ix.docs {
		if sc.Allows(d.Domain) {
			n++
		}
	}
	return n
}

// Search 返回按 BM25 排序的命中。limit <= 0 = DefaultLimit；超过 MaxLimit 会被夹到上限。
//
// 先按 Scope 过滤再打分：看不见的文档不参与 df / 排序 / 计数，避免「命中几条」泄露。
func (ix *Index) Search(query string, sc Scope, limit int) ([]Hit, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	terms := uniqueTokens(query)
	if len(terms) == 0 || len(ix.docs) == 0 {
		return nil, nil
	}
	visible := make([]bool, len(ix.docs))
	nVisible := 0
	totalVisibleLen := 0
	for i, d := range ix.docs {
		visible[i] = sc.Allows(d.Domain)
		if visible[i] {
			nVisible++
			totalVisibleLen += ix.lens[i]
		}
	}
	if nVisible == 0 {
		return nil, nil
	}
	// 词频 / 长度归一都只按**这次能看见的语料**算：看不见的文档连「让分数偏一点」都不该参与。
	avgLen := float64(totalVisibleLen) / float64(nVisible)
	if avgLen == 0 {
		avgLen = 1
	}
	scores := make([]float64, len(ix.docs))
	for _, t := range terms {
		ps := ix.postings[t]
		df := 0
		for _, p := range ps {
			if visible[p.doc] {
				df++
			}
		}
		if df == 0 {
			continue
		}
		idf := math.Log(1 + (float64(nVisible)-float64(df)+0.5)/(float64(df)+0.5))
		for _, p := range ps {
			if !visible[p.doc] {
				continue
			}
			tf := float64(p.tf)
			denom := tf + bm25K1*(1-bm25B+bm25B*float64(ix.lens[p.doc])/avgLen)
			scores[p.doc] += idf * tf * (bm25K1 + 1) / denom
		}
	}
	idx := make([]int, 0, len(ix.docs))
	for i, s := range scores {
		if s > 0 {
			idx = append(idx, i)
		}
	}
	sort.SliceStable(idx, func(a, b int) bool {
		i, j := idx[a], idx[b]
		if scores[i] != scores[j] {
			return scores[i] > scores[j]
		}
		return ix.docs[i].Path < ix.docs[j].Path
	})
	if len(idx) > limit {
		idx = idx[:limit]
	}
	out := make([]Hit, 0, len(idx))
	for _, i := range idx {
		d := ix.docs[i]
		out = append(out, Hit{
			Path: d.Path, Title: d.Title, Domain: d.Domain,
			Score:   math.Round(scores[i]*10000) / 10000,
			Snippet: snippet(d.Body, terms),
			Updated: d.ModTime.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}

// Get 取一篇正文（指针读）。路径必须**在索引里**且**可见**：这同时挡住了目录穿越 ——
// 只有建索引时真正读过的相对路径才认。
func (ix *Index) Get(path string, sc Scope) (Doc, bool, error) {
	p := strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(path)), "./")
	for _, d := range ix.docs {
		if d.Path == p {
			if !sc.Allows(d.Domain) {
				return Doc{}, false, nil
			}
			return d, true, nil
		}
	}
	return Doc{}, false, nil
}

// LoadOptions 是「这次索引哪些目录」的配置。空 Roots = DefaultRoots。
// 这些是部署参数，不是 org 真相：换一台客户机器可以不同，不该写进 git 的 org。
type LoadOptions struct {
	Roots    []string
	MaxBytes int64
}

// DefaultRoots 是出厂索引面：已整理的知识目录 + 每个业务域的 data 目录。
//
// 故意**不含** 00-inbox（还没整理，进去只会把噪音变成答案）与 _originals（原件层）。
// `10-knowledge` 与 `knowledge` 都认：前者是 `anc init` 的骨架名，后者是存量客户库常见的叫法；
// **只收真存在的目录** —— 否则每个用另一种命名的库都会收到一条没意义的「目录不存在」。
func DefaultRoots(vault string, o *org.Org) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(filepath.ToSlash(p))
		p = strings.TrimPrefix(p, "./")
		if p == "" || p == "." || strings.HasPrefix(p, "../") || strings.Contains(p, "/../") || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, d := range []string{"10-knowledge", "knowledge"} {
		if info, err := os.Stat(filepath.Join(vault, filepath.FromSlash(d))); err == nil && info.IsDir() {
			add(d)
		}
	}
	if o != nil {
		for _, d := range o.Domains {
			p := strings.TrimSpace(filepath.ToSlash(d.Data))
			if p == "" {
				continue
			}
			if info, err := os.Stat(filepath.Join(vault, filepath.FromSlash(p))); err == nil && info.IsDir() {
				add(p)
			}
		}
	}
	return out
}

// Load 建索引。返回的 warnings 是「这一篇没读成 / 跳过了」的如实交代 —— 不是错误：
// 一篇坏文件不该让整次检索变成假红；但也不能静默，否则「没找到」可能其实是「没读到」。
func Load(vault string, o *org.Org, opt LoadOptions) (*Index, []string, error) {
	roots := opt.Roots
	if len(roots) == 0 {
		roots = DefaultRoots(vault, o)
	}
	maxBytes := opt.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	var (
		docs     []Doc
		warnings []string
	)
	if len(roots) == 0 {
		warnings = append(warnings, "没有可索引的目录（10-knowledge / knowledge / 各域 data 都不存在）—— 索引是空的，不代表没有内容")
	}
	for _, root := range roots {
		root = strings.TrimSpace(filepath.ToSlash(root))
		if root == "" {
			continue
		}
		base := filepath.Join(vault, filepath.FromSlash(root))
		if _, err := os.Stat(base); os.IsNotExist(err) {
			warnings = append(warnings, "索引目录不存在，跳过："+root)
			continue
		} else if err != nil {
			warnings = append(warnings, "索引目录读不了，跳过："+root+"（"+err.Error()+"）")
			continue
		}
		err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				warnings = append(warnings, "读不了，跳过："+relOf(vault, p)+"（"+err.Error()+"）")
				return nil
			}
			name := d.Name()
			if d.IsDir() {
				if p != base && (strings.HasPrefix(name, ".") || name == "_originals" || name == "node_modules") {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type()&fs.ModeSymlink != 0 {
				warnings = append(warnings, "符号链接不跟，跳过："+relOf(vault, p))
				return nil
			}
			if !isTextDoc(name) {
				return nil
			}
			info, ierr := d.Info()
			if ierr != nil {
				warnings = append(warnings, "读不了文件信息，跳过："+relOf(vault, p)+"（"+ierr.Error()+"）")
				return nil
			}
			if info.Size() > maxBytes {
				warnings = append(warnings, fmt.Sprintf("文件超过 %d 字节，跳过：%s", maxBytes, relOf(vault, p)))
				return nil
			}
			b, rerr := os.ReadFile(p)
			if rerr != nil {
				warnings = append(warnings, "读不了，跳过："+relOf(vault, p)+"（"+rerr.Error()+"）")
				return nil
			}
			rel := relOf(vault, p)
			fm, body := splitFrontmatter(string(b))
			title := strings.TrimSpace(fm["title"])
			if title == "" {
				title = firstHeading(body)
			}
			if title == "" {
				title = strings.TrimSuffix(name, filepath.Ext(name))
			}
			docs = append(docs, Doc{
				Path: rel, Domain: domainFor(o, rel, fm), Title: title,
				Body: body, ModTime: info.ModTime(),
			})
			return nil
		})
		if err != nil {
			return nil, warnings, err
		}
	}
	return New(docs), warnings, nil
}

func relOf(vault, p string) string {
	rel, err := filepath.Rel(vault, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

func isTextDoc(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown", ".txt":
		return true
	}
	return false
}

// domainFor 判一篇文档挂在哪个域。优先级：frontmatter 明写 > 路径顶层目录 → domains.data。
//
// frontmatter 写了空值（`domain:` 或 `domain: -`）= **公司级**，这是唯一一种
// 「显式覆盖路径推断」的写法；没写才看路径。
func domainFor(o *org.Org, rel string, fm map[string]string) string {
	if v, ok := fm["domain"]; ok {
		v = strings.TrimSpace(v)
		if v == "-" || v == "~" || v == "null" {
			return ""
		}
		return v
	}
	if o == nil {
		return ""
	}
	top := strings.SplitN(rel, "/", 2)[0]
	for _, d := range o.Domains {
		if d.Data != "" && d.Data == top {
			return d.Slug
		}
	}
	return ""
}

// splitFrontmatter 只认最简单的 `key: value` 行；不引 YAML 依赖。
// 第二个返回值是去掉 frontmatter 的正文。
func splitFrontmatter(s string) (map[string]string, string) {
	fm := map[string]string{}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return fm, s
	}
	rest := s[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return fm, s
	}
	head := rest[:end]
	body := rest[end+len("\n---"):]
	body = strings.TrimPrefix(body, "\n")
	for _, line := range strings.Split(head, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(k))
		val := strings.TrimSpace(v)
		if val == `""` || val == "''" {
			val = ""
		} else if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			// YAML 允许给标量加引号；不去掉的话 `domain: "trade"` 会被当成另一个域。
			val = val[1 : len(val)-1]
		}
		fm[key] = val
	}
	return fm, body
}

func firstHeading(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
	}
	return ""
}

// tokenize 是「零依赖中文可用」的分词：
//   - ASCII 字母 / 数字：整词小写（anc、q3、bm25）；
//   - 汉字：单字 + 相邻二元组（「结算」→ 结算；「合同执行」→ 合同/同执/执行）。
//
// 不做词性、不做同义词表 —— 那需要词典，而词典没有真相源；宁可命中得朴素，
// 也不要引一套会过期的隐式知识。
func tokenize(s string) []string {
	var out []string
	var word []rune
	var han []rune
	flushWord := func() {
		if len(word) > 0 {
			out = append(out, strings.ToLower(string(word)))
			word = word[:0]
		}
	}
	flushHan := func() {
		if len(han) == 0 {
			return
		}
		if len(han) == 1 {
			out = append(out, string(han))
		} else {
			for i := 0; i+1 < len(han); i++ {
				out = append(out, string(han[i:i+2]))
			}
		}
		han = han[:0]
	}
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Han, r):
			flushWord()
			han = append(han, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushHan()
			word = append(word, r)
		default:
			flushWord()
			flushHan()
		}
	}
	flushWord()
	flushHan()
	return out
}

func uniqueTokens(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range tokenize(s) {
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// snippet 找「命中最多的那一段」，截成一段可读的片段。
// 找不到命中段就取第一段正文 —— 至少让人知道这篇大概在讲什么，而不是只给一个路径。
func snippet(body string, terms []string) string {
	paras := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n\n")
	best, bestScore := "", -1
	for _, p := range paras {
		clean := cleanPara(p)
		if clean == "" {
			continue
		}
		low := strings.ToLower(clean)
		score := 0
		for _, t := range terms {
			if strings.Contains(low, t) {
				score++
			}
		}
		if score > bestScore {
			best, bestScore = clean, score
		}
	}
	if best == "" {
		best = cleanPara(body)
	}
	return clipRunes(best, snippetRunes)
}

func cleanPara(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "#>-*| ")
		lines[i] = line
	}
	return strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
