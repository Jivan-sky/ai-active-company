import type { Data, HotEntryView, HotView, Kid, SettleReply } from "../core/types";
import { postJSON } from "../core/api";
import { banner, card, chip, dash, el, section, table } from "../core/dom";
import { durText, humanBytes, localTime } from "../core/format";
import { reload } from "../core/refresh";
import { sedimentSection } from "./sediment";
import { handOff, statusCell } from "./timeline";
import { cannot, emptyNote, statCard } from "../ui/blocks";
import { byWho, setByWho } from "../ui/bywho";
import { closeDrawer, openDrawer } from "../ui/drawer";
import { bandDot, bandWord, dotLine } from "../ui/status";
import { showToast } from "../ui/toast";

export function renderAssets(d: Data): Kid[] {
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

// 归档 —— 热层里唯一能写的一刀：终态条目落 timeline，再清走热层。
// 与审批同一条纪律：谁在点由页面出，后端原样留痕、不校验。
// 还在 queued / running 的不开这个口：结账等于把没做完的事从看板抹掉（后端也这么判）。
export function archivable(status: string): boolean {
  const s = status.trim().toLowerCase();
  return s !== "queued" && s !== "running";
}

export function archiveSection(v: HotView, e: HotEntryView): HTMLElement {
  if (!archivable(e.status)) {
    return card("归档", "还在进行中，不落库", [
      ["为什么", cannot("queued / running 还没结束 —— 要终止先回执，用 status=cancelled 把决定留在记录里")],
    ]);
  }
  if ((v.settle_path ?? "") === "") {
    return card("归档", "服务没给写口地址", [
      ["为什么", cannot("settle_path 是空的 —— 刷新看看，或看服务端日志")],
    ]);
  }
  const who = el("input", {
    class: "by", type: "text",
    placeholder: "谁在归档（会原样写进 timeline）", value: byWho,
  });
  const note = el("input", { class: "why", type: "text", placeholder: "归档说明（可空）" });
  const hint = el("p", { class: "empty" }, "归档 = 先落 timeline、再清走热层；同一条不会重复写。");
  const btn = el("button", { class: "act approve", type: "button" }, "归档到时间线");
  who.addEventListener("input", () => {
    setByWho((who as HTMLInputElement).value);
  });
  btn.addEventListener("click", () => {
    void settleHot(v, e, who as HTMLInputElement, note as HTMLInputElement, btn as HTMLButtonElement, hint);
  });
  const box = card("归档", "先落 timeline、再清热层", []);
  box.append(
    el("p", { class: "empty" }, "这条已经是终态。归档后人能在「时间线」里按 case 找到它。"),
    el("div", { class: "field" }, who),
    el("div", { class: "field" }, note),
    el("div", { class: "acts" }, btn),
    hint);
  return box;
}

// 交接与验收是同一件事的三个动作：交给谁、谁验、验成什么样。
// 这里只摆当前值；历史在 timeline / audit，不在这张卡上堆。
export function handoffSection(e: HotEntryView): HTMLElement {
  const rows: Array<[string, Kid]> = [];
  if ((e.accept ?? "") !== "") rows.push(["验收口径", dash(e.accept)]);
  if ((e.hop ?? 0) > 0) rows.push(["交接次数", String(e.hop) + " / " + String(e.max_hop ?? "—")]);
  if (e.handoff) {
    rows.push(["最近交接", dash(e.handoff.from) + " → " + dash(e.handoff.to) + " · " + localTime(e.handoff.at)]);
    rows.push(["交接原因", dash(e.handoff.note)]);
  }
  if (e.review) {
    rows.push(["验收结论", dash(e.review.verdict) + " · " + dash(e.review.by) + " · " + localTime(e.review.at)]);
    rows.push(["验收意见", dash(e.review.note)]);
  }
  if ((e.refs ?? []).length > 0) {
    rows.push(["证据 / 引用", el("div", {}, ...(e.refs ?? []).map((r) => chip(r, true)))]);
  }
  if (rows.length === 0) {
    return card("交接与验收", "还没走", [["说明", cannot("这件还没 handoff / submit / verify。")]]);
  }
  const badge = e.review ? "验收 " + dash(e.review.verdict) : (e.handoff ? "已交接" : "");
  const box = card("交接与验收", badge, rows);
  if (e.handoff?.cross_domain) {
    box.append(banner("warn", "跨域交接", [
      el("span", {}, dash(e.handoff.from) + " → " + dash(e.handoff.to) + " 不在同一业务域。"),
      el("br"),
      el("span", {}, "v1 默认放行，但已留痕 —— 需要人看就看这一条。"),
    ]));
  }
  if (e.review?.self) {
    box.append(banner("warn", "自验", [
      el("span", {}, "验收人就是提交人 —— 已放行并标记，不代表独立复核。"),
    ]));
  }
  return box;
}

// 点一次归档：成功就整页重取（热层真的少了一条）。
export async function settleHot(
  v: HotView, e: HotEntryView,
  who: HTMLInputElement, note: HTMLInputElement, btn: HTMLButtonElement, hint: HTMLElement,
): Promise<void> {
  const by = who.value.trim();
  if (by === "") {
    hint.textContent = "先填「谁在归档」：匿名归档等于没归档（后端也这么拒）。";
    return;
  }
  btn.setAttribute("disabled", "disabled");
  hint.textContent = "正在落库…";
  const res = await postJSON<SettleReply & { error?: string; hint?: string }>(v.settle_path, {
    id: e.id, by, note: note.value,
  });
  if (res.status === 200 && res.body && res.body.ok) {
    closeDrawer();
    showToast((res.body.already ? "早已归档：" : "已归档：") + e.id);
    reload(true);
    return;
  }
  btn.removeAttribute("disabled");
  const msg = res.body?.error ?? res.err;
  const h = res.body?.hint ?? "";
  hint.textContent = (msg === "" ? "HTTP " + res.status : msg) + (h === "" ? "" : "（" + h + "）");
}

// 热层 —— 「还在发生的事」本身。与「待批」同一份底层数据、两个问题：
// 那是队列（等谁点头），这是这一层（几条、几条不新鲜、多久自己走）。
export function renderHot(d: Data): Kid[] {
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
  if (v.overdue > 0) {
    out.push(banner("bad", "有 " + v.overdue + " 件已超期", [
      el("span", {}, "超期只打标，不改阶段；要催人就跑 "),
      el("span", { class: "mono" }, "anc task notify"),
      el("span", {}, "（默认只打印，加 --send 才发）。"),
      el("br"),
      el("span", {}, "另有卡住 " + v.blocked + " 件 · 待验收 " + v.review + " 件。"),
    ]));
  } else if (v.actionable > 0) {
    out.push(banner("warn", "有 " + v.actionable + " 件要人动手", [
      el("span", {}, "卡住 " + v.blocked + " 件 · 待验收 " + v.review + " 件；没有已超期的。"),
    ]));
  }
  out.push(section("这一层现在什么情况", "阈值由服务端给，前端只翻成人话；归档那一刀只开在终态条目上",
    el("div", { class: "figs" },
      statCard(String(v.total), "条目", "热层总数"),
      statCard(String(v.stale), "不新鲜", "只是标记，不拦不删"),
      statCard(String(v.overdue), "已超期", "due 已过，仍未终态"),
      statCard(String(v.blocked), "卡住", "blocked 等人接手"),
      statCard(String(v.review), "待验收", "review + pending"),
      statCard(durText(v.ttl_secs), "保鲜期", "0 = 不失效"),
      statCard(durText(v.stale_after_secs), "不新鲜阈值", "多久没动算旧"))));
  const cards = v.entries.map((e) => {
    const age = e.age_secs == null ? "没写时间" : durText(e.age_secs) + " 前";
    const cardEl = el("div", { class: "card", tabindex: "0" },
      el("h3", {}, el("span", { class: e.stale ? "dot warn" : "dot ok" }), dash(e.title), chip(e.id, true),
        e.overdue ? chip("超期") : "",
        e.handoff?.cross_domain ? chip("跨域") : "",
        e.review ? chip(e.review.verdict === "pending" ? "待验收" : "已验收") : ""),
      el("dl", {},
        el("dt", {}, "状态"), el("dd", {}, dash(e.status)),
        el("dt", {}, "阶段"), el("dd", {}, dash(e.stage)),
        el("dt", {}, "截止"), el("dd", {}, (e.due_at ?? "") === "" ? "没定" : dash(e.due_at) + (e.overdue ? " · 已超期 " + durText(e.overdue_secs ?? 0) : "")),
        el("dt", {}, "优先级"), el("dd", {}, dash(e.priority)),
        el("dt", {}, "谁 → 交给谁"), el("dd", {}, dash(e.by) + " → " + dash(e.to)),
        el("dt", {}, "多久没动"), el("dd", {}, age),
        el("dt", {}, "新鲜度"), el("dd", {}, e.stale ? cannot("不新鲜（只是标记）") : "新鲜")));
    const open = () => openDrawer({
      eyebrow: "HOT / 热层", title: dash(e.title), sub: dash(e.id) + " · " + dash(e.status),
      body: [
        card("这条现在在哪", "热层是缓存，不是永久资产", [
          ["id", el("span", { class: "mono" }, e.id)],
          ["状态", dash(e.status)],
          ["阶段", dash(e.stage) + " · 围栏 " + String(e.fence ?? "—") + " · 尝试 " + String(e.attempt ?? "—") + " / " + String(e.max_attempt ?? "—")],
          ["派单 → 执行", dash(e.from) + " → " + dash(e.by)],
          ["下一手", dash(e.to)],
          ["截止 / 优先级", (e.due_at ?? "") === "" ? "没定 · " + dash(e.priority) : dash(e.due_at) + " · " + dash(e.priority)],
          ["超期", e.overdue ? "已超期 " + durText(e.overdue_secs ?? 0) : "没有超期"],
          ["会话 / 意图", dash(e.session) + " · " + dash(e.intent)],
          ["版本", "v" + String(e.version ?? "—")],
          ["租约", e.lease_no_expiry ? "不过期" : dash(e.lease_to)],
          ["多久没动", age],
          ["新鲜度", e.stale ? "不新鲜（只是标记，不拦不删）" : "新鲜"],
        ]),
        handoffSection(e),
        archiveSection(v, e),
      ],
    });
    cardEl.addEventListener("click", open);
    cardEl.addEventListener("keydown", (ev) => { if (ev.key === "Enter" || ev.key === " ") { ev.preventDefault(); open(); } });
    return cardEl;
  });
  out.push(section("住着哪些事", "最多回显 " + v.entries.length + " 条；总数照报，不因为截断装作只有这些",
    cards.length === 0 ? emptyNote("热层现在是空的 —— 没有还在发生的事。") : el("div", { class: "grid" }, ...cards)));
  out.push(section("这一页能做什么、不做什么", "说清边界比多画几张卡有用",
    emptyNote("能在**终态**条目上归档（先落 timeline、再清走热层）；不认领、不改状态。"
      + "还在跑的条目要终止，先让执行的人回执；过期只是标记，不拦不删。")));
  return out;
}

// 可沉淀候选 —— 「已经闭环的事」离「可复用的技能」还差哪一步。
//
// 这一块只摆事实：候选口径只有「最后一条是绿档」；signals 是推进条数 / 字数 / refs /
// 同名重复次数，不是价值分。写不写技能是人的判断，这一页一个字节都不写。
export function distillSection(d: Data): Kid[] {
  const v = d.distill;
  if (!v) {
    return [section("可沉淀候选", "读不到就不摆空表",
      emptyNote(d.distillErr === "" ? "服务没回 anc.distill/v1" : d.distillErr))];
  }
  const out: Kid[] = [];
  if (!v.wired) {
    out.push(banner("warn", "候选这一段答不了", [
      el("span", {}, dash(v.error)),
      el("br"),
      el("span", {}, "读不动不等于「没有候选」—— 修好 timeline 再看。"),
    ]));
    return out;
  }
  const cand = v.candidates ?? [];
  out.push(section("可沉淀候选", "候选 = 最后一条是绿档的 case；只按时间排，不打分",
    el("p", { class: "note" },
      "case " + v.cases + " 个 · 闭环 " + v.green + " 个 · 候选 " + v.candidate_total + " 条" +
      (v.truncated ? "（本页只列前 " + cand.length + " 条）" : "") + "。"),
    cand.length === 0
      ? emptyNote("没有候选 —— 不等于「没有可沉淀的资产」：先把事结账成终态，候选才会出现。")
      : table(["case", "这件事", "证据", "归属", "最后活动"], cand.map((c) => [
          el("span", { class: "mono" }, c.case),
          el("div", {}, el("b", {}, c.title), c.project ? el("p", {}, "项目 " + c.project) : ""),
          el("div", {}, ...c.signals.map((s) => chip(s))),
          el("div", {}, c.domain ? chip("域 " + c.domain) : "", c.by ? el("p", {}, "执行 " + c.by) : ""),
          localTime(c.at),
        ]))));

  if (!v.skills_wired) {
    out.push(banner("warn", "技能覆盖答不了", [el("span", {}, dash(v.skills_error))]));
  } else {
    const skills = v.skills ?? [];
    out.push(section("技能覆盖", "声明在 roles/<岗位> 的 skills:；正文在 skills/<名>/SKILL.md",
      skills.length === 0
        ? emptyNote("没有任何岗位声明技能 —— 这是「没登记」，不是「没有技能」。")
        : table(["技能", "声明岗位", "正文"], skills.map((s) => [
            el("span", { class: "mono" }, s.name),
            el("div", {}, ...s.roles.map((r) => chip(r))),
            s.missing
              ? el("span", { class: "cannot" }, "缺正文 · " + s.where)
              : el("span", {}, "在 · " + s.where),
          ])),
      (v.orphans ?? []).length > 0
        ? banner("warn", "有正文、没有岗位声明（agent 取不到）", [
            el("div", {}, ...(v.orphans ?? []).map((o) => chip(o, true))),
          ])
        : ""));
  }

  const gaps = v.gaps ?? [];
  if (gaps.length > 0) {
    out.push(section("缺口", "报缺，不编 —— 这一页答不了什么，写清楚", el("ul", {}, ...gaps.map((g) => el("li", {}, g)))));
  }
  out.push(section("这一块只读", "候选不是技能，写入是另一刀", emptyNote(v.note)));
  return out;
}

// 沉淀 —— 只画「料 → 丢掉 → 热层 → 落库 → 隐匿」这条口径，不把原料数量说成资产。
export function renderSediment(d: Data): Kid[] {
  return [
    sedimentSection(d),
    ...distillSection(d),
    section("这一页只答什么", "沉淀是口径图，不是统计报表",
      emptyNote("原料在「原料」页；热层明细在「热层」页。这一页只画四层分工与每层能对上的真数。")),
  ];
}

// 卡点 —— 从时间线派生：只筛红 / 黄 / 等人给一句话的 case，不另存一份事实。
export function renderCases(d: Data): Kid[] {
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
