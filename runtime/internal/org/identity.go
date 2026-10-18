package org

import "strings"

// Identity 是接入面上的**主体** —— 成员 bot 与业务 agent 共用同一种形状。
//
// 为什么要合起来（2026-10-10）：两类主体接在同一张口上（`anc_read_context` / 信封），
// 在此之前 `who` 只认 members/ —— 业务 agent **出站被硬拒**（「我们公司没有这个人」）、
// 入站只标一条 warn，两边还不一样。同一件事两套解析，迟早对不上号（本仓库付过学费的那类）。
//
// 分辨一个主体要三样，缺一就「认不出这是谁」：
//   - Ref：**编号身份** —— 成员名 / 业务 agent 的 slug。不靠自报，来自真相源。
//   - Kind：哪一类 —— member / agent。失败代价不同（SPEC §2.3：成员 bot 答错人还能兜，
//     业务 bot 处理错单要赔钱），所以回话里要能分开。
//   - Domains：**归属** —— 挂在哪块业务（业务 agent 的 domain 列；成员在 persona 的 domains）。
//   - Inscription：**铭文** —— 干什么的 / 替谁干。业务 agent 取 agents.md 的 description；
//     成员 bot 这一格留空（它代人，来源是 members/<名>/persona.md，不在这张表上）。
type Identity struct {
	Kind        string
	Ref         string
	Name        string
	Role        string
	Domains     []string
	Inscription string
	Line        int
}

// Kind 的两个取值。写死成常量：回话里会当字段发给 harness，拼错就等于身份说不清。
const (
	KindMember = "member"
	KindAgent  = "agent"
)

// identityOfMember / identityOfAgent 是「真相源那一行 → 主体」的**唯一转换处**。
// Identity(ref) 与下面按平台身份查的那三条都走它 —— 各拼一遍，字段迟早漏一格。
func identityOfMember(m Member) Identity {
	return Identity{Kind: KindMember, Ref: m.Name, Name: m.DisplayName, Role: m.Role, Domains: m.Domains}
}

func identityOfAgent(a Agent) Identity {
	var domains []string
	if a.Domain != "" {
		domains = []string{a.Domain}
	}
	return Identity{
		Kind: KindAgent, Ref: a.Slug, Name: a.Name,
		Role: a.Role, Domains: domains,
		Inscription: a.Description, Line: a.Line,
	}
}

// Identity 按**编号**取主体：先成员、后业务 agent。
//
// 顺序写死（同 Resolves 的做法）：同一份输入永远同一个答案，不是猜。
// 两类主体的编号撞车在真相源里已经是红线（agent.slug.collides_member），
// 所以这里不会出现「两个都命中」的合法情形。
//
// 判不出来 = **这个主体不存在**，不是「参数写错了」—— 调用方的拒话照这个口径写。
func (o *Org) Identity(ref string) (Identity, bool) {
	if o == nil {
		return Identity{}, false
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Identity{}, false
	}
	if m, ok := o.Member(ref); ok {
		return identityOfMember(m), true
	}
	if a, ok := o.Agent(ref); ok {
		return identityOfAgent(a), true
	}
	return Identity{}, false
}

// SameSubject 是「这是同一个人吗」那把尺子：比的是**编号**（kind + ref），不是谁报上来的那个拼法。
//
// 三处问的是同一件事，就只留这一个定义：热层那条待办的 `to` / `by` 是不是派给这个主体、
// 一条授权是不是指着这个主体、接入面认出来的主体是不是这一个 —— 各写一遍比较，迟早出现
// 「看板说他有权、读出口说他没权」那种对不上号。
//
// 编号为空的算「不是」：没有编号就没有身份可比。**解不出来**这件事由调用方报缺，不在这里编一个。
func SameSubject(a, b Identity) bool {
	return strings.TrimSpace(a.Ref) != "" && a.Kind == b.Kind && a.Ref == b.Ref
}

// ---- 平台那三样：open_id / app_id / project 名 → 主体 ----
//
// 上面那个 Identity(ref) 认的是**编号**（成员名 / agent slug）。但平台那一侧递过来的
// 不是编号，是另外三样：谁发的（open_id）、发到哪个 app（app_id）、落在哪台 bot 上
// （project 名）。这一节就是把这三样各查一遍 —— **只查表，不猜**：
// 查不到就如实 false（同 Identity(ref) 的口径：判不出来 = 这个主体不存在）。
//
// 一条消息两头都要定位（谁发的 + 发到谁），所以每个入口都能单独用；一头解不出
// **不许**拿另一头顶上 —— 身份认错，后面每一条留痕都记在错的人头上。

// ProjectName 是「一个 bot 一个 project」的拼法：`<公司 id>-<名字>`。
//
// 规则住在真相源这一层 —— 它是 org 模型的事实（SPEC §4.7 ③：一个业务 agent 就是一个
// project，与成员 bot 共用同一条派生），不是某个渲染器的口味。渲染层只是导出一次
// （render.ProjectName），免得同一个拼法有两个定义。
func ProjectName(companyID, name string) string { return companyID + "-" + name }

// ByOpenID 按平台身份找人。open_id 是本系统里唯一的「人」标识（见 Feishu 的注释）；
// 业务 agent 不代表人、没有 open_id，所以这一条只可能落在成员上。
func (o *Org) ByOpenID(openID string) (Identity, bool) {
	m, ok := o.memberByOpenID(openID)
	if !ok {
		return Identity{}, false
	}
	return identityOfMember(m), true
}

// ByAppID 按「消息发到哪个 app」找那台 bot：成员 bot 与业务 agent 二选一
// （两者共用同一套平台接入，SPEC §4.7 ③）。
//
// 一个 app_id 只该挂一台，但这里**不替人判「挂错了」** —— 那是 `anc org check` 的事
// （agent.app_id.duplicate 那类规则），这一条只答「挂在谁身上」。
func (o *Org) ByAppID(appID string) (Identity, bool) {
	if o == nil {
		return Identity{}, false
	}
	id := strings.TrimSpace(appID)
	if id == "" {
		return Identity{}, false
	}
	for _, m := range o.Members {
		if strings.TrimSpace(m.Feishu.AppID) == id {
			return identityOfMember(m), true
		}
	}
	for _, a := range o.Agents {
		if strings.TrimSpace(a.AppID) == id {
			return identityOfAgent(a), true
		}
	}
	return Identity{}, false
}

// ByProject 按 project 名找那台 bot（`<公司 id>-<名字>`），归集器与网关递过来的就是它。
//
// 不带公司前缀的裸编号也收（走 Identity 那一支）—— 人在命令行里直接写名字是最常见的写法。
// 前缀要**对齐本公司 id**：`other-alice` 不是本公司的 project，不许被认成 alice。
func (o *Org) ByProject(project string) (Identity, bool) {
	if o == nil {
		return Identity{}, false
	}
	name := strings.TrimSpace(project)
	if name == "" {
		return Identity{}, false
	}
	if n := strings.TrimPrefix(name, o.Company.ID+"-"); n != name {
		return o.Identity(n)
	}
	return o.Identity(name)
}

// Owner 答「这个主体**属于谁**」：它挂的那块业务里，谁在管这件事。
//
// 成员返回空 —— 人没有归属人（他属于他自己，这是第一性的）；业务 agent 走
// `域 → domains.md 的 who（岗位） → 该岗位上启用中的成员`。后一段与渲染器给这个 agent
// 算**收件人**用的是同一支判据（render.agentAllowFrom 的第一段）—— 两处各算各的，
// 迟早出现「消息发给甲、看板说归乙」。
//
// 但这里**只答归属、不答收件**：域 who 岗位上没人时，渲染那边会落到公司 admins 兜底
// （不兜它就没人叫得动这个 agent），那是**收件**口径 —— 混进归属就等于宣称
// 「没人的域归老板所有」。
//
// 两个返回值：who = 归属人（顿号连起来；解不出则空），why = **判据原话**或解不出的原因。
// 别让人猜「为什么是他」。
func (o *Org) Owner(id Identity) (string, string) {
	if o == nil {
		return "", "没有加载到 org 真相源"
	}
	if id.Kind == KindMember {
		return "", "成员没有归属人 —— 他属于他自己"
	}
	domain := ""
	if len(id.Domains) > 0 {
		domain = strings.TrimSpace(id.Domains[0])
	}
	if domain == "" {
		return "", "这个主体没写 domain —— 没有域就没有「谁在管这块业务」"
	}
	d, ok := o.Domain(domain)
	if !ok {
		return "", "域 " + domain + " 不在 domains.md 里 —— 解不出属于谁"
	}
	who := strings.TrimSpace(d.Who)
	if who == "" {
		return "", "域 " + d.Slug + " 的 who 是空的 —— 这块业务没写谁在管"
	}
	var names []string
	for _, m := range o.Enabled() {
		if m.Role == who {
			names = append(names, m.Name)
		}
	}
	if len(names) == 0 {
		return "", "域 " + d.Slug + " 的 who 岗位 " + who +
			" 上没有启用中的成员（渲染那边这条会落到 admins 兜底 —— 那是收件口径，不是归属）"
	}
	return strings.Join(names, "、"), "域 " + d.Slug + " 的 who 岗位 " + who + " —— " + o.WhoLabel(who)
}
