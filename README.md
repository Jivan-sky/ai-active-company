# ai-native-company

![ANC 封面：一份真相源（SPEC.md），三个消费面（人读 / Agent 读 / 机器跑）](./assets/cover.jpg)

> **本仓库是 ANC 的载体。** 一份真相源，三个消费面。

## ANC 是什么

**ANC 把组织协调能力从人的职位上解绑、挂到系统上。** 0 中层是结果，提效是副产品。

判断它成不成，硬指标只有一句：**新人或 Agent 能不能不靠问人，拿到全部上下文、权限、工具，把活做完，并且有人能验证。**

完整口径见 `SPEC.md`（唯一真相源）。

## 结构

```
ai-native-company/
├─ SPEC.md          真相源：组织模型 · 四层两纵切 · 七连接件 · 三档可逆 · 判据
├─ assets/          封面：cover.svg 为源（引用 background.jpg），cover.jpg 为渲染成品
├─ docs/            给实现读：上游四份实现视图的副本（M1 设计 / 里程碑 / 轻形态 MVP / 调研）
├─ skill/           给 Agent 读：可单独安装的能力件（资产沉淀的预存件），FDE skill 路线 B 指向这里
└─ runtime/         给机器跑：ANC 驻场装配器（anc-onsite 已并入）
   ├─ DESIGN.md     运行层落地设计（v0.1 草案；D4 / D5 / D6 已拍板，D1 / D2 与 D3 的其余部分未拍板）
   └─ testdata/     测试样本一处收着（org vault 样本 · 卡片事件体 · golden 快照）—— 不进正式文件的目录
```

**真相源只有一份。** `skill/`、`runtime/` 都是 `SPEC.md` 的派生视图——改口径改 SPEC，不在这几处各改一遍。
**测试都跟包走，样本一处收着。** Go 的 `*_test.go` 必须与它测的包同目录（语言约束，不是随手放），
所以**测试代码挨着代码住**；而**一切测试样本**（vault 样本 / 事件体 / golden 快照）收到 `runtime/testdata/` 一处，
不进正式文件的目录。
**开放问题不在仓库里。** 全部未定事项记在 [GitHub Issues](https://github.com/Jivan-sky/ai-native-company/issues)，
每条带「要定什么 / 为什么没定 / 选项与建议 / 验收 / 卡住谁」。仓库内**不再**维护问题台账（曾有一份 `ISSUES.md`，2026-10-07 迁走并删除）。

## 与 FDE 48h sprint 的关系

**FDE 是前置条件，ANC 是核心。**

- `fde-48h-sprint`（`github.com/Jivan-sky/fde-48h-sprint`）负责接需求、判真伪、打穿一个点、换授权和现场。它的**路线 B 指向本仓库**。
- 本仓库负责打穿之后：把那一刀接回树干，长期跑下去。
- 两者是同一根链条的两段，不是两个竞品。

## 现状

| 块 | 状态 |
|---|---|
| `SPEC.md` | **v0.3 合并稿（2026-10-06）**· 骨架（工程实现层）来自上游 `HA7CH/ai-native-company` SPEC v0.1（MIT），组织模型与方法论层来自本仓库 v0.2 |
| 开放问题 | 全部在 [GitHub Issues](https://github.com/Jivan-sky/ai-native-company/issues)；仓库内不再有台账文件 |
| `skill/` | **已建**：三个能力件 `anc-distill` / `anc-distill-targets` / `anc-evolve` + `skill/README.md` 索引（地址 / 能力 / 使用方法）。这是**资产沉淀功能的预存件** —— 能力方法已落成可安装形态，**功能本身尚未实现** |
| `runtime/` | 已并入 `anc-onsite`。**阶段 A / B / C 都落了**：A = `anc init` / `org init` / `org check` / `render` / `doctor` / `assets`；B = `anc service`（**已退役**，2026-10-07 —— 它生成的单元跑的是根本不存在的 `anc serve`；见 `runtime/DESIGN.md` §1 与 §7.1.9）；C = `anc apply` / `probe` / `notify` / `trail` / `timeline` / `audit` / `gate` / `envelope` / `org export` / `board serve`。本机实测（2026-10-10）：`gofmt` / `go vet` / `go test -count=1 ./...` **十五包全绿**、六个目标（win / darwin / linux × amd64 / arm64）交叉编译 **6/6** 通过。**端到端已打通**：真飞书 app 走通「飞书发消息 → gateway → claude → 飞书回话」一轮，长连接形态免公网回调（见 `runtime/DESIGN.md` §7.1.3）；**装机腿** Windows（2026-10-07）与 Linux（2026-10-08）沙箱各真跑一次（§7.1.8 / §7.1.10）。**`anc serve` 不存在、也不该存在** —— 常驻交给上游 daemon（D4 拍板：`serve` 降级为 `apply`，见 `runtime/DESIGN.md` §0）。**仍未验证**：**macOS 腿（零覆盖，见议题 #23）**、多 bot 并发 |
| `runtime/DESIGN.md` | 运行层落地设计 **v0.1 草案（2026-10-06）**· §0 六条决策里 **D4 / D5 / D6 已拍板**（D3 只拍了「空闲重置关掉」，其余未拍板）、D1 / D2 未拍板；「必须实测清单」与逐条实测记录见其 §7 |


## 与上游的关系

`SPEC.md` v0.3 是**合并稿**：工程实现层（三层架构 / cc-connect / harness 双后端 / persona 七段式 / vault 规范 / inbox 写路径 / onboarding 五阶段 / 运维）以**上游 `HA7CH/ai-native-company`**（MIT，含 Climax Racing reference deployment 生产经验）为骨架；组织模型（中层五类工作 → 拆给谁 / 两类 Agent / 只存岗位）、七连接件、可逆性三档与单一出口、三个阶段与双节奏、判据，来自本仓库 v0.2。

上游的 `docs/LIGHT-MVP.md`、`docs/M1-DESIGN.md`、`docs/PLAN.md`、`docs/RESEARCH.md` 是**实现视图**：它们细化实现，不改口径。本仓库与上游的长期关系（跟随 / 分叉 / 回提 PR）见 SPEC §13 Q10，**待定**。

## 边界

- 凭证一律不进本仓库。
- 客户名 / 金额 / 内部人物不进对外材料。
- 知识性沉淀回知识库；本仓库只留**能力与用法**，不写私货。
