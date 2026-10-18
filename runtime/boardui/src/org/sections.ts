import type { BoardView, Kid, Member } from "../core/types";
import { card, dash, el, section, table } from "../core/dom";
import { emptyNote, sourceCell } from "../ui/blocks";

export function domainSection(v: BoardView): Kid {
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

export function projectSection(v: BoardView): Kid {
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

export function roleSection(v: BoardView): Kid {
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

export function routingSection(v: BoardView): Kid {
  if (v.routing.length === 0) {
    return section("数据路由", "", emptyNote("还没有顶层数据目录。"));
  }
  const rows = v.routing.map((r): Kid[] => [el("span", { class: "mono" }, dash(r.dir)), dash(r.summary)]);
  return section("数据路由", "顶层数据目录 → 说明（persona 段 3 用的就是这张表）",
    table(["目录", "说明"], rows));
}

