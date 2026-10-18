import type { Attrs, Kid } from "./types";

export const root = document.getElementById("app");
// ---------- 画界面的最小零件 ----------

export function el(tag: string, attrs: Attrs = {}, ...kids: Kid[]): HTMLElement {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "class") node.className = v;
    else node.setAttribute(k, v);
  }
  for (const kid of kids) node.append(kid);
  return node;
}

export const dash = (v: string | null | undefined): string => {
  const s = (v ?? "").trim();
  return s === "" ? "—" : s;
};

export const chip = (label: string, mono = false): HTMLElement =>
  el("span", { class: mono ? "chip mono" : "chip" }, label);

export function card(title: string, badge: string, rows: Array<[string, Kid]>): HTMLElement {
  const head = el("h3", {}, title);
  if (badge !== "") head.append(el("span", { class: "chip mono" }, badge));
  const dl = el("dl");
  for (const [k, v] of rows) dl.append(el("dt", {}, k), el("dd", {}, v));
  return el("div", { class: "card" }, head, dl);
}

export function table(headers: string[], rows: Kid[][]): HTMLElement {
  const head = el("tr");
  for (const h of headers) head.append(el("th", {}, h));
  const body = el("tbody");
  for (const r of rows) {
    const tr = el("tr");
    for (const c of r) tr.append(el("td", {}, c));
    body.append(tr);
  }
  // 套一层滚动容器：窄屏（比如嵌在侧栏里）表格不该把整页撑破。
  return el("div", { class: "tw" }, el("table", {}, el("thead", {}, head), body));
}

export function section(title: string, hint: string, ...body: Kid[]): HTMLElement {
  const h = el("h2", {}, title);
  if (hint !== "") h.append(el("span", { class: "hint" }, hint));
  return el("section", { class: "sec" }, h, ...body);
}

export function banner(kind: "bad" | "warn", title: string, items: Kid[]): HTMLElement {
  const ul = el("ul");
  for (const i of items) {
    // 调用方用 <br> 分隔「标题句 / 补充句」；它不该变成一个空的项目符号。
    if (i instanceof HTMLElement && i.tagName === "BR") continue;
    ul.append(el("li", {}, i));
  }
  return el("div", { class: "banner " + kind }, el("h3", {}, title), ul);
}
