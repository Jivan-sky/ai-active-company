package probe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeSessions 写一个会话文件。history 是 (role, 相对 now 的偏移) 序列；
// agentID 空 = agent 从没起来过（「engine started 是绿字、却回不了话」的签名）。
func writeSessions(t *testing.T, dir, name, agentID string, now time.Time, history ...string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	type msg struct {
		Role      string `json:"role"`
		Content   string `json:"content"`
		Timestamp string `json:"timestamp"`
	}
	msgs := make([]msg, 0, len(history)/2)
	for i := 0; i+1 < len(history); i += 2 {
		d, err := time.ParseDuration(history[i+1])
		if err != nil {
			t.Fatal(err)
		}
		msgs = append(msgs, msg{history[i], "…", now.Add(d).Format(time.RFC3339Nano)})
	}
	one := map[string]any{
		"sessions": map[string]any{
			"s1": map[string]any{
				"id": "s1", "name": "default",
				"agent_session_id": agentID,
				"history":          msgs,
			},
		},
	}
	b, err := json.Marshal(one)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// Judge 的三档判据：纯函数、定时定刻，不依赖机器时钟也不起进程。
func TestJudgeLevels(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	files := func(agentID string, hist ...string) []string {
		return []string{writeSessions(t, t.TempDir(), "p_x.json", agentID, now, hist...)}
	}
	cases := []struct {
		name  string
		files func() []string
		want  State
	}{
		{"刚回过话", func() []string { return files("s1", "user", "-2m", "assistant", "-1m") }, StateOK},
		{"最后一轮刚发出：在处理中，不是红", func() []string { return files("s1", "user", "-1m") }, StateWarn},
		{"最后一轮超过 stall 没回：卡住", func() []string { return files("s1", "user", "-40m") }, StateFail},
		{"久无成功交互：不报绿", func() []string { return files("s1", "user", "-50h", "assistant", "-49h") }, StateWarn},
		{"agent 从没起来过", func() []string { return files("", "user", "-5m") }, StateFail},
		{"没有会话文件", func() []string { return nil }, StateWarn},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Judge("p", c.files(), now, DefaultStale, DefaultStall)
			if got.State != c.want {
				t.Fatalf("档位 %s，期望 %s（%s）", got.State, c.want, got.Why)
			}
		})
	}
}

// writeSessionsRaw 与 writeSessions 的差别只有一处：内容由用例给 ——
// 「空回复」这一档必须能造出来，不能靠一个写着「…」的占位符假装。
func writeSessionsRaw(t *testing.T, dir, name, agentID string, now time.Time, msgs ...struct{ Role, Content, Offset string }) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	type msg struct {
		Role      string `json:"role"`
		Content   string `json:"content"`
		Timestamp string `json:"timestamp"`
	}
	out := make([]msg, 0, len(msgs))
	for _, m := range msgs {
		d, err := time.ParseDuration(m.Offset)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, msg{m.Role, m.Content, now.Add(d).Format(time.RFC3339Nano)})
	}
	one := map[string]any{"sessions": map[string]any{
		"s1": map[string]any{"id": "s1", "name": "default", "agent_session_id": agentID, "history": out},
	}}
	b, err := json.Marshal(one)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// 议题 #51：能机械判定的那部分不许漏 —— 空回复不算「回过话」。
// 语义对不对（回了错误话）仍留给人，这里只钉「有没有内容」。
func TestJudgeEmptyReplyIsNotGreen(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	j := func(msgs ...struct{ Role, Content, Offset string }) Finding {
		p := writeSessionsRaw(t, t.TempDir(), "p_x.json", "s1", now, msgs...)
		return Judge("p", []string{p}, now, DefaultStale, DefaultStall)
	}
	m := func(role, content, offset string) struct{ Role, Content, Offset string } {
		return struct{ Role, Content, Offset string }{role, content, offset}
	}

	if got := j(m("user", "在吗", "-2m"), m("assistant", "", "-1m")); got.State == StateOK {
		t.Fatalf("空回复被当成回过话（假绿）：%+v", got)
	} else if !strings.Contains(got.Why, "空") {
		t.Fatalf("原因里要说清是空回复：%s", got.Why)
	}
	if got := j(m("user", "在吗", "-2m"), m("assistant", "   \n", "-1m")); got.State == StateOK {
		t.Fatalf("纯空白回复被当成回过话（假绿）：%+v", got)
	}
	if got := j(
		m("user", "在吗", "-5m"), m("assistant", "", "-4m"),
		m("user", "再问一次", "-3m"), m("assistant", "在。", "-2m"),
	); got.State != StateOK {
		t.Fatalf("空回复之后又有真回复，应当绿：%+v", got)
	}
}

// 「没消息 = 没事」是反模式：一个从没人跟它说过话的 bot 也不许报绿。
// 这一条是新档位，容易被后来的「优化」顺手改回绿，所以单独钉住。
func TestJudgeNeverClaimsGreenWithoutEvidence(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	for _, f := range []Finding{
		Judge("p", nil, now, DefaultStale, DefaultStall),
		Judge("p", []string{writeSessions(t, t.TempDir(), "p_x.json", "s1", now)}, now, DefaultStale, DefaultStall),
		Judge("p", []string{writeSessions(t, t.TempDir(), "p_x.json", "s1", now, "user", "-1m")}, now, DefaultStale, DefaultStall),
	} {
		if f.State == StateOK {
			t.Fatalf("没有任何真实回复证据时报了绿：%+v", f)
		}
	}
}

func TestProjectOfSessionFile(t *testing.T) {
	cases := map[string]string{
		"demo-alice_00a71faa.json": "demo-alice",
		"bare.json":                "bare",
		"_leading.json":            "_leading",
	}
	for in, want := range cases {
		if got := ProjectOfSessionFile(in); got != want {
			t.Fatalf("%s → %q，期望 %q", in, got, want)
		}
	}
}

// AllGreen 只有「网关在跑 + 每个 bot 要么绿要么是声明未接 + 至少一个绿 + 没有残留」才为真。
func TestAllGreen(t *testing.T) {
	ok := Report{Gateway: "up", Bots: []Finding{{"a", StateOK, ""}}}
	if !ok.AllGreen() {
		t.Fatal("全绿却被判非绿")
	}
	for _, bad := range []Report{
		{Gateway: "down", Bots: []Finding{{"a", StateOK, ""}}},
		{Gateway: "up", Bots: []Finding{{"a", StateWarn, ""}}},
		{Gateway: "up", Bots: []Finding{{"a", StateOK, ""}}, Extras: []string{"gone"}},
		// 空集不判绿：什么都没验过，不是「没事」（全被声明未接 / config 里没有 project）。
		{Gateway: "up", Bots: []Finding{}},
		{Gateway: "up", Bots: []Finding{{"a", StateUnwired, ""}}},
	} {
		if bad.AllGreen() {
			t.Fatalf("不该判绿：%+v", bad)
		}
	}
}
