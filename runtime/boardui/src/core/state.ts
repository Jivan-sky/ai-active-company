import type { Data } from "./types";

// 全局只读缓存：取数成功一次后，换页只重画、不重取。
// 写入口只有 setCache，避免各页各自持有一份「最新数据」。
export let cache: Data | null = null;

export function setCache(next: Data | null): void {
  cache = next;
}
