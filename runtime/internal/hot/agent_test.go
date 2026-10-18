package hot

import (
	"strings"
	"testing"
	"time"
)

func TestAgentHeartbeatKeyIsItsOwnNamespace(t *testing.T) {
	if strings.HasPrefix(AgentKey("anc", "x"), strings.TrimSuffix(scanPattern("anc"), "*")) {
		t.Fatal("心跳 key 会被任务列表的 SCAN 一起捞出来 —— 列举任务时会混进 agent")
	}
	if !strings.HasPrefix(AgentKey("anc", "x"), strings.TrimSuffix(agentScanPattern("anc"), "*")) {
		t.Fatal("心跳 key 不在自己的命名空间里")
	}
}

func TestBeatAgentWritesHashAndNeverExpires(t *testing.T) {
	st, f := fakeStore(t, ":3\r\n", ":1\r\n")
	got, err := st.BeatAgent(AgentHeartbeat{Slug: "receptionbot", Status: "running", TaskID: "t-1"})
	if err != nil {
		t.Fatalf("心跳失败：%v", err)
	}
	if got.AsOf == "" {
		t.Fatal("as_of 没被推到「现在」—— 在线判定就没有依据")
	}
	sent := f.writes.String()
	if !strings.Contains(sent, "anc:agent:receptionbot") {
		t.Fatalf("没写进 agent 命名空间：%q", sent)
	}
	if !strings.Contains(sent, "PERSIST") {
		t.Fatalf("心跳必须显式 PERSIST —— 过期是打标，不是删记录：%q", sent)
	}
	if strings.Contains(sent, "anc:task:") {
		t.Fatalf("心跳写进了任务命名空间：%q", sent)
	}
}

func TestListAgentsScansAgentNamespace(t *testing.T) {
	reply := "*4\r\n$4\r\nslug\r\n$1\r\na\r\n$6\r\nstatus\r\n$7\r\nrunning\r\n"
	st, f := fakeStore(t,
		"*2\r\n$1\r\n0\r\n*1\r\n$11\r\nanc:agent:a\r\n",
		reply,
	)
	list, err := st.ListAgents()
	if err != nil {
		t.Fatalf("列举心跳失败：%v", err)
	}
	if len(list) != 1 || list[0].Slug != "a" || list[0].Status != "running" {
		t.Fatalf("心跳列举不对：%#v", list)
	}
	if !strings.Contains(f.writes.String(), "MATCH") || !strings.Contains(f.writes.String(), "anc:agent:*") {
		t.Fatalf("SCAN 没用 agent 模式：%q", f.writes.String())
	}
}

func TestAgentStaleUsesConfiguredThreshold(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	h := AgentHeartbeat{Slug: "a", AsOf: now.Add(-3 * time.Minute).Format(time.RFC3339)}
	if !h.Stale(now, 2*time.Minute) {
		t.Fatal("3 分钟没心跳、阈值 2 分钟，应算不新鲜")
	}
	if h.Stale(now, 5*time.Minute) {
		t.Fatal("阈值 5 分钟时不该判不新鲜")
	}
	broken := AgentHeartbeat{Slug: "b", AsOf: "不是时间"}
	if !broken.Stale(now, 2*time.Minute) {
		t.Fatal("时间读不懂时不能假装新鲜")
	}
}
