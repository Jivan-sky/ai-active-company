package board

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"anc/internal/approvals"
	"anc/internal/hot"
	"anc/internal/timeline"
)

// boardFakeQueue 是热层的假替身。这一层测的是**看板那张嘴**（路由 / 方法 / Content-Type /
// 状态码），不是 Redis —— 「用真 Redis 跑一遍」留给沙箱实测，不起服务。
type boardFakeQueue struct {
	items map[string]hot.State
	drops []string
}

func newBoardFakeQueue() *boardFakeQueue { return &boardFakeQueue{items: map[string]hot.State{}} }

func (f *boardFakeQueue) Get(id string) (hot.State, bool, error) {
	s, ok := f.items[strings.TrimSpace(id)]
	return s, ok, nil
}

func (f *boardFakeQueue) Put(s hot.State) (hot.State, error) {
	f.items[strings.TrimSpace(s.ID)] = s
	return s, nil
}

func (f *boardFakeQueue) Drop(id string) error {
	delete(f.items, strings.TrimSpace(id))
	f.drops = append(f.drops, id)
	return nil
}

func (f *boardFakeQueue) List() ([]hot.State, error) {
	out := make([]hot.State, 0, len(f.items))
	for _, s := range f.items {
		out = append(out, s)
	}
	return out, nil
}

// seedPending 往假队列里放一条待批（走真入口 Enqueue，不手搓 hot.State）。
func seedPending(t *testing.T, q *boardFakeQueue, id string) approvals.Pending {
	t.Helper()
	p := approvals.Pending{
		ID: id, Who: "bob-bot", To: "经理", Domain: "ops", Project: "",
		Title: "bob 申请 ops 的权", Body: "想把退货那摊接过来。",
		Enqueued: "2026-10-10T20:00:00+08:00",
	}
	saved, _, err := approvals.Enqueue(q, p)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func boardWith(t *testing.T, vault string, q approvals.Queue) http.Handler {
	t.Helper()
	return (&Server{Vault: vault, Now: func() time.Time { return fixedNow }, Approvals: q}).Handler()
}

func post(t *testing.T, h http.Handler, path, ctype, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// 回执里的落点是**摘过本机路径**的（scrub），所以要读真文件得顺着 vault 目录找。
func readDirText(t *testing.T, dir string) string {
	t.Helper()
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读不到目录 %s：%v", dir, err)
	}
	var b strings.Builder
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			t.Fatal(err)
		}
		b.Write(raw)
	}
	return b.String()
}

type approvalFailBody struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
	Hint  string `json:"hint"`
}

// 热层没接上：读要如实说没接上（不拿空队列冒充「没人提」），写要直接拒（没有队列就没地方落痕）。
func TestApprovalsUnwiredSaysSoAndRefusesWrite(t *testing.T) {
	h := boardWith(t, fixturePath(t, "domains"), nil)

	rec := get(t, h, "/api/approvals")
	if rec.Code != http.StatusOK {
		t.Fatalf("读待批的状态码 %d，期望 200（内容有问题也回 200，把问题显示出来）", rec.Code)
	}
	var v ApprovalView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("不是合法 JSON：%v", err)
	}
	if v.Wired {
		t.Fatal("没有队列时必须报 wired=false")
	}
	if strings.TrimSpace(v.Why) == "" {
		t.Fatal("没接上就要说清为什么、怎么接")
	}
	if v.Error != "" {
		t.Fatalf("没接上不算读错：error=%q", v.Error)
	}
	if v.WritePath != WritePath {
		t.Fatalf("写口地址该由服务端给：%q，期望 %q", v.WritePath, WritePath)
	}
	if v.Pending == nil {
		t.Fatal("空的待批队列该出 []，不是 null —— 前端不该为这个判空")
	}
	if len(v.Signals) != 3 {
		t.Fatalf("该给三个信号，拿到 %+v", v.Signals)
	}
	for i, want := range []string{"approve", "reject", "hold"} {
		if v.Signals[i].Value != want {
			t.Fatalf("第 %d 个信号是 %q，期望 %q", i, v.Signals[i].Value, want)
		}
		if v.Signals[i].Label == "" {
			t.Fatalf("%s 缺人话", want)
		}
	}

	rec = post(t, h, WritePath, "application/json", `{"id":"p-1","by":"ou_a","signal":"approve"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("没队列时点头该拒（503），拿到 %d：%s", rec.Code, rec.Body.String())
	}
	var f approvalFailBody
	if err := json.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatalf("错误响应该是 JSON：%v", err)
	}
	if f.OK || f.Error == "" || f.Hint == "" {
		t.Fatalf("拒绝要说清错在哪、下一步做什么：%+v", f)
	}
}

// 出到 HTTP 上的必须是对外形状（snake_case）—— 不是 Go 的字段名。
func TestApprovalsListIsContractShape(t *testing.T) {
	q := newBoardFakeQueue()
	seedPending(t, q, "p-1")
	raw := get(t, boardWith(t, fixturePath(t, "domains"), q), "/api/approvals").Body.String()

	for _, want := range []string{`"write_path"`, `"enqueued"`, `"title"`, `"who"`, `"to"`, `"domain"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("对外契约里该有 %s：%s", want, raw)
		}
	}
	// 大写字段名 = 直接把领域类型端出去了。这条挡的是「改个字段名会静默改契约」。
	for _, bad := range []string{`"Enqueued"`, `"WritePath"`, `"Pending"`, `"Body"`} {
		if strings.Contains(raw, bad) {
			t.Fatalf("泄漏了 Go 字段名 %s：%s", bad, raw)
		}
	}

	var v ApprovalView
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	if !v.Wired || v.Count != 1 || len(v.Pending) != 1 {
		t.Fatalf("该读到 1 条待批：%+v", v)
	}
	if v.Pending[0].Enqueued != "2026-10-10T20:00:00+08:00" {
		t.Fatalf("进队时间该原样端出来：%q", v.Pending[0].Enqueued)
	}
}

// 判真第四档：**在待批队列里 = 有人在等你给一句话**（靛蓝 + 暂停形，A2A 的 input-required）。
//
// 两件事都要钉住：
//  1. 这一档由**队列这个事实**给（每一行都带），不从提案自己的 status 推；
//  2. 它**不进 timeline 的词表** —— 那根轴分的是真相源里的 status 词，两路数据不许混（#59）。
func TestApprovalsCarryInputRequiredBand(t *testing.T) {
	q := newBoardFakeQueue()
	seedPending(t, q, "p-1")
	raw := get(t, boardWith(t, fixturePath(t, "domains"), q), "/api/approvals").Body.String()

	var v ApprovalView
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Pending) != 1 {
		t.Fatalf("该读到 1 条待批：%+v", v)
	}
	if v.Pending[0].Band != BandInputRequired {
		t.Fatalf("待批那一行该带第四档 %q，实际 %q", BandInputRequired, v.Pending[0].Band)
	}
	// 这一档不许改写 status：status 是提案自己写的词，band 是队列这个事实，两回事。
	if v.Pending[0].Status == BandInputRequired {
		t.Fatal("band 把 status 盖掉了 —— 提案写的词该原样端出来")
	}
	// 两路数据不许混：这一档落进 timeline 那根轴里，只该是「认不出的词」。
	if got := timeline.Band(BandInputRequired); got != "unknown" {
		t.Fatalf("第四档不该在 timeline 的词表里（该落 unknown），实际 %q", got)
	}
}

// 看板点头 = 与 CLI 同一个入口、同一份留痕：落库 + 审计两笔，然后清掉队列里那条。
func TestApprovalsDecideLandsOnSameEntryAsCLI(t *testing.T) {
	vault := t.TempDir()
	q := newBoardFakeQueue()
	seedPending(t, q, "p-1")
	h := boardWith(t, vault, q)

	rec := post(t, h, WritePath, "application/json",
		`{"id":"p-1","by":"ou_bob","signal":"approve","why":"退货这摊归他"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("点头状态码 %d：%s", rec.Code, rec.Body.String())
	}
	var r DecideReply
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("回执不是合法 JSON：%v", err)
	}
	if !r.OK {
		t.Fatalf("该回 ok=true：%+v", r)
	}
	// verdict 是「同意：<理由>」（Decision.Verdict 把 why 带上）—— 这里只认词，不认整句。
	if r.Proposal != "p-1" || !strings.HasPrefix(r.Verdict, "同意") || !strings.Contains(r.Verdict, "退货") || r.By != "ou_bob" {
		t.Fatalf("回执该说清批了哪条、怎么批的、谁批的：%+v", r)
	}
	if !strings.Contains(r.Note, "grants/") {
		t.Fatalf("回执必须说清「真相源没动」：%q", r.Note)
	}

	if strings.TrimSpace(r.Timeline) == "" || strings.TrimSpace(r.Audit) == "" {
		t.Fatalf("回执该给两个落点：%+v", r)
	}
	// 落点是观测面给外人看的那一份：本机绝对路径要摘掉。
	if strings.Contains(r.Timeline, vault) || strings.Contains(r.Audit, vault) {
		t.Fatalf("回执泄漏了本机绝对路径：%q / %q", r.Timeline, r.Audit)
	}
	if !strings.Contains(r.Timeline, "<vault>") {
		t.Fatalf("落点该是摘过路径的：%q", r.Timeline)
	}

	tl := readDirText(t, filepath.Join(vault, "timeline"))
	for _, want := range []string{`"kind":"decision"`, `"case":"p-1"`, `"status":"done"`, `"by":"ou_bob"`, "同意", `"to":"经理"`} {
		if !strings.Contains(tl, want) {
			t.Fatalf("落库那行里该有 %s：%s", want, tl)
		}
	}
	au := readDirText(t, filepath.Join(vault, "audit"))
	for _, want := range []string{`"result":"ok"`, `"action":"invoke"`, `"actor":"ou_bob"`, `"source":"manual"`} {
		if !strings.Contains(au, want) {
			t.Fatalf("审计那行里该有 %s：%s", want, au)
		}
	}
	if len(q.drops) != 1 {
		t.Fatalf("结账后该清掉队列里那条，清了 %d 次", len(q.drops))
	}

	// 同一个 id 再点一次：不能静默成功（否则一次点头会变成两条痕）。
	rec = post(t, h, WritePath, "application/json",
		`{"id":"p-1","by":"ou_bob","signal":"approve"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("重复点头该 409，拿到 %d：%s", rec.Code, rec.Body.String())
	}
}

// 这张嘴只收「POST + application/json + 三个字 + 必带 by」——错的一律拒，且说清为什么。
func TestApprovalsDecideRefusals(t *testing.T) {
	newH := func(t *testing.T) (http.Handler, *boardFakeQueue) {
		q := newBoardFakeQueue()
		seedPending(t, q, "p-1")
		return boardWith(t, t.TempDir(), q), q
	}
	cases := []struct {
		name  string
		ctype string
		body  string
		want  int
	}{
		{"跨站表单发不动", "application/x-www-form-urlencoded", "id=p-1&signal=approve", http.StatusUnsupportedMediaType},
		{"不写 Content-Type", "", `{"id":"p-1","by":"ou_a","signal":"approve"}`, http.StatusUnsupportedMediaType},
		{"匿名点头", "application/json", `{"id":"p-1","signal":"approve"}`, http.StatusBadRequest},
		{"认不出的信号", "application/json", `{"id":"p-1","by":"ou_a","signal":"yes"}`, http.StatusBadRequest},
		{"没写提案 id", "application/json", `{"by":"ou_a","signal":"approve"}`, http.StatusBadRequest},
		{"请求体不是 JSON", "application/json", `{`, http.StatusBadRequest},
		{"不在队列里", "application/json", `{"id":"p-9","by":"ou_a","signal":"approve"}`, http.StatusConflict},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, q := newH(t)
			rec := post(t, h, WritePath, c.ctype, c.body)
			if rec.Code != c.want {
				t.Fatalf("状态码 %d，期望 %d：%s", rec.Code, c.want, rec.Body.String())
			}
			var f approvalFailBody
			if err := json.Unmarshal(rec.Body.Bytes(), &f); err != nil {
				t.Fatalf("错误响应该是 JSON：%v", err)
			}
			if f.OK || f.Error == "" {
				t.Fatalf("拒绝要出人话：%+v", f)
			}
			if len(q.drops) != 0 {
				t.Fatal("被拒的请求不该动队列")
			}
		})
	}
}

// 写口只有这一条路：别的方法、别的路径一律 405 —— 没有第二个写入口。
func TestApprovalsWriteIsOnlyPostOnThatPath(t *testing.T) {
	h := boardWith(t, t.TempDir(), newBoardFakeQueue())
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(m, WritePath, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s 状态码 %d，期望 405", m, WritePath, rec.Code)
		}
		// 动词不对就该说清「该用什么动词」—— 而不是让它落到处理函数上报一个假的 Content-Type 错。
		if allow := rec.Header().Get("Allow"); allow != "POST" {
			t.Fatalf("%s %s 的 Allow=%q，期望 POST", m, WritePath, allow)
		}
	}
	// 近似路径也不行（少写/多写一段都不该落到那张嘴上）。
	for _, p := range []string{"/api/approvals/decide/", "/api/approvals/decide/x"} {
		rec := post(t, h, p, "application/json", `{"id":"p-1","by":"ou_a","signal":"approve"}`)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST %s 状态码 %d，期望 405", p, rec.Code)
		}
	}
}
