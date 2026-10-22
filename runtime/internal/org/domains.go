package org

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Domain 是 domains.md 里的一行业务域 —— 罗盘的一行。
//
// 一张表同时服务三件事，所以只有一处维护：
//   - 上下文：渲染时整行注入该域成员的 persona，回答「这是哪块业务、数据在哪、找谁」；
//   - 权限：data 指向的目录就是这一域的作用域（§6 授权模型里的「域标签」客体）；
//   - 标识：slug 是文件名与代码里唯一的机器标识（ASCII —— 中文文件名会撞 macOS 的 NFC/NFD 与跨平台差异）。
//
// 「怎么做」不在这里：那是活的，预先写细必然过期，留给 agent 自己写的沉淀层。
type Domain struct {
	Slug    string // ASCII 标识：机器用它，人不用
	Name    string // 中文显示名
	What    string // 这块业务是什么、要产出什么（1 句）
	Who     string // 谁在做 / 有分歧听谁的：填岗位，人名渲染时派生
	Data    string // 数据在哪：vault 顶层目录名（是指针，不是说明 —— 目录的说明由路由表负责）
	Sources string // 可选：原件来自哪个外部渠道
	Terms   string // 可选：这一域特有的口径 / 术语
	Line    int    // 在 domains.md 里的行号（报错定位用）
}

// DomainsFile 是域表的唯一落点。
const DomainsFile = "domains.md"

// knownDomainColumns 是认得的列名；不认得的列忽略（表格可以多列备注，不影响解析）。
var knownDomainColumns = map[string]bool{
	"slug": true, "name": true, "what": true, "who": true,
	"data": true, "sources": true, "terms": true,
}

// LoadDomains 读 domains.md。文件不存在 = 没有业务域（向后兼容，不报错）。
// 只做「表本身」的检查；who / data 指向的东西是否存在，属于跨表校验，放在 Validate 里。
func LoadDomains(root string, p *Policy) ([]Domain, []Issue, error) {
	b, err := os.ReadFile(filepath.Join(root, DomainsFile))
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	head, cols := findTable(lines, knownDomainColumns, "slug")
	if head < 0 {
		return nil, []Issue{p.Issue("domain.table.missing", DomainsFile,
			"没有找到带 slug 列的表：域表必须是一张 markdown 表（第一行表头，第二行分隔）")}, nil
	}

	var (
		out    []Domain
		issues []Issue
		seen   = map[string]int{}
	)
	for i := head + 2; i < len(lines); i++ {
		cells := tableCells(lines[i])
		if cells == nil {
			break
		}
		d := Domain{Line: i + 1}
		get := rowReader(cols, cells)
		d.Slug, d.Name, d.What = get("slug"), get("name"), get("what")
		d.Who, d.Data = get("who"), get("data")
		d.Sources, d.Terms = get("sources"), get("terms")
		where := domainWhere(d.Line)

		if d.Slug == "" {
			if d.Name != "" || d.What != "" {
				issues = append(issues, p.Issue("domain.slug.format", where, "缺 slug（机器标识）；这一行其余内容将被忽略"))
			}
			continue
		}
		if !reID.MatchString(d.Slug) {
			issues = append(issues, p.Issue("domain.slug.format", where,
				"slug=%q 必须是 ASCII 小写标识（%s）—— 它要进文件名与代码，中文会撞 macOS 的 NFC/NFD 与跨平台差异", d.Slug, reID))
		}
		if prev, dup := seen[d.Slug]; dup {
			issues = append(issues, p.Issue("domain.slug.duplicate", where, "slug=%q 与第 %d 行重复", d.Slug, prev))
		} else {
			seen[d.Slug] = d.Line
		}
		if d.Name == "" {
			issues = append(issues, p.Issue("domain.name.missing", where, "slug=%q 缺 name（显示名）", d.Slug))
		}
		if d.What == "" {
			issues = append(issues, p.Issue("domain.what.missing", where, "域 %q 缺 what（这块业务是什么）—— agent 少一条判断口径的依据", d.Slug))
		}
		if d.Who == "" {
			issues = append(issues, p.Issue("domain.who.missing", where, "域 %q 缺 who（谁在做）—— 卡住时没人可找", d.Slug))
		}
		if d.Data == "" {
			issues = append(issues, p.Issue("domain.data.missing", where, "域 %q 缺 data（数据在哪）—— 这一域没有作用域", d.Slug))
		}
		out = append(out, d)
	}
	return out, issues, nil
}

func domainWhere(line int) string {
	return DomainsFile + " 第 " + itoa(line) + " 行"
}

// DomainWhere 是 domains.md 第 n 行的定位串。导出给渲染器复用，
// 让「域表的问题」和「域表注入 persona 之后的问题」报的是同一个位置。
func DomainWhere(line int) string { return domainWhere(line) }

// Domain 按 slug 取业务域。
func (o *Org) Domain(slug string) (Domain, bool) {
	for _, d := range o.Domains {
		if d.Slug == slug {
			return d, true
		}
	}
	return Domain{}, false
}

// validateDomains 做跨表校验：who 是不是真岗位、data 是不是真目录、成员写的域存不存在。
// 只发现事实，档次由规则表定（实现里不许硬编码档次）。
func (o *Org) validateDomains(p *Policy, rep *Report) {
	dirs := map[string]bool{}
	for _, r := range o.Routing {
		dirs[r.Dir] = true
	}
	for _, d := range o.Domains {
		where := domainWhere(d.Line)
		if d.Who != "" {
			if _, ok := o.Roles[d.Who]; !ok {
				rep.Add(p.Issue("domain.who.unknown_role", where,
					"who=%q 不在 roles/ 里（现有：%s）—— 渲染时解不出人，只会原样显示岗位名", d.Who, strings.Join(o.roleNames(), " / ")))
			}
		}
		if d.Data != "" && !dirs[d.Data] {
			rep.Add(p.Issue("domain.data.unknown_dir", where,
				"data=%q 不是 vault 的顶层数据目录（现有：%s）—— 这一域的作用域指不到地方", d.Data, strings.Join(o.routingDirs(), " / ")))
		}
	}
	seen := map[string]bool{}
	for _, d := range o.Domains {
		seen[d.Slug] = true
	}
	for _, m := range o.Members {
		for _, s := range m.Domains {
			if !seen[s] {
				rep.Add(p.Issue("member.domains.unknown", "members/"+m.Name+"/persona.md",
					"domains 里的 %q 不是已定义的业务域（见 %s）—— 拼错的域等于悄悄少给他一块业务", s, DomainsFile))
			}
		}
	}
}

// roleNames / routingDirs 给报错文案用（排序，保证同输入同输出）。
func (o *Org) roleNames() []string { return o.RoleKeys() }

func (o *Org) routingDirs() []string {
	out := make([]string, 0, len(o.Routing))
	for _, r := range o.Routing {
		out = append(out, r.Dir)
	}
	sort.Strings(out)
	return out
}

// WhoLabel 把域表 who（以及立项书 owner / members）里的**岗位**解成「人名（岗位）」。
//
// 表里只维护岗位，人名在渲染时从 members/ 现算 —— persona 与看板拿的是同一份派生结果，
// 所以两处都不会各存一份名单，也就不会互相漂移（SPEC §3.5 一处维护）。
func (o *Org) WhoLabel(role string) string {
	title := strings.TrimSpace(role)
	if r, ok := o.Roles[role]; ok && strings.TrimSpace(r.Title) != "" {
		title = strings.TrimSpace(r.Title)
	}
	if title == "" {
		return "—"
	}
	var names []string
	for _, m := range o.Enabled() {
		if m.Role != role {
			continue
		}
		if n := strings.TrimSpace(m.DisplayName); n != "" {
			names = append(names, n)
		} else {
			names = append(names, m.Name)
		}
	}
	if len(names) == 0 {
		return title + "（暂无人）"
	}
	return strings.Join(names, "、") + "（" + title + "）"
}

// MemberLabel 把成员编号解成「人名（岗位）」。
//
// 与 WhoLabel 的区别：WhoLabel 吃的是**岗位**（域表 who / 项目 owner），
// MemberLabel 吃的是**成员 name**（persona 的 reports_to）。两处都只做派生，
// 不在真相源里多存一份人名，免得改名后两处对不上。
func (o *Org) MemberLabel(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	m, ok := o.Member(name)
	if !ok {
		return ""
	}
	label := strings.TrimSpace(m.DisplayName)
	if label == "" {
		label = m.Name
	}
	if r, ok := o.Roles[m.Role]; ok && strings.TrimSpace(r.Title) != "" {
		label += "（" + strings.TrimSpace(r.Title) + "）"
	}
	return label
}
