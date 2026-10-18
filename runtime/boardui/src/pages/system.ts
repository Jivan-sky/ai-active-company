import type { Attrs, Data, Kid } from "../core/types";
import { el } from "../core/dom";
import { currentId, hashQuery } from "../core/query";
import { renderHot } from "./assets";
import { renderDataflow } from "./dataflow";
import { renderGateway } from "./gateway";
import { renderGrants } from "./grants";
import { renderRuntime } from "./runtime";

// 系统 —— 基础设施类信息集中到一页。
// 运行矩阵 / 网关 / 数据流 / 热层 / 授权，这些是「异常时出现、正常时收起」的东西，
// 不该各占一条主导航。旧 hash（#/runtime 等）仍能打开，落到对应标签。
export type SystemTab = { id: string; label: string; hint: string; view: (d: Data) => Kid[] };

export const SYSTEM_TABS: SystemTab[] = [
  { id: "runtime", label: "运行与连接", hint: "每个 bot 真的能回话吗", view: renderRuntime },
  { id: "gateway", label: "网关与授权", hint: "执行面配置 + 谁被允许做什么", view: (d) => [...renderGateway(d), ...renderGrants(d)] },
  { id: "dataflow", label: "数据流", hint: "通道开着吗、手里有什么", view: renderDataflow },
  { id: "hot", label: "热层", hint: "还没落库的事", view: renderHot },
];

export function activeSystemTab(): SystemTab {
  const here = currentId();
  const alias = here === "grants" ? "gateway" : here;
  const q = (hashQuery().get("tab") ?? "").trim();
  return SYSTEM_TABS.find((t) => t.id === q)
    ?? SYSTEM_TABS.find((t) => t.id === alias)
    ?? SYSTEM_TABS[0]!;
}

export function systemHref(id: string): string {
  const who = (hashQuery().get("who") ?? "").trim();
  if (id === "gateway" && who !== "") return "#/system?tab=gateway&who=" + encodeURIComponent(who);
  return "#/system?tab=" + id;
}

export function renderSystem(d: Data): Kid[] {
  const cur = activeSystemTab();
  const tabs = el("div", { class: "subtabs", role: "tablist" },
    ...SYSTEM_TABS.map((t) => {
      const attrs: Attrs = {
        class: t.id === cur.id ? "subtab active" : "subtab",
        href: systemHref(t.id), role: "tab",
      };
      if (t.id === cur.id) attrs["aria-selected"] = "true";
      return el("a", attrs, el("b", {}, t.label), el("small", {}, t.hint));
    }));
  return [tabs, ...cur.view(d)];
}
