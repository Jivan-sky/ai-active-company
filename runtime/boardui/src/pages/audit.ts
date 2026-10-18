import type { AuditEntryView, Data, Kid } from "../core/types";
import { PER_PAGE, pager, sliced } from "../ui/pager";
import { banner, chip, dash, el, section, table } from "../core/dom";
import { localTime } from "../core/format";
import { hashQuery } from "../core/query";
import { cannot, emptyNote, statCard } from "../ui/blocks";
import { dotLine } from "../ui/status";

// 四档配色：ok 绿实心圆 / denied 菱形（被拦下）/ failed 菱形（真失败）/ 判不出的虚线圈。
// denied 与 failed 是两个词、两件事 —— 「权限没给」不能说成「故障」（后端同一条纪律）。
export const AUDIT_DOT: Record<string, string> = {
  ok: "dot ok", denied: "dot bad", failed: "dot bad", unknown: "dot gray",
};
export const AUDIT_WORD: Record<string, string> = {
  ok: "绿 · 成功", denied: "红 · 被拦下", failed: "红 · 失败", unknown: "灰 · 判不出",
};
export const auditDot = (b: string): string => AUDIT_DOT[b] ?? "dot gray";
export const auditWord = (b: string): string => AUDIT_WORD[b] ?? "灰 · 判不出";

// 「对谁」那一栏。四类各自是什么见后端 Zone 常量 ——
// 关键是**「判不出」不许显示成「不跨域」**：把「没判」画成「没事」是最坏的一种假绿。
export function zoneCell(e: AuditEntryView): Kid {
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

export function renderAudit(d: Data): Kid[] {
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
