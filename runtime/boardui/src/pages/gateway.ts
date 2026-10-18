import type { Data, Kid } from "../core/types";
import { banner, dash, el, section, table } from "../core/dom";
import { localTime } from "../core/format";
import { cannot, emptyNote } from "../ui/blocks";

// 页面注册表。顺序就是侧栏顺序；分组是「问题」，页名是「答案」。
// 网关 —— 从「数据流」里拆出来的执行面。那一页回答「每个 bot 手里有什么」，
// 这一页只回答「gateway 真吃的是哪份配置、relay 的执行口径是什么」。
// 两个问题、两个变化节奏：换 bot 不改 config，改 config 不改 bot 名单。
export function renderGateway(d: Data): Kid[] {
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
