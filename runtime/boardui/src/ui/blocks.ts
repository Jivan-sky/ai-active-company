import type { BoardView, CtxEvidence, Data, Finding, Issues, Kid, RuntimeBot, TimelineEntryView } from "../core/types";
import { banner, el, section } from "../core/dom";
import { runState } from "./status";

export const emptyNote = (s: string): HTMLElement => el("p", { class: "empty" }, s);

// 「本页判断不了 / 不加戏」这类否定断言，单独着色 —— 它是诚实的一部分，不该藏起来。
export const cannot = (s: string): HTMLElement => el("span", { class: "cannot" }, s);

// 真源指针只把 http(s) 当链接渲染；别的写法（含 javascript: 之类）一律当纯文字。
export function sourceCell(v: string): Kid {
  const s = (v ?? "").trim();
  if (s === "") return "—";
  if (!/^https?:\/\//i.test(s)) return el("span", { class: "mono" }, s);
  return el("a", { href: s, target: "_blank", rel: "noreferrer noopener" }, s);
}

// 证据指针：指回真相源的那一行（vault 相对路径 + 行号）—— 照着能回去核，不是本机路径。
export const ptr = (e: CtxEvidence | undefined): Kid =>
  e && (e.file ?? "") !== "" ? el("span", { class: "mono nw" }, e.file + ":" + e.line) : "—";

export function findingLine(f: Finding): Kid {
  const where = (f.where ?? "").trim();
  return el("span", {},
    el("span", { class: "mono" }, f.rule), " ",
    where === "" ? f.msg : where + "：" + f.msg);
}

export function statusDot(i: Issues): string {
  if (i.error !== "" || i.fatal.length > 0) return "dot bad";
  if (i.warn.length > 0) return "dot warn";
  return "dot ok";
}

export function statusText(i: Issues): string {
  if (i.error !== "") return "真相源读不了";
  if (i.fatal.length > 0) return "红档 " + i.fatal.length + " 条";
  if (i.warn.length > 0) return "非红档 " + i.warn.length + " 条";
  return "校验干净";
}
export function noBoard(d: Data): Kid {
  return banner("bad", "读不到投影", [
    el("span", {}, d.err === "" ? "服务返回的不是 anc.board 投影" : d.err),
    el("br"),
    el("span", { class: "mono" }, "组织 / 项目 两页都吃这份投影；先修它。"),
  ]);
}

export function alertBanners(i: Issues): Kid[] {
  const out: Kid[] = [];
  if (i.fatal.length > 0) {
    out.push(banner("bad", "红档 " + i.fatal.length + " 条 —— 拦住落盘，先修它", i.fatal.map(findingLine)));
  } else if (i.error !== "") {
    out.push(banner("bad", "校验状态读不了", [el("span", {}, i.error)]));
  }
  if (i.warn.length > 0) {
    out.push(banner("warn", "非红档 " + i.warn.length + " 条（不拦，但请过目）", i.warn.map(findingLine)));
  }
  return out;
}
// 第 4 个参数是可选的状态灯：数字旁的形状和下面表里读到的**一模一样** —— 图例跟实际用的是同一套，
// 不另画一套「好看的图例」（形状与含义见 index.html 的 .dot）。
export function statCard(n: string, k: string, sub: string, dot?: string): HTMLElement {
  return el("div", { class: "card stat" },
    el("div", { class: "n" }, n),
    el("div", { class: "k" },
      dot ? el("span", { class: "st" }, el("span", { class: dot }), el("span", {}, k)) : k),
    el("div", { class: "sub" }, sub));
}

export function statSection(v: BoardView): Kid {
  const active = v.members.filter((m) => !m.disabled).length;
  return section("规模", "全部由真相源现算，不缓存、不猜",
    el("div", { class: "figs" },
      statCard(String(v.domains.length), "业务域", "一块业务一行"),
      statCard(String(v.projects.length), "项目", "在做的事"),
      statCard(String(v.roles.length), "角色", "岗位"),
      statCard(active + "/" + v.members.length, "成员", "启用 / 全部"),
      statCard(String(v.routing.length), "路由目录", "顶层数据目录")));
}
export function dcard(title: string, badge: string, ...body: Kid[]): HTMLElement {
  const h = el("h3", {}, title);
  if (badge !== "") h.append(el("span", { class: "chip mono" }, badge));
  return el("section", { class: "dcard" }, h, ...body);
}

export function kv(rows: Array<[string, Kid]>): HTMLElement {
  const dl = el("dl");
  for (const [k, v] of rows) dl.append(el("dt", {}, k), el("dd", {}, v));
  return dl;
}
export function healthState(i: Issues): "ok" | "warn" | "bad" {
  if (i.error !== "" || i.fatal.length > 0) return "bad";
  if (i.warn.length > 0) return "warn";
  return "ok";
}

export function healthWord(i: Issues): string {
  if (i.error !== "") return "读不到真相源";
  if (i.fatal.length > 0) return "有红档";
  if (i.warn.length > 0) return "需过目";
  return "校验干净";
}

export function heroKpi(label: string, value: string, sub: string): HTMLElement {
  return el("div", { class: "kpi" },
    el("span", {}, label),
    el("strong", {}, value, sub === "" ? "" : el("small", {}, " " + sub)));
}

export function healthRow(dot: string, count: string, label: string, why: string): HTMLElement {
  return el("div", { class: "health-row" },
    el("span", { class: dot }), el("b", {}, count), el("span", { class: "lab" }, label),
    el("span", { class: "why" }, why));
}

export function feedRow(dot: string, title: string, sub: string, time: string, href: string): HTMLElement {
  return el("a", { class: "feed-row", href: href },
    el("span", { class: dot }),
    el("span", { class: "ft" }, el("b", {}, title), el("small", {}, sub)),
    el("span", { class: "tm" }, time));
}

export function botRow(b: RuntimeBot): HTMLElement {
  const st = runState(b.state);
  return el("div", { class: "bot-row", title: b.why },
    el("span", { class: st.dot }), el("span", { class: "bn" }, b.project),
    el("span", { class: "bw" }, st.label.split(" · ")[0] ?? ""));
}

export function bentoCard(title: string, hint: string, span: string, more: [string, string] | null, ...body: Kid[]): HTMLElement {
  const head = el("div", { class: "card-head" },
    el("div", {}, el("h3", {}, title), el("div", { class: "hint" }, hint)),
    more === null ? "" : el("a", { class: "more", href: more[0] }, more[1] + "  ›"));
  return el("section", { class: "glass bento-card " + span }, head, el("div", { class: "grow" }, ...body));
}

export function timelineRecent(d: Data, n: number): TimelineEntryView[] {
  const v = d.timeline;
  if (!v || !v.wired) return [];
  const rows: TimelineEntryView[] = [];
  for (const c of v.cases) for (const e of c.recent) rows.push(e);
  rows.sort((a, b) => (a.at < b.at ? 1 : a.at > b.at ? -1 : 0));
  return rows.slice(0, n);
}
