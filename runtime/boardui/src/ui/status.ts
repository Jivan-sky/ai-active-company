import type { Kid } from "../core/types";
import { dash, el } from "../core/dom";

// 运行态四档口径（拍板）：绿 = 近期真回过话；黄 = 运行中 / 这个窗口没观测到；红 = 卡点，需人介入；
// 灰 = 真相源里**声明**了「还没接凭据」—— 本来就不该期待它回话，单列一档，
// 免得跟黄糊在一起（混着看，整份报告都会不值得信）。
export const RUN_STATE: Record<string, { dot: string; label: string }> = {
  ok: { dot: "dot ok", label: "绿 · 近期回过话" },
  warn: { dot: "dot warn", label: "黄 · 运行中 / 未观测" },
  fail: { dot: "dot bad", label: "红 · 卡点，需介入" },
  // 未接凭据：**本来就不该期待它回话**。用灰**实心**，与「认不出的词」那个空心虚线圈分开 ——
  // 一个说「别催它」，一个说「去判它」（#59：灰原来把两种意思叠在一个符号上）。
  unwired: { dot: "dot gray-solid", label: "灰实心 · 未接凭据（本来就不该期待）" },
};
export const runState = (s: string) => RUN_STATE[s] ?? { dot: "dot", label: dash(s) };

// 带颜色的状态点：点 + 文字放进一个 inline-flex，点才有宽度（.dot 是固定宽高）。
export const dotLine = (cls: string, ...kids: Kid[]): HTMLElement =>
  el("span", { class: "st" }, el("span", { class: cls }), ...kids);
export const BAND_DOT: Record<string, string> = {
  green: "dot ok", yellow: "dot warn", red: "dot bad", unknown: "dot gray",
  // 第四档：等人给一句话。它**不来自 status** —— 来自「这条正躺在待批队列里」这个事实，
  // 所以它只在真有那个数据源的地方出现（审批页），不从热层的 status 词推（#59 拍的口径）。
  "input-required": "dot wait",
};
export const bandDot = (b: string): string => BAND_DOT[b] ?? "dot";
export const BAND_WORD: Record<string, string> = {
  green: "绿 · 已完成", yellow: "黄 · 运行中", red: "红 · 卡点 / 失败", unknown: "灰 · 认不出的词",
  "input-required": "靛 · 等你给一句话",
};
export const bandWord = (b: string): string => BAND_WORD[b] ?? "灰 · 认不出的词";

// 四档图例：形状 → 含义。摆出来是给**不看颜色**的读者留一条路（色盲、灰度截图、小屏）。
export function bandLegend(): HTMLElement {
  return el("p", { class: "legend" },
    dotLine("dot ok", el("span", {}, "实心圆 · 已完成")),
    dotLine("dot warn", el("span", {}, "带光环的圆 · 运行中")),
    dotLine("dot bad", el("span", {}, "菱形 · 卡点或失败")),
    dotLine("dot gray", el("span", {}, "空心虚线圈 · 认不出的词（没判过）")));
}
