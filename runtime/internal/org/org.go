package org

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"anc/internal/gateway"
)

// Company 是 company/company.md 的机器可读部分。
type Company struct {
	Name     string
	ID       string
	Language string
	Timezone string
	Platform string
	// Display 是 bot 在 IM 里的呈现档：quiet / compact / full（空 = 出厂默认 quiet）。
	// 它直接进 cc-connect 的 [display].mode —— 聊天窗是**给人看的观测面**，
	// 不是 agent 的工作日志，所以默认只出结果、不把「思考」「工具调用」各发一条。
	Display         string
	Admins          []string
	SyncIntervalMin int
	Defaults        Defaults
	Fallback        *FallbackProvider
	Body            string
}

// [display].mode 的合法取值（cc-connect 的口径，多一个都不认）。
const (
	DisplayQuiet   = "quiet"   // 隐藏思考与工具调用，所有文本合并进一张卡片 —— 只出结果（出厂默认）
	DisplayCompact = "compact" // 隐藏思考与工具调用，文本仍分段发
	DisplayFull    = "full"    // 思考、工具调用各发一条 —— 只在排查时用
)

// DisplayModes 是给报错文案用的合法值清单（顺序 = 从静到吵）。
// 它**不是**自己写的第二份 —— 取值口径来自网关形态表（SPEC §4.7 G1：
// 「上游认哪些枚举」是网关的形态，不是 org 的 schema），有测试钉住两边一致。
var DisplayModes = append([]string(nil), gateway.CCConnect.DisplayModes...)

// ValidDisplayMode 判一个值认不认识。空串算合法 —— 它是「没写」，不是「写错」。
// 判据只有一份：网关形态表里那个清单。
func ValidDisplayMode(s string) bool { return gateway.CCConnect.ValidDisplayMode(s) }

// DisplayMode 是生效的呈现档：没写就是出厂默认。
func (c Company) DisplayMode() string {
	if c.Display == "" {
		return DisplayQuiet
	}
	return c.Display
}

// Defaults 是公司级默认值；成员/角色可逐级覆盖 model。
type Defaults struct {
	Model                 string
	Mode                  string
	AutoCompressMaxTokens int
	// ResetOnIdleMins 是「空闲超过多少分钟就换新会话」。**0 = 关掉**，也是「没写」时的值。
	// 关掉是 2026-10-07 的拍板：换新会话由人**显式发 /new**，不靠计时器替人猜。
	ResetOnIdleMins int
}

// FallbackProvider 是兜底 provider 声明（api_key 只允许 env 引用）。
type FallbackProvider struct {
	Name    string
	BaseURL string
	Model   string
}

// Section 是 persona 正文里的一个 H2 段。保留声明顺序 —— 渲染要按作者的原文顺序走。
type Section struct {
	Name string
	Text string
}

// Role 是 roles/<role>/persona.md。
type Role struct {
	Role         string
	Title        string
	Model        string
	Mode         string
	AllowedTools []string
	VaultScope   []string
	Skills       []string
	Sections     map[string]string // 白名单段名 → 正文（清单见 company.persona_sections）
	Extra        []Section         // 白名单之外、但被降级放行的段：原样保留，不倒掉作者的稿子
}

// Feishu 是成员的平台接入信息。open_id 是本系统里唯一的「人」标识，且只在渲染时进 allow_from。
type Feishu struct {
	AppID          string
	OpenID         string
	ExtraAllowFrom []string
	AllowChat      []string
}

// Member 是 members/<name>/persona.md。
type Member struct {
	Name        string
	DisplayName string
	Role        string
	Model       string
	Admin       bool
	Disabled    bool
	// Unwired = 「这个 bot 还没接平台凭据」，是**声明**不是推断（探针第四档灰靠它）。
	// 为什么要有：一个公司恰好要有一个启用中的 devbot（不可豁免），只接了部分腿的客户
	// 必然有几个 bot 暂时没有真 app —— 少这一档，那几个会把运行态报告一路染黄。
	Unwired bool
	Domains []string // 所属业务域（slug，见 domains.md）；persona 第 8 段与授权作用域都用它
	Feishu  Feishu
	Body    string
}

// Routing 是 vault 顶层数据目录 → 说明，供 persona 段 3 的确定性路由表使用。
type Routing struct {
	Dir     string
	Summary string
}

// Org 是加载并校验过的组织真相源。
type Org struct {
	Root     string
	Company  Company
	Roles    map[string]Role
	Members  []Member      // 按 Name 排序
	Domains  []Domain      // domains.md 的业务域表（罗盘）；文件不存在时为空
	Projects []Project     // projects.md 的项目汇总（一行一个项目）；文件不存在时为空
	Agents   []Agent       // agents.md 的业务 agent 表（SPEC §2.3 第二类：代岗位，不代人）；文件不存在时为空
	Charters []CharterCopy // charters/ 的立项书副本条目；目录不存在时为空
	Grants   []Grant       // grants/ 的授权条目（SPEC §6 授权模型）；目录不存在 = 零条授权，不算错
	Routing  []Routing
	Policy   *Policy // 生效规则表（出厂默认 + company.md 的 policy 段覆盖）
	Warnings []Issue // 非红档的校验发现；调用方必须回显，不许吞
}

// Host 是本机层输入（不进 git）；参与 inputs 指纹，改主机参数同样触发重渲染。
type Host struct {
	VaultRoot string
	HomesRoot string
	DataDir   string

	// AgentHomes 是**业务 agent 的 cwd**（agent slug → 目录）。
	//
	// 为什么在这里而不在真相源：它是**本机事实**（这台机器上这个 agent 落在哪），
	// 进 git 就会在跨机同步时只对一台机器成立（SPEC §4.5：git = org 真相，~/.anc = 部署参数）。
	// 没给的那一个 = 还没落到机器上 —— 渲染时如实列缺，不猜一个目录出来。
	AgentHomes map[string]string
}

// AgentHome 取这个业务 agent 的 cwd；没配（或配成空白）→ false。
// 调用方拿到 false 要**如实报缺**，不许拿 HomesRoot 拼一个看着像的路径顶上 ——
// 那会把别的账号的目录当成本机目录，装载时要么建错地方、要么在别人家目录里建夹子。
func (h Host) AgentHome(slug string) (string, bool) {
	if h.AgentHomes == nil {
		return "", false
	}
	d, ok := h.AgentHomes[slug]
	if !ok || strings.TrimSpace(d) == "" {
		return "", false
	}
	return d, true
}

// skipDirs 是不当数据目录扫描的顶层目录。
//
// 判据只有一条：**它是不是「人把资料放进来、agent 去查」的地方**（SPEC §4.3 的 `<domain>/`）。
// 不是的就不进路由表 —— 路由表会整行进 persona 段 3「数据来源」，多进去一个不等于多一条线索，
// 等于给 agent 指错路（它会照表去一个根本没有业务资料的地方翻）。
//
// 分两组。加新顶层目录时先想清楚属于哪一组，别让它默认漏进路由表：
//   - 结构目录（SPEC §4.3 列举）：roles / members / skills / company / templates / scripts；
//   - 本库的落点（本仓库自己认的目录，不是客户的业务资料）：
//     docs（本库说明）、_originals（原件层，实际住在各 domain 目录里，这是顶层兜底）、
//     charters（立项书副本落点 —— 资产页已单独统计它，进路由表还会**重复计一次**）、
//     timeline（决策与执行留存：谁记谁读，不是给人放业务资料的地方）、
//     grants（授权表是**控制面真相源**，不是业务资料：路由表回答「去哪找业务资料」，
//     它回答「谁被允许做什么」——SPEC §3：真相源仓库 ≠ bot 的工作区）、
//     audit（行使的流水 —— SPEC §6 不变量 4：**系统记下来的**，不是给人放业务资料的地方。
//     它比 timeline 更敏感：含「谁想碰什么」，注册进路由表等于把审计面摊给 agent，见议题 #43）。
//
// ContextDir 是工作上下文**冻结副本**的落点（vault 顶层；`anc context freeze` 往这里写）。
//
// 名字跟 charters 是同一族：一个落「客户那边同步过来的副本」，一个落「我们这边算出来的快照」。
// 两者都不是真相源，所以都不进路由表（见下面的 skipDirs）。
const ContextDir = "context"

var skipDirs = map[string]bool{
	// 结构目录（SPEC §4.3）
	"roles": true, "members": true, "skills": true, "company": true,
	"templates": true, "scripts": true,
	// 本库的落点
	"docs": true, "_originals": true, "charters": true, "timeline": true, "grants": true,
	"audit": true,
	// context 是**算出来的**工作上下文冻结副本（`anc context freeze` 的落点）。
	// 它比 charters 更该被挡住：charters 至少是「客户那边的文件同步过来的」，
	// 这一份压根不是谁放进来的资料，是投影冻住的一刻 —— 进路由表等于让 agent
	// 照着一份会过期的快照去找业务资料。
	"context": true,
}

var (
	reID     = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	reOpenID = regexp.MustCompile(`^ou_`)

	reH2 = regexp.MustCompile(`(?m)^##\s+(.+?)\s*$`)
)

// ValidCompanyID 判断公司 id 合不合法。导出它是为了在生成骨架时就能拦住 ——
// 等渲染到一半才报，人已经填了一堆东西了。
func ValidCompanyID(s string) bool { return reID.MatchString(s) }

// MemberNameProblem 判断成员名能不能当**目录名**用（homes/<名>、members/<名>）与 project 名
// （<公司 id>-<名>）。返回 "" = 能用；否则给一句人话，直接进 member.name.format 的报错里。
//
// 尺子只量事实：这个名字会落成一段真实路径，三平台（Windows / Linux / macOS）都得建得出来。
// 所以中文、空格、- 、. 一律放行 —— 「名字是不是 ASCII」不是判据，那是**凭据键名**那一侧的事，
// 而键名的派生已经是全函数（render.FeishuSecretKey，见 runtime/DESIGN.md §7.1.16）。
//
// 拦的是真的会把路径撕开的那几种形状：
func MemberNameProblem(name string) string {
	switch {
	case name == "":
		return "名字是空的（拼不出路径）"
	case name == "." || name == "..":
		return "是 . 或 .. —— 路径会指到上一层去"
	case strings.ContainsAny(name, `/\`):
		return "含 / 或反斜杠 —— 会被当成两层路径，work_dir 落到别的目录"
	case strings.ContainsAny(name, `:*?"<>|`):
		return "含 Windows 建不出目录的字符（: * ? 双引号 < > |）"
	case strings.TrimSpace(name) != name:
		return "首尾有空白 —— 目录名与 project 名会带上看不见的字符"
	case strings.HasSuffix(name, "."):
		return "以 . 结尾 —— Windows 建目录时会把它吃掉，目录名与 project 名从此对不上"
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "含控制字符"
		}
	}
	return ""
}

// SplitRef 拆 kind:name 引用（member: / role: / domain: / project: …）；没有冒号 = 裸名。
//
// 导出它是为了让「怎么读一个引用」只有一处实现：org 的校验（hasActor）与 envelope 的
// on_behalf_of 解析（bind.resolves）必须解出同一个答案，两份实现迟早会漂。
// 裸名的**优先顺序由调用方定**（org 里是 成员 → 岗位；envelope 里是 成员 → 岗位 → 域），
// 这个函数只负责切开，不负责解释。
func SplitRef(s string) (kind, name string) {
	if i := strings.IndexByte(s, ':'); i > 0 {
		return strings.ToLower(strings.TrimSpace(s[:i])), strings.TrimSpace(s[i+1:])
	}
	return "", strings.TrimSpace(s)
}

// Load 读取并校验一棵 org 真相源树。
func Load(root string) (*Org, error) {
	o := &Org{Root: root, Roles: map[string]Role{}}

	cd, err := readDoc(filepath.Join(root, "company", "company.md"))
	if err != nil {
		return nil, err
	}
	c := Company{Platform: "feishu"}
	if c.Name, err = cd.Str("name"); err != nil {
		return nil, err
	}
	if c.ID, err = cd.Str("id"); err != nil {
		return nil, err
	}
	if c.Language, err = cd.Str("language"); err != nil {
		return nil, err
	}
	if c.Timezone, err = cd.Str("timezone"); err != nil {
		return nil, err
	}
	if c.Platform, err = cd.Str("platform"); err != nil {
		return nil, err
	}
	if c.Platform == "" {
		c.Platform = "feishu"
	}
	if c.Display, err = cd.Str("display"); err != nil {
		return nil, err
	}
	if c.Admins, err = cd.StrList("admins"); err != nil {
		return nil, err
	}
	if c.SyncIntervalMin, err = cd.Int("sync_interval_min"); err != nil {
		return nil, err
	}
	if c.Defaults.Model, err = cd.Str("defaults.model"); err != nil {
		return nil, err
	}
	if c.Defaults.Mode, err = cd.Str("defaults.mode"); err != nil {
		return nil, err
	}
	if c.Defaults.AutoCompressMaxTokens, err = cd.Int("defaults.auto_compress_max_tokens"); err != nil {
		return nil, err
	}
	if c.Defaults.ResetOnIdleMins, err = cd.Int("defaults.reset_on_idle_mins"); err != nil {
		return nil, err
	}
	if cd.Has("fallback_provider.name") || cd.Has("fallback_provider.base_url") || cd.Has("fallback_provider.model") {
		fp := &FallbackProvider{}
		if fp.Name, err = cd.Str("fallback_provider.name"); err != nil {
			return nil, err
		}
		if fp.BaseURL, err = cd.Str("fallback_provider.base_url"); err != nil {
			return nil, err
		}
		if fp.Model, err = cd.Str("fallback_provider.model"); err != nil {
			return nil, err
		}
		c.Fallback = fp
	}
	c.Body = cd.Body
	o.Company = c

	// 生效规则表：出厂默认 → company.md 的 policy 段覆盖。
	// 覆盖本身也要被校验（拼错规则 id = 你以为关了其实没关），所以它的发现跟主校验一起报。
	pol := DefaultPolicy()
	polIssues := pol.Apply(cd)
	o.Policy = pol

	sections := []string{"职责", "风格", "术语表"}
	if v, err := cd.StrList("persona_sections"); err != nil {
		return nil, err
	} else if len(v) > 0 {
		sections = v
	}
	allowedSection := map[string]bool{}
	for _, s := range sections {
		allowedSection[s] = true
	}

	roleDirs, err := subdirs(filepath.Join(root, "roles"))
	if err != nil {
		return nil, err
	}
	for _, name := range roleDirs {
		d, err := readDoc(filepath.Join(root, "roles", name, "persona.md"))
		if err != nil {
			return nil, err
		}
		r := Role{Role: name, Sections: map[string]string{}}
		if v, err := d.Str("role"); err != nil {
			return nil, err
		} else if v != "" {
			r.Role = v
		}
		if r.Title, err = d.Str("title"); err != nil {
			return nil, err
		}
		if r.Model, err = d.Str("model"); err != nil {
			return nil, err
		}
		if r.Mode, err = d.Str("mode"); err != nil {
			return nil, err
		}
		if r.AllowedTools, err = d.StrList("allowed_tools"); err != nil {
			return nil, err
		}
		if r.VaultScope, err = d.StrList("vault_scope"); err != nil {
			return nil, err
		}
		if r.Skills, err = d.StrList("skills"); err != nil {
			return nil, err
		}
		r.Sections = map[string]string{}
		for _, s := range splitH2(d.Body) {
			if allowedSection[s.Name] {
				r.Sections[s.Name] = s.Text
				continue
			}
			polIssues = append(polIssues, pol.Issue("role.persona.section_unknown",
				"roles/"+name+"/persona.md",
				"正文出现未约定的 H2 段「%s」；白名单是 %s —— 角色层不许私藏事实（canonical registry 纪律）。要放行就在 company.md 写 persona_sections，或在 policy 段把本规则降到 warn/off",
				s.Name, strings.Join(sections, " / ")))
			r.Extra = append(r.Extra, s)
		}
		o.Roles[r.Role] = r
	}

	memberDirs, err := subdirs(filepath.Join(root, "members"))
	if err != nil {
		return nil, err
	}
	for _, dir := range memberDirs {
		d, err := readDoc(filepath.Join(root, "members", dir, "persona.md"))
		if err != nil {
			return nil, err
		}
		m := Member{Name: dir, Body: d.Body}
		if v, err := d.Str("name"); err != nil {
			return nil, err
		} else if v != "" {
			m.Name = v
		}
		if m.DisplayName, err = d.Str("display_name"); err != nil {
			return nil, err
		}
		if m.Role, err = d.Str("role"); err != nil {
			return nil, err
		}
		if m.Model, err = d.Str("model"); err != nil {
			return nil, err
		}
		if m.Admin, err = d.Bool("admin"); err != nil {
			return nil, err
		}
		if m.Unwired, err = d.Bool("unwired"); err != nil {
			return nil, err
		}
		if m.Disabled, err = d.Bool("disabled"); err != nil {
			return nil, err
		}
		if m.Domains, err = d.StrList("domains"); err != nil {
			return nil, err
		}
		if m.Feishu.AppID, err = d.Str("feishu.app_id"); err != nil {
			return nil, err
		}
		if m.Feishu.OpenID, err = d.Str("feishu.open_id"); err != nil {
			return nil, err
		}
		if m.Feishu.ExtraAllowFrom, err = d.StrList("feishu.extra_allow_from"); err != nil {
			return nil, err
		}
		if m.Feishu.AllowChat, err = d.StrList("feishu.allow_chat"); err != nil {
			return nil, err
		}
		o.Members = append(o.Members, m)
	}
	sort.Slice(o.Members, func(i, j int) bool { return o.Members[i].Name < o.Members[j].Name })

	if o.Routing, err = scanRouting(root); err != nil {
		return nil, err
	}
	// 域表（罗盘）。文件不在 = 这家公司还没划域，不算错（存量 vault 向后兼容）。
	// 它的发现与 policy 段的问题同一批回显：都是「真相源本身」的问题，不是 persona 文本问题。
	domains, domIssues, err := LoadDomains(root, pol)
	if err != nil {
		return nil, err
	}
	o.Domains = domains
	polIssues = append(polIssues, domIssues...)
	// 项目表（立项书汇总）。同域表一档：文件不在 = 还没建表，不算错（存量 vault 向后兼容）。
	// 它是「谁在做哪个项目」的唯一机器落点 —— 所以和域表一样，只有一处维护。
	projects, projIssues, err := LoadProjects(root, pol)
	if err != nil {
		return nil, err
	}
	o.Projects = projects
	// 业务 agent 表（SPEC §2.3 第二类）。同域表 / 项目表一档：文件不在 = 还没有业务 agent，不算错。
	// 它的落点**只有这一处** —— 「谁代哪条流程在跑」和「谁在做哪个项目」是两件事，不共表。
	agents, agentIssues, err := LoadAgents(root, pol)
	if err != nil {
		return nil, err
	}
	o.Agents = agents
	polIssues = append(polIssues, agentIssues...)
	polIssues = append(polIssues, projIssues...)
	// 立项书副本落点（charters/）。同域表一档：目录不在 = 还没有副本，不算错 ——
	// 真源在客户侧，副本是保障。这一层只负责「落点在哪、名字对不对」；
	// 「谁在什么时候把副本放进来」是运行态的事，不在这一层（阶段 C）。
	if o.Charters, err = ScanCharters(root); err != nil {
		return nil, err
	}
	// 授权表（grants/）。判据与上面三处同一个：目录不在 = 还没建表，不算错 ——
	// 「没有 grant 就是不通」是口径，不是缺陷（SPEC §6 不变量 1）。
	// 一个 grant 一个文件，扫描递归：分层是为了人快速定位，不是为了代码 —— 换分层不用改代码。
	grants, grantIssues, err := ScanGrants(root, pol)
	if err != nil {
		return nil, err
	}
	o.Grants = grants
	polIssues = append(polIssues, grantIssues...)
	rep := o.Validate(pol)
	o.validateDomains(pol, rep)
	o.validateProjects(pol, rep)
	o.validateAgents(pol, rep)
	o.validateCharters(pol, rep)
	o.validateGrants(pol, rep)
	rep.Merge(&Report{Issues: polIssues})
	if len(rep.Fatal()) > 0 {
		return nil, &LoadError{Report: rep}
	}
	o.Warnings = rep.Warns()
	return o, nil
}

// Enabled 返回启用中的成员（disabled = 离职停用而不删档）。
func (o *Org) Enabled() []Member {
	out := make([]Member, 0, len(o.Members))
	for _, m := range o.Members {
		if !m.Disabled {
			out = append(out, m)
		}
	}
	return out
}

// Member 按 name 取成员。
func (o *Org) Member(name string) (Member, bool) {
	for _, m := range o.Members {
		if m.Name == name {
			return m, true
		}
	}
	return Member{}, false
}

// AdminLabels 把 company.admins 解成「人名（岗位）」，供**给人看**的地方使用
// （探针报红要说清交给谁、看板运行态页同理）。
//
// 取不到成员（名字写错 / 没有 display_name）时**原样返回那个名字** ——
// 报红时宁可给一个光秃秃的标识符，也不要去猜一个可能是错的人。
func (o *Org) AdminLabels() []string {
	out := make([]string, 0, len(o.Company.Admins))
	for _, name := range o.Company.Admins {
		m, ok := o.Member(name)
		if !ok || m.DisplayName == "" {
			out = append(out, name)
			continue
		}
		label := m.DisplayName
		if m.Role != "" {
			label += "（" + m.Role + "）"
		}
		out = append(out, label)
	}
	return out
}

// ModelFor 三级回退：member.model ∥ role.model ∥ company.defaults.model。
func (o *Org) ModelFor(m Member) string {
	if m.Model != "" {
		return m.Model
	}
	if r, ok := o.Roles[m.Role]; ok && r.Model != "" {
		return r.Model
	}
	return o.Company.Defaults.Model
}

// Validate 校验全量口径。一次返回所有发现，再由规则表的档次分流：红的拦，其余回显。
func (o *Org) Validate(p *Policy) *Report {
	rep := &Report{}
	companyFile := "company/company.md"

	if o.Company.Name == "" {
		rep.Add(p.Issue("company.name.missing", companyFile, "缺 name"))
	}
	if !reID.MatchString(o.Company.ID) {
		rep.Add(p.Issue("company.id.format", companyFile,
			"id=%q 必须是 ASCII 前缀（%s），它会进 launchd label 与项目名前缀", o.Company.ID, reID))
	}
	if o.Company.Platform != "feishu" {
		rep.Add(p.Issue("company.platform.unsupported", companyFile, "platform=%q，v1 只支持 feishu", o.Company.Platform))
	}
	if o.Company.Defaults.Model == "" {
		rep.Add(p.Issue("company.defaults.model.missing", companyFile,
			"缺 defaults.model（成员与角色都没写 model 时没有回退值）"))
	}
	if o.Company.Defaults.Mode == "bypassPermissions" {
		rep.Add(p.Issue("company.defaults.mode.bypass", companyFile,
			"defaults.mode 不许是 bypassPermissions（SPEC §6-5，无豁免开关）"))
	}
	// 上游的规矩是 reset_on_idle_mins must be >= 0，写负它**拒绝启动** —— 那是「让在跑的 bot 下线」，
	// 按 rules.go 的判据归红档（0 = 关掉空闲重置，是正当值）。
	if o.Company.Defaults.ResetOnIdleMins < 0 {
		rep.Add(p.Issue("company.defaults.reset_on_idle.invalid", companyFile,
			"defaults.reset_on_idle_mins=%d 不能为负（0 = 关掉空闲重置）", o.Company.Defaults.ResetOnIdleMins))
	}
	// 这个值直接进 cc-connect 的 [display].mode；上游只认 full / compact / quiet，
	// 写错它**拒绝启动** —— 不是「显示难看」，是全部 bot 下线，所以是红档。
	if !ValidDisplayMode(o.Company.Display) {
		rep.Add(p.Issue("company.display.invalid", companyFile,
			"display=%q 不认识：cc-connect 只吃 %s（不写 = 出厂默认 %s）",
			o.Company.Display, strings.Join(DisplayModes, " / "), DisplayQuiet))
	}

	var devbots int
	seenAppID := map[string]string{}
	seenName := map[string]bool{}
	seenOpenID := map[string]string{}
	for _, m := range o.Members {
		where := "members/" + m.Name + "/persona.md"
		if seenName[m.Name] {
			rep.Add(p.Issue("member.name.duplicate", where, "成员 id 重复"))
		}
		seenName[m.Name] = true
		if problem := MemberNameProblem(m.Name); problem != "" {
			rep.Add(p.Issue("member.name.format", where,
				"name=%q %s。它会落成 work_dir 的 homes/<名>、project 名 <%s>-<名> 与 members/<名>/ —— 名字得是三平台都建得出来的一段路径。中文 / 空格 / - / . 都合法（凭据键名另走 render.FeishuSecretKey，全函数派生，中文名也写得出键）；改成一个当得了目录名的名字重跑",
				m.Name, problem, o.Company.ID))
		}
		if m.DisplayName == "" {
			rep.Add(p.Issue("member.display_name.missing", where, "缺 display_name"))
		}
		if m.Role == "devbot" && !m.Disabled {
			devbots++
		}
		if m.Feishu.OpenID == "" {
			rep.Add(p.Issue("member.feishu.open_id.missing", where,
				"缺 feishu.open_id（渲染 allow_from 用；发 /whoami 给 bot 可取）"))
		} else if !reOpenID.MatchString(m.Feishu.OpenID) {
			rep.Add(p.Issue("member.feishu.open_id.format", where, "feishu.open_id=%q 不符合 ^ou_", m.Feishu.OpenID))
		} else if prev, dup := seenOpenID[m.Feishu.OpenID]; dup {
			rep.Add(p.Issue("member.feishu.open_id.duplicate", where, "feishu.open_id 与 members/%s 重复", prev))
		} else {
			seenOpenID[m.Feishu.OpenID] = m.Name
		}
		if m.Feishu.AppID == "" {
			rep.Add(p.Issue("member.feishu.app_id.missing", where, "缺 feishu.app_id"))
		} else if prev, dup := seenAppID[m.Feishu.AppID]; dup {
			rep.Add(p.Issue("member.feishu.app_id.duplicate", where, "feishu.app_id 与 members/%s 重复（app_id 必须全局唯一）", prev))
		} else {
			seenAppID[m.Feishu.AppID] = m.Name
		}
		for _, s := range append([]string{m.Feishu.OpenID}, append(m.Feishu.ExtraAllowFrom, m.Feishu.AllowChat...)...) {
			if s == "*" {
				rep.Add(p.Issue("feishu.wildcard", where, "allow_from / allow_chat 出现 \"*\"（等于对所有人开放，SPEC §6-1）"))
			}
		}
		// 角色相关检查放最后：角色不存在时，成员自身的问题也要一并报出来。
		r, ok := o.Roles[m.Role]
		if !ok {
			rep.Add(p.Issue("member.role.missing", where, "role=%q 在 roles/ 下不存在", m.Role))
			continue
		}
		if r.Mode == "bypassPermissions" && m.Role != "devbot" {
			rep.Add(p.Issue("role.bypass.not_devbot", "roles/"+m.Role+"/persona.md",
				"mode=bypassPermissions 只有 devbot 可以有（SPEC §6-5）"))
		}
		if m.Role != "devbot" && len(r.AllowedTools) == 0 {
			rep.Add(p.Issue("role.allowed_tools.empty", "roles/"+m.Role+"/persona.md",
				"allowed_tools 为空：dontAsk 下没预授权的工具会被自动拒绝，bot 连得上却干不了活（实测）；请填这个角色真正需要的工具名，或在 company.md 里显式覆盖这一档"))
		}
	}
	if devbots != 1 {
		rep.Add(p.Issue("company.devbot.count", companyFile, "全公司必须恰好一个启用中的 devbot，当前 %d 个", devbots))
	}
	for _, a := range o.Company.Admins {
		m, ok := o.Member(a)
		if !ok {
			rep.Add(p.Issue("company.admins.unknown", companyFile, "admins 里的 %q 不是任何成员", a))
			continue
		}
		if m.Disabled {
			rep.Add(p.Issue("company.admins.disabled", companyFile, "admins 里的 %q 已停用", a))
		}
	}
	for _, m := range o.Members {
		if _, ok := o.Roles[m.Role]; ok && o.ModelFor(m) == "" {
			rep.Add(p.Issue("member.model.unresolved", "members/"+m.Name,
				"没写 model，role 与 company 也没写，无处回退"))
		}
	}
	return rep
}

// InputsHash 是「重渲染判据」的输入指纹：org 树 + 本机层输入。
func (o *Org) InputsHash(h Host) string {
	var b strings.Builder
	b.WriteString("v8\n") // v8：业务 agent 表加 role / app_id(Q23 已拍)（v7 是 agents.md 与 host-agent-home 进场）
	fmt.Fprintf(&b, "host|%s|%s|%s\n", h.VaultRoot, h.HomesRoot, h.DataDir)
	for _, slug := range sortedMapKeys(h.AgentHomes) {
		fmt.Fprintf(&b, "host-agent-home|%s|%s\n", slug, h.AgentHomes[slug])
	}
	c := o.Company
	fmt.Fprintf(&b, "company|%s|%s|%s|%s|%s|%s|%v|%d|%s|%s|%d|%s\n",
		c.Name, c.ID, c.Language, c.Timezone, c.Platform, c.Display, c.Admins, c.SyncIntervalMin,
		c.Defaults.Model, c.Defaults.Mode, c.Defaults.AutoCompressMaxTokens, c.Body)
	for _, name := range sortedKeys(o.Roles) {
		r := o.Roles[name]
		fmt.Fprintf(&b, "role|%s|%s|%s|%s|%v|%v|%v|%s|%s|%s|%v\n", r.Role, r.Title, r.Model, r.Mode,
			r.AllowedTools, r.VaultScope, r.Skills, r.Sections["职责"], r.Sections["风格"], r.Sections["术语表"], r.Extra)
	}
	for _, m := range o.Members {
		fmt.Fprintf(&b, "member|%s|%s|%s|%s|%v|%v|%v|%v|%s|%s|%v|%v|%s\n", m.Name, m.DisplayName, m.Role, m.Model,
			m.Admin, m.Disabled, m.Unwired, m.Domains, m.Feishu.AppID, m.Feishu.OpenID, m.Feishu.ExtraAllowFrom, m.Feishu.AllowChat, m.Body)
	}
	for _, r := range o.Routing {
		fmt.Fprintf(&b, "routing|%s|%s\n", r.Dir, r.Summary)
	}
	for _, d := range o.Domains {
		fmt.Fprintf(&b, "domain|%s|%s|%s|%s|%s|%s|%s\n", d.Slug, d.Name, d.What, d.Who, d.Data, d.Sources, d.Terms)
	}
	for _, pr := range o.Projects {
		fmt.Fprintf(&b, "project|%s|%s|%s|%s|%s|%s\n",
			pr.Slug, pr.Name, pr.Domain, pr.Owner, pr.Period, pr.Source)
	}
	for _, c := range o.Charters {
		for _, a := range o.Agents {
			fmt.Fprintf(&b, "agent|%s|%s|%s|%s|%s|%s|%s|%s\n",
				a.Slug, a.Name, a.Domain, a.Harness, a.Tools, a.Model, a.Role, a.AppID)
		}
		fmt.Fprintf(&b, "charter|%s\n", c.Slug)
	}
	// 授权表也进指纹：它同样是「加载出来的组织状态」的一部分，改了要能被人看见。
	// 零条授权的 vault 不会因此多出任何字节 —— 存量指纹不变，所以 v5 不必跳版本。
	for _, g := range o.Grants {
		fmt.Fprintf(&b, "grant|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s\n",
			g.Path, g.Slug, g.From, g.To, g.Action, g.Object, g.TTL, g.OnBehalfOf, g.Reason, g.Body)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// sortedMapKeys 是 string→string 的稳定键序（指纹与报错文案都要同输入同输出）。
func sortedMapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedKeys(m map[string]Role) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// RoleKeys 是全部角色 id（已排序）。渲染、看板、报错文案共用这一处 ——
// 免得每多一个消费面，就多一份「怎么排序」的小抄。
func (o *Org) RoleKeys() []string { return sortedKeys(o.Roles) }

func readDoc(path string) (*Doc, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("缺文件: %s", path)
		}
		return nil, err
	}
	d, err := Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

func subdirs(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("缺目录: %s", dir)
		}
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// splitH2 把正文按 H2 段全量切开，保留声明顺序。
// 段名合不合规由调用方按规则表定档 —— 这里不丢内容：静默丢掉作者写的段，
// 等于偷偷改了真相源。
func splitH2(body string) []Section {
	idx := reH2.FindAllStringSubmatchIndex(body, -1)
	out := make([]Section, 0, len(idx))
	for i, loc := range idx {
		start := loc[1]
		end := len(body)
		if i+1 < len(idx) {
			end = idx[i+1][0]
		}
		out = append(out, Section{Name: body[loc[2]:loc[3]], Text: strings.TrimSpace(body[start:end])})
	}
	return out
}

// scanRouting 扫 vault 顶层数据目录，取目录 CLAUDE.md 的首句作说明。
func scanRouting(root string) ([]Routing, error) {
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []Routing
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || skipDirs[e.Name()] {
			continue
		}
		summary := ""
		if b, err := os.ReadFile(filepath.Join(root, e.Name(), "CLAUDE.md")); err == nil {
			summary = routingSummary(string(b))
		}
		out = append(out, Routing{Dir: e.Name(), Summary: summary})
	}
	return out, nil
}

// routingSummary 从数据目录的 CLAUDE.md 里取「这个目录放什么」—— 它会整行进 persona 段 3「数据来源」。
//
// 跳过四类行，每一类都是踩过的坑，别顺手删掉：
//   - 空行、`#` 标题：标题是名字不是说明（没写说明时显示 —，不是显示名字）；
//   - `<...>` 占位符整行：脚手架留下的「这里还没写」。它长得像说明，进去却是一条**假路** ——
//     agent 会照表去一个根本没被描述清楚的地方翻。存量 vault 里已经有这种文件，
//     所以这层判断不能只靠改模板（改模板只管以后新生成的）。
//   - `<!-- ... -->` 注释（含跨行）：注释是写给自己看的，模板提示就藏在这里，不该当说明。
//
// 一条有效行都没有时返回空串，由渲染层显示 `—`。
func routingSummary(text string) string {
	inComment := false
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if inComment {
			i := strings.Index(line, "-->")
			if i < 0 {
				continue
			}
			inComment = false
			line = strings.TrimSpace(line[i+3:])
		}
		if i := strings.Index(line, "<!--"); i >= 0 {
			if j := strings.Index(line[i+4:], "-->"); j >= 0 {
				line = strings.TrimSpace(line[:i] + " " + line[i+4+j+3:])
			} else {
				inComment = true
				line = strings.TrimSpace(line[:i])
			}
		}
		line = strings.TrimSpace(strings.TrimLeft(line, "-*0123456789. "))
		if line == "" || strings.HasPrefix(line, "#") || isPlaceholder(line) {
			continue
		}
		return line
	}
	return ""
}

// isPlaceholder 认整行就是一个 `<尖括号包起来的话>` —— 模板脚手架的「还没写」。
func isPlaceholder(s string) bool {
	if len(s) < 2 || s[0] != '<' || s[len(s)-1] != '>' {
		return false
	}
	return strings.Count(s, "<") == 1 && strings.Count(s, ">") == 1
}
