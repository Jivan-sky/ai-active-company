// Package timeline —— 「决策与执行」的留存记录。
//
// 一句话：把「卡在哪、谁拍的板、agent 怎么执行的、成没成、根因是什么」落成**行文本**，
// 让看板能按 case 折叠成红 / 黄 / 绿三档。
//
// 三条口径（都刻意留了「以后不用重构」的余地）：
//
//  1. **真相源是 `timeline/*.jsonl`，不是数据库。** vault 归 git 管，
//     留存记录最需要的恰恰是 diff / review / 回滚 —— 二进制库这三样全丢。
//
//  2. **词表不锁死。** status 只认下面是绿色的那几个去配色，**认不出的原样保留**、
//     落「未知档」。以后加一个 `waiting` 之类的词，只改数据、不改这里。
//
//  3. **一个 case 多行 = 一次推进。** 折叠取「最后一条」就是当前态；
//     历史一行都不删 —— 用户要的「决策和执行都要留存记录」就是这条。
package timeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"anc/internal/audit"
)

// DirName 是 vault 下这一层的名字。它**不是** agent 的「数据来源」目录
// （那些是 10-knowledge / 20-ops 那种带编号、进 persona 路由表的），
// 所以和 charters/ 一样不带编号前缀。
const DirName = "timeline"

// 三档，2026-10-08 human 拍板：绿 = 已完成，黄 = 运行中，红 = 失败**或**有卡点需介入。
// 前两个词是等价的白色写法；认不出的词落 unknown，不报错（见包注释 2）。
const (
	BandGreen   = "green"
	BandYellow  = "yellow"
	BandRed     = "red"
	BandUnknown = "unknown"
)

// 档位词表：这**不是**白名单，只是「这几个词画成什么颜色」。
var bandOf = map[string]string{
	"done":    BandGreen,
	"running": BandYellow,
	"blocked": BandRed,
	"failed":  BandRed,
	// pending 是 approvals 那一侧自己写进热层的词（`prop:` 那条队列的当前态）。
	// 它落红：待批的意思就是「卡在等人点头」—— 按三档口径（红 = 失败**或**有卡点需介入）
	// 就是这一档。写在这里而不是卡片那一层：状态词画什么颜色**只此一处**说了算。
	"pending": BandRed,
}

// Band 把 status 映射成配色档。认不出的返回 BandUnknown —— **刻意不报错**：
// 数据里出现新词时，看板照常显示原词 + 灰点，而不是整页崩掉或者把词吞掉。
func Band(status string) string {
	if b, ok := bandOf[strings.ToLower(strings.TrimSpace(status))]; ok {
		return b
	}
	return BandUnknown
}

// Entry 是时间线上的一条记录。字段全部是**数据**：加字段不用迁移，
// 老文件里没这一行就是空值。
//
// 只有 ID / At / Title 是必需的（读的时候缺了算「读不懂」）——
// 其余留空不拦：现场先记一句话，比逼人填全字段然后干脆不记要好。
type Entry struct {
	ID      string   `json:"id"`                // 这一条的唯一 id
	Case    string   `json:"case,omitempty"`    // 归属的 case；空 = 自己就是一条
	At      string   `json:"at"`                // RFC3339
	Kind    string   `json:"kind,omitempty"`    // proposal / decision / execution / note（约定，不强卡）
	Status  string   `json:"status,omitempty"`  // 当前态；词表不锁死，见 Band
	By      string   `json:"by,omitempty"`      // 谁记的（bot 名或人）
	Role    string   `json:"role,omitempty"`    // 岗位（来自真相源，冗余一份便于看板不联表）
	To      string   `json:"to,omitempty"`      // 该交给谁 —— 下一手是谁的决定
	Domain  string   `json:"domain,omitempty"`  // 业务域（domains.md）
	Project string   `json:"project,omitempty"` // 项目
	Title   string   `json:"title"`             // 一句话
	Detail  string   `json:"detail,omitempty"`  // 展开
	Refs    []string `json:"refs,omitempty"`    // 文件 / 链接
}

// Key 是折叠用的分组键：有 case 就跟 case 走，没有就自己一条。
func (e Entry) Key() string {
	if strings.TrimSpace(e.Case) != "" {
		return strings.TrimSpace(e.Case)
	}
	return e.ID
}

// Time 解析 At。解析不了返回零值 + false（调用方据此判「读不懂」）。
func (e Entry) Time() (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(e.At))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// Doc 是一次读取的结果。
//
// Bad 是**读不懂的行**（逐条列出来，不是静默跳过）：时间线是拿来做判断的东西，
// 少了一行就可能把「卡住」看成「没事」。宁可报出来让人去看。
type Doc struct {
	Entries []Entry
	Bad     []string // "<文件名>:<行号>"
	// Tainted 是**磁盘上还躺着明文**的行（"<文件名>:<行号>"）—— 读口兜底（同 audit.Doc.Tainted）。
	// 这份代码落地之前写下的行，读出来是干净的，但文件本身还是脏的：如实报出来，别当成没事。
	Tainted []string
}

// Len 方便调用方判空。
func (d Doc) Len() int { return len(d.Entries) }

// scrub 把一条记录里**可能夹带原话**的那几格过一遍脱敏 —— 与审计吃同一张表（议题 #64）。
//
// 只碰 title / detail / refs 三格：
//   - title / detail 是自由文本，「当时说了一句什么」就写在这里；
//   - refs 是「文件 / 链接」，链接可以带查询串里的凭据；
//   - `case` / `by` / `role` / `to` / `domain` / `project` / `kind` / `status` / `id` / `at`
//     是**标识与词表**（按 case 折叠、按人按域筛，全靠它们），抹了就看不出「谁在推进哪一格」
//     —— 同审计 `CleanRecord` 只碰 object / why / detail 那条理由。
func scrub(e *Entry) []string {
	var hits []string
	for _, f := range []*string{&e.Title, &e.Detail} {
		s, h := audit.Scrub(*f)
		*f = s
		hits = append(hits, h...)
	}
	for i, r := range e.Refs {
		s, h := audit.Scrub(r)
		e.Refs[i] = s
		hits = append(hits, h...)
	}
	return hits
}

// Load 读整个 timeline 目录，按时间**升序**返回。
//
// 目录不存在 = 还没人记过 = 空时间线，**不是错误**（刚 init 的机器不该红）。
func Load(vault string) (Doc, error) {
	dir := filepath.Join(vault, DirName)
	names, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return Doc{}, nil
		}
		return Doc{}, err
	}
	var doc Doc
	for _, n := range names {
		if n.IsDir() || !strings.HasSuffix(n.Name(), ".jsonl") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, n.Name()))
		if err != nil {
			return Doc{}, err
		}
		lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
		for i, ln := range lines {
			s := strings.TrimSpace(ln)
			if s == "" || strings.HasPrefix(s, "#") {
				continue
			}
			var e Entry
			if err := json.Unmarshal([]byte(s), &e); err != nil {
				doc.Bad = append(doc.Bad, fmt.Sprintf("%s:%d", n.Name(), i+1))
				continue
			}
			if strings.TrimSpace(e.ID) == "" || strings.TrimSpace(e.Title) == "" {
				doc.Bad = append(doc.Bad, fmt.Sprintf("%s:%d", n.Name(), i+1))
				continue
			}
			if _, ok := e.Time(); !ok {
				doc.Bad = append(doc.Bad, fmt.Sprintf("%s:%d", n.Name(), i+1))
				continue
			}
			// 读口兜底：这份代码落地之前写下的行可能夹着明文。抹掉的那一份给调用方用，
			// 但「磁盘上这行是脏的」要如实记下来（文件本身一个字节都不动 —— 它是 append-only 的）。
			if len(scrub(&e)) > 0 {
				doc.Tainted = append(doc.Tainted, fmt.Sprintf("%s:%d", n.Name(), i+1))
			}
			doc.Entries = append(doc.Entries, e)
		}
	}
	sort.SliceStable(doc.Entries, func(i, j int) bool {
		ti, _ := doc.Entries[i].Time()
		tj, _ := doc.Entries[j].Time()
		return ti.Before(tj)
	})
	return doc, nil
}

// ErrAuthor：by 不是空，但压成文件名后什么都不剩（例如纯中文名）。
// 单独一个哨兵：看板要把它判成「填错了」，而不是「服务坏了」。
var ErrAuthor = errors.New("by 写不成文件名")

// Enter 一句话说明为什么会返回 error：只有「写不进去」才报错。
//
// Append 追加一条。文件 = `<YYYY-MM>.<author>.jsonl` —— **按作者分片**，
// 两个 agent 同时写也碰不到同一个文件（并发这件事就地消掉，不靠锁）。
// 返回写到的文件路径，便于调用方回显。
func Append(vault string, e Entry) (string, error) {
	if strings.TrimSpace(e.ID) == "" || strings.TrimSpace(e.Title) == "" {
		return "", fmt.Errorf("id 与 title 不能为空")
	}
	t, ok := e.Time()
	if !ok {
		return "", fmt.Errorf("at 必须是 RFC3339（例如 2026-10-08T02:41:00+08:00）")
	}
	rawBy := strings.TrimSpace(e.By)
	if rawBy == "" {
		return "", fmt.Errorf("by 不能为空 —— 谁记的就署谁，这是留存记录的第一性要求")
	}
	author := safeName(rawBy)
	if author == "" {
		return "", fmt.Errorf("%w（只用字母、数字、._- 与空格）：%q —— 用成员 slug 或英文名", ErrAuthor, rawBy)
	}
	dir := filepath.Join(vault, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, t.Format("2006-01")+"."+author+".jsonl")
	// 落盘前过一遍脱敏（同 audit.Append 的位置与理由）：写口是承诺的兑现处。
	scrub(&e)
	b, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return "", err
	}
	return path, nil
}

// safeName 把作者名变成能当文件名的样子：只留字母数字与 ._-
// （中文岗位名会被压成空 → 调用方报错让人换个 by，而不是写出个怪文件）。
func safeName(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "._-")
}

// Case 是折叠后的一个 case：一行代表「一个正在被推进的事」。
type Case struct {
	Key     string  `json:"key"`
	Title   string  `json:"title"`
	Status  string  `json:"status"`
	Band    string  `json:"band"`
	FirstAt string  `json:"first_at"`
	LastAt  string  `json:"last_at"`
	By      string  `json:"by,omitempty"`
	Role    string  `json:"role,omitempty"`
	To      string  `json:"to,omitempty"`
	Domain  string  `json:"domain,omitempty"`
	Project string  `json:"project,omitempty"`
	Entries int     `json:"entries"`
	Recent  []Entry `json:"recent"`
}

// recentKeep 是每个 case 带几行历史。5 够看「怎么走到这一步」，再多就该去翻原文了。
const recentKeep = 5

// Fold 把流水折成 case 列表，按**最后活动时间倒序**（最近动过的排前面）。
//
// 折叠口径：取最后一条的状态与移交对象 —— 当前态就是这一条；
// 历史留在 Recent 里，一行不丢。
func Fold(es []Entry) []Case {
	idx := map[string]int{}
	var cs []Case
	for _, e := range es {
		k := e.Key()
		i, ok := idx[k]
		if !ok {
			cs = append(cs, Case{Key: k, FirstAt: e.At})
			i = len(cs) - 1
			idx[k] = i
		}
		c := &cs[i]
		c.Entries++
		c.Title = e.Title
		c.Status = e.Status
		c.Band = Band(e.Status)
		c.LastAt = e.At
		// 谁 / 交给谁 / 岗位这类「当前该找谁」的信息：最后一条写了就用它的，
		// 没写就沿用上一条（不然后面只补一句进展，就把责任人抹没了）。
		if strings.TrimSpace(e.By) != "" {
			c.By = e.By
		}
		if strings.TrimSpace(e.Role) != "" {
			c.Role = e.Role
		}
		if strings.TrimSpace(e.To) != "" {
			c.To = e.To
		}
		if strings.TrimSpace(e.Domain) != "" {
			c.Domain = e.Domain
		}
		if strings.TrimSpace(e.Project) != "" {
			c.Project = e.Project
		}
		c.Recent = append(c.Recent, e)
	}
	for i := range cs {
		r := cs[i].Recent
		if len(r) > recentKeep {
			r = r[len(r)-recentKeep:]
		}
		// Recent 倒序给出（最新在上），前端直接铺。
		out := make([]Entry, 0, len(r))
		for j := len(r) - 1; j >= 0; j-- {
			out = append(out, r[j])
		}
		cs[i].Recent = out
	}
	sort.SliceStable(cs, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, cs[i].LastAt)
		tj, _ := time.Parse(time.RFC3339, cs[j].LastAt)
		return ti.After(tj)
	})
	if cs == nil {
		// 空切片一律 []，不出 null —— 前端 .length / .map 会崩（与看板其它契约同一条纪律）
		cs = []Case{}
	}
	return cs
}

// Counts 是三档计数（+ 认不出的档）。
type Counts struct {
	Green   int `json:"green"`
	Yellow  int `json:"yellow"`
	Red     int `json:"red"`
	Unknown int `json:"unknown"`
}

// Tally 数每个档有几个 case。
func Tally(cs []Case) Counts {
	var n Counts
	for _, c := range cs {
		switch c.Band {
		case BandGreen:
			n.Green++
		case BandYellow:
			n.Yellow++
		case BandRed:
			n.Red++
		default:
			n.Unknown++
		}
	}
	return n
}
