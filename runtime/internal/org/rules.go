package org

import (
	"fmt"
	"sort"
	"strings"
)

// Level 是规则的处置级别。
//
// 门禁唯一正当的理由是防「静默的不可逆损失」：让在跑的 bot 下线、对外答错、
// 覆盖人的手写文件。所以只有两档：
//
//	FATAL —— 阻止落盘。
//	WARN  —— 照常走，但必须回显给 owner 看。
//
// 「可逆的小事也拦下来」只是给 owner 添堵，所以默认级别一律按上面这条判据取。
type Level string

const (
	LevelFatal Level = "fatal"
	LevelWarn  Level = "warn"
	LevelOff   Level = "off"
)

// ParseLevel 解析 company.md 里写的级别。
func ParseLevel(s string) (Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "fatal", "error", "err":
		return LevelFatal, true
	case "warn", "warning":
		return LevelWarn, true
	case "off", "skip", "none":
		return LevelOff, true
	}
	return "", false
}

// Rule 是一条校验规则。
//
// 规则住在表里，不埋在实现里：关一条不用发版（在 company.md 的 policy 段覆盖），
// 审计时看这张表，而不是读一千行 Go。
type Rule struct {
	ID     string
	Level  Level
	Locked bool   // true = 不许在 company.md 里降级或关闭
	Why    string // 为什么定这一档
}

// DefaultRules 是出厂规则表，顺序即回显顺序。
var DefaultRules = []Rule{
	// —— 公司层 ——
	{"company.name.missing", LevelFatal, false, "缺公司名，persona 段 1 会渲染成空"},
	{"company.id.format", LevelFatal, false, "id 会进 launchd label 与 project 名前缀，非 ASCII 会炸服务管理"},
	{"company.platform.unsupported", LevelFatal, false, "v1 只支持 feishu，别的平台没有渲染路径"},
	{"company.display.invalid", LevelFatal, false, "这个值直接进 cc-connect 的 [display].mode，上游只认 full / compact / quiet；写错它拒绝启动 = 全部 bot 下线"},
	{"company.defaults.model.missing", LevelFatal, false, "三级回退（member → role → company）没有终点，会渲染出空 model"},
	{"company.defaults.mode.bypass", LevelFatal, true, "SPEC §6-5 无例外红线：角色 bot 一律不授予 bypass"},
	{"company.defaults.reset_on_idle.invalid", LevelFatal, false, "cc-connect 要求 reset_on_idle_mins >= 0，写负它拒绝启动 = 该 bot 下线"},
	{"company.devbot.count", LevelFatal, false, "SPEC §1：一个公司恰好一个启用中的 devbot"},
	{"company.admins.unknown", LevelFatal, false, "admins 渲染进 admin_from，指向不存在的成员等于特权命令无人可发"},
	{"company.admins.disabled", LevelWarn, false, "管理员已停用，admin_from 仍会渲染它；确认是有意保留"},
	{"company.access.cross_domain.invalid", LevelFatal, false, "跨域可见性写错会静默改变「谁能看到什么」—— 权限口径不许猜；只认 visible / hidden，不写 = visible"},

	// —— 成员层 ——
	{"member.name.duplicate", LevelFatal, false, "同名成员会生成同名 project，gateway 只留一个"},
	{"member.name.format", LevelFatal, false, "名字要当**目录名**与 project 名用（homes/<名>、members/<名>、<公司 id>-<名>）：含 / 或反斜杠、Windows 建不出目录的字符（: * ? 双引号 < > |）、控制字符、首尾空白、结尾 . ，或本身就是 . / .. 时，路径会跑偏或建不出来 = 这个 bot 起不来。**中文 / 空格 / - / . 一律合法** —— 凭据键名不走名字，走 render.FeishuSecretKey 的全函数派生（2026-10-08 改，见 runtime/DESIGN.md §7.1.16）。不锁死：存量 vault 可在 company.md 写 `policy: member.name.format = warn`"},
	{"member.display_name.missing", LevelFatal, false, "persona 段 1 的「服务对象」会渲染成 —"},
	{"member.role.missing", LevelFatal, false, "role 不存在就没有 persona 基线，渲染直接失败"},
	{"member.model.unresolved", LevelFatal, false, "member 没写 model，role / company 也没写，无处回退"},
	{"member.feishu.open_id.missing", LevelFatal, false, "allow_from 靠它；缺了就没人能跟这个 bot 说话"},
	{"member.feishu.open_id.format", LevelWarn, false, "格式（ou 前缀）只是经验值，平台是可变层，不该拿它拦人"},
	{"member.feishu.open_id.duplicate", LevelFatal, false, "两个人共用一个 open_id，等于一个 bot 服务两个人"},
	{"member.feishu.app_id.missing", LevelFatal, false, "没有 app_id 建不起平台连接"},
	{"member.feishu.app_id.duplicate", LevelFatal, false, "同一个飞书应用被两个 project 抢连，消息归属不确定"},
	{"feishu.wildcard", LevelFatal, false, "SPEC §6-1：白名单默认关闭注册，星号等于对所有人开放"},

	// —— 角色层 ——
	{"role.bypass.not_devbot", LevelFatal, true, "SPEC §6-5 无例外红线：只有 devbot 可以有 bypassPermissions"},
	{"role.allowed_tools.empty", LevelFatal, false, "实测（上游 schema 790 行）：dontAsk 下未预授权的工具一律自动拒绝 —— 空白名单的 bot 连得上却干不了活，现场还全是绿的"},
	{"role.persona.section_unknown", LevelFatal, false, "canonical registry 纪律：角色层不许私藏事实（段名清单见 company.persona_sections）"},

	// —— persona 文本 ——
	{"persona.toml.quote_break", LevelFatal, true, "SPEC §171 生产事故：persona 内联进 TOML，裸三引号曾致全部 bot 下线"},
	{"persona.env.substitution", LevelFatal, false, "gateway 会替换美元大括号引用，persona 里这种引用会被吞掉或注入"},
	{"persona.slot.unreplaced", LevelWarn, false, "双花括号槽位没被替换就进了 persona，多半是模板没跑完"},
	{"persona.path.absolute", LevelWarn, false, "绝对路径写死本机布局，搬机器 / 换 vault 就打脸"},
	{"persona.fact.frozen", LevelWarn, false, "「当前仅有 / 目前只有」疑似把易变事实烤进 persona"},

	// —— 业务域（罗盘）——
	// 域表是新能力：没有这张表的存量 vault 不该被拦（LoadDomains 在文件缺席时一声不吭），
	// 但一旦建了表，表里的结构问题必须被看见 —— 所以整组默认 warn，一条红线都不设。
	// 想让某条变红，在 company.md 的 policy 段写 `规则id = fatal`（本组全部允许上调）。
	{"domain.table.missing", LevelWarn, false, "有 domains.md 但没找到带 slug 列的表 —— 表建歪了，域一个都读不出来"},
	{"domain.slug.format", LevelWarn, false, "slug 要进文件名与代码；中文/大写会撞 macOS 的 NFC/NFD 与跨平台差异"},
	{"domain.slug.duplicate", LevelWarn, false, "同 slug 两行 = 域身份有歧义，取哪一行只看行的先后"},
	{"domain.name.missing", LevelWarn, false, "缺显示名，persona 里只剩机器标识"},
	{"domain.what.missing", LevelWarn, false, "缺「这块业务是什么」—— agent 少一条判断口径的依据"},
	{"domain.who.missing", LevelWarn, false, "缺「谁在做」—— 这一域卡住时没人可找"},
	{"domain.data.missing", LevelWarn, false, "缺「数据在哪」—— 这一域没有作用域，授权无从落脚"},
	{"domain.who.unknown_role", LevelWarn, false, "who 不在 roles/ 里，渲染时解不出人，只会原样显示岗位名"},
	{"domain.data.unknown_dir", LevelWarn, false, "data 不是 vault 顶层数据目录，作用域指不到地方"},
	{"member.domains.unknown", LevelWarn, false, "成员写了未定义的域，多半是拼错 —— 等于悄悄少给他一块业务"},
	{"member.reports_to.self", LevelWarn, false, "汇报链指向自己 —— 看板会显示成自己管自己，人事关系失真"},
	{"member.reports_to.unknown", LevelWarn, false, "reports_to 不是成员 name —— 上级解不出人；显示名 / 岗位名都会落到这一档"},

	// —— 项目表（立项书汇总）——
	// 与域表同一档，理由也一样：立项书是别人写的、格式随公司/行业而变，ANC 只汇总不清点格式，
	// 所以整组 warn、一条红线都不设。想让某条变红，在 company.md 的 policy 段写 `规则id = fatal`。
	{"project.table.missing", LevelWarn, false, "有 projects.md 但没找到带 slug 列的表 —— 表建歪了，项目一个都读不出来"},
	{"project.slug.format", LevelWarn, false, "slug 要进文件名与代码；中文/大写会撞 macOS 的 NFC/NFD 与跨平台差异"},
	{"project.slug.duplicate", LevelWarn, false, "同 slug 两行 = 项目身份有歧义，取哪一行只看行的先后"},
	{"project.name.missing", LevelWarn, false, "缺项目名，看板上只剩机器标识"},
	{"project.domain.unknown", LevelWarn, false, "挂的域不在 domains.md 里 —— 多半是拼错；项目会分不到区块"},
	{"project.owner.unknown_role", LevelWarn, false, "owner 不在 roles/ 里，渲染时解不出人，只会原样显示岗位名"},

	// —— 业务 agent（SPEC §2.3 第二类：代岗位 / 流程）——
	// 结构问题与域表 / 项目表同一档：整组 warn，一条红线都不设（新能力先「看见」不先「拦」），
	// 想在 company.md 的 policy 段写 `agent.xxx = fatal` 就上调。
	// **唯一一条 fatal 是 tools 为空** —— 它与 role.allowed_tools.empty 是同一条不变量
	// （空白名单的 bot 连得上却干不了活），同一条不变量只登记成一条规则，不因为它出现在另一张表就换档。
	{"agent.table.missing", LevelWarn, false, "有 agents.md 但没找到带 slug 列的表 —— 表建歪了，业务 agent 一个都读不出来"},
	{"agent.slug.format", LevelWarn, false, "slug 要进 project 名、通常还要当 OS 账号名；中文/大写会撞 macOS 的 NFC/NFD 与跨平台差异"},
	{"agent.slug.duplicate", LevelWarn, false, "同 slug 两行 = agent 身份有歧义，取哪一行只看行的先后"},
	{"agent.name.missing", LevelWarn, false, "缺显示名，看板上只剩机器标识"},
	{"agent.domain.missing", LevelWarn, false, "没挂域 —— 数据视野那把尺子（VisibleDomains）以域为单位，没域等于没视野"},
	{"agent.domain.unknown", LevelWarn, false, "挂的域不在 domains.md 里 —— 多半是拼错；这个 agent 分不到任何数据"},
	{"agent.role.missing", LevelWarn, false, "没写 role —— 业务 agent 代的是岗位，persona 的职责 / 风格 / 术语表来自 roles/<role>/；没它这一层空着"},
	{"agent.role.unknown", LevelWarn, false, "role 不在 roles/ 里 —— 多半是拼错"},
	{"agent.slug.collides_member", LevelWarn, false, "slug 与某个成员同名 —— project 名（<公司 id>-<slug>）会撞成同一个，配置里只留得下一个"},
	{"agent.tools.empty", LevelFatal, false, "SPEC §4.7 ③ 红线：业务 agent 的 allowed_tools 必须非空（只有 devbot 例外）。实测：dontAsk 下未预授权的工具一律自动拒绝 —— 空白名单的 bot 连得上却干不了活，现场还全是绿的；不锁死：可在 company.md 写 `policy: agent.tools.empty = warn`"},

	// —— 立项书副本（接入链的**落点**）——
	// 落点的目录名要能与表行对上，否则「谁在做哪个项目」就有一半材料是孤岛。
	// 仍然只告警：副本目录可能比表行**早到**（材料先丢进来、抽取还没做），那是进度问题不是错。
	{"charter.entry.unmatched", LevelWarn, false, "落点里的副本目录在 projects.md 里没有对应 slug —— 多半是名字写错，或这一项还没抽成表行"},

	// —— 接入面（信封 #44）——
	// 与域表 / 项目表同一档、同一理由：接入面是新能力，身份与作用域先「看见」，不先「拦」。
	// 想让某条变红，在 company.md 的 policy 段写 `规则id = fatal`（本组全部允许上调，也允许 off：
	// 这就是 #44 说的「开关」—— 门禁是数据，不是代码里的 if）。
	{"envelope.who.unknown", LevelWarn, false, "who 不是真相源里的成员 —— 身份不许自报（#44 验收：who 来自真相源）"},
	{"envelope.on_behalf_of.missing", LevelWarn, false, "SPEC §2.3：bot 发出的必须带 on_behalf_of；缺了审计答不出「谁授权的」"},
	{"envelope.on_behalf_of.unknown", LevelWarn, false, "on_behalf_of 解不出人 / 岗位 / 业务域 —— 署名指不到人"},
	{"envelope.scope.domain.unknown", LevelWarn, false, "scope.domain 不在 domains.md 里 —— 这块业务不存在，授权无从落脚"},
	{"envelope.scope.project.unknown", LevelWarn, false, "scope.project 不在 projects.md 里 —— 多半是拼错，或还没抽成表行"},
	{"envelope.kind.missing", LevelWarn, false, "没写 kind —— 收件人得自己猜这是请示还是汇报"},
	{"envelope.kind.unknown", LevelWarn, false, "kind 不在已知词表 —— 照收，只是归类不了（词表不锁死）"},
	// —— 授权表（SPEC §6 授权模型 / grants/）——
	// 与域表 / 项目表 / 接入面同一条先例，理由也一样：授权是**新能力**，先「看见」不先「拦」。
	// 存量 vault 没有 grants/ 目录 = 零条授权，一声不吭（不报错）。
	// 出厂档：**九条 warn + 一条 off**（`grant.ttl.missing` —— 2026-10-10 改口径：期限可选、不写 = 永久）。
	// 想让某条变红，在 company.md 的 policy 段写 `规则id = fatal`（本组全部允许上调，也允许 off）；
	// 反过来，想让「授权必带期限」的口径回来，把 `grant.ttl.missing` 开成 warn / fatal 即可。
	// 这一层只做**结构**校验：from 解得出 actor。to / object 的客体词表（通道 #33、看板分区 #7）
	// 还没有定义处，现在校验它们只能是猜 —— 宁可留白，不做看着像校验的猜测。
	{"grant.file.unparsable", LevelWarn, false, "落点里有认不出的文件 —— 一个 grant 一个文件：要么是说明文件（CLAUDE.md / README.md），要么带 frontmatter；静默丢掉一条授权比报一条错危险得多"},
	{"grant.file.multiple", LevelWarn, false, "正文里还有第二段 grant frontmatter —— 一个 grant 一个文件；第二段不会被读，等于一条授权静默消失"},
	{"grant.slug.format", LevelWarn, false, "文件名要进路径与代码；中文/大写会撞 macOS 的 NFC/NFD 与跨平台差异"},
	{"grant.from.missing", LevelWarn, false, "缺授权者 —— 这条链的起点不清楚"},
	{"grant.to.missing", LevelWarn, false, "缺被授权者 —— 给谁的不清楚"},
	{"grant.action.missing", LevelWarn, false, "缺动作 —— read / write / invoke 一个都没写"},
	{"grant.object.missing", LevelWarn, false, "缺客体 —— 授权总得给到某个东西上"},
	{"grant.ttl.missing", LevelOff, false, "SPEC §6（2026-10-10 改口径）：期限可选、**不写 = 永久** —— 所以这条出厂档是 off，不是删掉：想要「授权必带期限」的客户在 company.md 里 `policy: grant.ttl.missing = warn` 把它开回来"},
	{"grant.action.unknown", LevelWarn, false, "action 不在已知词表 —— 照收，只是执行层认不出来（词表不锁死）"},
	{"grant.from.unknown", LevelWarn, false, "from 解不出成员或岗位 —— 授权者不存在，这条链是断的（域是客体，不能当授权者）"},
	{"grant.match.missing", LevelFatal, false, "SPEC §6 不变量 1「默认拒」：跨域信封没有被 grants/ 里任何一条覆盖 —— 没有 grant 就是不通（跨业务域、跨 bot、看板分区同理）。**与上面九条结构校验不同**：那九条是「新能力先看见不先拦」，这一条是**执行层**，出厂就拦。不锁死 —— 存量客户在 company.md 写 `policy: grant.match.missing = warn` 就能先看见不先拦"},

	// —— 策略层（策略自己也要被校验，否则拼错规则名 = 你以为关了其实没关）——
	{"policy.override.unknown", LevelFatal, true, "policy 段写了不存在的规则 id 或非法级别值"},
	{"policy.override.locked", LevelFatal, true, "试图降级 / 关闭无例外红线（SPEC §6-5 / §171）"},
}

// Policy 是叠加了公司级覆盖之后的生效规则表。
type Policy struct {
	levels    map[string]Level
	locked    map[string]bool
	why       map[string]string
	overrides map[string]Level
}

// DefaultPolicy 造一份出厂策略。
func DefaultPolicy() *Policy {
	p := &Policy{
		levels:    map[string]Level{},
		locked:    map[string]bool{},
		why:       map[string]string{},
		overrides: map[string]Level{},
	}
	for _, r := range DefaultRules {
		p.levels[r.ID] = r.Level
		p.why[r.ID] = r.Why
		if r.Locked {
			p.locked[r.ID] = true
		}
	}
	return p
}

// Level 取规则级别。未登记的 id 当 fatal —— 拼错规则名不许悄悄放行。
func (p *Policy) Level(id string) Level {
	if l, ok := p.levels[id]; ok {
		return l
	}
	return LevelFatal
}

// Issue 用本策略的级别造一条发现。
func (p *Policy) Issue(rule, where, format string, a ...any) Issue {
	return Issue{Rule: rule, Level: p.Level(rule), Where: where, Msg: fmt.Sprintf(format, a...)}
}

// Effective 返回生效规则表（顺序同 DefaultRules），供 `anc org check --rules` 回显。
func (p *Policy) Effective() []Rule {
	out := make([]Rule, 0, len(DefaultRules))
	for _, r := range DefaultRules {
		r.Level = p.Level(r.ID)
		out = append(out, r)
	}
	return out
}

// OverrideIDs 返回被 company.md 改过级别的规则 id（排序）。
func (p *Policy) OverrideIDs() []string {
	out := make([]string, 0, len(p.overrides))
	for id := range p.overrides {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Apply 把 company/company.md 里 `policy:` 段的覆盖叠加上去。
// 段内写法：一行一条 `规则id: fatal|warn|off`（与 README 一致；行尾可跟 # 注释）。
func (p *Policy) Apply(d *Doc) []Issue {
	var out []Issue
	keys := make([]string, 0, len(d.Meta))
	for k := range d.Meta {
		if strings.HasPrefix(k, "policy.") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		id := strings.TrimPrefix(k, "policy.")
		where := "company/company.md"
		if n := d.Line(k); n > 0 {
			where = fmt.Sprintf("company/company.md 第 %d 行", n)
		}
		if _, known := p.levels[id]; !known {
			out = append(out, p.Issue("policy.override.unknown", where,
				"policy 段里的 %q 不是已登记的规则 id（查 `anc org check --rules`）", id))
			continue
		}
		lv, ok := ParseLevel(fmt.Sprintf("%v", d.Meta[k]))
		if !ok {
			out = append(out, p.Issue("policy.override.unknown", where,
				"%s 的级别 %v 非法；只认 fatal / warn / off", id, d.Meta[k]))
			continue
		}
		if lv != LevelFatal && p.locked[id] {
			out = append(out, p.Issue("policy.override.locked", where,
				"%s 不许降级到 %s（%s）", id, lv, p.why[id]))
			continue
		}
		p.levels[id] = lv
		p.overrides[id] = lv
	}
	return out
}

// Issue 是一条校验发现。规则 id 稳定、文案会改 —— 所以测试断言 id，不断言文案。
type Issue struct {
	Rule  string
	Level Level
	Where string
	Msg   string
}

func (i Issue) String() string {
	if i.Where == "" {
		return fmt.Sprintf("[%s] %s", i.Rule, i.Msg)
	}
	return fmt.Sprintf("[%s] %s: %s", i.Rule, i.Where, i.Msg)
}

// Report 是一次校验的全量发现。
type Report struct {
	Issues []Issue
}

// Add 记录一条发现；off 档的直接丢弃。
// 逐字相同的重复发现只留一条：两个成员共用一个角色时，角色层的问题不该报两遍。
func (r *Report) Add(i Issue) {
	if i.Level == LevelOff || i.Rule == "" {
		return
	}
	for _, e := range r.Issues {
		if e == i {
			return
		}
	}
	r.Issues = append(r.Issues, i)
}

// Merge 并入另一份报告。
func (r *Report) Merge(o *Report) {
	for _, i := range o.Issues {
		r.Add(i)
	}
}

// ByLevel 取某一档的发现。
func (r *Report) ByLevel(l Level) []Issue {
	var out []Issue
	for _, i := range r.Issues {
		if i.Level == l {
			out = append(out, i)
		}
	}
	return out
}

// Fatal / Warns 按档取（Warns 是方法，与 Issue 的字段区分开）。
func (r *Report) Fatal() []Issue { return r.ByLevel(LevelFatal) }
func (r *Report) Warns() []Issue { return r.ByLevel(LevelWarn) }

// Err 把 fatal 档折成 error；没有 fatal 返回 nil。
func (r *Report) Err() error {
	f := r.Fatal()
	if len(f) == 0 {
		return nil
	}
	return fmt.Errorf("org 校验未通过（%d 项红）:\n  - %s", len(f), strings.Join(IssueStrings(f), "\n  - "))
}

// LoadError 携带全量报告。红档拦下，非红档也一并说出来 ——
// 一次报全，别让人修一个才看见下一个。
type LoadError struct {
	Report *Report
}

func (e *LoadError) Error() string {
	f := e.Report.Fatal()
	msg := fmt.Sprintf("org 校验未通过（%d 项红）:\n  - %s", len(f), strings.Join(IssueStrings(f), "\n  - "))
	if w := e.Report.Warns(); len(w) > 0 {
		msg += fmt.Sprintf("\n另有 %d 项非红档发现（不拦，但请过目）:\n  - %s", len(w), strings.Join(IssueStrings(w), "\n  - "))
	}
	return msg
}

// IssueStrings 把发现折成一行一条。
func IssueStrings(list []Issue) []string {
	out := make([]string, 0, len(list))
	for _, i := range list {
		out = append(out, i.String())
	}
	return out
}

// FatalOf 从一堆发现里挑出 fatal 档。
func FatalOf(list []Issue) []Issue {
	var out []Issue
	for _, i := range list {
		if i.Level == LevelFatal {
			out = append(out, i)
		}
	}
	return out
}
