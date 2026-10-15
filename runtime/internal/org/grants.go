package org

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Grant 是一条授权（SPEC §6 授权模型：一张授权表，四种客体）。
//
// **一个 grant 一个文件**，落在 <vault>/grants/ 下。扫描是**递归**的 —— 目录怎么分层是给
// 人看的（快速定位），不是给代码看的：换分层不用改代码。字段就是 SPEC §6 那七个，
// 一条不多：from / to / action / object / ttl / on_behalf_of / reason。
//
// 表**默认零条**：没有 grants/ 目录 = 还没有任何授权，不算错（存量 vault 零影响）。
// 「默认零条」是口径不是缺陷（SPEC §6 不变量 1：没有 grant 就是不通）。
//
// 这一层只做**结构**校验。语义校验只做到「from 解得出 actor」—— to / object 的客体词表
// （通道、看板分区）还没有定义处，现在校验它们只能是猜。见 DESIGN.md §7.1.15。
type Grant struct {
	Slug       string // 文件名（不含 .md）—— 人用来定位这一条
	Path       string // 相对 vault 根的路径（POSIX 分隔符）
	From       string // 谁给的（actor：成员 / 岗位，可带 member: role: 前缀）
	To         string // 给谁：actor 或客体
	Action     string // read | write | invoke
	Object     string // 客体标识
	TTL        string // 期限：**可选；空 = 永久**（SPEC §6 / Q25，2026-10-10 改口径）。原样透出，形态由执行层定
	OnBehalfOf string // 署名：bot 发出的必须带（它不自有权限，只是代理对象的投影）
	Reason     string // 一句话：为什么给
	Body       string // 正文（自由文本，给人写「展开」的地方）
}

// GrantsDir 是授权表的落点（vault 顶层）。
//
// 它是**控制面真相源**，而这里的 vault 是**真相源仓库**，不是 bot 的工作区
// （SPEC §3「真相源仓库 ≠ bot 的工作区」；bot 的 cwd 是渲染产物）。所以它落在这里是对的，
// 前提两条：① 渲染期过滤**不许**把它复制进任何 bot 的工作目录；
// ② 它不进 persona 段 3 的路由表（见 skipDirs）—— 路由表回答「去哪找业务资料」，授权表不是资料。
const GrantsDir = "grants"

// knownGrantActions 是认得的动作词表。不在表里的照收、只报出来（词表不锁死，同 envelope.kind）。
var knownGrantActions = map[string]bool{"read": true, "write": true, "invoke": true}

// ignoredGrantFiles 是落点里的说明文件：写给人看的，不是授权条目。
// 与 charters/ 同一约定（那里的 CLAUDE.md 也不当条目）。
var ignoredGrantFiles = map[string]bool{"CLAUDE.md": true, "README.md": true}

// ScanGrants 扫授权表。判据：**有 frontmatter 才算条目**。
//
// 说明文件按名字跳过；名字不在忽略名单里、又没有合法 frontmatter 的，**报出来** ——
// 静默丢掉一条授权，比报一条错危险得多（spec 的「宁可拒绝，不做静默猜测」）。
// 目录不在 = 还没有授权，不算错。
func ScanGrants(root string, p *Policy) ([]Grant, []Issue, error) {
	base := filepath.Join(root, GrantsDir)
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return nil, nil, nil
	}
	var (
		out    []Grant
		issues []Issue
	)
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != base && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".md") || strings.HasPrefix(name, ".") || ignoredGrantFiles[name] {
			return nil
		}
		g, gIssues := readGrant(path, relSlash(root, path), p)
		if g != nil {
			out = append(out, *g)
		}
		issues = append(issues, gIssues...)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, issues, nil
}

// readGrant 读一个授权文件。返回 nil = 这不是一个能读的条目（发现已记进 issues）。
func readGrant(path, rel string, p *Policy) (*Grant, []Issue) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, []Issue{p.Issue("grant.file.unparsable", rel, "读不动这个文件：%v", err)}
	}
	slug := strings.TrimSuffix(filepath.Base(path), ".md")
	var issues []Issue
	if !reID.MatchString(slug) {
		issues = append(issues, p.Issue("grant.slug.format", rel,
			"文件名 %q 不是小写 ASCII 标识（%s）—— 它要进路径与代码，中文会撞 macOS 的 NFC/NFD", slug, reID))
	}
	doc, perr := Parse(string(b))
	if perr != nil {
		return nil, append(issues, p.Issue("grant.file.unparsable", rel,
			"读不出授权条目：%s —— 一个 grant 一个文件，必须是 markdown + frontmatter（首行 `---`）", perr))
	}

	// 一个文件里塞了两条：第二段 frontmatter 的字段一个都读不到 —— 发现了必须报，
	// 否则「复制一条时粘多了」= 一条授权静默消失（正是本包最想避免的那类错）。
	if ln := secondFrontmatterLine(doc.Body); ln > 0 {
		issues = append(issues, p.Issue("grant.file.multiple", rel,
			"正文第 %d 行起还有一段 `grant.` frontmatter —— 一个 grant 一个文件；第二段不会被读，等于一条授权静默消失", ln))
	}

	g := &Grant{Slug: slug, Path: rel, Body: doc.Body}
	// 取字段：类型不符（多半写成了数组）单独报一条，别让它悄悄变空串。
	field := func(key string) string {
		v, err := doc.Str("grant." + key)
		if err != nil {
			issues = append(issues, p.Issue("grant.file.unparsable", rel, "%s", err))
			return ""
		}
		return strings.TrimSpace(v)
	}
	g.From, g.To, g.Action = field("from"), field("to"), field("action")
	g.Object, g.TTL = field("object"), field("ttl")
	g.OnBehalfOf, g.Reason = field("on_behalf_of"), field("reason")

	// 四个必填字段。**期限不在必填之列**（SPEC §6 / Q25，2026-10-10 改口径：`ttl` 可选、不写 = 永久）。
	// `grant.ttl.missing` 这条发现**照样发**，只是出厂档是 off（收集时被丢掉）—— 这样客户想要
	// 「授权必带期限」时，在 company.md 里开一下就到，而不是回来改代码。
	for _, f := range []struct {
		key, rule string
		val       string
	}{
		{"from", "grant.from.missing", g.From},
		{"to", "grant.to.missing", g.To},
		{"action", "grant.action.missing", g.Action},
		{"object", "grant.object.missing", g.Object},
		{"ttl", "grant.ttl.missing", g.TTL},
	} {
		if f.val == "" {
			issues = append(issues, p.Issue(f.rule, rel,
				"缺 %s —— 在 `grant:` 段里补一行 `%s: …`（写法见 DESIGN.md §7.1.15）", f.key, f.key))
		}
	}
	if g.Action != "" && !knownGrantActions[g.Action] {
		issues = append(issues, p.Issue("grant.action.unknown", rel,
			"action=%q 不在 read | write | invoke —— 照收，只是执行层认不出来", g.Action))
	}
	return g, issues
}

// validateGrants 跨表校验：from 必须解得出 actor（成员或岗位）。
//
// 只发现事实，档次由规则表定（实现里不许硬编码档次）。两处刻意留白：
//   - 不查 to / object：通道（#33）与看板分区（#7）的词表还没有定义处，现在校验只能是猜；
//   - 不管「这条权该不该给」：那是人的事（提案 + 人确认），不是校验器的事。
//
// 为什么 from 不许是域：域是**客体**不是主体 —— 域不会授权（SPEC §6「谁给的：自己必须先有这个能力」）。
func (o *Org) validateGrants(p *Policy, rep *Report) {
	for _, g := range o.Grants {
		if g.From == "" {
			continue // 缺字段已在扫描层报过，这里不报第二遍
		}
		if !o.hasActor(g.From) {
			rep.Add(p.Issue("grant.from.unknown", g.Path,
				"from=%q 解不出成员或岗位（写法：member:名字 | role:岗位 | 裸名字）—— 授权者不存在，这条链是断的", g.From))
		}
	}
}

// hasActor 判这个引用指不指向「一个能授权的主体」：成员或岗位。
//
// 裸名字按固定顺序解（成员 → 岗位）—— 顺序写死是为了同一份输入永远同一个答案，同 on_behalf_of。
func (o *Org) hasActor(ref string) bool {
	kind, name := SplitRef(ref)
	if name == "" {
		return false
	}
	switch kind {
	case "member":
		_, ok := o.Member(name)
		return ok
	case "role":
		_, ok := o.Roles[name]
		return ok
	case "":
		if _, ok := o.Member(name); ok {
			return true
		}
		_, ok := o.Roles[name]
		return ok
	default:
		return false
	}
}

// secondFrontmatterLine 找正文里**第二段** frontmatter 的起始行（1 起算，找不到返回 0）。
//
// 判据收得很紧：一行正好是 `---`，紧跟的第一行非空非注释内容以 `grant.` 开头。
// 只认这一种形态，是因为正文里出现横线（分隔线、表格）是常态，出现 `grant.from: …` 不是 ——
// 松一点就会把正常的说明文字报成错，那比漏报还烦人。
func secondFrontmatterLine(body string) int {
	lines := strings.Split(body, "\n")
	for i, raw := range lines {
		if strings.TrimSpace(raw) != "---" {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			t := strings.TrimSpace(lines[j])
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			if strings.HasPrefix(t, "grant.") {
				return i + 1
			}
			break
		}
	}
	return 0
}

// relSlash 取相对 vault 根的路径，并统一成 POSIX 分隔符（进看板 / 审计的路径要跨平台一致）。
func relSlash(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	return filepath.ToSlash(rel)
}

// ---- 「这条授权与我相关吗」—— 一把共用的尺子 ----
//
// 2026-10-16（议题 #68）：授权这条路上有**两个消费者** —— 看板上的人（全量 + 视角）与
// bot 侧那个「只出与我相关的几条」的视图。两个视图答的是**同一个问题**（这条权是不是指着你），
// 所以尺子只能有一把：各写一份，迟早出现「看板说这条与他相关、出口说无关」。
//
// 这一节只做**判据**，不做展示：看板要把引用解成「岗位 / 域 / 认不出」，出口只回原样字段，
// 那是各自的台面，不在这把尺子里。

// GrantSlot 是一条授权里**会指名道姓**的一个格子：原文照留，不解释。
type GrantSlot struct {
	Slot string // from | to | on_behalf_of
	Raw  string
}

// GrantSlots 摊出一条授权里会指着主体的那几格。
//
// SPEC §6 那七个字段里只有这三格指人。**空格子不出现** ——「没写」与「写了但认不出」
// 是两件事：后者要留给人看见（看板把那格标出来），前者没什么可看的。
func GrantSlots(g Grant) []GrantSlot {
	out := make([]GrantSlot, 0, 3)
	for _, s := range []GrantSlot{
		{Slot: "from", Raw: g.From},
		{Slot: "to", Raw: g.To},
		{Slot: "on_behalf_of", Raw: g.OnBehalfOf},
	} {
		if strings.TrimSpace(s.Raw) == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

// IdentityForRef 把一个格子里的写法解成**主体**（成员 / 业务 agent）。
//
// 与 Identity 同一把尺子（先成员、后业务 agent，顺序写死），只多一层前缀处理：
// 裸名字 / `member:` / `agent:` 才当主体；`role:` 与 `domain:` **明确不是主体** ——
// 岗位是「一类人」而不是一个编号身份，域是**客体**（SPEC §6：域不会授权）。
//
// 于是一条授权写 `to: role:manager` 时，它**不**命中任何一个具体人的「与我相关」——
// 这不是漏判，是「这条权给的是一个岗位，不是这个编号」。要判「我在不在这个岗位上」，
// 那是另一件事（岗位 → 人的展开在渲染那边），不混进这条判据。
func (o *Org) IdentityForRef(raw string) (Identity, bool) {
	kind, name := SplitRef(raw)
	switch kind {
	case "", "member", "agent":
		return o.Identity(name)
	default:
		return Identity{}, false
	}
}

// GrantHitsSubject 判一条授权是不是「与这个主体相关」。
//
// 判据只有一条：三格里任意一格解出来是**同一个编号身份**。看板的视角与 bot 侧的视图共用它。
// 比的是**解出来的**编号（kind + ref），不是请求里那个拼法 —— 同身份那条纪律。
func GrantHitsSubject(o *Org, g Grant, idn Identity) bool {
	if o == nil || strings.TrimSpace(idn.Ref) == "" {
		return false
	}
	for _, s := range GrantSlots(g) {
		other, ok := o.IdentityForRef(s.Raw)
		if ok && other.Kind == idn.Kind && other.Ref == idn.Ref {
			return true
		}
	}
	return false
}
