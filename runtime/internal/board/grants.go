package board

import (
	"net/http"
	"sort"
	"strings"

	"anc/internal/ctxpack"
	"anc/internal/org"
)

// GrantsSchema 是这一份视图的版本号（同 /api/assets 的理由）。
const GrantsSchema = "anc.board.grants/v1"

// GrantsView 是 /api/grants 的形状 -- 「谁被允许对谁做什么」。
//
// 为什么单独一页：设计记录 §7.1.15 就写着「看板上「谁能做什么」那个页（投影里已经有数据，
// 页上还没画）」-- 投影（anc.board/v1 的 grants）一直带着这张表，没人画。
//
// 与「数据流」页的分工：那一页答「通道开着吗」（上游 config 的策略面 + 执行面事实），
// 这一页只答「谁被允许对谁做什么」（真相源里的授权表）。两个数据源、两个变化节奏。
//
// 三条口径：
//
//  1. **全量、不筛选、不脱敏** -- 与投影同一条纪律（看板是观测面）。授权表里**本来就没有凭据**
//     （SPEC §6：它记的是「谁被允许做什么」，凭据怎么发是执行层的事），所以这一页也不带
//     app_id / app_secret。bot 侧那个「只出与我相关的几条」的视图**不在这里**：
//     两个消费者、两个视图，不共用一份产物（共用迟早把看板的全量漏给 bot）。
//  2. **视角（?who=）只筛、不改写**：把与这个主体相关的行露出来。认不出的 who **照实说认不出**，
//     不退回全量冒充「就是这些」，也不给一份空表冒充「它什么都没有」。
//  3. **这一页不判「这条权该不该给」** -- 判断归人（提案 + 人点头）。事实层的发现照抄
//     org 那张规则表，档次由它定，这一层不硬编码。
type GrantsView struct {
	Schema string      `json:"schema"`
	Wired  bool        `json:"wired"`
	Total  int         `json:"total"` // 表里一共几条（不受视角影响）
	Shown  int         `json:"shown"` // 这个视角下露出来几条
	Scope  GrantsScope `json:"scope"`
	Rows   []GrantsRow `json:"rows"`
	// Subjects 是**全表**里出现过的那些主体（不受视角影响）—— 视角选择器从这儿取。
	// 不能从 rows 里取：一旦按某个视角筛过，rows 只剩他相关的，选择器会跟着缩水，
	// 于是「换个视角看看」这条路就走不通了（换不回去、也换不到第二个人）。
	Subjects []ctxpack.IdentityOut `json:"subjects"`
	Findings []Finding             `json:"findings"`
	Missing  []string              `json:"missing"`
	Note     string                `json:"note"`
	Error    string                `json:"error,omitempty"`
}

// GrantsScope 是「我在看哪个主体」-- ?who= 那一格。
//
// Known=false 时 Who 为空、Why 说清是什么写法：**不给它配一份全量表**，
// 否则「认不出的人」会被读成「他的权限是全部」。
type GrantsScope struct {
	Ref   string               `json:"ref"`
	Known bool                 `json:"known"`
	Who   *ctxpack.IdentityOut `json:"who,omitempty"`
	Why   string               `json:"why,omitempty"`
}

// GrantsRow 一条授权：SPEC §6 那七个字段 + slug / path 原样搬，另加「这一行里的引用解出来是谁」。
//
// 原样搬是刻意的：这一页是给人**核对**的，转述一遍就等于多一处会漂的副本。
type GrantsRow struct {
	Slug       string `json:"slug"`
	Path       string `json:"path"`
	From       string `json:"from"`
	To         string `json:"to"`
	Action     string `json:"action"`
	Object     string `json:"object"`
	TTL        string `json:"ttl"`
	OnBehalfOf string `json:"on_behalf_of"`
	Reason     string `json:"reason"`
	Body       string `json:"body"`
	// Who 是这一行里出现的引用（from / to / on_behalf_of）各解出来是什么。
	// 解不出来的**照原样留着**（Kind="" + Note），不猜、不丢。
	Who []GrantsRef `json:"who"`
	// Hit 是视角命中（不带 who 时恒 true）。
	Hit bool `json:"hit"`
}

// GrantsRef 是「一行里的某一格引用」解出来的东西。
type GrantsRef struct {
	Slot     string               `json:"slot"` // from | to | on_behalf_of
	Raw      string               `json:"raw"`  // 原文里写的那个串
	Kind     string               `json:"kind"` // member | agent | role | domain | ""（认不出）
	Name     string               `json:"name,omitempty"`
	Note     string               `json:"note,omitempty"` // 认不出时的一句人话
	Identity *ctxpack.IdentityOut `json:"identity,omitempty"`
}

// handleGrants 把授权表端出来。用 200 而不是 5xx：读不到真相源**就是它要来报的那件事**
// （同 /api/assets）、一条授权都没有也是合法状态（SPEC §6 不变量 1：没有 grant 就是不通）。
func (s *Server) handleGrants(w http.ResponseWriter, r *http.Request) {
	view := GrantsView{
		Schema:   GrantsSchema,
		Rows:     []GrantsRow{},
		Subjects: []ctxpack.IdentityOut{},
		Missing:  grantMissing(),
		Note: "这一页答的是**授权表**（`grants/`，一个 grant 一个文件）：谁被允许对谁做什么。" +
			"它是控制面真相源，不是执行层 -- 一条权写在这里，执行层有没有真的拦住，看「数据流」页与 " +
			"`anc envelope check`。表默认零条 = 还没有任何授权，不算错（SPEC §6 不变量 1：没有 grant 就是不通）。",
	}
	view.Scope.Ref = strings.TrimSpace(r.URL.Query().Get("who"))
	// 不带视角 = **全量**：没有认不出的东西。这与「带了个认不出的 who」是两件事 ——
	// 后者不许退化成全量（那会被读成「他的权限是全部」），页面要能分开说。
	view.Scope.Known = view.Scope.Ref == ""
	o, err := org.Load(s.Vault)
	if err != nil {
		view.Error = "真相源读不动，授权表算不出来（不猜）：" + s.scrub(firstLine(err.Error()))
		writeJSON(w, http.StatusOK, view)
		return
	}
	view.Wired = true
	view.Total = len(o.Grants)

	// 视角：要的是**主体**（成员 / 业务 agent）。认不出就说认不出 --
	// 岗位 / 域不是主体（域是客体，SPEC §6），它们不该被当成「我在看谁」。
	// 滤不滤由**给没给** who 定（不是由认没认出来定）：认不出 → 一条都不命中 → 空表 + 认不出。
	scopeKey := ""
	if view.Scope.Ref != "" {
		if idn, ok := o.Identity(view.Scope.Ref); ok {
			view.Scope.Known = true
			view.Scope.Who = ctxpack.IdentityOf(o, idn)
			scopeKey = identityKey(idn.Kind, idn.Ref)
		} else {
			view.Scope.Why = "真相源里没有这个编号身份（写法：成员名 | 业务 agent 的 slug）-- " +
				"岗位与域不是主体，不能当视角（域是客体，SPEC §6）"
		}
	}

	var all []GrantsRow
	for _, g := range o.Grants {
		row := GrantsRow{
			Slug: g.Slug, Path: g.Path,
			From: g.From, To: g.To, Action: g.Action, Object: g.Object,
			TTL: g.TTL, OnBehalfOf: g.OnBehalfOf, Reason: g.Reason, Body: g.Body,
			Who: []GrantsRef{},
			Hit: view.Scope.Ref == "",
		}
		for _, slot := range []struct{ name, raw string }{
			{"from", g.From}, {"to", g.To}, {"on_behalf_of", g.OnBehalfOf},
		} {
			if strings.TrimSpace(slot.raw) == "" {
				continue
			}
			ref := resolveGrantRef(o, slot.name, slot.raw)
			row.Who = append(row.Who, ref)
			if scopeKey != "" && refKey(ref) == scopeKey {
				row.Hit = true
			}
		}
		all = append(all, row)
	}
	// 选择器从**筛之前**那一份取（见 grantSubjects 的注释），表里露出来的才是上面那份视角的结果。
	view.Subjects = grantSubjects(all)
	for _, r := range all {
		if r.Hit {
			view.Rows = append(view.Rows, r)
		}
	}
	view.Shown = len(view.Rows)

	// 发现：授权表那十条规则的 id 一律以 `grant.` 开头（rules.go 的出厂表），按前缀取。
	// **不按目录取** -- grants/ 里将来可能放别的说明文件，那不是授权表的发现；
	// 档次用的是 org 已经算好的生效档（这里不硬编码）。
	view.Findings = []Finding{}
	for _, f := range findings(o.Warnings) {
		if strings.HasPrefix(f.Rule, "grant.") {
			view.Findings = append(view.Findings, f)
		}
	}
	sort.SliceStable(view.Findings, func(i, j int) bool {
		if view.Findings[i].Where != view.Findings[j].Where {
			return view.Findings[i].Where < view.Findings[j].Where
		}
		return view.Findings[i].Rule < view.Findings[j].Rule
	})
	writeJSON(w, http.StatusOK, view)
}

// grantSubjects 收**全表**里出现过的主体（成员 / 业务 agent），去重后按编号排。
//
// 只收**主体**：岗位与域不是主体（域是客体，SPEC §6），摆在视角选择器上会让人以为
// 「可以站在一个域的位置上看」—— 那是另一件事，也还没有定义处。
// 调用方要在**筛之前**那一份 rows 上调它（视角选择器不该被视角自己缩水）。
func grantSubjects(rows []GrantsRow) []ctxpack.IdentityOut {
	seen := map[string]bool{}
	out := []ctxpack.IdentityOut{}
	for _, r := range rows {
		for _, ref := range r.Who {
			if ref.Identity == nil {
				continue
			}
			key := identityKey(ref.Identity.Kind, ref.Identity.Ref)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, *ref.Identity)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Ref < out[j].Ref
	})
	return out
}

// grantMissing 是这一页**答不了**的事，如实列 -- 与「数据流」页同一个纪律：
// 缺的部分不许用「图好看」补上。
//
// 每一条都是当前口径里**明确挂起或未做**的，不是漏写；改口径要连带改这里。
func grantMissing() []string {
	return []string{
		"期限到点怎么失效：**已拍挂起**（ttl 原样透出、这一层不判）-- 默认永久，等真有客户写下期限再做",
		"级联失效 / 撤回即时生效：未做（离职是外部事件，进 HR 与客户的管理系统那条路；断开是「给出去」的逆向操作）",
		"动作维度：词表只认 read / write / invoke，不在表里的照收；动作维度的判据还没有定义处",
		"客体的词表（通道 / 看板分区）：没有定义处，所以校验只做到「from 解得出主体」",
		"这一页不判「这条权该不该给」-- 判断归人（提案 + 人点头），机器只做校验与留痕",
		"bot 侧那个「只出与我相关的几条」的视图不在这里：两个消费者、两个视图，不共用一份产物",
	}
}

// resolveGrantRef 把一个引用解成「是什么」。四条出路，都不猜：
//
//	成员 / 业务 agent -> 走 o.Identity（**唯一**一条「编号 → 主体」的转换，看板不自己拼一份）
//	岗位 role:x       -> 标出它是岗位（能授权的主体之一，但不是「一个人」）
//	域 domain:x       -> 标出它是域（**客体**，不能当授权者）
//	其余              -> Kind 空 + Note 说清，原文照留
func resolveGrantRef(o *org.Org, slot, raw string) GrantsRef {
	ref := GrantsRef{Slot: slot, Raw: raw}
	kind, name := org.SplitRef(raw)
	switch kind {
	case "", "member", "agent":
		if idn, ok := o.Identity(name); ok {
			ref.Kind, ref.Name, ref.Identity = idn.Kind, idn.Name, ctxpack.IdentityOf(o, idn)
			return ref
		}
		if kind == "" {
			if _, ok := o.Roles[name]; ok {
				ref.Kind, ref.Name = "role", name
				return ref
			}
		}
		ref.Note = "真相源里没有这个编号身份 -- 这条链指着谁，现在答不了"
	case "role":
		if _, ok := o.Roles[name]; ok {
			ref.Kind, ref.Name = "role", name
			return ref
		}
		ref.Kind, ref.Name = "role", name
		ref.Note = "岗位表里没有这个岗位"
	case "domain":
		ref.Kind, ref.Name = "domain", name
		ref.Note = "域是客体、不是主体：域不会授权（SPEC §6）"
	default:
		ref.Name = name
		ref.Note = "认不出的写法（前缀 " + kind + " 没有定义处）"
	}
	return ref
}

// refKey 是「这个引用指的是同一个主体吗」的比对键。
//
// 只对**主体**成立（成员 / 业务 agent）：岗位与域不是主体，一律给空键 --
// 这样它们既不会被视角命中，也不会与别的引用撞车。
func refKey(r GrantsRef) string {
	if r.Identity == nil {
		return ""
	}
	return identityKey(r.Identity.Kind, r.Identity.Ref)
}

// identityKey 与视图的其余部分同一把尺子：kind + 编号身份。
func identityKey(kind, ref string) string {
	if strings.TrimSpace(kind) == "" || strings.TrimSpace(ref) == "" {
		return ""
	}
	return kind + ":" + ref
}
