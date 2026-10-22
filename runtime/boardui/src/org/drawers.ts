import { dash, el } from "../core/dom";
import { sinceText } from "../core/format";
import type { AgentRow, BoardView, Data, DomainRow, Member, ProjectRow } from "../core/types";
import { agentWork, agentsForDomain, agentsForMember, botProject, botStateOf, domainLabel, domainOf, flowBotOf, grantsFor, hotProjectOf, lastCall, liveAgentOf, memberAgentStats, memberProjects, memberTasks, periodState, roleTitle } from "./view";
import { cannot, dcard, kv, sourceCell } from "../ui/blocks";
import { openDrawer } from "../ui/drawer";
import { bandDot } from "../ui/status";

export function openMemberDrawer(d: Data, v: BoardView, m: Member): void {
  const links = agentsForMember(v, m);
  const tasks = memberTasks(d, m);
  const stats = memberAgentStats(d, v, m);
  const projects = memberProjects(v, m);
  const refs = [m.name, ...links.map((x) => x.agent.slug)];
  const grants = grantsFor(d, refs);
  const bot = botStateOf(d, botProject(v, m.name));
  const doms = m.domains.map((x) => domainOf(v, x)).filter((x): x is DomainRow => x !== undefined);
  const reportsTo = (m.reports_to ?? "").trim();
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
              ["上级", reportsTo === ""
                ? cannot("未提供 —— persona 没写 reports_to")
                : el("a", { href: "#/members?open=" + encodeURIComponent(reportsTo) },
                    dash(m.reports_to_label || reportsTo))],
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
export function openProjectDrawer(d: Data, v: BoardView, p: ProjectRow): void {
  const pct = periodState(p.period);
  const agents = agentsForDomain(v, p.domain);
  const hot = hotProjectOf(d, p.slug);
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
          dcard("在办任务", hot === null ? "热层未接" : String(hot.total) + " 件",
            hot === null
              ? el("p", { class: "empty" }, "热层没接上 —— 「读不到」不等于「没有在办的事」。")
              : hot.total === 0
                ? el("p", { class: "empty" }, "热层里没有挂在这个项目名下的事。")
                : kv([
                    ["在办", String(hot.total)],
                    ["卡住", String(hot.blocked)],
                    ["待验收", String(hot.review)],
                    ["超期", String(hot.overdue)],
                  ]),
            hot === null ? "" : el("p", { class: "dnote" },
              "按热层 task.project 汇总；已归档的事已经离开热层，所以这不是完成率，更不是交付进度。")),
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

export function openAgentDrawer(d: Data, v: BoardView, a: AgentRow): void {
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
