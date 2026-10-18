import type { ApprovalPending, ApprovalSignal, ApprovalView, Data, DecideReply, Kid } from "../core/types";
import { postJSON } from "../core/api";
import { banner, chip, dash, el, section } from "../core/dom";
import { localTime, sinceText } from "../core/format";
import { reload, rerender } from "../core/refresh";
import { cannot, emptyNote } from "../ui/blocks";
import { byWho, setByWho } from "../ui/bywho";
import { bandDot, bandWord, dotLine } from "../ui/status";

// 刚点完头的回执。只放内存：它讲的是「刚才那一下」，不是存档（存档在 timeline / audit）。
export let lastReceipt: DecideReply | null = null;
export let lastDecideErr = "";

export function receiptBanner(r: DecideReply): HTMLElement {
  return el("div", { class: "banner ok" },
    el("h3", {}, "刚刚点头：" + dash(r.verdict) + "（" + (r.by === "" ? "没记谁" : r.by) + "）"),
    el("ul", {},
      el("li", {}, el("span", { class: "mono" }, dash(r.proposal)), " —— ", dash(r.note)),
      el("li", {}, "落库 ", el("span", { class: "mono" }, dash(r.timeline))),
      el("li", {}, "审计 ", el("span", { class: "mono" }, dash(r.audit)))));
}

// 「谁在点」—— 看板没有免登，所以这一栏由页面出，并记在本机（下次不用再打）。
export function bySection(): Kid {
  const input = el("input", {
    class: "by", type: "text",
    placeholder: "你的名字 / 岗位（会原样写进留痕）",
    value: byWho,
  });
  input.addEventListener("input", () => {
    setByWho((input as HTMLInputElement).value);
  });
  return section("谁在点", "看板没有免登：这一栏由填的人负责；ANC 只把它原样留痕，不校",
    el("p", { class: "empty" },
      "飞书卡片那条路不需要它 —— 卡片点击事件自带 open_id，点的人是谁由事件给。"),
    el("div", { class: "field" }, input));
}

// 一条提案一张卡：人话在上，凭证（id / 时间 / 客体）在下，按钮在最底下。
export function pendingCard(v: ApprovalView, p: ApprovalPending): HTMLElement {
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
export function actsRow(v: ApprovalView, p: ApprovalPending): HTMLElement {
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
export async function decide(
  v: ApprovalView, p: ApprovalPending, sig: ApprovalSignal,
  why: HTMLInputElement, row: HTMLElement,
): Promise<void> {
  lastDecideErr = "";
  const who = byWho.trim();
  if (who === "") {
    // 后端也这么拒（「匿名点头等于没点头」）—— 这里先说，省一次往返。
    lastDecideErr = "先在上面填「谁在点」：匿名点头等于没点头（后端也这么拒）。";
    rerender();
    return;
  }
  if (v.write_path === "") {
    lastDecideErr = "服务没给写口地址（write_path 是空的）—— 刷新看看，或看服务端日志。";
    rerender();
    return;
  }
  // 点下去就先按住按钮：连着点两下 = 第二次必然「不在队列里」（那条已经结过账了）。
  for (const b of Array.from(row.querySelectorAll("button"))) b.setAttribute("disabled", "disabled");
  const res = await postJSON<DecideReply & { error?: string; hint?: string }>(v.write_path, {
    id: p.id, by: who, signal: sig.value, why: why.value,
  });
  if (res.status === 200 && res.body && res.body.ok) {
    lastReceipt = res.body;
    reload(true);
    return;
  }
  const msg = res.body?.error ?? res.err;
  const hint = res.body?.hint ?? "";
  lastDecideErr = (msg === "" ? "HTTP " + res.status : msg) + (hint === "" ? "" : "（" + hint + "）");
  rerender();
}

export function pendingSection(v: ApprovalView): Kid {
  if (v.pending.length === 0) {
    return section("队列", "进队时间从早到晚 —— 早提的先办",
      emptyNote("队列是空的：现在没有等点头的提案。"));
  }
  return section("队列", "一共 " + v.count + " 条 —— 早提的排前面；每条前面那个靛蓝暂停符号是判真第四档："
    + "有人在等你给一句话（没坏，也不是系统自己在走）",
    el("div", { class: "grid props" }, ...v.pending.map((p) => pendingCard(v, p))));
}

export function signalsSection(v: ApprovalView): Kid {
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

export function renderApprovals(d: Data): Kid[] {
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
