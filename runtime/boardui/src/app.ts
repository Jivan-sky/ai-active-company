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
type RoutingRow = { dir: string; summary: string };
type BoardView = {
  schema: string; generated_at: string; company: Company;
  roles: Role[]; members: Member[]; domains: DomainRow[];
  projects: ProjectRow[]; routing: RoutingRow[];
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
    projects: string[]; note?: string;
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
type Data = {
  board: BoardView | null; issues: Issues;
  runtime: RuntimeView | null; runtimeErr: string;
  dataflow: DataflowView | null; dataflowErr: string;
  assets: AssetsView | null; assetsErr: string;
  timeline: TimelineView | null; timelineErr: string;
  audit: AuditView | null; auditErr: string;
  approvals: ApprovalView | null; approvalsErr: string;
  err: string;
};

type Attrs = Record<string, string>;
type Kid = Node | string;

const root = document.getElementById("app");

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
  for (const i of items) ul.append(el("li", {}, i));
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
  const [boardRes, issuesRes, runtimeRes, dataflowRes, assetsRes, timelineRes, auditRes, approvalsRes] = await Promise.all([
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
  ]);
  const issues = issuesRes.body ?? emptyIssues(issuesRes.err);
  const rt = contract(runtimeRes, "运行态");
  const df = contract(dataflowRes, "数据流");
  const as = contract(assetsRes, "原料清单");
  const tl = contract(timelineRes, "时间线");
  const au = contract(auditRes, "审计");
  const ap = contract(approvalsRes, "待批队列");
  const schema = boardRes.body?.schema;
  if (boardRes.body && typeof schema === "string" && schema !== "") {
    return {
      board: boardRes.body, issues, err: "",
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
    el("span", {}, d.err === "" ? "服务返回的不是 anc.board/v1" : d.err),
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

// ---------- 页面 ----------
//
// 一页一个 Page：`id` 是 hash 路由，`question` 是这一页**唯一**要回答的问题。
// 加一页＝加一条；先问自己那句 question 写不写得出来，写不出来说明还没想清楚。

type Page = {
  id: string;
  label: string;
  group: string;   // 导航里的分组：组的标题是**问题**（现在/组织/记录/运行/原料），不是名词
  question: string;
  wired: boolean;
  view: (d: Data) => Kid[];
};

// 总览 —— 读者：所有人 / 老板。要的是「30 秒看完有没有事」，不是名词表。
function renderOverview(d: Data): Kid[] {
  const out: Kid[] = [];
  if (d.err !== "") out.push(banner("bad", "读不到投影", [el("span", {}, d.err)]));
  out.push(...alertBanners(d.issues));
  if (d.board) out.push(statSection(d.board));
  const clean = d.err === "" && d.issues.error === "" && d.issues.fatal.length === 0 && d.issues.warn.length === 0;
  if (clean) out.push(banner("warn", "没有发现", [
    el("span", {}, "校验零红档零非红档 —— 但这只说明"),
    el("strong", {}, "真相源格式没问题"),
    el("span", {}, "，不说明 agent 在跑。那件事要看「运行态」。"),
  ]));
  out.push(notWiredIndex());
  return out;
}

// 组织 —— 读者：管理者 / FDE。组织长什么样、边界在哪。
function renderOrg(d: Data): Kid[] {
  const v = d.board;
  if (!v) return [noBoard(d)];
  return [domainSection(v), roleSection(v), routingSection(v)];
}

// 项目 —— 读者：跟进的人 / 老板。谁在做什么、到几号、卡住找谁。
function renderProjects(d: Data): Kid[] {
  const v = d.board;
  if (!v) return [noBoard(d)];
  return [projectSection(v), progressGap()];
}

function progressGap(): Kid {
  return section("进度：口径还没定", "所以这一页不显示进度 —— 宁缺，不编",
    emptyNote("「进度」是活的、每天变：写进 git 真相源＝每天刷 diff、人手维护必腐烂。" +
      "候选：① 表里留一个指针、渲染时聚合（倾向）；② 完全交给 agent 在数据目录里写；③ 另开一张动态表、不进 git。" +
      "拍板前这里空着 —— 见 SPEC §13 Q15 与 GitHub #36。"));
}

// 运行态四档口径（拍板）：绿 = 近期真回过话；黄 = 运行中 / 这个窗口没观测到；红 = 卡点，需人介入；
// 灰 = 真相源里**声明**了「还没接凭据」—— 本来就不该期待它回话，单列一档，
// 免得跟黄糊在一起（混着看，整份报告都会不值得信）。
const RUN_STATE: Record<string, { dot: string; label: string }> = {
  ok: { dot: "dot ok", label: "绿 · 近期回过话" },
  warn: { dot: "dot warn", label: "黄 · 运行中 / 未观测" },
  fail: { dot: "dot bad", label: "红 · 卡点，需介入" },
  unwired: { dot: "dot gray", label: "灰 · 未接凭据" },
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

// 数据流 —— 读者：管理者 / 老板。这一页现在只有**策略面**：
//   ① 执行面（gateway 实际吃的那份 config.toml）里通道的口径 —— 上游默认开着，所以「没写」也是事实；
//   ② 真相源算出来的「每个 bot 手里有什么」。
// 事件面（谁尝试连了哪、有没有越权尝试）没有数据源，所以在页尾如实列缺 —— 不画通道图充数。
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
  if (!e.config_present) {
    return section("执行面", "gateway 实际吃的那份 config.toml",
      el("p", { class: "empty" }, cannot(dash(e.note))));
  }
  const rows: Array<[string, Kid]> = [
    ["指纹", e.has_fingerprint
      ? el("span", { class: "mono" }, (e.version ?? "") + " · inputs=" + (e.inputs ?? ""))
      : cannot("没有 anc 指纹 —— 手写或他源配置")],
    ["生成于", e.has_fingerprint ? localTime(e.generated_at ?? "") : "—"],
    ["project", e.projects.length === 0
      ? cannot("一个都没扫到")
      : el("span", { class: "mono" }, e.projects.join(", "))],
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
  out.push(sedimentSection(d));

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
  return el("div", { class: "card prop" },
    el("h3", {}, title === "" ? "（提案没写标题）" : title, chip(dash(p.id), true)),
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
  return section("队列", "一共 " + v.count + " 条 —— 早提的排前面",
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
};
const bandDot = (b: string): string => BAND_DOT[b] ?? "dot";
const BAND_WORD: Record<string, string> = {
  green: "绿 · 已完成", yellow: "黄 · 运行中", red: "红 · 卡点 / 失败", unknown: "灰 · 认不出的词",
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

// 审计 —— 读者：管理者 / 关心「谁碰了什么」的人。排在「时间线」后面：
// 一个讲「人怎么说的」，一个讲「机器记下来的是什么」。
const auditPage: Page = {
  id: "audit", group: "记录", label: "审计", wired: true,
  question: "谁在什么时候、对谁、行使了什么（含被拒的）？", view: renderAudit,
};
// 时间线 —— 读者：跟进的人 / 老板。它排在「项目」后面：一个讲「有哪几件事」，一个讲「卡在哪一步」。
const timelinePage: Page = {
  id: "timeline", group: "记录", label: "时间线", wired: true,
  question: "卡在哪、谁在跟、下一步是谁的决定？", view: renderTimeline,
};
const overviewPage: Page = {
  id: "overview", group: "现在", label: "总览", wired: true,
  question: "现在有没有事？", view: renderOverview,
};
// 待批 —— 读者：该点头的人。它是全看板唯一能写的一页，所以排在最靠前：
// 整块看板只有它等着人动手，而「有没有卡在等我」正是「现在有没有事」的下一句。
const approvalsPage: Page = {
  id: "approvals", group: "现在", label: "待批", wired: true,
  question: "哪几件事在等人点头、该谁点？", view: renderApprovals,
};
const orgPage: Page = {
  id: "org", group: "组织", label: "组织", wired: true,
  question: "公司长什么样、边界在哪？", view: renderOrg,
};
const projectsPage: Page = {
  id: "projects", group: "组织", label: "项目", wired: true,
  question: "谁在做什么、到几号、卡住找谁？", view: renderProjects,
};
const runtimePage: Page = {
  id: "runtime", group: "运行", label: "运行态", wired: true,
  question: "每个 bot 活着吗、真的能回话吗？", view: renderRuntime,
};
const dataflowPage: Page = {
  id: "dataflow", group: "运行", label: "数据流", wired: true,
  question: "谁可以做什么、通道开着吗？", view: renderDataflow,
};
// 页名还叫「沉淀」（那是这一块最终要回答的），但这一页现在只答得了「原料有哪些」——
// 所以问题句按**现在真答得了**的写，不按以后的写。口径差在哪，页内写清了。
const assetsPage: Page = {
  id: "assets", group: "原料", label: "沉淀", wired: true,
  question: "手上有什么料、沉淀到哪一层了？", view: renderAssets,
};

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

  const band = (key: string, cls: string, label: string, cnt: string, note: string): HTMLElement =>
    el("div", { class: "strata " + cls, "data-band": key },
      el("div", { class: "lbl" }, label),
      el("div", { class: "cnt" }, cnt),
      el("div", { class: "note" }, note));

  const column = el("div", { class: "column" },
    band("open", "s-open", "料口", "", "会话与文件都从这落进来"),
    band("noise", "s-noise", "丢掉", no, "重复、噪声、没写完的半句"),
    band("hot", "s-hot", "热层 · 缓存", no, "还在发生的事，带保鲜期"),
    band("cold", "s-cold", "落库 · 经验", num(files) + " 个文件", "达标才落，落下的就是资产"),
    band("mask", "s-mask", "隐匿 · 指针", num(masked) + " 条", "明文不落库：落库前抹成指针"),
    el("div", { class: "stream" }));

  const legend = el("div", { class: "legendbox" },
    el("div", { class: "card" },
      sedRow("w-noise", "丢掉", no, "重复、噪声、没写完的半句 —— 当场蒸发，不进任何一层"),
      sedRow("w-hot", "热层 · 缓存", no, "还在发生的事。带保鲜期，过期自己碎掉 —— 所以它便宜"),
      sedRow("w-cold", "落库 · 经验", num(files),
        "达标了才落。落下的这一条就是资产，永久（本机 " + num(dirs) + " 个目录 · 立项书副本 " + num(charters) + "）"),
      sedRow("w-mask", "隐匿 · 指针", num(masked),
        "明文不落库：落盘前抹成「" + MASK_MARK + ":规则名」的指针，原文件改名归档")));

  return section("沉淀柱", "口径图，不是统计 · 一层比一层更贵、更少、更不该丢",
    el("div", { class: "sedi" }, column, legend),
    el("p", { class: "note" },
      "柱是「口径图」—— 它画的是四层的分工，不是统计；层右边的数是真数，对不上的两格如实写「未接入」。",
      "热层要 Redis 连上才有数（本机没起），所以那一层现在是空的 —— 这一条不编。"));
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
const PAGES: Page[] = [overviewPage, approvalsPage, orgPage, projectsPage, timelinePage, auditPage, runtimePage, dataflowPage, assetsPage];

function pageFor(id: string): Page {
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

function shell(): void {
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

window.addEventListener("hashchange", () => { shell(); });

void boot();
