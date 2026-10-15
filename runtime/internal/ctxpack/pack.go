// Package ctxpack 装配「一个 agent 的工作上下文」—— 全平台**唯一一个装配点**。
//
// 为什么单开一个包：这份装配有三个消费方（MCP 的 anc_read_context、CLI 的 `anc context pack`、
// 看板的一页），而它原先长在 package main 里 —— 看板（internal/board）够不着。
// 留在 package main 只剩两条路：各写一份（迟早对不上号），
// 或者搬到一个三边都 import 得到的地方。这里是后者。
//
// 它只做装配：不加载真相源、不留痕、不打印、不落盘 ——
// 那几样各出口各有各的交代方式（见 runtime/envelope_read.go 与 runtime/context.go）。
package ctxpack

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"anc/internal/envelope"
	"anc/internal/hot"
	"anc/internal/org"
	renderpkg "anc/internal/render"
)

// Out 是「一个 agent 的工作上下文」这份投影的形状 —— MCP 回给 harness 的那一份。
//
// 回话是 **JSON**，字段就是契约，多一个都不给。这不只是好看：它同时是
// 「出站不泄漏本机路径与自由文本」的**结构性**保证 —— 只回契约字段，指针一律 vault 相对。
type Out struct {
	Who        string       `json:"who,omitempty"`          // 解出来的身份**编号**（成员名 / 业务 agent 的 slug）
	Identity   *IdentityOut `json:"identity,omitempty"`     // 你是谁 / 从哪来 / 干什么的 —— 光一个编号分辨不出来
	OnBehalfOf string       `json:"on_behalf_of,omitempty"` // 署名（原样回，便于人核对）
	Scope      string       `json:"scope,omitempty"`        // 实际答的域；空 = 你负责的全部
	Domains    []DomainOut  `json:"domains,omitempty"`      // 你负责的域：全行（拒的时候不出现）
	Directory  []BriefOut   `json:"directory,omitempty"`    // 别域目录：只有名称 / 是什么 / 找谁
	Skills     []SkillOut   `json:"skills,omitempty"`       // 岗位声明的技能（正文在不在，逐条标）
	Projects   []ProjectOut `json:"projects,omitempty"`     // 挂在你可见域上的项目
	Todo       []TaskOut    `json:"todo,omitempty"`         // 派给我的待办（热层 to == 我）；只读，不在这儿改状态
	Doing      []TaskOut    `json:"doing,omitempty"`        // 我正在做的（热层 by == 我）
	Grants     *GrantsOut   `json:"grants,omitempty"`       // 我被允许做什么 —— **只出与我相关的几条**（bot 侧那个视图，议题 #68）
	Gaps       []string     `json:"gaps,omitempty"`         // 这份上下文答不了什么（报缺，不编）
	Note       string       `json:"note,omitempty"`         // 只在「还没给你划域」时出现
	Refused    *Refusal     `json:"refused,omitempty"`      // 拒：不给数据，给理由（能指路就指路）
	At         string       `json:"at"`                     // 回答时刻（RFC3339）
}

// IdentityOut 是主体的**身份**：编号 + 归属 + 铭文。
//
// 为什么单列一块而不是只回一个名字：成员 bot 与业务 agent 接在同一张口上，
// 光一个编号分辨不出「这是谁、替哪个部门干活、干什么的」。三格全部来自真相源
// （编号不许自报这条纪律的延伸）：归属取域表的名字，铭文取 agents.md 的 description，
// 并附一条**能回去核对**的出处指针。
type IdentityOut struct {
	Kind        string    `json:"kind"`                  // member / agent（失败代价不同，回话里要能分开）
	Ref         string    `json:"ref"`                   // **编号身份**：成员名 / 业务 agent 的 slug
	Name        string    `json:"name,omitempty"`        // 显示名
	Role        string    `json:"role,omitempty"`        // 代哪个岗位（persona / 技能从这一层来）
	Domain      string    `json:"domain,omitempty"`      // **归属**：主域的 slug（业务 agent 就是它那一个）
	DomainName  string    `json:"domain_name,omitempty"` // 归属：给人看的域名字
	Inscription string    `json:"inscription,omitempty"` // **铭文**：干什么的 / 替谁干
	Evidence    *Evidence `json:"evidence,omitempty"`    // 铭文的出处（agents.md 第 n 行）；成员不出现
}

// DomainOut 是「自己的域」那一行的全貌：域是什么 / 数据在哪 / 找谁 / 证据指针。
type DomainOut struct {
	Slug     string    `json:"slug"`
	Name     string    `json:"name"`
	What     string    `json:"what"`
	Data     string    `json:"data,omitempty"`
	Who      string    `json:"who"`
	Terms    string    `json:"terms,omitempty"`
	Sources  string    `json:"sources,omitempty"`
	Evidence *Evidence `json:"evidence"`
}

// BriefOut 是别域的可见面：名称 / 是什么 / 找谁。**没有 data、没有 terms** ——
// 要跨域就按「找谁」接头，走人，不走近道（SPEC §6「数据默认不通」）。
type BriefOut struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	What string `json:"what"`
	Who  string `json:"who"`
}

// Evidence 是指回真相源的指针：**vault 相对**路径 + 行号 —— 绝不出现本机绝对路径。
type Evidence struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// SkillOut 是「技能从哪来」：清单在 roles/<role> 的 skills:，
// 正文在 skills/<name>/SKILL.md。**声明了但正文不在 = missing** —— 报缺，不编。
type SkillOut struct {
	Name    string `json:"name"`
	Where   string `json:"where"`             // vault 相对指针；本机绝对路径一个字都不出现
	Missing bool   `json:"missing,omitempty"` // true = 清单里声明了，正文还没落
}

// ProjectOut 是「在跟哪个项目」：projects.md 的一行。
// period / source **原样搬** —— 它们是指针（真源在别处），ANC 不解析、也不规定格式。
type ProjectOut struct {
	Slug     string    `json:"slug"`
	Name     string    `json:"name"`
	Owner    string    `json:"owner,omitempty"`
	Period   string    `json:"period,omitempty"`
	Source   string    `json:"source,omitempty"`
	Evidence *Evidence `json:"evidence"`
}

// TaskOut 是「一件进行中的事」在一个人眼里长什么样 —— ANC 侧那个任务本体（住热层，见 internal/hot）。
//
// 它**不是真相源**，也不是这份上下文现有的那几问之一：它是**反向下发**的那一步。
// ANC 不能主动往 agent 的会话里插话（插了就是打断：上一条还没跑完，第二条已经在改前提了），
// 所以「派活」拆成两件**各自可替换**的事：**决定派给谁**（ANC 的活，落在热层）+
// **让对方开始动**（对方自己的活 —— 它在自己的会话里 pull，pull 到了才开始）。
// 这一块就是那个 pull：agent 会话一开始要一次上下文，「派给我的」就在里面。
//
// 状态**原样给**：热层里 `status` 是什么就摆什么（done / running / blocked / failed，认不出的照收）。
// 哪些算「还没做完」是业务口径 —— 不在这层判（同 hot 包「认不出的照收」）。
type TaskOut struct {
	ID      string `json:"id"` // 与 timeline 的 case id 同一个 —— 一份 id 贯穿热层与真相源
	Title   string `json:"title,omitempty"`
	Status  string `json:"status,omitempty"`
	By      string `json:"by,omitempty"` // 谁在做
	To      string `json:"to,omitempty"` // 下一手该交给谁
	Domain  string `json:"domain,omitempty"`
	Project string `json:"project,omitempty"`
	Note    string `json:"note,omitempty"`
	AsOf    string `json:"as_of,omitempty"` // 这个当前值是「什么时候的」—— 新鲜度靠它，不靠 TTL
}

// GrantsOut 是「我被允许做什么」—— bot 侧那个**按人过滤**的视图（议题 #68）。
//
// 与看板「授权」页（全量）是**两份产物、不共用**：两个消费者、两个视图
// （共用迟早把全表漏给 bot）。但**尺子共用** —— 命中判据是 org.GrantHitsSubject，
// 与看板那个视角同一支：这两个视图答的是同一个问题。
//
// 四条口径：
//
//  1. **只筛、不改写**：字段原样搬（SPEC §6 那七个 + slug / path）。转述一遍就是多一处会漂的副本。
//  2. **Total 不受视角影响**：藏起「全表一共几条」就是藏事实 —— 看得见的只有与它相关的，
//     但「还有多少条不归你」这一格照样给。
//  3. **一条都没有 = 什么都不通**：SPEC §6 不变量 1（没有 grant 就是不通）。这句在 Note 里说清，
//     别让读的人把「表是空的」读成「我没权限看表」。
//  4. **它按主体算，不按域切**：授权表的客体可能是跨域的通道，切不出「这一块业务的那几条」——
//     所以这一块与 Todo / Doing / Gaps 同一条规矩：**只在要「全部」时给**。
type GrantsOut struct {
	Schema string        `json:"schema"`
	Total  int           `json:"total"` // 全表一共几条（不受视角影响）
	Shown  int           `json:"shown"` // 与这个主体相关的几条
	Rows   []GrantRowOut `json:"rows"`
	Note   string        `json:"note,omitempty"`
}

// GrantsSchema 是这一块的版本号（同看板那几页的理由：字段就是契约）。
const GrantsSchema = "anc.context.grants/v1"

// GrantRowOut 一行授权：**原样搬**，不转述（与看板 §7.1.53 那一页同一份字段清单）。
type GrantRowOut struct {
	Slug       string `json:"slug"`
	Path       string `json:"path"`
	From       string `json:"from"`
	To         string `json:"to"`
	Action     string `json:"action"`
	Object     string `json:"object"`
	TTL        string `json:"ttl"`
	OnBehalfOf string `json:"on_behalf_of"`
	Reason     string `json:"reason"`
	Body       string `json:"body"`
}

// TasksIn 是「派给我的待办」这一块的输入：热层当前值的一份快照。
//
// 为什么**传进来**而不是在这里读：装配点要一直是**纯函数**（不加载、不留痕、不打印）。
// 热层接没接上由三个出口各自交代，装配点只负责如实摆出来 —— 读不出来就报缺，
// 拿一份空清单冒充「没人派给你」，正是这个系统最怕的那种假绿。
//
// **就一个 Wired，不带原因**：原因（没配 / 连不上 / 读错）是**环境的**，各出口各说各话 ——
// 一进契约，三个出口的报缺文案就会各飘各的，「三处逐字段相同」当场作废。
// 原因说给人听，去处是各出口自己的台面：CLI / MCP 打 stderr，看板在起服务时先说一遍。
type TasksIn struct {
	Rows  []hot.State
	Wired bool // 这一层接上了没有（读到了才算）；false = 这一段答不了 —— 空清单 ≠ 没人派给你
}

// Refusal 是「没答出来」的那一格里的话：不给数据，给理由（能指路就指路）。
type Refusal struct {
	Reason string `json:"reason"`
	Ask    string `json:"ask,omitempty"` // 该找谁（能指路就指路）
}

// IdentityOf 把解出来的主体摊成回话里那一块。铭文出处只给业务 agent —— 成员 bot 没有 agents.md 那一行。
//
// 导出它是因为看板那一页也要摆同一块身份（名单入口），而「身份怎么摊」必须只有一处实现。
func IdentityOf(o *org.Org, idn org.Identity) *IdentityOut {
	out := &IdentityOut{
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
		out.Evidence = &Evidence{File: org.AgentsFile, Line: idn.Line}
	}
	return out
}

// Assemble 是「一个 agent 的工作上下文」**唯一一个装配点**。
//
// 三个出口共用它：MCP 的 anc_read_context（agent 来要）、`anc context pack`（人看）、
// 看板的一页（人看，同一个问题的另一种摆法）；`anc context freeze` 也是它 + 一份落盘。
// 装配只有一份，才不会出现「MCP 说的工作上下文」与「CLI 打的那一份」对不上号 ——
// 同一把尺子（VisibleDomainsOf）、同一套身份解析（o.Identity）、同一份报缺口径。
//
// 它只做装配：不加载真相源、不留痕、不打印 —— 那三样各出口有各自的交代方式。
// 回 (这一份上下文, 是不是拒了)。拒了的时候 out.Refused 有理由，数据一格都没有。
func Assemble(o *org.Org, vault, who, onBehalf, ask string, now time.Time, tasks TasksIn) (Out, bool) {
	// 自由文本（要哪个域）在**装配这一层**就压成能回显的短串：三个入口各清一遍，
	// 迟早有一边忘了清 —— 那一边的回话里就会出现控制字符或一整段注入文本。清一次，三边都干净。
	ask = SafeToken(ask)

	// 身份：解不出（含压根没给）→ **当这个人不存在**。所以理由里不许出现「参数错误」
	// 这类话 —— 那会把读的人引去改参数，而问题是他不在这家公司（envelope 包注释第 1 条）。
	subj, ok := o.Identity(who)
	if !ok {
		reason := "我们公司没有这个人 —— 身份不许自报：who 必须能在 members/ 或 agents.md 里解出来"
		return Out{
			Refused: &Refusal{Reason: reason},
			At:      now.Format(time.RFC3339),
		}, true
	}

	// 署名（可选）：给了就必须解得出来 —— 同一套身份解析（envelope.Resolves），不另开一套。
	if onBehalf != "" && !envelope.Resolves(o, onBehalf) {
		reason := "on_behalf_of 解不出 —— 写 member:名字 | role:岗位 | domain:slug，或直接写名字"
		return Out{
			Who: subj.Ref, OnBehalfOf: onBehalf,
			Refused: &Refusal{Reason: reason},
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
			return Out{
				Who: subj.Ref, OnBehalfOf: onBehalf,
				Refused: &Refusal{Reason: reason, Ask: helper},
				At:      now.Format(time.RFC3339),
			}, true
		}
	}

	out := Out{Who: subj.Ref, Identity: IdentityOf(o, subj), OnBehalfOf: onBehalf, At: now.Format(time.RFC3339)}
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
	// 「派给我的待办」（反向下发的那一步）。同目录 / 报缺那条纪律：只在要「全部」时给 ——
	// 单域问就是「只要那一块业务的那一行」，不把与那一块无关的东西塞进去。
	// 判据只有一条：热层的 to / by **解出来的主体编号**是不是这个人（署名不参与这一块）。
	var pick taskPick
	if ask == "" {
		pick = taskRows(o, subj, tasks)
		out.Todo, out.Doing = pick.Todo, pick.Doing
	}
	// 「我被允许做什么」—— bot 侧那个**按人过滤**的视图（议题 #68）。
	// 与 Todo / Doing 同一条规矩：只在要「全部」时给 —— 授权按**主体**算（客体可能是跨域的通道），
	// 切不出「这一块业务的那几条」，硬塞进单域问里就是编。
	if ask == "" {
		out.Grants = grantRows(o, subj)
	}
	// 报缺（gaps）同样只在要「全部」时给：要单独一域的人不该被一段通用说明淹掉 ——
	// 他要的是那一块业务的那一行，不是「这份上下文还答不了什么」那张清单。
	if ask == "" {
		out.Gaps = contextGaps(out, tasks, pick)
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

func domainRows(rows []renderpkg.DomainRow) []DomainOut {
	out := make([]DomainOut, 0, len(rows))
	for _, d := range rows {
		out = append(out, DomainOut{
			Slug: d.Slug, Name: d.Name, What: d.What, Data: d.Data, Who: d.Who,
			Terms: d.Terms, Sources: d.Sources,
			Evidence: &Evidence{File: org.DomainsFile, Line: d.Line},
		})
	}
	return out
}

// skillsOf 把这个成员**岗位**声明的技能摊出来，并逐条去看正文在不在。
// 只有清单、没有正文 = missing：报缺，不编（同 persona 的诚实条款）。
func skillsOf(o *org.Org, roleSlug, vault string) []SkillOut {
	role, ok := o.Roles[roleSlug]
	if !ok {
		return nil
	}
	out := make([]SkillOut, 0, len(role.Skills))
	for _, name := range role.Skills {
		if strings.TrimSpace(name) == "" {
			continue
		}
		where := "skills/" + name + "/SKILL.md" // 指针一律正斜杠：vault 里的形状，不是本机路径
		_, err := os.Stat(filepath.Join(vault, "skills", name, "SKILL.md"))
		out = append(out, SkillOut{Name: name, Where: where, Missing: err != nil})
	}
	return out
}

// projectRows 把**挂在这些域上**的项目摊出来。域不在可见范围里 = 不出现（与域行同一条纪律：
// 别人的域连项目名都不给，只给「找谁」）。owner 走与域行同一个 WhoLabel（岗位 → 人名）。
func projectRows(o *org.Org, chosen []renderpkg.DomainRow) []ProjectOut {
	inScope := make(map[string]bool, len(chosen))
	for _, d := range chosen {
		inScope[d.Slug] = true
	}
	var out []ProjectOut
	for _, p := range o.Projects {
		if !inScope[p.Domain] {
			continue
		}
		owner := o.WhoLabel(p.Owner)
		if owner == "—" {
			owner = ""
		}
		out = append(out, ProjectOut{
			Slug: p.Slug, Name: p.Name, Owner: owner,
			Period: p.Period, Source: p.Source,
			Evidence: &Evidence{File: org.ProjectsFile, Line: p.Line},
		})
	}
	return out
}

// taskPick 是 taskRows 的产物：两摞当前值 + **解不出主体**的那些写法。
//
// 解不出的分两摞记，因为后果不一样：`to` 解不出 = 这条**派不到任何人手里**（谁都不认领，也不报错）；
// `by` 解不出 = 这条不会出现在任何人的「我在做的」里（谁在做这件事，这一层答不了）。
type taskPick struct {
	Todo      []TaskOut
	Doing     []TaskOut
	UnknownTo []string
	UnknownBy []string
}

// taskRows 把热层那一摞当前值挑成「派给我的」与「我在做的」两摞，顺带把解不出的写法挑出来。
//
// 三条口径：
//   - 两边走**同一把尺子**（org.IdentityForRef + org.SameSubject）：热层里写 `alice` 还是
//     `member:alice` 是同一件事；写 `Alice Wang`（显示名）或 `role:manager`（岗位）**谁都不是** ——
//     岗位是「一类人」不是一个编号身份（与授权那条口径同一支，见 §7.1.55）。
//   - 解不出来的**不静默**：收进 UnknownTo / UnknownBy，由 contextGaps 报缺。
//     写作是人的事；写错了却一声不吭，那几条就永远落不到任何人手里 —— 正是这个系统最怕的失真。
//   - 状态一条都不筛：热层里还留着一条 done，说明它还没清走（达标是「先落库、再清热层」），
//     那不是这一层该替人做的判断。
func taskRows(o *org.Org, subj org.Identity, tasks TasksIn) taskPick {
	var pok taskPick
	// 一格三种结果：不是写给我的 / 是写给我的 / 这写法解不出人。
	hit := func(raw string) (mine, unresolvable bool) {
		if raw == "" {
			return false, false // 没写这一格 = 没这一手，不是写错
		}
		if idn, ok := o.IdentityForRef(raw); ok {
			return org.SameSubject(idn, subj), false
		}
		return false, true
	}
	for _, st := range tasks.Rows {
		if st.ID == "" {
			continue
		}
		row := TaskOut{
			ID: st.ID, Title: st.Title, Status: st.Status, By: st.By, To: st.To,
			Domain: st.Domain, Project: st.Project, Note: st.Note, AsOf: st.AsOf,
		}
		switch mine, bad := hit(st.To); {
		case mine:
			pok.Todo = append(pok.Todo, row)
		case bad:
			pok.UnknownTo = append(pok.UnknownTo, st.To)
		}
		switch mine, bad := hit(st.By); {
		case mine:
			pok.Doing = append(pok.Doing, row)
		case bad:
			pok.UnknownBy = append(pok.UnknownBy, st.By)
		}
	}
	// 热层的列举走 SCAN，顺序不保证：这里定死一个（编号升序）——
	// 三个出口要「除回答时刻外逐字段相同」，就不能让顺序飘。解不出的那两摞也定死（去重后字典序）。
	pok.UnknownTo = dedupeSorted(pok.UnknownTo)
	pok.UnknownBy = dedupeSorted(pok.UnknownBy)
	sort.Slice(pok.Todo, func(i, j int) bool { return pok.Todo[i].ID < pok.Todo[j].ID })
	sort.Slice(pok.Doing, func(i, j int) bool { return pok.Doing[i].ID < pok.Doing[j].ID })
	return pok
}

// dedupeSorted 去重 + 字典序：报缺里列的是**写法**，同一种写法写错三条只该说一次；
// 顺序定死，三个出口那句报缺才可能逐字相同。
func dedupeSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// capList 把报缺里要列的写法压成一句人话：先过 SafeToken（外部文本不许原样回显 —— 同 SafeToken 那条口径），
// 最多列 5 种，多的收成「……等 N 种」。报缺是给人一眼看懂的，不是把整摞倒出来。
func capList(in []string) string {
	const most = 5
	shown, tail := in, ""
	if len(in) > most {
		shown = in[:most]
		tail = "……等 " + strconv.Itoa(len(in)) + " 种"
	}
	out := make([]string, 0, len(shown)+1)
	for _, s := range shown {
		out = append(out, "`"+SafeToken(s)+"`")
	}
	if tail != "" {
		out = append(out, tail)
	}
	return strings.Join(out, "、")
}

// grantRows 组「我被允许做什么」那一块：**只出与这个主体相关的行**。
//
// 命中判据借 org.GrantHitsSubject（与看板那个视角**同一支** —— 两个视图答的是同一个问题），
// 这里只负责摆盘：字段原样搬、total 给全表数、空表把「空就是不通」说清楚、
// 并明说这一份是**视图**不是访问控制。
func grantRows(o *org.Org, subj org.Identity) *GrantsOut {
	out := &GrantsOut{
		Schema: GrantsSchema,
		Total:  len(o.Grants),
		Rows:   []GrantRowOut{},
		Note: "只出**与你相关**的几条（`from` / `to` / `on_behalf_of` 里指着你的）· " +
			"`total` 是全表一共多少条，不受这一份视图影响 · " +
			"这一份是**视图**、不是全量（全量在看板「授权」页）：它的过滤依据是 `who`，" +
			"而 `who` 是请求里带的自报值、**还没验过**（与接入面身份那一层同一件事）—— 所以它不是访问控制。",
	}
	for _, g := range o.Grants {
		if !org.GrantHitsSubject(o, g, subj) {
			continue
		}
		out.Rows = append(out.Rows, GrantRowOut{
			Slug: g.Slug, Path: g.Path,
			From: g.From, To: g.To, Action: g.Action, Object: g.Object,
			TTL: g.TTL, OnBehalfOf: g.OnBehalfOf, Reason: g.Reason, Body: g.Body,
		})
	}
	out.Shown = len(out.Rows)
	if out.Total == 0 {
		out.Note = "授权表是**空的**（`grants/` 里一条都没有）—— 按 SPEC §6 不变量 1「没有 grant 就是不通」，" +
			"现在什么都不通。这不是「你没权限看表」。"
	}
	return out
}

// contextGaps 明说「这份上下文答不了什么」。**只列没有真相源的那几样** ——
// 不编、也不假装完整：读的人据此知道该去问谁，而不是以为这就是全部。
func contextGaps(out Out, tasks TasksIn, pick taskPick) []string {
	var g []string
	// 热层没接上 = 这一段答不了。**必须与「没人派给你」分开说** ——
	// 否则读的人会拿一份「看不到」当成「没有」，那正是假绿。
	if !tasks.Wired {
		g = append(g, "派给我的待办：热层没接上 —— 这一段答不了，**不是**「没人派给你」")
	}
	// 热层里解不出主体的写法**必须说出来**：写 `Alice Wang`（显示名）或 `role:manager`（岗位）的那几条
	// 谁都不认领、也不报错 —— 那正是「看着绿、其实丢了」的那一类。热层没接上时不说（上一条已经交代了这一整段）。
	if tasks.Wired && len(pick.UnknownTo) > 0 {
		g = append(g, "派给我的待办：热层里有 "+strconv.Itoa(len(pick.UnknownTo))+" 种 `to` 写法解不出主体（"+
			capList(pick.UnknownTo)+"）—— 那几条**落不到任何人手里**。`to` 认的是编号（成员名 / 业务 agent 的 slug）；"+
			"写给外部对象的不算错，但它也不会出现在任何人的待办里")
	}
	if tasks.Wired && len(pick.UnknownBy) > 0 {
		g = append(g, "我在做的：热层里有 "+strconv.Itoa(len(pick.UnknownBy))+" 种 `by` 写法解不出主体（"+
			capList(pick.UnknownBy)+"）—— 那几条不会出现在任何人的「我在做的」里（谁在做这件事，这一层答不了）")
	}
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

func briefRows(rows []renderpkg.DomainBrief) []BriefOut {
	if len(rows) == 0 {
		return nil
	}
	out := make([]BriefOut, 0, len(rows))
	for _, d := range rows {
		out = append(out, BriefOut{Slug: d.Slug, Name: d.Name, What: d.What, Who: d.Who})
	}
	return out
}

// SafeToken 把外部递进来的自由文本压成一条能落盘、能回显的短串：去控制字符、压成单行、封顶 64 字。
//
// 三个入口（MCP / CLI / 看板）共用同一个 —— 落进审计痕里的是它，回显到拒话里的也是它；
// 请求原文不落盘、不回显（审计流水不是给人塞私货的地方，更不是给注入留的一条通道）。
//
// 幂等：清过的再清一遍还是它（Fields 与封顶两步都是收敛的），所以调用方多清一次不会出岔。
func SafeToken(s string) string {
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
