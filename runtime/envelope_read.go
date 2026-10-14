package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"anc/internal/audit"
	"anc/internal/envelope"
	"anc/internal/org"
	renderpkg "anc/internal/render"
)

// ---------- 出站读出口（W2）：桥的另一半 ----------
//
// 入站是 agent **递**给 ANC（anc_send_envelope）；出站是 agent 向 ANC **要**上下文
// （anc_read_context）—— **只读**：一个字都不写回真相源。
//
// 三件事全部复用既有件，一件都不新造（各造一套迟早会对不上号）：
//
//  1 **可见范围**：renderpkg.VisibleDomains —— 自己的域全行 / 别人的域只给「找谁」。
//     persona 段 8 用的是同一把尺子。
//  2 **身份**：与信封同一条口径（envelope.Resolves）—— 解不出**不是格式错，是这个人不存在**。
//  3 **留痕**：audit.Append 一条（谁 / 要什么 / 给没给 / 为什么拒）。审计包自己写着
//     「出站动作由人 / agent 显式补记」—— 这里就是那条出站动作的自动落点。
//
// 回话是 **JSON**，字段就是契约，多一个都不给。这不只是好看：它同时是
// 「出站不泄漏本机路径与自由文本」的**结构性**保证 —— 只回契约字段，指针一律 vault 相对。

// readContextOut 是 anc_read_context 回给 harness 的那一份。
type readContextOut struct {
	Who        string           `json:"who,omitempty"`          // 解出来的身份**编号**（成员名 / 业务 agent 的 slug）
	Identity   *readIdentityOut `json:"identity,omitempty"`     // 你是谁 / 从哪来 / 干什么的 —— 光一个编号分辨不出来
	OnBehalfOf string           `json:"on_behalf_of,omitempty"` // 署名（原样回，便于人核对）
	Scope      string           `json:"scope,omitempty"`        // 实际答的域；空 = 你负责的全部
	Domains    []readDomainOut  `json:"domains,omitempty"`      // 你负责的域：全行（拒的时候不出现）
	Directory  []readBriefOut   `json:"directory,omitempty"`    // 别域目录：只有名称 / 是什么 / 找谁
	Skills     []readSkillOut   `json:"skills,omitempty"`       // 岗位声明的技能（正文在不在，逐条标）
	Projects   []readProjectOut `json:"projects,omitempty"`     // 挂在你可见域上的项目
	Gaps       []string         `json:"gaps,omitempty"`         // 这份上下文答不了什么（报缺，不编）
	Note       string           `json:"note,omitempty"`         // 只在「还没给你划域」时出现
	Refused    *readRefusalOut  `json:"refused,omitempty"`      // 拒：不给数据，给理由（能指路就指路）
	At         string           `json:"at"`                     // 回答时刻（RFC3339）
}

// readIdentityOut 是主体的**身份**：编号 + 归属 + 铭文。
//
// 为什么单列一块而不是只回一个名字：成员 bot 与业务 agent 接在同一张口上，
// 光一个编号分辨不出「这是谁、替哪个部门干活、干什么的」。三格全部来自真相源
// （编号不许自报这条纪律的延伸）：归属取域表的名字，铭文取 agents.md 的 description，
// 并附一条**能回去核对**的出处指针。
type readIdentityOut struct {
	Kind        string        `json:"kind"`                  // member / agent（失败代价不同，回话里要能分开）
	Ref         string        `json:"ref"`                   // **编号身份**：成员名 / 业务 agent 的 slug
	Name        string        `json:"name,omitempty"`        // 显示名
	Role        string        `json:"role,omitempty"`        // 代哪个岗位（persona / 技能从这一层来）
	Domain      string        `json:"domain,omitempty"`      // **归属**：主域的 slug（业务 agent 就是它那一个）
	DomainName  string        `json:"domain_name,omitempty"` // 归属：给人看的域名字
	Inscription string        `json:"inscription,omitempty"` // **铭文**：干什么的 / 替谁干
	Evidence    *readEvidence `json:"evidence,omitempty"`    // 铭文的出处（agents.md 第 n 行）；成员不出现
}

// identityOf 把解出来的主体摊成回话里那一块。铭文出处只给业务 agent —— 成员 bot 没有 agents.md 那一行。
func identityOf(o *org.Org, idn org.Identity) *readIdentityOut {
	out := &readIdentityOut{
		Kind: idn.Kind, Ref: idn.Ref, Name: idn.Name, Role: idn.Role,
		Inscription: idn.Inscription,
	}
	if len(idn.Domains) > 0 {
		out.Domain = idn.Domains[0]
		for _, d := range o.Domains {
			if d.Slug == out.Domain {
				out.DomainName = d.Name
				break
			}
		}
	}
	if idn.Kind == org.KindAgent && idn.Line > 0 {
		out.Evidence = &readEvidence{File: org.AgentsFile, Line: idn.Line}
	}
	return out
}

// readDomainOut 是「自己的域」那一行的全貌：域是什么 / 数据在哪 / 找谁 / 证据指针。
type readDomainOut struct {
	Slug     string        `json:"slug"`
	Name     string        `json:"name"`
	What     string        `json:"what"`
	Data     string        `json:"data,omitempty"`
	Who      string        `json:"who"`
	Terms    string        `json:"terms,omitempty"`
	Sources  string        `json:"sources,omitempty"`
	Evidence *readEvidence `json:"evidence"`
}

// readBriefOut 是别域的可见面：名称 / 是什么 / 找谁。**没有 data、没有 terms** ——
// 要跨域就按「找谁」接头，走人，不走近道（SPEC §6「数据默认不通」）。
type readBriefOut struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	What string `json:"what"`
	Who  string `json:"who"`
}

// readEvidence 是指回真相源的指针：**vault 相对**路径 + 行号 —— 绝不出现本机绝对路径。
type readEvidence struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// readSkillOut 是「技能从哪来」：清单在 roles/<role> 的 skills:，
// 正文在 skills/<name>/SKILL.md。**声明了但正文不在 = missing** —— 报缺，不编。
type readSkillOut struct {
	Name    string `json:"name"`
	Where   string `json:"where"`             // vault 相对指针；本机绝对路径一个字都不出现
	Missing bool   `json:"missing,omitempty"` // true = 清单里声明了，正文还没落
}

// readProjectOut 是「在跟哪个项目」：projects.md 的一行。
// period / source **原样搬** —— 它们是指针（真源在别处），ANC 不解析、也不规定格式。
type readProjectOut struct {
	Slug     string        `json:"slug"`
	Name     string        `json:"name"`
	Owner    string        `json:"owner,omitempty"`
	Period   string        `json:"period,omitempty"`
	Source   string        `json:"source,omitempty"`
	Evidence *readEvidence `json:"evidence"`
}

type readRefusalOut struct {
	Reason string `json:"reason"`
	Ask    string `json:"ask,omitempty"` // 该找谁（能指路就指路）
}

// readContext 是工具实现：解身份 → 按可见范围组一份 JSON → 落一条痕。
//
// 装配那一半在 assembleContext（**唯一一个装配点** —— `anc context pack` 走的是同一个函数）；
// 这里只多做两件事：真相源读不出来时的报错口径，和每一次出站调用的留痕。
//
// isErr=true 的三种情形都是「没办成」而不是「工具坏了」：这个人不存在 / on_behalf_of 解不出 /
// 要的是别人的域。三种都留痕 —— 被拒的那次同样是「一次想行使」。
func (g *envelopeIngress) readContext(args map[string]any) (string, bool) {
	now := g.now()
	who := strArg(args, "who")
	onBehalf := strArg(args, "on_behalf_of")
	ask := safeToken(strArg(args, "domain"))

	o, err := org.Load(g.Vault)
	if err != nil {
		// 真相源本身是红的：读不了就不编一份绿灯（同 envelope check 的口径）。
		return "真相源加载失败，读不了：" + err.Error() + "\n（先修 org：anc org check <vault>）", true
	}

	out, refused := assembleContext(o, g.Vault, who, onBehalf, ask, now)

	// 留痕：谁 / 要什么 / 给没给 / 为什么拒。被拒的那次同样是「一次想行使」。
	// 身份都没解出来那一次，账记在「报上来的那个名字」（清洗后）或 unknown 上 —— 一次都不许静默不记。
	actor := out.Who
	if actor == "" {
		actor = readActor(who)
	}
	if refused {
		g.auditRead(actor, ask, audit.ResultDenied, out.Refused.Reason, now)
	} else {
		g.auditRead(actor, ask, audit.ResultOK, "", now)
	}
	return mustJSON(out), refused
}

// assembleContext 是「一个 agent 的工作上下文」**唯一一个装配点**。
//
// 两个出口共用它：MCP 的 anc_read_context（agent 来要）与 `anc context pack`（人看 / 交付要一份冻结副本）。
// 装配只有一份，才不会出现「MCP 说的工作上下文」与「CLI 打的那一份」对不上号 ——
// 同一把尺子（VisibleDomainsOf）、同一套身份解析（o.Identity）、同一份报缺口径。
//
// 它只做装配：不加载真相源、不留痕、不打印 —— 那三样两个出口各有各的交代方式。
// 回 (这一份上下文, 是不是拒了)。拒了的时候 out.Refused 有理由，数据一格都没有。
func assembleContext(o *org.Org, vault, who, onBehalf, ask string, now time.Time) (readContextOut, bool) {
	// 身份：解不出（含压根没给）→ **当这个人不存在**。所以理由里不许出现「参数错误」
	// 这类话 —— 那会把读的人引去改参数，而问题是他不在这家公司（envelope 包注释第 1 条）。
	subj, ok := o.Identity(who)
	if !ok {
		reason := "我们公司没有这个人 —— 身份不许自报：who 必须能在 members/ 或 agents.md 里解出来"
		return readContextOut{
			Refused: &readRefusalOut{Reason: reason},
			At:      now.Format(time.RFC3339),
		}, true
	}

	// 署名（可选）：给了就必须解得出来 —— 同一套身份解析（envelope.Resolves），不另开一套。
	if onBehalf != "" && !envelope.Resolves(o, onBehalf) {
		reason := "on_behalf_of 解不出 —— 写 member:名字 | role:岗位 | domain:slug，或直接写名字"
		return readContextOut{
			Who: subj.Ref, OnBehalfOf: onBehalf,
			Refused: &readRefusalOut{Reason: reason},
			At:      now.Format(time.RFC3339),
		}, true
	}

	mine, others := renderpkg.VisibleDomainsOf(o, subj.Domains)

	// 不写 domain = 你负责的全部域（外加「找谁」目录）；写了 = 只要那一行。
	var chosen []renderpkg.DomainRow
	if ask == "" {
		chosen = mine
	} else {
		for _, d := range mine {
			if d.Slug == ask {
				chosen = []renderpkg.DomainRow{d}
			}
		}
		if len(chosen) == 0 {
			reason, helper := refuseDomain(o, ask)
			return readContextOut{
				Who: subj.Ref, OnBehalfOf: onBehalf,
				Refused: &readRefusalOut{Reason: reason, Ask: helper},
				At:      now.Format(time.RFC3339),
			}, true
		}
	}

	out := readContextOut{Who: subj.Ref, Identity: identityOf(o, subj), OnBehalfOf: onBehalf, At: now.Format(time.RFC3339)}
	if ask != "" {
		out.Scope = ask
	}
	out.Domains = domainRows(chosen)
	if ask == "" {
		out.Directory = briefRows(others)
		if len(chosen) == 0 {
			if subj.Kind == org.KindAgent {
				out.Note = "这个业务 agent 还没有域（agents.md 的 domain 列指不到 domains.md 里的行）—— 没有域就没有数据视野"
			} else {
				out.Note = "还没给你划域（在 members/" + subj.Ref + "/persona.md 的 domains 里写）"
			}
		}
	}

	// 「工作上下文」的另两问：技能从哪来、在跟哪个项目。
	// 都走已经算好的 chosen（可见范围）—— 项目按域过滤，域不在范围里就不出现。
	out.Skills = skillsOf(o, subj.Role, vault)
	out.Projects = projectRows(o, chosen)
	// 报缺只在要「全部」时给：要单独一域的人不该被一段通用说明淹掉。
	if ask == "" {
		out.Gaps = contextGaps(out)
	}
	return out, false
}

// refuseDomain 组「这块业务不在你的可见范围」这句拒话。两种情况分开：
// 域表里有这个域 → 指路「找谁」（这才是让人不必再问人的那一步）；压根没有 → 明说没有，别编一个谁。
func refuseDomain(o *org.Org, ask string) (reason, helper string) {
	for _, d := range o.Domains {
		if d.Slug != ask {
			continue
		}
		who := o.WhoLabel(d.Who)
		if who == "—" {
			who = ""
		}
		return renderpkg.RefuseDomainText(ask, who), who
	}
	return "domains.md 里没有「" + ask + "」这块业务 —— 要么拼错了，要么这一域还没抽成表行", ""
}

func domainRows(rows []renderpkg.DomainRow) []readDomainOut {
	out := make([]readDomainOut, 0, len(rows))
	for _, d := range rows {
		out = append(out, readDomainOut{
			Slug: d.Slug, Name: d.Name, What: d.What, Data: d.Data, Who: d.Who,
			Terms: d.Terms, Sources: d.Sources,
			Evidence: &readEvidence{File: org.DomainsFile, Line: d.Line},
		})
	}
	return out
}

// skillsOf 把这个成员**岗位**声明的技能摊出来，并逐条去看正文在不在。
// 只有清单、没有正文 = missing：报缺，不编（同 persona 的诚实条款）。
func skillsOf(o *org.Org, roleSlug, vault string) []readSkillOut {
	role, ok := o.Roles[roleSlug]
	if !ok {
		return nil
	}
	out := make([]readSkillOut, 0, len(role.Skills))
	for _, name := range role.Skills {
		if strings.TrimSpace(name) == "" {
			continue
		}
		where := "skills/" + name + "/SKILL.md" // 指针一律正斜杠：vault 里的形状，不是本机路径
		_, err := os.Stat(filepath.Join(vault, "skills", name, "SKILL.md"))
		out = append(out, readSkillOut{Name: name, Where: where, Missing: err != nil})
	}
	return out
}

// projectRows 把**挂在这些域上**的项目摊出来。域不在可见范围里 = 不出现（与域行同一条纪律：
// 别人的域连项目名都不给，只给「找谁」）。owner 走与域行同一个 WhoLabel（岗位 → 人名）。
func projectRows(o *org.Org, chosen []renderpkg.DomainRow) []readProjectOut {
	inScope := make(map[string]bool, len(chosen))
	for _, d := range chosen {
		inScope[d.Slug] = true
	}
	var out []readProjectOut
	for _, p := range o.Projects {
		if !inScope[p.Domain] {
			continue
		}
		owner := o.WhoLabel(p.Owner)
		if owner == "—" {
			owner = ""
		}
		out = append(out, readProjectOut{
			Slug: p.Slug, Name: p.Name, Owner: owner,
			Period: p.Period, Source: p.Source,
			Evidence: &readEvidence{File: org.ProjectsFile, Line: p.Line},
		})
	}
	return out
}

// contextGaps 明说「这份上下文答不了什么」。**只列没有真相源的那几样** ——
// 不编、也不假装完整：读的人据此知道该去问谁，而不是以为这就是全部。
func contextGaps(out readContextOut) []string {
	var g []string
	if len(out.Skills) == 0 {
		g = append(g, "技能：这个岗位在 roles/<role> 里没声明 skills（不是「没有技能」，是「没登记」）")
	}
	for _, s := range out.Skills {
		if s.Missing {
			g = append(g, "技能正文还没落："+s.Where)
		}
	}
	if len(out.Projects) == 0 {
		g = append(g, "项目：你可见的域上还没挂项目（projects.md 里没有，或还没建这张表）")
	}
	g = append(g,
		"新鲜度：某块上下文最近一次从外部渠道更新是什么时候 —— 没有这个真相源，答不了",
		"版本：改一处上下文，谁的工作上下文跟着变了 —— 只有 persona 指纹那一半",
	)
	return g
}

func briefRows(rows []renderpkg.DomainBrief) []readBriefOut {
	if len(rows) == 0 {
		return nil
	}
	out := make([]readBriefOut, 0, len(rows))
	for _, d := range rows {
		out = append(out, readBriefOut{Slug: d.Slug, Name: d.Name, What: d.What, Who: d.Who})
	}
	return out
}

// auditRead 给一次出站调用留一条痕：谁 / 要什么 / 给没给 / 为什么拒。
//
// 留痕失败**不改回话**：读出口的职责是答问题，不是把审计故障变成对 harness 的报错。
// 但要在 stderr 吼一声 —— 静默丢一条痕，等于审计少一次行使。
func (g *envelopeIngress) auditRead(actor, domain, result, why string, at time.Time) {
	if actor == "" {
		actor = "unknown"
	}
	rec := audit.Record{
		ID:     fmt.Sprintf("read-%d-%s", at.UnixNano(), audit.ShardName(actor)),
		At:     at.Format(time.RFC3339),
		Actor:  actor,
		Action: audit.ActionRead,
		Object: domain,
		Result: result,
		Why:    why,
		Tool:   "anc_read_context",
		Source: "serve",
	}
	if _, err := audit.Append(g.Vault, rec); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  出站调用没留上痕（audit）：%v\n", err)
	}
}

// readActor 决定「解不出的那个 who」往谁的账上记：用请求里报上来的名字（清洗后），空则 unknown。
// 一次都不许静默不记 —— 「没有身份」不是「没有发生」。
func readActor(claimed string) string {
	if s := safeToken(claimed); s != "" {
		return s
	}
	return "unknown"
}

// safeToken 把请求里的自由文本压成一条能落盘的短串：去控制字符、压成单行、封顶 64 字。
// 落进痕里的是它 —— 请求原文不落盘（审计流水不是给人塞私货的地方，更不是给注入留的一条通道）。
func safeToken(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 64 {
		s = string(r[:64]) + "…"
	}
	return s
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		// 结构是我们自己定的，序列化不了只可能是编程错误 —— 别把 panic 扔给 harness。
		return `{"refused":{"reason":"内部错误：回话序列化失败"}}`
	}
	return string(b)
}
