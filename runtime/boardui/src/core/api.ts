import { ctxQuery, grantsQuery, searchQuery } from "./query";
import type { AgentsView, ApprovalView, AssetsView, AuditView, BoardView, ContextView, Data, DataflowView, DistillView, GrantsView, HotView, Issues, RuntimeView, SearchView, TimelineView } from "./types";

// ---------- 取数 ----------

export type Fetched<T> = { status: number; body: T | null; err: string };

export async function getJSON<T>(url: string): Promise<Fetched<T>> {
  try {
    const res = await fetch(url, { headers: { accept: "application/json" } });
    const raw = await res.text();
    try {
      return { status: res.status, body: JSON.parse(raw) as T, err: "" };
    } catch {
      return { status: res.status, body: null, err: "返回的不是 JSON（HTTP " + res.status + "）" };
    }
  } catch (e) {
    return { status: 0, body: null, err: "连不上看板服务：" + (e instanceof Error ? e.message : String(e)) };
  }
}

// 看板只有一处写：审批信号。它只收 POST + application/json —— 跨站表单发不出这个 Content-Type
// （浏览器会先发预检，而看板不回 CORS 头），所以没有 token 也挡得住「别的网页替你点头」。
export async function postJSON<T>(url: string, body: unknown): Promise<Fetched<T>> {
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: { "content-type": "application/json", accept: "application/json" },
      body: JSON.stringify(body),
    });
    const raw = await res.text();
    try {
      return { status: res.status, body: JSON.parse(raw) as T, err: "" };
    } catch {
      return { status: res.status, body: null, err: "返回的不是 JSON（HTTP " + res.status + "）" };
    }
  } catch (e) {
    return { status: 0, body: null, err: "连不上看板服务：" + (e instanceof Error ? e.message : String(e)) };
  }
}

export const emptyIssues = (err: string): Issues => ({ ok: false, fatal: [], warn: [], error: err });

// 「读到了一份能吃的契约」= body 在、schema 是非空字符串。五份契约（board / runtime /
// dataflow / assets / timeline）各判各的 —— 一份坏了不该让别的页跟着空着。
export function contract<T extends { schema?: string }>(res: Fetched<T>, label: string): { v: T | null; err: string } {
  const body = res.body;
  if (body && typeof body.schema === "string" && body.schema !== "") return { v: body, err: "" };
  return {
    v: null,
    err: res.err !== "" ? res.err : label + "读取失败（HTTP " + res.status + "）",
  };
}

export async function fetchData(): Promise<Data> {
  // 搜索只在 URL 里真的带了 q 时才取：没有查询词就发一次注定被拒的请求，
  // 既浪费一次全库建索引，又会让页面一进来就挂着一条假错误。
  const sq = searchQuery();
  const searchP: Promise<Fetched<SearchView> | null> = sq === "" ? Promise.resolve(null) : getJSON<SearchView>("/api/search" + sq);
  const [boardRes, issuesRes, runtimeRes, dataflowRes, assetsRes, distillRes, timelineRes, auditRes, approvalsRes, hotRes, agentsRes, ctxRes, grantsRes, searchRes] = await Promise.all([
    getJSON<BoardView & { error?: string }>("/api/board"),
    getJSON<Issues>("/api/issues"),
    getJSON<RuntimeView>("/api/runtime"),
    getJSON<DataflowView>("/api/dataflow"),
    getJSON<AssetsView>("/api/assets"),
    getJSON<DistillView>("/api/distill"),
    // 长列表按接口给的上限一次取回（timeline 上限 200 / audit 上限 500），翻页在视图里切。
    // 不这么取的话，翻页每翻一页要重取一次，而这一页只看得到 20 行 —— 划不来。
    getJSON<TimelineView>("/api/timeline?limit=200"),
    getJSON<AuditView>("/api/audit?limit=500"),
    getJSON<ApprovalView>("/api/approvals"),
    getJSON<HotView>("/api/hot"),
    // 在线态单开一条：它不是真相源投影，也不该混进任务热层。
    getJSON<AgentsView>("/api/agents"),
    // 上下文那一页：选中的人进 hash（#/context?who=alice），所以取数也照 URL 来 ——
    // 刷新、把链接发给别人、前进后退，看到的都是同一个人。
    getJSON<ContextView>("/api/context" + ctxQuery()),
    // 授权表那一页：视角也在 hash 里（#/grants?who=bizbot），所以取数照 URL 来。
    getJSON<GrantsView>("/api/grants" + grantsQuery()),
    searchP,
  ]);
  const issues = issuesRes.body ?? emptyIssues(issuesRes.err);
  const rt = contract(runtimeRes, "运行态");
  const df = contract(dataflowRes, "数据流");
  const as = contract(assetsRes, "原料清单");
  const ds = contract(distillRes, "可沉淀候选");
  const tl = contract(timelineRes, "时间线");
  const au = contract(auditRes, "审计");
  const ap = contract(approvalsRes, "待批队列");
  const ho = contract(hotRes, "热层");
  const ag = contract(agentsRes, "Agent 在线态");
  const cx = contract(ctxRes, "工作上下文");
  const gr = contract(grantsRes, "授权表");
  const se = searchRes ? contract(searchRes, "知识检索") : { v: null, err: "" };
  const schema = boardRes.body?.schema;
  if (boardRes.body && typeof schema === "string" && schema !== "") {
    return {
      board: boardRes.body, issues, err: "",
      agents: ag.v, agentsErr: ag.err,
      hot: ho.v, hotErr: ho.err,
      context: cx.v, contextErr: cx.err,
      grants: gr.v, grantsErr: gr.err,
      runtime: rt.v, runtimeErr: rt.err,
      dataflow: df.v, dataflowErr: df.err,
      assets: as.v, assetsErr: as.err,
      search: se.v, searchErr: se.err,
      distill: ds.v, distillErr: ds.err,
      timeline: tl.v, timelineErr: tl.err,
      audit: au.v, auditErr: au.err,
      approvals: ap.v, approvalsErr: ap.err,
    };
  }
  const why = boardRes.body?.error ?? boardRes.err ?? ("看板数据读取失败（HTTP " + boardRes.status + "）");
  return {
    board: null, issues, err: why,
    agents: ag.v, agentsErr: ag.err,
    hot: ho.v, hotErr: ho.err,
    context: cx.v, contextErr: cx.err,
    grants: gr.v, grantsErr: gr.err,
    runtime: rt.v, runtimeErr: rt.err,
    dataflow: df.v, dataflowErr: df.err,
    assets: as.v, assetsErr: as.err,
    search: se.v, searchErr: se.err,
    distill: ds.v, distillErr: ds.err,
    timeline: tl.v, timelineErr: tl.err,
    audit: au.v, auditErr: au.err,
    approvals: ap.v, approvalsErr: ap.err,
  };
}

