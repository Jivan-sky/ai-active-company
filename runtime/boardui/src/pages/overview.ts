import { dash, el, section, table } from "../core/dom";
import { localTime, sinceText } from "../core/format";
import { icon } from "../core/icons";
import type { Data, Kid } from "../core/types";
import { PAGES } from "../router";
import { botRow, emptyNote, feedRow, healthState, heroKpi, timelineRecent } from "../ui/blocks";
import { bandDot } from "../ui/status";

// 总览 —— 读者：所有人 / 老板。要的是「30 秒看完有没有事」，不是名词表。
// 这一页只放「现在要不要人动手」的几类事实；没有数据源就写「未接入」，不画空图。
export function renderOverview(d: Data): Kid[] {
  const b = d.board;
  const i = d.issues;
  const pending = d.approvals && d.approvals.wired ? d.approvals.count : null;
  const bots = d.runtime && d.runtime.wired ? d.runtime.bots : [];
  const green = bots.filter((x) => x.state === "ok").length;
  const red = bots.filter((x) => x.state === "fail").length;
  const yellow = bots.filter((x) => x.state === "warn").length;
  const gray = bots.filter((x) => x.state === "unwired").length;
  const activeMembers = b ? b.members.filter((m) => !m.disabled).length : 0;
  const totalMembers = b ? b.members.length : 0;
  const state = healthState(i);
  let headline = "现在没有异常";
  let lead = "校验干净、待批队列为空 —— 但这也只说明真相源格式没问题；agent 是不是真的在跑，要看运行矩阵。";
  if (d.err !== "") {
    headline = "现在读不到投影";
    lead = d.err;
  } else if (i.error !== "") {
    headline = "现在读不到校验";
    lead = i.error;
  } else if (i.fatal.length > 0) {
    headline = "现在有 " + i.fatal.length + " 条红档";
    lead = "红档会拦住落盘。先修真相源，再看运行态 —— 看板不替你把红档藏起来。";
  } else if (pending !== null && pending > 0) {
    headline = "现在有 " + pending + " 件事等你点头";
    lead = "待批队列里有人等你给一句话。点头只留痕，不改真相源；该不该批仍然由人判断。";
  } else if (i.warn.length > 0) {
    headline = "现在有 " + i.warn.length + " 条非红档";
    lead = "非红档不拦落盘，但值得过目。完整清单在审计与校验页，不在这里堆成一面墙。";
  } else if (pending === null) {
    headline = "待批队列还没接上";
    lead = "「读不到」不等于「没人提」。运行矩阵和真相源校验仍然可看；队列接上后再补这一格。";
  }
  const orbValue = pending !== null && pending > 0 ? String(pending) : red > 0 ? String(red) : "OK";
  const orbLabel = pending !== null && pending > 0 ? "待批" : red > 0 ? "卡点" : "稳态";
  const recent = timelineRecent(d, 6);
  const assets = d.assets && d.assets.wired ? d.assets : null;
  const hot = d.hot && d.hot.wired ? d.hot : null;
  const files = assets ? assets.dirs.reduce((n, x) => n + x.files, 0) : null;
  const auditTotal = d.audit && d.audit.wired ? d.audit.total : null;
  const timelineTotal = d.timeline && d.timeline.wired ? d.timeline.total : null;

  // 状态环：四段全部来自运行态，不另算一套。一个 bot 都没接时画灰环，不画绿灯。
  const botTotal = bots.length;
  const deg = (n: number): number => (botTotal === 0 ? 0 : (n / botTotal) * 360);
  const d1 = deg(green);
  const d2 = d1 + deg(yellow);
  const d3 = d2 + deg(red);
  const ring = botTotal === 0
    ? "conic-gradient(rgba(150,153,147,.30) 0 360deg)"
    : "conic-gradient(#4ade9a 0 " + d1 + "deg, #f0b96a " + d1 + "deg " + d2 + "deg, #ff6b81 " + d2 + "deg " + d3 + "deg, rgba(140,143,137,.46) " + d3 + "deg 360deg)";
  const num = (n: number | null): string => (n === null ? "—" : String(n));

  const dockCard = (title: string, en: string, href: string, ...body: Kid[]): HTMLElement =>
    el("section", { class: "dock-card" },
      el("header", {},
        el("div", {}, el("p", { class: "eyebrow" }, en), el("h3", {}, title)),
        el("a", { class: "dock-more", href: href }, "全部", icon("arrow", 12))),
      el("div", { class: "dock-body" }, ...body));

  const loopStep = (no: string, label: string, count: number | null, href: string): HTMLElement =>
    el("a", { class: "loop-step", href: href },
      el("span", { class: "ls-no" }, no), el("b", {}, num(count)), el("span", { class: "ls-label" }, label));

  const dashPage = el("div", { class: "dash-stage" },
    el("canvas", { class: "field-canvas", "aria-hidden": "true" }),
    el("div", { class: "field-veil", "aria-hidden": "true" }),
    el("section", { class: "stage-hero" },
      el("div", { class: "hero-copy" },
        el("p", { class: "eyebrow" }, "LIVE PICTURE",
          el("span", {}, "真相源实时投影 · " + (b ? dash(b.company.name) : "还没读到投影"))),
        el("h2", {}, headline.split("现在")[0] ?? "",
          el("span", { class: "state " + state }, headline.replace(/^现在/, "").trim())),
        el("p", { class: "lead" }, lead),
        el("div", { class: "hero-actions" },
          el("a", { class: "primary-btn", href: "#/approvals" },
            icon("check", 15), "去待批", el("span", { class: "count" }, pending === null ? "—" : String(pending))),
          el("a", { class: "secondary-btn", href: "#/system?tab=runtime" }, icon("pulse", 14), "运行矩阵"),
          el("a", { class: "secondary-btn", href: "#/graph" }, icon("graph", 14), "关系图谱")),
        el("div", { class: "hero-kpis" },
          heroKpi("业务域", b ? String(b.domains.length) : "—", "DOMAINS"),
          heroKpi("项目", b ? String(b.projects.length) : "—", "PROJECTS"),
          heroKpi("成员", b ? activeMembers + "/" + totalMembers : "—", "ACTIVE"),
          heroKpi("bot 绿", d.runtime && d.runtime.wired ? green + "/" + botTotal : "—", "LIVE"))),
      el("div", { class: "hero-orbit", "aria-label": "运行态状态环" },
        el("div", { class: "orbit-shell" },
          el("div", { class: "orbit-ring", style: "background:" + ring }),
          el("div", { class: "orbit-core" },
            el("b", {}, orbValue), el("small", {}, orbLabel),
            el("span", {}, b ? dash(b.company.platform) : "WAITING"))),
        el("div", { class: "orbit-legend" },
          el("span", {}, el("i", { class: "lg ok" }), "绿 " + green),
          el("span", {}, el("i", { class: "lg warn" }), "黄 " + yellow),
          el("span", {}, el("i", { class: "lg bad" }), "红 " + red),
          el("span", {}, el("i", { class: "lg gray" }), "灰 " + gray)))),
    el("div", { class: "stage-dock" },
      dockCard("待批队列", "APPROVALS", "#/approvals",
        pending === null
          ? emptyNote("队列没接上 —— 读不到不等于没人提。")
          : d.approvals && d.approvals.pending.length > 0
            ? el("div", { class: "feed" }, ...d.approvals.pending.slice(0, 3).map((p) =>
                feedRow(bandDot(p.band ?? ""), p.title, dash(p.who) + " → " + dash(p.to), sinceText(p.enqueued), "#/approvals")))
            : emptyNote("队列是空的：现在没有等点头的提案。")),
      dockCard("运行矩阵", "RUNTIME", "#/system?tab=runtime",
        d.runtime === null
          ? emptyNote("运行态没接入。")
          : !d.runtime.wired
            ? emptyNote("运行态没接上：" + dash(d.runtime.gateway_why ?? d.runtime.error))
            : bots.length === 0
              ? emptyNote("config 里一个 project 都没有。")
              : el("div", { class: "bot-grid" }, ...bots.slice(0, 4).map(botRow))),
      dockCard("最近活动", "ACTIVITY", "#/timeline",
        recent.length === 0
          ? emptyNote("时间线是空的 —— 「还没开始记」是个真实状态。")
          : el("div", { class: "feed" }, ...recent.slice(0, 4).map((e) =>
              feedRow(bandDot(e.band), e.title, dash(e.kind) + " · " + dash(e.by) + " → " + dash(e.to), localTime(e.at), "#/timeline"))))),
    el("section", { class: "loop" },
      el("div", { class: "loop-head" },
        el("p", { class: "eyebrow" }, "OPERATING LOOP"),
        el("span", {}, "原料进来 → 判断出去 → 结果留下")),
      el("div", { class: "loop-track" },
        loopStep("01", "原料", files, "#/assets"),
        el("i", { class: "loop-arrow" }),
        loopStep("02", "热层", hot ? hot.total : null, "#/system?tab=hot"),
        el("i", { class: "loop-arrow" }),
        loopStep("03", "待批", pending, "#/approvals"),
        el("i", { class: "loop-arrow" }),
        loopStep("04", "审计", auditTotal, "#/audit"),
        el("i", { class: "loop-arrow" }),
        loopStep("05", "沉淀", timelineTotal, "#/timeline"))),
    ...(notWiredIndex() === "" ? [] : [notWiredIndex()]));
  return [dashPage];
}
export function linkTo(id: string): Kid {
  const p = PAGES.find((x) => x.id === id);
  return el("a", { href: "#/" + id }, p ? p.label : id);
}

// 从 PAGES 现算「哪几页还没接数据源」—— 手抄一份清单，迟早跟导航上说的不一样
// （以前这里写着「两块还没接」，而导航早把那两页标成「已接入」了）。
export function notWiredIndex(): Kid {
  const left = PAGES.filter((p) => !p.wired);
  if (left.length === 0) {
    return section("没有还没接数据源的页面", "导航上的「已接入 / 未接入」就是从这张表现算的",
      emptyNote("全部 " + PAGES.length + " 页都接上了。看板照样能显示「还没开始」这个真实状态，但不该再留白页。"));
  }
  const rows: Kid[][] = left.map((p) => [linkTo(p.id), p.question]);
  return section(left.length + " 页还没接数据源", "如实写明缺什么 —— 不编数据填界面",
    table(["页面", "将回答的问题"], rows));
}
