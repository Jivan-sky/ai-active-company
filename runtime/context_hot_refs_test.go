package main

import (
	"encoding/json"
	"strings"
	"testing"

	"anc/internal/ctxpack"
	"anc/internal/hot"
)

// ---------- 议题 #60：热层里解不出的 to / by 不许静默 ----------
//
// 这一组盯三件事：
//  1. 两边走**同一把尺子**：热层里写 `member:alice` 与写 `alice` 是同一件事（都该落到 alice 手里），
//     写 `agent:order-bot` 也一样 —— 判据是 org.IdentityForRef + org.SameSubject，与授权那块同一支；
//  2. 认不出的写法**报缺**：写显示名（`Alice Wang`）或岗位（`role:manager`）的那几条谁都不认领，
//     但绝不许一声不吭 —— 那正是「看着绿、其实丢了」（写错一次，这条待办永远落不到任何人手里）；
//  3. 没写这一格（空）**不算写错**：不是每一手都有人接，也不是每一手都派出去过。

// hotFixture：domains fixture 再补一张 agents.md —— 待办要能派给**两类主体**（成员 + 业务 agent）。
func hotFixture(t *testing.T) string {
	t.Helper()
	v := copyFixture(t, "domains")
	writeIntoFixture(t, v, "agents.md",
		"| slug | name | domain | harness | tools | model | role | app_id |\n"+
			"|---|---|---|---|---|---|---|---|\n"+
			"| order-bot | 订单处理 | trade | codex | read,bash | claude-sonnet-5 | manager | cli_demo_order |\n")
	return v
}

// packTasks 跑一次 `anc context pack`，热层喂的是调用方给的那几条。
func packTasks(t *testing.T, vault string, states []hot.State, extra ...string) ctxpack.Out {
	t.Helper()
	g := &envelopeIngress{}
	withHot(t, g, states)
	out, code := ctxPackOnWire(t, vault, extra...)
	if code != 0 {
		t.Fatalf("退出码 %d：%s", code, out)
	}
	var got ctxpack.Out
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("不是合法 JSON：%v\n%s", err, out)
	}
	return got
}

// gapLine 在 gaps 里找**含某句话**的那一条（找不到回空串）。报缺的文案即契约，所以按原话找。
func gapLine(got ctxpack.Out, contains string) string {
	for _, g := range got.Gaps {
		if strings.Contains(g, contains) {
			return g
		}
	}
	return ""
}

func ids(rows []ctxpack.TaskOut) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

// 一把尺子：同一个人两种写法（`alice` / `member:alice`）都落在他手里；
// 派给业务 agent 的（`agent:order-bot`）**不是**他的、也**不算写错**（解得出，只是不是我）。
func TestContextPackTaskUsesSameRulerAsIdentity(t *testing.T) {
	states := []hot.State{
		{ID: "case-bare", Title: "裸名字", To: "alice", By: "alice"},
		{ID: "case-prefix", Title: "带前缀", To: "member:alice", By: "member:alice"},
		{ID: "case-agent", Title: "派给业务 agent", To: "agent:order-bot", By: "order-bot"},
	}

	got := packTasks(t, hotFixture(t), states, "--who", "alice")
	if want := "case-bare,case-prefix"; strings.Join(ids(got.Todo), ",") != want {
		t.Fatalf("派给 alice 的待办不对（两种写法都该认）：%v", ids(got.Todo))
	}
	if want := "case-bare,case-prefix"; strings.Join(ids(got.Doing), ",") != want {
		t.Fatalf("alice 在做的那些不对：%v", ids(got.Doing))
	}
	if line := gapLine(got, "解不出主体"); line != "" {
		t.Fatalf("这几条都解得出人，不该报缺：%s", line)
	}

	// 换个人问：业务 agent 那一份拿到的是派给它自己的那一条（两条腿都走同一把尺子）。
	bot := packTasks(t, hotFixture(t), states, "--who", "order-bot")
	if want := "case-agent"; strings.Join(ids(bot.Todo), ",") != want {
		t.Fatalf("业务 agent 的待办不对：%v", ids(bot.Todo))
	}
	if want := "case-agent"; strings.Join(ids(bot.Doing), ",") != want {
		t.Fatalf("业务 agent 在做的那些不对：%v", ids(bot.Doing))
	}
}

// 认不出的写法**报缺不静默**：显示名与岗位都不是编号身份 —— 那几条谁都不认领，但必须说出来。
// 顺带钉两条：同一种写法写错三条只说一次（去重）、报缺里不回显整条记录（只列写法）。
func TestContextPackTaskSaysWhenToCannotBeResolved(t *testing.T) {
	got := packTasks(t, hotFixture(t), []hot.State{
		{ID: "case-1", To: "Alice Wang"},
		{ID: "case-2", To: "role:manager"},
		{ID: "case-3", To: "Alice Wang"},
		{ID: "case-4", By: "nonexistent"},
	}, "--who", "alice")

	if len(got.Todo) != 0 || len(got.Doing) != 0 {
		t.Fatalf("解不出人的那些条一个都不该落进来：%+v", got)
	}
	to := gapLine(got, "`to` 写法解不出主体")
	if to == "" {
		t.Fatalf("`to` 解不出人却没说：%v", got.Gaps)
	}
	for _, want := range []string{"2 种", "`Alice Wang`", "`role:manager`"} {
		if !strings.Contains(to, want) {
			t.Fatalf("`to` 那句报缺少了 %q：%s", want, to)
		}
	}
	// 去重：`Alice Wang` 写错三条，只算一种；条数不是「3 种」。
	if strings.Contains(to, "3 种") {
		t.Fatalf("同一种写法写错三条该只说一次：%s", to)
	}
	if strings.Contains(to, "case-") {
		t.Fatalf("报缺列的是写法，不该把整条记录倒出来：%s", to)
	}

	by := gapLine(got, "`by` 写法解不出主体")
	if by == "" || !strings.Contains(by, "1 种") || !strings.Contains(by, "`nonexistent`") {
		t.Fatalf("`by` 那句报缺不对：%q\n%v", by, got.Gaps)
	}
}

// 空 != 写错：没写这一格不算缺陷（不是每一手都有人接），写的是别人也不算（解得出人）。
func TestContextPackTaskBlanksAreNotDefects(t *testing.T) {
	got := packTasks(t, hotFixture(t), []hot.State{
		{ID: "case-1", To: "alice"},
		{ID: "case-2", Title: "只是记一笔"},
		{ID: "case-3", By: "bob"},
	}, "--who", "alice")
	if line := gapLine(got, "解不出主体"); line != "" {
		t.Fatalf("这三种都不算写错，不该报缺：%s", line)
	}
	if want := "case-1"; strings.Join(ids(got.Todo), ",") != want {
		t.Fatalf("待办不对：%v", ids(got.Todo))
	}
}

// 单域问：这一整段（两摞 + 报缺）都不跟着进来 —— 同「找谁」目录那条纪律。
func TestContextPackScopedAskCarriesNoTaskGaps(t *testing.T) {
	got := packTasks(t, hotFixture(t), []hot.State{{ID: "case-1", To: "Alice Wang"}},
		"--who", "alice", "--domain", "trade")
	if len(got.Gaps) != 0 {
		t.Fatalf("单域问不该捎报缺那一整段：%v", got.Gaps)
	}
	if len(got.Todo) != 0 || len(got.Doing) != 0 {
		t.Fatalf("单域问不该捎待办：%+v", got)
	}
}
