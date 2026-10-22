import type { ContextView, GrantsView, SearchView } from "./core/types";
import { contract, fetchData, getJSON } from "./core/api";
import { ctxQuery, currentId, grantsQuery, hashQuery, searchQuery } from "./core/query";
import { setReloader, setRerender } from "./core/refresh";
import { cache, setCache } from "./core/state";
import { activeSystemTab } from "./pages/system";
import { pageFor } from "./router";
import { shell } from "./ui/shell";

// ANC 看板前端（浏览器侧）入口。
//
// 目录分工：
//   core/   协议类型、取数、URL 参数、主题基础件
//   ui/     通用块、状态口径、外壳、抽屉、命令面板、两张 Canvas
//   org/    组织派生关系与详情抽屉
//   pages/  一页一个文件，只回答页头写下的那一个问题
//   router.ts  页面注册表（顺序就是侧栏顺序）
//
// 五条纪律仍在（真相源文字只走 textContent / 不猜不补 / 几乎只读 / 一页一问 / 未接入如实标）。
// 构建：`npm run build`（esbuild 打成 app.js，随 anc 二进制一起嵌进去）。


async function boot(force = false): Promise<void> {
  if (force) setCache(null);
  if (cache === null) {
    shell();
    setCache(await fetchData());
  }
  shell();
}

// 授权表那一页的取数：换视角时只重取这一份（其余几份不用动）。
async function loadGrants(): Promise<void> {
  const res = await getJSON<GrantsView>("/api/grants" + grantsQuery());
  const gr = contract(res, "授权表");
  if (cache) setCache({ ...cache, grants: gr.v, grantsErr: gr.err });
  shell();
}

// 上下文那一页的取数：换人 / 换范围时就重取那一份（其余几份不用动）。
// 只重取它自己，不是整页重取 —— 这一页的选择是**参数**，不是刷新。
async function loadContext(): Promise<void> {
  const res = await getJSON<ContextView>("/api/context" + ctxQuery());
  const cx = contract(res, "工作上下文");
  if (cache) setCache({ ...cache, context: cx.v, contextErr: cx.err });
  shell();
}

// 检索页的取数：查询词 / 视角 / 域都在 URL 里。换一次条件只重取这一份；
// 索引仍然是后端每次从 markdown 重建 —— 看板不另存一份会过期的答案。
async function loadSearch(): Promise<void> {
  if (searchQuery() === "") {
    if (cache) setCache({ ...cache, search: null, searchErr: "" });
    shell();
    return;
  }
  const res = await getJSON<SearchView>("/api/search" + searchQuery());
  const sr = contract(res, "知识检索");
  if (cache) setCache({ ...cache, search: sr.v, searchErr: sr.err });
  shell();
}

setRerender(shell);
setReloader((force) => void boot(force));

window.addEventListener("hashchange", () => {
  // 这一页的「选中谁」在 hash 里：进了这一页就按 URL 重取一次，别的页照旧（取数只在启动 / 刷新）。
  if (pageFor(currentId()).id === "context") {
    void loadContext();
    return;
  }
  if (pageFor(currentId()).id === "search") {
    void loadSearch();
    return;
  }
  // 授权表并进了「系统 · 网关与授权」：换视角只重取它自己（那是**参数**，不是刷新）。
  if (pageFor(currentId()).id === "system" && activeSystemTab().id === "gateway" && (hashQuery().get("who") ?? "") !== "") {
    void loadGrants();
    return;
  }
  shell();
});

void boot();
