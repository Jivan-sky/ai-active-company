import type { Data, Kid } from "../core/types";
import { card, el, section } from "../core/dom";
import { agentsOf } from "../org/view";
import { noBoard } from "../ui/blocks";

// 关系图谱 —— 一张只画「结构」的图。节点来自真相源 + 运行态，连线只表示挂靠关系。
// 它不是装饰：点节点能打开同一个详情抽屉，和命令面板里搜出来的是同一份字段。
export function renderGraph(d: Data): Kid[] {
  const b = d.board;
  if (!b) return [noBoard(d)];
  const bots = d.runtime && d.runtime.wired ? d.runtime.bots : [];
  const rows: Array<[string, number, string]> = [
    ["业务域", b.domains.length, "domain"], ["项目", b.projects.length, "project"],
    ["成员", b.members.length, "member"], ["Agent", agentsOf(b).length, "agent"], ["bot", bots.length, "bot"],
  ];
  return [
    el("section", { class: "graph-stage" },
      el("canvas", { class: "org-canvas", "aria-label": "组织关系图" }),
      el("div", { class: "graph-counts" }, ...rows.map(([k, n, cls]) =>
        el("div", { class: "gc " + cls }, el("b", {}, String(n)), el("span", {}, k))))),
    section("这张图怎么读", "只画结构，不画业务流；不认识的节点不补",
      el("div", { class: "grid" },
        card("连线", "两种关系", [["业务域 → 项目", "项目属于哪块业务"], ["项目 → bot", "bot 服务哪个项目"],
          ["业务域 → 成员", "成员负责哪块业务"], ["成员 → 上级", "persona 的 reports_to（金色线）"]]),
        card("不画什么", "避免误读", [["业务流", "不在这张图里"], ["消息", "通道状态看「系统 · 数据流」"], ["权限", "看「系统 · 网关与授权」"]]),
        card("数据从哪来", "两份投影", [["结构", "anc.board/v3"], ["运行态", "anc.runtime/v1"], ["缺一份", "图仍然画，缺口在页脚说明"]]))),
  ];
}
