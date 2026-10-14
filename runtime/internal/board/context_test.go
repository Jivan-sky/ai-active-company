package board

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"anc/internal/hot"
	"anc/internal/timeline"
)

// ctxOf 打一次 /api/context，解出视图。状态码必须 200 —— 内容有问题**也回 200**：
// 这一页要把「拒」和「读不到」都显示出来，不是当故障处理（同 /api/issues 的口径）。
func ctxOf(t *testing.T, h http.Handler, path string) (ContextView, string) {
	t.Helper()
	rec := get(t, h, path)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 %d，期望 200：%s", rec.Code, rec.Body.String())
	}
	var v ContextView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("不是合法 JSON：%v\n%s", err, rec.Body.String())
	}
	if v.Schema != ContextSchema {
		t.Fatalf("schema=%q，期望 %s", v.Schema, ContextSchema)
	}
	return v, rec.Body.String()
}

// 不给 who：回名单 —— 一页要能自己答完「可以看谁」，不让前端去别的端点拼。
// 名单来自真相源（成员 + 业务 agent），编号是它们的，不许自报。
func TestContextRosterWithoutWho(t *testing.T) {
	v, body := ctxOf(t, newHandler(t, "domains"), "/api/context")
	if v.Error != "" {
		t.Fatalf("干净 fixture 不该读不动：%s", v.Error)
	}
	if v.Pack != nil {
		t.Fatalf("没指定 who 就不该有那一份上下文：%s", body)
	}
	if len(v.Roster) != 3 {
		t.Fatalf("名单应当是 fixture 里那三个成员，实际 %+v", v.Roster)
	}
	refs := map[string]string{}
	for _, e := range v.Roster {
		refs[e.Ref] = e.Kind
	}
	for _, want := range []string{"alice", "bob", "devbot"} {
		if refs[want] != "member" {
			t.Errorf("名单里少了 %q（或 kind 不是 member）：%+v", want, v.Roster)
		}
	}
}

// 给了 who：五问 + 项目都要在 —— 与 MCP / CLI 走的是**同一个装配点**，看板不另立一份。
func TestContextPackForWho(t *testing.T) {
	v, body := ctxOf(t, newHandler(t, "domains"), "/api/context?who=alice")
	if v.Pack == nil {
		t.Fatalf("指定了 who 却没回上下文：%s", body)
	}
	p := v.Pack
	if p.Who != "alice" {
		t.Errorf("who = %q，期望 alice", p.Who)
	}
	if p.Identity == nil || p.Identity.Ref != "alice" || p.Identity.Role != "manager" {
		t.Errorf("身份那格不对：%+v", p.Identity)
	}
	if len(p.Domains) != 1 || p.Domains[0].Slug != "trade" || p.Domains[0].Data == "" {
		t.Errorf("业务域 / 数据源没答出来：%+v", p.Domains)
	}
	if len(p.Directory) != 1 || p.Directory[0].Slug != "logistics" {
		t.Errorf("别域只该给「找谁」目录：%+v", p.Directory)
	}
	if len(p.Skills) == 0 || len(p.Projects) == 0 || len(p.Gaps) == 0 {
		t.Errorf("技能 / 项目 / 报缺三样都要在：skills=%+v projects=%+v gaps=%v", p.Skills, p.Projects, p.Gaps)
	}
}

// 拒**不是** HTTP 错误：这个人不存在时照样 200，拒话在 pack.refused 里给前端显示。
func TestContextRefusalIsNotAnHTTPError(t *testing.T) {
	v, body := ctxOf(t, newHandler(t, "domains"), "/api/context?who=查无此人")
	if v.Pack == nil || v.Pack.Refused == nil {
		t.Fatalf("该把拒话放在 pack.refused 里：%s", body)
	}
	if v.Pack.Who != "" {
		t.Errorf("解不出的人不该有编号：%q", v.Pack.Who)
	}
	// 越域也一样：拒话要指路「找谁」，且不夹带别人的数据。
	v2, body2 := ctxOf(t, newHandler(t, "domains"), "/api/context?who=alice&domain=logistics")
	if v2.Pack == nil || v2.Pack.Refused == nil || v2.Pack.Refused.Ask == "" {
		t.Fatalf("越域该拒且要指路：%s", body2)
	}
	if len(v2.Pack.Domains) != 0 || len(v2.Pack.Projects) != 0 {
		t.Errorf("被拒的那一份里不该带数据：%+v", v2.Pack)
	}
}

// 只给那一域：scope 落上、目录不出现（同 CLI 的 --domain 口径）。
func TestContextScopesToOneDomain(t *testing.T) {
	v, body := ctxOf(t, newHandler(t, "domains"), "/api/context?who=alice&domain=trade")
	if v.Pack == nil || v.Pack.Scope != "trade" {
		t.Fatalf("scope 应当是 trade：%s", body)
	}
	if len(v.Pack.Domains) != 1 || len(v.Pack.Directory) != 0 || len(v.Pack.Gaps) != 0 {
		t.Errorf("要单独一域时不该给目录与通用报缺：%+v", v.Pack)
	}
}

// 看板是观测面：回话里不许出现本机绝对路径（同 /api/board 的口径）。
func TestContextLeaksNoLocalPaths(t *testing.T) {
	s := &Server{Vault: fixturePath(t, "domains"), Now: func() time.Time { return fixedNow }}
	_, body := ctxOf(t, s.Handler(), "/api/context?who=alice")
	if strings.Contains(body, s.Vault) {
		t.Fatalf("回话里出现了本机路径：%s", body)
	}
}

// 热层没接上：这一页必须说「这一段答不了」，**不许**摆一张空列表把「看不到」
// 读成「没人派给你」（同 /api/hot 的 wired/why 口径，也就是 MCP / CLI 两边报的那条）。
func TestContextSaysWhenHotIsNotWired(t *testing.T) {
	v, body := ctxOf(t, newHandler(t, "domains"), "/api/context?who=alice")
	if v.TasksWired {
		t.Fatalf("没接热层却报 tasks_wired=true：%s", body)
	}
	if v.TasksWhy == "" {
		t.Fatalf("tasks_wired=false 必须带上 why（那一段为什么答不了）：%s", body)
	}
	if v.Pack == nil || len(v.Pack.Todo) != 0 || len(v.Pack.Doing) != 0 {
		t.Fatalf("没接上就不该给出待办：%+v", v.Pack)
	}
	if len(v.TaskBands) != 0 {
		t.Fatalf("一条任务都没摆出来，不该有配色档：%+v", v.TaskBands)
	}
	var said bool
	for _, g := range v.Pack.Gaps {
		if strings.Contains(g, "热层没接上") {
			said = true
		}
	}
	if !said {
		t.Fatalf("报缺里要说清「这一段答不了」：%v", v.Pack.Gaps)
	}
}

// 热层接上了：两摞分得清（to = 你 / by = 你），别人的事一个字段都不漏进来；
// 配色档跟着状态走 —— 词表仍然只有一处说了算（timeline.Band），看板不另抄一份。
// 要单独一域时这两摞都不出现（同 CLI / MCP 那条纪律）。
func TestContextTasksAndBands(t *testing.T) {
	s := &Server{
		Vault: fixturePath(t, "domains"),
		Now:   func() time.Time { return fixedNow },
		Hot: &fakeHot{ttl: time.Minute, list: []hot.State{
			{ID: "case-2", Status: "running", Title: "把 Q3 结算的差异对一遍", To: "alice"},
			{ID: "case-1", Status: "blocked", Title: "等船公司回 ETA", By: "alice"},
			{ID: "case-9", Status: "running", Title: "bob 的事", By: "bob", To: "bob"},
		}},
	}
	v, body := ctxOf(t, s.Handler(), "/api/context?who=alice")
	if !v.TasksWired || v.TasksWhy != "" {
		t.Fatalf("接上了就该 wired=true 且不带 why：wired=%v why=%q", v.TasksWired, v.TasksWhy)
	}
	if v.Pack == nil || len(v.Pack.Todo) != 1 || v.Pack.Todo[0].ID != "case-2" {
		t.Fatalf("派给我的待办不对：%s", body)
	}
	if len(v.Pack.Doing) != 1 || v.Pack.Doing[0].ID != "case-1" {
		t.Fatalf("我在做的不对：%s", body)
	}
	if v.TaskBands["case-2"] != timeline.BandYellow || v.TaskBands["case-1"] != timeline.BandRed {
		t.Fatalf("配色档没跟状态走：%+v", v.TaskBands)
	}
	if _, ok := v.TaskBands["case-9"]; ok {
		t.Fatalf("别人的任务不该出现在这一页（连编号都不该有）：%+v", v.TaskBands)
	}

	scoped, body2 := ctxOf(t, s.Handler(), "/api/context?who=alice&domain=trade")
	if len(scoped.Pack.Todo) != 0 || len(scoped.Pack.Doing) != 0 || len(scoped.TaskBands) != 0 {
		t.Fatalf("单域问不该捎待办：%s", body2)
	}
}

// 热层接上了但这一会儿读不动：也是「答不了」，不是「没人派给你」—— why 里要把原因说清。
func TestContextSaysWhenHotReadFails(t *testing.T) {
	s := &Server{
		Vault: fixturePath(t, "domains"),
		Now:   func() time.Time { return fixedNow },
		Hot:   &fakeHot{ttl: time.Minute, err: errors.New("dial tcp 10.0.0.5:6379: connect refused")},
	}
	v, body := ctxOf(t, s.Handler(), "/api/context?who=alice")
	if v.TasksWired {
		t.Fatalf("读不动却报 tasks_wired=true：%s", body)
	}
	if !strings.Contains(v.TasksWhy, "读不了") {
		t.Fatalf("why 里要说清是读不动：%q", v.TasksWhy)
	}
	if len(v.Pack.Todo) != 0 || len(v.Pack.Doing) != 0 {
		t.Fatalf("读不动就不该给出待办：%+v", v.Pack)
	}
}
