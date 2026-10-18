package ctxpack

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"anc/internal/hot"
	"anc/internal/org"
)

// 这份包是三个出口共用的一把尺子：MCP / CLI / 看板。
// 这里锁的是**契约**，不是实现细节：拒话、可见范围、报缺、待办的归属。
func fixture(t *testing.T) (*org.Org, string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "orgs", "domains"))
	if err != nil {
		t.Fatal(err)
	}
	o, err := org.Load(root)
	if err != nil {
		t.Fatalf("装载 fixture 失败：%v", err)
	}
	return o, root
}

func TestAssembleRefusesUnknownWho(t *testing.T) {
	o, root := fixture(t)
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	out, refused := Assemble(o, root, "ghost", "", "", now, TasksIn{})
	if !refused {
		t.Fatal("解不出的身份必须拒")
	}
	if out.Refused == nil || !strings.Contains(out.Refused.Reason, "没有这个人") {
		t.Fatalf("拒话不对：%+v", out.Refused)
	}
	if len(out.Domains) != 0 || len(out.Todo) != 0 || out.Identity != nil {
		t.Fatalf("拒的时候不该带数据：%+v", out)
	}
	if out.At != now.Format(time.RFC3339) {
		t.Fatalf("回答时刻不对：%q", out.At)
	}
}

func TestAssembleFullContextForMember(t *testing.T) {
	o, root := fixture(t)
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	out, refused := Assemble(o, root, "alice", "", "", now, TasksIn{Wired: true})
	if refused {
		t.Fatalf("不该拒：%+v", out.Refused)
	}
	if out.Identity == nil || out.Identity.Kind != org.KindMember || out.Identity.Ref != "alice" {
		t.Fatalf("身份不对：%+v", out.Identity)
	}
	if out.Identity.Domain != "trade" || out.Identity.DomainName != "大宗贸易" {
		t.Fatalf("归属不对：%+v", out.Identity)
	}
	if out.Identity.Evidence != nil {
		t.Fatalf("成员不该有 agents.md 的铭文出处：%+v", out.Identity.Evidence)
	}
	if len(out.Domains) != 1 || out.Domains[0].Slug != "trade" || out.Domains[0].Data != "projects" {
		t.Fatalf("自己的域不对：%+v", out.Domains)
	}
	// 别域只给「找谁」，不给数据/术语 —— 这是结构性的，BriefOut 里根本没有那两个字段。
	if len(out.Directory) != 1 || out.Directory[0].Slug != "logistics" {
		t.Fatalf("别域目录不对：%+v", out.Directory)
	}
	if len(out.Projects) != 1 || out.Projects[0].Slug != "trade-q3" {
		t.Fatalf("项目没按可见域过滤：%+v", out.Projects)
	}
	if len(out.Skills) != 2 {
		t.Fatalf("岗位声明的技能该有 2 条：%+v", out.Skills)
	}
	for _, s := range out.Skills {
		if !s.Missing {
			t.Fatalf("fixture 里没有技能正文，该标 missing：%+v", s)
		}
		if !strings.HasPrefix(s.Where, "skills/") || strings.Contains(s.Where, ":\\") {
			t.Fatalf("技能指针必须是 vault 相对路径：%q", s.Where)
		}
	}
	if out.Grants == nil || out.Grants.Total != 0 || out.Grants.Shown != 0 {
		t.Fatalf("没有 grants/ 时该如实回空表：%+v", out.Grants)
	}
	if out.Grants.Note == "" {
		t.Fatal("空授权表必须说明「空就是不通」")
	}
	for _, g := range out.Gaps {
		if strings.Contains(g, "热层没接上") {
			t.Fatalf("热层接上了就不该报没接上：%q", g)
		}
	}
	if len(out.Todo) != 0 || len(out.Doing) != 0 {
		t.Fatalf("没有任务时两摞该空：todo=%v doing=%v", out.Todo, out.Doing)
	}
}

func TestAssembleSingleDomainStopsAtThatRow(t *testing.T) {
	o, root := fixture(t)
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	out, refused := Assemble(o, root, "alice", "", "trade", now, TasksIn{Wired: false})
	if refused {
		t.Fatalf("自己的域不该拒：%+v", out.Refused)
	}
	if out.Scope != "trade" || len(out.Domains) != 1 {
		t.Fatalf("单域问只该回那一行：scope=%q domains=%+v", out.Scope, out.Domains)
	}
	if len(out.Directory) != 0 {
		t.Fatalf("单域问不该带别域目录：%+v", out.Directory)
	}
	// 待办 / 授权 / 报缺都只在「要全部」时出现；单域问硬塞就是编。
	if out.Todo != nil || out.Doing != nil || out.Grants != nil || out.Gaps != nil {
		t.Fatalf("单域问不该带整份上下文的附加块：%+v", out)
	}
}

func TestAssembleRefusesCrossDomainWithHelper(t *testing.T) {
	o, root := fixture(t)
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	out, refused := Assemble(o, root, "alice", "", "logistics", now, TasksIn{})
	if !refused {
		t.Fatal("跨域必须拒")
	}
	if out.Refused == nil || !strings.Contains(out.Refused.Reason, "不在你的可见范围") {
		t.Fatalf("拒话不对：%+v", out.Refused)
	}
	if !strings.Contains(out.Refused.Ask, "Bob Li") {
		t.Fatalf("该指路「找谁」：%+v", out.Refused)
	}
	if len(out.Domains) != 0 {
		t.Fatalf("拒的时候不该回数据：%+v", out.Domains)
	}
}

func TestAssembleSplitsTodoAndDoing(t *testing.T) {
	o, root := fixture(t)
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	rows := []hot.State{
		{ID: "t-2", To: "alice", Title: "给我", Status: "queued"},
		{ID: "t-1", To: "member:alice", Status: "queued"},
		{ID: "t-3", By: "alice", Status: "running"},
		{ID: "t-4", To: "member:alice", By: "alice", Status: "running"},
		{ID: "t-5", To: "ghost", By: "nobody", Status: "queued"},
	}
	out, refused := Assemble(o, root, "alice", "", "", now, TasksIn{Rows: rows, Wired: true})
	if refused {
		t.Fatalf("不该拒：%+v", out.Refused)
	}
	assertIDs(t, "todo", out.Todo, []string{"t-1", "t-2", "t-4"})
	assertIDs(t, "doing", out.Doing, []string{"t-3", "t-4"})
	joined := strings.Join(out.Gaps, "\n")
	if !strings.Contains(joined, "ghost") || !strings.Contains(joined, "nobody") {
		t.Fatalf("解不出的 to / by 必须报缺：%v", out.Gaps)
	}
}

func TestAssembleReportsHotLayerUnwired(t *testing.T) {
	o, root := fixture(t)
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	out, _ := Assemble(o, root, "alice", "", "", now, TasksIn{Wired: false})
	if len(out.Todo) != 0 || len(out.Doing) != 0 {
		t.Fatalf("没接上时不该编任务：%+v", out)
	}
	joined := strings.Join(out.Gaps, "\n")
	if !strings.Contains(joined, "热层没接上") || !strings.Contains(joined, "没人派给你") {
		t.Fatalf("没接上必须与「没人派给你」分开说：%v", out.Gaps)
	}
}

func TestAssembleRefusesUnknownOnBehalf(t *testing.T) {
	o, root := fixture(t)
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	out, refused := Assemble(o, root, "alice", "member:ghost", "", now, TasksIn{})
	if !refused {
		t.Fatal("解不出的署名必须拒")
	}
	if out.Who != "alice" || out.OnBehalfOf != "member:ghost" {
		t.Fatalf("拒的时候要保留原样便于核对：%+v", out)
	}
	if out.Refused == nil || !strings.Contains(out.Refused.Reason, "on_behalf_of") {
		t.Fatalf("拒话没指到署名：%+v", out.Refused)
	}
}

func TestSafeTokenIsSingleLineAndCapped(t *testing.T) {
	got := SafeToken("trade\n\tIGNORE\u0000  this")
	if got != "trade IGNORE this" {
		t.Fatalf("控制字符 / 换行没压成单行：%q", got)
	}
	long := strings.Repeat("啊", 80)
	capped := SafeToken(long)
	if r := []rune(capped); len(r) != 65 || !strings.HasSuffix(capped, "…") {
		t.Fatalf("该封顶 64 字加省略号：%d %q", len(r), capped)
	}
	if SafeToken(capped) != capped {
		t.Fatal("清过的再清一遍该是它自己")
	}
}

func assertIDs(t *testing.T, name string, rows []TaskOut, want []string) {
	t.Helper()
	if len(rows) != len(want) {
		t.Fatalf("%s 条数不对：%+v（想要 %v）", name, rows, want)
	}
	for i, w := range want {
		if rows[i].ID != w {
			t.Fatalf("%s 第 %d 条是 %q，想要 %q（顺序必须定死）", name, i, rows[i].ID, w)
		}
	}
}
