package board

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"anc/internal/hot"
	"anc/internal/timeline"
)

func newSettleServer(t *testing.T, vault string) (http.Handler, *boardFakeQueue) {
	t.Helper()
	q := newBoardFakeQueue()
	q.items["t-1"] = hot.State{
		ID: "t-1", Status: "done", By: "tradebot", From: "anc", To: "tradebot",
		Title: "处理单据", Note: "已答复客户", Session: "s-1", Intent: "queue", Version: 3,
		AsOf: "2026-10-18T11:00:00Z",
	}
	s := &Server{Vault: vault, Now: func() time.Time { return fixedNow }, Tasks: q}
	return s.Handler(), q
}

func TestSettleEndpointWritesTimeline(t *testing.T) {
	vault := t.TempDir()
	h, q := newSettleServer(t, vault)

	rec := post(t, h, SettlePath, "application/json", `{"id":"t-1","by":"alice","note":"看板归档"}`)
	if rec.Code != 200 {
		t.Fatalf("归档失败：%d %s", rec.Code, rec.Body.String())
	}
	var out SettleReply
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK || out.Already || out.Timeline == "" || out.ID != "t-1" || out.By != "alice" {
		t.Fatalf("回执不对：%+v", out)
	}
	if _, ok := q.items["t-1"]; ok {
		t.Fatal("归档后热层应清走")
	}
	doc, err := timeline.Load(vault)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Entries) != 1 {
		t.Fatalf("timeline 应有一条：%d", len(doc.Entries))
	}
	e := doc.Entries[0]
	if e.Case != "t-1" || e.By != "alice" || e.Status != "done" || !strings.Contains(e.Detail, "看板归档") {
		t.Fatalf("落库字段不对：%+v", e)
	}
}

func TestSettleEndpointIsIdempotent(t *testing.T) {
	vault := t.TempDir()
	h, _ := newSettleServer(t, vault)
	post(t, h, SettlePath, "application/json", `{"id":"t-1","by":"alice"}`)
	rec := post(t, h, SettlePath, "application/json", `{"id":"t-1","by":"alice"}`)
	if rec.Code != 200 {
		t.Fatalf("重复归档应幂等：%d %s", rec.Code, rec.Body.String())
	}
	var out SettleReply
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if !out.Already {
		t.Fatalf("应报 already：%+v", out)
	}
	doc, _ := timeline.Load(vault)
	if len(doc.Entries) != 1 {
		t.Fatalf("不该追加第二条：%d", len(doc.Entries))
	}
}

func TestSettleEndpointRefusesOpenTask(t *testing.T) {
	vault := t.TempDir()
	h, q := newSettleServer(t, vault)
	q.items["t-1"] = hot.State{ID: "t-1", Status: "running", To: "tradebot", Title: "处理中", Version: 2}
	rec := post(t, h, SettlePath, "application/json", `{"id":"t-1","by":"alice"}`)
	if rec.Code != 409 {
		t.Fatalf("running 应 409：%d %s", rec.Code, rec.Body.String())
	}
	if _, ok := q.items["t-1"]; !ok {
		t.Fatal("被拒时热层不许动")
	}
	if doc, _ := timeline.Load(vault); len(doc.Entries) != 0 {
		t.Fatalf("被拒时不该落库：%d", len(doc.Entries))
	}
}

func TestSettleEndpointRejectsAnonymous(t *testing.T) {
	h, _ := newSettleServer(t, t.TempDir())
	rec := post(t, h, SettlePath, "application/json", `{"id":"t-1"}`)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "谁在归档") {
		t.Fatalf("匿名归档应被拒：%d %s", rec.Code, rec.Body.String())
	}
}

func TestSettleEndpointOnlyPost(t *testing.T) {
	h, _ := newSettleServer(t, t.TempDir())
	rec := get(t, h, SettlePath)
	if rec.Code != 405 {
		t.Fatalf("GET 写口应 405：%d", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != "POST" {
		t.Fatalf("Allow 应是 POST：%q", allow)
	}
}

func TestSettleMapsDisplayNameToSlug(t *testing.T) {
	src := fixturePath(t, "domains")
	vault := t.TempDir()
	if err := os.CopyFS(vault, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	h, _ := newSettleServer(t, vault)
	rec := post(t, h, SettlePath, "application/json", `{"id":"t-1","by":"Alice Wang"}`)
	if rec.Code != 200 {
		t.Fatalf("归档失败：%d %s", rec.Code, rec.Body.String())
	}
	doc, err := timeline.Load(vault)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Entries) != 1 || doc.Entries[0].By != "alice" {
		t.Fatalf("中文/显示名应对到 slug：%+v", doc.Entries)
	}
	if _, err := os.Stat(filepath.Join(vault, timeline.DirName)); err != nil {
		t.Fatalf("timeline 目录应已创建：%v", err)
	}
}

func TestHotViewCarriesSettlePath(t *testing.T) {
	v, _ := hotOf(t, &Server{Hot: &fakeHot{}})
	if v.SettlePath != SettlePath {
		t.Fatalf("热层视图应带上归档写口：%q", v.SettlePath)
	}
}
