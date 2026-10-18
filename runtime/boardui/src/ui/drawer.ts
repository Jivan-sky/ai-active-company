import { el } from "../core/dom";
import { icon } from "../core/icons";
import type { Kid } from "../core/types";

export type DrawerSpec = { eyebrow: string; title: string; sub: string; body: Kid[] };
export let drawerLayer: HTMLElement | null = null;
export let drawerEyebrow: HTMLElement | null = null;
export let drawerTitle: HTMLElement | null = null;
export let drawerSub: HTMLElement | null = null;
export let drawerBody: HTMLElement | null = null;

export function mountDrawer(): HTMLElement {
  const eyebrow = el("div", { class: "eyebrow" });
  const title = el("h2");
  const sub = el("div", { class: "dsub" });
  const body = el("div", { class: "drawer-body" });
  const closeBtn = el("button", { class: "icon-btn", type: "button", "aria-label": "关闭详情" }, icon("close", 15));
  closeBtn.addEventListener("click", closeDrawer);
  const layer = el("div", { class: "drawer-layer", "data-open": "0" },
    el("div", { class: "drawer-scrim" }),
    el("aside", { class: "drawer" },
      el("header", {},
        el("div", { class: "dh" }, eyebrow, title, sub),
        closeBtn),
      body));
  layer.querySelector(".drawer-scrim")?.addEventListener("click", closeDrawer);
  drawerEyebrow = eyebrow;
  drawerTitle = title;
  drawerSub = sub;
  drawerBody = body;
  drawerLayer = layer;
  return layer;
}

export function openDrawer(spec: DrawerSpec): void {
  if (!drawerLayer || !drawerBody || !drawerTitle || !drawerSub || !drawerEyebrow) return;
  drawerEyebrow.textContent = spec.eyebrow;
  drawerTitle.textContent = spec.title;
  drawerSub.textContent = spec.sub;
  drawerBody.replaceChildren(...spec.body);
  drawerLayer.dataset.open = "1";
  document.body.style.overflow = "hidden";
}

export function closeDrawer(): void {
  if (!drawerLayer) return;
  drawerLayer.dataset.open = "0";
  document.body.style.overflow = "";
}

export function isDrawerOpen(): boolean {
  return drawerLayer?.dataset.open === "1";
}
