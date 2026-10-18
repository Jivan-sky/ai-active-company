// Package outbox —— 任务通知的可重放发件箱。
//
// 一句话：**通知会丢，所以先写成一行，再送出去。**
//
// 与 internal/notify 的分工：
//   - notify 回答「运行态红了没有」—— 判据来自 probe，收件人是管理员；
//   - outbox 回答「哪件任务该催谁」—— 判据来自任务当前态，收件人是负责人 / 验收人。
//
// 三条口径：
//
//  1. **append-only + 事件 id。** 同一件事（同一个任务版本 / 同一个截止时刻）只该有一条；
//     重跑生成器不会把同一条再写一遍。事件 id 是确定的，所以重放能去重。
//
//  2. **游标记「已送到」。** 发送失败不推进游标；下一次还能重放。发送是 at-least-once：
//     宁可重复一次，也不要「以为送了、其实没送」。
//
//  3. **只记事实，不记判断。** 超期是不是要升级、要不要改 stage —— 不在这里。
//     这里只把「该让人知道的事」写成一条可发送的记录。
package outbox

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"anc/internal/hot"
)

const Schema = "anc.outbox/v1"

// Kind 是这条通知为什么发。
const (
	KindOverdue       = "overdue"
	KindBlocked       = "blocked"
	KindReviewPending = "review_pending"
)

// Record 是一条待送出的通知。字段全是数据；加字段不用迁移。
type Record struct {
	ID          string `json:"id"`
	At          string `json:"at"`
	Kind        string `json:"kind"`
	Task        string `json:"task"`
	To          string `json:"to,omitempty"`
	Title       string `json:"title,omitempty"`
	Body        string `json:"body"`
	DueAt       string `json:"due_at,omitempty"`
	OverdueSecs int64  `json:"overdue_secs,omitempty"`
	Version     int64  `json:"version,omitempty"`
}

// Cursor 记「哪些 id 已经送到」。id 是事件 id，不是任务 id ——
// 同一件任务在不同版本上会各有一条通知，送过的不能因为版本变了就当没送。
type Cursor struct {
	Schema string            `json:"schema"`
	Sent   map[string]string `json:"sent"`
}

// Path 是发件箱落点。它跟 gateway 的 data_dir 在一起，不进 git。
func Path(dataDir string) string {
	return filepath.Join(dataDir, "state", "task-outbox.jsonl")
}

// CursorPath 是发送游标。
func CursorPath(dataDir string) string {
	return filepath.Join(dataDir, "state", "task-outbox.cursor.json")
}

// Plan 把任务当前态折成「这一轮该发什么」。纯函数：无时钟、无盘、无网。
//
// 只发三类需要人动手的事：
//   - 已超期（due 已过，且不在终态）；
//   - 卡住（stage=blocked）；
//   - 等验收（stage=review 且结论仍是 pending）。
//
// queued / running 的正常推进不发 —— 那是过程，不是异常。
func Plan(tasks []hot.State, now time.Time) []Record {
	out := make([]Record, 0, len(tasks))
	for _, s := range tasks {
		if s.ID == "" {
			continue
		}
		title := strings.TrimSpace(s.Title)
		if title == "" {
			title = s.ID
		}
		if s.Overdue(now) {
			late, _ := s.OverdueFor(now)
			out = append(out, Record{
				ID:          fmt.Sprintf("overdue:%s:%s:v%d", s.ID, s.DueAt, s.Version),
				At:          now.Format(time.RFC3339),
				Kind:        KindOverdue,
				Task:        s.ID,
				To:          strings.TrimSpace(s.To),
				Title:       title,
				Body:        fmt.Sprintf("任务《%s》已超期 %s（截止 %s）。当前阶段 %s，负责人 %s。", title, humanDur(late), s.DueAt, dash(s.Stage), dash(s.To)),
				DueAt:       s.DueAt,
				OverdueSecs: int64(late / time.Second),
				Version:     s.Version,
			})
		}
		if s.Stage == hot.StageBlocked {
			out = append(out, Record{
				ID:      fmt.Sprintf("blocked:%s:v%d", s.ID, s.Version),
				At:      now.Format(time.RFC3339),
				Kind:    KindBlocked,
				Task:    s.ID,
				To:      strings.TrimSpace(s.To),
				Title:   title,
				Body:    fmt.Sprintf("任务《%s》卡住，下一手 %s。卡点：%s", title, dash(s.To), lastLine(s.Note)),
				DueAt:   s.DueAt,
				Version: s.Version,
			})
		}
		if s.Stage == hot.StageReview && strings.TrimSpace(s.Review.Verdict) == hot.VerdictPending {
			reviewer := firstNonEmpty(s.Review.By, s.To)
			out = append(out, Record{
				ID:      fmt.Sprintf("review:%s:v%d", s.ID, s.Version),
				At:      now.Format(time.RFC3339),
				Kind:    KindReviewPending,
				Task:    s.ID,
				To:      reviewer,
				Title:   title,
				Body:    fmt.Sprintf("任务《%s》等待 %s 验收。验收口径：%s", title, dash(reviewer), clip(firstNonEmpty(s.Accept, "（没写验收口径）"), 200)),
				DueAt:   s.DueAt,
				Version: s.Version,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Task != out[j].Task {
			return out[i].Task < out[j].Task
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// Load 读发件箱。文件不存在 = 空箱；有一行读不懂就报错 ——
// 静默跳过等于把一条通知弄丢，还让人以为它从没发生过。
func Load(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Record
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for ln := 1; sc.Scan(); ln++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("发件箱 %s 第 %d 行读不懂：%w", path, ln, err)
		}
		if r.ID == "" {
			return nil, fmt.Errorf("发件箱 %s 第 %d 行没有 id —— 没有 id 就没法去重 / 重放", path, ln)
		}
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		out = append(out, r)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Append 只追加还没出现过的事件。返回真正写进去几条。
// 不覆盖、不删除 —— 重放靠游标，历史留在文件里。
func Append(path string, recs []Record) (int, error) {
	if len(recs) == 0 {
		return 0, nil
	}
	old, err := Load(path)
	if err != nil {
		return 0, err
	}
	seen := make(map[string]bool, len(old)+len(recs))
	for _, r := range old {
		seen[r.ID] = true
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return 0, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	for _, r := range recs {
		if strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.At) == "" || strings.TrimSpace(r.Kind) == "" {
			return n, fmt.Errorf("发件箱记录缺 id / at / kind：%+v", r)
		}
		if seen[r.ID] {
			continue
		}
		b, err := json.Marshal(r)
		if err != nil {
			return n, err
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			return n, err
		}
		seen[r.ID] = true
		n++
	}
	if err := f.Sync(); err != nil {
		return n, err
	}
	return n, nil
}

// LoadCursor 读游标。文件不存在 = 还没送过任何一条。
func LoadCursor(path string) (Cursor, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Cursor{Schema: Schema, Sent: map[string]string{}}, nil
		}
		return Cursor{}, err
	}
	var c Cursor
	if err := json.Unmarshal(b, &c); err != nil {
		return Cursor{}, fmt.Errorf("发件箱游标 %s 解析不了（删掉它会重放全部）：%w", path, err)
	}
	if c.Sent == nil {
		c.Sent = map[string]string{}
	}
	c.Schema = Schema
	return c, nil
}

// Save 原子写游标。
func (c Cursor) Save(path string) error {
	if c.Sent == nil {
		c.Sent = map[string]string{}
	}
	c.Schema = Schema
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Mark 记「这条送到了」。
func (c *Cursor) Mark(id string, at time.Time) {
	if c.Sent == nil {
		c.Sent = map[string]string{}
	}
	c.Sent[id] = at.Format(time.RFC3339)
}

// Unsent 按文件顺序挑出还没送过的。顺序稳定，重放可预期。
func Unsent(records []Record, cur Cursor) []Record {
	out := make([]Record, 0, len(records))
	for _, r := range records {
		if _, ok := cur.Sent[r.ID]; !ok {
			out = append(out, r)
		}
	}
	return out
}

// Text 是推到聊天里的正文。只带业务事实，不带本机路径、不带凭据。
func Text(r Record) string {
	head := map[string]string{
		KindOverdue:       "[任务超期]",
		KindBlocked:       "[任务卡住]",
		KindReviewPending: "[待验收]",
	}[r.Kind]
	if head == "" {
		head = "[任务提醒]"
	}
	return head + " " + r.Task + "\n" + r.Body + "\n（anc task notify · 同一事件只送一次；失败会留在发件箱里等重放）"
}

func humanDur(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%.1f 天", d.Hours()/24)
	case d >= time.Hour:
		return fmt.Sprintf("%.1f 小时", d.Hours())
	case d >= time.Minute:
		return fmt.Sprintf("%d 分钟", int(d.Minutes()))
	default:
		return fmt.Sprintf("%d 秒", int(d.Seconds()))
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return clip(line, 200)
		}
	}
	return "（没写卡点说明）"
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(b)
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "（空）"
	}
	return strings.TrimSpace(s)
}
