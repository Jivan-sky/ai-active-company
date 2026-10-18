// Package reportimport 把日报里的「已经发生的事实」与「还没做完的待办」
// 解析成可导入 ANC 的两类记录：timeline 条目与任务入队。
//
// 三条口径：
//  1. 只认日报自己的结构（章节 / 项目子标题 / checkbox / 待办表），
//     不用模型猜「这句话是不是任务」—— 猜出来的东西一旦进队列就是假工作。
//  2. 同一条待办在多份日报里反复出现时，以**最后一次出现**为准：
//     最后一次仍是 [ ] 才入队；后面已 [x] 的算历史，不再派单。
//  3. 解析不了的文件不静默跳过：在 FileStat 里写明原因，
//     由调用方决定要不要报给人。少导一条比导错一条便宜。
package reportimport

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const (
	SectionDone     = "done"
	SectionDecision = "decision"
	SectionTodo     = "todo"

	StateOpen    = "open"
	StateDone    = "done"
	StateDropped = "dropped"
)

// Item 是日报里的一条事实或一条待办。字段全是数据：加字段不用迁移。
type Item struct {
	Date     string `json:"date"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Section  string `json:"section"`
	State    string `json:"state"`
	Project  string `json:"project,omitempty"`
	Title    string `json:"title"`
	Detail   string `json:"detail,omitempty"`
	Priority string `json:"priority,omitempty"`
}

// Report 是一份日报的解析结果。
type Report struct {
	Name     string
	Date     string
	Title    string
	Tags     []string
	Items    []Item
	Warnings []string
}

// FileStat 是「这份文件导出了什么」的一行 —— 没导出东西也要有一行。
type FileStat struct {
	Name  string `json:"name"`
	Date  string `json:"date,omitempty"`
	Items int    `json:"items"`
	Note  string `json:"note,omitempty"`
}

// Plan 是一次导入的完整计划：写什么、派什么、跳过什么。
type Plan struct {
	Entries    []Item
	Tasks      []Item
	Skipped    []Item
	Superseded int
	Files      []FileStat
	Warnings   []string
}

var (
	reReportName = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\.md$`)
	reBullet     = regexp.MustCompile(`^\s*[-*]\s+(.+?)\s*$`)
	reCheck      = regexp.MustCompile(`^\[([ xX])\]\s*(.*)$`)
	reParen      = regexp.MustCompile(`（[^）]*）|\([^)]*\)`)
)

// Parse 解析一份日报。name 只用于回显与稳定 id（调用方传相对路径）。
func Parse(name string, raw []byte) Report {
	r := Report{Name: name}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(text, "\n")

	// frontmatter：只认 created / title / tags。读不懂就不猜。
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i < len(lines) && strings.TrimSpace(lines[i]) == "---" {
		i++
		inTags := false
		for ; i < len(lines); i++ {
			line := strings.TrimRight(lines[i], " ")
			if strings.TrimSpace(line) == "---" {
				i++
				break
			}
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, "tags:") {
				inTags = true
				continue
			}
			if inTags && strings.HasPrefix(trim, "- ") {
				r.Tags = append(r.Tags, strings.TrimSpace(trim[2:]))
				continue
			}
			inTags = false
			if k, v, ok := strings.Cut(trim, ":"); ok {
				v = strings.Trim(strings.TrimSpace(v), `"'`)
				switch strings.TrimSpace(k) {
				case "created":
					r.Date = v
				case "title":
					r.Title = v
				}
			}
		}
	}

	if r.Date == "" {
		if m := reReportName.FindStringSubmatch(baseName(name)); m != nil {
			r.Date = m[1]
		}
	}
	if r.Date == "" {
		r.Warnings = append(r.Warnings, "读不出日期（frontmatter 没有 created，文件名也不是 YYYY-MM-DD.md）")
		return r
	}

	tagProject := projectFromTags(r.Tags)
	project := tagProject
	section := ""
	inSub := false
	sawSection := false
	var tableHeader []string
	inTable := false

	for n := i; n < len(lines); n++ {
		line := lines[n]
		trim := strings.TrimSpace(line)

		if strings.HasPrefix(trim, "## ") {
			section = sectionOf(strings.TrimSpace(trim[3:]))
			if section != "" {
				sawSection = true
			}
			project, inSub = tagProject, false
			tableHeader, inTable = nil, false
			continue
		}
		if strings.HasPrefix(trim, "### ") && section != "" {
			if p := stripDecor(strings.TrimSpace(trim[4:])); p != "" {
				project = p
			}
			inSub = true
			tableHeader, inTable = nil, false
			continue
		}
		// 分隔线把「子标题 + 说明」这段收口：后面的 bullet 回到顶层。
		if section != "" && isRule(trim) {
			project, inSub = tagProject, false
			tableHeader, inTable = nil, false
			continue
		}
		if section == "" {
			continue
		}

		if section == SectionTodo && strings.HasPrefix(trim, "|") {
			cells := splitRow(trim)
			if !inTable {
				if hasCell(cells, "任务") {
					tableHeader = cells
					inTable = true
				}
				continue
			}
			if isSepRow(cells) {
				continue
			}
			it, ok := tableItem(r.Date, name, n+1, project, tableHeader, cells)
			if !ok {
				continue
			}
			r.Items = append(r.Items, it)
			continue
		}
		tableHeader, inTable = nil, false

		m := reBullet.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		body := strings.TrimSpace(m[1])
		switch section {
		case SectionDone, SectionDecision:
			body = stripCheckbox(body)
			title, detail := splitTitleDetail(body)
			if title == "" {
				continue
			}
			r.Items = append(r.Items, Item{
				Date: r.Date, File: name, Line: n + 1, Section: section, State: StateDone,
				Project: project, Title: title, Detail: detail,
			})
		case SectionTodo:
			state, rest := todoState(body)
			if rest == "" {
				continue
			}
			if state == "" {
				// 子标题下的说明性 bullet 不是待办；只有顶层没写 checkbox 的才按未完成收。
				if inSub {
					continue
				}
				state = StateOpen
				r.Warnings = append(r.Warnings, fmt.Sprintf("第 %d 行待办没有 checkbox，按未完成处理", n+1))
			}
			title, detail := splitTitleDetail(rest)
			if title == "" {
				continue
			}
			r.Items = append(r.Items, Item{
				Date: r.Date, File: name, Line: n + 1, Section: section, State: state,
				Project: project, Title: title, Detail: detail,
			})
		}
	}

	if len(r.Items) == 0 && len(r.Warnings) == 0 {
		if sawSection {
			r.Warnings = append(r.Warnings, "章节在，但没有可导入的 bullet / 表格")
		} else {
			r.Warnings = append(r.Warnings, "没有可识别的章节（完成事项 / 关键决策 / 待办）")
		}
	}
	return r
}

// Build 把多份日报折叠成一份导入计划。
//
// 待办按「标题」归并：标题里的括号说明常改（今天写原因、明天写进展），
// 归并时去掉括号再比，避免同一件事每天都被当成新任务。
// 不按项目归并 —— 日报里的 project tag 时常缺一份，带上项目就会把同一件事拆成两条。
func Build(reports []Report) Plan {
	cp := append([]Report(nil), reports...)
	sort.SliceStable(cp, func(i, j int) bool {
		if cp[i].Date != cp[j].Date {
			return cp[i].Date < cp[j].Date
		}
		return cp[i].Name < cp[j].Name
	})

	var p Plan
	latest := map[string]Item{}
	order := []string{}
	todoTotal := 0

	for _, r := range cp {
		p.Files = append(p.Files, FileStat{Name: r.Name, Date: r.Date, Items: len(r.Items), Note: firstWarning(r)})
		p.Warnings = append(p.Warnings, prefixWarnings(r)...)
		for _, it := range r.Items {
			if it.Section != SectionTodo {
				continue
			}
			todoTotal++
			k := todoKey(it.Title)
			if _, ok := latest[k]; !ok {
				order = append(order, k)
			}
			if old, ok := latest[k]; !ok || it.Date >= old.Date {
				latest[k] = it
			}
		}
	}

	for _, r := range cp {
		for _, it := range r.Items {
			if it.Section == SectionTodo {
				continue
			}
			p.Entries = append(p.Entries, it)
		}
	}

	for _, k := range order {
		it := latest[k]
		switch it.State {
		case StateOpen:
			p.Tasks = append(p.Tasks, it)
		case StateDone:
			p.Entries = append(p.Entries, it)
		default:
			p.Skipped = append(p.Skipped, it)
		}
	}
	p.Superseded = todoTotal - len(latest)
	if p.Superseded < 0 {
		p.Superseded = 0
	}
	return p
}

// EntryID 是稳定 id：同一份文件同一行的同一条事实，重导多少次都是同一个 id。
func EntryID(it Item) string {
	sum := sha256.Sum256([]byte(it.File + "\x00" + fmt.Sprint(it.Line) + "\x00" + it.Section + "\x00" + it.Title))
	return fmt.Sprintf("rpt-%s-%s-%s", it.Date, shortSection(it.Section), hex.EncodeToString(sum[:])[:10])
}

// CaseID 把同一天同一个项目的记录折成一个 case（看板按 case 折叠当前态）。
func CaseID(it Item) string {
	if strings.TrimSpace(it.Project) == "" {
		return "report:" + it.Date
	}
	return "report:" + it.Date + "#" + it.Project
}

// Kind 是 timeline 的 kind 约定：决策与执行分开，别的没有。
func Kind(it Item) string {
	if it.Section == SectionDecision {
		return "decision"
	}
	return "execution"
}

func shortSection(s string) string {
	switch s {
	case SectionDecision:
		return "dc"
	case SectionTodo:
		return "td"
	default:
		return "ex"
	}
}

func sectionOf(s string) string {
	s = stripDecor(s)
	switch {
	case strings.Contains(s, "完成"):
		return SectionDone
	case strings.Contains(s, "决策"):
		return SectionDecision
	case strings.Contains(s, "待办"):
		return SectionTodo
	default:
		return ""
	}
}

func projectFromTags(tags []string) string {
	for _, t := range tags {
		if strings.HasPrefix(t, "project/") {
			return strings.TrimSpace(strings.TrimPrefix(t, "project/"))
		}
	}
	return ""
}

// stripDecor 去掉标题前的 emoji / 符号与空白，留下人能读的那段。
func stripDecor(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		r := runes[i]
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			break
		}
		i++
	}
	return strings.TrimSpace(string(runes[i:]))
}

func stripCheckbox(s string) string {
	if m := reCheck.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[2])
	}
	return s
}

// todoState 返回 (状态, 正文)。空状态 = 没有 checkbox，由调用方决定怎么报。
func todoState(s string) (string, string) {
	if m := reCheck.FindStringSubmatch(s); m != nil {
		if strings.EqualFold(m[1], "x") {
			return StateDone, strings.TrimSpace(m[2])
		}
		return StateOpen, strings.TrimSpace(m[2])
	}
	if strings.HasPrefix(strings.TrimSpace(s), "~~") {
		return StateDropped, strings.TrimSpace(strings.Trim(strings.TrimSpace(s), "~"))
	}
	return "", s
}

func splitTitleDetail(s string) (string, string) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "**") {
		if j := strings.Index(s[2:], "**"); j >= 0 {
			title := strings.TrimSpace(s[2 : 2+j])
			rest := strings.TrimSpace(s[2+j+2:])
			rest = strings.TrimLeft(rest, " \u3000：:—-·、")
			if title != "" {
				return title, rest
			}
		}
	}
	for _, sep := range []string{"——", "：", ": "} {
		if i := strings.Index(s, sep); i > 0 {
			return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+len(sep):])
		}
	}
	return s, ""
}

func todoKey(title string) string {
	s := reParen.ReplaceAllString(title, "")
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '#' {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func isRule(s string) bool {
	n := 0
	for _, r := range s {
		if r != '-' {
			return false
		}
		n++
	}
	return n >= 3
}

func splitRow(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "|")
	s = strings.TrimSuffix(s, "|")
	parts := strings.Split(s, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func hasCell(cells []string, want string) bool {
	for _, c := range cells {
		if strings.Contains(c, want) {
			return true
		}
	}
	return false
}

func isSepRow(cells []string) bool {
	for _, c := range cells {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		for _, r := range c {
			if r != '-' && r != ':' && r != ' ' {
				return false
			}
		}
	}
	return true
}

func tableItem(date, file string, line int, project string, header, cells []string) (Item, bool) {
	col := func(name string) int {
		for i, h := range header {
			if strings.Contains(h, name) {
				return i
			}
		}
		return -1
	}
	ti, si := col("任务"), col("状态")
	if ti < 0 || si < 0 || ti >= len(cells) || si >= len(cells) {
		return Item{}, false
	}
	title := strings.TrimSpace(cells[ti])
	if title == "" {
		return Item{}, false
	}
	rawState := strings.TrimSpace(cells[si])
	state := StateOpen
	switch {
	case strings.Contains(rawState, "✅"):
		state = StateDone
	case strings.Contains(rawState, "~~"):
		state = StateDropped
	}
	detail := ""
	if pi := col("优先级"); pi >= 0 && pi < len(cells) {
		detail = "优先级：" + strings.TrimSpace(cells[pi])
	}
	if src := col("来源"); src >= 0 && src < len(cells) && strings.TrimSpace(cells[src]) != "" {
		if detail != "" {
			detail += "；"
		}
		detail += "来源：" + strings.TrimSpace(cells[src])
	}
	return Item{
		Date: date, File: file, Line: line, Section: SectionTodo, State: state,
		Project: project, Title: title, Detail: detail,
	}, true
}

func baseName(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func firstWarning(r Report) string {
	if len(r.Items) > 0 {
		return ""
	}
	if len(r.Warnings) > 0 {
		return r.Warnings[0]
	}
	return ""
}

func prefixWarnings(r Report) []string {
	out := make([]string, 0, len(r.Warnings))
	for _, w := range r.Warnings {
		out = append(out, r.Name+": "+w)
	}
	return out
}
