import { card, dash, el } from "../core/dom";
import { icon } from "../core/icons";
import { cache } from "../core/state";
import { openAgentDrawer, openMemberDrawer, openProjectDrawer } from "../org/drawers";
import { agentsOf } from "../org/view";
import { PAGES } from "../router";
import { closeDrawer, isDrawerOpen, openDrawer } from "./drawer";
import { bandWord, runState } from "./status";

export type PaletteItem = { label: string; hint: string; icon: string; href?: string; run?: () => void };
export let paletteLayer: HTMLElement | null = null;
export let paletteInput: HTMLInputElement | null = null;
export let paletteList: HTMLElement | null = null;
export let paletteResults: PaletteItem[] = [];
export let paletteActive = 0;

export function mountPalette(): HTMLElement {
  const input = el("input", { type: "text", placeholder: "跳转到页面、业务域、项目、成员、bot…", "aria-label": "搜索" }) as HTMLInputElement;
  const list = el("div", { class: "palette-list" });
  const layer = el("div", { class: "palette-layer", "data-open": "0" },
    el("div", { class: "palette" },
      input, list,
      el("div", { class: "palette-foot" },
        el("span", {}, "↑↓ 选择"), el("span", {}, "Enter 打开"), el("span", {}, "Esc 关闭"))));
  layer.addEventListener("click", (e) => { if (e.target === layer) closePalette(); });
  input.addEventListener("input", () => renderPalette(input.value));
  input.addEventListener("keydown", (e) => paletteKey(e as KeyboardEvent));
  paletteInput = input;
  paletteList = list;
  paletteLayer = layer;
  return layer;
}

export function closePalette(): void {
  if (!paletteLayer) return;
  paletteLayer.dataset.open = "0";
  document.body.style.overflow = "";
}
export function paletteItems(): PaletteItem[] {
  const out: PaletteItem[] = [];
  for (const p of PAGES) {
    out.push({ label: p.label, hint: p.group + " · " + p.question, icon: p.icon, href: "#/" + p.id });
  }
  const d = cache;
  if (d?.board) {
    for (const x of d.board.domains) {
      out.push({
        label: x.name, hint: "业务域 · " + x.slug, icon: "grid",
        run: () => openDrawer({
          eyebrow: "DOMAIN / 业务域", title: dash(x.name), sub: dash(x.slug),
          body: [card(dash(x.name), dash(x.slug), [
            ["是什么", dash(x.what)], ["谁在做", dash(x.who_label)],
            ["数据在哪", dash(x.data)], ["原件来源", dash(x.sources)], ["口径 / 术语", dash(x.terms)],
          ])],
        }),
      });
    }
    for (const x of d.board.projects) {
      out.push({
        label: x.name, hint: "项目 · " + x.owner_label + " · " + x.period, icon: "layers",
        run: () => openProjectDrawer(d, d.board!, x),
      });
    }
    for (const x of d.board.members) {
      out.push({
        label: x.display_name || x.name, hint: "成员 · " + x.role + (x.disabled ? " · 已停用" : ""), icon: "users",
        run: () => openMemberDrawer(d, d.board!, x),
      });
    }
    for (const x of agentsOf(d.board)) {
      out.push({
        label: x.name || x.slug, hint: "业务 Agent · " + x.domain + " · " + x.role, icon: "spark",
        run: () => openAgentDrawer(d, d.board!, x),
      });
    }
  }
  if (d?.runtime?.wired) {
    for (const x of d.runtime.bots) {
      out.push({
        label: x.project, hint: "bot · " + runState(x.state).label, icon: "pulse",
        run: () => openDrawer({
          eyebrow: "BOT / 运行态", title: dash(x.project), sub: runState(x.state).label,
          body: [card(dash(x.project), x.state, [["状态", runState(x.state).label], ["依据", dash(x.why)]])],
        }),
      });
    }
  }
  if (d?.approvals?.wired) {
    for (const x of d.approvals.pending) {
      out.push({ label: x.title, hint: "待批 · " + dash(x.who) + " → " + dash(x.to), icon: "check", href: "#/approvals" });
    }
  }
  if (d?.hot?.wired) {
    for (const x of d.hot.entries) {
      out.push({ label: x.title, hint: "热层 · " + dash(x.status), icon: "flame", href: "#/system?tab=hot" });
    }
  }
  if (d?.timeline?.wired) {
    for (const x of d.timeline.cases) {
      out.push({ label: x.title, hint: "case · " + bandWord(x.band), icon: "clock", href: "#/timeline" });
    }
  }
  return out;
}

export function runPaletteItem(item: PaletteItem): void {
  closePalette();
  if (item.href) {
    location.hash = item.href;
    return;
  }
  item.run?.();
}

export function setPaletteActive(i: number): void {
  if (!paletteList || paletteResults.length === 0) return;
  paletteActive = (i + paletteResults.length) % paletteResults.length;
  Array.from(paletteList.children).forEach((n, j) => n.classList.toggle("active", j === paletteActive));
  const active = paletteList.children[paletteActive];
  if (active instanceof HTMLElement) active.scrollIntoView({ block: "nearest" });
}

export function renderPalette(q: string): void {
  if (!paletteList) return;
  const needle = q.trim().toLowerCase();
  const all = paletteItems();
  paletteResults = (needle === "" ? all : all.filter((x) => (x.label + " " + x.hint).toLowerCase().includes(needle))).slice(0, 40);
  paletteActive = 0;
  if (paletteResults.length === 0) {
    paletteList.replaceChildren(el("div", { class: "palette-item" }, el("span", { class: "pl" }, el("b", {}, "没有匹配"), el("small", {}, "换个词试试：业务域、项目、成员、bot、待批"))));
    return;
  }
  paletteList.replaceChildren(...paletteResults.map((item, i) => {
    const row = el("div", { class: i === 0 ? "palette-item active" : "palette-item", role: "button" },
      el("span", { class: "pi" }, icon(item.icon, 13)),
      el("span", { class: "pl" }, el("b", {}, item.label), el("small", {}, item.hint)),
      el("span", { class: "pk" }, item.href ? "↵" : "详情"));
    row.addEventListener("mouseenter", () => setPaletteActive(i));
    row.addEventListener("click", () => runPaletteItem(item));
    return row;
  }));
}

export function openPalette(): void {
  if (!paletteLayer || !paletteInput) return;
  paletteLayer.dataset.open = "1";
  paletteInput.value = "";
  renderPalette("");
  document.body.style.overflow = "hidden";
  paletteInput.focus();
}

export function paletteKey(e: KeyboardEvent): void {
  if (e.key === "ArrowDown") { e.preventDefault(); setPaletteActive(paletteActive + 1); return; }
  if (e.key === "ArrowUp") { e.preventDefault(); setPaletteActive(paletteActive - 1); return; }
  if (e.key === "Enter") {
    e.preventDefault();
    const item = paletteResults[paletteActive];
    if (item) runPaletteItem(item);
    return;
  }
  if (e.key === "Escape") { e.preventDefault(); closePalette(); }
}

document.addEventListener("keydown", (e) => {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
    e.preventDefault();
    openPalette();
    return;
  }
  if (e.key !== "Escape") return;
  if (paletteLayer?.dataset.open === "1") { e.preventDefault(); closePalette(); return; }
  if (isDrawerOpen()) { e.preventDefault(); closeDrawer(); }
});
