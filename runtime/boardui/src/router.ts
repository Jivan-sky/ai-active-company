import { icon } from "./core/icons";
import type { Page } from "./core/types";
import { renderApprovals } from "./pages/approvals";
import { renderAssets, renderCases, renderSediment } from "./pages/assets";
import { renderAudit } from "./pages/audit";
import { renderContext } from "./pages/context";
import { renderGraph } from "./pages/graph";
import { renderDomains, renderMembers, renderProjects } from "./pages/org";
import { renderOverview } from "./pages/overview";
import { renderSystem } from "./pages/system";
import { renderTimeline } from "./pages/timeline";

// 页面注册表。顺序就是侧栏顺序；分组是「问题」，页名是「答案」。
// 这一版把导航收敛成六组：基础设施不再各占一条，组织关系落到「成员与 Agent」。
export const overviewPage: Page = {
  id: "overview", group: "总览", label: "总览", icon: "overview", wired: true,
  question: "现在有没有事、要不要我动手？",
  sub: "一张运营态势图：校验、待批、运行、热层、资产，五路信号汇到同一个视野里。",
  view: renderOverview,
};
export const approvalsPage: Page = {
  id: "approvals", group: "总览", label: "待批", icon: "check", wired: true,
  question: "哪几件事在等人点头、该谁点？", view: renderApprovals,
};
export const graphPage: Page = {
  id: "graph", group: "组织", label: "关系图谱", icon: "graph", wired: true,
  question: "业务域、项目、成员、bot 是怎么挂在一起的？",
  sub: "节点全部来自真相源与运行态投影；连线只表示「挂在哪」，不表示业务流。",
  view: renderGraph,
};
export const membersPage: Page = {
  id: "members", group: "组织", label: "成员管理", icon: "users", wired: true,
  question: "谁在负责什么、身上挂着哪些 Agent、现在在做什么？",
  sub: "一人一张卡，点开是身份、名下 Agent 的接入与在线状态、当前在做的事；每一段关系都标了推导依据。",
  view: renderMembers,
};
export const domainsPage: Page = {
  id: "domains", group: "组织", label: "业务域", icon: "grid", wired: true,
  question: "一块业务是什么、谁在做、数据在哪？", view: renderDomains,
};
export const projectsPage: Page = {
  id: "projects", group: "工作", label: "项目", icon: "layers", wired: true,
  question: "谁在做什么、到几号、卡住找谁、进度到哪？",
  sub: "周期时间消耗按原文日期推算；交付进度没有真相源时明确留白。",
  view: renderProjects,
};
export const contextPage: Page = {
  id: "context", group: "工作", label: "上下文", icon: "context", wired: true,
  question: "一个 agent 的工作上下文是什么？", view: renderContext,
};
export const timelinePage: Page = {
  id: "timeline", group: "记录", label: "时间线", icon: "clock", wired: true,
  question: "卡在哪、谁在跟、下一步是谁的决定？", view: renderTimeline,
};
export const casesPage: Page = {
  id: "cases", group: "记录", label: "卡点", icon: "alert", wired: true,
  question: "现在哪几件事正在卡住、谁该动？", view: renderCases,
};
export const auditPage: Page = {
  id: "audit", group: "记录", label: "审计", icon: "shield", wired: true,
  question: "谁在什么时候、对谁、行使了什么（含被拒的）？", view: renderAudit,
};
export const assetsPage: Page = {
  id: "assets", group: "资产", label: "原料", icon: "box", wired: true,
  question: "手上有什么料、多久没动了？", view: renderAssets,
};
export const sedimentPage: Page = {
  id: "sediment", group: "资产", label: "沉淀", icon: "strata", wired: true,
  question: "料怎么分层、哪一层现在是真数？", view: renderSediment,
};
export const systemPage: Page = {
  id: "system", group: "系统", label: "运行与连接", icon: "pulse", wired: true,
  question: "基础设施还健康吗、执行面和授权有没有缺口？",
  sub: "运行矩阵 / 网关 / 授权 / 数据流 / 热层集中在这里；正常时不打扰，异常时才要人看。",
  view: renderSystem,
};

export const PAGES: Page[] = [
  overviewPage, approvalsPage,
  projectsPage, contextPage,
  graphPage, membersPage, domainsPage,
  timelinePage, casesPage, auditPage,
  assetsPage, sedimentPage,
  systemPage,
];

// 旧 hash 仍要能打开：这些页并进「系统」或「成员与 Agent」之后，书签和飞书里的旧链接不能断。
// 只改写落点，不改语义 —— 页面自己会按 tab 参数决定显示哪一段。
export const LEGACY: Record<string, string> = {
  runtime: "system", dataflow: "system", gateway: "system", hot: "system", grants: "system",
  roles: "members", org: "overview",
};

export function pageFor(id: string): Page {
  const to = LEGACY[id];
  if (to !== undefined) return to === "members" ? membersPage : to === "system" ? systemPage : overviewPage;
  for (const p of PAGES) if (p.id === id) return p;
  return overviewPage;
}
