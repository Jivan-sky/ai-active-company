import type { Data, GrantRow, GrantsView, Kid } from "../core/types";
import { banner, dash, el, section, table } from "../core/dom";
import { cannot, emptyNote, findingLine } from "../ui/blocks";

// ---------- 授权表页：谁被允许对谁做什么 ----------
//
// 读者：管理者 / 想知道「谁被允许做什么」的人。这一页只回答**一个问题**（授权表本身）：
// 「通道开没开」在「数据流」页（那里的数据源是上游 config，跟这里不是一份）。
//
// 视角（`#/grants?who=<编号身份>`）是这一页的**位置**，不是状态：进 URL，刷新 / 前进后退 /
// 把链接发给别人，都还停在同一个主体上；不带 who 就是全量。换视角只重取这一份（见 loadGrants）。

export const grantKindLabel = (kind: string): string =>
  kind === "agent" ? "业务 agent" : kind === "member" ? "成员"
    : kind === "role" ? "岗位" : kind === "domain" ? "业务域" : "认不出";

// 面包屑：这一页在哪、正以谁的视角看。视角**有效还不够，得看得见** —— 否则读的人不知道
// 手里这一份是谁的那一份（服务端认不出时也照实写「认不出」）。
export function grantsCrumb(v: GrantsView): Kid {
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
export function grantSubjectCell(r: GrantRow, slot: string): Kid {
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
export function grantPicker(v: GrantsView): Kid {
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

export function grantEmpty(v: GrantsView): Kid {
  if (v.scope.ref !== "" && v.scope.known) {
    return emptyNote("这个视角下一条都没有：表里没有一条与它有关。" +
      " —— 这**不等于**「它什么都做不了」：这一页只看真相源里写没写权，执行层拦不拦是另一页的事。");
  }
  return emptyNote("授权表是空的（`grants/` 里还没有条目）。这不算错：" +
    "没有 grant 就是不通（SPEC §6 不变量 1）—— 表空只说明还没有给过任何一条权。");
}

export function renderGrants(d: Data): Kid[] {
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
