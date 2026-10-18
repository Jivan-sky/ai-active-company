package distill

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"anc/internal/audit"
	"anc/internal/org"
	"anc/internal/timeline"
)

// CheckSchema 是发布前校验的版本号。
const CheckSchema = "anc.distill.check/v1"

// 三档：fail 才拦发布；warn 是「发得出去，但会出问题」；ok 是这一项过了。
const (
	LevelOK   = "ok"
	LevelWarn = "warn"
	LevelFail = "fail"
)

// RequiredSections 是技能正文的四段 —— 草稿模板与检查共用这一份清单。
var RequiredSections = []string{"何时用", "输入", "步骤", "验证"}

// CheckItem 是一条检查结果。ID 稳定，便于机器过滤与回归。
type CheckItem struct {
	ID     string `json:"id"`
	Level  string `json:"level"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

// CheckReport 是 `anc distill check` 与未来看板共用的形状。
type CheckReport struct {
	Schema   string      `json:"schema"`
	Name     string      `json:"name"`
	Draft    string      `json:"draft"`
	Official string      `json:"official"`
	Ok       bool        `json:"ok"`
	Fails    int         `json:"fails"`
	Warns    int         `json:"warns"`
	Items    []CheckItem `json:"items"`
	Note     string      `json:"note"`
}

// CheckInput 是纯函数输入：文件系统在 LoadCheck 里读完。
type CheckInput struct {
	Name string

	DraftExists bool
	Draft       string

	OfficialExists bool

	RolesWired bool
	Roles      []RoleSkills

	TimelineWired bool
	Entries       []timeline.Entry
}

// Check 做发布前体检。只读：不修草稿、不写 skills/、不登记 roles。
//
// 拦发布（fail）的只有「写出去会坏」的几件事：名字不合法、草稿不在、frontmatter 对不上、
// 四段缺内容、占位符、明文命中了脱敏规则、正式落点已被占用。
// 「发出去 agent 也看不见」（没有岗位声明）与「来源对不上」只提醒 —— 那是人的决定，
// 不是代码该替人拦的门。
func Check(in CheckInput) CheckReport {
	r := CheckReport{
		Schema: CheckSchema, Name: in.Name,
		Draft:    "skills/_drafts/" + in.Name + ".md",
		Official: "skills/" + in.Name + "/SKILL.md",
		Items:    []CheckItem{},
		Note:     "这一条只读：不改草稿、不写 skills/、不登记 roles。",
	}
	add := func(id, level, title, detail string) {
		r.Items = append(r.Items, CheckItem{ID: id, Level: level, Title: title, Detail: detail})
	}

	if !ValidSkillName(in.Name) {
		add("name.valid", LevelFail, "技能名不合法", "必须是小写 ASCII（^[a-z][a-z0-9-]*$）、字母开头、最长 64")
		return finish(r)
	}
	add("name.valid", LevelOK, "技能名合法", in.Name)

	if !in.DraftExists {
		add("draft.exists", LevelFail, "草稿不存在", r.Draft+" —— 先用 anc distill draft 生成")
	} else {
		add("draft.exists", LevelOK, "草稿存在", r.Draft)
		fm, fmOK := parseFrontmatter(in.Draft)
		switch {
		case !fmOK:
			add("draft.frontmatter", LevelFail, "frontmatter 读不懂", "第一行必须是 ---，并用第二个 --- 收尾")
		default:
			if got := strings.TrimSpace(fm["name"]); got != in.Name {
				add("draft.name_matches", LevelFail, "frontmatter 的 name 与文件名不一致", "文件是 "+in.Name+"，里面写的是 "+quoteOrEmpty(got))
			} else {
				add("draft.name_matches", LevelOK, "frontmatter 与文件名一致", in.Name)
			}
		}
		secs := parseSections(in.Draft)
		for _, name := range RequiredSections {
			body := strings.TrimSpace(secs[name])
			if body == "" {
				add("draft.section."+name, LevelFail, "「"+name+"」缺内容", "发布前必须写清楚；草稿只装事实，这一段要人补")
				continue
			}
			add("draft.section."+name, LevelOK, "「"+name+"」有内容", firstLineOfBody(body))
		}
		if tok := firstPlaceholder(in.Draft); tok != "" {
			add("draft.placeholder", LevelFail, "正文里还有占位符", "命中："+tok+" —— 发布前删掉或写完")
		} else {
			add("draft.placeholder", LevelOK, "没有占位符", "TODO / TBD / FIXME / 待补 / 以后补")
		}
		if _, hits := audit.Effective().Apply(in.Draft); len(hits) > 0 {
			add("draft.secret", LevelFail, "正文里有会被脱敏规则命中的内容", "规则："+strings.Join(hits, " / ")+" —— 明文不该写进技能正文")
		} else {
			add("draft.secret", LevelOK, "没有命中脱敏规则", "按出厂规则表 + 本机凭据名")
		}
		if lines := absolutePathLines(in.Draft); len(lines) > 0 {
			add("draft.absolute_path", LevelWarn, "正文里有本机绝对路径", "第 "+joinInts(lines)+" 行 —— 技能正文应写 vault 相对指针，换台机器会失效")
		} else {
			add("draft.absolute_path", LevelOK, "没有本机绝对路径", "")
		}
	}

	if in.OfficialExists {
		add("official.collision", LevelFail, "正式落点已被占用", r.Official+" 已存在 —— 要么改 --name，要么直接改那一份，别用草稿覆盖")
	} else {
		add("official.collision", LevelOK, "不撞正式技能", r.Official+" 还不存在")
	}

	if !in.RolesWired {
		add("roles.wired", LevelWarn, "岗位清单读不动", "无法确认发布后有没有岗位声明它")
	} else if roles := declaringRoles(in.Roles, in.Name); len(roles) == 0 {
		add("roles.declared", LevelWarn, "没有任何岗位声明这个技能", "发布后 agent 取不到 —— 在 roles/<岗位>/persona.md 的 skills: 里登记")
	} else {
		add("roles.declared", LevelOK, "已被岗位声明", strings.Join(roles, " / "))
	}

	if !in.TimelineWired {
		add("case.provenance", LevelWarn, "timeline 读不动", "无法核对来源 case")
	} else if c := findGreenCase(in.Entries, in.Name, in.Draft); c == "" {
		add("case.provenance", LevelWarn, "来源 case 对不上", "frontmatter 里的 case 不在绿档闭环里（或被删了）—— 来源无法回溯")
	} else {
		add("case.provenance", LevelOK, "来源 case 是绿档", c)
	}
	return finish(r)
}

// LoadCheck 读 vault 里的草稿 / 正式落点 / 岗位 / timeline，再交给 Check。
func LoadCheck(vault, name string) CheckReport {
	in := CheckInput{Name: name}
	if !ValidSkillName(name) {
		return Check(in)
	}
	if b, err := os.ReadFile(filepath.Join(vault, "skills", "_drafts", name+".md")); err == nil {
		in.DraftExists = true
		in.Draft = string(b)
	}
	if _, err := os.Stat(filepath.Join(vault, "skills", name, "SKILL.md")); err == nil {
		in.OfficialExists = true
	}
	if doc, err := timeline.Load(vault); err == nil {
		in.TimelineWired = true
		in.Entries = doc.Entries
	}
	if o, err := org.Load(vault); err == nil {
		in.RolesWired = true
		for role, rs := range o.Roles {
			in.Roles = append(in.Roles, RoleSkills{Role: role, Skills: rs.Skills})
		}
		sort.Slice(in.Roles, func(i, j int) bool { return in.Roles[i].Role < in.Roles[j].Role })
	}
	return Check(in)
}

func finish(r CheckReport) CheckReport {
	for _, it := range r.Items {
		switch it.Level {
		case LevelFail:
			r.Fails++
		case LevelWarn:
			r.Warns++
		}
	}
	r.Ok = r.Fails == 0
	return r
}

func parseFrontmatter(md string) (map[string]string, bool) {
	s := strings.TrimPrefix(md, "\ufeff")
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, false
	}
	out := map[string]string{}
	for i := 1; i < len(lines); i++ {
		ln := strings.TrimSpace(lines[i])
		if ln == "---" {
			return out, true
		}
		k, v, ok := strings.Cut(ln, ":")
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key != "" {
			out[key] = val
		}
	}
	return nil, false
}

var h2RE = regexp.MustCompile(`^##\s+(.+?)\s*$`)

// parseSections 把正文按二级标题切开；自闭合的 ``` 代码块里不切。
func parseSections(md string) map[string]string {
	out := map[string]string{}
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	cur := ""
	var b strings.Builder
	fence := false
	flush := func() {
		if cur != "" {
			out[cur] = strings.TrimSpace(b.String())
		}
		b.Reset()
	}
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "```") {
			fence = !fence
			b.WriteString(ln + "\n")
			continue
		}
		if !fence {
			if m := h2RE.FindStringSubmatch(ln); m != nil {
				flush()
				cur = strings.TrimSpace(m[1])
				continue
			}
		}
		if cur != "" {
			b.WriteString(ln + "\n")
		}
	}
	flush()
	return out
}

func firstPlaceholder(md string) string {
	up := strings.ToUpper(md)
	for _, tok := range []string{"TODO", "TBD", "FIXME"} {
		if strings.Contains(up, tok) {
			return tok
		}
	}
	for _, tok := range []string{"待补", "以后补"} {
		if strings.Contains(md, tok) {
			return tok
		}
	}
	return ""
}

var absPathRE = regexp.MustCompile(`(?i)([A-Za-z]:\\|/Users/|/home/|/mnt/)`)

func absolutePathLines(md string) []int {
	var out []int
	for i, ln := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		if absPathRE.MatchString(ln) {
			out = append(out, i+1)
		}
	}
	return out
}

func declaringRoles(roles []RoleSkills, name string) []string {
	var out []string
	for _, rs := range roles {
		for _, s := range rs.Skills {
			if strings.TrimSpace(s) == name {
				out = append(out, rs.Role)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

func findGreenCase(entries []timeline.Entry, name, draft string) string {
	fm, ok := parseFrontmatter(draft)
	if !ok {
		return ""
	}
	key := strings.TrimSpace(fm["case"])
	if key == "" {
		return ""
	}
	for _, c := range Build(Input{Entries: entries, TimelineWired: true}).Candidates {
		if c.Case == key {
			return key
		}
	}
	return ""
}

func firstLineOfBody(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			if len([]rune(t)) > 60 {
				return string([]rune(t)[:60]) + "…"
			}
			return t
		}
	}
	return ""
}

func quoteOrEmpty(s string) string {
	if s == "" {
		return "（空）"
	}
	return s
}

func joinInts(v []int) string {
	parts := make([]string, 0, len(v))
	for _, n := range v {
		parts = append(parts, strconv.Itoa(n))
	}
	return strings.Join(parts, ", ")
}
