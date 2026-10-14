package main

import (
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"anc/internal/org"
)

// ---------- 外部材料进真相源（议题 #47）的用例 ----------
//
// 盯的三条不变量：
//  1. 落点只有白名单那四处 —— 别处**没有那个入口**（不是靠约定）；
//  2. 红档那一刻**真 vault 一行没动**（校验发生在临时副本上）；
//  3. 机器不写 git 历史。

// capture 抓 stdout 与退出码 —— 拒收的理由必须打在 stdout 上给人看，比的是那段话。
func capture(t *testing.T, f func() int) (string, int) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := f()
	w.Close()
	os.Stdout = old
	b, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(b), code
}

// ingestVault 建一棵**绿的** vault。
//
// 骨架自带一条红档（manager 的 allowed_tools 空着），那是留给角色层的，与本条管道无关：
// 不填掉，下面每条用例都会被它拦，验出来的就不是这条管道自己的事了。
func ingestVault(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "vault")
	if code := quiet(t, func() int {
		return cmdOrgInit([]string{dir, "--client", "某车队", "--id", "demo", "--members", "alice,bob"})
	}); code != 0 {
		t.Fatalf("建 vault 失败，退出码 %d", code)
	}
	role := filepath.Join(dir, "roles", "manager", "persona.md")
	b, err := os.ReadFile(role)
	if err != nil {
		t.Fatal(err)
	}
	fixed := strings.Replace(string(b), "allowed_tools: []", "allowed_tools: [Read, Grep]", 1)
	if fixed == string(b) {
		t.Fatal("模板里没找到 allowed_tools: [] —— 这条断言跟着模板一起漂了")
	}
	if err := os.WriteFile(role, []byte(fixed), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := org.Load(dir); err != nil {
		t.Fatalf("这棵 vault 该是绿的（本用例只验 ingest，不验角色层）：%v", err)
	}
	return dir
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// swapLine 换掉某个键那一行的右值 —— 模板里那几行长什么样由模板定，用例只认键名。
func swapLine(t *testing.T, doc, key, value string) string {
	t.Helper()
	lines := strings.Split(doc, "\n")
	done := false
	for i, ln := range lines {
		if !strings.HasPrefix(strings.TrimSpace(ln), key+":") {
			continue
		}
		indent := ln[:len(ln)-len(strings.TrimLeft(ln, " \t"))]
		lines[i] = indent + key + ": " + value
		done = true
	}
	if !done {
		t.Fatalf("模板里没有 %s: 这一行 —— 这条断言跟着模板一起漂了", key)
	}
	return strings.Join(lines, "\n")
}

// newMemberPersona 拿现成的一份成员 persona 改出另一个成员 ——
// 用例不该背下 persona 的字段清单（那是模板的事）。
func newMemberPersona(t *testing.T, vault, name, appID, openID string) string {
	t.Helper()
	p := readFile(t, filepath.Join(vault, "members", "alice", "persona.md"))
	p = swapLine(t, p, "name", name)
	p = swapLine(t, p, "display_name", name)
	p = swapLine(t, p, "app_id", appID)
	p = swapLine(t, p, "open_id", openID)
	return p
}

// addTableRow 往 markdown 表的表头下面插一行：表周围那些说明与注释不用背进来。
func addTableRow(t *testing.T, doc, row string) string {
	t.Helper()
	lines := strings.Split(doc, "\n")
	for i, ln := range lines {
		if !strings.HasPrefix(strings.TrimSpace(ln), "| slug ") || i+1 >= len(lines) {
			continue
		}
		out := append([]string{}, lines[:i+2]...)
		out = append(out, row)
		return strings.Join(append(out, lines[i+2:]...), "\n")
	}
	t.Fatalf("模板里没找到以「| slug 」开头的表头 —— 这条断言跟着模板一起漂了")
	return ""
}

// proposalFile 落一份提案。用 json.Marshal 而不是手写字符串：
// 手写的话，content 里的换行与引号要自己转义，用例自己先错一遍。
func proposalFile(t *testing.T, name string, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func proposalBody(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// snapshot 把整棵树读成 map —— 「一行没动」这句话得比得出来，不能只看某一个文件。
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		out[filepath.ToSlash(rel)] = readFile(t, p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestIngestLandsFilesAndAudit(t *testing.T) {
	vault := ingestVault(t)
	row := "| q3-eta | Q3 到货跟踪 | | manager | 2026-07-01 → 2026-09-30 | https://example.feishu.cn/docx/x |"
	proj := addTableRow(t, readFile(t, filepath.Join(vault, "projects.md")), row)
	member := newMemberPersona(t, vault, "赵四", "cli_demo_zhaosi", "ou_zhaosi")

	prop := proposalFile(t, "proposal.json", map[string]any{
		"by":  "张三",
		"why": "立项会那份材料",
		"writes": []map[string]string{
			{"path": "projects.md", "content": proj},
			{"path": "members/赵四/persona.md", "content": member},
		},
	})

	out, code := capture(t, func() int { return cmdIngest([]string{vault, "--from", prop}) })
	if code != 0 {
		t.Fatalf("退出码 %d，输出：\n%s", code, out)
	}
	if got := readFile(t, filepath.Join(vault, "members", "赵四", "persona.md")); got != member {
		t.Fatal("成员 persona 没按提案落盘")
	}
	if got := readFile(t, filepath.Join(vault, "projects.md")); got != proj {
		t.Fatal("projects.md 没整份替换")
	}
	// 落完必须是一棵**合法**的树 —— 否则这条管道就把真相源写坏了。
	if _, err := org.Load(vault); err != nil {
		t.Fatalf("落完之后这棵树不合法：%v", err)
	}
	// 审计：谁、什么时候、写了什么。这是「写真相源必须留痕」那一条的落点。
	auditPath := filepath.Join(vault, "audit", "2026-10.张三.jsonl")
	line := readFile(t, auditPath)
	for _, want := range []string{`"actor":"张三"`, `"action":"write"`, `"source":"ingest"`, "members/赵四/persona.md"} {
		if !strings.Contains(line, want) {
			t.Fatalf("审计那一行缺 %s：%s", want, line)
		}
	}
	// 机器不写 git 历史：它连版本库都不建。
	if _, err := os.Stat(filepath.Join(vault, ".git")); !os.IsNotExist(err) {
		t.Fatal("ingest 不该动 git")
	}
	// 输出得说清「机器不 commit」——不然人会以为已经提交了。
	if !strings.Contains(out, "机器不 commit") {
		t.Fatalf("输出没提醒「机器不 commit」：\n%s", out)
	}
}

// 同一份提案再来一次：**一条都没变 → 不落盘、也不记审计**。
// 记了就等于「同一个改动留两条痕」，后面的账会跟着失真。
func TestIngestSameProposalIsNoop(t *testing.T) {
	vault := ingestVault(t)
	row := "| q3-eta | Q3 到货跟踪 | | manager | 2026-07 | |"
	proj := addTableRow(t, readFile(t, filepath.Join(vault, "projects.md")), row)
	prop := proposalFile(t, "proposal.json", map[string]any{
		"by":     "张三",
		"writes": []map[string]string{{"path": "projects.md", "content": proj}},
	})
	if _, code := capture(t, func() int { return cmdIngest([]string{vault, "--from", prop}) }); code != 0 {
		t.Fatalf("第一次退出码 %d", code)
	}
	before := snapshot(t, vault)
	out, code := capture(t, func() int { return cmdIngest([]string{vault, "--from", prop}) })
	if code != 0 {
		t.Fatalf("第二次退出码 %d，输出：\n%s", code, out)
	}
	if !strings.Contains(out, "一条都没变") {
		t.Fatalf("第二遍该报「一条都没变」：\n%s", out)
	}
	if !reflect.DeepEqual(before, snapshot(t, vault)) {
		t.Fatal("没有改动却又动了树（多出来的多半是审计那一行）")
	}
}

func TestIngestRefusesOnRedAndLeavesVaultUntouched(t *testing.T) {
	vault := ingestVault(t)
	bad := swapLine(t, readFile(t, filepath.Join(vault, "members", "alice", "persona.md")), "role", "不存在的岗位")
	prop := proposalFile(t, "proposal.json", map[string]any{
		"by":     "张三",
		"writes": []map[string]string{{"path": "members/alice/persona.md", "content": bad}},
	})
	before := snapshot(t, vault)

	out, code := capture(t, func() int { return cmdIngest([]string{vault, "--from", prop}) })
	if code != 1 {
		t.Fatalf("红档该拒收（退出码 1），实际 %d：\n%s", code, out)
	}
	// 说清**哪一条**不过 —— 只说「校验没过」等于让人自己去猜。
	if !strings.Contains(out, "member.role.missing") {
		t.Fatalf("拒收理由里要有规则 id：\n%s", out)
	}
	if !reflect.DeepEqual(before, snapshot(t, vault)) {
		t.Fatal("红档那一刻真 vault 一行都不该动 —— 校验该发生在临时副本上")
	}
}

func TestIngestDryRunLandsNothing(t *testing.T) {
	vault := ingestVault(t)
	changed := swapLine(t, readFile(t, filepath.Join(vault, "members", "alice", "persona.md")), "display_name", "Alice2")
	prop := proposalFile(t, "proposal.json", map[string]any{
		"by":     "张三",
		"writes": []map[string]string{{"path": "members/alice/persona.md", "content": changed}},
	})
	before := snapshot(t, vault)

	out, code := capture(t, func() int { return cmdIngest([]string{vault, "--from", prop, "--dry-run"}) })
	if code != 0 {
		t.Fatalf("空跑该通过（退出码 0），实际 %d：\n%s", code, out)
	}
	if !strings.Contains(out, "空跑") {
		t.Fatalf("空跑得说明自己没落盘：\n%s", out)
	}
	if !reflect.DeepEqual(before, snapshot(t, vault)) {
		t.Fatal("--dry-run 一行都不该落")
	}
}

func TestIngestRefusesOutsideWhitelist(t *testing.T) {
	vault := ingestVault(t)
	before := snapshot(t, vault)
	cases := []struct{ name, path, want string }{
		{"公司文件", "company/company.md", "落点不在白名单里"},
		{"角色层", "roles/manager/persona.md", "落点不在白名单里"},
		{"members 一层不够", "members/alice", "落点不在白名单里"},
		{"绝对路径", "C:/vault/domains.md", "相对路径"},
		{"反斜杠", `members\alice\persona.md`, "用 / 分隔"},
		{"上跳一层", "../domains.md", "不能有空段"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prop := proposalFile(t, "proposal.json", map[string]any{
				"by":     "张三",
				"writes": []map[string]string{{"path": c.path, "content": "x"}},
			})
			out, code := capture(t, func() int { return cmdIngest([]string{vault, "--from", prop}) })
			if code != 1 {
				t.Fatalf("该拒收（退出码 1），实际 %d：\n%s", code, out)
			}
			if !strings.Contains(out, c.want) {
				t.Fatalf("拒收理由里要有 %q：\n%s", c.want, out)
			}
		})
	}
	if !reflect.DeepEqual(before, snapshot(t, vault)) {
		t.Fatal("落点不过时真 vault 一行都不该动")
	}
}

// members/<名>/ 下面再分层是**允许**的：白名单管的是落点（这一层是不是成员目录），
// 不是深度 —— 成员目录里放几个自己的文件（笔记、附件清单）是成员的正常形态。
func TestIngestAllowsNestedUnderMember(t *testing.T) {
	vault := ingestVault(t)
	prop := proposalFile(t, "proposal.json", map[string]any{
		"by":     "张三",
		"writes": []map[string]string{{"path": "members/alice/notes/2026-10.md", "content": "一"}},
	})
	out, code := capture(t, func() int { return cmdIngest([]string{vault, "--from", prop}) })
	if code != 0 {
		t.Fatalf("成员目录里再分层该放行，实际退出码 %d：\n%s", code, out)
	}
	if got := readFile(t, filepath.Join(vault, "members", "alice", "notes", "2026-10.md")); got != "一" {
		t.Fatalf("文件没落对：%q", got)
	}
}

func TestIngestProposalShapeRefusals(t *testing.T) {
	vault := ingestVault(t)
	cases := []struct{ name, body, want string }{
		{"缺 by", `{"writes":[{"path":"projects.md","content":"x"}]}`, "没写 by"},
		{"writes 是空的", `{"by":"张三","writes":[]}`, "一条改写都没有"},
		{"认不出的字段", `{"by":"张三","writes":[{"path":"projects.md","content":"x"}],"paths":[]}`, "提案读不懂"},
		{"不是 JSON", `写作式的提案`, "提案读不懂"},
		{"同一份里出现两次", `{"by":"张三","writes":[{"path":"projects.md","content":"x"},{"path":"projects.md","content":"y"}]}`, "出现了两次"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prop := proposalBody(t, "proposal.json", c.body)
			out, code := capture(t, func() int { return cmdIngest([]string{vault, "--from", prop}) })
			if code != 1 {
				t.Fatalf("该拒收（退出码 1），实际 %d：\n%s", code, out)
			}
			if !strings.Contains(out, c.want) {
				t.Fatalf("拒收理由里要有 %q：\n%s", c.want, out)
			}
		})
	}
	// 用法错与内容错分开：缺 --from 是**用法**错（退出码 2），不是拒收。
	if out, code := capture(t, func() int { return cmdIngest([]string{vault}) }); code != 2 {
		t.Fatalf("缺 --from 该是用法错（退出码 2），实际 %d：\n%s", code, out)
	}
}
