package retrieval

import (
	"strings"

	"anc/internal/org"
	renderpkg "anc/internal/render"
)

// Refusal 是「这次不给他看」的唯一回话形状。Reason 是为什么，Ask 是找谁。
//
// 它只描述**检索入口**的拒绝：身份解不出、成员停用、域没声明、域不在可见范围。
// 真正的权限事实仍然在 org；这里只是把 org 的判定翻译成检索要的 Scope。
type Refusal struct {
	Reason string `json:"reason"`
	Ask    string `json:"ask,omitempty"`
}

// ScopeFor 解「这个主体能搜哪些域」。who 为空 = 本机操作者（全部域）。
//
// 成员跨域口径与看板进门判定同源（admin 或 access.cross_domain=visible）；业务 agent
// **不跟着放宽** —— 它代的是岗位 / 流程，agents.md 的 domain 就是它的数据视野。
//
// 这个函数是 CLI / MCP / 看板共用的唯一入口：三个前端各写一遍，迟早会出现
// 「命令行说能看、看板说不能看」这种比没有权限判定更坏的漂移。
func ScopeFor(o *org.Org, who string) (Scope, *Refusal) {
	who = strings.TrimSpace(who)
	if who == "" {
		return Scope{All: true}, nil
	}
	id, ok := o.Identity(who)
	if !ok {
		return Scope{}, &Refusal{
			Reason: "我们公司没有这个人 —— 身份不许自报：who 必须能在 members/ 或 agents.md 里解出来",
		}
	}
	sc := Scope{Domains: id.Domains}
	if id.Kind == org.KindMember {
		v, found := o.ResolveMemberViewer(id.Ref)
		if !found {
			return Scope{}, &Refusal{Reason: "这个成员不在白名单里或已停用 —— 拒绝（停用是离职回收的第一道闸）"}
		}
		sc.All, sc.Domains = v.AllDomains, v.Domains
	}
	return sc, nil
}

// NarrowToDomain 把范围收成单独一域。空 slug = 不变。
//
// 先判「域有没有在 domains.md 里声明」，再判「这次范围能不能看见」；两件事分开报，
// 否则拼错域会被说成越权。收窄后的 Scope **仍包含公司级资料**（domain 空）——
// 与 Viewer.CanSeeDomain("") 同一条口径。
func NarrowToDomain(o *org.Org, sc Scope, slug string) (Scope, *Refusal) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return sc, nil
	}
	d, declared := o.Domain(slug)
	if !declared {
		return Scope{}, &Refusal{Reason: "domains.md 里没有「" + slug + "」这块业务 —— 要么拼错了，要么这一域还没抽成表行"}
	}
	if !sc.Allows(slug) {
		who := o.WhoLabel(d.Who)
		if who == "—" {
			who = ""
		}
		return Scope{}, &Refusal{Reason: renderpkg.RefuseDomainText(slug, who), Ask: who}
	}
	return Scope{Domains: []string{slug}}, nil
}

// ScopeLabels 是给人看的范围标签：["*"] = 全部域。
func ScopeLabels(sc Scope) []string {
	if sc.All {
		return []string{"*"}
	}
	return append([]string(nil), sc.Domains...)
}
