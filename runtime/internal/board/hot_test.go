package board

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"anc/internal/hot"
)

// fakeHot 是一份假热层：只实现视图要的两件事。
type fakeHot struct {
	list []hot.State
	err  error
	ttl  time.Duration
}

func (f *fakeHot) List() ([]hot.State, error) { return f.list, f.err }
func (f *fakeHot) TTL() time.Duration         { return f.ttl }

func hotOf(t *testing.T, s *Server) (HotView, string) {
	t.Helper()
	rec := get(t, s.Handler(), "/api/hot")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 %d，期望 200（内容有问题也回 200，把问题显示出来）：%s", rec.Code, rec.Body.String())
	}
	var v HotView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("不是合法 JSON：%v", err)
	}
	if v.Schema != HotSchema {
		t.Fatalf("schema=%q，期望 %s", v.Schema, HotSchema)
	}
	return v, rec.Body.String()
}

// 热层没接上：必须如实说「没接上」，**不许**拿一份空列表冒充「热层是空的」——
// 「读不到」和「里面没东西」是两件事，后者会让看板上的人以为真没人有事。
func TestHotUnwiredSaysWhyNot(t *testing.T) {
	v, body := hotOf(t, &Server{})
	if v.Wired {
		t.Fatal("没接热层却报 wired=true")
	}
	if v.Why == "" {
		t.Fatalf("wired=false 必须带上 why（怎么接上），实际为空：%s", body)
	}
	if v.Total != 0 || len(v.Entries) != 0 {
		t.Fatalf("没接上就不该有数：total=%d entries=%d", v.Total, len(v.Entries))
	}
	if v.StaleAfterSecs != int64(hot.DefaultStale/time.Second) {
		t.Fatalf("「多久没动算不新鲜」应当由服务端给（%d 秒），实际 %d",
			int64(hot.DefaultStale/time.Second), v.StaleAfterSecs)
	}
}

// 接上了就得数得出：总数、几条不新鲜、TTL 一并端出来 —— 且新鲜的排前面。
func TestHotCountsAndMarksStale(t *testing.T) {
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	fresh := now.Add(-2 * time.Hour)
	old := now.Add(-30 * time.Hour)
	f := &fakeHot{ttl: 0, list: []hot.State{
		{ID: "t-old", Status: "running", Title: "很久没动的一件", AsOf: old.Format(time.RFC3339)},
		{ID: "t-fresh", Status: "running", Title: "刚发生的一件", AsOf: fresh.Format(time.RFC3339)},
		{ID: "t-nodate", Status: "blocked", Title: "没写「这是什么时候的」"},
	}}
	v, body := hotOf(t, &Server{Hot: f, Now: func() time.Time { return now }})
	if !v.Wired {
		t.Fatalf("接上了却报 wired=false：%s", v.Error)
	}
	if v.Total != 3 {
		t.Fatalf("热层里 3 条，实际 total=%d：%s", v.Total, body)
	}
	// 「没写时间」= 没法说它新鲜，照灰；不是「刚发生」。
	if v.Stale != 2 {
		t.Fatalf("超期 + 没写时间 共 2 条算不新鲜，实际 stale=%d：%s", v.Stale, body)
	}
	if v.TTLSecs != 0 {
		t.Fatalf("TTL=0 的意思是「不失效」，要原样端出来，实际 %d", v.TTLSecs)
	}
	if len(v.Entries) != 3 || v.Entries[0].ID != "t-fresh" {
		t.Fatalf("最新的要排最前，实际 %+v", v.Entries)
	}
	if v.Entries[0].Stale {
		t.Fatalf("2 小时前动过的不该算不新鲜：%+v", v.Entries[0])
	}
	if v.Entries[0].AgeSecs == nil || *v.Entries[0].AgeSecs != int64((2*time.Hour)/time.Second) {
		t.Fatalf("age_secs 该是 7200，实际 %v", v.Entries[0].AgeSecs)
	}
	// 没写时间的条目沉在最后，且**不出 age_secs 这个键**（出个 0 会读成「刚发生」）：
	last := v.Entries[len(v.Entries)-1]
	if last.ID != "t-nodate" {
		t.Fatalf("没写时间的该沉到最后，实际 %+v", v.Entries)
	}
	if last.AgeSecs != nil {
		t.Fatalf("没写时间的条目不该有 age_secs，实际 %d", *last.AgeSecs)
	}
}

// 热层读不动：报出来，但不拦看板 —— 别的页照样该看得到。
func TestHotReadErrorIsShownNotHidden(t *testing.T) {
	f := &fakeHot{err: errors.New("dial tcp 127.0.0.1:6379: connect: connection refused")}
	v, _ := hotOf(t, &Server{Hot: f})
	if !v.Wired {
		t.Fatal("接上了但读失败，不该退成 wired=false")
	}
	if v.Error == "" {
		t.Fatal("读不动必须把原因写出来，不许静默当成空")
	}
	if v.Total != 0 {
		t.Fatalf("读不动就不该有数，实际 total=%d", v.Total)
	}
}

// 契约：字段名（前端认的是这些），锁住 snake_case。
func TestHotFieldNamesAreContract(t *testing.T) {
	v, body := hotOf(t, &Server{Hot: &fakeHot{list: []hot.State{sampleState(t)}}, Now: func() time.Time { return sampleNow }})
	if v.Total != 1 {
		t.Fatalf("夹具应当有 1 条，实际 %s", body)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"schema", "wired", "ttl_secs", "stale_after_secs", "total", "stale", "entries", "note"} {
		if _, ok := raw[k]; !ok {
			t.Fatalf("契约里少了 %q：%s", k, body)
		}
	}
}

var sampleNow = time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)

func sampleState(t *testing.T) hot.State {
	t.Helper()
	return hot.State{ID: "t-1", Status: "running", Title: "一件事", AsOf: sampleNow.Add(-time.Hour).Format(time.RFC3339)}
}