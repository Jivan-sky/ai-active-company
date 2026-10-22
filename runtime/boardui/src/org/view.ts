import { dash } from "../core/dom";
import type { AgentRow, AuditEntryView, BoardView, Data, DataflowBot, DomainRow, HotProjectView, LiveAgent, Member, ProjectRow } from "../core/types";
import { runState } from "../ui/status";

//
// 这一层只做「连接已有事实」，不造新事实：
//   - 人 → agent：同岗位 / 同业务域，两把尺子都写在标签上；
//   - 人 → 项目：业务域重叠 / 岗位负责人，同样标出来源；
//   - 项目进度：只有周期原文能解析成起止日期时才画一条「时间消耗」，
//     交付进度没有数据源就写「未提供」—— 宁缺，不编。

export const agentsOf = (v: BoardView): AgentRow[] => Array.isArray(v.agents) ? v.agents : [];

export function roleTitle(v: BoardView, role: string): string {
  const hit = v.roles.find((r) => r.role === role);
  return hit ? dash(hit.title) : dash(role);
}

export function domainOf(v: BoardView, slug: string): DomainRow | undefined {
  return v.domains.find((d) => d.slug === slug);
}

export function domainLabel(v: BoardView, slug: string): string {
  return domainOf(v, slug)?.name ?? dash(slug);
}

export function memberProjects(v: BoardView, m: Member): ProjectRow[] {
  return v.projects.filter((p) => m.domains.includes(p.domain) || (p.owner !== "" && p.owner === m.role));
}

export type AgentLink = { agent: AgentRow; why: string };

export function agentsForMember(v: BoardView, m: Member): AgentLink[] {
  const out: AgentLink[] = [];
  for (const a of agentsOf(v)) {
    const byRole = a.role !== "" && a.role === m.role;
    const byDomain = m.domains.includes(a.domain);
    if (!byRole && !byDomain) continue;
    out.push({ agent: a, why: byRole && byDomain ? "同岗位 · 同业务域" : byRole ? "同岗位" : "同业务域" });
  }
  return out;
}

export function agentsForDomain(v: BoardView, domain: string): AgentRow[] {
  return agentsOf(v).filter((a) => a.domain === domain);
}

// 一个业务 agent 现在在做什么：先看热层（进行中），再看时间线（已留痕的那条 case）。
// 两条都没有就如实说没有 —— 不拿项目名或「在线」冒充正在干活。
export function agentWork(d: Data, slug: string): { title: string; status: string; source: string } | null {
  const h = d.hot?.wired ? d.hot.entries.find((e) => e.to === slug || e.by === slug) : undefined;
  if (h) return { title: h.title, status: h.status, source: "热层" };
  const c = d.timeline?.wired ? d.timeline.cases.find((x) => x.to === slug || x.by === slug) : undefined;
  if (c) return { title: c.title, status: c.status, source: "时间线" };
  return null;
}

// 「最近调用」来自审计：它只能说明「最近连过」，不能说明「此刻在跑」。
// 在线判定只看 /api/agents 的心跳 —— 两件事在界面上必须分开写。
export function lastCall(d: Data, slug: string): AuditEntryView | null {
  if (!d.audit?.wired) return null;
  return d.audit.records.find((r) => r.actor === slug) ?? null;
}

export function recentCallers(d: Data): Set<string> {
  const out = new Set<string>();
  if (!d.audit?.wired) return out;
  const now = Date.now();
  for (const r of d.audit.records) {
    const t = Date.parse(r.at);
    if (!Number.isNaN(t) && now - t <= 24 * 3600 * 1000) out.add(r.actor);
  }
  return out;
}

// 一个 agent 的最后心跳。心跳层没接上、读失败、或根本没这条 → null（不知道，不当离线）。
export function liveAgentOf(d: Data, slug: string): LiveAgent | null {
  if (!d.agents?.wired || (d.agents.error ?? "") !== "") return null;
  return d.agents.entries.find((x) => x.slug === slug) ?? null;
}

// 成员名下 agent 的三档口径：接入 = agents.md 有 app_id；在线 = 最近心跳在阈值内；
// 心跳层没接上时 online 是 null —— 「不知道」不当 0。
export function memberAgentStats(d: Data, v: BoardView, m: Member): { total: number; wired: number; online: number | null; recent: number } {
  const links = agentsForMember(v, m);
  const wired = links.filter((x) => x.agent.wired).length;
  const seen = recentCallers(d);
  const recent = links.filter((x) => seen.has(x.agent.slug)).length;
  const hbWired = d.agents?.wired === true && (d.agents.error ?? "") === "";
  if (!hbWired) {
    return { total: links.length, wired, online: null, recent };
  }
  const online = links.filter((x) => liveAgentOf(d, x.agent.slug)?.online === true).length;
  return { total: links.length, wired, online, recent };
}

// 项目名 / bot 名由 org 的渲染规则派生：<company id>-<slug|name>。
// 只做「查一下有没有」，查不到就说没观测到，不拿别的 bot 顶上。
export function botProject(v: BoardView, ref: string): string {
  const id = (v.company.id ?? "").trim();
  return id === "" ? ref : id + "-" + ref;
}

export function flowBotOf(d: Data, project: string): DataflowBot | null {
  return d.dataflow?.bots.find((b) => b.project === project) ?? null;
}

export function botStateOf(d: Data, project: string): { cls: string; label: string } {
  if (!d.runtime || !d.runtime.wired) return { cls: "dot gray", label: "运行态未接入" };
  const hit = d.runtime.bots.find((b) => b.project === project);
  if (!hit) return { cls: "dot", label: "未观测到" };
  const st = runState(hit.state);
  return { cls: st.dot, label: st.label };
}

// 一个项目的热层汇总。null = 热层没接上 / 读不动 —— 不拿 0 冒充「没有在办的事」。
// 这是**在办任务数**，不是交付进度：已归档的事已经离开热层，算不出完成率。
export function hotProjectOf(d: Data, slug: string): HotProjectView | null {
  if (!d.hot?.wired || (d.hot.error ?? "") !== "") return null;
  // 老服务（v1）没有 projects 这一格：那是「问不出来」，不是「0 件在办」。
  if (!Array.isArray(d.hot.projects)) return null;
  return d.hot.projects.find((p) => p.slug === slug) ?? { slug, total: 0, blocked: 0, review: 0, overdue: 0 };
}

export type TaskLine = { title: string; status: string; band: string; at: string; href: string; source: string };

// 「当前任务」有三个来源：热层的当前态、时间线的 case、待批队列。名字对不上就写没有 ——
// 岗位名只在**这个岗位只有一个人**时才当命中，避免把同岗位两个人的事混到一张卡上。
export function memberTasks(d: Data, m: Member): TaskLine[] {
  const v = d.board;
  if (!v) return [];
  const names = new Set([m.name, m.display_name].filter((s) => s !== ""));
  const soleRole = v.members.filter((x) => x.role === m.role).length === 1;
  const hit = (s: string | undefined): boolean => {
    if (s === undefined || s === "") return false;
    return names.has(s) || (soleRole && s === m.role);
  };
  const out: TaskLine[] = [];
  if (d.hot?.wired) {
    for (const e of d.hot.entries) {
      if (!hit(e.by) && !hit(e.to)) continue;
      out.push({ title: e.title, status: e.status, band: "yellow", at: e.as_of ?? "", href: "#/system?tab=hot", source: "热层" });
    }
  }
  if (d.timeline?.wired) {
    for (const c of d.timeline.cases) {
      if (!hit(c.by) && !hit(c.to)) continue;
      out.push({ title: c.title, status: c.status, band: c.band, at: c.last_at, href: "#/timeline", source: "时间线" });
    }
  }
  if (d.approvals?.wired) {
    for (const p of d.approvals.pending) {
      if (!hit(p.who) && !hit(p.to)) continue;
      out.push({ title: p.title, status: p.status, band: p.band ?? "yellow", at: p.enqueued, href: "#/approvals", source: "待批" });
    }
  }
  return out;
}

export type DateWindow = { start: number; end: number };

// 只认「两个 YYYY-MM-DD + 一个分隔符」这一种写法。认不出就返回 null，
// 由调用方原样显示周期文本 —— 不把自由文本硬解释成日期。
export function periodWindow(period: string): DateWindow | null {
  const m = /(\d{4})-(\d{2})-(\d{2})\s*(?:→|->|~|—|–|至|到)\s*(\d{4})-(\d{2})-(\d{2})/.exec(period);
  if (!m) return null;
  const start = new Date(m[1] + "-" + m[2] + "-" + m[3] + "T00:00:00").getTime();
  const end = new Date(m[4] + "-" + m[5] + "-" + m[6] + "T23:59:59").getTime();
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return null;
  return { start, end };
}

export function periodState(period: string): { pct: number; note: string; cls: string } | null {
  const w = periodWindow(period);
  if (!w) return null;
  const now = Date.now();
  const pct = Math.max(0, Math.min(100, ((now - w.start) / (w.end - w.start)) * 100));
  const note = now < w.start ? "周期还没开始" : now > w.end ? "周期已结束" : "周期进行中";
  return { pct, note, cls: now > w.end ? "over" : now < w.start ? "idle" : "live" };
}

// 授权表里命中几个主体。-1 = 授权表没接入（与「0 条」不是一回事）。
export function grantsFor(d: Data, refs: string[]): number {
  if (!d.grants || !d.grants.wired) return -1;
  const set = new Set(refs.filter((x) => x !== ""));
  return d.grants.rows.filter((r) => r.who.some((w) =>
    set.has(w.raw) || (w.name !== undefined && set.has(w.name)) || (w.identity?.ref !== undefined && set.has(w.identity.ref)))).length;
}

