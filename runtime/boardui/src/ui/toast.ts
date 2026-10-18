import { el } from "../core/dom";

export let toastEl: HTMLElement | null = null;
export let toastTimer = 0;
export function showToast(msg: string): void {
  if (!toastEl) {
    toastEl = el("div", { class: "toast", role: "status" });
    document.body.append(toastEl);
  }
  toastEl.textContent = msg;
  toastEl.classList.add("on");
  window.clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => toastEl?.classList.remove("on"), 2200);
}
