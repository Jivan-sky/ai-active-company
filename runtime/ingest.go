package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"anc/internal/audit"
	"anc/internal/org"
)

// ingest —— 外部材料进真相源的**唯一机器入口**（议题 #47）。
//
// 真相源的写者只许有两类：① 人手改 markdown ② 这条管道。
// 第三方系统（含客户自己的知识库）**不许直接写** —— 那就是第二个真相源，
// 两边迟早各说各话，而「哪边是真的」到那天没人答得出来。
//
// 这条管道只做三件事：**校验 → 原子写 → 记一条审计**。
// 智能那半（harness 出提案）只提案、不落盘，落盘由这里做 ——
// 与「agent 只能提案、不能行使」是同一条边界（#32 / #34）。
//
// 判据只有一条：**改完之后的树合不合法**（`anc org check` 同一张规则表）。
// 所以是「改在临时副本上 → 校验 → 通过才落真 vault」，而不是「先落盘、不对再回滚」：
// 红档那一刻真 vault 一行都没动。
const ingestUsage = `anc ingest —— 外部材料进真相源的**唯一机器入口**

用法：
  anc ingest <vault> --from <提案.json|-> [--dry-run] [--json]

提案（JSON；多出来的字段一律报错 —— 拼错的字段不许静默成「没写」）：
  {
    "by": "张三",                        谁提的（必填）
    "why": "立项会那份材料",             附注（可选）
    "writes": [                          改哪些文件（至少一条）
      {"path": "projects.md", "content": "……整份文件……"}
    ]
  }

content 是**整份文件**的全文，不是补丁 —— 补丁要匹配上下文，人写错一行整条都落不下去。
整份替换 + 「换完之后这棵树合不合法」这一条判据，与人手改 markdown 是同一把尺子；
git 那边照样只看得到一行 diff。

落点白名单（就这四处，别处**没有那个入口** —— 有第二个入口就等于有第二个真相源）：
  domains.md · projects.md · agents.md · members/<成员名>/<文件>

走的步骤：
  1. 提案形状 + 落点白名单 —— 不过就拒收，真 vault 一行都没动
  2. 把改写应用在 vault 的**临时副本**上，用 「anc org check」 同一张规则表校验
  3. 有红档 → 不落盘，退出码 1，逐条说清哪一条不过
  4. 通过 → 原子写回真 vault，并记一条审计（谁、何时、写了什么）
  ⚠️ **不 git commit** —— 历史归人，机器不写历史：落完自己 「git diff」 看一眼再提交。

选项：
  --from <文件|->   提案；- = 从 stdin 读
  --dry-run         只走到第 3 步：打印会动哪几个文件，一行不落
  --json            机器读的出口

退出码：0 落盘（dry-run 通过）· 1 校验不过 / 提案读不懂 / 环境错 · 2 用法错
`

// ingestWhitelist 是落点的**全部**范围，出错时就着它说话 —— 人就该在出错的地方看见白名单。
const ingestWhitelist = "domains.md · projects.md · agents.md · members/<成员名>/<文件>"

// maxProposal 是提案上限：提案里装的是整份文件，正常到不了这个数。
const maxProposal = 16 << 20

// ingestProposal 是一份提案。字段刻意只有三个 —— 多一个字段就多一处「填错了没人管」。
// 提案是**数据**，不是脚本：里面没有「怎么算」，只有「哪几个文件换成什么内容」。
type ingestProposal struct {
	By     string        `json:"by"` // 谁提的（必填：与信封「缺 who = 拒收」同一条口径）
	Why    string        `json:"why,omitempty"`
	Writes []ingestWrite `json:"writes"`
}

// ingestWrite 是一条改写：**整份文件**，不是补丁。
type ingestWrite struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ingestFileView 是「这次动了哪几个文件」的一条。change = new | changed | same。
type ingestFileView struct {
	Path   string `json:"path"`
	Change string `json:"change"`
	Bytes  int    `json:"bytes"`
}

type ingestOut struct {
	Vault   string           `json:"vault"`
	DryRun  bool             `json:"dry_run"`
	By      string           `json:"by"`
	Why     string           `json:"why,omitempty"`
	Files   []ingestFileView `json:"files"`
	Audit   string           `json:"audit,omitempty"`
	Warns   []string         `json:"warns,omitempty"`
	Refused []string         `json:"refused,omitempty"`
}

// cmdIngest 是这条管道的唯一入口。
func cmdIngest(args []string) int {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	from := fs.String("from", "", "提案文件（- = 从 stdin 读）")
	dry := fs.Bool("dry-run", false, "只校验不落盘")
	asJSON := fs.Bool("json", false, "机器读的出口")
	flagArgs, posArgs := splitArgs(args, map[string]bool{"dry-run": true, "json": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) != 1 || strings.TrimSpace(*from) == "" {
		fmt.Fprint(os.Stderr, ingestUsage)
		return 2
	}
	abs, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 2
	}
	raw, err := readProposal(*from)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 读不到提案（%s）: %v\n", *from, err)
		return 1
	}
	p, refused := parseProposal(raw)
	// Files 从空切片起步（不是 nil）：JSON 出口里出 `[]` 而不是 `null` ——
	// 机器读的那一侧不该为「一条改写都没有」多写一个 null 分支（与 roster / pending 同一条写法）。
	out := ingestOut{Vault: abs, DryRun: *dry, By: strings.TrimSpace(p.By), Why: strings.TrimSpace(p.Why), Files: []ingestFileView{}}
	// 形状与落点不过，就地拒收：真 vault 一行没动，临时副本也还没建。
	// **读不懂提案也算拒收**（不是环境错）：它和「落点不在白名单里」是同一件事 ——
	// 这份东西不该落进去。两个出口（文本 / --json）因此说的是同一份理由。
	if len(refused) > 0 {
		out.Refused = refused
		return emitIngest(out, *asJSON)
	}

	// 校验发生在**临时副本**上。真 vault 到第 4 步之前一行没动 ——
	// 「先落盘、不对再回滚」在进程被打断时会留下一棵半改的树，这里不给它这个机会。
	stage, err := os.MkdirTemp("", "anc-ingest-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 建不出临时副本: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(stage) }()
	tree := filepath.Join(stage, "vault")
	if err := copyTree(abs, tree); err != nil {
		fmt.Fprintf(os.Stderr, "错误: vault 复制到临时副本失败（校验要在一棵完整的树上做）: %v\n", err)
		return 1
	}
	for _, w := range p.Writes {
		dst := filepath.Join(tree, filepath.FromSlash(w.Path))
		change, err := ingestChange(dst, w.Content)
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %s 读不动: %v\n", w.Path, err)
			return 1
		}
		out.Files = append(out.Files, ingestFileView{Path: w.Path, Change: change, Bytes: len(w.Content)})
		if err := writeFileAtomic(dst, []byte(w.Content), 0o644, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "错误: 改写进不了临时副本（%s）: %v\n", w.Path, err)
			return 1
		}
	}
	o, lerr := org.Load(tree)
	if lerr != nil {
		var le *org.LoadError
		if errors.As(lerr, &le) {
			// 红档拦住：逐条说清哪一条不过，非红档也一并说出来（一次报全，别让人修一个才看见下一个）。
			out.Refused = org.IssueStrings(le.Report.Fatal())
			out.Warns = org.IssueStrings(le.Report.Warns())
			return emitIngest(out, *asJSON)
		}
		fmt.Fprintf(os.Stderr, "错误: 校验读不动临时副本: %v\n", lerr)
		return 1
	}
	out.Warns = org.IssueStrings(o.Warnings)

	if *dry || !ingestAnyChanged(out.Files) {
		return emitIngest(out, *asJSON)
	}
	for _, w := range p.Writes {
		dst := filepath.Join(abs, filepath.FromSlash(w.Path))
		if err := writeFileAtomic(dst, []byte(w.Content), 0o644, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "错误: 落盘失败 %s: %v —— 前面几个已经落了：先从 git 回退，别接着重跑\n", w.Path, err)
			return 1
		}
	}
	now := time.Now()
	rec := audit.Record{
		ID:     "ingest:" + now.Format(time.RFC3339Nano),
		At:     now.Format(time.RFC3339),
		Actor:  strings.TrimSpace(p.By),
		Action: audit.ActionWrite,
		Object: strings.Join(ingestPaths(p.Writes), ", "),
		Result: audit.ResultOK,
		Why:    strings.TrimSpace(p.Why),
		Source: "ingest",
		Ref:    *from,
		Detail: ingestDetail(out.Files),
	}
	path, aerr := audit.Append(abs, rec)
	if aerr != nil {
		fmt.Fprintf(os.Stderr, "错误: 文件落了，但审计没落 —— 补上审计再动别的（别重跑这份提案）: %v\n", aerr)
		return 1
	}
	out.Audit = path
	return emitIngest(out, *asJSON)
}

// emitIngest 出结果：JSON 出口与文本出口共用同一份 out（两个出口说的是同一件事）。
func emitIngest(out ingestOut, asJSON bool) int {
	if asJSON {
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			return 1
		}
		fmt.Println(string(b))
	} else {
		printIngest(out)
	}
	if len(out.Refused) > 0 {
		return 1
	}
	return 0
}

// ---------- 提案：读、解析、形状 ----------

// readProposal 读提案。`-` = stdin（harness 直接管道递进来，不落中间文件）。
func readProposal(from string) ([]byte, error) {
	if strings.TrimSpace(from) == "-" {
		b, err := io.ReadAll(io.LimitReader(os.Stdin, maxProposal+1))
		if err != nil {
			return nil, err
		}
		if len(b) > maxProposal {
			return nil, fmt.Errorf("提案超过 %d 字节", maxProposal)
		}
		return b, nil
	}
	b, err := os.ReadFile(from)
	if err != nil {
		return nil, err
	}
	if len(b) > maxProposal {
		return nil, fmt.Errorf("提案超过 %d 字节", maxProposal)
	}
	return b, nil
}

// parseProposal 解析并做**不改树**的那部分校验：形状、白名单、重复。
//
// 认不出的字段直接报错（DisallowUnknownFields）：拼错一个字段名，人以为写了、其实没写，
// 而这条管道是「改写真相源」的 —— 静默少写一条，比拒收坏得多。
func parseProposal(raw []byte) (ingestProposal, []string) {
	var p ingestProposal
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return p, []string{fmt.Sprintf("提案读不懂（%v）—— 形状是 {\"by\":\"…\",\"writes\":[{\"path\":\"…\",\"content\":\"…\"}]}", err)}
	}
	if dec.More() {
		return p, []string{"提案后面还有别的内容 —— 一份提案一个 JSON"}
	}

	var refused []string
	if strings.TrimSpace(p.By) == "" {
		refused = append(refused,
			"提案没写 by：写真相源必须知道是谁提的 —— 身份认错，后面每一条留痕都记在错的人头上，比不记还坏（与信封「缺 who = 拒收」同一条口径）")
	}
	if len(p.Writes) == 0 {
		refused = append(refused, "提案里一条改写都没有（writes 是空的）")
	}
	seen := map[string]bool{}
	for _, w := range p.Writes {
		rel := strings.TrimSpace(w.Path)
		if why := ingestTargetProblem(rel); why != "" {
			refused = append(refused, rel+"："+why)
			continue
		}
		if seen[rel] {
			refused = append(refused, rel+"：同一份提案里出现了两次 —— 哪一次算数说不清")
			continue
		}
		seen[rel] = true
	}
	return p, refused
}

// ingestTargetProblem：这条路能不能由机器写。返回 "" = 能；否则给人话。
//
// 白名单就四处（议题 #47 定的）。**别处没有那个入口，这一条不靠约定，靠的是这张表** ——
// 第三个系统想直接往 vault 里塞东西时，它能撞上的只有这里，而不是「大家说好了不写」。
func ingestTargetProblem(rel string) string {
	if rel == "" {
		return "path 是空的"
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) || strings.Contains(rel, ":") {
		return "path 只能是 vault 里的相对路径（不吃盘符、不吃绝对路径、不吃冒号）"
	}
	if strings.Contains(rel, `\`) {
		return "path 用 / 分隔（vault 的形状与平台无关）"
	}
	segs := strings.Split(rel, "/")
	for _, s := range segs {
		if s == "" || s == "." || s == ".." {
			return "path 里不能有空段、. 或 .."
		}
	}
	if len(segs) == 1 {
		switch rel {
		case org.DomainsFile, org.ProjectsFile, org.AgentsFile:
			return ""
		}
		return "落点不在白名单里 —— 只有 " + ingestWhitelist
	}
	if segs[0] != "members" || len(segs) < 3 {
		return "落点不在白名单里 —— 只有 " + ingestWhitelist
	}
	// members/ 下这一层是**成员名**：它会落成目录名、也会进 project 名，
	// 所以与 org 层用同一把尺子（不另立一套判据 —— 两套迟早漂）。
	if why := org.MemberNameProblem(segs[1]); why != "" {
		return "members/ 下这一层是成员名（会落成目录、也进 project 名）：" + why
	}
	return ""
}

// ---------- 树的操作 ----------

// copyTree 把一棵 vault 复到别处 —— 只为校验：改在副本上，真 vault 一行没动。
//
// .git 不跟着走：版本库不是 vault 的一部分，也不是被校验的对象。
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		// 非普通文件（符号链接、设备…）不复制：跟着走等于把「链到哪」也带进校验树，
		// 而真相源里本来就不该有这种东西。
		if !d.Type().IsRegular() {
			return nil
		}
		fi, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		return writeFileAtomic(filepath.Join(dst, rel), b, fi.Mode().Perm(), 0o755)
	})
}

// ingestChange 比对提案与库里现在那一份：new（本来没有）/ changed / same。
func ingestChange(path, content string) (string, error) {
	old, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		return "new", nil
	case err != nil:
		return "", err
	case string(old) == content:
		return "same", nil
	}
	return "changed", nil
}

func ingestAnyChanged(vs []ingestFileView) bool {
	for _, v := range vs {
		if v.Change != "same" {
			return true
		}
	}
	return false
}

func ingestPaths(ws []ingestWrite) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.Path)
	}
	return out
}

func ingestDetail(vs []ingestFileView) string {
	var nw, ch int
	for _, v := range vs {
		switch v.Change {
		case "new":
			nw++
		case "changed":
			ch++
		}
	}
	return fmt.Sprintf("%d 个文件，新增 %d / 改写 %d", len(vs), nw, ch)
}

// printIngest 出文本。三件事要说清：动了哪几个文件、有没有被红档拦住、机器不写历史。
func printIngest(out ingestOut) {
	mode := "落真"
	if len(out.Refused) > 0 {
		mode = "拒收"
	} else if out.DryRun {
		mode = "空跑（一行没落）"
	}
	fmt.Printf("anc ingest —— %s\n", mode)
	fmt.Printf("  vault    %s\n", out.Vault)
	if out.By != "" {
		fmt.Printf("  谁提的   %s\n", out.By)
	}
	if out.Why != "" {
		fmt.Printf("  附注     %s\n", out.Why)
	}
	for i, f := range out.Files {
		label := "没变"
		switch f.Change {
		case "new":
			label = "新增"
		case "changed":
			label = "改写"
		}
		lead := "  改到     "
		if i > 0 {
			lead = "           "
		}
		fmt.Printf("%s%s %s（%d 字节）\n", lead, padRight(f.Path, 34), label, f.Bytes)
	}
	if len(out.Refused) > 0 {
		fmt.Printf("\n🚫 拒收 %d 条 —— 真 vault 一行没动\n", len(out.Refused))
		for _, s := range out.Refused {
			fmt.Printf("  - %s\n", s)
		}
	}
	if len(out.Warns) > 0 {
		fmt.Printf("\n⚠️  非红档发现 %d 条（不拦，但请过目）\n", len(out.Warns))
		for _, s := range out.Warns {
			fmt.Printf("  - %s\n", s)
		}
	}
	if len(out.Refused) > 0 {
		return // 拒收：这一行没落，没什么 diff 可看
	}
	if out.Audit != "" {
		fmt.Printf("\n  审计     %s\n", out.Audit)
		fmt.Printf("  ⚠️  机器不 commit —— 自己 `git diff` 看一眼再提交（历史归人）\n")
		return
	}
	if !out.DryRun && len(out.Files) > 0 {
		fmt.Printf("\n  一条都没变（库里已与提案逐字相同）—— 没落盘，也没记审计\n")
	}
}
