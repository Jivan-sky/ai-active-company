import type { Data, Kid } from "../core/types";
import { banner, dash, el, section, table } from "../core/dom";
import { cannot, emptyNote } from "../ui/blocks";
import { dotLine, runState } from "../ui/status";

// 运行态 —— 读者：运维 / FDE。数据源是防假绿探针（与 `anc probe` **同一份判据**，只读磁盘）。
//
// 它的纪律：判活靠**真拨 socket**、判定靠**真回过话**，都不看「进程在不在」——
// 进程活着 / engine started 打着绿字 / 却一条也回不了，这个坑真踩过两次。
// 「现在这一秒能不能回话」的裁判权在**人**：这一页只观测、只报红，不做任何自动处置。
export function renderRuntime(d: Data): Kid[] {
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
