import type { Data, Kid } from "../core/types";
import { PER_PAGE, pager, sliced } from "../ui/pager";
import { banner, chip, dash, el, section, table } from "../core/dom";
import { localTime } from "../core/format";
import { hashQuery } from "../core/query";
import { cannot, emptyNote, statCard } from "../ui/blocks";
import { bandDot, bandLegend, bandWord, dotLine } from "../ui/status";

// 状态原词照排（不翻译、不吞）：认不出的词也得让人看见它长什么样。
export const statusCell = (status: string | undefined): Kid =>
  (status ?? "").trim() === "" ? cannot("没写 status") : el("span", { class: "mono" }, status ?? "");

// 「谁 → 交给谁」：两头都可能没写 —— 缺哪个就说缺哪个，不拿一个「—」糊过去。
// 这一列是这一页的重点：**下一手是谁的决定**。
export function handOff(by: string | undefined, to: string | undefined): Kid {
  const b = (by ?? "").trim();
  const t = (to ?? "").trim();
  if (b === "" && t === "") return cannot("没写谁在跟、也没说交给谁");
  return el("span", {},
    b === "" ? cannot("没写谁记的") : el("span", { class: "mono" }, b),
    el("span", {}, " → "),
    t === "" ? cannot("还没说交给谁") : el("strong", {}, t));
}

export function renderTimeline(d: Data): Kid[] {
  const v = d.timeline;
  if (!v) {
    return [banner("bad", "读不到时间线", [
      el("span", {}, d.timelineErr === "" ? "服务没回 anc.timeline/v1" : d.timelineErr),
    ])];
  }
  if (!v.wired) {
    return [banner("warn", "时间线还没接上", [
      el("span", {}, dash(v.error)),
      el("br"),
      el("span", { class: "mono" }, "anc timeline add <vault> -by <谁> -title <一句话>"),
    ])];
  }

  const out: Kid[] = [];
  if ((v.error ?? "") !== "") out.push(banner("bad", "这一页读不动", [el("span", {}, dash(v.error))]));
  out.push(banner("warn", "这些行是人 / agent 自己写下来的", [el("span", {}, dash(v.note))]));
  if (v.bad.length > 0) {
    out.push(banner("warn", "有 " + v.bad.length + " 行读不懂（逐行列出来，不静默跳过）",
      v.bad.map((b) => el("span", { class: "mono" }, b))));
  }

  out.push(section("三档",
    "只按 status 配色，形状跟档走：done=实心圆 / running=带光环的圆 / blocked·failed=菱形 / 认不出的词=空心虚线圈（原样留着，不吞）",
    el("div", { class: "figs" },
      statCard(String(v.counts.green), "绿 · 已完成", "status=done", "dot ok"),
      statCard(String(v.counts.yellow), "黄 · 运行中", "status=running", "dot warn"),
      statCard(String(v.counts.red), "红 · 卡点 / 失败", "blocked · failed", "dot bad"),
      statCard(String(v.counts.unknown), "灰 · 认不出的词",
        v.counts.unknown === 0 ? "没有，词表没跑偏" : "原样保留，不吞", "dot gray"),
      statCard(String(v.entries), "流水条数", "折叠成 " + v.total + " 个 case")),
    bandLegend()));

  const tl = sliced(v.cases, hashQuery());
  const rows: Kid[][] = tl.rows.map((c) => [
    dotLine(bandDot(c.band), statusCell(c.status)),
    el("div", {}, el("span", { class: "mono" }, dash(c.key)), el("div", {}, dash(c.title))),
    handOff(c.by, c.to),
    dash([c.domain, c.project].filter((x) => (x ?? "").trim() !== "").join(" · ")),
    String(c.entries),
    localTime(c.last_at),
  ]);
  out.push(section("每个 case 现在在哪一步",
    "取最后一条当当前态 —— 它写着「遇到了什么、走到了哪一步、这一步是谁的决定」"
      + (v.cases.length > PER_PAGE
        ? "（共 " + v.cases.length + " 个 · 这一屏第 " + tl.cur + " / " + tl.pages + " 页）" : ""),
    tl.rows.length === 0
      ? emptyNote("时间线还是空的（`timeline/` 里一行都没有）。空不是错 —— 「还没开始记」是个真实状态。")
      : el("div", {},
          table(["档 / status", "case", "谁 → 交给谁", "业务域 · 项目", "几条", "最近活动"], rows),
          pager(v.cases.length, tl.cur, "timeline"))));

  const steps: Kid[] = tl.rows.map((c) => {
    const head: Kid[] = [
      el("span", { class: bandDot(c.band) }),
      el("span", { class: "mono" }, dash(c.key)), " ",
      el("strong", {}, dash(c.title)), " ",
      chip(bandWord(c.band) + " · " + ((c.status ?? "").trim() === "" ? "没写 status" : c.status)),
      chip("共 " + c.entries + " 条", true),
    ];
    const inner: Kid[][] = c.recent.map((e) => [
      dotLine(bandDot(e.band), statusCell(e.status)),
      localTime(e.at),
      el("div", {},
        (e.kind ?? "").trim() === "" ? cannot("没写 kind") : el("span", { class: "chip mono" }, (e.kind ?? "").trim()),
        el("div", {}, dash(e.title)),
        (e.detail ?? "").trim() === "" ? "" : el("p", { class: "empty" }, dash(e.detail)),
        (e.refs ?? []).length === 0 ? "" : el("p", { class: "empty mono" }, (e.refs ?? []).join(" · "))),
      handOff(e.by, e.to),
    ]);
    return el("details", {}, el("summary", {}, el("span", { class: "st" }, ...head)),
      table(["档 / status", "时间", "这一步（kind + 一句话）", "谁 → 交给谁"], inner));
  });
  if (steps.length > 0) {
    out.push(section("每一步都留着",
      "每个 case 带最近几步（最新在上）；完整流水在 `timeline/*.jsonl` 里 —— 一行都没删"
        + (v.cases.length > PER_PAGE ? "（跟上面那张表同一页）" : ""),
      ...steps));
  }

  if (v.total > v.cases.length) {
    out.push(section("还有更早的",
      "按「最近动过的排前面」，这一页只铺了 " + v.cases.length + " 个（共 " + v.total + " 个）",
      el("p", { class: "empty" },
        el("span", {}, "往后翻："), el("span", { class: "mono" }, "anc timeline list <vault> --before " + dash(v.next_before)),
        el("span", {}, "；按档筛："), el("span", { class: "mono" }, "anc timeline list <vault> --band red"))));
  }

  out.push(section("这一页答不了什么", "答不了的照写 —— 这一页是「卡在哪、谁在跟」，不是审计",
    v.missing.length === 0
      ? emptyNote("后端没列缺。")
      : el("ul", {}, ...v.missing.map((m) => el("li", {}, m)))));
  return out;
}
