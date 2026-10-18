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
export function currentId(): string {
  return location.hash.replace(/^#\/?/, "").split("?")[0]!.trim();
}

// hash 里的参数（`#/timeline?p=2`）—— 翻页状态进 URL：刷新、前进后退、把链接发给别人，
// 都还停在那一页。不引入路由器，也不引入第二份状态。
export function hashQuery(): URLSearchParams {
  const i = location.hash.indexOf("?");
  return new URLSearchParams(i < 0 ? "" : location.hash.slice(i + 1));
}
