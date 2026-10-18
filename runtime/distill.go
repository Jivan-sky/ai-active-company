package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"anc/internal/distill"
	"anc/internal/timeline"
)

// anc distill —— 只读：哪些事已经闭环、哪些技能还没落正文。
//
// 它**不写 skills/**：候选只是原料，值不值得写成技能由人判断。
const distillUsage = `anc distill —— 可沉淀候选与草稿

用法：
  anc distill ls <vault> [--limit 50] [--json]
  anc distill draft <vault> <case> [--name <技能名>] [--force]
  anc distill check <vault> <技能名> [--json]

口径：
  · 候选 = timeline 里**最后一条是绿档**（done）的 case；running / blocked / failed 不算。
  · 一个 case 多行时取最后一条当当前态；历史一行不丢。
  · signals 只陈述证据（几条推进 / detail 多少字 / 几个 refs / 同名重复几次），不打分。
  · 技能清单来自 roles/<role>/persona.md 的 skills:，正文看 skills/<名>/SKILL.md 在不在。
  · 正文存在但没有任何岗位声明的技能列为「孤儿」—— 它对 agent 不可见。
  · 这一条命令只读：不写 skills/、不改 timeline、不碰热层。

draft：
  · 只对**绿档闭环**的 case 生成草稿；queued / running / blocked / failed 一律拒绝。
  · 落点是 skills/_drafts/<技能名>.md —— 不是 skills/<技能名>/SKILL.md。
  · 草稿只装 timeline 里的事实（归属 / 证据 / 原始记录 / 引用），不编「怎么做」。
  · 名字默认从 case 推；推不出来或已有同名正式技能时，用 --name 另起一个。
  · 已存在同名草稿要覆盖时显式加 --force。

check：
  · 只读：不改草稿、不写 skills/、不登记 roles。
  · 拦发布（fail）：名字不合法 / 草稿不在 / frontmatter 对不上 / 四段缺内容 /
    占位符 / 明文命中脱敏规则 / 正式落点已被占用。
  · 只提醒（warn）：没有岗位声明（发布后 agent 取不到）/ 来源 case 对不上 / 正文有本机绝对路径。
  · 退出码：0 = 没有 fail（warn 仍可发布）· 1 = 有 fail · 2 = 用法错。
`

func cmdDistill(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, distillUsage)
		return 2
	}
	switch args[0] {
	case "ls", "list":
		return cmdDistillLs(args[1:])
	case "draft":
		return cmdDistillDraft(args[1:])
	case "check":
		return cmdDistillCheck(args[1:])
	}
	fmt.Fprintf(os.Stderr, "错误：不认识子命令 %q\n\n", args[0])
	fmt.Fprint(os.Stderr, distillUsage)
	return 2
}

func cmdDistillLs(args []string) int {
	fs := flag.NewFlagSet("distill ls", flag.ContinueOnError)
	limit := fs.Int("limit", 50, "最多列几条候选（1-500）")
	asJSON := fs.Bool("json", false, "输出 JSON（与 /api/distill 同一份契约）")
	flagArgs, posArgs := splitArgs(args, map[string]bool{"json": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) == 0 {
		fmt.Fprintln(os.Stderr, "错误：缺少 <vault>")
		return 2
	}
	if *limit < 1 || *limit > 500 {
		fmt.Fprintln(os.Stderr, "错误：--limit 要在 1-500 之间")
		return 2
	}
	abs, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	r := distill.Load(abs)
	if r.Error != "" {
		fmt.Fprintf(os.Stderr, "错误：timeline 读不动：%s\n", r.Error)
		return 1
	}
	if len(r.Candidates) > *limit {
		r.Candidates = r.Candidates[:*limit]
		r.Truncated = true
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			fmt.Fprintf(os.Stderr, "错误：%v\n", err)
			return 1
		}
		return 0
	}

	fmt.Println("anc distill ls —— 可沉淀候选（只读）")
	fmt.Println()
	fmt.Printf("case %d 个，其中闭环（绿档）%d 个；候选 %d 条%s\n",
		r.Cases, r.Green, r.CandidateTotal, truncNote(r))
	if len(r.Candidates) == 0 {
		fmt.Println("  （没有候选）")
	}
	for _, c := range r.Candidates {
		where := []string{}
		if c.Domain != "" {
			where = append(where, "域="+c.Domain)
		}
		if c.Project != "" {
			where = append(where, "项目="+c.Project)
		}
		if c.By != "" {
			where = append(where, "执行="+c.By)
		}
		fmt.Printf("  - [%s] %s\n", c.Case, c.Title)
		if len(where) > 0 {
			fmt.Println("      " + strings.Join(where, " · ") + " · " + c.At)
		} else {
			fmt.Println("      " + c.At)
		}
		fmt.Println("      " + strings.Join(c.Signals, " · "))
	}
	fmt.Println()
	if !r.SkillsWired {
		fmt.Println("技能覆盖：答不了（" + dashText(r.SkillsError) + "）")
	} else {
		missing := 0
		for _, s := range r.Skills {
			if s.Missing {
				missing++
			}
		}
		fmt.Printf("技能 %d 份（正文缺失 %d）\n", len(r.Skills), missing)
		for _, s := range r.Skills {
			mark := "✓"
			if s.Missing {
				mark = "✗"
			}
			fmt.Printf("  %s %-16s 岗位：%s\n", mark, s.Name, strings.Join(s.Roles, " / "))
			if s.Missing {
				fmt.Println("      正文没落：" + s.Where)
			}
		}
		if len(r.Orphans) > 0 {
			fmt.Printf("孤儿技能 %d 份（有正文、没有岗位声明，agent 取不到）：%s\n",
				len(r.Orphans), strings.Join(r.Orphans, ", "))
		}
	}
	if len(r.Gaps) > 0 {
		fmt.Println()
		fmt.Println("缺口（报缺，不编）：")
		for _, g := range r.Gaps {
			fmt.Println("  - " + g)
		}
	}
	fmt.Println()
	fmt.Println("要把某条候选变成草稿：anc distill draft <vault> <case>")
	fmt.Println("这一条只读：写不写技能由人判断，写入点是 skills/<名>/SKILL.md。")
	return 0
}

func cmdDistillDraft(args []string) int {
	fs := flag.NewFlagSet("distill draft", flag.ContinueOnError)
	name := fs.String("name", "", "技能名（默认从 case 推；小写 ASCII，字母开头）")
	force := fs.Bool("force", false, "同名草稿已存在时覆盖")
	flagArgs, posArgs := splitArgs(args, map[string]bool{"force": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) < 2 {
		fmt.Fprintln(os.Stderr, "错误：缺少 <vault> <case>")
		return 2
	}
	abs, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	key := strings.TrimSpace(posArgs[1])
	doc, err := timeline.Load(abs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：timeline 读不动：%v\n", err)
		return 1
	}
	d, err := distill.BuildDraft(doc.Entries, key)
	if err != nil {
		switch {
		case errors.Is(err, distill.ErrCaseNotFound):
			fmt.Fprintf(os.Stderr, "错误：timeline 里没有 case %q —— 先用 `anc timeline list` 看 key。\n", key)
		case errors.Is(err, distill.ErrCaseNotClosed):
			fmt.Fprintf(os.Stderr, "错误：case %q 还没闭环（最后一条不是绿档）—— 先让它成为终态（settle / 显式写 done）。\n", key)
		default:
			fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		}
		return 1
	}
	skill := strings.TrimSpace(*name)
	if skill == "" {
		skill = distill.Slug(key)
	}
	if !distill.ValidSkillName(skill) {
		fmt.Fprintf(os.Stderr, "错误：技能名 %q 不合法。必须是小写 ASCII（^[a-z][a-z0-9-]*$）、字母开头、最长 64 ——\n", skill)
		fmt.Fprintln(os.Stderr, "它会成为目录名；推不出来就用 --name 起一个。")
		return 2
	}
	official := filepath.Join(abs, "skills", skill, "SKILL.md")
	if _, err := os.Stat(official); err == nil {
		fmt.Fprintf(os.Stderr, "错误：skills/%s/SKILL.md 已经有正式技能正文 —— 换 --name，或直接改那一份。\n", skill)
		return 1
	}
	out := filepath.Join(abs, "skills", "_drafts", skill+".md")
	if _, err := os.Stat(out); err == nil && !*force {
		fmt.Fprintf(os.Stderr, "错误：草稿 skills/_drafts/%s.md 已存在（要覆盖加 --force）。\n", skill)
		return 1
	}
	md := d.Markdown(skill, time.Now().Format(time.RFC3339))
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	if err := os.WriteFile(out, []byte(md), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	fmt.Println("anc distill draft —— 生成沉淀草稿（不碰正式技能）")
	fmt.Printf("  case  %s\n", d.Case.Case)
	fmt.Printf("  名称  %s\n", skill)
	fmt.Printf("  落点  skills/_drafts/%s.md\n", skill)
	fmt.Printf("  事实  %s\n", strings.Join(d.Case.Signals, " · "))
	fmt.Printf("  正文  %d 字节\n", len(md))
	fmt.Println("下一步：补「何时用 / 输入 / 步骤 / 验证」四段，再写到 skills/<名>/SKILL.md。")
	return 0
}

func cmdDistillCheck(args []string) int {
	fs := flag.NewFlagSet("distill check", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "输出 JSON")
	flagArgs, posArgs := splitArgs(args, map[string]bool{"json": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) < 2 {
		fmt.Fprintln(os.Stderr, "错误：缺少 <vault> <技能名>")
		return 2
	}
	abs, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	name := strings.TrimSpace(posArgs[1])
	if !distill.ValidSkillName(name) {
		fmt.Fprintf(os.Stderr, "错误：技能名 %q 不合法。必须是小写 ASCII（^[a-z][a-z0-9-]*$）、字母开头、最长 64。\n", name)
		return 2
	}
	r := distill.LoadCheck(abs, name)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			fmt.Fprintf(os.Stderr, "错误：%v\n", err)
			return 1
		}
		if !r.Ok {
			return 1
		}
		return 0
	}
	fmt.Println("anc distill check —— 发布前校验（只读，不写 skills/）")
	fmt.Println()
	fmt.Printf("  name    %s\n", r.Name)
	fmt.Printf("  草稿    %s\n", r.Draft)
	fmt.Printf("  正式    %s\n", r.Official)
	fmt.Println()
	for _, it := range r.Items {
		mark := "✓"
		switch it.Level {
		case distill.LevelWarn:
			mark = "!"
		case distill.LevelFail:
			mark = "✗"
		}
		fmt.Printf("  %s %s\n", mark, it.Title)
		if strings.TrimSpace(it.Detail) != "" {
			fmt.Printf("      %s\n", it.Detail)
		}
	}
	fmt.Println()
	if r.Ok {
		if r.Warns > 0 {
			fmt.Printf("结论：可以发布（%d 项提醒）\n", r.Warns)
		} else {
			fmt.Println("结论：可以发布")
		}
	} else if r.Warns > 0 {
		fmt.Printf("结论：不能发布（%d 项失败 · %d 项提醒）\n", r.Fails, r.Warns)
	} else {
		fmt.Printf("结论：不能发布（%d 项失败）\n", r.Fails)
	}
	fmt.Println(r.Note)
	if !r.Ok {
		return 1
	}
	return 0
}

func truncNote(r distill.Report) string {
	if !r.Truncated {
		return ""
	}
	return "（只列前 " + strconv.Itoa(len(r.Candidates)) + " 条）"
}

func dashText(s string) string {
	if strings.TrimSpace(s) == "" {
		return "没有给原因"
	}
	return s
}
