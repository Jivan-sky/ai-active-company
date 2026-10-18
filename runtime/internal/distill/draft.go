package distill

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"anc/internal/timeline"
)

var (
	// ErrCaseNotFound：timeline 里根本没有这个 case（写法错，或还没记过）。
	ErrCaseNotFound = errors.New("timeline 里没有这个 case")
	// ErrCaseNotClosed：case 在，但最后一条不是绿档 —— 没闭环的事不该长成草稿。
	ErrCaseNotClosed = errors.New("这个 case 还没闭环（最后一条不是绿档）")
)

// Draft 是一份「沉淀草稿」的全部事实：候选本身 + 它的全部流水（不是 Recent 截断的那 5 条）。
//
// 它**不是技能**：这里只有 timeline 里已经写下的事实，没有「怎么做」。
// 「怎么做」要靠接手的人补 —— 运行时不替人编步骤。
type Draft struct {
	Case Candidate
	Rows []timeline.Entry
}

// BuildDraft 按与候选表完全同一把尺子挑 case：Fold 后最后一条是绿档。
// 这样草稿不会从「候选表里没有的东西」长出来。
func BuildDraft(entries []timeline.Entry, caseKey string) (Draft, error) {
	key := strings.TrimSpace(caseKey)
	if key == "" {
		return Draft{}, ErrCaseNotFound
	}
	var rows []timeline.Entry
	for _, e := range entries {
		if e.Key() == key {
			rows = append(rows, e)
		}
	}
	if len(rows) == 0 {
		return Draft{}, ErrCaseNotFound
	}
	rep := Build(Input{Entries: entries, TimelineWired: true})
	var cand *Candidate
	for i := range rep.Candidates {
		if rep.Candidates[i].Case == key {
			cand = &rep.Candidates[i]
			break
		}
	}
	if cand == nil {
		return Draft{}, ErrCaseNotClosed
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ti, _ := rows[i].Time()
		tj, _ := rows[j].Time()
		return ti.Before(tj)
	})
	return Draft{Case: *cand, Rows: rows}, nil
}

// Markdown 渲染草稿正文。name 是技能名（已校验），at 是生成时刻。
func (d Draft) Markdown(name, at string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\nname: %s\nstatus: draft\ncase: %s\ngenerated_at: %s\nsource: timeline\n---\n\n", name, d.Case.Case, at)
	fmt.Fprintf(&b, "# %s\n\n", oneLine(d.Case.Title))
	b.WriteString("这份是**草稿**：只装 timeline 里可核对的事实，不是可执行技能。\n")
	fmt.Fprintf(&b, "要发布，先补「何时用 / 输入 / 步骤 / 验证」四段，再落到 `skills/%s/SKILL.md`。\n\n", name)

	b.WriteString("## 归属\n")
	if d.Case.Domain != "" {
		fmt.Fprintf(&b, "- 业务域：%s\n", oneLine(d.Case.Domain))
	}
	if d.Case.Project != "" {
		fmt.Fprintf(&b, "- 项目：%s\n", oneLine(d.Case.Project))
	}
	if d.Case.By != "" {
		fmt.Fprintf(&b, "- 执行：%s\n", oneLine(d.Case.By))
	}
	fmt.Fprintf(&b, "- 最后活动：%s\n", d.Case.At)
	fmt.Fprintf(&b, "- 推进记录：%d 条\n", d.Case.Entries)

	b.WriteString("\n## 证据\n")
	for _, s := range d.Case.Signals {
		fmt.Fprintf(&b, "- %s\n", s)
	}

	b.WriteString("\n## 原始记录\n")
	for _, e := range d.Rows {
		fmt.Fprintf(&b, "\n### %s · %s · %s\n", e.At, oneLine(e.Status), oneLine(e.By))
		fmt.Fprintf(&b, "%s\n", oneLine(e.Title))
		if detail := strings.TrimSpace(e.Detail); detail != "" {
			fmt.Fprintf(&b, "\n%s\n", detail)
		}
	}

	refs := draftRefs(d.Rows)
	if len(refs) > 0 {
		b.WriteString("\n## 引用\n")
		for _, r := range refs {
			fmt.Fprintf(&b, "- %s\n", r)
		}
	}

	b.WriteString("\n## 发布步骤\n")
	b.WriteString("1. 补齐「何时用 / 输入 / 步骤 / 验证」四段 —— 这份草稿只装事实，没有编这些。\n")
	fmt.Fprintf(&b, "2. 写到 `skills/%s/SKILL.md`。\n", name)
	b.WriteString("3. 在 `roles/<岗位>/persona.md` 的 `skills:` 里登记。\n")
	b.WriteString("4. 跑 `anc distill ls <vault>`：这个名字不该再报 missing，也不该出现在 orphans。\n")
	return b.String()
}

var skillNameRE = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ValidSkillName 是技能名的唯一门槛：小写 ASCII、字母开头、只含 a-z0-9-、最长 64。
// 它同时是目录名 —— 让中文名或大写名进来，落地时会在文件系统上出问题。
func ValidSkillName(s string) bool {
	return len(s) <= 64 && skillNameRE.MatchString(s)
}

// Slug 从 case key 推一个候选名：小写、非 [a-z0-9] 压成连字符、首尾去连字符。
// 推不出来就回空串 —— 调用方据此要求 `--name`，不替人编一个名字。
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 64 {
		out = strings.Trim(out[:64], "-")
	}
	return out
}

func draftRefs(rows []timeline.Entry) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range rows {
		for _, r := range e.Refs {
			s := strings.TrimSpace(r)
			if s == "" || seen[s] {
				continue
			}
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}
