import type { BoardView, Data, Kid, Member, ProjectRow } from "../core/types";
import { card, dash, el, section } from "../core/dom";
import { hashQuery } from "../core/query";
import { openMemberDrawer, openProjectDrawer } from "../org/drawers";
import { domainSection, roleSection, routingSection } from "../org/sections";
import { agentsForDomain, agentsForMember, agentsOf, botProject, botStateOf, domainLabel, memberAgentStats, memberTasks, periodState, roleTitle } from "../org/view";
import { cannot, emptyNote, noBoard, statSection } from "../ui/blocks";

// 组织 —— 读者：管理者 / FDE。组织长什么样、边界在哪。
export function renderOrg(d: Data): Kid[] {
  const v = d.board;
  if (!v) return [noBoard(d)];
  return [domainSection(v), roleSection(v), routingSection(v)];
}

// 组织总览 —— 只放「公司是什么、数据从哪走」；域、角色、项目各自单独一页。
export function renderOrgOverview(d: Data): Kid[] {
  const v = d.board;
  if (!v) return [noBoard(d)];
  const c = v.company;
  return [
    section("公司", "投影里的公司档案：字段来自真相源，不在前端拼",
      el("div", { class: "grid" },
        card(dash(c.name), dash(c.id), [
          ["平台", dash(c.platform)],
          ["语言", dash(c.language)],
          ["时区", dash(c.timezone)],
          ["schema", el("span", { class: "mono" }, dash(v.schema))],
        ]),
        card("数据路由", "顶层目录 → 说明", v.routing.length === 0
          ? [["状态", cannot("还没有顶层数据目录")]]
          : v.routing.slice(0, 5).map((r): [string, Kid] => [r.dir, dash(r.summary)])))),
    statSection(v),
    routingSection(v),
  ];
}

// 业务域 —— 从「组织」里拆出来：一块业务一行，回答「这是哪块业务、数据在哪、卡住找谁」。
export function renderDomains(d: Data): Kid[] {
  const v = d.board;
  if (!v) return [noBoard(d)];
  return [
    domainSection(v),
    section("这一页只答什么", "域是罗盘，不是项目清单",
      emptyNote("项目、角色、授权各自另有页。这里只回答「一块业务是什么、谁在做、数据在哪」。")),
  ];
}

// 成员与 Agent —— 一人一张卡。卡片只摆「能对上号」的关系：
// 同岗位 / 同业务域的 agent 标出来源；任务只从时间线与待批队列取。
export function memberCard(d: Data, v: BoardView, m: Member, badge?: string): HTMLElement {
  const links = agentsForMember(v, m);
  const tasks = memberTasks(d, m);
  const stats = memberAgentStats(d, v, m);
  const bot = botStateOf(d, botProject(v, m.name));
  const state = m.disabled ? "已停用" : m.admin ? "管理员" : "启用";
  const mark = (m.display_name || m.name).trim().slice(0, 1).toUpperCase() || "?";
  const node = el("article", {
    class: "entity-card member-card" + (m.disabled ? " is-off" : ""),
    role: "button", tabindex: "0", "aria-label": "打开 " + dash(m.display_name || m.name) + " 的详情",
  },
    el("header", { class: "ec-head" },
      el("span", { class: "avatar" }, mark),
      el("div", { class: "ec-id" },
        el("h3", {}, dash(m.display_name || m.name)),
        el("p", {}, roleTitle(v, m.role), el("span", { class: "dot-sep" }, "·"), el("span", { class: "mono" }, dash(m.name)))),
      el("span", { class: "ec-state" + (m.disabled ? " off" : "") }, state)),
    el("div", { class: "ec-tags" },
      ...(badge === undefined ? [] : [el("span", { class: "ec-tag on" }, badge)]),
      ...(m.domains.length === 0
        ? [el("span", { class: "ec-tag muted" }, "未挂业务域")]
        : m.domains.map((x) => el("span", { class: "ec-tag" }, domainLabel(v, x))))),
    el("p", { class: "ec-line" }, links.length === 0
      ? "没有挂得上的人 ↔ agent 关系"
      : links.slice(0, 3).map((x) => dash(x.agent.name || x.agent.slug) + "（" + x.why + "）").join("、")
        + (links.length > 3 ? " 等 " + links.length + " 个" : "")),
    el("div", { class: "ec-foot" },
      el("span", {}, "Agent ", el("b", {}, String(stats.total))),
      el("span", {}, "接入 ", el("b", {}, String(stats.wired))),
      el("span", {}, "在线 ", el("b", {}, stats.online === null ? "—" : String(stats.online))),
      el("span", {}, "近期 ", el("b", {}, String(stats.recent))),
      el("span", {}, "任务 ", el("b", {}, String(tasks.length))),
      el("span", { class: "ec-run" }, el("span", { class: bot.cls }), bot.label)));
  const open = (): void => openMemberDrawer(d, v, m);
  node.addEventListener("click", open);
  node.addEventListener("keydown", (e) => {
    if (e.key === "Enter" || e.key === " ") { e.preventDefault(); open(); }
  });
  return node;
}
export function renderMembers(d: Data): Kid[] {
  const v = d.board;
  if (!v) return [noBoard(d)];
  const people = v.members.slice().sort((a, b) =>
    Number(a.disabled) - Number(b.disabled) || (a.display_name || a.name).localeCompare(b.display_name || b.name, "zh"));
  const vac = v.roles.filter((r) => !v.members.some((m) => m.role === r.role && !m.disabled));
  const orphans = agentsOf(v).filter((a) =>
    !v.members.some((m) => !m.disabled && agentsForMember(v, m).some((l) => l.agent.slug === a.slug)));
  // 「我的卡片」不猜登录态：URL 上的 who 优先，没写就取第一个启用中的管理员。
  // 它只决定以谁的视角看，不改任何权限 —— 看板不做登录校验。
  const want = (hashQuery().get("who") ?? "").trim();
  const me = people.find((m) => m.name === want || (m.display_name || "") === want)
    ?? people.find((m) => m.admin && !m.disabled && m.role === "fde-lead")
    ?? people.find((m) => m.admin && !m.disabled)
    ?? people[0];
  const nameCount = new Map<string, number>();
  for (const m of people) {
    const key = m.display_name || m.name;
    nameCount.set(key, (nameCount.get(key) ?? 0) + 1);
  }
  const chipLabel = (m: Member): string =>
    (nameCount.get(m.display_name || m.name) ?? 0) > 1
      ? (m.display_name || m.name) + " · " + m.name
      : (m.display_name || m.name);
  const openWho = (hashQuery().get("open") ?? "").trim();
  if (openWho !== "") {
    const target = people.find((m) => m.name === openWho);
    if (target !== undefined) setTimeout(() => openMemberDrawer(d, v, target), 0);
  }
  return [
    me === undefined ? "" : section("我的卡片", "看板不做登录：这里只决定「以谁的视角看」，不改任何权限",
      el("div", { class: "ec-tags" }, ...people.map((m) =>
        el("a", { class: "ec-tag" + (m === me ? " on" : ""), href: "#/members?who=" + encodeURIComponent(m.name) },
          dash(chipLabel(m))))),
      el("div", { class: "entity-grid", style: "margin-top:12px" }, memberCard(d, v, me, "我的视角"))),
    section("成员管理", "一人一张卡；点开看身份、名下 Agent 是否在线、在做什么、以及当前任务",
      people.length === 0
        ? emptyNote("还没有成员（members/ 为空）。")
        : el("div", { class: "entity-grid" }, ...people.map((m) => memberCard(d, v, m)))),
    vac.length === 0 ? "" : section("还空着的岗位", "roles/ 里有岗位，但 members/ 里没有人",
      el("div", { class: "ec-tags" }, ...vac.map((r) =>
        el("span", { class: "ec-tag" }, dash(r.title) + " · ", el("span", { class: "mono" }, r.role))))),
    orphans.length === 0 ? "" : section("未落到成员身上的业务 Agent", "按同岗位 / 同业务域都挂不上 —— 不是错误，但需要有人认领",
      el("div", { class: "dlist" }, ...orphans.map((a) =>
        el("div", { class: "drow" },
          el("span", { class: "dot gold" }),
          el("span", { class: "drow-main" }, el("b", {}, dash(a.name || a.slug)), el("small", {}, dash(a.description))),
          el("span", { class: "chip mono" }, "域 " + dash(a.domain)))))),
    section("这一页怎么读", "关系是派生的，不是编的",
      el("div", { class: "grid" },
        card("人 → Agent", "两把尺子", [["同岗位", "agent.role = 成员岗位"], ["同业务域", "agent.domain ∈ 成员业务域"]]),
        card("当前任务", "三个来源", [["热层", "进行中的 by / to 命中姓名"], ["时间线", "case 的 by / to 命中姓名"], ["待批", "队列里的 who / to 命中"]]),
        card("没有的东西", "明确留白", [["上级", "org 还没有 reports_to 字段"], ["交付进度", "没有进度真相源"]]))),
  ];
}

// 项目 —— 读者：跟进的人 / 老板。谁在做什么、到几号、卡住找谁。
// 进度拆成两件事：周期时间消耗（可由原文日期推导）与交付进度（没有真相源）。
export function projectCard(d: Data, v: BoardView, p: ProjectRow): HTMLElement {
  const pct = periodState(p.period);
  const agents = agentsForDomain(v, p.domain);
  const recent = d.timeline?.wired ? d.timeline.cases.filter((c) => c.project === p.slug).length : -1;
  const node = el("article", {
    class: "entity-card project-card",
    role: "button", tabindex: "0", "aria-label": "打开项目 " + dash(p.name) + " 的详情",
  },
    el("header", { class: "ec-head" },
      el("span", { class: "avatar project" }, "项"),
      el("div", { class: "ec-id" },
        el("h3", {}, dash(p.name)),
        el("p", {}, domainLabel(v, p.domain), el("span", { class: "dot-sep" }, "·"), el("span", { class: "mono" }, dash(p.slug))))),
    el("div", { class: "ec-tags" },
      el("span", { class: "ec-tag" }, "负责人 " + dash(p.owner_label)),
      el("span", { class: "ec-tag" }, "周期 " + dash(p.period)),
      ...(p.charter === "" ? [] : [el("span", { class: "ec-tag" }, "立项书副本")])),
    pct
      ? el("div", { class: "meter " + pct.cls },
          el("div", { class: "meter-head" }, el("span", {}, "周期时间进度"), el("b", {}, String(Math.round(pct.pct)) + "%")),
          el("div", { class: "track" }, el("i", { style: "width:" + pct.pct.toFixed(1) + "%" })),
          el("p", { class: "meter-note" }, pct.note + " · 按周期起止推算，只表示时间消耗，不是交付进度。"))
      : el("p", { class: "meter-note" }, "周期是自由文本，解析不出起止日期 —— 原样显示，不画进度条。"),
    el("div", { class: "ec-foot" },
      el("span", {}, "交付进度 ", el("b", { class: "muted" }, "未提供")),
      el("span", {}, "Agent ", el("b", {}, String(agents.length))),
      el("span", {}, "记录 ", recent < 0 ? "—" : String(recent))));
  const open = (): void => openProjectDrawer(d, v, p);
  node.addEventListener("click", open);
  node.addEventListener("keydown", (e) => {
    if (e.key === "Enter" || e.key === " ") { e.preventDefault(); open(); }
  });
  return node;
}
export function renderProjects(d: Data): Kid[] {
  const v = d.board;
  if (!v) return [noBoard(d)];
  return [
    section("全部项目", "一张卡一个项目；进度拆成「周期时间消耗」与「交付进度」两件事",
      v.projects.length === 0
        ? emptyNote("项目表还是空的（projects.md 没有数据行）—— 由 agent 从立项书抽取后填。")
        : el("div", { class: "entity-grid" }, ...v.projects.map((p) => projectCard(d, v, p)))),
    section("进度的口径", "两个数字不能混",
      el("div", { class: "grid" },
        card("周期时间消耗", "可推导", [["来源", "projects.md 的 period 原文"], ["算法", "起止日期之间消耗的比例"], ["含义", "只说明时间过去了多少"]]),
        card("交付进度", "未提供", [["来源", "没有机器可读的进度字段"], ["处理", "不画百分比、不拿时间冒充"], ["缺口", "见 #36 / SPEC §13 Q15"]]))),
  ];
}
