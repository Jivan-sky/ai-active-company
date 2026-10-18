package approvals

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"anc/internal/card"
	"anc/internal/org"
	"anc/internal/timeline"
)

// 这一份说的是**审批这类卡片的「值」从哪来**（2026-10-10 拍板）。
//
// 帧说长什么样（数据，住在文件里），这里说这一张卡片**此刻**要显示的东西 ——
// 全部现算：热层那条提案 + 真相源（谁、哪个岗位、属于谁）+ 钟。一个字段都不进代码。
//
// 槽位键是**这一处的契约**（帧里写 {{submitter}} 就用下面这个键）。加一个键 = 在这里加一行，
// 帧那边就能用了；帧里要一个没有的键，渲染会把它喊出来（见 card.fill），不会静默空着。
//
//	id / title / body / status / enqueued / waited / tone / company
//	domain / project / target / to
//	submitter / submitter_owner / submitter_kind / approver
const (
	SlotSubmitter      = "submitter"       // 谁提的：人话一句（名字 + 类别 + 挂在哪块业务）
	SlotSubmitterOwner = "submitter_owner" // 这个主体**属于谁**（业务 agent 才答得出来）
	SlotSubmitterKind  = "submitter_kind"  // member / agent / 认不出
	SlotApprover       = "approver"        // 该谁批：人名（岗位）—— 真人是现算的
)

// Facts 是这一张卡片此刻的值。o 可以是 nil（真相源没加载上）—— 那时身份那几格如实说不知道，
// **不出空白**：空白与「本来就没有」在卡片上分不出来。
func Facts(o *org.Org, p Pending, now time.Time) map[string]string {
	f := map[string]string{
		"id":       p.ID,
		"title":    p.Title,
		"body":     p.Body,
		"status":   p.Status,
		"enqueued": p.Enqueued,
		"waited":   humanWaited(p.Enqueued, now),
		"tone":     Tone(p.Status),
		"domain":   p.Domain,
		"project":  p.Project,
		"to":       p.To,
	}
	f["target"] = joinTarget(p.Domain, p.Project)
	f["company"] = companyOf(o)

	f[SlotSubmitter], f[SlotSubmitterKind], f[SlotSubmitterOwner] = whoSlots(o, p.Who)

	if o == nil {
		f[SlotApprover] = "没加载到 org 真相源"
	} else if strings.TrimSpace(p.To) == "" {
		f[SlotApprover] = "（没解出该谁批 —— 信封没写 scope.domain，或域表里没这个域）"
	} else {
		f[SlotApprover] = p.To + " · " + o.WhoLabel(p.To)
	}
	return f
}

// whoSlots 答「谁提的 / 哪一类 / 属于谁」这三格。
//
// 三样各答各的，**不互相代偿**：认不出这个主体就是认不出（同 org.Identity 的口径 ——
// 判不出来 = 这个主体不存在），不许拿「没写谁」或者别人的名字顶上去。
func whoSlots(o *org.Org, who string) (line, kind, owner string) {
	if o == nil {
		return "没加载到 org 真相源 —— 认不出这是谁", "认不出", "没加载到 org 真相源"
	}
	if strings.TrimSpace(who) == "" {
		return "（没写谁）", "认不出", "（没写谁 —— 无从谈起属于谁）"
	}
	id, ok := o.Identity(who)
	if !ok {
		return who + "（台账里没有这个主体）", "认不出", "认不出这个主体，因此答不了属于谁"
	}
	who2, why := o.Owner(id)
	if who2 != "" {
		return submitterLine(id), id.Kind, who2 + "（" + why + "）"
	}
	return submitterLine(id), id.Kind, why
}

// submitterLine 把一个主体写成卡片上那一句：**名字 + 类别**（成员答错人还能兜，
// 业务 bot 处理错单要赔钱，所以两类必须在卡片上分得开，见 SPEC §2.3）。
func submitterLine(id org.Identity) string {
	name := strings.TrimSpace(id.Name)
	if name == "" {
		name = id.Ref
	}
	switch id.Kind {
	case org.KindAgent:
		where := strings.Join(id.Domains, "、")
		if strings.TrimSpace(where) == "" {
			where = "没写挂在哪块业务"
		}
		line := name + "（" + KindWord(id.Kind) + " · " + where + "）"
		if ins := strings.TrimSpace(id.Inscription); ins != "" {
			line += "：" + ins
		}
		return line
	case org.KindMember:
		if role := strings.TrimSpace(id.Role); role != "" {
			return name + "（" + KindWord(id.Kind) + " · " + role + "）"
		}
		return name + "（" + KindWord(id.Kind) + "）"
	}
	return name + "（" + id.Kind + "）"
}

// KindWord 把主体类别写成给人看的词。**只此一处**：卡片、回执、日志都照这一份 ——
// 各写一遍的话，看的和记的早晚不是一个词。
func KindWord(kind string) string {
	switch kind {
	case org.KindMember:
		return "成员 bot"
	case org.KindAgent:
		return "业务 agent"
	}
	return kind
}

// joinTarget 是「这件事落在哪」那一句。两样都有就都写 —— 只写一个的时候，
// 收卡片的人会以为另一个不存在。
func joinTarget(domain, project string) string {
	var parts []string
	if d := strings.TrimSpace(domain); d != "" {
		parts = append(parts, "域 "+d)
	}
	if p := strings.TrimSpace(project); p != "" {
		parts = append(parts, "项目 "+p)
	}
	if len(parts) == 0 {
		return "（没写哪块业务）"
	}
	return strings.Join(parts, " · ")
}

func companyOf(o *org.Org) string {
	if o == nil {
		return ""
	}
	id, name := strings.TrimSpace(o.Company.ID), strings.TrimSpace(o.Company.Name)
	switch {
	case id != "" && name != "":
		return name + "（" + id + "）"
	case id != "":
		return id
	}
	return name
}

// Tone 把状态词落成**飞书 header 的模板名**。
//
// 档位仍然是 `timeline.Band` 那一处说了算（绿=完成 / 黄=运行中 / 红=失败或卡点需介入 /
// 认不出=灰）—— 这里只把四档翻成飞书那套颜色名，**不另立一套判据**。
func Tone(status string) string {
	switch timeline.Band(status) {
	case timeline.BandGreen:
		return "green"
	case timeline.BandYellow:
		return "yellow"
	case timeline.BandRed:
		return "red"
	}
	return "grey"
}

// humanWaited 是「已等多久」的人话。读不懂进队时间就照实说 —— 不猜成 0。
func humanWaited(enqueued string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(enqueued))
	if err != nil {
		return "进队时间读不懂"
	}
	d := now.Sub(t)
	switch {
	case d < 0:
		return "进队时间在未来（钟不对？）"
	case d < time.Minute:
		return "刚进队"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时", int(d.Hours()))
	default:
		return fmt.Sprintf("%d 天", int(d.Hours()/24))
	}
}

// Card 把这一条提案渲染成一张卡片（帧 + 现算的值），并把渲染时的发现一并交出来。
//
// 两个只有在这里才判得了的事，判完写成 Note（**不替换成别的、也不拦**）：
//   - 颜色名飞书认不认（认不出的话发出去会被平台拒 —— 让人知道，而不是悄悄改成蓝色）；
//   - 帧里要的槽位值里有没有（这个在 card.fill 里就喊了）。
func (p Pending) Card(f card.Frame, o *org.Org, now time.Time) (card.Card, []card.Note) {
	c, notes := f.Render(Facts(o, p, now))
	return c, append(notes, toneCheck(c)...)
}

// DoneFacts 是**收口那一帧**的值：提案那几格与待批帧同一套（帧可以互换），
// 外加这一次点头的结果：`verdict`（结论）/ `by` / `via` / `at` / `why`。
//
// 状态与颜色取**落库那一行写的那个**（`d.TimelineEntry`）—— 收口卡不另编一套色：
// 库里那笔痕什么色，卡片就什么色（同“颜色只此一处说了算”）。
func DoneFacts(o *org.Org, p Pending, d Decision) map[string]string {
	entry := d.TimelineEntry(p)
	f := map[string]string{
		"id":      p.ID,
		"title":   p.Title,
		"body":    p.Body,
		"status":  entry.Status,
		"tone":    Tone(entry.Status),
		"domain":  p.Domain,
		"project": p.Project,
		"target":  joinTarget(p.Domain, p.Project),
		"company": companyOf(o),
		"to":      p.To,
		"verdict": d.Signal.Verdict(),
		"by":      d.By,
		"via":     d.Via,
		"why":     d.Why,
		"at":      d.At.Format(time.RFC3339),
	}
	f[SlotSubmitter], f[SlotSubmitterKind], f[SlotSubmitterOwner] = whoSlots(o, p.Who)
	switch {
	case strings.TrimSpace(p.To) == "":
		f[SlotApprover] = "（没解出该谁批 —— 信封没写 scope.domain，或域表里没这个域）"
	case o == nil:
		f[SlotApprover] = "没加载到 org 真相源"
	default:
		f[SlotApprover] = p.To + " · " + o.WhoLabel(p.To)
	}
	return f
}

// DoneCard 把**一次点头之后**那张卡渲染出来（收口帧 + 现算的值）。
//
// 它只管渲染：**递出去、覆盖那一条消息不在这里** —— 那是网关那一侧的事（§7.1.39 口径：我们不自己连飞书 API）。
func (p Pending) DoneCard(o *org.Org, d Decision, f card.Frame) (card.Card, []card.Note) {
	c, notes := f.Render(DoneFacts(o, p, d))
	return c, append(notes, toneCheck(c)...)
}

// toneCheck 是两张卡片共用的那一声喊：颜色名飞书不认就如实说，
// **不替你换成蓝色** —— 悄悄改颜色比报错难查得多。
func toneCheck(c card.Card) []card.Note {
	if card.ToneOK(c.Header.Tone) {
		return nil
	}
	return []card.Note{{Where: "header",
		What: fmt.Sprintf("颜色 %q 飞书不认 —— 它只认 %s；发出去可能被平台拒（这里不替你换成别的）",
			c.Header.Tone, strings.Join(card.FeishuTones, " / "))}}
}

// Find 按提案 id 从队列里取一条。**不在队里返回 ok=false** —— 那不一定是错：
// 可能压根没提过，也可能已经结过账（同 ErrNotQueued 的两种情形）。
func Find(q Queue, id string) (Pending, bool, error) {
	if q == nil {
		return Pending{}, false, fmt.Errorf("没有队列 —— 热层没接上")
	}
	s, ok, err := q.Get(QueueID(id))
	if err != nil || !ok {
		return Pending{}, false, err
	}
	p, err := FromState(s)
	if err != nil {
		return Pending{}, false, err
	}
	return p, true, nil
}

// SlotKeys 是给帧作者看的那张键表：**这一版有哪些槽位能用**。
// 排序后返回（提示文案、--dump-frame 都照这一份，不各排一遍）。
func DoneSlotKeys() []string {
	keys := []string{
		"id", "title", "body", "status", "tone", "company",
		"domain", "project", "target", "to",
		"verdict", "by", "via", "why", "at",
		SlotSubmitter, SlotSubmitterOwner, SlotSubmitterKind, SlotApprover,
	}
	sort.Strings(keys)
	return keys
}

func SlotKeys() []string {
	keys := []string{
		"id", "title", "body", "status", "enqueued", "waited", "tone", "company",
		"domain", "project", "target", "to",
		SlotSubmitter, SlotSubmitterOwner, SlotSubmitterKind, SlotApprover,
	}
	sort.Strings(keys)
	return keys
}
