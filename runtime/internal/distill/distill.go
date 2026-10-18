// Package distill —— 「已经闭环的事」离「可复用的技能」还差哪一步。
//
// 它只回答两件事，且只读：
//  1. 哪些 case 已经闭环（绿档）—— 这些是沉淀的**原料**，不是资产本身；
//  2. 岗位声明的技能里，哪些正文没落、哪些正文没有任何岗位声明（对 agent 不可见）。
//
// 刻意不做的事：不读会话、不替人总结、不打分排序、不写 skills/。
// 「值不值得写成技能」是人的判断；这一层只把可核对的事实摆出来。
package distill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"anc/internal/org"
	"anc/internal/timeline"
)

// Schema 是这一份视图的版本号（同 /api/assets：字段增删一律改版本号）。
const Schema = "anc.distill/v1"

// RoleSkills 是一个岗位声明的技能清单（roles/<role>/persona.md 的 skills:）。
type RoleSkills struct {
	Role   string
	Skills []string
}

// Input 是 Build 的全部输入 —— 文件系统在 Load 里读完，Build 只做纯计算。
type Input struct {
	Entries       []timeline.Entry
	TimelineWired bool
	SkillsWired   bool
	Roles         []RoleSkills
	Bodies        []string // skills/<name>/SKILL.md 确实存在的名字
}

// Report 是 `/api/distill` 与 `anc distill ls` 的共同形状。
type Report struct {
	Schema         string      `json:"schema"`
	Wired          bool        `json:"wired"`
	SkillsWired    bool        `json:"skills_wired"`
	Cases          int         `json:"cases"`
	Green          int         `json:"green"`
	Candidates     []Candidate `json:"candidates"`
	CandidateTotal int         `json:"candidate_total"`
	Truncated      bool        `json:"truncated,omitempty"`
	Skills         []Skill     `json:"skills"`
	Orphans        []string    `json:"orphans"`
	Gaps           []string    `json:"gaps"`
	Note           string      `json:"note"`
	Error          string      `json:"error,omitempty"`
	SkillsError    string      `json:"skills_error,omitempty"`
}

// Candidate 是一个已闭环的 case。
//
// 字段全部是从 timeline 现算的事实：没有「可复用度」「价值分」这种编出来的判断。
// Signals 只陈述证据（几条推进 / 多少字 / 几个引用 / 同名重复几次），
// 供人决定要不要把它写成技能。
type Candidate struct {
	Case        string   `json:"case"`
	Title       string   `json:"title"`
	Domain      string   `json:"domain,omitempty"`
	Project     string   `json:"project,omitempty"`
	By          string   `json:"by,omitempty"`
	At          string   `json:"at"`
	Entries     int      `json:"entries"`
	DetailChars int      `json:"detail_chars"`
	Refs        int      `json:"refs"`
	SameTitle   int      `json:"same_title"`
	SameDomain  int      `json:"same_domain"`
	Signals     []string `json:"signals"`
}

// Skill 是一份岗位声明的技能 + 正文在不在 + 谁声明了它。
type Skill struct {
	Name    string   `json:"name"`
	Where   string   `json:"where"`
	Missing bool     `json:"missing"`
	Roles   []string `json:"roles"`
}

// Build 把 timeline 流水与岗位技能清单折成一份只读报告。
//
// 候选口径只有一条：**最后一条是绿档**（done）。queued / running / blocked / failed
// 都不算 —— 没闭环的事还不是原料；要终止的事得先显式结账成终态。
func Build(in Input) Report {
	r := Report{
		Schema: Schema, Wired: in.TimelineWired, SkillsWired: in.SkillsWired,
		Candidates: []Candidate{}, Skills: []Skill{}, Orphans: []string{}, Gaps: []string{},
		Note: "候选 = 已闭环（绿档）的 case，只按最后活动时间排；不打分、不排优先级 —— " +
			"值不值得写成 skill 由人判断。这一页只读，不写 skills/。",
	}

	cases := timeline.Fold(in.Entries)
	r.Cases = len(cases)
	byKey := map[string][]timeline.Entry{}
	for _, e := range in.Entries {
		k := e.Key()
		byKey[k] = append(byKey[k], e)
	}

	var green []timeline.Case
	titleCount := map[string]int{}
	domainCount := map[string]int{}
	for _, c := range cases {
		if c.Band != timeline.BandGreen {
			continue
		}
		green = append(green, c)
		titleCount[normTitle(c.Title)]++
		if strings.TrimSpace(c.Domain) != "" {
			domainCount[strings.TrimSpace(c.Domain)]++
		}
	}
	r.Green = len(green)

	for _, c := range green {
		rows := byKey[c.Key]
		chars := 0
		refs := map[string]bool{}
		for _, e := range rows {
			chars += len([]rune(strings.TrimSpace(e.Detail)))
			for _, ref := range e.Refs {
				if s := strings.TrimSpace(ref); s != "" {
					refs[s] = true
				}
			}
		}
		sameTitle := titleCount[normTitle(c.Title)]
		sameDomain := 0
		if d := strings.TrimSpace(c.Domain); d != "" {
			sameDomain = domainCount[d]
		}
		signals := []string{fmt.Sprintf("%d 条推进", c.Entries)}
		if chars > 0 {
			signals = append(signals, fmt.Sprintf("detail %d 字", chars))
		}
		if len(refs) > 0 {
			signals = append(signals, fmt.Sprintf("refs %d 个", len(refs)))
		}
		if sameTitle > 1 {
			signals = append(signals, fmt.Sprintf("同名 %d 次", sameTitle))
		}
		if sameDomain > 1 {
			signals = append(signals, fmt.Sprintf("同域 %d 条", sameDomain))
		}
		r.Candidates = append(r.Candidates, Candidate{
			Case: c.Key, Title: c.Title, Domain: c.Domain, Project: c.Project,
			By: c.By, At: c.LastAt, Entries: c.Entries,
			DetailChars: chars, Refs: len(refs),
			SameTitle: sameTitle, SameDomain: sameDomain,
			Signals: signals,
		})
	}
	r.CandidateTotal = len(r.Candidates)

	if !in.TimelineWired {
		r.Gaps = append(r.Gaps, "timeline：读不动 —— 这一页答不了「哪些事已闭环」")
	} else if len(in.Entries) == 0 {
		r.Gaps = append(r.Gaps, "timeline：还没有任何记录 —— 现在没有候选，不等于「没有可沉淀的资产」")
	} else if len(green) == 0 {
		r.Gaps = append(r.Gaps, fmt.Sprintf("timeline：有 %d 条记录，但还没有绿档 case —— 没有候选，不等于「没有可沉淀的资产」", len(in.Entries)))
	}

	if !in.SkillsWired {
		r.Gaps = append(r.Gaps, "技能覆盖：org 真相源读不动 —— 这一段答不了")
		return r
	}
	skillRows(in, &r)
	return r
}

// skillRows 摊开岗位声明的技能，并把「有正文但没人声明」的孤立技能单列出来。
func skillRows(in Input, r *Report) {
	present := map[string]bool{}
	for _, b := range in.Bodies {
		if s := strings.TrimSpace(b); s != "" {
			present[s] = true
		}
	}
	rolesOf := map[string][]string{}
	var names []string
	for _, rs := range in.Roles {
		role := strings.TrimSpace(rs.Role)
		for _, raw := range rs.Skills {
			name := strings.TrimSpace(raw)
			if name == "" {
				continue
			}
			if _, ok := rolesOf[name]; !ok {
				names = append(names, name)
			}
			if !contains(rolesOf[name], role) {
				rolesOf[name] = append(rolesOf[name], role)
			}
		}
	}
	sort.Strings(names)
	for _, name := range names {
		roles := rolesOf[name]
		sort.Strings(roles)
		s := Skill{Name: name, Where: "skills/" + name + "/SKILL.md", Missing: !present[name], Roles: roles}
		r.Skills = append(r.Skills, s)
		if s.Missing {
			r.Gaps = append(r.Gaps, "技能正文没落："+s.Where+"（岗位："+strings.Join(roles, " / ")+"）")
		}
	}
	for _, b := range in.Bodies {
		if _, ok := rolesOf[b]; ok {
			continue
		}
		r.Orphans = append(r.Orphans, b)
	}
	sort.Strings(r.Orphans)
}

// Load 读 vault 的 timeline、org 真相源与 skills 正文，再交给 Build。
func Load(vault string) Report {
	in := Input{}
	doc, err := timeline.Load(vault)
	if err != nil {
		r := Build(in)
		r.Error = err.Error()
		return r
	}
	in.TimelineWired = true
	in.Entries = doc.Entries

	o, err := org.Load(vault)
	if err != nil {
		r := Build(in)
		r.SkillsError = firstLine(err.Error())
		r.Gaps = append(r.Gaps, "技能覆盖：org 真相源读不动（"+r.SkillsError+"）—— 这一段答不了")
		return r
	}
	in.SkillsWired = true
	for name, role := range o.Roles {
		in.Roles = append(in.Roles, RoleSkills{Role: name, Skills: role.Skills})
	}
	sort.Slice(in.Roles, func(i, j int) bool { return in.Roles[i].Role < in.Roles[j].Role })
	in.Bodies = Bodies(vault)
	return Build(in)
}

// Bodies 列出 skills/<name>/SKILL.md 确实存在的技能名。
// 目录不存在 = 一份正文都没落 = 空清单，不是错（刚 init 的库就是这样）。
func Bodies(vault string) []string {
	dir := filepath.Join(vault, "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Name(), "SKILL.md")); err != nil {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

func normTitle(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
