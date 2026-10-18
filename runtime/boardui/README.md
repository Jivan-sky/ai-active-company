# ANC 看板前端

浏览器侧只做一件事：把 `/api/board`（`anc.board/v1` 投影）、`/api/issues` 与运行态投影摆成人能看的界面。
运行时不需要 Node；`app.js` / `app.css` 是构建产物，随 Go 二进制一起 `go:embed`。

## 目录

```
src/
  app.ts            入口：启动、刷新、hash 变更后的局部重取
  router.ts         页面注册表（顺序就是侧栏顺序）
  core/             协议类型、取数、URL 参数、DOM 最小件、格式化
  ui/               外壳、通用块、状态口径、抽屉、命令面板、两张 Canvas
  org/              组织派生关系、组织页区块、成员/项目/Agent 抽屉
  pages/            一页一个文件；只回答页头写下的那一个问题
  styles/           样式按层拆分，index.css 的顺序就是层叠顺序
```

## 加东西

- 新页面：在 `pages/` 加一个文件，导出 `renderXxx(d: Data): Kid[]`，再到 `router.ts` 注册 `Page`。页头 `question` 写不出来就先别开页。
- 新样式：放进对应的 `styles/*.css`；只有确实需要新的层叠层时才改 `index.css` 的顺序。浅色主题必须最后。
- 新接口字段：先加 `core/types.ts`，再在 `core/api.ts` 取数；页面不直接 `fetch`。
- 页面要触发重画：用 `core/refresh.ts` 的 `rerender()`；要重取数据用 `reload(true)`。页面不直接 import `ui/shell`（会绕成循环）。

## 纪律

1. 真相源里的字只走 `textContent`，绝不 `innerHTML`。
2. 没有数据源就写「未接入 / 读不到」，不编数据填空。
3. 看板几乎只读：写只有「待批点头」与「热层终态归档」两处，且都留痕。
4. 动效分层：换页才进场，只有「在动 / 要人管」的状态灯呼吸，数据区保持静止。

## 构建与验证

```bash
npm run check   # tsc --noEmit + esbuild（app.js + app.css）
```

产物落在 `../internal/board/ui/`，不要手改。提交前跑一次 `npm run check`；只改 TS/CSS 而不重建，二进制里仍是旧界面。
