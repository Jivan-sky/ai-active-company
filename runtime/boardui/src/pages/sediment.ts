import type { AuditView, Data, Kid } from "../core/types";
import { el, section } from "../core/dom";
import { durText } from "../core/format";

// ---------- 沉淀柱（口径图） ----------
//
// 「沉淀」页那根柱**不是统计**，它画的是四层的分工：
//   料落进来 → 重复与噪声在「丢掉」那一层当场散掉（所以那一层不增厚）
//            → 还在发生的事住「热层」（带保鲜期，过期自己碎掉，所以它便宜）
//            → 达标的落「库」变成经验（落下的就是资产，永久）
//            → 里面夹的明文，在落库之前被抹成指针（原文件改名，指针指向真源）
//
// 每层右边写的是**真数**：能对上的才写数，对不上的如实写「未接入」——
// 不拿动画冒充数据。同一批料的落点每次刷新都一样（固定种子），可复现才可校对。
export const MASK_MARK = "已脱敏";

// 审计里「读出来就已经被抹过」的条数 —— 「隐匿」那一层唯一能对上的真数。
// 它从流水原文里数出来，不是估的；一条都没有就照写 0。
export function maskedCount(v: AuditView | null): number | null {
  if (!v) return null;
  let n = 0;
  for (const r of v.records) {
    const text = (r.why ?? "") + " " + (r.detail ?? "") + " " + (r.object ?? "");
    if (text.indexOf(MASK_MARK) >= 0) n++;
  }
  return n;
}

// 一批料（固定的一批，不是随机的）：每一颗是一个**类别**，不是一个数。
export type SedKind = "live" | "dead" | "masked" | "settled";
export type SedToken = { kind: SedKind; band: string; label: string };

export const SED_TOKENS: SedToken[] = [
  { kind: "live",    band: "hot",   label: "session · 一轮对话" },
  { kind: "dead",    band: "noise", label: "repeat · 重复的话" },
  { kind: "live",    band: "hot",   label: "decision · 一个决定" },
  { kind: "dead",    band: "noise", label: "noise · 没写完的半句" },
  { kind: "masked",  band: "mask",  label: "明文 · 一把 key" },
  { kind: "settled", band: "cold",  label: "经验 · 沉下来的" },
  { kind: "live",    band: "hot",   label: "case · 一个卡点" },
  { kind: "dead",    band: "noise", label: "repeat · 重复的话" },
  { kind: "settled", band: "cold",  label: "立项书 · 副本" },
  { kind: "masked",  band: "mask",  label: "明文 · 一份凭据" },
];

// 一颗料的宽度：按字数现算（中文比西文宽近一倍），不写死 px —— 改字不用改样式。
export function tokenWidth(s: string): number {
  let w = 18;
  for (const ch of s) w += ch.charCodeAt(0) > 0x2e80 ? 9.6 : 5.4;
  return Math.round(w);
}

// 图例一行：色块 + 名字 + 一句话 + **真数**（右边的数就是这个图层现在的实况）。
export function sedRow(sw: string, name: string, n: string, text: string): HTMLElement {
  return el("div", { class: "li" },
    el("span", { class: "sw " + sw }),
    el("div", {}, el("b", {}, name), el("p", {}, text)),
    el("div", { class: "n" }, n));
}

export function sedimentSection(d: Data): Kid {
  const a = d.assets;
  const files = a ? a.dirs.reduce((n, x) => n + x.files, 0) : null;
  const dirs = a ? a.dirs.length : null;
  const charters = a ? a.charters : null;
  const masked = maskedCount(d.audit);
  const num = (v: number | null): string => (v === null ? "读不到" : String(v));
  const no = "未接入";
  // 热层那一格。三个条件缺一个都问不出「有几条」：接口回了 / 接上了 / 读得动。
  // 缺一个就如实写「未接入」或「读不到」—— **0 会被读成「热层是空的」**，那是在编。
  const h = d.hot;
  const hotCnt = (): string => {
    const v = d.hot;
    if (v === null || !v.wired) return no;
    if ((v.error ?? "") !== "") return "读不到";
    return num(v.total) + " 条";
  };
  const hotBandNote = (): string => {
    if (!h) return "还在发生的事，带保鲜期";
    if (!h.wired) return "没接上 —— 读不到，不等于里面是空的";
    if ((h.error ?? "") !== "") return "接上了，但这会儿读不动";
    return h.stale > 0 ? "还在发生的事；" + h.stale + " 条不新鲜" : "还在发生的事，带保鲜期";
  };
  const hotText = (): string => {
    const base = "还在发生的事。带保鲜期，过期自己碎掉 —— 所以它便宜";
    if (!h) return base;
    if (!h.wired) return base + "（" + (h.why ?? "") + "）";
    if ((h.error ?? "") !== "") return base + "（" + h.error + "）";
    return base + "（保鲜期 " + durText(h.ttl_secs) + " · 超过 " + durText(h.stale_after_secs) + " 没动就标不新鲜）";
  };
  const hotFoot = (): string => {
    if (!h) return "热层那一格没问出来：" + (d.hotErr === "" ? "服务没回 anc.hot/v2" : d.hotErr) + "。";
    if (!h.wired) return (h.why ?? "热层没接上") + "。所以那一格是「未接入」—— 它不等于「热层是空的」。";
    if ((h.error ?? "") !== "") return "热层接上了，但这会儿读不动：" + h.error + "。";
    return "热层那一格的数来自 /api/hot：现在 " + num(h.total) + " 条，其中 " + h.stale + " 条不新鲜。";
  };

  const band = (key: string, cls: string, label: string, cnt: string, note: string): HTMLElement =>
    el("div", { class: "strata " + cls, "data-band": key },
      el("div", { class: "lbl" }, label),
      el("div", { class: "cnt" }, cnt),
      el("div", { class: "note" }, note));

  const column = el("div", { class: "column" },
    band("open", "s-open", "料口", "", "会话与文件都从这落进来"),
    band("noise", "s-noise", "丢掉", no, "重复、噪声、没写完的半句"),
    band("hot", "s-hot", "热层 · 缓存", hotCnt(), hotBandNote()),
    band("cold", "s-cold", "落库 · 经验", num(files) + " 个文件", "达标才落，落下的就是资产"),
    band("mask", "s-mask", "隐匿 · 指针", num(masked) + " 条", "明文不落库：落库前抹成指针"),
    el("div", { class: "stream" }));

  const legend = el("div", { class: "legendbox" },
    el("div", { class: "card" },
      sedRow("w-noise", "丢掉", no, "重复、噪声、没写完的半句 —— 当场蒸发，不进任何一层"),
      sedRow("w-hot", "热层 · 缓存", hotCnt(), hotText()),
      sedRow("w-cold", "落库 · 经验", num(files),
        "达标了才落。落下的这一条就是资产，永久（本机 " + num(dirs) + " 个目录 · 立项书副本 " + num(charters) + "）"),
      sedRow("w-mask", "隐匿 · 指针", num(masked),
        "明文不落库：落盘前抹成「" + MASK_MARK + ":规则名」的指针，原文件改名归档")));

  return section("沉淀柱", "口径图，不是统计 · 一层比一层更贵、更少、更不该丢",
    el("div", { class: "sedi" }, column, legend),
    el("p", { class: "note" },
      "柱是「口径图」—— 它画的是四层的分工，不是统计；层右边那一格是真数，读不到就写「未接入」或「读不到」，不拿空列表冒充「里面是空的」。",
      hotFoot(),
      "「丢掉」那一格没有数据源 —— 丢弃发生在写入之前，落盘里不留痕迹；四层分法拍板之后再接。"));
}

// 落料：柱子插进 DOM 之后再算落点（层高是按布局量出来的，不是写死的数）。
// motion=false 时（重取数据后重画）直接把料摆到那一层上：不落，但一层不缺 ——
// 刷新不该把柱子刷空，也不该把同一屏重播一遍。
export function paintSediment(scope: HTMLElement, motion: boolean): void {
  const column = scope.querySelector(".column");
  if (!(column instanceof HTMLElement)) return;
  const bands = new Map<string, HTMLElement>();
  column.querySelectorAll<HTMLElement>(".strata").forEach((s) => {
    const k = s.getAttribute("data-band");
    if (k !== null) bands.set(k, s);
  });
  const stream = column.querySelector(".stream");
  if (!(stream instanceof HTMLElement)) return;
  const still = !motion || window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  let seed = 20261012;
  const rnd = (): number => {
    seed = (seed * 1103515245 + 12345) & 0x7fffffff;
    return seed / 0x7fffffff;
  };
  // 落点：左边那段留给层的字，右边按格子排（同一层不叠）
  const left = 196;
  const usable = Math.max(120, column.clientWidth - left - 84);
  const seat = new Map<string, number>();
  SED_TOKENS.forEach((t, i) => {
    const at = bands.get(t.band);
    if (!at) return;
    const n = seat.get(t.band) ?? 0;
    seat.set(t.band, n + 1);
    const top = at.offsetTop;
    const dead = t.band === "noise";
    const mask = t.kind === "masked";
    const row = dead ? 0 : Math.floor(n / 2);
    const col = dead ? 0 : n % 2;
    const y = dead
      ? top + 26 + rnd() * 22
      : Math.min(top + 40 + row * 28, top + at.offsetHeight - 26);
    const tk = el("div",
      { class: "tk" + (dead ? " dead" : "") + (mask ? " masked" : "") + (still ? " still" : "") },
      el("span", { class: "txt" }, t.label),
      mask ? el("span", { class: "ptr" }, "→ 指针 · 飞书") : "");
    tk.style.setProperty("--x", (left + col * (usable / 2) + (dead ? rnd() * 40 : 0)).toFixed(0) + "px");
    tk.style.setProperty("--w1", tokenWidth(t.label) + "px");
    tk.style.setProperty("--w2", tokenWidth("→ 指针 · 飞书") + "px");
    tk.style.setProperty("--to", y.toFixed(0) + "px");
    tk.style.setProperty("--mid", (top + 6 + rnd() * 24).toFixed(0) + "px");
    tk.style.setProperty("--dur", (1.45 + rnd() * 0.6).toFixed(2) + "s");
    tk.style.setProperty("--d", (i * 0.1).toFixed(2) + "s");
    tk.style.setProperty("--g", (i * 0.1 + 0.84).toFixed(2) + "s");
    stream.append(tk);
  });
}
