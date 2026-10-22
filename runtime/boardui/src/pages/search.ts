import type { Data, Kid, SearchHit, SearchView } from "../core/types";
import { banner, dash, el, section } from "../core/dom";
import { localTime } from "../core/format";
import { hashQuery } from "../core/query";
import { reload } from "../core/refresh";
import { emptyNote, statCard } from "../ui/blocks";
import { showToast } from "../ui/toast";

// ---------- 知识检索：按当前可见范围，哪几篇相关 ----------
//
// 这一页只做一件事：把问题交给 `/api/search`，把命中的指针 + 片段摆出来。
// 它不打开正文、不猜测答案，也不把「没命中」说成「库里没有」—— 索引面与跳过项
// 由后端如实回在 warnings / note 里。

function searchValue(values: URLSearchParams, key: string): string {
  return (values.get(key) ?? "").trim();
}

function formSection(d: Data, q: string, who: string, domain: string): Kid {
  const qInput = el("input", {
    class: "search-q", type: "search", name: "q", value: q,
    placeholder: "查什么？例如：结算、订舱、客户、某个卡点…",
    autocomplete: "off", spellcheck: "false",
  });
  const whoSel = el("select", { class: "search-who", name: "who" });
  whoSel.append(el("option", { value: "" }, "本机操作者 · 全部域（观察视角）"));
  for (const id of d.context?.roster ?? []) {
    const label = id.kind === "agent" ? "agent · " + dash(id.ref) : dash(id.name) + " · " + id.ref;
    const opt = el("option", { value: id.ref }, label);
    if (id.ref === who) opt.setAttribute("selected", "selected");
    whoSel.append(opt);
  }
  const domainSel = el("select", { class: "search-domain", name: "domain" });
  domainSel.append(el("option", { value: "" }, "全部可见域"));
  for (const row of d.board?.domains ?? []) {
    const opt = el("option", { value: row.slug }, row.name + " · " + row.slug);
    if (row.slug === domain) opt.setAttribute("selected", "selected");
    domainSel.append(opt);
  }
  const form = el("form", { class: "search-bar", role: "search" },
    el("label", { class: "search-field wide" }, el("span", {}, "问题"), qInput),
    el("label", { class: "search-field" }, el("span", {}, "以谁的身份看"), whoSel),
    el("label", { class: "search-field" }, el("span", {}, "业务域"), domainSel),
    el("button", { class: "act approve search-go", type: "submit" }, "检索"));
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    const query = (qInput as HTMLInputElement).value.trim();
    if (query === "") {
      showToast("先写要查什么");
      (qInput as HTMLInputElement).focus();
      return;
    }
    const p = new URLSearchParams({ q: query });
    const w = (whoSel as HTMLSelectElement).value;
    const g = (domainSel as HTMLSelectElement).value;
    if (w !== "") p.set("who", w);
    if (g !== "") p.set("domain", g);
    const href = "#/search?" + p.toString();
    // 同一条查询再点一次也要真的重算：索引是后端每次从 markdown 重建的。
    if (location.hash === href) reload(true);
    else location.hash = href;
  });
  return section("问知识库", "指针 + 片段；查不到就说没入库。视角是观察过滤器，不是登录凭证。", form);
}

function hitCard(h: SearchHit): HTMLElement {
  const title = (h.title ?? "").trim() === "" ? h.path : h.title;
  return el("article", { class: "search-hit" },
    el("div", { class: "sr-top" },
      el("b", {}, dash(title)),
      el("span", { class: "chip mono" }, h.domain ? h.domain : "公司级")),
    el("div", { class: "search-path mono" }, h.path),
    el("p", { class: "search-snippet" }, dash(h.snippet)),
    el("div", { class: "search-meta" },
      el("span", {}, "score " + h.score.toFixed(4)),
      h.updated ? el("span", {}, localTime(h.updated)) : ""));
}

function resultSection(v: SearchView): Kid {
  const out: Kid[] = [];
  out.push(section("结果", "先按可见域过滤，再打分、后截断 —— 看不见的文档不参与分数，也不计入命中",
    el("div", { class: "figs" },
      statCard(String(v.indexed), "索引", "从 markdown 重建"),
      statCard(String(v.visible), "可见", "当前视角"),
      statCard(String(v.count), "命中", "这次返回"),
      statCard(v.backend ?? "—", "后端", "不静默回落"))));

  if ((v.error ?? "") !== "") {
    out.push(banner("bad", "检索没答完", [el("span", {}, dash(v.error))]));
  }
  if (v.refused) {
    out.push(banner("warn", "这一问被拒", [
      el("span", {}, dash(v.refused.reason)),
      v.refused.ask ? el("span", {}, "找谁：" + v.refused.ask) : "",
    ]));
  }
  if ((v.warnings ?? []).length > 0) {
    out.push(banner("warn", "索引跳过了一些东西（不是零命中）", (v.warnings ?? []).map((w) => el("span", {}, w))));
  }
  if (v.hits.length === 0) {
    out.push(section("没有命中", "空结果不是「库里没有」—— 可能只是还没入库", emptyNote(v.note)));
  } else {
    out.push(section("命中", "每一条都指回 vault 相对路径；正文要靠 anc_get / CLI --get 取",
      el("div", { class: "search-results" }, ...v.hits.map(hitCard))));
  }
  out.push(section("这一页的边界", "少一点聪明，多一点可核",
    el("p", { class: "empty" }, dash(v.note),
      el("br"),
      "索引不是真相源；删掉索引不影响答案。权限只认 org：成员看自己的域，管理员 / 跨域可见口径按 company.md，业务 agent 只看 agents.md 的 domain。")));
  return el("div", {}, ...out);
}

export function renderSearch(d: Data): Kid[] {
  const values = hashQuery();
  const q = searchValue(values, "q");
  const who = searchValue(values, "who");
  const domain = searchValue(values, "domain");
  const out: Kid[] = [formSection(d, q, who, domain)];
  if (q === "") {
    out.push(section("还差一个问题", "这一页不自动替你猜要查什么", emptyNote("在上面写词，或者从「原料」「沉淀」「上下文」里带着问题过来。")));
    return out;
  }
  if (d.searchErr !== "") {
    out.push(banner("bad", "检索服务没回话", [el("span", {}, d.searchErr)]));
    return out;
  }
  if (!d.search) {
    out.push(section("正在检索", "服务现算，不缓存旧答案", emptyNote("稍等……如果一直停在这里，点右上角刷新重取。")));
    return out;
  }
  out.push(resultSection(d.search));
  return out;
}
