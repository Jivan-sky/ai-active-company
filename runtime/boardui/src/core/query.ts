export function ctxQuery(): string {
  const q = hashQuery();
  const who = (q.get("who") ?? "").trim();
  const domain = (q.get("domain") ?? "").trim();
  const p = new URLSearchParams();
  if (who !== "") p.set("who", who);
  if (domain !== "") p.set("domain", domain);
  const s = p.toString();
  return s === "" ? "" : "?" + s;
}
// 授权表那一页的视角（`#/grants?who=bizbot`）—— 同一条理由：位置进 URL，
// 刷新 / 前进后退 / 把链接发给别人都还停在同一个主体上；不带就是全量。
export function grantsQuery(): string {
  const who = (hashQuery().get("who") ?? "").trim();
  if (who === "") return "";
  const p = new URLSearchParams();
  p.set("who", who);
  return "?" + p.toString();
}

// 知识检索的取数参数（`#/search?q=结算&who=bob&domain=trade`）。
// 与上下文页同一条纪律：问题 / 视角进 URL，刷新、前进后退、把链接发给别人都还停在同一次检索。
export function searchQuery(): string {
  const q = hashQuery();
  // 没有查询词就不发请求：视角 / 域只有在有 q 时才有意义，否则只会换来一条
  // 「q 为空」的假错误，还会白白建一次全库索引。
  if ((q.get("q") ?? "").trim() === "") return "";
  const p = new URLSearchParams();
  for (const key of ["q", "who", "domain"]) {
    const v = (q.get(key) ?? "").trim();
    if (v !== "") p.set(key, v);
  }
  const s = p.toString();
  return s === "" ? "" : "?" + s;
}
export function currentId(): string {
  return location.hash.replace(/^#\/?/, "").split("?")[0]!.trim();
}

// hash 里的参数（`#/timeline?p=2`）—— 翻页状态进 URL：刷新、前进后退、把链接发给别人，
// 都还停在那一页。不引入路由器，也不引入第二份状态。
export function hashQuery(): URLSearchParams {
  const i = location.hash.indexOf("?");
  return new URLSearchParams(i < 0 ? "" : location.hash.slice(i + 1));
}
