// ANC 看板前端（浏览器侧）。
//
// 它只做一件事：把 `/api/board`（anc.board/v1 投影）与 `/api/issues`（真相源健康度）
// 摆成人能看的界面。
//
// 五条纪律：
//   1. 真相源里的字一律走 textContent，绝不 innerHTML —— 看板不许成为注入点；
//   2. 不猜、不补：哪块没有数据源就写清楚「还没接入」，不编假数据把界面填满；
//   3. 几乎只读：唯一的写口是「待批」页的点头（一次留痕，不动真相源）—— 其余全是读；
//   4. 一页只回答一个问题 —— 那句问题写在页头（Page.question），写不出来就该先想清楚再开页；
//   5. 数据源没接通的页面，导航上直接标「未接入」，不靠人点进去才发现；
//   6. 动效分层：换页才进一次场、只有「在动 / 要人管」的状态灯呼吸，数据区一律静 ——
//      看板是天天看的工具，不是落地页（样式与动效预算写在 index.html 顶上）。
//
// 构建：`npm run build`（esbuild 打成 app.js，随 anc 二进制一起嵌进去）。
function renderOverview(d: Data): Kid[] {
  const b = d.board;
  const i = d.issues;
  const pending = d.approvals && d.approvals.wired ? d.approvals.count : null;
  const bots = d.runtime && d.runtime.wired ? d.runtime.bots : [];
  const green = bots.filter((x) => x.state === "ok").length;
  const red = bots.filter((x) => x.state === "fail").length;
  const yellow = bots.filter((x) => x.state === "warn").length;
  const gray = bots.filter((x) => x.state === "unwired").length;
  const activeMembers = b ? b.members.filter((m) => !m.disabled).length : 0;
  const totalMembers = b ? b.members.length : 0;
  const state = healthState(i);
  let headline = "现在没有异常";
  let lead = "校验干净、待批队列为空 —— 但这也只说明真相源格式没问题；agent 是不是真的在跑，要看运行矩阵。";
  if (d.err !== "") {
    headline = "现在读不到投影";
    lead = d.err;
  } else if (i.error !== "") {
    headline = "现在读不到校验";
    lead = i.error;
  } else if (i.fatal.length > 0) {
    headline = "现在有 " + i.fatal.length + " 条红档";
    lead = "红档会拦住落盘。先修真相源，再看运行态 —— 看板不替你把红档藏起来。";
  } else if (pending !== null && pending > 0) {
    headline = "现在有 " + pending + " 件事等你点头";
    lead = "待批队列里有人等你给一句话。点头只留痕，不改真相源；该不该批仍然由人判断。";
  } else if (i.warn.length > 0) {
    headline = "现在有 " + i.warn.length + " 条非红档";
    lead = "非红档不拦落盘，但值得过目。完整清单在审计与校验页，不在这里堆成一面墙。";
  } else if (pending === null) {
    headline = "待批队列还没接上";
    lead = "「读不到」不等于「没人提」。运行矩阵和真相源校验仍然可看；队列接上后再补这一格。";
  }
  const orbValue = pending !== null && pending > 0 ? String(pending) : red > 0 ? String(red) : "OK";
  const orbLabel = pending !== null && pending > 0 ? "待批" : red > 0 ? "卡点" : "稳态";
  const recent = timelineRecent(d, 6);
  const assets = d.assets && d.assets.wired ? d.assets : null;
  const hot = d.hot && d.hot.wired ? d.hot : null;
  const files = assets ? assets.dirs.reduce((n, x) => n + x.files, 0) : null;
  const auditTotal = d.audit && d.audit.wired ? d.audit.total : null;
  const timelineTotal = d.timeline && d.timeline.wired ? d.timeline.total : null;

  // 状态环：四段全部来自运行态，不另算一套。一个 bot 都没接时画灰环，不画绿灯。
  const botTotal = bots.length;
  const deg = (n: number): number => (botTotal === 0 ? 0 : (n / botTotal) * 360);
  const d1 = deg(green);
  const d2 = d1 + deg(yellow);
  const d3 = d2 + deg(red);
  const ring = botTotal === 0
    ? "conic-gradient(rgba(150,153,147,.30) 0 360deg)"
    : "conic-gradient(#4ade9a 0 " + d1 + "deg, #f0b96a " + d1 + "deg " + d2 + "deg, #ff6b81 " + d2 + "deg " + d3 + "deg, rgba(140,143,137,.46) " + d3 + "deg 360deg)";
  const num = (n: number | null): string => (n === null ? "—" : String(n));

  const dockCard = (title: string, en: string, href: string, ...body: Kid[]): HTMLElement =>
    el("section", { class: "dock-card" },
      el("header", {},
        el("div", {}, el("p", { class: "eyebrow" }, en), el("h3", {}, title)),
        el("a", { class: "dock-more", href: href }, "全部", icon("arrow", 12))),
      el("div", { class: "dock-body" }, ...body));

  const loopStep = (no: string, label: string, count: number | null, href: string): HTMLElement =>
    el("a", { class: "loop-step", href: href },
      el("span", { class: "ls-no" }, no), el("b", {}, num(count)), el("span", { class: "ls-label" }, label));

  const dashPage = el("div", { class: "dash-stage" },
    el("canvas", { class: "field-canvas", "aria-hidden": "true" }),
    el("div", { class: "field-veil", "aria-hidden": "true" }),
    el("section", { class: "stage-hero" },
      el("div", { class: "hero-copy" },
        el("p", { class: "eyebrow" }, "LIVE PICTURE",
          el("span", {}, "真相源实时投影 · " + (b ? dash(b.company.name) : "还没读到投影"))),
        el("h2", {}, headline.split("现在")[0] ?? "",
          el("span", { class: "state " + state }, headline.replace(/^现在/, "").trim())),
        el("p", { class: "lead" }, lead),
        el("div", { class: "hero-actions" },
          el("a", { class: "primary-btn", href: "#/approvals" },
            icon("check", 15), "去待批", el("span", { class: "count" }, pending === null ? "—" : String(pending))),
          el("a", { class: "secondary-btn", href: "#/system?tab=runtime" }, icon("pulse", 14), "运行矩阵"),
          el("a", { class: "secondary-btn", href: "#/graph" }, icon("graph", 14), "关系图谱")),
        el("div", { class: "hero-kpis" },
          heroKpi("业务域", b ? String(b.domains.length) : "—", "DOMAINS"),
          heroKpi("项目", b ? String(b.projects.length) : "—", "PROJECTS"),
          heroKpi("成员", b ? activeMembers + "/" + totalMembers : "—", "ACTIVE"),
          heroKpi("bot 绿", d.runtime && d.runtime.wired ? green + "/" + botTotal : "—", "LIVE"))),
      el("div", { class: "hero-orbit", "aria-label": "运行态状态环" },
        el("div", { class: "orbit-shell" },
          el("div", { class: "orbit-ring", style: "background:" + ring }),
          el("div", { class: "orbit-core" },
            el("b", {}, orbValue), el("small", {}, orbLabel),
            el("span", {}, b ? dash(b.company.platform) : "WAITING"))),
        el("div", { class: "orbit-legend" },
          el("span", {}, el("i", { class: "lg ok" }), "绿 " + green),
          el("span", {}, el("i", { class: "lg warn" }), "黄 " + yellow),
          el("span", {}, el("i", { class: "lg bad" }), "红 " + red),
          el("span", {}, el("i", { class: "lg gray" }), "灰 " + gray)))),
    el("div", { class: "stage-dock" },
      dockCard("待批队列", "APPROVALS", "#/approvals",
        pending === null
          ? emptyNote("队列没接上 —— 读不到不等于没人提。")
          : d.approvals && d.approvals.pending.length > 0
            ? el("div", { class: "feed" }, ...d.approvals.pending.slice(0, 3).map((p) =>
                feedRow(bandDot(p.band ?? ""), p.title, dash(p.who) + " → " + dash(p.to), sinceText(p.enqueued), "#/approvals")))
            : emptyNote("队列是空的：现在没有等点头的提案。")),
      dockCard("运行矩阵", "RUNTIME", "#/system?tab=runtime",
        d.runtime === null
          ? emptyNote("运行态没接入。")
          : !d.runtime.wired
            ? emptyNote("运行态没接上：" + dash(d.runtime.gateway_why ?? d.runtime.error))
            : bots.length === 0
              ? emptyNote("config 里一个 project 都没有。")
              : el("div", { class: "bot-grid" }, ...bots.slice(0, 4).map(botRow))),
      dockCard("最近活动", "ACTIVITY", "#/timeline",
        recent.length === 0
          ? emptyNote("时间线是空的 —— 「还没开始记」是个真实状态。")
          : el("div", { class: "feed" }, ...recent.slice(0, 4).map((e) =>
              feedRow(bandDot(e.band), e.title, dash(e.kind) + " · " + dash(e.by) + " → " + dash(e.to), localTime(e.at), "#/timeline"))))),
    el("section", { class: "loop" },
      el("div", { class: "loop-head" },
        el("p", { class: "eyebrow" }, "OPERATING LOOP"),
        el("span", {}, "原料进来 → 判断出去 → 结果留下")),
      el("div", { class: "loop-track" },
        loopStep("01", "原料", files, "#/assets"),
        el("i", { class: "loop-arrow" }),
        loopStep("02", "热层", hot ? hot.total : null, "#/system?tab=hot"),
        el("i", { class: "loop-arrow" }),
        loopStep("03", "待批", pending, "#/approvals"),
        el("i", { class: "loop-arrow" }),
        loopStep("04", "审计", auditTotal, "#/audit"),
        el("i", { class: "loop-arrow" }),
        loopStep("05", "沉淀", timelineTotal, "#/timeline"))),
    ...(notWiredIndex() === "" ? [] : [notWiredIndex()]));
  return [dashPage];
}
type Company = { name: string; id: string; platform: string; language: string; timezone: string };
type Role = { role: string; title: string; mode: string; model: string; vault_scope: string[] };
type Member = {
  name: string; display_name: string; role: string;
  admin: boolean; disabled: boolean; model: string; domains: string[];
};
type DomainRow = {
  slug: string; name: string; what: string; who: string; who_label: string;
  data: string; sources: string; terms: string;
};
type ProjectRow = {
  slug: string; name: string; domain: string; owner: string; owner_label: string;
  period: string; source: string; charter: string;
};
// 业务 agent（agents.md 一行）。与 Member 是两件事：成员 bot 代人，业务 agent 代岗位 / 流程。
// 字段来自 anc.board/v2；老服务没有这一份时按空数组处理，不当成「没有 agent」下结论。
type AgentRow = {
  slug: string; name: string; description: string; domain: string; role: string;
  harness: string; model: string; tools: string[]; wired: boolean;
};
// 业务 agent 的运行态（`/api/agents`，schema = anc.agents/v1）。
// 它与 AgentRow 是两件事：那份讲**登记了什么**，这份讲**现在谁还在心跳**。
// online 只由心跳推出；last_seen 空 = 从未心跳，不是「离线很久」。
type LiveAgent = {
  slug: string; name: string; domain?: string; domain_name?: string; role?: string;
  harness?: string; model?: string; credentialed: boolean;
  online: boolean; stale: boolean;
  last_seen?: string; age_secs?: number;
  status?: string; task_id?: string; task_title?: string; note?: string;
  session?: string; running_tasks: number; queued_tasks: number;
};
type AgentsView = {
  schema: string; wired: boolean; why?: string; error?: string;
  stale_after_secs: number; total: number; online: number;
  entries: LiveAgent[]; note: string;
};
type RoutingRow = { dir: string; summary: string };
type BoardView = {
  schema: string; generated_at: string; company: Company;
  roles: Role[]; members: Member[]; domains: DomainRow[];
  projects: ProjectRow[]; agents?: AgentRow[]; routing: RoutingRow[];
};
type Finding = { rule: string; level: string; where: string; msg: string };
type Issues = { ok: boolean; fatal: Finding[]; warn: Finding[]; error: string };

// 运行态观测（`/api/runtime`，schema = anc.runtime/v1）—— 它与 anc.board/v1 是**两份**
// 契约：一份讲真相源长什么样，一份讲运行态健不健康。别把两者混着用。
type RuntimeBot = { project: string; state: string; why: string };
type RuntimeView = {
  schema: string; wired: boolean; gateway: string; gateway_why?: string;
  bots: RuntimeBot[]; extras?: string[];
  handlers: string[]; handler_why?: string; error?: string;
};
// 数据流策略面（`/api/dataflow`，schema = anc.dataflow/v1）—— 「谁可以做什么、通道开着吗」。
// 入站只出**人数**，不出标识符：看板是观测面，不是凭据面（SPEC §2.3 / §6-3）。
type DataflowBot = {
  project: string; role: string; mode: string; model: string;
  tools: string[]; inbound: number; inbound_extra: number;
};
type DataflowView = {
  schema: string; wired: boolean;
  relay: { declared: boolean; timeout_secs: number; note: string };
  exec: {
    config_present: boolean; has_fingerprint: boolean;
    version?: string; inputs?: string; generated_at?: string;
    // projects 在「配置读不到」时是 null，不是空数组 —— 这两件事不能混。
    projects: string[] | null; note?: string;
  };
  bots: DataflowBot[]; gaps: string[]; missing: string[];
  error?: string;
};

// 原料清单（`/api/assets`，schema = anc.assets/v1）。这一页给的是**原料**，不是沉淀。
type AssetsFile = { path: string; bytes: number; modified: string };
type AssetsDir = {
  dir: string; summary: string; files: number; bytes: number;
  newest?: string; recent: AssetsFile[]; truncated: boolean;
};
type AssetsView = {
  schema: string; wired: boolean; dirs: AssetsDir[];
  charters: number; note: string; error?: string;
};

// 授权表（`/api/grants`，schema = anc.board.grants/v1）—— 「谁被允许对谁做什么」。
//
// 与「数据流」页的分工：那一页答「通道开着吗」（上游 config 的策略面 + 执行面事实），
// 这一页只答授权表本身。**全量、不筛选**（看板是观测面）—— 授权表里本来就没有凭据
// （SPEC §6：它记的是「谁被允许做什么」），所以这一页也没有 app_id / app_secret。
//
// 主体那一格解的是**编号身份**（成员名 / 业务 agent 的 slug），与「上下文」页同一把尺子：
// 认不出的照原样留着并标出来，不猜。
type GrantIdentity = {
  kind: string; ref: string; name?: string; role?: string;
  domain?: string; domain_name?: string; inscription?: string;
};
type GrantRef = {
  slot: string; raw: string; kind: string; name?: string;
  note?: string; identity?: GrantIdentity;
};
type GrantRow = {
  slug: string; path: string; from: string; to: string; action: string; object: string;
  ttl: string; on_behalf_of: string; reason: string; body: string;
  who: GrantRef[]; hit: boolean;
};
// scope.known=false（带了个认不出的 who）时**不给全量表** —— 那会被读成「他的权限是全部」；
// 也不给一份空表冒充「他什么都没有」，而是照实说认不出。不带 who（ref 为空）= 全量，另一回事。
type GrantScope = { ref: string; known: boolean; who?: GrantIdentity; why?: string };
type GrantsView = {
  schema: string; wired: boolean; total: number; shown: number;
  scope: GrantScope; rows: GrantRow[]; subjects: GrantIdentity[];
  findings: Finding[]; missing: string[]; note: string; error?: string;
};

// 决策与执行的留存（`/api/timeline`，schema = anc.timeline/v1）—— 读者：跟进的人 / 老板。
// 这一页给的是**人和 agent 自己写下来的**记录（`anc timeline add`），不是从会话里推出来的。
// 三档只按 status 配色、形状跟「档」走（实心圆 / 带光环的圆 / 菱形 / 空心虚线圈，见 index.html 的 .dot）；
// **认不出的词原样留着、画空心虚线圈** —— 以后加词只改数据，不用改这里。
type TimelineEntryView = {
  id: string; case?: string; at: string; kind?: string; status?: string;
  by?: string; role?: string; to?: string; domain?: string; project?: string;
  title: string; detail?: string; refs?: string[]; band: string;
};
type TimelineCaseView = {
  key: string; title: string; status: string; band: string;
  first_at: string; last_at: string;
  by?: string; role?: string; to?: string; domain?: string; project?: string;
  entries: number; recent: TimelineEntryView[];
};
type TimelineView = {
  schema: string; wired: boolean;
  counts: { green: number; yellow: number; red: number; unknown: number };
  total: number; entries: number; cases: TimelineCaseView[];
  bad: string[]; limit: number; next_before?: string;
  missing: string[]; note: string; error?: string;
};

// 行使的流水（`/api/audit`，schema = anc.audit/v1）—— 读者：管理者 / 合规感的人。
// 与「时间线」互补：那是**人 / agent 自己写下来的**留存，这是**系统记下来的**流水。
// 事实存当时的样子（object 是原样路径）；zone / cross 是按**当前**域表算出来的（派生）。
type AuditEntryView = {
  id: string; at: string; actor: string; action: string;
  object?: string; result: string; why?: string;
  on_behalf_of?: string; tool?: string; source?: string; ref?: string; detail?: string;
  band: string; actee: string; actee_name: string;
  zone: string; cross: boolean; known: boolean; has_why: boolean;
};
type AuditView = {
  schema: string; wired: boolean;
  bands: { ok: number; denied: number; failed: number; unknown: number };
  scopes: { domain: number; vault: number; outside: number; unknown: number; cross: number };
  total: number; records: AuditEntryView[]; bad: string[]; limit: number;
  missing: string[]; note: string; error?: string;
};

// 待批队列（`/api/approvals`，schema = anc.approvals/v1）—— 读者：该点头的人。
// 这是看板**唯一**能写的一页：点按钮 = 往 write_path 发一次信号，走的是与 CLI 同一个入口、
// 同一份留痕（SPEC §7：CLI / 对话式 bot / 看板是同一条变更管道的三个前端）。
//
// 三条口径（与后端 internal/approvals 是同一份，前端不另立）：
//   - 只认三个字：可点的信号由服务端给（signals[]），前端照画，不自己写死一份；
//   - 不校「点的人是不是该批的人」：by 原样留痕，判断交给 agent（后端也这么收）；
//   - 点头**不写真相源**：grants/ 由人侧的管理者 bot 写，ANC 只留痕。
type ApprovalSignal = { value: string; label: string };
type ApprovalPending = {
  id: string; who: string; to: string; domain: string; project: string;
  title: string; body: string; enqueued: string; status: string;
  // band 是判真那一档：「在队列里」= 有人在等你给一句话（服务端给词，前端照画）。
  // 它与 status 是两回事 —— status 是提案自己写的词，band 是队列这个事实。
  band?: string;
};
type ApprovalView = {
  schema: string; wired: boolean; why?: string; error?: string;
  write_path: string; signals: ApprovalSignal[];
  count: number; pending: ApprovalPending[];
};
// 点头的回执。两个落点是「写到了哪个文件」，不是内容 —— 内容在库里、在审计里。
type DecideReply = {
  ok: boolean; proposal: string; verdict: string; by: string;
  timeline: string; audit: string; note: string;
};

// 一次取数、全程共用：已接入的页吃的是同一份投影，切页不重新打网络（按「刷新」才重取）。
// 热层（`/api/hot`，schema = anc.hot/v1）—— 「热层里现在住着什么」。
// 与 `/api/approvals` 是同一份底层数据、两个问题：那是**队列**（等谁点头），这是**这一层本身**。
// wired=false 与 total=0 是两件事，所以两个字段都在（同 approvals 的口径）。
type HotEntryView = {
  id: string; status: string; title: string; by?: string; to?: string;
  as_of?: string; age_secs?: number; stale: boolean;
  session?: string; intent?: string; version?: number;
};
type HotView = {
  schema: string; wired: boolean; why?: string; error?: string;
  ttl_secs: number; stale_after_secs: number;
  total: number; stale: number; entries: HotEntryView[]; note: string;
};

// 工作上下文（`/api/context`，schema = anc.context/v1）—— 「一个 agent 的工作上下文是什么」。
//
// 字段名与后端（internal/ctxpack）**一字不差**：那一份同时是 agent 那条路上的契约
// （MCP 回给 harness 的就是它）。看板不另立一份字段表 —— 另立一份迟早跟那边对不上号。
type CtxEvidence = { file: string; line: number };
type CtxIdentity = {
  kind: string; ref: string; name?: string; role?: string;
  domain?: string; domain_name?: string; inscription?: string; evidence?: CtxEvidence;
};
type CtxDomain = {
  slug: string; name: string; what: string; data?: string; who: string;
  terms?: string; sources?: string; evidence: CtxEvidence;
};
type CtxBrief = { slug: string; name: string; what: string; who: string };
type CtxSkill = { name: string; where: string; missing?: boolean };
type CtxProject = {
  slug: string; name: string; owner?: string; period?: string; source?: string; evidence: CtxEvidence;
};
// 一件「进行中的事」（反向下发那一步的任务本体，住热层）。字段名与后端 internal/ctxpack.TaskOut
// 一字不差 —— 它同时是 agent 那条路上的契约（MCP 回给 harness 的就是它）。
type CtxTask = {
  id: string; title?: string; status?: string; by?: string; to?: string;
  domain?: string; project?: string; note?: string; as_of?: string;
};
type CtxPack = {
  who?: string; identity?: CtxIdentity; on_behalf_of?: string; scope?: string;
  domains?: CtxDomain[]; directory?: CtxBrief[]; skills?: CtxSkill[]; projects?: CtxProject[];
  todo?: CtxTask[]; doing?: CtxTask[];
  gaps?: string[]; note?: string; refused?: { reason: string; ask?: string }; at: string;
};
type ContextView = {
  schema: string; who: string; domain: string;
  roster: CtxIdentity[]; pack?: CtxPack; error?: string; note: string;
  // tasks_wired / tasks_why：「派给我的待办 / 我在做的」这两摞问出来没有 ——
  // 「热层没接上」与「没人派给你」在这一页上必须是两句话。
  tasks_wired: boolean; tasks_why?: string;
  // 状态 → 配色档。词表在后端（internal/timeline.Band），这一页不另抄一份。
  task_bands?: Record<string, string>;
};

type Data = {
  board: BoardView | null; issues: Issues;
  agents: AgentsView | null; agentsErr: string;
  hot: HotView | null; hotErr: string;
  runtime: RuntimeView | null; runtimeErr: string;
  dataflow: DataflowView | null; dataflowErr: string;
  assets: AssetsView | null; assetsErr: string;
  timeline: TimelineView | null; timelineErr: string;
  audit: AuditView | null; auditErr: string;
  approvals: ApprovalView | null; approvalsErr: string;
  context: ContextView | null; contextErr: string;
  grants: GrantsView | null; grantsErr: string;
  err: string;
};

type Attrs = Record<string, string>;
type Kid = Node | string;

const root = document.getElementById("app");

// ---------- 主题 ----------
// 亮/暗只是同一套语义色的两组取值。选择存在 localStorage；没选过默认浅色。
// 切换后重新挂载一次外壳：Canvas 的颜色不是 CSS 能改的，必须按新主题重画。
type Theme = "light" | "dark";
function currentTheme(): Theme {
  return document.documentElement.dataset.theme === "light" ? "light" : "dark";
}
function applyTheme(t: Theme): void {
  document.documentElement.dataset.theme = t;
  try { localStorage.setItem("anc.theme", t); } catch { /* 隐私模式不记 */ }
  shell();
}

// ---------- 图标：静态内联 SVG，不引外部图标库 ----------
// 只画线条，不加载字体或图片；颜色跟随 currentColor，放在哪个状态里就继承哪个颜色。
const ICONS: Record<string, string> = {
  overview: "M12 3v3M12 18v3M3 12h3M18 12h3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M18.4 5.6l-2.1 2.1M7.7 16.3l-2.1 2.1M12 8a4 4 0 100 8 4 4 0 000-8",
  check: "M4 12l5 5L20 6",
  pulse: "M3 12h4l2-6 4 12 2-6h6",
  flow: "M4 7h9a4 4 0 010 8H4M4 7l3-3M4 7l3 3M20 17h-9a4 4 0 010-8",
  flame: "M12 3c3 4 5 6 5 9a5 5 0 01-10 0c0-2 1-3 2-5 0 2 1 3 2 3 0-3 1-5 1-7z",
  building: "M4 21V8l8-5 8 5v13M9 21v-6h6v6M8 10h.01M12 10h.01M16 10h.01",
  grid: "M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h6v6h-6z",
  users: "M9 11a4 4 0 100-8 4 4 0 000 8zm-7 9a7 7 0 0114 0M17 11a3 3 0 100-6M18 21a6 6 0 00-3-5.2",
  layers: "M12 3l9 5-9 5-9-5 9-5zm-9 9l9 5 9-5M3 16l9 5 9-5",
  context: "M4 5h16v10H4zM8 19h8M12 15v4",
  key: "M14.5 4a5.5 5.5 0 00-3.9 9.4L4 20v-3h3v-3h3l.6-.6A5.5 5.5 0 1014.5 4z",
  clock: "M12 3a9 9 0 100 18 9 9 0 000-18zm0 4v5l3 2",
  alert: "M12 3l9 17H3L12 3zm0 6v5m0 3v.5",
  shield: "M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6l8-3z",
  box: "M12 3l8 4.5v9L12 21l-8-4.5v-9L12 3zm0 0v9m8-4.5l-8 4.5-8-4.5M12 21v-9",
  strata: "M3 6h18M6 12h15M9 18h12",
  graph: "M5.5 6.5a2.5 2.5 0 105 0 2.5 2.5 0 00-5 0zM13.5 17.5a2.5 2.5 0 105 0 2.5 2.5 0 00-5 0zM13.5 6.5a2.5 2.5 0 105 0 2.5 2.5 0 00-5 0zM8 8.6l5.5 6.8M16 9v6",
  gateway: "M3 9h18M3 15h18M8 3v6M16 15v6M12 9v6",
  spark: "M12 3l1.7 5.3L19 10l-5.3 1.7L12 17l-1.7-5.3L5 10l5.3-1.7zM18.5 17.5l.7 2.1 2.1.7-2.1.7-.7 2.1-.7-2.1-2.1-.7 2.1-.7z",
  chevronL: "M15 6l-6 6 6 6",
  link: "M10 13a5 5 0 007.1 0l1.4-1.4a5 5 0 00-7.1-7.1L10.5 5.4M14 11a5 5 0 00-7.1 0l-1.4 1.4a5 5 0 007.1 7.1l.9-.9",
  search: "M10.5 4a6.5 6.5 0 104.4 11.3L20 21M10.5 4a6.5 6.5 0 014.4 11.3",
  menu: "M4 7h16M4 12h16M4 17h16",
  close: "M6 6l12 12M18 6L6 18",
  refresh: "M20 12a8 8 0 11-2.3-5.7M20 4v5h-5",
  copy: "M9 9h10v11H9zM5 15V4h10",
  sun: "M12 8a4 4 0 100 8 4 4 0 000-8M12 2v2M12 20v2M2 12h2M20 12h2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M19.1 4.9l-1.4 1.4M6.3 17.7l-1.4 1.4",
  moon: "M20.2 14.6A8.6 8.6 0 019.4 3.8a7 7 0 108.8 10.8z",
  arrow: "M5 12h14M13 6l6 6-6 6",
  chevron: "M6 9l6 6 6-6",
  dot: "M12 8a4 4 0 100 8 4 4 0 000-8",
};

function icon(name: string, size = 16): SVGSVGElement {
  const ns = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(ns, "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("width", String(size));
  svg.setAttribute("height", String(size));
  svg.setAttribute("aria-hidden", "true");
  const path = document.createElementNS(ns, "path");
  path.setAttribute("d", ICONS[name] ?? ICONS.dot!);
  svg.append(path);
  return svg;
}

function iconBox(name: string, size = 16): HTMLElement {
  return el("span", { class: "nav-icon" }, icon(name, size));
}

// ---------- 画界面的最小零件 ----------

function el(tag: string, attrs: Attrs = {}, ...kids: Kid[]): HTMLElement {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "class") node.className = v;
    else node.setAttribute(k, v);
  }
  for (const kid of kids) node.append(kid);
  return node;
}

const dash = (v: string | null | undefined): string => {
  const s = (v ?? "").trim();
  return s === "" ? "—" : s;
};

const chip = (label: string, mono = false): HTMLElement =>
  el("span", { class: mono ? "chip mono" : "chip" }, label);

function card(title: string, badge: string, rows: Array<[string, Kid]>): HTMLElement {
  const head = el("h3", {}, title);
  if (badge !== "") head.append(el("span", { class: "chip mono" }, badge));
  const dl = el("dl");
  for (const [k, v] of rows) dl.append(el("dt", {}, k), el("dd", {}, v));
  return el("div", { class: "card" }, head, dl);
}

function table(headers: string[], rows: Kid[][]): HTMLElement {
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

function section(title: string, hint: string, ...body: Kid[]): HTMLElement {
  const h = el("h2", {}, title);
  if (hint !== "") h.append(el("span", { class: "hint" }, hint));
  return el("section", { class: "sec" }, h, ...body);
}

function banner(kind: "bad" | "warn", title: string, items: Kid[]): HTMLElement {
  const ul = el("ul");
  for (const i of items) {
    // 调用方用 <br> 分隔「标题句 / 补充句」；它不该变成一个空的项目符号。
    if (i instanceof HTMLElement && i.tagName === "BR") continue;
    ul.append(el("li", {}, i));
  }
  return el("div", { class: "banner " + kind }, el("h3", {}, title), ul);
}

const emptyNote = (s: string): HTMLElement => el("p", { class: "empty" }, s);

// 「本页判断不了 / 不加戏」这类否定断言，单独着色 —— 它是诚实的一部分，不该藏起来。
const cannot = (s: string): HTMLElement => el("span", { class: "cannot" }, s);

// 真源指针只把 http(s) 当链接渲染；别的写法（含 javascript: 之类）一律当纯文字。
function sourceCell(v: string): Kid {
  const s = (v ?? "").trim();
  if (s === "") return "—";
  if (!/^https?:\/\//i.test(s)) return el("span", { class: "mono" }, s);
  return el("a", { href: s, target: "_blank", rel: "noreferrer noopener" }, s);
}

// 证据指针：指回真相源的那一行（vault 相对路径 + 行号）—— 照着能回去核，不是本机路径。
const ptr = (e: CtxEvidence | undefined): Kid =>
  e && (e.file ?? "") !== "" ? el("span", { class: "mono nw" }, e.file + ":" + e.line) : "—";

function findingLine(f: Finding): Kid {
  const where = (f.where ?? "").trim();
  return el("span", {},
    el("span", { class: "mono" }, f.rule), " ",
    where === "" ? f.msg : where + "：" + f.msg);
}

function localTime(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? dash(iso) : d.toLocaleString();
}
// 体积给人看，不给机器看（机器那份在接口里，是字节数）。
function humanBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return "—";
  if (n < 1024) return n + " B";
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + " KB";
  if (n < 1024 * 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + " MB";
  return (n / 1024 / 1024 / 1024).toFixed(1) + " GB";
}

function statusDot(i: Issues): string {
  if (i.error !== "" || i.fatal.length > 0) return "dot bad";
  if (i.warn.length > 0) return "dot warn";
  return "dot ok";
}

function statusText(i: Issues): string {
  if (i.error !== "") return "真相源读不了";
  if (i.fatal.length > 0) return "红档 " + i.fatal.length + " 条";
  if (i.warn.length > 0) return "非红档 " + i.warn.length + " 条";
  return "校验干净";
}

// ---------- 取数 ----------

type Fetched<T> = { status: number; body: T | null; err: string };

async function getJSON<T>(url: string): Promise<Fetched<T>> {
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
async function postJSON<T>(url: string, body: unknown): Promise<Fetched<T>> {
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

const emptyIssues = (err: string): Issues => ({ ok: false, fatal: [], warn: [], error: err });

// 「读到了一份能吃的契约」= body 在、schema 是非空字符串。五份契约（board / runtime /
// dataflow / assets / timeline）各判各的 —— 一份坏了不该让别的页跟着空着。
function contract<T extends { schema?: string }>(res: Fetched<T>, label: string): { v: T | null; err: string } {
  const body = res.body;
  if (body && typeof body.schema === "string" && body.schema !== "") return { v: body, err: "" };
  return {
    v: null,
    err: res.err !== "" ? res.err : label + "读取失败（HTTP " + res.status + "）",
  };
}

async function fetchData(): Promise<Data> {
  const [boardRes, issuesRes, runtimeRes, dataflowRes, assetsRes, timelineRes, auditRes, approvalsRes, hotRes, agentsRes, ctxRes, grantsRes] = await Promise.all([
    getJSON<BoardView & { error?: string }>("/api/board"),
    getJSON<Issues>("/api/issues"),
    getJSON<RuntimeView>("/api/runtime"),
    getJSON<DataflowView>("/api/dataflow"),
    getJSON<AssetsView>("/api/assets"),
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
  ]);
  const issues = issuesRes.body ?? emptyIssues(issuesRes.err);
  const rt = contract(runtimeRes, "运行态");
  const df = contract(dataflowRes, "数据流");
  const as = contract(assetsRes, "原料清单");
  const tl = contract(timelineRes, "时间线");
  const au = contract(auditRes, "审计");
  const ap = contract(approvalsRes, "待批队列");
  const ho = contract(hotRes, "热层");
  const ag = contract(agentsRes, "Agent 在线态");
  const cx = contract(ctxRes, "工作上下文");
  const gr = contract(grantsRes, "授权表");
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
    timeline: tl.v, timelineErr: tl.err,
    audit: au.v, auditErr: au.err,
    approvals: ap.v, approvalsErr: ap.err,
  };
}

// ---------- 可复用的块 ----------

function noBoard(d: Data): Kid {
  return banner("bad", "读不到投影", [
    el("span", {}, d.err === "" ? "服务返回的不是 anc.board 投影" : d.err),
    el("br"),
    el("span", { class: "mono" }, "组织 / 项目 两页都吃这份投影；先修它。"),
  ]);
}

function alertBanners(i: Issues): Kid[] {
  const out: Kid[] = [];
  if (i.fatal.length > 0) {
    out.push(banner("bad", "红档 " + i.fatal.length + " 条 —— 拦住落盘，先修它", i.fatal.map(findingLine)));
  } else if (i.error !== "") {
    out.push(banner("bad", "校验状态读不了", [el("span", {}, i.error)]));
  }
  if (i.warn.length > 0) {
    out.push(banner("warn", "非红档 " + i.warn.length + " 条（不拦，但请过目）", i.warn.map(findingLine)));
  }
  return out;
}

// 第 4 个参数是可选的状态灯：数字旁的形状和下面表里读到的**一模一样** —— 图例跟实际用的是同一套，
// 不另画一套「好看的图例」（形状与含义见 index.html 的 .dot）。
function statCard(n: string, k: string, sub: string, dot?: string): HTMLElement {
  return el("div", { class: "card stat" },
    el("div", { class: "n" }, n),
    el("div", { class: "k" },
      dot ? el("span", { class: "st" }, el("span", { class: dot }), el("span", {}, k)) : k),
    el("div", { class: "sub" }, sub));
}

function statSection(v: BoardView): Kid {
  const active = v.members.filter((m) => !m.disabled).length;
  return section("规模", "全部由真相源现算，不缓存、不猜",
    el("div", { class: "figs" },
      statCard(String(v.domains.length), "业务域", "一块业务一行"),
      statCard(String(v.projects.length), "项目", "在做的事"),
      statCard(String(v.roles.length), "角色", "岗位"),
      statCard(active + "/" + v.members.length, "成员", "启用 / 全部"),
      statCard(String(v.routing.length), "路由目录", "顶层数据目录")));
}

function linkTo(id: string): Kid {
  const p = PAGES.find((x) => x.id === id);
  return el("a", { href: "#/" + id }, p ? p.label : id);
}

// 从 PAGES 现算「哪几页还没接数据源」—— 手抄一份清单，迟早跟导航上说的不一样
// （以前这里写着「两块还没接」，而导航早把那两页标成「已接入」了）。
function notWiredIndex(): Kid {
  const left = PAGES.filter((p) => !p.wired);
  if (left.length === 0) {
    return section("没有还没接数据源的页面", "导航上的「已接入 / 未接入」就是从这张表现算的",
      emptyNote("全部 " + PAGES.length + " 页都接上了。看板照样能显示「还没开始」这个真实状态，但不该再留白页。"));
  }
  const rows: Kid[][] = left.map((p) => [linkTo(p.id), p.question]);
  return section(left.length + " 页还没接数据源", "如实写明缺什么 —— 不编数据填界面",
    table(["页面", "将回答的问题"], rows));
}

function domainSection(v: BoardView): Kid {
  if (v.domains.length === 0) {
    return section("业务域（罗盘）", "",
      emptyNote("还没划域（domains.md 里没有数据行）—— agent 少一条判断口径的依据。"));
  }
  const cards = v.domains.map((d) => card(dash(d.name), dash(d.slug), [
    ["是什么", dash(d.what)],
    ["谁在做", dash(d.who_label)],
    ["数据在哪", dash(d.data)],
    ["原件来源", dash(d.sources)],
    ["口径 / 术语", dash(d.terms)],
  ]));
  return section("业务域（罗盘）", "一块业务一行 —— agent 靠它回答「这是哪块业务、数据在哪、卡住找谁」",
    el("div", { class: "grid" }, ...cards));
}

function projectSection(v: BoardView): Kid {
  if (v.projects.length === 0) {
    return section("项目", "", emptyNote("项目表还是空的（projects.md 没有数据行）—— 由 agent 从立项书抽取后填。"));
  }
  const rows = v.projects.map((p): Kid[] => [
    el("span", {}, dash(p.name), " ", el("span", { class: "chip mono" }, dash(p.slug))),
    dash(p.domain),
    dash(p.owner_label),
    dash(p.period),
    sourceCell(p.source),
    p.charter === "" ? "还没有" : el("span", { class: "mono" }, p.charter),
  ]);
  return section("项目", "一行一个项目；周期是原文照搬，ANC 不解读",
    table(["项目", "挂哪块业务", "卡住找谁", "周期（原文）", "真源", "立项书副本"], rows));
}

function roleSection(v: BoardView): Kid {
  const byRole = new Map<string, Member[]>();
  for (const m of v.members) {
    const list = byRole.get(m.role);
    if (list) list.push(m);
    else byRole.set(m.role, [m]);
  }
  if (v.roles.length === 0) {
    return section("角色与成员", "", emptyNote("还没有角色（roles/ 为空）。"));
  }
  const cards = v.roles.map((r) => {
    const people = (byRole.get(r.role) ?? []).map((m) =>
      dash(m.display_name) + (m.disabled ? "（已停用）" : "") + (m.admin ? "（管理员）" : "")).join("、");
    return card(dash(r.title), r.role, [
      ["模型", dash(r.model)],
      ["模式", dash(r.mode)],
      ["主力目录", r.vault_scope.length > 0 ? r.vault_scope.join("、") : "—"],
      ["成员", people === "" ? "暂无人" : people],
    ]);
  });
  return section("角色与成员", "人名由 members/ 现算 —— 表里只维护岗位，人事变动不用改表",
    el("div", { class: "grid" }, ...cards));
}

function routingSection(v: BoardView): Kid {
  if (v.routing.length === 0) {
    return section("数据路由", "", emptyNote("还没有顶层数据目录。"));
  }
  const rows = v.routing.map((r): Kid[] => [el("span", { class: "mono" }, dash(r.dir)), dash(r.summary)]);
  return section("数据路由", "顶层数据目录 → 说明（persona 段 3 用的就是这张表）",
    table(["目录", "说明"], rows));
}

// ---------- 组织视图的派生 ----------
//
// 这一层只做「连接已有事实」，不造新事实：
//   - 人 → agent：同岗位 / 同业务域，两把尺子都写在标签上；
//   - 人 → 项目：业务域重叠 / 岗位负责人，同样标出来源；
//   - 项目进度：只有周期原文能解析成起止日期时才画一条「时间消耗」，
//     交付进度没有数据源就写「未提供」—— 宁缺，不编。

const agentsOf = (v: BoardView): AgentRow[] => Array.isArray(v.agents) ? v.agents : [];

function roleTitle(v: BoardView, role: string): string {
  const hit = v.roles.find((r) => r.role === role);
  return hit ? dash(hit.title) : dash(role);
}

function domainOf(v: BoardView, slug: string): DomainRow | undefined {
  return v.domains.find((d) => d.slug === slug);
}

function domainLabel(v: BoardView, slug: string): string {
  return domainOf(v, slug)?.name ?? dash(slug);
}

function memberProjects(v: BoardView, m: Member): ProjectRow[] {
  return v.projects.filter((p) => m.domains.includes(p.domain) || (p.owner !== "" && p.owner === m.role));
}

type AgentLink = { agent: AgentRow; why: string };

function agentsForMember(v: BoardView, m: Member): AgentLink[] {
  const out: AgentLink[] = [];
  for (const a of agentsOf(v)) {
    const byRole = a.role !== "" && a.role === m.role;
    const byDomain = m.domains.includes(a.domain);
    if (!byRole && !byDomain) continue;
    out.push({ agent: a, why: byRole && byDomain ? "同岗位 · 同业务域" : byRole ? "同岗位" : "同业务域" });
  }
  return out;
}

function agentsForDomain(v: BoardView, domain: string): AgentRow[] {
  return agentsOf(v).filter((a) => a.domain === domain);
}

// 一个业务 agent 现在在做什么：先看热层（进行中），再看时间线（已留痕的那条 case）。
// 两条都没有就如实说没有 —— 不拿项目名或「在线」冒充正在干活。
function agentWork(d: Data, slug: string): { title: string; status: string; source: string } | null {
  const h = d.hot?.wired ? d.hot.entries.find((e) => e.to === slug || e.by === slug) : undefined;
  if (h) return { title: h.title, status: h.status, source: "热层" };
  const c = d.timeline?.wired ? d.timeline.cases.find((x) => x.to === slug || x.by === slug) : undefined;
  if (c) return { title: c.title, status: c.status, source: "时间线" };
  return null;
}

// 「最近调用」来自审计：它只能说明「最近连过」，不能说明「此刻在跑」。
// 在线判定只看 /api/agents 的心跳 —— 两件事在界面上必须分开写。
function lastCall(d: Data, slug: string): AuditEntryView | null {
  if (!d.audit?.wired) return null;
  return d.audit.records.find((r) => r.actor === slug) ?? null;
}

function recentCallers(d: Data): Set<string> {
  const out = new Set<string>();
  if (!d.audit?.wired) return out;
  const now = Date.now();
  for (const r of d.audit.records) {
    const t = Date.parse(r.at);
    if (!Number.isNaN(t) && now - t <= 24 * 3600 * 1000) out.add(r.actor);
  }
  return out;
}

// 一个 agent 的最后心跳。心跳层没接上、读失败、或根本没这条 → null（不知道，不当离线）。
function liveAgentOf(d: Data, slug: string): LiveAgent | null {
  if (!d.agents?.wired || (d.agents.error ?? "") !== "") return null;
  return d.agents.entries.find((x) => x.slug === slug) ?? null;
}

// 成员名下 agent 的三档口径：接入 = agents.md 有 app_id；在线 = 最近心跳在阈值内；
// 心跳层没接上时 online 是 null —— 「不知道」不当 0。
function memberAgentStats(d: Data, v: BoardView, m: Member): { total: number; wired: number; online: number | null; recent: number } {
  const links = agentsForMember(v, m);
  const wired = links.filter((x) => x.agent.wired).length;
  const seen = recentCallers(d);
  const recent = links.filter((x) => seen.has(x.agent.slug)).length;
  const hbWired = d.agents?.wired === true && (d.agents.error ?? "") === "";
  if (!hbWired) {
    return { total: links.length, wired, online: null, recent };
  }
  const online = links.filter((x) => liveAgentOf(d, x.agent.slug)?.online === true).length;
  return { total: links.length, wired, online, recent };
}

// 项目名 / bot 名由 org 的渲染规则派生：<company id>-<slug|name>。
// 只做「查一下有没有」，查不到就说没观测到，不拿别的 bot 顶上。
function botProject(v: BoardView, ref: string): string {
  const id = (v.company.id ?? "").trim();
  return id === "" ? ref : id + "-" + ref;
}

function flowBotOf(d: Data, project: string): DataflowBot | null {
  return d.dataflow?.bots.find((b) => b.project === project) ?? null;
}

function botStateOf(d: Data, project: string): { cls: string; label: string } {
  if (!d.runtime || !d.runtime.wired) return { cls: "dot gray", label: "运行态未接入" };
  const hit = d.runtime.bots.find((b) => b.project === project);
  if (!hit) return { cls: "dot", label: "未观测到" };
  const st = runState(hit.state);
  return { cls: st.dot, label: st.label };
}

type TaskLine = { title: string; status: string; band: string; at: string; href: string; source: string };

// 「当前任务」有三个来源：热层的当前态、时间线的 case、待批队列。名字对不上就写没有 ——
// 岗位名只在**这个岗位只有一个人**时才当命中，避免把同岗位两个人的事混到一张卡上。
function memberTasks(d: Data, m: Member): TaskLine[] {
  const v = d.board;
  if (!v) return [];
  const names = new Set([m.name, m.display_name].filter((s) => s !== ""));
  const soleRole = v.members.filter((x) => x.role === m.role).length === 1;
  const hit = (s: string | undefined): boolean => {
    if (s === undefined || s === "") return false;
    return names.has(s) || (soleRole && s === m.role);
  };
  const out: TaskLine[] = [];
  if (d.hot?.wired) {
    for (const e of d.hot.entries) {
      if (!hit(e.by) && !hit(e.to)) continue;
      out.push({ title: e.title, status: e.status, band: "yellow", at: e.as_of ?? "", href: "#/system?tab=hot", source: "热层" });
    }
  }
  if (d.timeline?.wired) {
    for (const c of d.timeline.cases) {
      if (!hit(c.by) && !hit(c.to)) continue;
      out.push({ title: c.title, status: c.status, band: c.band, at: c.last_at, href: "#/timeline", source: "时间线" });
    }
  }
  if (d.approvals?.wired) {
    for (const p of d.approvals.pending) {
      if (!hit(p.who) && !hit(p.to)) continue;
      out.push({ title: p.title, status: p.status, band: p.band ?? "yellow", at: p.enqueued, href: "#/approvals", source: "待批" });
    }
  }
  return out;
}

type DateWindow = { start: number; end: number };

// 只认「两个 YYYY-MM-DD + 一个分隔符」这一种写法。认不出就返回 null，
// 由调用方原样显示周期文本 —— 不把自由文本硬解释成日期。
function periodWindow(period: string): DateWindow | null {
  const m = /(\d{4})-(\d{2})-(\d{2})\s*(?:→|->|~|—|–|至|到)\s*(\d{4})-(\d{2})-(\d{2})/.exec(period);
  if (!m) return null;
  const start = new Date(m[1] + "-" + m[2] + "-" + m[3] + "T00:00:00").getTime();
  const end = new Date(m[4] + "-" + m[5] + "-" + m[6] + "T23:59:59").getTime();
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return null;
  return { start, end };
}

function periodState(period: string): { pct: number; note: string; cls: string } | null {
  const w = periodWindow(period);
  if (!w) return null;
  const now = Date.now();
  const pct = Math.max(0, Math.min(100, ((now - w.start) / (w.end - w.start)) * 100));
  const note = now < w.start ? "周期还没开始" : now > w.end ? "周期已结束" : "周期进行中";
  return { pct, note, cls: now > w.end ? "over" : now < w.start ? "idle" : "live" };
}

// 授权表里命中几个主体。-1 = 授权表没接入（与「0 条」不是一回事）。
function grantsFor(d: Data, refs: string[]): number {
  if (!d.grants || !d.grants.wired) return -1;
  const set = new Set(refs.filter((x) => x !== ""));
  return d.grants.rows.filter((r) => r.who.some((w) =>
    set.has(w.raw) || (w.name !== undefined && set.has(w.name)) || (w.identity?.ref !== undefined && set.has(w.identity.ref)))).length;
}

function dcard(title: string, badge: string, ...body: Kid[]): HTMLElement {
  const h = el("h3", {}, title);
  if (badge !== "") h.append(el("span", { class: "chip mono" }, badge));
  return el("section", { class: "dcard" }, h, ...body);
}

function kv(rows: Array<[string, Kid]>): HTMLElement {
  const dl = el("dl");
  for (const [k, v] of rows) dl.append(el("dt", {}, k), el("dd", {}, v));
  return dl;
}

// ---------- 页面 ----------
//
// 一页一个 Page：`id` 是 hash 路由，`question` 是这一页**唯一**要回答的问题。
// 加一页＝加一条；先问自己那句 question 写不写得出来，写不出来说明还没想清楚。

type Page = {
  id: string;
  label: string;
  icon: string;
  group: string;   // 导航里的分组：组的标题是**问题**（现在/组织/记录/运行/原料），不是名词
  question: string;
  sub?: string;    // 页头第二行：这页读什么、边界在哪。缺省就用 question。
  wired: boolean;
  view: (d: Data) => Kid[];
};

// 总览 —— 读者：所有人 / 老板。要的是「30 秒看完有没有事」，不是名词表。
//
// 这一页是仪表盘，不是把其它页的表搬过来：只放「现在要不要人动手」的四类事实
// （校验、待批、运行、最近活动），每一项都能点进对应的细页。没有数据源就写
// 「未接入」，不画一个看起来热闹的空图。
function healthState(i: Issues): "ok" | "warn" | "bad" {
  if (i.error !== "" || i.fatal.length > 0) return "bad";
  if (i.warn.length > 0) return "warn";
  return "ok";
}

function healthWord(i: Issues): string {
  if (i.error !== "") return "读不到真相源";
  if (i.fatal.length > 0) return "有红档";
  if (i.warn.length > 0) return "需过目";
  return "校验干净";
}

function heroKpi(label: string, value: string, sub: string): HTMLElement {
  return el("div", { class: "kpi" },
    el("span", {}, label),
    el("strong", {}, value, sub === "" ? "" : el("small", {}, " " + sub)));
}

function healthRow(dot: string, count: string, label: string, why: string): HTMLElement {
  return el("div", { class: "health-row" },
    el("span", { class: dot }), el("b", {}, count), el("span", { class: "lab" }, label),
    el("span", { class: "why" }, why));
}

function feedRow(dot: string, title: string, sub: string, time: string, href: string): HTMLElement {
  return el("a", { class: "feed-row", href: href },
    el("span", { class: dot }),
    el("span", { class: "ft" }, el("b", {}, title), el("small", {}, sub)),
    el("span", { class: "tm" }, time));
}

function botRow(b: RuntimeBot): HTMLElement {
  const st = runState(b.state);
  return el("div", { class: "bot-row", title: b.why },
    el("span", { class: st.dot }), el("span", { class: "bn" }, b.project),
    el("span", { class: "bw" }, st.label.split(" · ")[0] ?? ""));
}

function bentoCard(title: string, hint: string, span: string, more: [string, string] | null, ...body: Kid[]): HTMLElement {
  const head = el("div", { class: "card-head" },
    el("div", {}, el("h3", {}, title), el("div", { class: "hint" }, hint)),
    more === null ? "" : el("a", { class: "more", href: more[0] }, more[1] + "  ›"));
  return el("section", { class: "glass bento-card " + span }, head, el("div", { class: "grow" }, ...body));
}

function timelineRecent(d: Data, n: number): TimelineEntryView[] {
  const v = d.timeline;
  if (!v || !v.wired) return [];
  const rows: TimelineEntryView[] = [];
  for (const c of v.cases) for (const e of c.recent) rows.push(e);
  rows.sort((a, b) => (a.at < b.at ? 1 : a.at > b.at ? -1 : 0));
  return rows.slice(0, n);
}

// 组织 —— 读者：管理者 / FDE。组织长什么样、边界在哪。
function renderOrg(d: Data): Kid[] {
  const v = d.board;
  if (!v) return [noBoard(d)];
  return [domainSection(v), roleSection(v), routingSection(v)];
}

// 组织总览 —— 只放「公司是什么、数据从哪走」；域、角色、项目各自单独一页。
function renderOrgOverview(d: Data): Kid[] {
  const v = d.board;
  if (!v) return [noBoard(d)];
  const c = v.company;
  return [
    section("公司", "投影里的公司档案：字段来自真相源，不在前端拼",
      el("div", { class: "grid" },
        card(dash(c.name), dash(c.id), [
          ["平台", dash(c.platform)],
          ["语言", dash(c.language)],
          ["时区", dash(c.timezone)],
          ["schema", el("span", { class: "mono" }, dash(v.schema))],
        ]),
        card("数据路由", "顶层目录 → 说明", v.routing.length === 0
          ? [["状态", cannot("还没有顶层数据目录")]]
          : v.routing.slice(0, 5).map((r): [string, Kid] => [r.dir, dash(r.summary)])))),
    statSection(v),
    routingSection(v),
  ];
}

// 业务域 —— 从「组织」里拆出来：一块业务一行，回答「这是哪块业务、数据在哪、卡住找谁」。
function renderDomains(d: Data): Kid[] {
  const v = d.board;
  if (!v) return [noBoard(d)];
  return [
    domainSection(v),
    section("这一页只答什么", "域是罗盘，不是项目清单",
      emptyNote("项目、角色、授权各自另有页。这里只回答「一块业务是什么、谁在做、数据在哪」。")),
  ];
}

// 成员与 Agent —— 一人一张卡。卡片只摆「能对上号」的关系：
// 同岗位 / 同业务域的 agent 标出来源；任务只从时间线与待批队列取。
function memberCard(d: Data, v: BoardView, m: Member, badge?: string): HTMLElement {
  const links = agentsForMember(v, m);
  const tasks = memberTasks(d, m);
  const stats = memberAgentStats(d, v, m);
  const bot = botStateOf(d, botProject(v, m.name));
  const state = m.disabled ? "已停用" : m.admin ? "管理员" : "启用";
  const mark = (m.display_name || m.name).trim().slice(0, 1).toUpperCase() || "?";
  const node = el("article", {
    class: "entity-card member-card" + (m.disabled ? " is-off" : ""),
    role: "button", tabindex: "0", "aria-label": "打开 " + dash(m.display_name || m.name) + " 的详情",
  },
    el("header", { class: "ec-head" },
      el("span", { class: "avatar" }, mark),
      el("div", { class: "ec-id" },
        el("h3", {}, dash(m.display_name || m.name)),
        el("p", {}, roleTitle(v, m.role), el("span", { class: "dot-sep" }, "·"), el("span", { class: "mono" }, dash(m.name)))),
      el("span", { class: "ec-state" + (m.disabled ? " off" : "") }, state)),
    el("div", { class: "ec-tags" },
      ...(badge === undefined ? [] : [el("span", { class: "ec-tag on" }, badge)]),
      ...(m.domains.length === 0
        ? [el("span", { class: "ec-tag muted" }, "未挂业务域")]
        : m.domains.map((x) => el("span", { class: "ec-tag" }, domainLabel(v, x))))),
    el("p", { class: "ec-line" }, links.length === 0
      ? "没有挂得上的人 ↔ agent 关系"
      : links.slice(0, 3).map((x) => dash(x.agent.name || x.agent.slug) + "（" + x.why + "）").join("、")
        + (links.length > 3 ? " 等 " + links.length + " 个" : "")),
    el("div", { class: "ec-foot" },
      el("span", {}, "Agent ", el("b", {}, String(stats.total))),
      el("span", {}, "接入 ", el("b", {}, String(stats.wired))),
      el("span", {}, "在线 ", el("b", {}, stats.online === null ? "—" : String(stats.online))),
      el("span", {}, "近期 ", el("b", {}, String(stats.recent))),
      el("span", {}, "任务 ", el("b", {}, String(tasks.length))),
      el("span", { class: "ec-run" }, el("span", { class: bot.cls }), bot.label)));
  const open = (): void => openMemberDrawer(d, v, m);
  node.addEventListener("click", open);
  node.addEventListener("keydown", (e) => {
    if (e.key === "Enter" || e.key === " ") { e.preventDefault(); open(); }
  });
  return node;
}

function openMemberDrawer(d: Data, v: BoardView, m: Member): void {
  const links = agentsForMember(v, m);
  const tasks = memberTasks(d, m);
  const stats = memberAgentStats(d, v, m);
  const projects = memberProjects(v, m);
  const refs = [m.name, ...links.map((x) => x.agent.slug)];
  const grants = grantsFor(d, refs);
  const bot = botStateOf(d, botProject(v, m.name));
  const doms = m.domains.map((x) => domainOf(v, x)).filter((x): x is DomainRow => x !== undefined);
  openDrawer({
    eyebrow: "MEMBER / 成员与 Agent",
    title: dash(m.display_name || m.name),
    sub: roleTitle(v, m.role) + " · " + (m.disabled ? "已停用" : "启用") + (m.admin ? " · 管理员" : ""),
    body: [
      el("div", { class: "drawer-grid" },
        el("div", { class: "dg-col" },
          dcard("身份与归属", "IDENTITY",
            kv([
              ["显示名", dash(m.display_name)],
              ["登录名", el("span", { class: "mono" }, dash(m.name))],
              ["岗位", roleTitle(v, m.role) + "（" + dash(m.role) + "）"],
              ["模型", dash(m.model)],
              ["状态", m.disabled ? "已停用" : m.admin ? "启用 · 管理员" : "启用"],
            ]),
            el("p", { class: "dnote" }, "成员 bot 运行态：", el("span", { class: bot.cls }), " " + bot.label,
              el("span", { class: "mono" }, " · " + botProject(v, m.name)))),
          dcard("业务范围", "SCOPE",
            doms.length === 0
              ? el("p", { class: "empty" }, "没挂业务域。")
              : el("div", { class: "dlist" }, ...doms.map((x) =>
                  el("div", { class: "drow" },
                    el("span", { class: "dot gold" }),
                    el("span", { class: "drow-main" }, el("b", {}, dash(x.name)), el("small", {}, dash(x.what)))))),
            projects.length === 0 ? "" : el("p", { class: "dnote" }, "相关项目（按业务域 / 岗位负责人推断）：", projects.map((p) => p.name).join("、"))),
          dcard("当前任务", tasks.length === 0 ? "0" : String(tasks.length),
            tasks.length === 0
              ? el("p", { class: "empty" }, "没有以本人姓名指派的任务（热层 by/to、时间线、待批都没命中）；名下 Agent 正在做的事见右侧「我的 Agent」。")
              : el("div", { class: "dlist" }, ...tasks.slice(0, 8).map((t) =>
                  el("a", { class: "drow", href: t.href },
                    el("span", { class: bandDot(t.band) }),
                    el("span", { class: "drow-main" }, el("b", {}, t.title), el("small", {}, t.source + " · " + dash(t.status))),
                    el("span", { class: "drow-time" }, sinceText(t.at)))))),
        ),
        el("div", { class: "dg-col" },
          dcard("我的 Agent", stats.online === null
              ? "接入 " + String(stats.wired) + " / 共 " + String(stats.total) + " · 在线未知（心跳未接） · 24h 内调用 " + String(stats.recent)
              : "在线 " + String(stats.online) + " / 共 " + String(stats.total) + " · 接入 " + String(stats.wired) + " · 24h 内调用 " + String(stats.recent),
            links.length === 0
              ? el("p", { class: "empty" }, "按「同岗位 / 同业务域」都挂不上 agent。")
              : el("div", { class: "dlist" }, ...links.map((x) => {
                  const st = botStateOf(d, botProject(v, x.agent.slug));
                  const live = liveAgentOf(d, x.agent.slug);
                  const work = agentWork(d, x.agent.slug);
                  const call = lastCall(d, x.agent.slug);
                  const project = v.projects.find((p) => p.domain === x.agent.domain);
                  const heartbeat = live === null
                    ? "心跳状态未知"
                    : live.online
                      ? "心跳在线"
                      : (live.last_seen ?? "") === ""
                        ? "从未心跳"
                        : "心跳离线（最后 " + sinceText(live.last_seen ?? "") + "）";
                  const doing = live?.task_title
                    ? " · 任务：" + live.task_title
                    : (live?.task_id ?? "") !== ""
                      ? " · 任务：" + live?.task_id
                      : "";
                  const queue = live === null || (live.running_tasks + live.queued_tasks) === 0
                    ? ""
                    : " · 队列 " + live.running_tasks + " 跑 / " + live.queued_tasks + " 等";
                  const sess = (live?.session ?? "") === "" ? "" : " · 会话 " + live?.session;
                  return el("div", { class: "drow" },
                    el("span", { class: live?.online ? "dot ok" : "dot gray" }),
                    el("span", { class: "drow-main" },
                      el("b", {}, dash(x.agent.name || x.agent.slug) + " · " + dash(x.agent.harness || "默认 harness")),
                      el("small", { class: "wrap" }, dash(x.agent.slug)
                        + " · " + (x.agent.wired ? "已接入" : "未接凭据")
                        + " · " + heartbeat
                        + ((live?.status ?? "") === "" ? "" : " · " + live?.status)
                        + doing
                        + queue
                        + sess
                        + " · 运行态 " + st.label
                        + " · " + (call === null ? "无调用记录" : "最近调用 " + sinceText(call.at))),
                      ((live?.note ?? "") === "" ? "" : el("small", { class: "wrap" }, "自报：" + live?.note)),
                      el("small", { class: "wrap" },
                        work === null ? "热层 / 时间线里没有正在做的任务" : "正在做：" + work.title + "（" + work.source + " / " + work.status + "）"),
                      project === undefined ? "" : el("small", { class: "wrap" }, "服务项目：" + dash(project.name))),
                    el("span", { class: "chip mono" }, x.why));
                }))),
          dcard("汇报与指向", "NO FAKE HIERARCHY",
            kv([
              ["上级", cannot("未提供 —— org 里还没有 reports_to 字段")],
              ["相关项目", projects.length === 0 ? "—" : projects.map((p) => p.name).join("、") + "（推断）"],
              ["数据指向", doms.length === 0 ? "—" : doms.map((x) => dash(x.data)).join("、")],
            ])),
          dcard("授权摘要", "GRANTS",
            grants < 0
              ? el("p", { class: "empty" }, "授权表没接入。")
              : el("p", {}, "命中 " + String(grants) + " 条。",
                  grants === 0 ? cannot(" —— 没有 grant 就是不通（SPEC §6 不变量 1）。") : ""))),
      ),
    ],
  });
}

function renderMembers(d: Data): Kid[] {
  const v = d.board;
  if (!v) return [noBoard(d)];
  const people = v.members.slice().sort((a, b) =>
    Number(a.disabled) - Number(b.disabled) || (a.display_name || a.name).localeCompare(b.display_name || b.name, "zh"));
  const vac = v.roles.filter((r) => !v.members.some((m) => m.role === r.role && !m.disabled));
  const orphans = agentsOf(v).filter((a) =>
    !v.members.some((m) => !m.disabled && agentsForMember(v, m).some((l) => l.agent.slug === a.slug)));
  // 「我的卡片」不猜登录态：URL 上的 who 优先，没写就取第一个启用中的管理员。
  // 它只决定以谁的视角看，不改任何权限 —— 看板不做登录校验。
  const want = (hashQuery().get("who") ?? "").trim();
  const me = people.find((m) => m.name === want || (m.display_name || "") === want)
    ?? people.find((m) => m.admin && !m.disabled && m.role === "fde-lead")
    ?? people.find((m) => m.admin && !m.disabled)
    ?? people[0];
  const nameCount = new Map<string, number>();
  for (const m of people) {
    const key = m.display_name || m.name;
    nameCount.set(key, (nameCount.get(key) ?? 0) + 1);
  }
  const chipLabel = (m: Member): string =>
    (nameCount.get(m.display_name || m.name) ?? 0) > 1
      ? (m.display_name || m.name) + " · " + m.name
      : (m.display_name || m.name);
  const openWho = (hashQuery().get("open") ?? "").trim();
  if (openWho !== "") {
    const target = people.find((m) => m.name === openWho);
    if (target !== undefined) setTimeout(() => openMemberDrawer(d, v, target), 0);
  }
  return [
    me === undefined ? "" : section("我的卡片", "看板不做登录：这里只决定「以谁的视角看」，不改任何权限",
      el("div", { class: "ec-tags" }, ...people.map((m) =>
        el("a", { class: "ec-tag" + (m === me ? " on" : ""), href: "#/members?who=" + encodeURIComponent(m.name) },
          dash(chipLabel(m))))),
      el("div", { class: "entity-grid", style: "margin-top:12px" }, memberCard(d, v, me, "我的视角"))),
    section("成员管理", "一人一张卡；点开看身份、名下 Agent 是否在线、在做什么、以及当前任务",
      people.length === 0
        ? emptyNote("还没有成员（members/ 为空）。")
        : el("div", { class: "entity-grid" }, ...people.map((m) => memberCard(d, v, m)))),
    vac.length === 0 ? "" : section("还空着的岗位", "roles/ 里有岗位，但 members/ 里没有人",
      el("div", { class: "ec-tags" }, ...vac.map((r) =>
        el("span", { class: "ec-tag" }, dash(r.title) + " · ", el("span", { class: "mono" }, r.role))))),
    orphans.length === 0 ? "" : section("未落到成员身上的业务 Agent", "按同岗位 / 同业务域都挂不上 —— 不是错误，但需要有人认领",
      el("div", { class: "dlist" }, ...orphans.map((a) =>
        el("div", { class: "drow" },
          el("span", { class: "dot gold" }),
          el("span", { class: "drow-main" }, el("b", {}, dash(a.name || a.slug)), el("small", {}, dash(a.description))),
          el("span", { class: "chip mono" }, "域 " + dash(a.domain)))))),
    section("这一页怎么读", "关系是派生的，不是编的",
      el("div", { class: "grid" },
        card("人 → Agent", "两把尺子", [["同岗位", "agent.role = 成员岗位"], ["同业务域", "agent.domain ∈ 成员业务域"]]),
        card("当前任务", "三个来源", [["热层", "进行中的 by / to 命中姓名"], ["时间线", "case 的 by / to 命中姓名"], ["待批", "队列里的 who / to 命中"]]),
        card("没有的东西", "明确留白", [["上级", "org 还没有 reports_to 字段"], ["交付进度", "没有进度真相源"]]))),
  ];
}

// 项目 —— 读者：跟进的人 / 老板。谁在做什么、到几号、卡住找谁。
// 进度拆成两件事：周期时间消耗（可由原文日期推导）与交付进度（没有真相源）。
function projectCard(d: Data, v: BoardView, p: ProjectRow): HTMLElement {
  const pct = periodState(p.period);
  const agents = agentsForDomain(v, p.domain);
  const recent = d.timeline?.wired ? d.timeline.cases.filter((c) => c.project === p.slug).length : -1;
  const node = el("article", {
    class: "entity-card project-card",
    role: "button", tabindex: "0", "aria-label": "打开项目 " + dash(p.name) + " 的详情",
  },
    el("header", { class: "ec-head" },
      el("span", { class: "avatar project" }, "项"),
      el("div", { class: "ec-id" },
        el("h3", {}, dash(p.name)),
        el("p", {}, domainLabel(v, p.domain), el("span", { class: "dot-sep" }, "·"), el("span", { class: "mono" }, dash(p.slug))))),
    el("div", { class: "ec-tags" },
      el("span", { class: "ec-tag" }, "负责人 " + dash(p.owner_label)),
      el("span", { class: "ec-tag" }, "周期 " + dash(p.period)),
      ...(p.charter === "" ? [] : [el("span", { class: "ec-tag" }, "立项书副本")])),
    pct
      ? el("div", { class: "meter " + pct.cls },
          el("div", { class: "meter-head" }, el("span", {}, "周期时间进度"), el("b", {}, String(Math.round(pct.pct)) + "%")),
          el("div", { class: "track" }, el("i", { style: "width:" + pct.pct.toFixed(1) + "%" })),
          el("p", { class: "meter-note" }, pct.note + " · 按周期起止推算，只表示时间消耗，不是交付进度。"))
      : el("p", { class: "meter-note" }, "周期是自由文本，解析不出起止日期 —— 原样显示，不画进度条。"),
    el("div", { class: "ec-foot" },
      el("span", {}, "交付进度 ", el("b", { class: "muted" }, "未提供")),
      el("span", {}, "Agent ", el("b", {}, String(agents.length))),
      el("span", {}, "记录 ", recent < 0 ? "—" : String(recent))));
  const open = (): void => openProjectDrawer(d, v, p);
  node.addEventListener("click", open);
  node.addEventListener("keydown", (e) => {
    if (e.key === "Enter" || e.key === " ") { e.preventDefault(); open(); }
  });
  return node;
}

function openProjectDrawer(d: Data, v: BoardView, p: ProjectRow): void {
  const pct = periodState(p.period);
  const agents = agentsForDomain(v, p.domain);
  const cases = d.timeline?.wired ? d.timeline.cases.filter((c) => c.project === p.slug) : [];
  const grants = grantsFor(d, agents.map((a) => a.slug));
  openDrawer({
    eyebrow: "PROJECT / 项目",
    title: dash(p.name),
    sub: dash(p.slug) + " · " + domainLabel(v, p.domain) + " · " + dash(p.owner_label),
    body: [
      el("div", { class: "drawer-grid" },
        el("div", { class: "dg-col" },
          dcard("项目档案", "DOSSIER",
            kv([
              ["挂哪块业务", domainLabel(v, p.domain) + "（" + dash(p.domain) + "）"],
              ["卡住找谁", dash(p.owner_label)],
              ["周期（原文）", dash(p.period)],
              ["真源", sourceCell(p.source)],
              ["立项书副本", p.charter === "" ? cannot("还没有副本") : el("span", { class: "mono" }, p.charter)],
            ])),
          dcard("进度", "TWO KINDS",
            pct
              ? el("div", { class: "meter " + pct.cls },
                  el("div", { class: "meter-head" }, el("span", {}, "周期时间消耗"), el("b", {}, String(Math.round(pct.pct)) + "%")),
                  el("div", { class: "track" }, el("i", { style: "width:" + pct.pct.toFixed(1) + "%" })),
                  el("p", { class: "meter-note" }, pct.note + "。"))
              : el("p", { class: "empty" }, "周期原文解析不出起止日期，所以不画时间条。"),
            el("p", { class: "dnote" }, "交付进度：", cannot("未提供 —— 项目表与立项书里没有机器可读的进度字段。"),
              " 这一条不能拿时间消耗冒充。"))),
        el("div", { class: "dg-col" },
          dcard("服务这个项目的 Agent", String(agents.length),
            agents.length === 0
              ? el("p", { class: "empty" }, "没有业务 agent 挂在这个业务域上。")
              : el("div", { class: "dlist" }, ...agents.map((a) => {
                  const st = botStateOf(d, botProject(v, a.slug));
                  const flow = flowBotOf(d, botProject(v, a.slug));
                  return el("div", { class: "drow" },
                    el("span", { class: st.cls }),
                    el("span", { class: "drow-main" },
                      el("b", {}, dash(a.name || a.slug)),
                      el("small", {}, dash(a.description) + (flow ? " · 入站 " + String(flow.inbound) + " 人" : ""))),
                    el("span", { class: "drow-time" }, st.label));
                })),
            grants < 0 ? "" : el("p", { class: "dnote" }, "这些 agent 在授权表里命中 " + String(grants) + " 条。")),
          dcard("最近记录", cases.length === 0 ? "0" : String(cases.length),
            cases.length === 0
              ? el("p", { class: "empty" }, "时间线里没有这个项目的 case。")
              : el("div", { class: "dlist" }, ...cases.slice(0, 8).map((c) =>
                  el("a", { class: "drow", href: "#/timeline" },
                    el("span", { class: bandDot(c.band) }),
                    el("span", { class: "drow-main" }, el("b", {}, c.title), el("small", {}, dash(c.status))),
                    el("span", { class: "drow-time" }, sinceText(c.last_at)))))),
          dcard("交付进度为什么空着", "GAP",
            el("p", { class: "empty" }, "「进度」每天变：写进 git 真相源＝每天刷 diff、人手维护必腐烂。" +
              "候选：① 表里留一个指针、渲染时聚合；② 交给 agent 在数据目录里写；③ 另开一张动态表、不进 git。" +
              "拍板前这里留白 —— 见 SPEC §13 Q15 与 GitHub #36。"))),
      ),
    ],
  });
}

function openAgentDrawer(d: Data, v: BoardView, a: AgentRow): void {
  const st = botStateOf(d, botProject(v, a.slug));
  const flow = flowBotOf(d, botProject(v, a.slug));
  const grants = grantsFor(d, [a.slug]);
  openDrawer({
    eyebrow: "AGENT / 业务 Agent",
    title: dash(a.name || a.slug),
    sub: dash(a.slug) + " · " + domainLabel(v, a.domain) + " · " + roleTitle(v, a.role),
    body: [
      el("div", { class: "drawer-grid" },
        el("div", { class: "dg-col" },
          dcard("身份与铭文", "INSCRIPTION",
            kv([
              ["slug", el("span", { class: "mono" }, dash(a.slug))],
              ["铭文", dash(a.description)],
              ["挂哪块业务", domainLabel(v, a.domain) + "（" + dash(a.domain) + "）"],
              ["代哪个岗位", roleTitle(v, a.role) + "（" + dash(a.role) + "）"],
            ]))),
        el("div", { class: "dg-col" },
          dcard("接入与运行", "RUNTIME",
            kv([
              ["平台绑定", a.wired ? "已声明 app_id" : cannot("还没有 app_id —— 按未接线处理")],
              ["运行态", st.label],
              ["harness", dash(a.harness)],
              ["模型", a.model === "" ? "走公司 / 岗位默认" : a.model],
              ["工具", a.tools.length === 0 ? cannot("空（没预授权）") : el("span", { class: "mono" }, a.tools.join(", "))],
              ["入站授权", flow ? String(flow.inbound) + " 人" : "未在数据流里读到"],
              ["授权表", grants < 0 ? "未接入" : String(grants) + " 条"],
            ]),
            el("p", { class: "dnote" }, "project：", el("span", { class: "mono" }, botProject(v, a.slug))))),
      ),
    ],
  });
}

function renderProjects(d: Data): Kid[] {
  const v = d.board;
  if (!v) return [noBoard(d)];
  return [
    section("全部项目", "一张卡一个项目；进度拆成「周期时间消耗」与「交付进度」两件事",
      v.projects.length === 0
        ? emptyNote("项目表还是空的（projects.md 没有数据行）—— 由 agent 从立项书抽取后填。")
        : el("div", { class: "entity-grid" }, ...v.projects.map((p) => projectCard(d, v, p)))),
    section("进度的口径", "两个数字不能混",
      el("div", { class: "grid" },
        card("周期时间消耗", "可推导", [["来源", "projects.md 的 period 原文"], ["算法", "起止日期之间消耗的比例"], ["含义", "只说明时间过去了多少"]]),
        card("交付进度", "未提供", [["来源", "没有机器可读的进度字段"], ["处理", "不画百分比、不拿时间冒充"], ["缺口", "见 #36 / SPEC §13 Q15"]]))),
  ];
}

// 运行态四档口径（拍板）：绿 = 近期真回过话；黄 = 运行中 / 这个窗口没观测到；红 = 卡点，需人介入；
// 灰 = 真相源里**声明**了「还没接凭据」—— 本来就不该期待它回话，单列一档，
// 免得跟黄糊在一起（混着看，整份报告都会不值得信）。
const RUN_STATE: Record<string, { dot: string; label: string }> = {
  ok: { dot: "dot ok", label: "绿 · 近期回过话" },
  warn: { dot: "dot warn", label: "黄 · 运行中 / 未观测" },
  fail: { dot: "dot bad", label: "红 · 卡点，需介入" },
  // 未接凭据：**本来就不该期待它回话**。用灰**实心**，与「认不出的词」那个空心虚线圈分开 ——
  // 一个说「别催它」，一个说「去判它」（#59：灰原来把两种意思叠在一个符号上）。
  unwired: { dot: "dot gray-solid", label: "灰实心 · 未接凭据（本来就不该期待）" },
};
const runState = (s: string) => RUN_STATE[s] ?? { dot: "dot", label: dash(s) };

// 带颜色的状态点：点 + 文字放进一个 inline-flex，点才有宽度（.dot 是固定宽高）。
const dotLine = (cls: string, ...kids: Kid[]): HTMLElement =>
  el("span", { class: "st" }, el("span", { class: cls }), ...kids);

// 运行态 —— 读者：运维 / FDE。数据源是防假绿探针（与 `anc probe` **同一份判据**，只读磁盘）。
//
// 它的纪律：判活靠**真拨 socket**、判定靠**真回过话**，都不看「进程在不在」——
// 进程活着 / engine started 打着绿字 / 却一条也回不了，这个坑真踩过两次。
// 「现在这一秒能不能回话」的裁判权在**人**：这一页只观测、只报红，不做任何自动处置。
function renderRuntime(d: Data): Kid[] {
  const r = d.runtime;
  if (!r) {
    return [banner("bad", "读不到运行态", [
      el("span", {}, d.runtimeErr === "" ? "服务没回 anc.runtime/v1" : d.runtimeErr),
    ])];
  }
  if (!r.wired) {
    return [banner("warn", "运行态还没接上", [
      el("span", {}, dash(r.error)),
      el("br"),
      el("span", { class: "mono" }, "anc board serve <vault> --data <data 目录>"),
    ])];
  }

  const out: Kid[] = [];
  if ((r.error ?? "") !== "") out.push(banner("bad", "探针跑不动", [el("span", {}, dash(r.error))]));

  const up = r.gateway === "up";
  out.push(section("gateway", "判活 = 真拨一次 socket，不看文件在不在（残留文件会把「挂了」看成「在跑」）",
    el("p", { class: "empty" },
      dotLine(up ? "dot ok" : "dot bad",
        up ? " 在跑（socket 拨得通）" : " 没在跑：" + dash(r.gateway_why)))));

  const rows: Kid[][] = r.bots.map((b) => {
    const st = runState(b.state);
    return [el("span", { class: st.dot }), el("span", { class: "mono" }, dash(b.project)), st.label, b.why];
  });
  out.push(section("每个 bot 能不能回话", "只读磁盘事实（socket + 会话记录），不烧 token",
    r.bots.length === 0
      ? emptyNote("config 里一个 project 都没有 —— 没有可观测的对象。")
      : table(["", "bot", "状态", "依据"], rows)));

  out.push(section("出事了交给谁", "探针不做自动处置 —— 谁去处理、怎么处理，由人定",
    el("p", { class: "empty" },
      r.handlers.length > 0
        ? el("span", {}, "报红交给：", el("strong", {}, r.handlers.join("、")), "（公司 admins）")
        : cannot(dash(r.handler_why)))));

  const extras = r.extras ?? [];
  if (extras.length > 0) {
    out.push(banner("warn", "配置外的残留（" + extras.length + " 条，不是运行态问题，但该清）",
      extras.map((e) => el("span", {}, e))));
  }

  out.push(section("这一页不做什么", "说清楚边界，比多画几个卡片有用",
    el("p", { class: "empty" },
      "不发消息去试（那要烧 token，裁判权也留给人）；不起服务、不重启进程、不清残留 —— ",
      "这里全是只读观测。「现在能不能回话」由人下判断，这一页只保证不给他一个假的绿灯。")));
  return out;
}

// 数据流 —— 读者：管理者 / 老板。问题句只有「通道开着吗 / 每个 bot 手里有什么」两件 ——
//   ① 执行面（gateway 实际吃的那份 config.toml）里通道的口径 —— 上游默认开着，所以「没写」也是事实；
//   ② 真相源算出来的「每个 bot 手里有什么」。
// 事件面（谁尝试连了哪、有没有越权尝试）没有数据源，所以在页尾如实列缺 —— 不画通道图充数。
// 注：这句原来写的是「谁可以做什么、通道开着吗？」—— 「谁可以做什么」自 2026-10-15 起归「授权」页
// （#/grants，读真相源 `grants/`）：那一页答授权表，这一页答执行面，两个数据源、两个变化节奏。
function renderDataflow(d: Data): Kid[] {
  const v = d.dataflow;
  if (!v) {
    return [banner("bad", "读不到数据流观测", [
      el("span", {}, d.dataflowErr === "" ? "服务没回 anc.dataflow/v1" : d.dataflowErr),
    ])];
  }
  const out: Kid[] = [];
  if ((v.error ?? "") !== "") out.push(banner("bad", "真相源读不动", [el("span", {}, dash(v.error))]));

  const rl = v.relay;
  const open = !rl.declared || rl.timeout_secs !== 0;
  out.push(section("bot 之间的通道（relay）",
    "读的是**执行面** —— 我们打算让它吃什么不算数，它真吃着什么才算",
    el("p", { class: "empty" }, dotLine(open ? "dot bad" : "dot ok",
      open ? " 通道开着" : " 通道关着（v1 口径：机制保留、默认零绑定）")),
    el("p", { class: "empty" }, dash(rl.note))));

  if (v.gaps.length > 0) {
    out.push(section("结构性风险", "与 `anc render --check` 同一套判据（渲染器里的 SecurityGaps）",
      el("ul", {}, ...v.gaps.map((g) => el("li", {}, g)))));
  } else if (v.exec.config_present) {
    out.push(section("结构性风险", "与 `anc render --check` 同一套判据（渲染器里的 SecurityGaps）",
      el("p", { class: "empty" },
        "该显式声明的安全段都在，这条判据没发现结构性缺口。",
        cannot(" —— 这只说明 config 写对了，不说明没人越权（那要有事件面）。"))));
  }

  const rows: Kid[][] = v.bots.map((b) => [
    el("span", { class: "mono" }, b.project),
    dash(b.role),
    el("span", { class: "mono" }, dash(b.mode)),
    dash(b.model),
    b.tools.length === 0 ? cannot("空（没预授权）") : el("span", { class: "mono" }, b.tools.join(", ")),
    b.inbound === 0
      ? cannot("没配")
      : String(b.inbound) + " 人" + (b.inbound_extra > 0 ? "（另放行 " + b.inbound_extra + "）" : ""),
  ]);
  out.push(section("每个 bot 手里有什么", "真相源算出来的（渲染器的输入）—— 入站只出人数，不出标识符",
    v.bots.length === 0
      ? emptyNote("没有启用的成员 —— 没有可授权的对象。")
      : table(["bot", "角色", "工具档", "模型", "harness 白名单", "入站授权"], rows)));

  out.push(execSection(v));
  out.push(section("这一页现在缺什么", "如实列 —— 缺的部分不许用「图好看」补上",
    v.missing.length === 0
      ? emptyNote("没有已知缺口。")
      : el("ul", {}, ...v.missing.map((m) => el("li", {}, m)))));
  return out;
}

// 执行面那一块：这份 config 是不是渲染产物、按哪份真相源生成的、里头有哪几个 project。
function execSection(v: DataflowView): Kid {
  const e = v.exec;
  const projects = Array.isArray(e.projects) ? e.projects : [];
  if (!e.config_present) {
    return section("执行面", "gateway 实际吃的那份 config.toml",
      el("p", { class: "empty" }, cannot(dash(e.note))));
  }
  const rows: Array<[string, Kid]> = [
    ["指纹", e.has_fingerprint
      ? el("span", { class: "mono" }, (e.version ?? "") + " · inputs=" + (e.inputs ?? ""))
      : cannot("没有 anc 指纹 —— 手写或他源配置")],
    ["生成于", e.has_fingerprint ? localTime(e.generated_at ?? "") : "—"],
    ["project", projects.length === 0
      ? cannot("一个都没扫到")
      : el("span", { class: "mono" }, projects.join(", "))],
  ];
  return section("执行面", "gateway 实际吃的那份 config.toml —— 策略面说得再好，也得它真吃上了才算",
    card("这份 config 是谁", "", rows),
    e.note ? el("p", { class: "empty" }, cannot(dash(e.note))) : "");
}

// 原料 —— 读者：所有人。这一页给的是**原料**：数据目录里现在有什么、多久没动过。
// 「有目录 / 有文件」离「有资产」差着一整层，所以标题、口径、页脚都写「原料」，不写「沉淀」。
function renderAssets(d: Data): Kid[] {
  const v = d.assets;
  if (!v) {
    return [banner("bad", "读不到原料清单", [
      el("span", {}, d.assetsErr === "" ? "服务没回 anc.assets/v1" : d.assetsErr),
    ])];
  }
  const out: Kid[] = [];
  if ((v.error ?? "") !== "") out.push(banner("bad", "真相源读不动", [el("span", {}, dash(v.error))]));
  out.push(banner("warn", "这是原料，不是沉淀", [el("span", {}, dash(v.note))]));

  const total = v.dirs.reduce((n, x) => n + x.files, 0);
  out.push(section("规模", "真相源里那张「数据路由」表的一行一行，加上它底下现在有多少东西",
    el("div", { class: "figs" },
      statCard(String(v.dirs.length), "数据目录", "顶层"),
      statCard(String(total), "原料文件", "全部加起来"),
      statCard(String(v.charters), "立项书副本", "真源在客户侧"))));

  const dirRows: Kid[][] = v.dirs.map((x) => [
    el("span", { class: "mono" }, x.dir),
    dash(x.summary),
    String(x.files) + (x.truncated ? "（只数到这里）" : ""),
    humanBytes(x.bytes),
    x.newest ? localTime(x.newest) : cannot("没有文件"),
  ]);
  out.push(section("每个目录里有什么", "空目录照报 0 —— 「还没开始沉淀」是个真实状态，不是错误",
    v.dirs.length === 0
      ? emptyNote("真相源里还没有数据目录（`anc org init` 之后是一个都没有）。")
      : table(["目录", "说明（真相源）", "文件", "体积", "最近变动"], dirRows)));

  const flat: Array<{ path: string; dir: string; bytes: number; modified: string }> = [];
  for (const x of v.dirs) for (const f of x.recent) flat.push({ path: f.path, dir: x.dir, bytes: f.bytes, modified: f.modified });
  flat.sort((a, b) => (a.modified === b.modified ? (a.path < b.path ? -1 : 1) : (a.modified < b.modified ? 1 : -1)));
  const top = flat.slice(0, 10);
  out.push(section("最近动的文件", "每个目录各取最近 5 个，再合起来取前 10 —— 只列相对路径",
    top.length === 0
      ? emptyNote("所有数据目录都是空的。")
      : table(["文件", "目录", "体积", "时间"],
        top.map((f) => [el("span", { class: "mono" }, f.path), f.dir, humanBytes(f.bytes), localTime(f.modified)]))));

  out.push(section("这一页不做什么", "说清楚边界，比多画几个卡片有用",
    el("p", { class: "empty" },
      "不把「有文件」说成「有资产」；不给产出打「能不能复用」的分（那要有判据，见议题 #26 / #27）；",
      "只读 —— 没有任何写入口，也不动这些文件。")));
  return out;
}

// 热层 —— 「还在发生的事」本身。与「待批」同一份底层数据、两个问题：
// 那是队列（等谁点头），这是这一层（几条、几条不新鲜、多久自己走）。
function renderHot(d: Data): Kid[] {
  const v = d.hot;
  if (!v) {
    return [banner("bad", "读不到热层", [el("span", {}, d.hotErr === "" ? "服务没回 anc.hot/v1" : d.hotErr)])];
  }
  if (!v.wired) {
    return [banner("warn", "热层还没接上", [
      el("span", {}, dash(v.why)),
      el("br"),
      el("span", {}, "「读不到」不等于「里面是空的」—— 这一页不拿空列表冒充没事。"),
    ])];
  }
  const out: Kid[] = [];
  if ((v.error ?? "") !== "") out.push(banner("bad", "热层读不动", [el("span", {}, dash(v.error))]));
  out.push(section("这一层现在什么情况", "阈值由服务端给，前端只翻成人话；不认领、不清走、不改任何一条",
    el("div", { class: "figs" },
      statCard(String(v.total), "条目", "热层总数"),
      statCard(String(v.stale), "不新鲜", "只是标记，不拦不删"),
      statCard(durText(v.ttl_secs), "保鲜期", "0 = 不失效"),
      statCard(durText(v.stale_after_secs), "不新鲜阈值", "多久没动算旧"))));
  const cards = v.entries.map((e) => {
    const age = e.age_secs == null ? "没写时间" : durText(e.age_secs) + " 前";
    const cardEl = el("div", { class: "card", tabindex: "0" },
      el("h3", {}, el("span", { class: e.stale ? "dot warn" : "dot ok" }), dash(e.title), chip(e.id, true)),
      el("dl", {},
        el("dt", {}, "状态"), el("dd", {}, dash(e.status)),
        el("dt", {}, "谁 → 交给谁"), el("dd", {}, dash(e.by) + " → " + dash(e.to)),
        el("dt", {}, "多久没动"), el("dd", {}, age),
        el("dt", {}, "新鲜度"), el("dd", {}, e.stale ? cannot("不新鲜（只是标记）") : "新鲜")));
    const open = () => openDrawer({
      eyebrow: "HOT / 热层", title: dash(e.title), sub: dash(e.id) + " · " + dash(e.status),
      body: [
        card("这条现在在哪", "热层是缓存，不是永久资产", [
          ["id", el("span", { class: "mono" }, e.id)],
          ["状态", dash(e.status)],
          ["谁 → 交给谁", dash(e.by) + " → " + dash(e.to)],
          ["多久没动", age],
          ["新鲜度", e.stale ? "不新鲜（只是标记，不拦不删）" : "新鲜"],
        ]),
      ],
    });
    cardEl.addEventListener("click", open);
    cardEl.addEventListener("keydown", (ev) => { if (ev.key === "Enter" || ev.key === " ") { ev.preventDefault(); open(); } });
    return cardEl;
  });
  out.push(section("住着哪些事", "最多回显 " + v.entries.length + " 条；总数照报，不因为截断装作只有这些",
    cards.length === 0 ? emptyNote("热层现在是空的 —— 没有还在发生的事。") : el("div", { class: "grid" }, ...cards)));
  out.push(section("这一页不做什么", "说清边界比多画几张卡有用",
    emptyNote("不认领、不清走、不改任何一条；过期只是标记。要处理，去「待批」或「时间线」。")));
  return out;
}

// 沉淀 —— 只画「料 → 丢掉 → 热层 → 落库 → 隐匿」这条口径，不把原料数量说成资产。
function renderSediment(d: Data): Kid[] {
  return [
    sedimentSection(d),
    section("这一页只答什么", "沉淀是口径图，不是统计报表",
      emptyNote("原料在「原料」页；热层明细在「热层」页。这一页只画四层分工与每层能对上的真数。")),
  ];
}

// 卡点 —— 从时间线派生：只筛红 / 黄 / 等人给一句话的 case，不另存一份事实。
function renderCases(d: Data): Kid[] {
  const v = d.timeline;
  if (!v) {
    return [banner("bad", "读不到时间线", [el("span", {}, d.timelineErr === "" ? "服务没回 anc.timeline/v1" : d.timelineErr)])];
  }
  if (!v.wired) {
    return [banner("warn", "时间线还没接上", [el("span", {}, dash(v.error))])];
  }
  const rows = v.cases.filter((c) => c.band === "red" || c.band === "yellow" || c.band === "input-required");
  const out: Kid[] = [];
  out.push(section("卡点口径", "这一页是时间线的派生视图：只筛，不另存",
    el("div", { class: "figs" },
      statCard(String(v.counts.red), "红 · 卡点 / 失败", "blocked · failed", "dot bad"),
      statCard(String(v.counts.yellow), "黄 · 运行中", "running", "dot warn"),
      statCard(String(rows.length), "这一页列出的 case", "红 / 黄 / 等人给一句话"),
      statCard(d.audit ? String(d.audit.bands.denied + d.audit.bands.failed) : "—", "审计里的被拒 / 失败", "另一份数据源"))));
  const cards = rows.map((c) => {
    const cardEl = el("div", { class: "card", tabindex: "0" },
      el("h3", {}, dotLine(bandDot(c.band), el("span", {}, c.title)), chip(c.key, true)),
      el("dl", {},
        el("dt", {}, "当前状态"), el("dd", {}, statusCell(c.status)),
        el("dt", {}, "谁 → 交给谁"), el("dd", {}, handOff(c.by, c.to)),
        el("dt", {}, "业务域 · 项目"), el("dd", {}, dash([c.domain, c.project].filter((x) => (x ?? "").trim() !== "").join(" · "))),
        el("dt", {}, "最近活动"), el("dd", {}, localTime(c.last_at))),
      el("p", { class: "empty" }, c.recent[0] ? dash(c.recent[0].title) : "这一条没有最近一步。"));
    const open = () => openDrawer({
      eyebrow: "CASE / 卡点", title: dash(c.title), sub: dash(c.key) + " · " + bandWord(c.band),
      body: [
        card("当前态", "取最后一条当当前态", [
          ["status", statusCell(c.status)],
          ["谁 → 交给谁", handOff(c.by, c.to)],
          ["最近活动", localTime(c.last_at)],
          ["共几条", String(c.entries)],
        ]),
        ...c.recent.slice(0, 8).map((e) => card(dash(e.kind), localTime(e.at), [
          ["这一步", dash(e.title)],
          ["detail", dash(e.detail)],
          ["谁 → 交给谁", handOff(e.by, e.to)],
        ])),
      ],
    });
    cardEl.addEventListener("click", open);
    cardEl.addEventListener("keydown", (ev) => { if (ev.key === "Enter" || ev.key === " ") { ev.preventDefault(); open(); } });
    return cardEl;
  });
  out.push(section("正在卡住的事", "按最近动过的排前面；完整历史在「时间线」",
    cards.length === 0
      ? emptyNote("没有红 / 黄 / 等人给一句话的 case —— 当前没有卡点。")
      : el("div", { class: "grid" }, ...cards)));
  return out;
}

// 待批 —— 读者：该点头的人。**全看板唯一能写的一页**（点头 = 一次留痕，不是改配置）。
//
// 这一页不做什么：不替人判断该不该给、不自动过期、不代填 by、不解析提案正文
// （正文是人话，结构只出现在终点 —— grants/ 那七个字段）。
//
// by 存在本机（localStorage）：它是「谁点的」，不是凭据 —— 看板没有免登这一层，
// 所以这一栏由填的人负责，ANC 原样留痕；飞书卡片那条嘴靠事件自带的 open_id，不用它。
const BY_KEY = "anc.board.by";
let byWho = "";
try {
  byWho = localStorage.getItem(BY_KEY) ?? "";
} catch {
  byWho = ""; // 读不到就当没填 —— 不是错误，也不伪造一个名字
}

// 刚点完头的回执。只放内存：它讲的是「刚才那一下」，不是存档（存档在 timeline / audit）。
let lastReceipt: DecideReply | null = null;
let lastDecideErr = "";

function sinceText(iso: string): string {
  const t = new Date(iso);
  if (Number.isNaN(t.getTime())) return localTime(iso);
  const mins = Math.floor((Date.now() - t.getTime()) / 60000);
  if (mins < 0) return localTime(iso);
  if (mins < 60) return mins + " 分钟前";
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return hrs + " 小时前";
  return Math.floor(hrs / 24) + " 天前";
}

// 秒数说人话：保鲜期与「多久没动算不新鲜」这两个阈值都由服务端给（/api/hot），
// 前端不另立一份口径，只负责把秒数翻成看得懂的写法。0 = 不失效。
function durText(secs: number): string {
  if (secs <= 0) return "永久";
  if (secs % 86400 === 0) return secs / 86400 + " 天";
  if (secs % 3600 === 0) return secs / 3600 + " 小时";
  if (secs % 60 === 0) return secs / 60 + " 分钟";
  return secs + " 秒";
}

function receiptBanner(r: DecideReply): HTMLElement {
  return el("div", { class: "banner ok" },
    el("h3", {}, "刚刚点头：" + dash(r.verdict) + "（" + (r.by === "" ? "没记谁" : r.by) + "）"),
    el("ul", {},
      el("li", {}, el("span", { class: "mono" }, dash(r.proposal)), " —— ", dash(r.note)),
      el("li", {}, "落库 ", el("span", { class: "mono" }, dash(r.timeline))),
      el("li", {}, "审计 ", el("span", { class: "mono" }, dash(r.audit)))));
}

// 「谁在点」—— 看板没有免登，所以这一栏由页面出，并记在本机（下次不用再打）。
function bySection(): Kid {
  const input = el("input", {
    class: "by", type: "text",
    placeholder: "你的名字 / 岗位（会原样写进留痕）",
    value: byWho,
  });
  input.addEventListener("input", () => {
    byWho = (input as HTMLInputElement).value;
    try {
      localStorage.setItem(BY_KEY, byWho);
    } catch {
      // 记不住就算了：留痕照写，只是下次要重打。这不是错误，不弹东西吓人。
    }
  });
  return section("谁在点", "看板没有免登：这一栏由填的人负责；ANC 只把它原样留痕，不校",
    el("p", { class: "empty" },
      "飞书卡片那条路不需要它 —— 卡片点击事件自带 open_id，点的人是谁由事件给。"),
    el("div", { class: "field" }, input));
}

// 一条提案一张卡：人话在上，凭证（id / 时间 / 客体）在下，按钮在最底下。
function pendingCard(v: ApprovalView, p: ApprovalPending): HTMLElement {
  const title = (p.title ?? "").trim();
  const subject = [p.domain, p.project].map((x) => (x ?? "").trim()).filter((x) => x !== "").join(" / ");
  // 判真那一档：**在队列里 = 有人在等你给一句话**（靛蓝 + 暂停形）。
  // 档由服务端给（band），前端照画；服务端没给就不画点 —— 宁可少一个符号，
  // 也不画一个说不清什么意思的圆。符号旁边永远带词（颜色不许单独承载信息）。
  const band = (p.band ?? "").trim();
  const lamp: Kid[] = band === "" ? [] : [dotLine(bandDot(band), el("span", {}, bandWord(band)))];
  return el("div", { class: "card prop" },
    el("h3", {}, ...lamp, el("span", {}, title === "" ? "（提案没写标题）" : title), chip(dash(p.id), true)),
    el("dl", {},
      el("dt", {}, "该谁批"),
      el("dd", {}, (p.to ?? "").trim() === "" ? cannot("域表里没写 who —— 不知道该谁批") : p.to),
      el("dt", {}, "谁提的"), el("dd", {}, dash(p.who)),
      el("dt", {}, "客体"), el("dd", {}, subject === "" ? "—" : subject),
      el("dt", {}, "进队"),
      el("dd", {}, localTime(p.enqueued) + "（" + sinceText(p.enqueued) + "）"),
      el("dt", {}, "当前态"), el("dd", {}, dash(p.status))),
    el("p", { class: "body" }, (p.body ?? "").trim() === "" ? cannot("提案没有正文") : p.body),
    actsRow(v, p));
}

// 三个按钮来自服务端给的 signals —— 前端不写死信号词，加一个词只改服务端。
function actsRow(v: ApprovalView, p: ApprovalPending): HTMLElement {
  const why = el("input", { class: "why", type: "text", placeholder: "为什么这么批（可空；驳回最好写一句）" });
  const row = el("div", { class: "acts" });
  for (const sig of v.signals) {
    const b = el("button", { class: "act " + sig.value, type: "button" },
      sig.label === "" ? sig.value : sig.label);
    b.addEventListener("click", () => {
      void decide(v, p, sig, why as HTMLInputElement, row);
    });
    row.append(b);
  }
  if (v.signals.length === 0) row.append(cannot("服务没给可点的信号 —— 这一版点不了"));
  return el("div", {}, why, row);
}

// 点一次头：发一次信号，成功就整页重取（队列真的变了）。
async function decide(
  v: ApprovalView, p: ApprovalPending, sig: ApprovalSignal,
  why: HTMLInputElement, row: HTMLElement,
): Promise<void> {
  lastDecideErr = "";
  const who = byWho.trim();
  if (who === "") {
    // 后端也这么拒（「匿名点头等于没点头」）—— 这里先说，省一次往返。
    lastDecideErr = "先在上面填「谁在点」：匿名点头等于没点头（后端也这么拒）。";
    shell();
    return;
  }
  if (v.write_path === "") {
    lastDecideErr = "服务没给写口地址（write_path 是空的）—— 刷新看看，或看服务端日志。";
    shell();
    return;
  }
  // 点下去就先按住按钮：连着点两下 = 第二次必然「不在队列里」（那条已经结过账了）。
  for (const b of Array.from(row.querySelectorAll("button"))) b.setAttribute("disabled", "disabled");
  const res = await postJSON<DecideReply & { error?: string; hint?: string }>(v.write_path, {
    id: p.id, by: who, signal: sig.value, why: why.value,
  });
  if (res.status === 200 && res.body && res.body.ok) {
    lastReceipt = res.body;
    await boot(true);
    return;
  }
  const msg = res.body?.error ?? res.err;
  const hint = res.body?.hint ?? "";
  lastDecideErr = (msg === "" ? "HTTP " + res.status : msg) + (hint === "" ? "" : "（" + hint + "）");
  shell();
}

function pendingSection(v: ApprovalView): Kid {
  if (v.pending.length === 0) {
    return section("队列", "进队时间从早到晚 —— 早提的先办",
      emptyNote("队列是空的：现在没有等点头的提案。"));
  }
  return section("队列", "一共 " + v.count + " 条 —— 早提的排前面；每条前面那个靛蓝暂停符号是判真第四档："
    + "有人在等你给一句话（没坏，也不是系统自己在走）",
    el("div", { class: "grid props" }, ...v.pending.map((p) => pendingCard(v, p))));
}

function signalsSection(v: ApprovalView): Kid {
  const legend = v.signals.length === 0
    ? emptyNote("服务没给信号表 —— 这一版点不了。")
    : el("p", { class: "legend" }, ...v.signals.map((sig) => el("span", { class: "st" },
        chip(sig.value, true),
        el("span", {}, sig.label === "" ? "（服务没给人话）" : sig.label))));
  return section("这一页只认三个字", "别的写法一律不认，也不猜 —— 判据永远只看这三个字",
    legend,
    el("p", { class: "empty" },
      "点完头，ANC 只做三件事：收提案、送给该批的人、留痕 —— 不替人判断该不该给；",
      "也不动真相源（grants/ 那个文件由人侧的管理者 bot 写）。"));
}

function renderApprovals(d: Data): Kid[] {
  const v = d.approvals;
  if (!v) {
    return [banner("bad", "读不到待批队列", [
      el("span", {}, d.approvalsErr === "" ? "服务没回 anc.approvals/v1" : d.approvalsErr),
    ])];
  }
  const out: Kid[] = [];
  if (lastReceipt) out.push(receiptBanner(lastReceipt));
  if (lastDecideErr !== "") out.push(banner("bad", "上一次点头没成功", [el("span", {}, lastDecideErr)]));
  if ((v.error ?? "") !== "") out.push(banner("bad", "热层读不动", [el("span", {}, dash(v.error))]));
  if (!v.wired) {
    out.push(banner("warn", "待批队列还没接上", [
      el("span", {}, dash(v.why)),
      el("br"),
      el("span", {}, "接不上不代表「没人提」—— 这一页宁可不显示，也不拿一份空队列冒充它。"),
    ]));
  }
  out.push(bySection());
  if (v.wired) out.push(pendingSection(v));
  out.push(signalsSection(v));
  return out;
}

// 时间线 —— 读者：跟进的人 / 老板。数据源是人和 agent 用 `anc timeline add` 写下的留存记录。
//
// 这一页的纪律（与后端 timeline 包同一条口径）：
//   - 这些行是**写下来的**，不是看板从会话里推出来的 —— 没记就是没记，不替谁总结、更不编一条；
//   - 三档只按 status 配色，认不出的词**原样保留**（画灰），以后加词只改数据、不改这里；
//   - 一个 case 多行 = 一次推进：表里取最后一条当当前态，历史留在「每一步」里，一行不删。
const BAND_DOT: Record<string, string> = {
  green: "dot ok", yellow: "dot warn", red: "dot bad", unknown: "dot gray",
  // 第四档：等人给一句话。它**不来自 status** —— 来自「这条正躺在待批队列里」这个事实，
  // 所以它只在真有那个数据源的地方出现（审批页），不从热层的 status 词推（#59 拍的口径）。
  "input-required": "dot wait",
};
const bandDot = (b: string): string => BAND_DOT[b] ?? "dot";
const BAND_WORD: Record<string, string> = {
  green: "绿 · 已完成", yellow: "黄 · 运行中", red: "红 · 卡点 / 失败", unknown: "灰 · 认不出的词",
  "input-required": "靛 · 等你给一句话",
};
const bandWord = (b: string): string => BAND_WORD[b] ?? "灰 · 认不出的词";

// 四档图例：形状 → 含义。摆出来是给**不看颜色**的读者留一条路（色盲、灰度截图、小屏）。
function bandLegend(): HTMLElement {
  return el("p", { class: "legend" },
    dotLine("dot ok", el("span", {}, "实心圆 · 已完成")),
    dotLine("dot warn", el("span", {}, "带光环的圆 · 运行中")),
    dotLine("dot bad", el("span", {}, "菱形 · 卡点或失败")),
    dotLine("dot gray", el("span", {}, "空心虚线圈 · 认不出的词（没判过）")));
}

// 状态原词照排（不翻译、不吞）：认不出的词也得让人看见它长什么样。
const statusCell = (status: string | undefined): Kid =>
  (status ?? "").trim() === "" ? cannot("没写 status") : el("span", { class: "mono" }, status ?? "");

// 「谁 → 交给谁」：两头都可能没写 —— 缺哪个就说缺哪个，不拿一个「—」糊过去。
// 这一列是这一页的重点：**下一手是谁的决定**。
function handOff(by: string | undefined, to: string | undefined): Kid {
  const b = (by ?? "").trim();
  const t = (to ?? "").trim();
  if (b === "" && t === "") return cannot("没写谁在跟、也没说交给谁");
  return el("span", {},
    b === "" ? cannot("没写谁记的") : el("span", { class: "mono" }, b),
    el("span", {}, " → "),
    t === "" ? cannot("还没说交给谁") : el("strong", {}, t));
}

function renderTimeline(d: Data): Kid[] {
  const v = d.timeline;
  if (!v) {
    return [banner("bad", "读不到时间线", [
      el("span", {}, d.timelineErr === "" ? "服务没回 anc.timeline/v1" : d.timelineErr),
    ])];
  }
  if (!v.wired) {
    return [banner("warn", "时间线还没接上", [
      el("span", {}, dash(v.error)),
      el("br"),
      el("span", { class: "mono" }, "anc timeline add <vault> -by <谁> -title <一句话>"),
    ])];
  }

  const out: Kid[] = [];
  if ((v.error ?? "") !== "") out.push(banner("bad", "这一页读不动", [el("span", {}, dash(v.error))]));
  out.push(banner("warn", "这些行是人 / agent 自己写下来的", [el("span", {}, dash(v.note))]));
  if (v.bad.length > 0) {
    out.push(banner("warn", "有 " + v.bad.length + " 行读不懂（逐行列出来，不静默跳过）",
      v.bad.map((b) => el("span", { class: "mono" }, b))));
  }

  out.push(section("三档",
    "只按 status 配色，形状跟档走：done=实心圆 / running=带光环的圆 / blocked·failed=菱形 / 认不出的词=空心虚线圈（原样留着，不吞）",
    el("div", { class: "figs" },
      statCard(String(v.counts.green), "绿 · 已完成", "status=done", "dot ok"),
      statCard(String(v.counts.yellow), "黄 · 运行中", "status=running", "dot warn"),
      statCard(String(v.counts.red), "红 · 卡点 / 失败", "blocked · failed", "dot bad"),
      statCard(String(v.counts.unknown), "灰 · 认不出的词",
        v.counts.unknown === 0 ? "没有，词表没跑偏" : "原样保留，不吞", "dot gray"),
      statCard(String(v.entries), "流水条数", "折叠成 " + v.total + " 个 case")),
    bandLegend()));

  const tl = sliced(v.cases, hashQuery());
  const rows: Kid[][] = tl.rows.map((c) => [
    dotLine(bandDot(c.band), statusCell(c.status)),
    el("div", {}, el("span", { class: "mono" }, dash(c.key)), el("div", {}, dash(c.title))),
    handOff(c.by, c.to),
    dash([c.domain, c.project].filter((x) => (x ?? "").trim() !== "").join(" · ")),
    String(c.entries),
    localTime(c.last_at),
  ]);
  out.push(section("每个 case 现在在哪一步",
    "取最后一条当当前态 —— 它写着「遇到了什么、走到了哪一步、这一步是谁的决定」"
      + (v.cases.length > PER_PAGE
        ? "（共 " + v.cases.length + " 个 · 这一屏第 " + tl.cur + " / " + tl.pages + " 页）" : ""),
    tl.rows.length === 0
      ? emptyNote("时间线还是空的（`timeline/` 里一行都没有）。空不是错 —— 「还没开始记」是个真实状态。")
      : el("div", {},
          table(["档 / status", "case", "谁 → 交给谁", "业务域 · 项目", "几条", "最近活动"], rows),
          pager(v.cases.length, tl.cur, "timeline"))));

  const steps: Kid[] = tl.rows.map((c) => {
    const head: Kid[] = [
      el("span", { class: bandDot(c.band) }),
      el("span", { class: "mono" }, dash(c.key)), " ",
      el("strong", {}, dash(c.title)), " ",
      chip(bandWord(c.band) + " · " + ((c.status ?? "").trim() === "" ? "没写 status" : c.status)),
      chip("共 " + c.entries + " 条", true),
    ];
    const inner: Kid[][] = c.recent.map((e) => [
      dotLine(bandDot(e.band), statusCell(e.status)),
      localTime(e.at),
      el("div", {},
        (e.kind ?? "").trim() === "" ? cannot("没写 kind") : el("span", { class: "chip mono" }, (e.kind ?? "").trim()),
        el("div", {}, dash(e.title)),
        (e.detail ?? "").trim() === "" ? "" : el("p", { class: "empty" }, dash(e.detail)),
        (e.refs ?? []).length === 0 ? "" : el("p", { class: "empty mono" }, (e.refs ?? []).join(" · "))),
      handOff(e.by, e.to),
    ]);
    return el("details", {}, el("summary", {}, el("span", { class: "st" }, ...head)),
      table(["档 / status", "时间", "这一步（kind + 一句话）", "谁 → 交给谁"], inner));
  });
  if (steps.length > 0) {
    out.push(section("每一步都留着",
      "每个 case 带最近几步（最新在上）；完整流水在 `timeline/*.jsonl` 里 —— 一行都没删"
        + (v.cases.length > PER_PAGE ? "（跟上面那张表同一页）" : ""),
      ...steps));
  }

  if (v.total > v.cases.length) {
    out.push(section("还有更早的",
      "按「最近动过的排前面」，这一页只铺了 " + v.cases.length + " 个（共 " + v.total + " 个）",
      el("p", { class: "empty" },
        el("span", {}, "往后翻："), el("span", { class: "mono" }, "anc timeline list <vault> --before " + dash(v.next_before)),
        el("span", {}, "；按档筛："), el("span", { class: "mono" }, "anc timeline list <vault> --band red"))));
  }

  out.push(section("这一页答不了什么", "答不了的照写 —— 这一页是「卡在哪、谁在跟」，不是审计",
    v.missing.length === 0
      ? emptyNote("后端没列缺。")
      : el("ul", {}, ...v.missing.map((m) => el("li", {}, m)))));
  return out;
}
// 四档配色：ok 绿实心圆 / denied 菱形（被拦下）/ failed 菱形（真失败）/ 判不出的虚线圈。
// denied 与 failed 是两个词、两件事 —— 「权限没给」不能说成「故障」（后端同一条纪律）。
const AUDIT_DOT: Record<string, string> = {
  ok: "dot ok", denied: "dot bad", failed: "dot bad", unknown: "dot gray",
};
const AUDIT_WORD: Record<string, string> = {
  ok: "绿 · 成功", denied: "红 · 被拦下", failed: "红 · 失败", unknown: "灰 · 判不出",
};
const auditDot = (b: string): string => AUDIT_DOT[b] ?? "dot gray";
const auditWord = (b: string): string => AUDIT_WORD[b] ?? "灰 · 判不出";

// 「对谁」那一栏。四类各自是什么见后端 Zone 常量 ——
// 关键是**「判不出」不许显示成「不跨域」**：把「没判」画成「没事」是最坏的一种假绿。
function zoneCell(e: AuditEntryView): Kid {
  switch (e.zone) {
    case "domain":
      if (!e.known) return el("span", {}, chip(e.actee, true), cannot(" 跨域未判（行使者没配域）"));
      if (e.cross) return el("span", {}, chip(e.actee, true), el("strong", {}, " ⚠ 跨域"));
      return chip(e.actee, true);
    case "vault":
      return cannot("vault 内 · 不属于任何域");
    case "outside":
      return el("strong", {}, "⚠ 域外");
    default:
      return cannot("判不出（目标抽不出）");
  }
}

function renderAudit(d: Data): Kid[] {
  const v = d.audit;
  if (!v) {
    return [banner("bad", "读不到审计流水", [
      el("span", {}, d.auditErr === "" ? "服务没回 anc.audit/v1" : d.auditErr),
    ])];
  }
  if (!v.wired) {
    return [banner("warn", "审计还没接上", [
      el("span", {}, dash(v.error)),
      el("br"),
      el("span", { class: "mono" }, "anc audit collect <vault> --config <config.toml> --write"),
    ])];
  }

  const out: Kid[] = [];
  if ((v.error ?? "") !== "") out.push(banner("bad", "这一页读不动", [el("span", {}, dash(v.error))]));
  out.push(banner("warn", "这些行是系统记下来的，不是谁写的", [el("span", {}, dash(v.note))]));
  if (v.bad.length > 0) {
    out.push(banner("warn", "有 " + v.bad.length + " 行读不懂（逐行列出来，不静默跳过）",
      v.bad.map((b) => el("span", { class: "mono" }, b))));
  }

  out.push(section("结果四档",
    "只按 result 分形状：ok=实心圆 / denied=菱形（被拦下）/ failed=菱形（真失败）/ 认不出的词=空心虚线圈（原样留着，不吞）",
    el("div", { class: "figs" },
      statCard(String(v.bands.ok), "绿 · 成功", "result=ok", "dot ok"),
      statCard(String(v.bands.denied), "红 · 被拦下", "result=denied", "dot bad"),
      statCard(String(v.bands.failed), "红 · 失败", "result=failed", "dot bad"),
      statCard(String(v.bands.unknown), "灰 · 判不出",
        v.bands.unknown === 0 ? "没有，结果都认得" : "原样保留，不吞", "dot gray"))));

  out.push(section("这次行使跨出去没有",
    "「跨域」按 SPEC §6 的严格含义用：域与域之间。域外与 vault 内非域目录不叫跨域 —— 它们各自是独立的信号",
    el("div", { class: "figs" },
      statCard(String(v.scopes.domain), "域内", "目标落在某个业务域"),
      statCard(String(v.scopes.cross), "⚠ 跨域", "目标域不是自己的",
        v.scopes.cross > 0 ? "dot bad" : "dot ok"),
      statCard(String(v.scopes.outside), "⚠ 域外", "目标是 vault 之外",
        v.scopes.outside > 0 ? "dot bad" : "dot ok"),
      statCard(String(v.scopes.vault), "vault 内 · 非域", "结构目录 / 还没划域"),
      statCard(String(v.scopes.unknown), "判不出", "目标抽不出（Bash 这类）", "dot gray"))));

  const au = sliced(v.records, hashQuery());
  const rows: Kid[][] = au.rows.map((e) => [
    dotLine(auditDot(e.band), el("span", {}, auditWord(e.band))),
    localTime(e.at),
    el("span", { class: "mono" }, dash(e.actor)),
    el("span", { class: "chip mono" }, dash(e.action)),
    zoneCell(e),
    el("div", {}, el("span", { class: "mono" }, dash(e.object)),
      (e.tool ?? "").trim() === "" ? "" : el("span", { class: "chip mono" }, dash(e.tool)),
      e.has_why ? el("span", { class: "chip" }, "原话见 CLI") : ""),
  ]);
  out.push(section("流水（最新在上）",
    "共 " + v.total + " 条，看板手上 " + v.records.length + " 条。完整流水在 audit/*.jsonl 里 —— 一行一条，一行都没删。" +
    "原话（工具结果 / 命令行）不带到看板：它是本机排查资料、夹着本机路径，要看用 anc audit log",
    au.rows.length === 0
      ? emptyNote("审计还是空的（audit/ 里一行都没有）。跑 anc audit collect <vault> --write 从 harness 记录归集，" +
        "或用 anc audit add 补记一条。空不是错 —— 「还没有行使」是个真实状态。")
      : el("div", {},
          table(["结果", "何时", "谁", "类别", "对谁", "目标（+ 原话）"], rows),
          pager(v.records.length, au.cur, "audit"))));

  // 被拦下/失败是一个**横切视图**（不是主流水的一段），所以它自己不分页：
  // 列前 PER_PAGE 条，超出的写清去哪儿看，不假装这就是全部。
  const flaggedAll = v.records.filter((e) => e.band === "denied" || e.band === "failed");
  const flagged = flaggedAll.slice(0, PER_PAGE);
  if (flagged.length > 0) {
    out.push(section("被拦下的与失败的",
      "这两类要分开读：denied = 权没给（去查授权，不是查故障）；failed = 真的执行失败（去查环境）。" +
      "原话不带到看板，要看用 anc audit log <vault> --result denied",
      el("div", { class: "figs" },
        statCard(String(v.bands.denied), "被拦下", "权没给 —— 查授权，不是查故障", "dot bad"),
        statCard(String(v.bands.failed), "失败", "真执行失败 —— 查环境", "dot bad")),
      table(["结果", "何时", "谁", "类别", "对谁", "目标（+ 原话）"],
        flagged.map((e) => [
          dotLine(auditDot(e.band), el("span", {}, auditWord(e.band))),
          localTime(e.at),
          el("span", { class: "mono" }, dash(e.actor)),
          el("span", { class: "chip mono" }, dash(e.action)),
          zoneCell(e),
          el("div", {}, el("span", { class: "mono" }, dash(e.object)),
            e.has_why ? el("span", { class: "chip" }, "原话见 CLI") : ""),
        ])),
      flaggedAll.length > flagged.length
        ? el("p", { class: "empty" }, "这里只列前 " + PER_PAGE + " 条（共 " + flaggedAll.length +
            " 条）—— 完整的用 anc audit log <vault> --result denied")
        : ""));
  }

  if (v.total > v.records.length) {
    out.push(section("还有更早的",
      "这一页只铺最近 " + v.records.length + " 条（共 " + v.total + " 条）",
      el("p", { class: "empty" },
        el("span", {}, "往后翻："), el("span", { class: "mono" }, "anc audit log <vault> --limit 200"),
        el("span", {}, "；只看被拒的："), el("span", { class: "mono" }, "anc audit log <vault> --result denied"))));
  }

  out.push(section("这一页答不了什么",
    "答不了的照写 —— 这一页是「系统记下来的行使」，不是「全部行使」",
    v.missing.length === 0
      ? emptyNote("后端没列缺。")
      : el("ul", {}, ...v.missing.map((m) => el("li", {}, m)))));
  return out;
}

// ---------- 授权表页：谁被允许对谁做什么 ----------
//
// 读者：管理者 / 想知道「谁被允许做什么」的人。这一页只回答**一个问题**（授权表本身）：
// 「通道开没开」在「数据流」页（那里的数据源是上游 config，跟这里不是一份）。
//
// 视角（`#/grants?who=<编号身份>`）是这一页的**位置**，不是状态：进 URL，刷新 / 前进后退 /
// 把链接发给别人，都还停在同一个主体上；不带 who 就是全量。换视角只重取这一份（见 loadGrants）。

const grantKindLabel = (kind: string): string =>
  kind === "agent" ? "业务 agent" : kind === "member" ? "成员"
    : kind === "role" ? "岗位" : kind === "domain" ? "业务域" : "认不出";

// 面包屑：这一页在哪、正以谁的视角看。视角**有效还不够，得看得见** —— 否则读的人不知道
// 手里这一份是谁的那一份（服务端认不出时也照实写「认不出」）。
function grantsCrumb(v: GrantsView): Kid {
  const parts: Kid[] = [
    el("a", { href: "#/system?tab=gateway" }, "看板"),
    el("span", { class: "sep" }, "›"),
    el("span", {}, "授权"),
    el("span", { class: "sep" }, "｜"),
  ];
  const sc = v.scope;
  if (sc.ref === "") {
    parts.push(el("span", {}, "视角："), el("span", { class: "mono" }, "全量"));
  } else if (!sc.known) {
    parts.push(el("span", {}, "视角："), el("span", { class: "mono" }, sc.ref), " ", cannot("（认不出）"));
  } else {
    const w = sc.who!;
    parts.push(el("span", {}, "视角："), el("span", { class: "mono" }, w.ref), " ",
      el("span", { class: "chip" }, grantKindLabel(w.kind)));
    if ((w.name ?? "") !== "" && w.name !== w.ref) parts.push(el("span", {}, " " + w.name));
    if ((w.domain ?? "") !== "") {
      parts.push(el("span", { class: "sep" }, "·"), el("span", {}, "归属 " + dash(w.domain_name === "" ? w.domain : w.domain_name)));
    }
  }
  return el("p", { class: "crumb" }, ...parts);
}

// 一格主体：原文照写（那是要核对的东西）+ 解出来是谁。解不出来的**照原样留着**并说清为什么。
function grantSubjectCell(r: GrantRow, slot: string): Kid {
  const ref = r.who.find((w) => w.slot === slot);
  if (!ref) return cannot("没写");
  const raw = el("span", { class: "mono" }, ref.raw);
  const id = ref.identity;
  if (!id) {
    return el("span", {}, raw, el("br"),
      cannot(grantKindLabel(ref.kind) + (ref.note ? " · " + ref.note : "")));
  }
  const nm = (id.name ?? "") === "" ? id.ref : id.name;
  const extra = (id.domain ?? "") !== "" ? " · " + dash(id.domain_name === "" ? id.domain : id.domain_name) : "";
  return el("span", {}, raw, el("br"),
    el("span", { class: "chip" }, grantKindLabel(id.kind)), " ",
    el("span", {}, dash(nm) + extra));
}

// 视角选择器：只列**授权表里出现过的主体**（真相源算出来的名单）—— 不另给一份「所有人」的
// 名单：这里能站的位置，就是表里真出现过的那些。名单由服务端给（不受当前视角影响，否则
// 一筛就缩水，换不回全量、也换不到第二个人）。
function grantPicker(v: GrantsView): Kid {
  const links: Kid[] = [
    el("a", { class: v.scope.ref === "" ? "pg on" : "pg", href: "#/system?tab=gateway" },
      "全量 · " + String(v.total) + " 条"),
  ];
  for (const sub of v.subjects) {
    const nm = (sub.name ?? "") === "" ? sub.ref : sub.name;
    links.push(el("a", {
      class: v.scope.ref === sub.ref ? "pg on" : "pg",
      href: "#/system?tab=gateway&who=" + encodeURIComponent(sub.ref),
    }, el("span", { class: "chip" }, grantKindLabel(sub.kind)), " ",
      nm + "（" + sub.ref + "）"));
  }
  const now = v.scope.ref === ""
    ? el("p", { class: "empty" }, "现在看的是**全量**：" + String(v.total) + " 条。")
    : !v.scope.known
      ? el("p", { class: "empty" }, cannot("认不出 " + v.scope.ref + "：" + dash(v.scope.why)),
          " —— 认不出就不给表（给全量会被读成「他的权限是全部」，给空表会被读成「他什么都没有」）。")
      : el("p", { class: "empty" },
          "现在看的是 " + dash(v.scope.who?.ref) + " 相关的 " + String(v.shown) + " / " + String(v.total) + " 条（全表仍是 " + String(v.total) + " 条）。");
  if (v.subjects.length === 0) {
    return el("div", {}, now, emptyNote("表里还没出现过任何主体 —— 一条授权都还没有。"));
  }
  return el("div", {}, now, el("div", { class: "pager" }, ...links));
}

function grantEmpty(v: GrantsView): Kid {
  if (v.scope.ref !== "" && v.scope.known) {
    return emptyNote("这个视角下一条都没有：表里没有一条与它有关。" +
      " —— 这**不等于**「它什么都做不了」：这一页只看真相源里写没写权，执行层拦不拦是另一页的事。");
  }
  return emptyNote("授权表是空的（`grants/` 里还没有条目）。这不算错：" +
    "没有 grant 就是不通（SPEC §6 不变量 1）—— 表空只说明还没有给过任何一条权。");
}

function renderGrants(d: Data): Kid[] {
  const v = d.grants;
  if (!v) {
    return [banner("bad", "读不到授权表", [
      el("span", {}, d.grantsErr === "" ? "服务没回 anc.board.grants/v1" : d.grantsErr),
    ])];
  }
  const out: Kid[] = [grantsCrumb(v)];
  if ((v.error ?? "") !== "") {
    out.push(banner("bad", "真相源读不动", [el("span", {}, dash(v.error))]));
    return out;
  }

  out.push(section("这一页的口径", "", el("p", { class: "empty" }, dash(v.note))));

  out.push(section("视角", "换一个主体，就只看与他有关的那几条；链接可以发给别人",
    grantPicker(v)));

  out.push(section("表里有什么",
    "`grants/` 里逐条搬过来，一个字不改 —— 这一页是给人核对的",
    v.rows.length === 0
      ? grantEmpty(v)
      : table(["谁给的", "给谁", "动作", "客体", "期限", "为什么", "落点"],
          v.rows.map((r) => [
            grantSubjectCell(r, "from"),
            grantSubjectCell(r, "to"),
            el("span", { class: "mono nw" }, dash(r.action)),
            el("span", { class: "mono nw" }, dash(r.object)),
            (r.ttl ?? "").trim() === ""
              ? cannot("永久（没写期限）")
              : el("span", { class: "mono" }, r.ttl),
            dash(r.reason),
            el("span", { class: "mono nw" }, r.path),
          ]))));

  out.push(section("授权表上的校验发现",
    "与 `anc org check` 同一张规则表 —— 档次由规则表定，这一页不另判",
    v.findings.length === 0
      ? emptyNote("没有发现 —— 授权表这一层没报出问题。")
      : el("ul", { class: "props" }, ...v.findings.map((f) => el("li", {},
          el("span", { class: "chip mono" }, f.level), " ", findingLine(f))))));

  out.push(section("这一页答不了什么", "如实列 —— 缺的部分不许用「图好看」补上",
    el("ul", {}, ...v.missing.map((m) => el("li", {}, m)))));
  return out;
}

// 页面注册表。顺序就是侧栏顺序；分组是「问题」，页名是「答案」。
// 网关 —— 从「数据流」里拆出来的执行面。那一页回答「每个 bot 手里有什么」，
// 这一页只回答「gateway 真吃的是哪份配置、relay 的执行口径是什么」。
// 两个问题、两个变化节奏：换 bot 不改 config，改 config 不改 bot 名单。
function renderGateway(d: Data): Kid[] {
  const v = d.dataflow;
  if (!v) {
    return [banner("bad", "读不到网关观测", [
      el("span", {}, d.dataflowErr === "" ? "服务没回 anc.dataflow/v1" : d.dataflowErr),
    ])];
  }
  const out: Kid[] = [];
  if ((v.error ?? "") !== "") out.push(banner("bad", "网关读不动", [el("span", {}, dash(v.error))]));
  const rl = v.relay;
  const open = !rl.declared || rl.timeout_secs !== 0;
  const e = v.exec;
  const projects = Array.isArray(e.projects) ? e.projects : [];

  out.push(el("div", { class: "gw-grid" },
    el("section", { class: "glass gw-card" },
      el("header", {},
        el("p", { class: "eyebrow" }, "RELAY", el("span", {}, "bot 之间的通道")),
        el("span", { class: "gw-state " + (open ? "bad" : "ok") }, open ? "开着" : "关着")),
      el("h2", {}, open ? "通道默认放行" : "通道零绑定"),
      el("p", { class: "gw-lead" }, dash(rl.note)),
      el("div", { class: "gw-facts" },
        el("div", {}, el("span", {}, "声明"), el("b", {}, rl.declared ? "有" : "没有")),
        el("div", {}, el("span", {}, "超时"), el("b", {}, rl.timeout_secs !== 0 ? rl.timeout_secs + "s" : "∞")),
        el("div", {}, el("span", {}, "口径"), el("b", {}, open ? "上游默认" : "默认拒"))),
      el("p", { class: "gw-note" }, "这一格读的是执行面：gateway 真正吃进去的那份配置。")),
    el("section", { class: "glass gw-card" },
      el("header", {},
        el("p", { class: "eyebrow" }, "EXECUTE", el("span", {}, "执行面")),
        el("span", { class: "gw-state " + (e.config_present ? "ok" : "gray") }, e.config_present ? "在位" : "缺配置")),
      el("h2", {}, e.config_present ? "config.toml 在位" : "没有读到 config.toml"),
      el("p", { class: "gw-lead" },
        e.config_present
          ? "策略面说得再好，也得它真吃上了才算。"
          : dash(e.note ?? "gateway 还没生成或没指向这份配置。")),
      el("div", { class: "gw-facts" },
        el("div", {}, el("span", {}, "指纹"), el("b", {}, e.has_fingerprint ? "anc 生成" : "手写 / 他源")),
        el("div", {}, el("span", {}, "project"), el("b", {}, e.config_present ? String(projects.length) : "—")),
        el("div", {}, el("span", {}, "生成于"), el("b", {}, e.has_fingerprint ? localTime(e.generated_at ?? "") : "—"))),
      projects.length > 0
        ? el("p", { class: "gw-projects mono" }, projects.join("  ·  "))
        : el("p", { class: "gw-note" }, e.config_present ? "一个 project 都没扫到 —— 配置可能是空的。" : "配置不在，没有 project 可数。"))));

  out.push(section("结构性风险", "与 `anc render --check` 同一套判据（渲染器里的 SecurityGaps）",
    v.gaps.length === 0
      ? el("p", { class: "empty" },
          e.config_present
            ? "该显式声明的安全段都在，这条判据没发现结构性缺口。"
            : "还没有可检查的配置。",
          cannot(" —— 这只说明 config 写对了，不说明没人越权（那要有事件面）。"))
      : el("ul", { class: "risk-list" }, ...v.gaps.map((g) => el("li", {}, g)))));

  const rows: Kid[][] = v.bots.map((b) => [
    el("span", { class: "mono" }, b.project), dash(b.role), el("span", { class: "mono" }, dash(b.mode)),
    dash(b.model),
    b.tools.length === 0 ? cannot("空（没预授权）") : el("span", { class: "mono" }, b.tools.join(", ")),
    b.inbound === 0 ? cannot("没配") : String(b.inbound) + " 人",
  ]);
  out.push(section("执行面上的 bot", "真相源算出来的白名单与入站授权 —— 入站只出人数，不出标识符",
    v.bots.length === 0 ? emptyNote("没有启用的成员 —— 没有可授权的对象。")
      : table(["bot", "角色", "工具档", "模型", "harness 白名单", "入站授权"], rows)));

  out.push(section("还缺什么", "如实列 —— 缺的部分不许用「图好看」补上",
    v.missing.length === 0 ? emptyNote("没有已知缺口。")
      : el("ul", { class: "risk-list" }, ...v.missing.map((m) => el("li", {}, m)))));
  return out;
}

// 关系图谱 —— 一张只画「结构」的图。节点来自真相源 + 运行态，连线只表示挂靠关系。
// 它不是装饰：点节点能打开同一个详情抽屉，和命令面板里搜出来的是同一份字段。
function renderGraph(d: Data): Kid[] {
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
        card("连线", "挂靠关系", [["业务域 → 项目", "项目属于哪块业务"], ["项目 → bot", "bot 服务哪个项目"], ["业务域 → 成员", "成员负责哪块业务"]]),
        card("不画什么", "避免误读", [["业务流", "不在这张图里"], ["消息", "通道状态看「系统 · 数据流」"], ["权限", "看「系统 · 网关与授权」"]]),
        card("数据从哪来", "两份投影", [["结构", "anc.board/v2"], ["运行态", "anc.runtime/v1"], ["缺一份", "图仍然画，缺口在页脚说明"]]))),
  ];
}

// 系统 —— 基础设施类信息集中到一页。
// 运行矩阵 / 网关 / 数据流 / 热层 / 授权，这些是「异常时出现、正常时收起」的东西，
// 不该各占一条主导航。旧 hash（#/runtime 等）仍能打开，落到对应标签。
type SystemTab = { id: string; label: string; hint: string; view: (d: Data) => Kid[] };

const SYSTEM_TABS: SystemTab[] = [
  { id: "runtime", label: "运行与连接", hint: "每个 bot 真的能回话吗", view: renderRuntime },
  { id: "gateway", label: "网关与授权", hint: "执行面配置 + 谁被允许做什么", view: (d) => [...renderGateway(d), ...renderGrants(d)] },
  { id: "dataflow", label: "数据流", hint: "通道开着吗、手里有什么", view: renderDataflow },
  { id: "hot", label: "热层", hint: "还没落库的事", view: renderHot },
];

function activeSystemTab(): SystemTab {
  const here = currentId();
  const alias = here === "grants" ? "gateway" : here;
  const q = (hashQuery().get("tab") ?? "").trim();
  return SYSTEM_TABS.find((t) => t.id === q)
    ?? SYSTEM_TABS.find((t) => t.id === alias)
    ?? SYSTEM_TABS[0]!;
}

function systemHref(id: string): string {
  const who = (hashQuery().get("who") ?? "").trim();
  if (id === "gateway" && who !== "") return "#/system?tab=gateway&who=" + encodeURIComponent(who);
  return "#/system?tab=" + id;
}

function renderSystem(d: Data): Kid[] {
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

// 页面注册表。顺序就是侧栏顺序；分组是「问题」，页名是「答案」。
// 这一版把导航收敛成六组：基础设施不再各占一条，组织关系落到「成员与 Agent」。
const overviewPage: Page = {
  id: "overview", group: "总览", label: "总览", icon: "overview", wired: true,
  question: "现在有没有事、要不要我动手？",
  sub: "一张运营态势图：校验、待批、运行、热层、资产，五路信号汇到同一个视野里。",
  view: renderOverview,
};
const approvalsPage: Page = {
  id: "approvals", group: "总览", label: "待批", icon: "check", wired: true,
  question: "哪几件事在等人点头、该谁点？", view: renderApprovals,
};
const graphPage: Page = {
  id: "graph", group: "组织", label: "关系图谱", icon: "graph", wired: true,
  question: "业务域、项目、成员、bot 是怎么挂在一起的？",
  sub: "节点全部来自真相源与运行态投影；连线只表示「挂在哪」，不表示业务流。",
  view: renderGraph,
};
const membersPage: Page = {
  id: "members", group: "组织", label: "成员管理", icon: "users", wired: true,
  question: "谁在负责什么、身上挂着哪些 Agent、现在在做什么？",
  sub: "一人一张卡，点开是身份、名下 Agent 的接入与在线状态、当前在做的事；每一段关系都标了推导依据。",
  view: renderMembers,
};
const domainsPage: Page = {
  id: "domains", group: "组织", label: "业务域", icon: "grid", wired: true,
  question: "一块业务是什么、谁在做、数据在哪？", view: renderDomains,
};
const projectsPage: Page = {
  id: "projects", group: "工作", label: "项目", icon: "layers", wired: true,
  question: "谁在做什么、到几号、卡住找谁、进度到哪？",
  sub: "周期时间消耗按原文日期推算；交付进度没有真相源时明确留白。",
  view: renderProjects,
};
const contextPage: Page = {
  id: "context", group: "工作", label: "上下文", icon: "context", wired: true,
  question: "一个 agent 的工作上下文是什么？", view: renderContext,
};
const timelinePage: Page = {
  id: "timeline", group: "记录", label: "时间线", icon: "clock", wired: true,
  question: "卡在哪、谁在跟、下一步是谁的决定？", view: renderTimeline,
};
const casesPage: Page = {
  id: "cases", group: "记录", label: "卡点", icon: "alert", wired: true,
  question: "现在哪几件事正在卡住、谁该动？", view: renderCases,
};
const auditPage: Page = {
  id: "audit", group: "记录", label: "审计", icon: "shield", wired: true,
  question: "谁在什么时候、对谁、行使了什么（含被拒的）？", view: renderAudit,
};
const assetsPage: Page = {
  id: "assets", group: "资产", label: "原料", icon: "box", wired: true,
  question: "手上有什么料、多久没动了？", view: renderAssets,
};
const sedimentPage: Page = {
  id: "sediment", group: "资产", label: "沉淀", icon: "strata", wired: true,
  question: "料怎么分层、哪一层现在是真数？", view: renderSediment,
};
const systemPage: Page = {
  id: "system", group: "系统", label: "运行与连接", icon: "pulse", wired: true,
  question: "基础设施还健康吗、执行面和授权有没有缺口？",
  sub: "运行矩阵 / 网关 / 授权 / 数据流 / 热层集中在这里；正常时不打扰，异常时才要人看。",
  view: renderSystem,
};

// ---------- 上下文页：一个 agent 的工作上下文 ----------
//
// 读者：FDE / 想看清「这个 agent 眼里的世界」的人。
//
// 这一页**不另立一套字段**：后端 /api/context 回的 pack 就是 internal/ctxpack 那两份结构，
// 也就是 harness 走 MCP 要的那一份、`anc context pack` 打的那一份 —— 同一份上下文，换一个摆法。
//
// 选中谁进 URL（`#/context?who=alice&domain=trade`）：刷新、把链接发给别人、前进后退，
// 都还停在那个人 / 那一域上。这一页**只读**：一个字都不写回真相源。

function ctxQuery(): string {
  const q = hashQuery();
  const who = (q.get("who") ?? "").trim();
  const domain = (q.get("domain") ?? "").trim();
  const p = new URLSearchParams();
  if (who !== "") p.set("who", who);
  if (domain !== "") p.set("domain", domain);
  const s = p.toString();
  return s === "" ? "" : "?" + s;
}

function ctxHref(who: string, domain = ""): string {
  const p = new URLSearchParams({ who });
  if (domain !== "") p.set("domain", domain);
  return "#/context?" + p.toString();
}

// 授权表那一页的视角（`#/grants?who=bizbot`）—— 同一条理由：位置进 URL，
// 刷新 / 前进后退 / 把链接发给别人都还停在同一个主体上；不带就是全量。
function grantsQuery(): string {
  const who = (hashQuery().get("who") ?? "").trim();
  if (who === "") return "";
  const p = new URLSearchParams();
  p.set("who", who);
  return "?" + p.toString();
}

function ctxChip(label: string, href: string, on: boolean, mono = false): HTMLElement {
  const attrs: Attrs = { class: mono ? "chip mono" : "chip", href };
  if (on) attrs["aria-current"] = "true";
  return el("a", attrs, label);
}

function ctxWhoSection(v: ContextView): Kid {
  if (v.roster.length === 0) {
    return section("可以看谁", "名单来自真相源（members/ 与 agents.md）",
      emptyNote("真相源里没有成员，也没有业务 agent。"));
  }
  const row = el("div", { class: "legend" });
  for (const e of v.roster) {
    const name = (e.name ?? "").trim();
    const label = name !== "" && name !== e.ref ? name + " · " + e.ref : dash(e.ref);
    row.append(ctxChip(label, ctxHref(e.ref), e.ref === v.who, e.kind === "agent"));
  }
  const out: Kid[] = [section("可以看谁",
    "名单来自真相源：成员 bot 与业务 agent 接在同一张口上 —— 「这是谁」也是真相源说的，不许自报",
    row)];
  // 范围：不写 domain = 它负责的全部域；写一个 = 只要那一块（同 CLI 的 --domain）。
  const p = v.pack;
  if (p && !p.refused && p.who) {
    const scopes = el("div", { class: "legend" },
      ctxChip("全部域", ctxHref(p.who), v.domain === ""));
    for (const d of p.domains ?? []) {
      scopes.append(ctxChip(d.name + " · " + d.slug, ctxHref(p.who, d.slug), v.domain === d.slug, true));
    }
    out.push(section("看多大范围", "要单独一域时不给别域目录、也不附通用报缺",
      p.domains && p.domains.length > 0 ? scopes : emptyNote("这个人名下还没有域，所以没有范围可选。")));
  }
  return el("div", {}, ...out);
}

function ctxIdentitySection(p: CtxPack): Kid {
  const id = p.identity;
  if (!id) {
    return section("身份", "这一份里没有身份那一块", emptyNote("——"));
  }
  const rows: Array<[string, Kid]> = [
    ["类型", id.kind === "agent" ? "业务 agent · 代岗位" : "成员 bot · 代人"],
    ["编号", el("span", { class: "mono" }, dash(id.ref))],
    ["显示名", dash(id.name)],
    ["岗位", dash(id.role)],
    ["归属", id.domain ? dash(id.domain_name) + "（" + id.domain + "）" : cannot("没划域 —— 没有域就没有数据视野")],
    ["铭文", id.inscription ? dash(id.inscription) : cannot("没有铭文（成员 bot 不在这张表上）")],
  ];
  if (id.evidence) {
    rows.push(["铭文出处", el("span", { class: "mono" }, id.evidence.file + ":" + id.evidence.line)]);
  }
  return section("身份", "三格全部来自真相源：归属取自域表，铭文取自 agents.md",
    el("div", { class: "grid" }, card(dash(id.name), id.kind, rows)));
}

function ctxDomainsSection(p: CtxPack): Kid {
  const rows: Kid[][] = (p.domains ?? []).map((d) => [
    el("span", { class: "mono nw" }, d.slug), dash(d.name), dash(d.what),
    el("span", { class: "mono nw" }, dash(d.data)), dash(d.who),
    dash(d.terms), dash(d.sources), ptr(d.evidence),
  ]);
  return section("业务域（你负责的）", "全行：域是什么 / 数据在哪 / 找谁 / 指回真相源的那一行",
    rows.length === 0
      ? emptyNote(p.note !== "" && p.note !== undefined ? p.note : "没有域 —— 公司还没给你划域。")
      : table(["slug", "名称", "是什么", "数据在哪", "找谁", "术语口径", "来源", "证据"], rows));
}

function ctxDirectorySection(p: CtxPack): Kid {
  const rows: Kid[][] = (p.directory ?? []).map((d) => [
    el("span", { class: "mono nw" }, d.slug), dash(d.name), dash(d.what), dash(d.who),
  ]);
  return section("别域（只给「找谁」）", "跨域按「找谁」接头 —— 走人，不走近道（数据默认不通）",
    rows.length === 0 ? emptyNote("没有别域。") : table(["slug", "名称", "是什么", "找谁"], rows));
}

function ctxSkillsSection(p: CtxPack): Kid {
  const rows: Kid[][] = (p.skills ?? []).map((s) => [
    el("span", { class: "mono nw" }, s.name),
    el("span", { class: "mono nw" }, s.where),
    s.missing ? cannot("正文还没落") : "在",
  ]);
  return section("技能从哪来", "清单在 roles/<岗位> 的 skills:，正文在 skills/<名>/SKILL.md",
    rows.length === 0
      ? emptyNote("这个岗位在 roles/ 里没声明 skills —— 不是「没有技能」，是「没登记」。")
      : table(["技能", "正文指针", "正文"], rows));
}

function ctxProjectsSection(p: CtxPack): Kid {
  const rows: Kid[][] = (p.projects ?? []).map((x) => [
    el("span", { class: "mono nw" }, x.slug), dash(x.name), dash(x.owner),
    dash(x.period), sourceCell(x.source ?? ""), ptr(x.evidence),
  ]);
  return section("在跟的项目", "只列你可见域上的；period / source 原样搬（它们本身是指针，ANC 不解读）",
    rows.length === 0
      ? emptyNote("你可见的域上还没挂项目。")
      : table(["slug", "项目", "负责人", "周期", "真源", "证据"], rows));
}

// 「派给我的待办 / 我在做的」—— 反向下发的那一步落在一个人眼里长什么样。
//
// 为什么是这两摞而不是一张任务表：ANC **不往 agent 的会话里插话**（插了就是打断），
// 派活拆成两件各自可替换的事 —— 决定派给谁（ANC 的活，落在热层）、让对方开始动（对方 pull）。
// 这一块就是那个 pull 的可见面：会话一开始要一次上下文，「派给我的」就在里面。
// 看板这一页只摆出来看：不认领、不改状态（要动是 `anc hot` 那边的事）。
function ctxTasksSection(
  list: CtxTask[] | undefined, bands: Record<string, string> | undefined,
  title: string, hint: string, emptyWhat: string,
): Kid {
  const rows: Kid[][] = (list ?? []).map((t) => [
    dotLine(bandDot(bands?.[t.id] ?? "unknown"), statusCell(t.status)),
    el("div", {}, el("span", { class: "mono" }, dash(t.id)), el("div", {}, dash(t.title))),
    handOff(t.by, t.to),
    dash([t.domain, t.project].filter((x) => (x ?? "").trim() !== "").join(" · ")),
    t.as_of ? localTime(t.as_of) : cannot("没写 as_of"),
    (t.note ?? "").trim() === "" ? "" : el("p", { class: "empty" }, dash(t.note)),
  ]);
  return section(title, hint,
    rows.length === 0
      ? emptyNote(emptyWhat)
      : table(["档 / status", "编号 · 这一件", "谁 → 交给谁", "业务域 · 项目", "当前值是什么时候的", "备注"], rows));
}

function ctxGapsSection(p: CtxPack): Kid {
  const gaps = p.gaps ?? [];
  return section("这份上下文答不了什么", "报缺，不编 —— 读的人据此知道该去问谁，而不是以为这就是全部",
    gaps.length === 0
      ? emptyNote("没报缺：要么都答上了，要么这次只问了一域（那就不附通用报缺）。")
      : el("ul", {}, ...gaps.map((g) => el("li", {}, g))));
}

function renderContext(d: Data): Kid[] {
  const v = d.context;
  if (!v) {
    return [banner("bad", "读不到工作上下文", [
      el("span", {}, d.contextErr === "" ? "服务没回 anc.context/v1" : d.contextErr)])];
  }
  const out: Kid[] = [];
  if ((v.error ?? "") !== "") {
    out.push(banner("bad", "真相源读不动", [el("span", {}, dash(v.error))]));
  }
  out.push(ctxWhoSection(v));
  const p = v.pack;
  if (!p) {
    out.push(section("挑一个", "选中的人进 URL：刷新、分享、前进后退都还停在同一个人身上",
      emptyNote("还没有选。上面挑一个 —— 这一页会摆出它眼里的世界。")));
    return out;
  }
  if (p.refused) {
    out.push(banner("warn", "没答出来", [
      el("span", {}, dash(p.refused.reason)),
      p.refused.ask ? el("span", {}, "该找谁：" + p.refused.ask) : "",
    ]));
    out.push(section("这一页不做什么", "拒了就是拒了 —— 不给半份数据",
      emptyNote("被拒的那一次照样是一次「想行使」：它记在审计流水里，不记在这一页上。")));
    return out;
  }
  out.push(ctxIdentitySection(p));
  // 反向下发的那一半：agent 自己 pull 到的就是这两摞。
  // 热层没接上时**不摆空列表** —— 那会把「看不到」读成「没有」，正是这一页最该防的假绿。
  if (!v.tasks_wired) {
    out.push(banner("warn", "派给我的待办：这一段答不了", [
      el("span", {}, dash(v.tasks_why)),
      el("br"),
      el("span", {}, "接不上不代表「没人派给你」—— 这一页宁可不显示，也不拿一份空清单冒充它。"),
    ]));
  } else {
    out.push(ctxTasksSection(p.todo, v.task_bands, "派给我的待办",
      "热层的 to = 你。这一页只读：派谁、派什么在 ANC 那边，改状态用 anc hot",
      "没人派给你 —— 热层接上了，里面确实没有 to 是你的。"));
    out.push(ctxTasksSection(p.doing, v.task_bands, "我在做的",
      "热层的 by = 你。状态原样摆（done / running / blocked / failed，认不出的照收）",
      "你手上没有在做的 —— 热层接上了，里面确实没有 by 是你的。"));
    if ((p.todo ?? []).length + (p.doing ?? []).length > 0) out.push(bandLegend());
  }
  out.push(ctxDomainsSection(p));
  out.push(ctxDirectorySection(p));
  out.push(ctxSkillsSection(p));
  out.push(ctxProjectsSection(p));
  out.push(ctxGapsSection(p));
  out.push(section("这一份是什么时候算的", "投影是纯函数产物：同一份真相源，谁算都一样",
    el("p", { class: "empty" },
      "算于 " + (p.at ? localTime(p.at) : "—") +
      (p.scope ? "，范围只到 " + p.scope + " 这一域" : "，范围是它负责的全部域") + "。",
      "这一页只摆出来看，不落盘；要一份冻住的副本用 ", el("span", { class: "mono" }, "anc context freeze"),
      "（落 vault 的 context/，进 git）。")));
  return out;
}
// ---------- 沉淀柱（口径图） ----------
//
// 「沉淀」页那根柱**不是统计**，它画的是四层的分工：
//   料落进来 → 重复与噪声在「丢掉」那一层当场散掉（所以那一层不增厚）
//            → 还在发生的事住「热层」（带保鲜期，过期自己碎掉，所以它便宜）
//            → 达标的落「库」变成经验（落下的就是资产，永久）
//            → 里面夹的明文，在落库之前被抹成指针（原文件改名，指针指向真源）
//
// 每层右边写的是**真数**：能对上的才写数，对不上的如实写「未接入」——
// 不拿动画冒充数据。同一批料的落点每次刷新都一样（固定种子），可复现才可校对。
const MASK_MARK = "已脱敏";

// 审计里「读出来就已经被抹过」的条数 —— 「隐匿」那一层唯一能对上的真数。
// 它从流水原文里数出来，不是估的；一条都没有就照写 0。
function maskedCount(v: AuditView | null): number | null {
  if (!v) return null;
  let n = 0;
  for (const r of v.records) {
    const text = (r.why ?? "") + " " + (r.detail ?? "") + " " + (r.object ?? "");
    if (text.indexOf(MASK_MARK) >= 0) n++;
  }
  return n;
}

// 一批料（固定的一批，不是随机的）：每一颗是一个**类别**，不是一个数。
type SedKind = "live" | "dead" | "masked" | "settled";
type SedToken = { kind: SedKind; band: string; label: string };

const SED_TOKENS: SedToken[] = [
  { kind: "live",    band: "hot",   label: "session · 一轮对话" },
  { kind: "dead",    band: "noise", label: "repeat · 重复的话" },
  { kind: "live",    band: "hot",   label: "decision · 一个决定" },
  { kind: "dead",    band: "noise", label: "noise · 没写完的半句" },
  { kind: "masked",  band: "mask",  label: "明文 · 一把 key" },
  { kind: "settled", band: "cold",  label: "经验 · 沉下来的" },
  { kind: "live",    band: "hot",   label: "case · 一个卡点" },
  { kind: "dead",    band: "noise", label: "repeat · 重复的话" },
  { kind: "settled", band: "cold",  label: "立项书 · 副本" },
  { kind: "masked",  band: "mask",  label: "明文 · 一份凭据" },
];

// 一颗料的宽度：按字数现算（中文比西文宽近一倍），不写死 px —— 改字不用改样式。
function tokenWidth(s: string): number {
  let w = 18;
  for (const ch of s) w += ch.charCodeAt(0) > 0x2e80 ? 9.6 : 5.4;
  return Math.round(w);
}

// 图例一行：色块 + 名字 + 一句话 + **真数**（右边的数就是这个图层现在的实况）。
function sedRow(sw: string, name: string, n: string, text: string): HTMLElement {
  return el("div", { class: "li" },
    el("span", { class: "sw " + sw }),
    el("div", {}, el("b", {}, name), el("p", {}, text)),
    el("div", { class: "n" }, n));
}

function sedimentSection(d: Data): Kid {
  const a = d.assets;
  const files = a ? a.dirs.reduce((n, x) => n + x.files, 0) : null;
  const dirs = a ? a.dirs.length : null;
  const charters = a ? a.charters : null;
  const masked = maskedCount(d.audit);
  const num = (v: number | null): string => (v === null ? "读不到" : String(v));
  const no = "未接入";
  // 热层那一格。三个条件缺一个都问不出「有几条」：接口回了 / 接上了 / 读得动。
  // 缺一个就如实写「未接入」或「读不到」—— **0 会被读成「热层是空的」**，那是在编。
  const h = d.hot;
  const hotCnt = (): string => {
    const v = d.hot;
    if (v === null || !v.wired) return no;
    if ((v.error ?? "") !== "") return "读不到";
    return num(v.total) + " 条";
  };
  const hotBandNote = (): string => {
    if (!h) return "还在发生的事，带保鲜期";
    if (!h.wired) return "没接上 —— 读不到，不等于里面是空的";
    if ((h.error ?? "") !== "") return "接上了，但这会儿读不动";
    return h.stale > 0 ? "还在发生的事；" + h.stale + " 条不新鲜" : "还在发生的事，带保鲜期";
  };
  const hotText = (): string => {
    const base = "还在发生的事。带保鲜期，过期自己碎掉 —— 所以它便宜";
    if (!h) return base;
    if (!h.wired) return base + "（" + (h.why ?? "") + "）";
    if ((h.error ?? "") !== "") return base + "（" + h.error + "）";
    return base + "（保鲜期 " + durText(h.ttl_secs) + " · 超过 " + durText(h.stale_after_secs) + " 没动就标不新鲜）";
  };
  const hotFoot = (): string => {
    if (!h) return "热层那一格没问出来：" + (d.hotErr === "" ? "服务没回 anc.hot/v1" : d.hotErr) + "。";
    if (!h.wired) return (h.why ?? "热层没接上") + "。所以那一格是「未接入」—— 它不等于「热层是空的」。";
    if ((h.error ?? "") !== "") return "热层接上了，但这会儿读不动：" + h.error + "。";
    return "热层那一格的数来自 /api/hot：现在 " + num(h.total) + " 条，其中 " + h.stale + " 条不新鲜。";
  };

  const band = (key: string, cls: string, label: string, cnt: string, note: string): HTMLElement =>
    el("div", { class: "strata " + cls, "data-band": key },
      el("div", { class: "lbl" }, label),
      el("div", { class: "cnt" }, cnt),
      el("div", { class: "note" }, note));

  const column = el("div", { class: "column" },
    band("open", "s-open", "料口", "", "会话与文件都从这落进来"),
    band("noise", "s-noise", "丢掉", no, "重复、噪声、没写完的半句"),
    band("hot", "s-hot", "热层 · 缓存", hotCnt(), hotBandNote()),
    band("cold", "s-cold", "落库 · 经验", num(files) + " 个文件", "达标才落，落下的就是资产"),
    band("mask", "s-mask", "隐匿 · 指针", num(masked) + " 条", "明文不落库：落库前抹成指针"),
    el("div", { class: "stream" }));

  const legend = el("div", { class: "legendbox" },
    el("div", { class: "card" },
      sedRow("w-noise", "丢掉", no, "重复、噪声、没写完的半句 —— 当场蒸发，不进任何一层"),
      sedRow("w-hot", "热层 · 缓存", hotCnt(), hotText()),
      sedRow("w-cold", "落库 · 经验", num(files),
        "达标了才落。落下的这一条就是资产，永久（本机 " + num(dirs) + " 个目录 · 立项书副本 " + num(charters) + "）"),
      sedRow("w-mask", "隐匿 · 指针", num(masked),
        "明文不落库：落盘前抹成「" + MASK_MARK + ":规则名」的指针，原文件改名归档")));

  return section("沉淀柱", "口径图，不是统计 · 一层比一层更贵、更少、更不该丢",
    el("div", { class: "sedi" }, column, legend),
    el("p", { class: "note" },
      "柱是「口径图」—— 它画的是四层的分工，不是统计；层右边那一格是真数，读不到就写「未接入」或「读不到」，不拿空列表冒充「里面是空的」。",
      hotFoot(),
      "「丢掉」那一格没有数据源 —— 丢弃发生在写入之前，落盘里不留痕迹；四层分法拍板之后再接。"));
}

// 落料：柱子插进 DOM 之后再算落点（层高是按布局量出来的，不是写死的数）。
// motion=false 时（重取数据后重画）直接把料摆到那一层上：不落，但一层不缺 ——
// 刷新不该把柱子刷空，也不该把同一屏重播一遍。
function paintSediment(scope: HTMLElement, motion: boolean): void {
  const column = scope.querySelector(".column");
  if (!(column instanceof HTMLElement)) return;
  const bands = new Map<string, HTMLElement>();
  column.querySelectorAll<HTMLElement>(".strata").forEach((s) => {
    const k = s.getAttribute("data-band");
    if (k !== null) bands.set(k, s);
  });
  const stream = column.querySelector(".stream");
  if (!(stream instanceof HTMLElement)) return;
  const still = !motion || window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  let seed = 20261012;
  const rnd = (): number => {
    seed = (seed * 1103515245 + 12345) & 0x7fffffff;
    return seed / 0x7fffffff;
  };
  // 落点：左边那段留给层的字，右边按格子排（同一层不叠）
  const left = 196;
  const usable = Math.max(120, column.clientWidth - left - 84);
  const seat = new Map<string, number>();
  SED_TOKENS.forEach((t, i) => {
    const at = bands.get(t.band);
    if (!at) return;
    const n = seat.get(t.band) ?? 0;
    seat.set(t.band, n + 1);
    const top = at.offsetTop;
    const dead = t.band === "noise";
    const mask = t.kind === "masked";
    const row = dead ? 0 : Math.floor(n / 2);
    const col = dead ? 0 : n % 2;
    const y = dead
      ? top + 26 + rnd() * 22
      : Math.min(top + 40 + row * 28, top + at.offsetHeight - 26);
    const tk = el("div",
      { class: "tk" + (dead ? " dead" : "") + (mask ? " masked" : "") + (still ? " still" : "") },
      el("span", { class: "txt" }, t.label),
      mask ? el("span", { class: "ptr" }, "→ 指针 · 飞书") : "");
    tk.style.setProperty("--x", (left + col * (usable / 2) + (dead ? rnd() * 40 : 0)).toFixed(0) + "px");
    tk.style.setProperty("--w1", tokenWidth(t.label) + "px");
    tk.style.setProperty("--w2", tokenWidth("→ 指针 · 飞书") + "px");
    tk.style.setProperty("--to", y.toFixed(0) + "px");
    tk.style.setProperty("--mid", (top + 6 + rnd() * 24).toFixed(0) + "px");
    tk.style.setProperty("--dur", (1.45 + rnd() * 0.6).toFixed(2) + "s");
    tk.style.setProperty("--d", (i * 0.1).toFixed(2) + "s");
    tk.style.setProperty("--g", (i * 0.1 + 0.84).toFixed(2) + "s");
    stream.append(tk);
  });
}
const PAGES: Page[] = [
  overviewPage, approvalsPage,
  projectsPage, contextPage,
  graphPage, membersPage, domainsPage,
  timelinePage, casesPage, auditPage,
  assetsPage, sedimentPage,
  systemPage,
];

// 旧 hash 仍要能打开：这些页并进「系统」或「成员与 Agent」之后，书签和飞书里的旧链接不能断。
// 只改写落点，不改语义 —— 页面自己会按 tab 参数决定显示哪一段。
const LEGACY: Record<string, string> = {
  runtime: "system", dataflow: "system", gateway: "system", hot: "system", grants: "system",
  roles: "members", org: "overview",
};

function pageFor(id: string): Page {
  const to = LEGACY[id];
  if (to !== undefined) return to === "members" ? membersPage : to === "system" ? systemPage : overviewPage;
  for (const p of PAGES) if (p.id === id) return p;
  return overviewPage;
}

function currentId(): string {
  return location.hash.replace(/^#\/?/, "").split("?")[0]!.trim();
}

// hash 里的参数（`#/timeline?p=2`）—— 翻页状态进 URL：刷新、前进后退、把链接发给别人，
// 都还停在那一页。不引入路由器，也不引入第二份状态。
function hashQuery(): URLSearchParams {
  const i = location.hash.indexOf("?");
  return new URLSearchParams(i < 0 ? "" : location.hash.slice(i + 1));
}

const pageNo = (q: URLSearchParams, key = "p"): number => {
  const n = Number.parseInt((q.get(key) ?? "").trim(), 10);
  return Number.isFinite(n) && n > 0 ? n : 1;
};

// 一屏铺多少行。翻页是**视图**的事，不是门禁 —— 取多少由 limit 决定（见 fetchData），
// 翻页只决定这一屏铺哪一段。所以这里不拦任何人，也不改变任何接口的语义。
const PER_PAGE = 20;

// 翻页条：只有一页就**不出现**（少一行噪音），越界自动夹回第一页（不报错、不假装有第 9 页）。
function pager(total: number, page: number, id: string): Kid {
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
function sliced<T>(list: T[], q: URLSearchParams): { rows: T[]; cur: number; pages: number } {
  const pages = Math.max(1, Math.ceil(list.length / PER_PAGE));
  const cur = Math.min(pageNo(q), pages);
  return { rows: list.slice((cur - 1) * PER_PAGE, cur * PER_PAGE), cur, pages };
}

// ---------- 外壳与路由 ----------

function refreshButton(): HTMLElement {
  const b = el("button", { class: "refresh" }, "刷新");
  b.addEventListener("click", () => { void boot(true); });
  return b;
}

function topline(d: Data | null, page: Page): HTMLElement {
  const i = d ? d.issues : null;
  const c = d && d.board ? d.board.company : null;
  const meta = el("div", { class: "meta" },
    i ? el("span", { class: "chip" }, el("span", { class: statusDot(i) }), statusText(i)) : "",
    d && d.board ? chip("生成 " + localTime(d.board.generated_at)) : "",
    d && d.board ? chip("schema " + d.board.schema, true) : "",
    refreshButton());
  return el("div", { class: "topline" },
    el("div", { class: "brandrow" },
      el("span", { class: "glyph" }),
      el("span", { class: "nm" }, "ANC"),
      el("span", { class: "rule" }),
      el("span", { class: "co" }, c ? dash(c.name) : "还没读到投影",
        c ? el("span", { class: "sub" }, dash(c.platform) + " · " + dash(c.language) + " · " + dash(c.timezone)) : "")),
    el("div", { class: "spacer" }),
    meta);
}

// 页头：整页唯一的大字。它只问那一句话，答案在下面几节里。
function hero(page: Page): HTMLElement {
  return el("section", { class: "hero" }, el("h1", {}, page.label), el("p", { class: "ask" }, page.question));
}

function rail(page: Page): HTMLElement {
  // 分组：组的标题是问题本身，页名才是答案。PAGES 的顺序就是分组顺序 —— 加页只要写对 group。
  const tabs = el("nav", { class: "tabs" });
  let group = "";
  let items = el("div", { class: "gitems" });
  for (const p of PAGES) {
    if (p.group !== group) {
      if (group !== "") tabs.append(el("div", { class: "navgroup" }, el("div", { class: "glabel" }, group), items));
      group = p.group;
      items = el("div", { class: "gitems" });
    }
    items.append(el("a", { class: p.id === page.id ? "tab active" : "tab", href: "#/" + p.id },
      el("span", { class: "lab" }, p.label),
      el("span", { class: p.wired ? "res on" : "res off" }, p.wired ? "已接入" : "未接入")));
  }
  if (group !== "") tabs.append(el("div", { class: "navgroup" }, el("div", { class: "glabel" }, group), items));
  return el("aside", { class: "rail" }, tabs);
}

function footer(): HTMLElement {
  return el("footer", {},
    el("div", {}, el("strong", {}, "几乎只读"),
      "：数据是真相源（vault）的实时投影，点「刷新」重取；全站唯一能写的一处是「待批」页的点头，",
      "它只留痕（timeline + audit），不动真相源。"),
    el("div", {}, el("strong", {}, "校验发现"), " 与 ", el("span", { class: "mono" }, "anc org check"),
      " 同源：红档拦住落盘，非红档只回显；完整清单在「总览」。"),
    el("div", {}, el("strong", {}, "分页口径"),
      "：一页回答一个问题。还没接数据源的页如实写明缺什么、依赖哪个议题，不编数据。"));
}

// 上一屏是哪一页 —— 用它决定要不要放进场动效。**刷新不算换页**（刷新时不动）：
// 看板是天天看的工具，每次重取都全屏重播一遍进场，很快就变成噪音。
let lastPageId = "";

function legacyShell(): void {
  if (!root) return;
  const page = pageFor(currentId());
  const d = cache;
  const entering = d !== null && page.id !== lastPageId;
  const col = el("div", { class: "col" });
  col.append(hero(page));
  if (!d) {
    col.append(el("div", { class: "glass pad" },
      el("div", { class: "loading", style: "font-size:12.5px;margin-bottom:14px" }, "正在读取真相源…"),
      el("div", { class: "sk", style: "width:58%" }),
      el("div", { class: "sk", style: "width:82%" }),
      el("div", { class: "sk", style: "width:44%" })));
  } else {
    for (const kid of page.view(d)) col.append(kid);
    col.append(footer());
    lastPageId = page.id;
  }
  // 进场：换页时每节各进一次，错峰 ≤ 0.3s（动效预算：只进，不循环）。
  // 翻页不触发 —— 那还是同一页，动起来只会让人以为「换了内容」。
  if (entering) {
    const h1 = col.querySelector("header.top h1");
    if (h1) h1.classList.add("sweep");
    Array.from(col.querySelectorAll("section")).forEach((sec, i) => {
      sec.classList.add("reveal");
      (sec as HTMLElement).style.animationDelay = String(Math.min(i, 5) * 0.06) + "s";
    });
  }
  document.title = page.label + " · " + (d && d.board ? dash(d.board.company.name) : "ANC 看板");
  root.replaceChildren(el("div", { class: "wrap" },
    topline(d, page),
    el("div", { class: "shell" }, rail(page), col)));
  // 沉淀柱：进这一页时落一次（落定就停住）；重取数据后重画只是把料摆回那一层上，不再播一遍。
  if (col.querySelector(".column")) paintSediment(col, entering);
}

let cache: Data | null = null;

async function boot(force = false): Promise<void> {
  if (force) cache = null;
  if (cache === null) {
    shell();
    cache = await fetchData();
  }
  shell();
}

// 授权表那一页的取数：换视角时只重取这一份（其余几份不用动）。
async function loadGrants(): Promise<void> {
  const res = await getJSON<GrantsView>("/api/grants" + grantsQuery());
  const gr = contract(res, "授权表");
  if (cache) cache = { ...cache, grants: gr.v, grantsErr: gr.err };
  shell();
}

// 上下文那一页的取数：换人 / 换范围时就重取那一份（其余几份不用动）。
// 只重取它自己，不是整页重取 —— 这一页的选择是**参数**，不是刷新。
async function loadContext(): Promise<void> {
  const res = await getJSON<ContextView>("/api/context" + ctxQuery());
  const cx = contract(res, "工作上下文");
  if (cache) cache = { ...cache, context: cx.v, contextErr: cx.err };
  shell();
}

window.addEventListener("hashchange", () => {
  // 这一页的「选中谁」在 hash 里：进了这一页就按 URL 重取一次，别的页照旧（取数只在启动 / 刷新）。
  if (pageFor(currentId()).id === "context") {
    void loadContext();
    return;
  }
  // 授权表并进了「系统 · 网关与授权」：换视角只重取它自己（那是**参数**，不是刷新）。
  if (pageFor(currentId()).id === "system" && activeSystemTab().id === "gateway" && (hashQuery().get("who") ?? "") !== "") {
    void loadGrants();
    return;
  }
  shell();
});

// ---------- 新外壳：抽屉、命令面板、提示 ----------
//
// 这一层只做「把人带到该看的地方」：详情抽屉不写任何东西，命令面板只做导航。
// 看板仍然只有一个写口 —— 「待批」页的点头。

type DrawerSpec = { eyebrow: string; title: string; sub: string; body: Kid[] };
type PaletteItem = { label: string; hint: string; icon: string; href?: string; run?: () => void };

let drawerLayer: HTMLElement | null = null;
let drawerEyebrow: HTMLElement | null = null;
let drawerTitle: HTMLElement | null = null;
let drawerSub: HTMLElement | null = null;
let drawerBody: HTMLElement | null = null;
let paletteLayer: HTMLElement | null = null;
let paletteInput: HTMLInputElement | null = null;
let paletteList: HTMLElement | null = null;
let toastEl: HTMLElement | null = null;
let toastTimer = 0;
let paletteResults: PaletteItem[] = [];
let paletteActive = 0;

function showToast(msg: string): void {
  if (!toastEl) {
    toastEl = el("div", { class: "toast", role: "status" });
    document.body.append(toastEl);
  }
  toastEl.textContent = msg;
  toastEl.classList.add("on");
  window.clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => toastEl?.classList.remove("on"), 2200);
}

function openDrawer(spec: DrawerSpec): void {
  if (!drawerLayer || !drawerBody || !drawerTitle || !drawerSub || !drawerEyebrow) return;
  drawerEyebrow.textContent = spec.eyebrow;
  drawerTitle.textContent = spec.title;
  drawerSub.textContent = spec.sub;
  drawerBody.replaceChildren(...spec.body);
  drawerLayer.dataset.open = "1";
  document.body.style.overflow = "hidden";
}

function closeDrawer(): void {
  if (!drawerLayer) return;
  drawerLayer.dataset.open = "0";
  document.body.style.overflow = "";
}

function closePalette(): void {
  if (!paletteLayer) return;
  paletteLayer.dataset.open = "0";
  document.body.style.overflow = "";
}

function paletteItems(): PaletteItem[] {
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

function runPaletteItem(item: PaletteItem): void {
  closePalette();
  if (item.href) {
    location.hash = item.href;
    return;
  }
  item.run?.();
}

function setPaletteActive(i: number): void {
  if (!paletteList || paletteResults.length === 0) return;
  paletteActive = (i + paletteResults.length) % paletteResults.length;
  Array.from(paletteList.children).forEach((n, j) => n.classList.toggle("active", j === paletteActive));
  const active = paletteList.children[paletteActive];
  if (active instanceof HTMLElement) active.scrollIntoView({ block: "nearest" });
}

function renderPalette(q: string): void {
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

function openPalette(): void {
  if (!paletteLayer || !paletteInput) return;
  paletteLayer.dataset.open = "1";
  paletteInput.value = "";
  renderPalette("");
  document.body.style.overflow = "hidden";
  paletteInput.focus();
}

function paletteKey(e: KeyboardEvent): void {
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
  if (drawerLayer?.dataset.open === "1") { e.preventDefault(); closeDrawer(); }
});

// ---------- 总览背景：一张只画「结构」的神经场 ----------
//
// 节点来自真实投影：业务域 / 项目 / 成员 / bot。连线只表示「挂在哪」，不表示
// 任何业务含义；脉冲只是让它看起来是活的，不承载数据。零数据时只画中心点，
// 不拿装饰节点冒充「有东西」。
type FieldNode = { x: number; y: number; r: number; color: string; label: string; kind: string };
type FieldLink = { a: number; b: number; color: string; seed: number; bend: number };

let fieldRaf = 0;
let fieldResize: ResizeObserver | null = null;

function stopField(): void {
  if (fieldRaf) cancelAnimationFrame(fieldRaf);
  fieldRaf = 0;
  fieldResize?.disconnect();
  fieldResize = null;
}

function mountField(scope: HTMLElement, d: Data): void {
  stopField();
  const canvas = scope.querySelector<HTMLCanvasElement>(".field-canvas");
  if (!canvas) return;
  const ctx = canvas.getContext("2d");
  if (!ctx) return;
  const light = currentTheme() === "light";
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const nodes: FieldNode[] = [];
  const links: FieldLink[] = [];
  // 尘埃与轨道只是**气氛**，不带标签、不参与命中 —— 有数据的节点永远比它们亮。
  const dust: Array<{ x: number; y: number; r: number; a: number; ph: number }> = [];
  let W = 0;
  let H = 0;
  let dpr = 1;
  let span = 0;
  const TAU = Math.PI * 2;

  const build = (): void => {
    W = Math.max(320, canvas.clientWidth);
    H = Math.max(260, canvas.clientHeight);
    dpr = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = Math.round(W * dpr);
    canvas.height = Math.round(H * dpr);
    nodes.length = 0;
    links.length = 0;
    const cx = W * 0.52;
    const cy = H * 0.43;
    span = Math.min(W, H);
    dust.length = 0;
    for (let i = 0; i < 56; i++) {
      const a = i * 2.39996;
      const rr = Math.sqrt((i + 0.5) / 56);
      dust.push({
        x: cx + Math.cos(a) * rr * W * 0.46,
        y: cy + Math.sin(a) * rr * H * 0.42,
        r: 0.5 + (i % 3) * 0.28,
        a: 0.05 + (i % 5) * 0.012,
        ph: i * 0.7,
      });
    }
    nodes.push({ x: cx, y: cy, r: 5.5, color: light ? "36,38,34" : "236,236,232", label: "ANC", kind: "core" });

    const domainPos = new Map<string, number>();
    const domains = d.board ? d.board.domains : [];
    const dn = Math.min(domains.length, 14);
    for (let i = 0; i < dn; i++) {
      const x = domains[i]!;
      const a = 0.2 + (i / Math.max(1, dn)) * TAU;
      const idx = nodes.length;
      nodes.push({ x: cx + Math.cos(a) * span * 0.27, y: cy + Math.sin(a) * span * 0.23, r: 4.2, color: light ? "158,118,38" : "230,207,152", label: x.name, kind: "domain" });
      domainPos.set(x.slug, idx);
      links.push({ a: 0, b: idx, color: light ? "158,118,38" : "230,207,152", seed: (i * 0.17) % 1, bend: 0.10 });
    }

    const projectPos = new Map<string, number>();
    const projects = d.board ? d.board.projects : [];
    const pn = Math.min(projects.length, 22);
    for (let i = 0; i < pn; i++) {
      const x = projects[i]!;
      const a = 1.25 + (i / Math.max(1, pn)) * TAU;
      const idx = nodes.length;
      nodes.push({ x: cx + Math.cos(a) * span * 0.42, y: cy + Math.sin(a) * span * 0.34, r: 3.4, color: light ? "104,96,178" : "195,188,242", label: x.name, kind: "project" });
      projectPos.set(x.slug, idx);
      links.push({ a: domainPos.get(x.domain) ?? 0, b: idx, color: light ? "104,96,178" : "195,188,242", seed: (i * 0.13 + 0.3) % 1, bend: 0.14 });
    }

    const members = d.board ? d.board.members : [];
    const mn = Math.min(members.length, 36);
    for (let i = 0; i < mn; i++) {
      const x = members[i]!;
      const a = 2.4 + (i / Math.max(1, mn)) * TAU;
      const idx = nodes.length;
      nodes.push({ x: cx + Math.cos(a) * span * 0.56, y: cy + Math.sin(a) * span * 0.44, r: 2.4, color: light ? "42,132,108" : "172,224,207", label: x.display_name || x.name, kind: "member" });
      let di = 0;
      for (const s of x.domains) {
        const hit = domainPos.get(s);
        if (hit !== undefined) { di = hit; break; }
      }
      links.push({ a: di, b: idx, color: light ? "42,132,108" : "172,224,207", seed: (i * 0.11 + 0.6) % 1, bend: 0.08 });
    }

    const bots = d.runtime && d.runtime.wired ? d.runtime.bots : [];
    const bn = Math.min(bots.length, 20);
    for (let i = 0; i < bn; i++) {
      const x = bots[i]!;
      const a = 0.75 + (i / Math.max(1, bn)) * TAU;
      const idx = nodes.length;
      nodes.push({ x: cx + Math.cos(a) * span * 0.36, y: cy + Math.sin(a) * span * 0.30, r: 3.0, color: light ? "52,122,152" : "172,214,232", label: x.project, kind: "bot" });
      links.push({ a: projectPos.get(x.project) ?? 0, b: idx, color: light ? "52,122,152" : "172,214,232", seed: (i * 0.19 + 0.4) % 1, bend: 0.12 });
    }
  };

  const qp = (a: FieldNode, b: FieldNode, mx: number, my: number, u: number): { x: number; y: number } => {
    const v = 1 - u;
    return { x: v * v * a.x + 2 * v * u * mx + u * u * b.x, y: v * v * a.y + 2 * v * u * my + u * u * b.y };
  };

  const draw = (now: number): void => {
    const t = now / 1000;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);
    const core = nodes[0]!;
    const halo = ctx.createRadialGradient(core.x, core.y, 0, core.x, core.y, 92);
    halo.addColorStop(0, light ? "rgba(168,130,50,0.05)" : "rgba(230,207,152,0.18)");
    halo.addColorStop(1, light ? "rgba(168,130,50,0)" : "rgba(230,207,152,0)");
    ctx.fillStyle = halo;
    ctx.beginPath();
    ctx.arc(core.x, core.y, 92, 0, TAU);
    ctx.fill();

    // 同心轨道：只是尺度参照，不表示任何层级或流向。
    ctx.save();
    ctx.setLineDash([2, 8]);
    for (let k = 1; k <= 3; k++) {
      ctx.strokeStyle = light
        ? "rgba(30,32,28," + (0.075 - k * 0.012) + ")"
        : "rgba(236,236,232," + (0.042 - k * 0.008) + ")";
      ctx.lineWidth = 0.6;
      ctx.beginPath();
      ctx.arc(core.x, core.y, span * (0.16 + k * 0.12), 0, TAU);
      ctx.stroke();
    }
    ctx.restore();

    for (const p of dust) {
      const tw = 0.5 + 0.5 * Math.sin(t * 0.45 + p.ph);
      ctx.fillStyle = light
        ? "rgba(58,62,54," + (p.a * (0.55 + 0.45 * tw) * 1.8).toFixed(3) + ")"
        : "rgba(240,236,224," + (p.a * (0.35 + 0.65 * tw)).toFixed(3) + ")";
      ctx.beginPath();
      ctx.arc(p.x, p.y + Math.sin(t * 0.16 + p.ph) * 5, p.r, 0, TAU);
      ctx.fill();
    }

    for (const l of links) {
      const a = nodes[l.a];
      const b = nodes[l.b];
      if (!a || !b) continue;
      const mx = (a.x + b.x) / 2 + (b.y - a.y) * l.bend;
      const my = (a.y + b.y) / 2 - (b.x - a.x) * l.bend;
      ctx.strokeStyle = "rgba(" + l.color + ",0.13)";
      ctx.lineWidth = 0.8;
      ctx.beginPath();
      ctx.moveTo(a.x, a.y);
      ctx.quadraticCurveTo(mx, my, b.x, b.y);
      ctx.stroke();
      if (reduced) continue;
      const u = (t * 0.10 + l.seed) % 1;
      const p = qp(a, b, mx, my, u);
      const pulse = ctx.createRadialGradient(p.x, p.y, 0, p.x, p.y, 6);
      pulse.addColorStop(0, "rgba(" + l.color + ",0.85)");
      pulse.addColorStop(1, "rgba(" + l.color + ",0)");
      ctx.fillStyle = pulse;
      ctx.beginPath();
      ctx.arc(p.x, p.y, 6, 0, TAU);
      ctx.fill();
    }

    for (const n of nodes) {
      const glow = ctx.createRadialGradient(n.x, n.y, 0, n.x, n.y, n.r * 5);
      glow.addColorStop(0, "rgba(" + n.color + "," + (light ? (n.kind === "core" ? 0.16 : 0.13) : 0.55) + ")");
      glow.addColorStop(1, "rgba(" + n.color + ",0)");
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(n.x, n.y, n.r * 5, 0, TAU);
      ctx.fill();
      ctx.fillStyle = "rgba(" + n.color + ",0.95)";
      ctx.beginPath();
      ctx.arc(n.x, n.y, n.r, 0, TAU);
      ctx.fill();
      if ((n.kind === "domain" || n.kind === "project") && n.label !== "") {
        ctx.fillStyle = light ? "rgba(28,30,26,0.78)" : "rgba(236,236,233,0.68)";
        ctx.font = "10.5px -apple-system, 'PingFang SC', sans-serif";
        ctx.fillText(n.label.slice(0, 12), n.x + n.r + 5, n.y + 3);
      }
    }
    if (!reduced) fieldRaf = requestAnimationFrame(draw);
  };

  const start = (): void => {
    build();
    if (fieldRaf) cancelAnimationFrame(fieldRaf);
    if (reduced) draw(performance.now());
    else fieldRaf = requestAnimationFrame(draw);
  };
  fieldResize = new ResizeObserver(start);
  fieldResize.observe(canvas);
  start();
}

// ---------- 关系图谱：可缩放、可点开节点的结构图 ----------
//
// 与总览那张「氛围场」不同：这一张是**可操作**的。滚轮缩放、拖动平移、悬停高亮、
// 点节点打开详情。节点和连线都来自同一份投影 —— 图上没有装饰节点。
type OrgNode = {
  x: number; y: number; r: number; color: string; kind: string;
  label: string; sub: string; facts: Array<[string, string]>;
};
type OrgEdge = { a: number; b: number; color: string; seed: number };

let orgRaf = 0;
let orgResize: ResizeObserver | null = null;

function stopOrgGraph(): void {
  if (orgRaf) cancelAnimationFrame(orgRaf);
  orgRaf = 0;
  orgResize?.disconnect();
  orgResize = null;
}

function mountOrgGraph(scope: HTMLElement, d: Data): void {
  stopOrgGraph();
  const canvas = scope.querySelector<HTMLCanvasElement>(".org-canvas");
  if (!canvas) return;
  const ctx = canvas.getContext("2d");
  if (!ctx) return;
  const light = currentTheme() === "light";
  const b = d.board;
  if (!b) return;
  const bots = d.runtime && d.runtime.wired ? d.runtime.bots : [];
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const nodes: OrgNode[] = [];
  const edges: OrgEdge[] = [];
  const transform = { x: 0, y: 0, k: 1 };
  let W = 0;
  let H = 0;
  let dpr = 1;
  let hover = -1;
  let dragging = false;
  let moved = 0;
  let lastX = 0;
  let lastY = 0;

  const build = (): void => {
    W = Math.max(360, canvas.clientWidth);
    H = Math.max(360, canvas.clientHeight);
    dpr = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = Math.round(W * dpr);
    canvas.height = Math.round(H * dpr);
    transform.x = W / 2;
    transform.y = H / 2;
    transform.k = 1;
    nodes.length = 0;
    edges.length = 0;
    nodes.push({
      x: 0, y: 0, r: 9, color: light ? "36,38,34" : "236,236,232", kind: "core", label: b.company.name || "ANC",
      sub: b.company.platform + " · " + b.company.language,
      facts: [["公司", b.company.name], ["平台", b.company.platform], ["业务域", String(b.domains.length)], ["项目", String(b.projects.length)]],
    });
    const domainIndex = new Map<string, number>();
    b.domains.forEach((x, i) => {
      const a = -Math.PI / 2 + (i / Math.max(1, b.domains.length)) * Math.PI * 2;
      const idx = nodes.length;
      nodes.push({
        x: Math.cos(a) * W * 0.24, y: Math.sin(a) * H * 0.26, r: 6.5, color: light ? "158,118,38" : "230,207,152", kind: "domain",
        label: x.name, sub: x.slug,
        facts: [["是什么", x.what], ["谁在做", x.who_label], ["数据在哪", x.data], ["口径", x.terms]],
      });
      domainIndex.set(x.slug, idx);
      edges.push({ a: 0, b: idx, color: light ? "158,118,38" : "230,207,152", seed: (i * 0.17) % 1 });
    });
    const projectIndex = new Map<string, number>();
    b.projects.forEach((x, i) => {
      const host = nodes[domainIndex.get(x.domain) ?? 0]!;
      const a = Math.atan2(host.y, host.x) + ((i % 5) - 2) * 0.10;
      const rad = 0.34 + (i % 4) * 0.028;
      const idx = nodes.length;
      nodes.push({
        x: Math.cos(a) * W * rad, y: Math.sin(a) * H * rad, r: 5.2, color: light ? "104,96,178" : "195,188,242", kind: "project",
        label: x.name, sub: x.slug,
        facts: [["挂哪块业务", x.domain], ["卡住找谁", x.owner_label], ["周期", x.period], ["真源", x.source]],
      });
      projectIndex.set(x.slug, idx);
      edges.push({ a: domainIndex.get(x.domain) ?? 0, b: idx, color: light ? "104,96,178" : "195,188,242", seed: (i * 0.13 + 0.3) % 1 });
    });
    b.members.forEach((x, i) => {
      const dom = x.domains.find((s) => domainIndex.has(s));
      const host = nodes[domainIndex.get(dom ?? "") ?? 0]!;
      const a = Math.atan2(host.y, host.x) + ((i % 7) - 3) * 0.075;
      const rad = 0.40 + (i % 3) * 0.02;
      const idx = nodes.length;
      nodes.push({
        x: Math.cos(a) * W * rad, y: Math.sin(a) * H * rad, r: 3.8, color: light ? "42,132,108" : "172,224,207", kind: "member",
        label: x.display_name || x.name, sub: x.role + (x.disabled ? " · 已停用" : ""),
        facts: [["角色", x.role], ["模型", x.model], ["业务域", x.domains.join("、") || "—"], ["状态", x.disabled ? "已停用" : x.admin ? "启用 · 管理员" : "启用"]],
      });
      edges.push({ a: domainIndex.get(dom ?? "") ?? 0, b: idx, color: light ? "42,132,108" : "172,224,207", seed: (i * 0.11 + 0.6) % 1 });
    });
    agentsOf(b).forEach((x, i) => {
      const dom = domainIndex.has(x.domain) ? x.domain : "";
      const host = nodes[domainIndex.get(dom) ?? 0]!;
      const a = Math.atan2(host.y, host.x) + 0.55 + ((i % 5) - 2) * 0.09;
      const rad = 0.38 + (i % 3) * 0.02;
      const idx = nodes.length;
      const st = botStateOf(d, botProject(b, x.slug));
      nodes.push({
        x: Math.cos(a) * W * rad, y: Math.sin(a) * H * rad, r: 4.6,
        color: light ? "168,67,99" : "227,179,196", kind: "agent",
        label: x.name || x.slug, sub: x.role,
        facts: [["slug", x.slug], ["铭文", x.description || "—"], ["业务域", x.domain || "—"],
          ["岗位", x.role || "—"], ["运行态", st.label], ["harness", x.harness || "默认"]],
      });
      edges.push({ a: domainIndex.get(dom) ?? 0, b: idx, color: light ? "168,67,99" : "227,179,196", seed: (i * 0.23 + 0.2) % 1 });
    });
    bots.forEach((x, i) => {
      const hit = projectIndex.get(x.project);
      const host = nodes[hit ?? 0]!;
      const a = Math.atan2(host.y, host.x) + ((i % 3) - 1) * 0.13;
      const rad = 46 + (i % 2) * 18;
      const idx = nodes.length;
      nodes.push({
        x: host.x + Math.cos(a) * rad, y: host.y + Math.sin(a) * rad, r: 4.2, color: light ? "52,122,152" : "172,214,232", kind: "bot",
        label: x.project, sub: x.state,
        facts: [["project", x.project], ["状态", runState(x.state).label], ["为什么", x.why]],
      });
      edges.push({ a: hit ?? 0, b: idx, color: light ? "52,122,152" : "172,214,232", seed: (i * 0.19 + 0.4) % 1 });
    });
    let maxX = 0;
    let maxY = 0;
    for (const n of nodes) {
      const labelW = n.kind === "domain" || n.kind === "project" || n.kind === "core" ? 160 : 0;
      maxX = Math.max(maxX, Math.abs(n.x) + n.r + labelW);
      maxY = Math.max(maxY, Math.abs(n.y) + n.r + 16);
    }
    const fit = Math.min((W / 2 - 24) / Math.max(1, maxX), (H / 2 - 24) / Math.max(1, maxY));
    transform.k = Math.max(0.55, Math.min(1, fit));
  };

  const project = (n: OrgNode): { x: number; y: number } => ({
    x: transform.x + n.x * transform.k, y: transform.y + n.y * transform.k,
  });

  const hitAt = (sx: number, sy: number): number => {
    let best = -1;
    let bestD = 18;
    for (let i = 0; i < nodes.length; i++) {
      const p = project(nodes[i]!);
      const dd = Math.hypot(p.x - sx, p.y - sy);
      if (dd < Math.max(bestD, nodes[i]!.r * transform.k + 6)) { best = i; bestD = dd; }
    }
    return best;
  };

  const draw = (now: number): void => {
    const t = now / 1000;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);
    const core = project(nodes[0]!);
    const halo = ctx.createRadialGradient(core.x, core.y, 0, core.x, core.y, 150 * transform.k);
    halo.addColorStop(0, light ? "rgba(168,130,50,0.045)" : "rgba(230,207,152,0.10)");
    halo.addColorStop(1, light ? "rgba(168,130,50,0)" : "rgba(230,207,152,0)");
    ctx.fillStyle = halo;
    ctx.beginPath();
    ctx.arc(core.x, core.y, 150 * transform.k, 0, Math.PI * 2);
    ctx.fill();

    for (const e of edges) {
      const a = project(nodes[e.a]!);
      const bb = project(nodes[e.b]!);
      ctx.strokeStyle = "rgba(" + e.color + "," + (light ? "0.24" : "0.15") + ")";
      ctx.lineWidth = 0.9;
      ctx.beginPath();
      ctx.moveTo(a.x, a.y);
      ctx.lineTo(bb.x, bb.y);
      ctx.stroke();
      if (!reduced) {
        const u = (t * 0.12 + e.seed) % 1;
        const px = a.x + (bb.x - a.x) * u;
        const py = a.y + (bb.y - a.y) * u;
        const g = ctx.createRadialGradient(px, py, 0, px, py, 5);
        g.addColorStop(0, "rgba(" + e.color + ",0.9)");
        g.addColorStop(1, "rgba(" + e.color + ",0)");
        ctx.fillStyle = g;
        ctx.beginPath();
        ctx.arc(px, py, 5, 0, Math.PI * 2);
        ctx.fill();
      }
    }
    nodes.forEach((n, i) => {
      const p = project(n);
      const r = Math.max(2.4, n.r * transform.k);
      const on = i === hover;
      const glow = ctx.createRadialGradient(p.x, p.y, 0, p.x, p.y, r * (on ? 8 : 4.5));
      glow.addColorStop(0, "rgba(" + n.color + "," + (light ? (on ? 0.20 : 0.12) : (on ? 0.55 : 0.34)) + ")");
      glow.addColorStop(1, "rgba(" + n.color + ",0)");
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(p.x, p.y, r * (on ? 8 : 4.5), 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = "rgba(" + n.color + ",0.97)";
      ctx.beginPath();
      ctx.arc(p.x, p.y, r, 0, Math.PI * 2);
      ctx.fill();
      if (on) {
        ctx.strokeStyle = light ? "rgba(20,22,18,0.55)" : "rgba(255,255,255,0.5)";
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.arc(p.x, p.y, r + 6, 0, Math.PI * 2);
        ctx.stroke();
      }
      if ((n.kind === "domain" || n.kind === "project" || n.kind === "core") && n.label !== "") {
        const size = n.kind === "core" ? 12 : 10.5;
        ctx.font = "500 " + size + "px -apple-system, 'PingFang SC', sans-serif";
        ctx.fillStyle = light
          ? "rgba(26,28,24," + (n.kind === "core" ? 0.9 : 0.72) + ")"
          : "rgba(236,236,233," + (n.kind === "core" ? 0.86 : 0.62) + ")";
        const label = n.label.length > 14 ? n.label.slice(0, 14) + "…" : n.label;
        ctx.fillText(label, p.x + r + 7, p.y + 3.5);
      }
    });
    if (!reduced) orgRaf = requestAnimationFrame(draw);
  };

  const openNode = (i: number): void => {
    const n = nodes[i];
    if (!n) return;
    openDrawer({
      eyebrow: n.kind.toUpperCase() + " / 关系图谱", title: n.label, sub: n.sub,
      body: [card(n.label, n.sub, n.facts)],
    });
  };

  const onMove = (ev: MouseEvent): void => {
    const rect = canvas.getBoundingClientRect();
    const sx = ev.clientX - rect.left;
    const sy = ev.clientY - rect.top;
    if (dragging) {
      const dx = ev.clientX - lastX;
      const dy = ev.clientY - lastY;
      moved += Math.abs(dx) + Math.abs(dy);
      transform.x += dx;
      transform.y += dy;
      lastX = ev.clientX;
      lastY = ev.clientY;
      return;
    }
    hover = hitAt(sx, sy);
    canvas.style.cursor = hover >= 0 ? "pointer" : "grab";
  };
  const onDown = (ev: MouseEvent): void => {
    dragging = true;
    moved = 0;
    lastX = ev.clientX;
    lastY = ev.clientY;
    canvas.style.cursor = "grabbing";
  };
  const onUp = (ev: MouseEvent): void => {
    if (dragging && moved < 5) {
      const rect = canvas.getBoundingClientRect();
      const i = hitAt(ev.clientX - rect.left, ev.clientY - rect.top);
      if (i >= 0) openNode(i);
    }
    dragging = false;
    canvas.style.cursor = "grab";
  };
  const onWheel = (ev: WheelEvent): void => {
    ev.preventDefault();
    const rect = canvas.getBoundingClientRect();
    const sx = ev.clientX - rect.left;
    const sy = ev.clientY - rect.top;
    const wx = (sx - transform.x) / transform.k;
    const wy = (sy - transform.y) / transform.k;
    const k = Math.max(0.55, Math.min(2.6, transform.k * (ev.deltaY < 0 ? 1.12 : 0.89)));
    transform.k = k;
    transform.x = sx - wx * k;
    transform.y = sy - wy * k;
  };
  canvas.addEventListener("mousemove", onMove);
  canvas.addEventListener("mousedown", onDown);
  canvas.addEventListener("mouseup", onUp);
  canvas.addEventListener("mouseleave", () => { hover = -1; dragging = false; });
  canvas.addEventListener("wheel", onWheel, { passive: false });
  canvas.addEventListener("dblclick", () => { build(); });

  build();
  orgResize = new ResizeObserver(() => { build(); if (reduced) draw(performance.now()); });
  orgResize.observe(canvas);
  if (reduced) draw(performance.now());
  else orgRaf = requestAnimationFrame(draw);
}

// ---------- 新外壳：侧栏 / 顶栏 / 页头 ----------
let lastShellPage = "";

function navBadge(p: Page, d: Data | null): Kid {
  if (!d) return "";
  if (p.id === "approvals" && d.approvals?.wired && d.approvals.count > 0) {
    return el("span", { class: "nav-badge hot" }, String(d.approvals.count));
  }
  if (p.id === "system" && d.runtime?.wired) {
    const n = d.runtime.bots.filter((b) => b.state === "fail").length;
    if (n > 0) return el("span", { class: "nav-badge hot" }, String(n));
  }
  if (p.id === "cases" && d.timeline?.wired) {
    const n = d.timeline.cases.filter((c) => c.band === "red").length;
    if (n > 0) return el("span", { class: "nav-badge hot" }, String(n));
  }
  if (p.id === "system" && d.hot?.wired && d.hot.stale > 0) {
    return el("span", { class: "nav-badge gold" }, String(d.hot.stale));
  }
  if (p.id === "audit" && d.audit) {
    const n = d.audit.bands.denied + d.audit.bands.failed;
    if (n > 0) return el("span", { class: "nav-badge hot" }, String(n));
  }
  return "";
}

function sidebar(page: Page, d: Data | null): HTMLElement {
  const nav = el("nav", { class: "side-nav", "aria-label": "主导航" });
  let group = "";
  let box: HTMLElement | null = null;
  for (const p of PAGES) {
    if (p.group !== group) {
      group = p.group;
      box = el("div", { class: "nav-group" }, el("div", { class: "nav-label" }, group), el("div", { class: "nav-rail" }));
      nav.append(box);
    }
    const a = el("a", {
      class: p.id === page.id ? "nav-item active" : "nav-item",
      href: "#/" + p.id,
      title: p.question,
    }, iconBox(p.icon), el("span", { class: "lab" }, p.label), el("span", { class: "nav-hint" }, p.question), navBadge(p, d));
    if (p.id === page.id) a.setAttribute("aria-current", "page");
    box?.append(a);
  }
  const c = d?.board?.company;
  const status = d ? d.issues : null;
  const pending = d && d.approvals && d.approvals.wired ? d.approvals.count : null;
  const cases = d && d.timeline && d.timeline.wired ? d.timeline.cases.filter((x) => x.band === "red").length : null;
  const bots = d && d.runtime && d.runtime.wired ? d.runtime.bots : [];
  const botRed = d && d.runtime && d.runtime.wired ? bots.filter((x) => x.state === "fail").length : null;
  const meter = (label: string, n: number | null, cls: string): HTMLElement =>
    el("div", { class: "side-meter " + cls },
      el("span", {}, label), el("b", {}, n === null ? "—" : String(n)));
  const foot = el("div", { class: "side-foot" },
    el("div", { class: "side-health" },
      el("span", { class: status ? statusDot(status) : "dot" }),
      el("span", {}, status ? statusText(status) : "读取中"),
      el("span", { class: "micro" }, d?.board ? localTime(d.board.generated_at) : "")),
    el("div", { class: "side-meters" },
      meter("待批", pending, "gold"), meter("卡点", cases, "red"), meter("bot 红", botRed, "red")),
    el("div", { class: "micro env" }, d?.board ? d.board.schema : "vault · waiting"),
    el("div", { class: "foot-links" },
      el("a", { href: "#/overview" }, "总览"),
      el("a", { href: "#/timeline" }, "时间线"),
      el("span", {}, "只读观测")));
  const collapse = el("button", { class: "collapse-btn", type: "button", "aria-label": "收起或展开侧栏", title: "收起 / 展开侧栏" }, icon("chevronL", 14));
  collapse.addEventListener("click", () => {
    const app = document.querySelector(".app");
    app?.classList.toggle("rail-min");
    try { localStorage.setItem("anc.rail.min", app?.classList.contains("rail-min") ? "1" : "0"); } catch { /* 隐私模式不记 */ }
  });
  return el("aside", { class: "side" },
    el("div", { class: "side-brand" },
      el("span", { class: "brand-orb" }),
      el("span", { class: "bt" }, el("b", {}, "ANC"), el("small", {}, "AI-NATIVE COMPANY")),
      collapse),
    el("div", { class: "side-workspace" },
      el("span", { class: "ws-dot" }),
      el("span", { class: "ws-text" },
        el("b", {}, c ? dash(c.name) : "还没读到投影"),
        el("small", {}, c ? dash(c.platform) + " · " + dash(c.timezone) : "vault · waiting"))),
    nav, foot);
}

function copyLink(): void {
  const url = location.href;
  if (navigator.clipboard && navigator.clipboard.writeText) {
    void navigator.clipboard.writeText(url).then(
      () => showToast("链接已复制"),
      () => showToast("复制失败，手动复制地址栏"),
    );
    return;
  }
  showToast("当前浏览器不支持剪贴板");
}

function topbar(page: Page, d: Data | null): HTMLElement {
  const menu = el("button", { class: "icon-btn menu-btn", type: "button", "aria-label": "打开导航" }, icon("menu", 15));
  menu.addEventListener("click", () => document.querySelector(".app")?.classList.toggle("nav-open"));
  const cmd = el("button", { class: "cmd-btn", type: "button", "aria-label": "搜索与跳转" },
    icon("search", 14), el("span", { class: "label" }, "搜索页面、成员、项目、Agent…"), el("span", { class: "kbd" }, "Ctrl K"));
  cmd.addEventListener("click", openPalette);
  const refresh = el("button", { class: "icon-btn", type: "button", "aria-label": "刷新", title: "刷新" }, icon("refresh", 15));
  refresh.addEventListener("click", () => { void boot(true); });
  const dark = currentTheme() === "dark";
  const themeBtn = el("button", { class: "icon-btn", type: "button", "aria-label": dark ? "切换到浅色" : "切换到深色", title: dark ? "切换到浅色" : "切换到深色" }, icon(dark ? "sun" : "moon", 15));
  themeBtn.addEventListener("click", () => applyTheme(dark ? "light" : "dark"));
  const copy = el("button", { class: "icon-btn", type: "button", "aria-label": "复制本页链接", title: "复制本页链接" }, icon("copy", 15));
  copy.addEventListener("click", copyLink);
  const i = d ? d.issues : null;
  const pill = el("span", { class: "status-pill " + (i ? healthState(i) : "warn") },
    el("span", { class: i ? statusDot(i) : "dot" }), i ? statusText(i) : "读取中");
  const live = d && d.board ? el("span", { class: "live-chip" }, el("i"), "LIVE") : "";
  return el("header", { class: "topbar" },
    menu,
    el("div", { class: "crumbs" },
      el("span", { class: "grp" }, page.group),
      el("span", { class: "sep" }, "/"),
      el("b", {}, page.label)),
    cmd,
    el("div", { class: "top-actions" },
      live,
      pill,
      d?.board ? el("span", { class: "meta-pill" }, "生成 " + localTime(d.board.generated_at)) : "",
      themeBtn, refresh, copy));
}

function pageHead(page: Page): HTMLElement {
  const copy = el("button", { class: "ghost-btn", type: "button" }, icon("copy", 14), "复制链接");
  copy.addEventListener("click", copyLink);
  const idx = PAGES.findIndex((x) => x.id === page.id) + 1;
  const no = String(Math.max(idx, 1)).padStart(2, "0");
  return el("section", { class: "page-head" },
    el("div", { class: "ph-main" },
      el("p", { class: "eyebrow" },
        el("span", { class: "ph-no" }, no), page.group, el("span", { class: "ph-sep" }, "/"), el("span", {}, page.id)),
      el("h1", {}, page.label),
      el("p", { class: "ask" }, page.question),
      page.sub ? el("p", { class: "ph-sub" }, page.sub) : ""),
    el("div", { class: "page-actions" },
      page.id === "overview" ? "" : el("a", { class: "ghost-btn", href: "#/overview" }, icon("overview", 14), "总览"),
      copy));
}

function pageFooter(): HTMLElement {
  return el("footer", { class: "note" },
    el("strong", {}, "几乎只读"), "：数据是真相源（vault）的实时投影，点刷新重取；全站唯一能写的一处是「待批」页的点头，",
    "它只留痕（timeline + audit），不动真相源。",
    el("br"),
    el("strong", {}, "校验发现"), " 与 ", el("span", { class: "mono" }, "anc org check"), " 同源；完整清单在「总览」和「审计」。");
}

function loadingBlock(): HTMLElement {
  return el("div", { class: "glass pad" },
    el("div", { class: "loading", style: "font-size:12.5px;margin-bottom:14px" }, "正在读取真相源…"),
    el("div", { class: "sk", style: "width:58%" }),
    el("div", { class: "sk", style: "width:82%" }),
    el("div", { class: "sk", style: "width:44%" }));
}

// 一页的渲染出错，只该让这一页报错，不该把整个外壳留在「读取中」。
// 报出来的是页面 id 和原始错误 —— 不猜原因，也不把异常吞掉。
function pageCrash(page: Page, err: unknown): HTMLElement {
  const msg = err instanceof Error ? err.message : String(err);
  return banner("bad", "这一页渲染失败", [
    el("span", { class: "mono" }, page.id),
    el("br"),
    el("span", {}, msg),
    el("br"),
    el("span", { class: "quiet" }, "其余页面不受影响；刷新可重取一次数据。"),
  ]);
}

function shell(): void {
  if (!root) return;
  const page = pageFor(currentId());
  const d = cache;
  const entering = d !== null && page.id !== lastShellPage;
  stopField();
  stopOrgGraph();
  if (drawerLayer?.dataset.open === "1") closeDrawer();
  if (paletteLayer?.dataset.open === "1") closePalette();

  const col = el("div", { class: "col" });
  col.append(pageHead(page));
  if (!d) {
    col.append(loadingBlock());
  } else {
    try {
      for (const kid of page.view(d)) col.append(kid);
    } catch (err) {
      col.append(pageCrash(page, err));
    }
    col.append(pageFooter());
    lastShellPage = page.id;
  }
  if (entering) {
    Array.from(col.querySelectorAll("section")).forEach((sec, i) => {
      if (sec.classList.contains("page-head")) return;
      sec.classList.add("reveal");
      (sec as HTMLElement).style.animationDelay = String(Math.min(i, 6) * 0.055) + "s";
    });
    // 卡片再晚半拍落位：一层是页面分区，一层是分区里的实体。
    Array.from(col.querySelectorAll(".entity-card")).forEach((n, i) => {
      n.classList.add("rise");
      (n as HTMLElement).style.animationDelay = String(0.06 + Math.min(i, 10) * 0.045) + "s";
    });
  }
  document.title = page.label + " · " + (d?.board ? dash(d.board.company.name) : "ANC 看板");

  const app = el("div", { class: "app" },
    sidebar(page, d),
    el("div", { class: "side-scrim" }),
    el("div", { class: "main" },
      topbar(page, d),
      el("main", { class: "pagewrap" }, el("div", { class: "page page-" + page.id }, col)),
      el("footer", { class: "statusbar" },
        el("span", { class: "sb-item" }, el("i", { class: "sb-dot" }), d?.board ? dash(d.board.company.name) : "ANC 看板"),
        el("span", { class: "sb-item" }, "schema ", el("b", {}, d?.board ? d.board.schema : "—")),
        el("span", { class: "sb-item" }, d?.board ? "生成 " + localTime(d.board.generated_at) : "等待投影"),
        el("span", { class: "sb-grow" }),
        el("span", { class: "sb-item" }, "几乎只读 · 唯一写口在待批"),
        el("span", { class: "sb-item" }, "Ctrl K"))));
  try {
    if (localStorage.getItem("anc.rail.min") === "1") app.classList.add("rail-min");
  } catch { /* 隐私模式不读 */ }

  drawerEyebrow = el("div", { class: "eyebrow" });
  drawerTitle = el("h2");
  drawerSub = el("div", { class: "dsub" });
  drawerBody = el("div", { class: "drawer-body" });
  const dClose = el("button", { class: "icon-btn", type: "button", "aria-label": "关闭详情" }, icon("close", 15));
  dClose.addEventListener("click", closeDrawer);
  drawerLayer = el("div", { class: "drawer-layer", "data-open": "0" },
    el("div", { class: "drawer-scrim" }),
    el("aside", { class: "drawer" },
      el("header", {},
        el("div", { class: "dh" }, drawerEyebrow, drawerTitle, drawerSub),
        dClose),
      drawerBody));
  drawerLayer.querySelector(".drawer-scrim")?.addEventListener("click", closeDrawer);

  paletteInput = el("input", { type: "text", placeholder: "跳转到页面、业务域、项目、成员、bot…", "aria-label": "搜索" }) as HTMLInputElement;
  paletteList = el("div", { class: "palette-list" });
  paletteLayer = el("div", { class: "palette-layer", "data-open": "0" },
    el("div", { class: "palette" },
      paletteInput, paletteList,
      el("div", { class: "palette-foot" },
        el("span", {}, "↑↓ 选择"), el("span", {}, "Enter 打开"), el("span", {}, "Esc 关闭"))));
  paletteLayer.addEventListener("click", (e) => { if (e.target === paletteLayer) closePalette(); });
  paletteInput.addEventListener("input", () => renderPalette(paletteInput?.value ?? ""));
  paletteInput.addEventListener("keydown", (e) => paletteKey(e as KeyboardEvent));

  app.append(drawerLayer, paletteLayer);
  root.replaceChildren(app);
  app.querySelector(".side-scrim")?.addEventListener("click", () => app.classList.remove("nav-open"));

  if (page.id === "overview" && d !== null) {
    requestAnimationFrame(() => mountField(col, d));
  }
  if (page.id === "graph" && d !== null) {
    requestAnimationFrame(() => mountOrgGraph(col, d));
  }
}

void boot();
