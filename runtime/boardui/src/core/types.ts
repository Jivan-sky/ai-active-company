import { root } from "./dom";
import { icon } from "./icons";

export type Company = { name: string; id: string; platform: string; language: string; timezone: string };
export type Role = { role: string; title: string; mode: string; model: string; vault_scope: string[] };
export type Member = {
  name: string; display_name: string; role: string;
  admin: boolean; disabled: boolean; model: string; domains: string[];
};
export type DomainRow = {
  slug: string; name: string; what: string; who: string; who_label: string;
  data: string; sources: string; terms: string;
};
export type ProjectRow = {
  slug: string; name: string; domain: string; owner: string; owner_label: string;
  period: string; source: string; charter: string;
};
// 业务 agent（agents.md 一行）。与 Member 是两件事：成员 bot 代人，业务 agent 代岗位 / 流程。
// 字段来自 anc.board/v2；老服务没有这一份时按空数组处理，不当成「没有 agent」下结论。
export type AgentRow = {
  slug: string; name: string; description: string; domain: string; role: string;
  harness: string; model: string; tools: string[]; wired: boolean;
};
// 业务 agent 的运行态（`/api/agents`，schema = anc.agents/v1）。
// 它与 AgentRow 是两件事：那份讲**登记了什么**，这份讲**现在谁还在心跳**。
// online 只由心跳推出；last_seen 空 = 从未心跳，不是「离线很久」。
export type LiveAgent = {
  slug: string; name: string; domain?: string; domain_name?: string; role?: string;
  harness?: string; model?: string; credentialed: boolean;
  online: boolean; stale: boolean;
  last_seen?: string; age_secs?: number;
  status?: string; task_id?: string; task_title?: string; note?: string;
  session?: string; running_tasks: number; queued_tasks: number;
};
export type AgentsView = {
  schema: string; wired: boolean; why?: string; error?: string;
  stale_after_secs: number; total: number; online: number;
  entries: LiveAgent[]; note: string;
};
export type RoutingRow = { dir: string; summary: string };
export type BoardView = {
  schema: string; generated_at: string; company: Company;
  roles: Role[]; members: Member[]; domains: DomainRow[];
  projects: ProjectRow[]; agents?: AgentRow[]; routing: RoutingRow[];
};
export type Finding = { rule: string; level: string; where: string; msg: string };
export type Issues = { ok: boolean; fatal: Finding[]; warn: Finding[]; error: string };

// 运行态观测（`/api/runtime`，schema = anc.runtime/v1）—— 它与 anc.board/v1 是**两份**
// 契约：一份讲真相源长什么样，一份讲运行态健不健康。别把两者混着用。
export type RuntimeBot = { project: string; state: string; why: string };
export type RuntimeView = {
  schema: string; wired: boolean; gateway: string; gateway_why?: string;
  bots: RuntimeBot[]; extras?: string[];
  handlers: string[]; handler_why?: string; error?: string;
};
// 数据流策略面（`/api/dataflow`，schema = anc.dataflow/v1）—— 「谁可以做什么、通道开着吗」。
// 入站只出**人数**，不出标识符：看板是观测面，不是凭据面（SPEC §2.3 / §6-3）。
export type DataflowBot = {
  project: string; role: string; mode: string; model: string;
  tools: string[]; inbound: number; inbound_extra: number;
};
export type DataflowView = {
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
export type AssetsFile = { path: string; bytes: number; modified: string };
export type AssetsDir = {
  dir: string; summary: string; files: number; bytes: number;
  newest?: string; recent: AssetsFile[]; truncated: boolean;
};
export type AssetsView = {
  schema: string; wired: boolean; dirs: AssetsDir[];
  charters: number; note: string; error?: string;
};

// 可沉淀候选（`/api/distill`，schema = anc.distill/v1）。
//
// 候选 = timeline 里**最后一条是绿档**的 case；它只是原料，不是技能。
// signals 只陈述证据（几条推进 / detail 多少字 / 几个 refs / 同名几次），后端不打分，
// 这一页也不替谁排优先级。
export type DistillCandidate = {
  case: string; title: string; domain?: string; project?: string; by?: string;
  at: string; entries: number; detail_chars: number; refs: number;
  same_title: number; same_domain: number; signals: string[];
};
export type DistillSkill = { name: string; where: string; missing: boolean; roles: string[] };
export type DistillView = {
  schema: string; wired: boolean; skills_wired: boolean;
  cases: number; green: number; candidates: DistillCandidate[];
  candidate_total: number; truncated?: boolean;
  skills: DistillSkill[]; orphans: string[]; gaps: string[];
  note: string; error?: string; skills_error?: string;
};

// 授权表（`/api/grants`，schema = anc.board.grants/v1）—— 「谁被允许对谁做什么」。
//
// 与「数据流」页的分工：那一页答「通道开着吗」（上游 config 的策略面 + 执行面事实），
// 这一页只答授权表本身。**全量、不筛选**（看板是观测面）—— 授权表里本来就没有凭据
// （SPEC §6：它记的是「谁被允许做什么」），所以这一页也没有 app_id / app_secret。
//
// 主体那一格解的是**编号身份**（成员名 / 业务 agent 的 slug），与「上下文」页同一把尺子：
// 认不出的照原样留着并标出来，不猜。
export type GrantIdentity = {
  kind: string; ref: string; name?: string; role?: string;
  domain?: string; domain_name?: string; inscription?: string;
};
export type GrantRef = {
  slot: string; raw: string; kind: string; name?: string;
  note?: string; identity?: GrantIdentity;
};
export type GrantRow = {
  slug: string; path: string; from: string; to: string; action: string; object: string;
  ttl: string; on_behalf_of: string; reason: string; body: string;
  who: GrantRef[]; hit: boolean;
};
// scope.known=false（带了个认不出的 who）时**不给全量表** —— 那会被读成「他的权限是全部」；
// 也不给一份空表冒充「他什么都没有」，而是照实说认不出。不带 who（ref 为空）= 全量，另一回事。
export type GrantScope = { ref: string; known: boolean; who?: GrantIdentity; why?: string };
export type GrantsView = {
  schema: string; wired: boolean; total: number; shown: number;
  scope: GrantScope; rows: GrantRow[]; subjects: GrantIdentity[];
  findings: Finding[]; missing: string[]; note: string; error?: string;
};

// 决策与执行的留存（`/api/timeline`，schema = anc.timeline/v1）—— 读者：跟进的人 / 老板。
// 这一页给的是**人和 agent 自己写下来的**记录（`anc timeline add`），不是从会话里推出来的。
// 三档只按 status 配色、形状跟「档」走（实心圆 / 带光环的圆 / 菱形 / 空心虚线圈，见 index.html 的 .dot）；
// **认不出的词原样留着、画空心虚线圈** —— 以后加词只改数据，不用改这里。
export type TimelineEntryView = {
  id: string; case?: string; at: string; kind?: string; status?: string;
  by?: string; role?: string; to?: string; domain?: string; project?: string;
  title: string; detail?: string; refs?: string[]; band: string;
};
export type TimelineCaseView = {
  key: string; title: string; status: string; band: string;
  first_at: string; last_at: string;
  by?: string; role?: string; to?: string; domain?: string; project?: string;
  entries: number; recent: TimelineEntryView[];
};
export type TimelineView = {
  schema: string; wired: boolean;
  counts: { green: number; yellow: number; red: number; unknown: number };
  total: number; entries: number; cases: TimelineCaseView[];
  bad: string[]; limit: number; next_before?: string;
  missing: string[]; note: string; error?: string;
};

// 行使的流水（`/api/audit`，schema = anc.audit/v1）—— 读者：管理者 / 合规感的人。
// 与「时间线」互补：那是**人 / agent 自己写下来的**留存，这是**系统记下来的**流水。
// 事实存当时的样子（object 是原样路径）；zone / cross 是按**当前**域表算出来的（派生）。
export type AuditEntryView = {
  id: string; at: string; actor: string; action: string;
  object?: string; result: string; why?: string;
  on_behalf_of?: string; tool?: string; source?: string; ref?: string; detail?: string;
  band: string; actee: string; actee_name: string;
  zone: string; cross: boolean; known: boolean; has_why: boolean;
};
export type AuditView = {
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
export type ApprovalSignal = { value: string; label: string };
export type ApprovalPending = {
  id: string; who: string; to: string; domain: string; project: string;
  title: string; body: string; enqueued: string; status: string;
  // band 是判真那一档：「在队列里」= 有人在等你给一句话（服务端给词，前端照画）。
  // 它与 status 是两回事 —— status 是提案自己写的词，band 是队列这个事实。
  band?: string;
};
export type ApprovalView = {
  schema: string; wired: boolean; why?: string; error?: string;
  write_path: string; signals: ApprovalSignal[];
  count: number; pending: ApprovalPending[];
};
// 点头的回执。两个落点是「写到了哪个文件」，不是内容 —— 内容在库里、在审计里。
export type DecideReply = {
  ok: boolean; proposal: string; verdict: string; by: string;
  timeline: string; audit: string; note: string;
};

// 一次取数、全程共用：已接入的页吃的是同一份投影，切页不重新打网络（按「刷新」才重取）。
// 热层（`/api/hot`，schema = anc.hot/v1）—— 「热层里现在住着什么」。
// 与 `/api/approvals` 是同一份底层数据、两个问题：那是**队列**（等谁点头），这是**这一层本身**。
// wired=false 与 total=0 是两件事，所以两个字段都在（同 approvals 的口径）。
export type HotEntryView = {
  id: string; status: string; stage?: string; fence?: number; attempt?: number; max_attempt?: number;
  title: string; by?: string; from?: string; to?: string;
  due_at?: string; priority?: string; overdue: boolean; overdue_secs?: number;
  root?: string; parent?: string; accept?: string; hop?: number; max_hop?: number;
  refs?: string[]; handoff?: HandoffView; review?: ReviewView;
  as_of?: string; age_secs?: number; stale: boolean;
  session?: string; intent?: string; version?: number; lease_to?: string; lease_no_expiry?: boolean;
};
export type HandoffView = {
  from: string; to: string; note?: string; refs?: string[]; cross_domain?: boolean; at: string;
};
export type ReviewView = {
  by: string; verdict: string; note?: string; at: string; self?: boolean;
};
export type HotView = {
  schema: string; wired: boolean; why?: string; error?: string;
  ttl_secs: number; stale_after_secs: number;
  total: number; stale: number; overdue: number; blocked: number; review: number; actionable: number;
  entries: HotEntryView[];
  settle_path: string; note: string;
};
export type SettleReply = {
  ok?: boolean; id?: string; by?: string; status?: string;
  already?: boolean; timeline?: string; note?: string;
};

// 工作上下文（`/api/context`，schema = anc.context/v1）—— 「一个 agent 的工作上下文是什么」。
//
// 字段名与后端（internal/ctxpack）**一字不差**：那一份同时是 agent 那条路上的契约
// （MCP 回给 harness 的就是它）。看板不另立一份字段表 —— 另立一份迟早跟那边对不上号。
export type CtxEvidence = { file: string; line: number };
export type CtxIdentity = {
  kind: string; ref: string; name?: string; role?: string;
  domain?: string; domain_name?: string; inscription?: string; evidence?: CtxEvidence;
};
export type CtxDomain = {
  slug: string; name: string; what: string; data?: string; who: string;
  terms?: string; sources?: string; evidence: CtxEvidence;
};
export type CtxBrief = { slug: string; name: string; what: string; who: string };
export type CtxSkill = { name: string; where: string; missing?: boolean };
export type CtxProject = {
  slug: string; name: string; owner?: string; period?: string; source?: string; evidence: CtxEvidence;
};
// 一件「进行中的事」（反向下发那一步的任务本体，住热层）。字段名与后端 internal/ctxpack.TaskOut
// 一字不差 —— 它同时是 agent 那条路上的契约（MCP 回给 harness 的就是它）。
export type CtxTask = {
  id: string; title?: string; status?: string; by?: string; to?: string;
  domain?: string; project?: string; note?: string; as_of?: string;
};
export type CtxPack = {
  who?: string; identity?: CtxIdentity; on_behalf_of?: string; scope?: string;
  domains?: CtxDomain[]; directory?: CtxBrief[]; skills?: CtxSkill[]; projects?: CtxProject[];
  todo?: CtxTask[]; doing?: CtxTask[];
  gaps?: string[]; note?: string; refused?: { reason: string; ask?: string }; at: string;
};
export type ContextView = {
  schema: string; who: string; domain: string;
  roster: CtxIdentity[]; pack?: CtxPack; error?: string; note: string;
  // tasks_wired / tasks_why：「派给我的待办 / 我在做的」这两摞问出来没有 ——
  // 「热层没接上」与「没人派给你」在这一页上必须是两句话。
  tasks_wired: boolean; tasks_why?: string;
  // 状态 → 配色档。词表在后端（internal/timeline.Band），这一页不另抄一份。
  task_bands?: Record<string, string>;
};

export type Data = {
  board: BoardView | null; issues: Issues;
  agents: AgentsView | null; agentsErr: string;
  hot: HotView | null; hotErr: string;
  runtime: RuntimeView | null; runtimeErr: string;
  dataflow: DataflowView | null; dataflowErr: string;
  assets: AssetsView | null; assetsErr: string;
  distill: DistillView | null; distillErr: string;
  timeline: TimelineView | null; timelineErr: string;
  audit: AuditView | null; auditErr: string;
  approvals: ApprovalView | null; approvalsErr: string;
  context: ContextView | null; contextErr: string;
  grants: GrantsView | null; grantsErr: string;
  err: string;
};

export type Attrs = Record<string, string>;
export type Kid = Node | string;

export type Page = {
  id: string;
  label: string;
  icon: string;
  group: string;   // 导航里的分组：组的标题是**问题**（现在/组织/记录/运行/原料），不是名词
  question: string;
  sub?: string;    // 页头第二行：这页读什么、边界在哪。缺省就用 question。
  wired: boolean;
  view: (d: Data) => Kid[];
};
