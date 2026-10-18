import { el } from "../core/dom";
import type { Kid } from "../core/types";
import { hashQuery } from "../core/query";

export const pageNo = (q: URLSearchParams, key = "p"): number => {
  const n = Number.parseInt((q.get(key) ?? "").trim(), 10);
  return Number.isFinite(n) && n > 0 ? n : 1;
};

// 一屏铺多少行。翻页是**视图**的事，不是门禁 —— 取多少由 limit 决定（见 fetchData），
// 翻页只决定这一屏铺哪一段。
export const PER_PAGE = 20;

// 翻页条：只有一页就**不出现**，越界自动夹回第一页。
export function pager(total: number, page: number, id: string): Kid {
  const pages = Math.max(1, Math.ceil(total / PER_PAGE));
  if (pages <= 1) return "";
  const cur = Math.min(Math.max(page, 1), pages);
  const step = (target: number, label: string, off: boolean): HTMLElement => {
    const a = el("a", { class: "pg", href: "#/" + id + "?p=" + String(target) }, label);
    if (off) {
      a.removeAttribute("href");
      a.setAttribute("aria-disabled", "true");
    }
    return a;
  };
  return el("div", { class: "pager" },
    step(cur - 1, "‹ 上一页", cur <= 1),
    el("span", { class: "pgind" },
      "第 " + cur + " / " + pages + " 页 · 每页 " + PER_PAGE + " · 共 " + total),
    step(cur + 1, "下一页 ›", cur >= pages));
}

// 把一份长列表切成当前这一屏，并夹住页号。
export function sliced<T>(list: T[], q: URLSearchParams): { rows: T[]; cur: number; pages: number } {
  const pages = Math.max(1, Math.ceil(list.length / PER_PAGE));
  const cur = Math.min(pageNo(q), pages);
  return { rows: list.slice((cur - 1) * PER_PAGE, cur * PER_PAGE), cur, pages };
}
