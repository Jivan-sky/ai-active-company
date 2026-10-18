package outbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"anc/internal/hot"
)

var testNow = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func TestPlanPicksOnlyActionable(t *testing.T) {
	late := testNow.Add(-2 * time.Hour).Format(time.RFC3339)
	tasks := []hot.State{
		{ID: "t-overdue", Stage: hot.StageRunning, Status: "running", Title: "超期了", To: "alice", DueAt: late, Version: 3},
		{ID: "t-blocked", Stage: hot.StageBlocked, Status: "blocked", Title: "卡住", To: "bob", Note: "先等\n卡在客户确认", Version: 2},
		{ID: "t-review", Stage: hot.StageReview, Status: "review", Title: "等验收", To: "carol", Accept: "抽检 200 通", Version: 5,
			Review: hot.Review{By: "carol", Verdict: hot.VerdictPending}},
		{ID: "t-done-late", Stage: hot.StageDone, Status: "done", Title: "晚交但已完成", To: "alice", DueAt: late, Version: 9},
		{ID: "t-future", Stage: hot.StageRunning, Status: "running", Title: "还没到", To: "alice", DueAt: testNow.Add(time.Hour).Format(time.RFC3339), Version: 1},
	}
	recs := Plan(tasks, testNow)
	if len(recs) != 3 {
		t.Fatalf("应只挑 3 条要人动手的，实得 %d：%+v", len(recs), recs)
	}
	kinds := map[string]Record{}
	for _, r := range recs {
		kinds[r.Task+"/"+r.Kind] = r
	}
	if r, ok := kinds["t-overdue/overdue"]; !ok || r.To != "alice" || r.OverdueSecs != 7200 {
		t.Fatalf("超期记录不对：%+v", r)
	}
	if r, ok := kinds["t-blocked/blocked"]; !ok || r.To != "bob" || !strings.Contains(r.Body, "卡在客户确认") {
		t.Fatalf("卡住记录不对：%+v", r)
	}
	if r, ok := kinds["t-review/review_pending"]; !ok || r.To != "carol" || !strings.Contains(r.Body, "抽检 200 通") {
		t.Fatalf("待验收记录不对：%+v", r)
	}
}

func TestPlanIDsAreStablePerVersion(t *testing.T) {
	s := []hot.State{{ID: "t-1", Stage: hot.StageBlocked, Status: "blocked", To: "alice", Version: 4}}
	a := Plan(s, testNow)
	b := Plan(s, testNow.Add(time.Minute))
	if len(a) != 1 || len(b) != 1 || a[0].ID != b[0].ID {
		t.Fatalf("同一版本的事件 id 必须稳定，重跑才能去重：%+v / %+v", a, b)
	}
	s[0].Version = 5
	c := Plan(s, testNow)
	if len(c) != 1 || c[0].ID == a[0].ID {
		t.Fatalf("新版本应产生新事件：%+v / %+v", a, c)
	}
}

func TestAppendDedupesByID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "task-outbox.jsonl")
	recs := []Record{{ID: "e-1", At: testNow.Format(time.RFC3339), Kind: KindBlocked, Task: "t-1", Body: "x"}}
	n, err := Append(path, recs)
	if err != nil || n != 1 {
		t.Fatalf("首次追加：%d %v", n, err)
	}
	n, err = Append(path, recs)
	if err != nil || n != 0 {
		t.Fatalf("重复事件不该再写：%d %v", n, err)
	}
	got, err := Load(path)
	if err != nil || len(got) != 1 || got[0].ID != "e-1" {
		t.Fatalf("读回来不对：%+v %v", got, err)
	}
}

func TestLoadFailsLoudOnBadLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task-outbox.jsonl")
	if err := os.WriteFile(path, []byte("{not json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("坏行必须报错 —— 静默跳过等于丢一条通知")
	}
}

func TestCursorRoundTripAndUnsent(t *testing.T) {
	dir := t.TempDir()
	path := CursorPath(dir)
	cur, err := LoadCursor(path)
	if err != nil || len(cur.Sent) != 0 {
		t.Fatalf("空游标：%+v %v", cur, err)
	}
	recs := []Record{{ID: "e-1"}, {ID: "e-2"}}
	if got := Unsent(recs, cur); len(got) != 2 {
		t.Fatalf("没送过时应两条都待发：%+v", got)
	}
	cur.Mark("e-1", testNow)
	if err := cur.Save(path); err != nil {
		t.Fatal(err)
	}
	back, err := LoadCursor(path)
	if err != nil {
		t.Fatal(err)
	}
	got := Unsent(recs, back)
	if len(got) != 1 || got[0].ID != "e-2" {
		t.Fatalf("游标没生效：%+v", got)
	}
}

func TestPathsLiveUnderState(t *testing.T) {
	dir := t.TempDir()
	if !strings.HasSuffix(Path(dir), filepath.Join("state", "task-outbox.jsonl")) {
		t.Fatalf("发件箱落点不对：%s", Path(dir))
	}
	if !strings.HasSuffix(CursorPath(dir), filepath.Join("state", "task-outbox.cursor.json")) {
		t.Fatalf("游标落点不对：%s", CursorPath(dir))
	}
}
