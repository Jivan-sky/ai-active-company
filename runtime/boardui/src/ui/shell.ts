import type { Data, Kid, Page } from "../core/types";
import { banner, chip, dash, el, root } from "../core/dom";
import { localTime } from "../core/format";
import { icon, iconBox } from "../core/icons";
import { currentId } from "../core/query";
import { reload } from "../core/refresh";
import { cache } from "../core/state";
import { paintSediment } from "../pages/sediment";
import { PAGES, pageFor } from "../router";
import { healthState, statusDot, statusText } from "./blocks";
import { closeDrawer, mountDrawer } from "./drawer";
import { mountField, stopField } from "./field";
import { mountOrgGraph, stopOrgGraph } from "./orggraph";
import { closePalette, mountPalette, openPalette } from "./palette";
import { showToast } from "./toast";

// ---------- 主题 ----------
// 亮/暗只是同一套语义色的两组取值。选择存在 localStorage；没选过默认浅色。
// 切换后重新挂载一次外壳：Canvas 的颜色不是 CSS 能改的，必须按新主题重画。
export type Theme = "light" | "dark";
export function currentTheme(): Theme {
  return document.documentElement.dataset.theme === "light" ? "light" : "dark";
}
export function applyTheme(t: Theme): void {
  document.documentElement.dataset.theme = t;
  try { localStorage.setItem("anc.theme", t); } catch { /* 隐私模式不记 */ }
  shell();
}
// ---------- 外壳与路由 ----------

export function refreshButton(): HTMLElement {
  const b = el("button", { class: "refresh" }, "刷新");
  b.addEventListener("click", () => reload(true));
  return b;
}

export function topline(d: Data | null, page: Page): HTMLElement {
  const i = d ? d.issues : null;
  const c = d && d.board ? d.board.company : null;
  const meta = el("div", { class: "meta" },
    i ? el("span", { class: "chip" }, el("span", { class: statusDot(i) }), statusText(i)) : "",
    d && d.board ? chip("生成 " + localTime(d.board.generated_at)) : "",
    d && d.board ? chip("schema " + d.board.schema, true) : "",
    refreshButton());
  return el("div", { class: "topline" },
    el("div", { class: "brandrow" },
      el("span", { class: "glyph" }),
      el("span", { class: "nm" }, "ANC"),
      el("span", { class: "rule" }),
      el("span", { class: "co" }, c ? dash(c.name) : "还没读到投影",
        c ? el("span", { class: "sub" }, dash(c.platform) + " · " + dash(c.language) + " · " + dash(c.timezone)) : "")),
    el("div", { class: "spacer" }),
    meta);
}

// 页头：整页唯一的大字。它只问那一句话，答案在下面几节里。
export function hero(page: Page): HTMLElement {
  return el("section", { class: "hero" }, el("h1", {}, page.label), el("p", { class: "ask" }, page.question));
}

export function rail(page: Page): HTMLElement {
  // 分组：组的标题是问题本身，页名才是答案。PAGES 的顺序就是分组顺序 —— 加页只要写对 group。
  const tabs = el("nav", { class: "tabs" });
  let group = "";
  let items = el("div", { class: "gitems" });
  for (const p of PAGES) {
    if (p.group !== group) {
      if (group !== "") tabs.append(el("div", { class: "navgroup" }, el("div", { class: "glabel" }, group), items));
      group = p.group;
      items = el("div", { class: "gitems" });
    }
    items.append(el("a", { class: p.id === page.id ? "tab active" : "tab", href: "#/" + p.id },
      el("span", { class: "lab" }, p.label),
      el("span", { class: p.wired ? "res on" : "res off" }, p.wired ? "已接入" : "未接入")));
  }
  if (group !== "") tabs.append(el("div", { class: "navgroup" }, el("div", { class: "glabel" }, group), items));
  return el("aside", { class: "rail" }, tabs);
}

export function footer(): HTMLElement {
  return el("footer", {},
    el("div", {}, el("strong", {}, "几乎只读"),
      "：数据是真相源（vault）的实时投影，点「刷新」重取；写只有两处 ——「待批」页的点头",
      "（留痕：timeline + audit，不动真相源）与热层终态条目的归档（先落 timeline、再清热层）。"),
    el("div", {}, el("strong", {}, "校验发现"), " 与 ", el("span", { class: "mono" }, "anc org check"),
      " 同源：红档拦住落盘，非红档只回显；完整清单在「总览」。"),
    el("div", {}, el("strong", {}, "分页口径"),
      "：一页回答一个问题。还没接数据源的页如实写明缺什么、依赖哪个议题，不编数据。"));
}

// 上一屏是哪一页 —— 用它决定要不要放进场动效。**刷新不算换页**（刷新时不动）：
// 看板是天天看的工具，每次重取都全屏重播一遍进场，很快就变成噪音。
export let lastPageId = "";

export function legacyShell(): void {
  if (!root) return;
  const page = pageFor(currentId());
  const d = cache;
  const entering = d !== null && page.id !== lastPageId;
  const col = el("div", { class: "col" });
  col.append(hero(page));
  if (!d) {
    col.append(el("div", { class: "glass pad" },
      el("div", { class: "loading", style: "font-size:12.5px;margin-bottom:14px" }, "正在读取真相源…"),
      el("div", { class: "sk", style: "width:58%" }),
      el("div", { class: "sk", style: "width:82%" }),
      el("div", { class: "sk", style: "width:44%" })));
  } else {
    for (const kid of page.view(d)) col.append(kid);
    col.append(footer());
    lastPageId = page.id;
  }
  // 进场：换页时每节各进一次，错峰 ≤ 0.3s（动效预算：只进，不循环）。
  // 翻页不触发 —— 那还是同一页，动起来只会让人以为「换了内容」。
  if (entering) {
    const h1 = col.querySelector("header.top h1");
    if (h1) h1.classList.add("sweep");
    Array.from(col.querySelectorAll("section")).forEach((sec, i) => {
      sec.classList.add("reveal");
      (sec as HTMLElement).style.animationDelay = String(Math.min(i, 5) * 0.06) + "s";
    });
  }
  document.title = page.label + " · " + (d && d.board ? dash(d.board.company.name) : "ANC 看板");
  root.replaceChildren(el("div", { class: "wrap" },
    topline(d, page),
    el("div", { class: "shell" }, rail(page), col)));
  // 沉淀柱：进这一页时落一次（落定就停住）；重取数据后重画只是把料摆回那一层上，不再播一遍。
  if (col.querySelector(".column")) paintSediment(col, entering);
}

// ---------- 新外壳：侧栏 / 顶栏 / 页头 ----------
export let lastShellPage = "";

export function navBadge(p: Page, d: Data | null): Kid {
  if (!d) return "";
  if (p.id === "approvals" && d.approvals?.wired && d.approvals.count > 0) {
    return el("span", { class: "nav-badge hot" }, String(d.approvals.count));
  }
  if (p.id === "system" && d.runtime?.wired) {
    const n = d.runtime.bots.filter((b) => b.state === "fail").length;
    if (n > 0) return el("span", { class: "nav-badge hot" }, String(n));
  }
  if (p.id === "cases" && d.timeline?.wired) {
    const n = d.timeline.cases.filter((c) => c.band === "red").length;
    if (n > 0) return el("span", { class: "nav-badge hot" }, String(n));
  }
  if (p.id === "system" && d.hot?.wired && d.hot.stale > 0) {
    return el("span", { class: "nav-badge gold" }, String(d.hot.stale));
  }
  if (p.id === "audit" && d.audit) {
    const n = d.audit.bands.denied + d.audit.bands.failed;
    if (n > 0) return el("span", { class: "nav-badge hot" }, String(n));
  }
  return "";
}

export function sidebar(page: Page, d: Data | null): HTMLElement {
  const nav = el("nav", { class: "side-nav", "aria-label": "主导航" });
  let group = "";
  let box: HTMLElement | null = null;
  for (const p of PAGES) {
    if (p.group !== group) {
      group = p.group;
      box = el("div", { class: "nav-group" }, el("div", { class: "nav-label" }, group), el("div", { class: "nav-rail" }));
      nav.append(box);
    }
    const a = el("a", {
      class: p.id === page.id ? "nav-item active" : "nav-item",
      href: "#/" + p.id,
      title: p.question,
    }, iconBox(p.icon), el("span", { class: "lab" }, p.label), el("span", { class: "nav-hint" }, p.question), navBadge(p, d));
    if (p.id === page.id) a.setAttribute("aria-current", "page");
    box?.append(a);
  }
  const c = d?.board?.company;
  const status = d ? d.issues : null;
  const pending = d && d.approvals && d.approvals.wired ? d.approvals.count : null;
  const cases = d && d.timeline && d.timeline.wired ? d.timeline.cases.filter((x) => x.band === "red").length : null;
  const bots = d && d.runtime && d.runtime.wired ? d.runtime.bots : [];
  const botRed = d && d.runtime && d.runtime.wired ? bots.filter((x) => x.state === "fail").length : null;
  const meter = (label: string, n: number | null, cls: string): HTMLElement =>
    el("div", { class: "side-meter " + cls },
      el("span", {}, label), el("b", {}, n === null ? "—" : String(n)));
  const foot = el("div", { class: "side-foot" },
    el("div", { class: "side-health" },
      el("span", { class: status ? statusDot(status) : "dot" }),
      el("span", {}, status ? statusText(status) : "读取中"),
      el("span", { class: "micro" }, d?.board ? localTime(d.board.generated_at) : "")),
    el("div", { class: "side-meters" },
      meter("待批", pending, "gold"), meter("卡点", cases, "red"), meter("bot 红", botRed, "red")),
    el("div", { class: "micro env" }, d?.board ? d.board.schema : "vault · waiting"),
    el("div", { class: "foot-links" },
      el("a", { href: "#/overview" }, "总览"),
      el("a", { href: "#/timeline" }, "时间线"),
      el("span", {}, "只读观测")));
  const collapse = el("button", { class: "collapse-btn", type: "button", "aria-label": "收起或展开侧栏", title: "收起 / 展开侧栏" }, icon("chevronL", 14));
  collapse.addEventListener("click", () => {
    const app = document.querySelector(".app");
    app?.classList.toggle("rail-min");
    try { localStorage.setItem("anc.rail.min", app?.classList.contains("rail-min") ? "1" : "0"); } catch { /* 隐私模式不记 */ }
  });
  return el("aside", { class: "side" },
    el("div", { class: "side-brand" },
      el("span", { class: "brand-orb" }),
      el("span", { class: "bt" }, el("b", {}, "ANC"), el("small", {}, "AI-NATIVE COMPANY")),
      collapse),
    el("div", { class: "side-workspace" },
      el("span", { class: "ws-dot" }),
      el("span", { class: "ws-text" },
        el("b", {}, c ? dash(c.name) : "还没读到投影"),
        el("small", {}, c ? dash(c.platform) + " · " + dash(c.timezone) : "vault · waiting"))),
    nav, foot);
}

export function copyLink(): void {
  const url = location.href;
  if (navigator.clipboard && navigator.clipboard.writeText) {
    void navigator.clipboard.writeText(url).then(
      () => showToast("链接已复制"),
      () => showToast("复制失败，手动复制地址栏"),
    );
    return;
  }
  showToast("当前浏览器不支持剪贴板");
}

export function topbar(page: Page, d: Data | null): HTMLElement {
  const menu = el("button", { class: "icon-btn menu-btn", type: "button", "aria-label": "打开导航" }, icon("menu", 15));
  menu.addEventListener("click", () => document.querySelector(".app")?.classList.toggle("nav-open"));
  const cmd = el("button", { class: "cmd-btn", type: "button", "aria-label": "搜索与跳转" },
    icon("search", 14), el("span", { class: "label" }, "搜索页面、成员、项目、Agent…"), el("span", { class: "kbd" }, "Ctrl K"));
  cmd.addEventListener("click", openPalette);
  const refresh = el("button", { class: "icon-btn", type: "button", "aria-label": "刷新", title: "刷新" }, icon("refresh", 15));
  refresh.addEventListener("click", () => reload(true));
  const dark = currentTheme() === "dark";
  const themeBtn = el("button", { class: "icon-btn", type: "button", "aria-label": dark ? "切换到浅色" : "切换到深色", title: dark ? "切换到浅色" : "切换到深色" }, icon(dark ? "sun" : "moon", 15));
  themeBtn.addEventListener("click", () => applyTheme(dark ? "light" : "dark"));
  const copy = el("button", { class: "icon-btn", type: "button", "aria-label": "复制本页链接", title: "复制本页链接" }, icon("copy", 15));
  copy.addEventListener("click", copyLink);
  const i = d ? d.issues : null;
  const pill = el("span", { class: "status-pill " + (i ? healthState(i) : "warn") },
    el("span", { class: i ? statusDot(i) : "dot" }), i ? statusText(i) : "读取中");
  const live = d && d.board ? el("span", { class: "live-chip" }, el("i"), "LIVE") : "";
  return el("header", { class: "topbar" },
    menu,
    el("div", { class: "crumbs" },
      el("span", { class: "grp" }, page.group),
      el("span", { class: "sep" }, "/"),
      el("b", {}, page.label)),
    cmd,
    el("div", { class: "top-actions" },
      live,
      pill,
      d?.board ? el("span", { class: "meta-pill" }, "生成 " + localTime(d.board.generated_at)) : "",
      themeBtn, refresh, copy));
}

export function pageHead(page: Page): HTMLElement {
  const copy = el("button", { class: "ghost-btn", type: "button" }, icon("copy", 14), "复制链接");
  copy.addEventListener("click", copyLink);
  const idx = PAGES.findIndex((x) => x.id === page.id) + 1;
  const no = String(Math.max(idx, 1)).padStart(2, "0");
  return el("section", { class: "page-head" },
    el("div", { class: "ph-main" },
      el("p", { class: "eyebrow" },
        el("span", { class: "ph-no" }, no), page.group, el("span", { class: "ph-sep" }, "/"), el("span", {}, page.id)),
      el("h1", {}, page.label),
      el("p", { class: "ask" }, page.question),
      page.sub ? el("p", { class: "ph-sub" }, page.sub) : ""),
    el("div", { class: "page-actions" },
      page.id === "overview" ? "" : el("a", { class: "ghost-btn", href: "#/overview" }, icon("overview", 14), "总览"),
      copy));
}

export function pageFooter(): HTMLElement {
  return el("footer", { class: "note" },
    el("strong", {}, "几乎只读"), "：数据是真相源（vault）的实时投影，点刷新重取；写只有两处 ——",
    "「待批」页的点头（留痕：timeline + audit，不动真相源）与热层终态条目的归档（落 timeline、再清热层）。",
    el("br"),
    el("strong", {}, "校验发现"), " 与 ", el("span", { class: "mono" }, "anc org check"), " 同源；完整清单在「总览」和「审计」。");
}

export function loadingBlock(): HTMLElement {
  return el("div", { class: "glass pad" },
    el("div", { class: "loading", style: "font-size:12.5px;margin-bottom:14px" }, "正在读取真相源…"),
    el("div", { class: "sk", style: "width:58%" }),
    el("div", { class: "sk", style: "width:82%" }),
    el("div", { class: "sk", style: "width:44%" }));
}

// 一页的渲染出错，只该让这一页报错，不该把整个外壳留在「读取中」。
// 报出来的是页面 id 和原始错误 —— 不猜原因，也不把异常吞掉。
export function pageCrash(page: Page, err: unknown): HTMLElement {
  const msg = err instanceof Error ? err.message : String(err);
  return banner("bad", "这一页渲染失败", [
    el("span", { class: "mono" }, page.id),
    el("br"),
    el("span", {}, msg),
    el("br"),
    el("span", { class: "quiet" }, "其余页面不受影响；刷新可重取一次数据。"),
  ]);
}

export function shell(): void {
  if (!root) return;
  const page = pageFor(currentId());
  const d = cache;
  const entering = d !== null && page.id !== lastShellPage;
  stopField();
  stopOrgGraph();
  closeDrawer();
  closePalette();

  const col = el("div", { class: "col" });
  col.append(pageHead(page));
  if (!d) {
    col.append(loadingBlock());
  } else {
    try {
      for (const kid of page.view(d)) col.append(kid);
    } catch (err) {
      col.append(pageCrash(page, err));
    }
    col.append(pageFooter());
    lastShellPage = page.id;
  }
  if (entering) {
    Array.from(col.querySelectorAll("section")).forEach((sec, i) => {
      if (sec.classList.contains("page-head")) return;
      sec.classList.add("reveal");
      (sec as HTMLElement).style.animationDelay = String(Math.min(i, 6) * 0.055) + "s";
    });
    // 卡片再晚半拍落位：一层是页面分区，一层是分区里的实体。
    Array.from(col.querySelectorAll(".entity-card")).forEach((n, i) => {
      n.classList.add("rise");
      (n as HTMLElement).style.animationDelay = String(0.06 + Math.min(i, 10) * 0.045) + "s";
    });
  }
  document.title = page.label + " · " + (d?.board ? dash(d.board.company.name) : "ANC 看板");

  const app = el("div", { class: "app" },
    sidebar(page, d),
    el("div", { class: "side-scrim" }),
    el("div", { class: "main" },
      topbar(page, d),
      el("main", { class: "pagewrap" }, el("div", { class: "page page-" + page.id }, col)),
      el("footer", { class: "statusbar" },
        el("span", { class: "sb-item" }, el("i", { class: "sb-dot" }), d?.board ? dash(d.board.company.name) : "ANC 看板"),
        el("span", { class: "sb-item" }, "schema ", el("b", {}, d?.board ? d.board.schema : "—")),
        el("span", { class: "sb-item" }, d?.board ? "生成 " + localTime(d.board.generated_at) : "等待投影"),
        el("span", { class: "sb-grow" }),
        el("span", { class: "sb-item" }, "几乎只读 · 写口：待批 / 归档"),
        el("span", { class: "sb-item" }, "Ctrl K"))));
  try {
    if (localStorage.getItem("anc.rail.min") === "1") app.classList.add("rail-min");
  } catch { /* 隐私模式不读 */ }

  app.append(mountDrawer(), mountPalette());
  root.replaceChildren(app);
  app.querySelector(".side-scrim")?.addEventListener("click", () => app.classList.remove("nav-open"));

  if (page.id === "overview" && d !== null) {
    requestAnimationFrame(() => mountField(col, d));
  }
  if (page.id === "graph" && d !== null) {
    requestAnimationFrame(() => mountOrgGraph(col, d));
  }
}

