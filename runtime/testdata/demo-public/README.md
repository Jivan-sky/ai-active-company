# 脱敏演示租户（demo-public）

这里的公司、成员、项目、业务 Agent、审计记录**全部是虚构的**，不对应任何真实公司、人员、客户、凭据或内部系统。`vault/` 里所有项目 `source` 都是 `demo://...` 指针。

用途只有两个：看板截图、端到端自测。截图见 `assets/delivery-2026-10-18/`。

## 目录

| 路径 | 说明 |
|---|---|
| `vault/` | 一份可直接喂给 `anc board serve` 的 org 真相源（5 业务域 / 8 项目 / 7 业务 Agent / 6 成员） |
| `hot-seed.ps1` | 往本机 Redis 写 3 条演示用的进行中任务（前缀 `ancdemo`），只影响热层 |

## 复现

前提：本机有 Redis（默认 `127.0.0.1:6379`）。热层是执行面的当前态，不写进 vault；没有 Redis 时热层为空，其余页面照常。

```powershell
cd runtime
go build -o ..\dist\anc.exe .

# 可选：预热热层，让「热层 / 成员抽屉」出现进行中的任务
powershell -ExecutionPolicy Bypass -File testdata\demo-public\hot-seed.ps1 -Anc ..\dist\anc.exe

..\dist\anc.exe board serve testdata\demo-public\vault `
  --addr 127.0.0.1:8797 `
  --data "$env:TEMP\anc-demo-data" `
  --hot-addr 127.0.0.1:6379 `
  --hot-prefix ancdemo `
  --ui-dir internal\board\ui
```

打开 `http://127.0.0.1:8797/#/overview`。

## 截图口径

- 视口 `1600 × 1240`，浅色主题，Chromium headless，`--force-device-scale-factor=1`。
- 页面：`#/overview`、`#/graph`、`#/domains`、`#/projects`、`#/members`、`#/members?who=fde-lead&open=fde-lead`、`#/system?tab=hot`、`#/audit`。
- 截图里的「在线」列一律是「未观测」：演示租户没有真实 harness 心跳，看板不拿审计里的「最近调用」冒充「此刻在线」。
