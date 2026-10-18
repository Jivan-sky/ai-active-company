import type { Data, DataflowView, Kid } from "../core/types";
import { banner, card, dash, el, section, table } from "../core/dom";
import { localTime } from "../core/format";
import { cannot, emptyNote } from "../ui/blocks";
import { dotLine } from "../ui/status";

export function renderDataflow(d: Data): Kid[] {
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
export function execSection(v: DataflowView): Kid {
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
