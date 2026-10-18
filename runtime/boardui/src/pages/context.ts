import type { Attrs, ContextView, CtxPack, CtxTask, Data, Kid } from "../core/types";
import { banner, card, dash, el, section, table } from "../core/dom";
import { localTime } from "../core/format";
import { handOff, statusCell } from "./timeline";
import { cannot, emptyNote, ptr, sourceCell } from "../ui/blocks";
import { bandDot, bandLegend, dotLine } from "../ui/status";

// ---------- 上下文页：一个 agent 的工作上下文 ----------
//
// 读者：FDE / 想看清「这个 agent 眼里的世界」的人。
//
// 这一页**不另立一套字段**：后端 /api/context 回的 pack 就是 internal/ctxpack 那两份结构，
// 也就是 harness 走 MCP 要的那一份、`anc context pack` 打的那一份 —— 同一份上下文，换一个摆法。
//
// 选中谁进 URL（`#/context?who=alice&domain=trade`）：刷新、把链接发给别人、前进后退，
// 都还停在那个人 / 那一域上。这一页**只读**：一个字都不写回真相源。

export function ctxHref(who: string, domain = ""): string {
  const p = new URLSearchParams({ who });
  if (domain !== "") p.set("domain", domain);
  return "#/context?" + p.toString();
}
export function ctxChip(label: string, href: string, on: boolean, mono = false): HTMLElement {
  const attrs: Attrs = { class: mono ? "chip mono" : "chip", href };
  if (on) attrs["aria-current"] = "true";
  return el("a", attrs, label);
}

export function ctxWhoSection(v: ContextView): Kid {
  if (v.roster.length === 0) {
    return section("可以看谁", "名单来自真相源（members/ 与 agents.md）",
      emptyNote("真相源里没有成员，也没有业务 agent。"));
  }
  const row = el("div", { class: "legend" });
  for (const e of v.roster) {
    const name = (e.name ?? "").trim();
    const label = name !== "" && name !== e.ref ? name + " · " + e.ref : dash(e.ref);
    row.append(ctxChip(label, ctxHref(e.ref), e.ref === v.who, e.kind === "agent"));
  }
  const out: Kid[] = [section("可以看谁",
    "名单来自真相源：成员 bot 与业务 agent 接在同一张口上 —— 「这是谁」也是真相源说的，不许自报",
    row)];
  // 范围：不写 domain = 它负责的全部域；写一个 = 只要那一块（同 CLI 的 --domain）。
  const p = v.pack;
  if (p && !p.refused && p.who) {
    const scopes = el("div", { class: "legend" },
      ctxChip("全部域", ctxHref(p.who), v.domain === ""));
    for (const d of p.domains ?? []) {
      scopes.append(ctxChip(d.name + " · " + d.slug, ctxHref(p.who, d.slug), v.domain === d.slug, true));
    }
    out.push(section("看多大范围", "要单独一域时不给别域目录、也不附通用报缺",
      p.domains && p.domains.length > 0 ? scopes : emptyNote("这个人名下还没有域，所以没有范围可选。")));
  }
  return el("div", {}, ...out);
}

export function ctxIdentitySection(p: CtxPack): Kid {
  const id = p.identity;
  if (!id) {
    return section("身份", "这一份里没有身份那一块", emptyNote("——"));
  }
  const rows: Array<[string, Kid]> = [
    ["类型", id.kind === "agent" ? "业务 agent · 代岗位" : "成员 bot · 代人"],
    ["编号", el("span", { class: "mono" }, dash(id.ref))],
    ["显示名", dash(id.name)],
    ["岗位", dash(id.role)],
    ["归属", id.domain ? dash(id.domain_name) + "（" + id.domain + "）" : cannot("没划域 —— 没有域就没有数据视野")],
    ["铭文", id.inscription ? dash(id.inscription) : cannot("没有铭文（成员 bot 不在这张表上）")],
  ];
  if (id.evidence) {
    rows.push(["铭文出处", el("span", { class: "mono" }, id.evidence.file + ":" + id.evidence.line)]);
  }
  return section("身份", "三格全部来自真相源：归属取自域表，铭文取自 agents.md",
    el("div", { class: "grid" }, card(dash(id.name), id.kind, rows)));
}

export function ctxDomainsSection(p: CtxPack): Kid {
  const rows: Kid[][] = (p.domains ?? []).map((d) => [
    el("span", { class: "mono nw" }, d.slug), dash(d.name), dash(d.what),
    el("span", { class: "mono nw" }, dash(d.data)), dash(d.who),
    dash(d.terms), dash(d.sources), ptr(d.evidence),
  ]);
  return section("业务域（你负责的）", "全行：域是什么 / 数据在哪 / 找谁 / 指回真相源的那一行",
    rows.length === 0
      ? emptyNote(p.note !== "" && p.note !== undefined ? p.note : "没有域 —— 公司还没给你划域。")
      : table(["slug", "名称", "是什么", "数据在哪", "找谁", "术语口径", "来源", "证据"], rows));
}

export function ctxDirectorySection(p: CtxPack): Kid {
  const rows: Kid[][] = (p.directory ?? []).map((d) => [
    el("span", { class: "mono nw" }, d.slug), dash(d.name), dash(d.what), dash(d.who),
  ]);
  return section("别域（只给「找谁」）", "跨域按「找谁」接头 —— 走人，不走近道（数据默认不通）",
    rows.length === 0 ? emptyNote("没有别域。") : table(["slug", "名称", "是什么", "找谁"], rows));
}

export function ctxSkillsSection(p: CtxPack): Kid {
  const rows: Kid[][] = (p.skills ?? []).map((s) => [
    el("span", { class: "mono nw" }, s.name),
    el("span", { class: "mono nw" }, s.where),
    s.missing ? cannot("正文还没落") : "在",
  ]);
  return section("技能从哪来", "清单在 roles/<岗位> 的 skills:，正文在 skills/<名>/SKILL.md",
    rows.length === 0
      ? emptyNote("这个岗位在 roles/ 里没声明 skills —— 不是「没有技能」，是「没登记」。")
      : table(["技能", "正文指针", "正文"], rows));
}

export function ctxProjectsSection(p: CtxPack): Kid {
  const rows: Kid[][] = (p.projects ?? []).map((x) => [
    el("span", { class: "mono nw" }, x.slug), dash(x.name), dash(x.owner),
    dash(x.period), sourceCell(x.source ?? ""), ptr(x.evidence),
  ]);
  return section("在跟的项目", "只列你可见域上的；period / source 原样搬（它们本身是指针，ANC 不解读）",
    rows.length === 0
      ? emptyNote("你可见的域上还没挂项目。")
      : table(["slug", "项目", "负责人", "周期", "真源", "证据"], rows));
}

// 「派给我的待办 / 我在做的」—— 反向下发的那一步落在一个人眼里长什么样。
//
// 为什么是这两摞而不是一张任务表：ANC **不往 agent 的会话里插话**（插了就是打断），
// 派活拆成两件各自可替换的事 —— 决定派给谁（ANC 的活，落在热层）、让对方开始动（对方 pull）。
// 这一块就是那个 pull 的可见面：会话一开始要一次上下文，「派给我的」就在里面。
// 看板这一页只摆出来看：不认领、不改状态（要动是 `anc hot` 那边的事）。
export function ctxTasksSection(
  list: CtxTask[] | undefined, bands: Record<string, string> | undefined,
  title: string, hint: string, emptyWhat: string,
): Kid {
  const rows: Kid[][] = (list ?? []).map((t) => [
    dotLine(bandDot(bands?.[t.id] ?? "unknown"), statusCell(t.status)),
    el("div", {}, el("span", { class: "mono" }, dash(t.id)), el("div", {}, dash(t.title))),
    handOff(t.by, t.to),
    dash([t.domain, t.project].filter((x) => (x ?? "").trim() !== "").join(" · ")),
    t.as_of ? localTime(t.as_of) : cannot("没写 as_of"),
    (t.note ?? "").trim() === "" ? "" : el("p", { class: "empty" }, dash(t.note)),
  ]);
  return section(title, hint,
    rows.length === 0
      ? emptyNote(emptyWhat)
      : table(["档 / status", "编号 · 这一件", "谁 → 交给谁", "业务域 · 项目", "当前值是什么时候的", "备注"], rows));
}

export function ctxGapsSection(p: CtxPack): Kid {
  const gaps = p.gaps ?? [];
  return section("这份上下文答不了什么", "报缺，不编 —— 读的人据此知道该去问谁，而不是以为这就是全部",
    gaps.length === 0
      ? emptyNote("没报缺：要么都答上了，要么这次只问了一域（那就不附通用报缺）。")
      : el("ul", {}, ...gaps.map((g) => el("li", {}, g))));
}

export function renderContext(d: Data): Kid[] {
  const v = d.context;
  if (!v) {
    return [banner("bad", "读不到工作上下文", [
      el("span", {}, d.contextErr === "" ? "服务没回 anc.context/v1" : d.contextErr)])];
  }
  const out: Kid[] = [];
  if ((v.error ?? "") !== "") {
    out.push(banner("bad", "真相源读不动", [el("span", {}, dash(v.error))]));
  }
  out.push(ctxWhoSection(v));
  const p = v.pack;
  if (!p) {
    out.push(section("挑一个", "选中的人进 URL：刷新、分享、前进后退都还停在同一个人身上",
      emptyNote("还没有选。上面挑一个 —— 这一页会摆出它眼里的世界。")));
    return out;
  }
  if (p.refused) {
    out.push(banner("warn", "没答出来", [
      el("span", {}, dash(p.refused.reason)),
      p.refused.ask ? el("span", {}, "该找谁：" + p.refused.ask) : "",
    ]));
    out.push(section("这一页不做什么", "拒了就是拒了 —— 不给半份数据",
      emptyNote("被拒的那一次照样是一次「想行使」：它记在审计流水里，不记在这一页上。")));
    return out;
  }
  out.push(ctxIdentitySection(p));
  // 反向下发的那一半：agent 自己 pull 到的就是这两摞。
  // 热层没接上时**不摆空列表** —— 那会把「看不到」读成「没有」，正是这一页最该防的假绿。
  if (!v.tasks_wired) {
    out.push(banner("warn", "派给我的待办：这一段答不了", [
      el("span", {}, dash(v.tasks_why)),
      el("br"),
      el("span", {}, "接不上不代表「没人派给你」—— 这一页宁可不显示，也不拿一份空清单冒充它。"),
    ]));
  } else {
    out.push(ctxTasksSection(p.todo, v.task_bands, "派给我的待办",
      "热层的 to = 你。这一页只读：派谁、派什么在 ANC 那边，改状态用 anc hot",
      "没人派给你 —— 热层接上了，里面确实没有 to 是你的。"));
    out.push(ctxTasksSection(p.doing, v.task_bands, "我在做的",
      "热层的 by = 你。状态原样摆（done / running / blocked / failed，认不出的照收）",
      "你手上没有在做的 —— 热层接上了，里面确实没有 by 是你的。"));
    if ((p.todo ?? []).length + (p.doing ?? []).length > 0) out.push(bandLegend());
  }
  out.push(ctxDomainsSection(p));
  out.push(ctxDirectorySection(p));
  out.push(ctxSkillsSection(p));
  out.push(ctxProjectsSection(p));
  out.push(ctxGapsSection(p));
  out.push(section("这一份是什么时候算的", "投影是纯函数产物：同一份真相源，谁算都一样",
    el("p", { class: "empty" },
      "算于 " + (p.at ? localTime(p.at) : "—") +
      (p.scope ? "，范围只到 " + p.scope + " 这一域" : "，范围是它负责的全部域") + "。",
      "这一页只摆出来看，不落盘；要一份冻住的副本用 ", el("span", { class: "mono" }, "anc context freeze"),
      "（落 vault 的 context/，进 git）。")));
  return out;
}
