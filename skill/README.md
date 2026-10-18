# skill/ —— 给 Agent 读的一面

本目录是 `SPEC.md` 的派生视图，装**可单独安装的能力件**。改口径改 SPEC，不在这里各改一遍。

**现在这一层装的是「资产沉淀」功能的预存件。** 资产沉淀（把原料蒸馏成可调用的能力、并让能力持续变强）
是后续要做的功能，尚未实现；这里先把能力方法本身落到可安装的形态，避免以后从零起步。

## 能力清单

| 地址 | 能力 | 使用方法 |
|---|---|---|
| `skill/anc-distill/SKILL.md` | **蒸馏流水线**：定范围 → 6 路并行提取 → 三重验证 → 晋级门 → 能力卡（R/I/A1/A2/E/B）→ 链接 → 压力测试 → 编译交付。把原料（文档 / 会议记录 / 长文 / 人的痕迹 / 流程）变成 agent 能调用的原子能力 | `/anc-distill <原料路径或来源> [对象类型]`；在阶段 0 / 3 / 5 三处会停下来等 human 确认 |
| `skill/anc-distill-targets/SKILL.md` | **蒸馏对象与知识域**：12 类对象各自要哪些维度、最小证据集是什么、哪些话不许写；业务域怎么打包成「1 个路由入口 + N 个原子卡」；授权与红线 | `/anc-distill-targets <待蒸对象> [业务域]`；通常在 `anc-distill` 阶段 0 之前调用 |
| `skill/anc-evolve/SKILL.md` | **资产评测与迭代**：9 维 100 分评分卡、两条闸门项（运行时中立 / 单一事实源）、优化循环（绝对分只排序，keep/revert 走同 judge 成对比较）、`results.tsv` 与 `test-prompts.json` 口径 | `/anc-evolve <skill 目录或域包> [--only-score]` |

三者是一条链：`anc-distill-targets`（定对象）→ `anc-distill`（做出来）→ `anc-evolve`（变强）。

### 已蒸馏出的能力件

上面三件是**方法**；下面是它们**蒸出来的产物**（`anc-distill` 阶段 8 的编译结果）。两者共用同一个扁平命名空间，装配方式相同；区别只在「谁是方法、谁是产物」。

| 地址 | 能力 | 来源 |
|---|---|---|
| `skill/kb-retrieval-acceptance/SKILL.md` | **自托管检索层验收**：把「找得着吗」与「照着答对了吗」分开算分，数「检索已命中、答案仍错」的条数定主因侧，并给出判停点 | 蒸馏自一次真实运行；过程记录在 `skill/runs/kb-retrieval-acceptance/`。**自用样例，不算客户验收** |

自带评测用例：`skill/kb-retrieval-acceptance/test-prompts.json`（8 条：正向 / 负向 / 近邻 / 输出断言）；评测台账：`skill/kb-retrieval-acceptance/results.tsv`（`anc-evolve` 首次分 **84.0 / 100**；闸门「单一事实源」已在第 1 轮修掉 —— 规则改为单文件单向维护；第 1 轮 keep，参考分 86.4）。

### 入口那一面

下面这件不是蒸出来的产物，是**接入面**的能力件 —— 它管「模型怎么把材料交进真相源」，与资产沉淀那条链并列。

| 地址 | 能力 | 边界 |
|---|---|---|
| `skill/anc-ingest-propose/SKILL.md` | **外部材料进真相源的提案**：把一段材料读成一份 `anc ingest` 提案（只提案、不落盘），自检走 `--dry-run --patch`，人看过 diff 再由管道落 | 口径来自议题 #47 与 `anc ingest` 那条管道；形状不抄文档，读 `anc ingest --dump-shape` |

它只说方法与边界（只提案、不落盘、形状照 `--dump-shape`、抽不到就空着）；校验 / 原子写 / 留痕全在 `anc ingest` 那一条管道里，不在这里重复实现。

### 交付那一面

再往下这件也不是蒸出来的产物，是**交付流程**的能力件 —— 它管「怎么把一家公司从零带到能跑」，
与资产沉淀那条链、上面那条接入面都并列。

| 地址 | 能力 | 边界 |
|---|---|---|
| `skill/anc-onboard/SKILL.md` | **冷启动五阶段**：访谈 → 名单 → 资料 → 装机 → 验证；向导与体检成对、做不了的显式列成 checklist、状态断点续走 | 口径来自 `SPEC.md` §9.2；能固化的部分全在 `anc` 现有的命令里（`org init` / `org check` / `render --check` / `apply` / `probe` / `audit` / `doctor` / `ingest`），这里只写流程与检查点 |

它**不复写** `anc apply` 的七步、也不另造一份 vault 结构 —— 那两样各自只有一份实现。
阶段 4 那三条 checklist（建 IM 应用 / harness 登录 / 自动登录决策）是**机器做不了**的，
所以本件把它们写成引导式清单，而不是假装能自动。
**轻形态**复用阶段 1–3；轻形态的代码与成员侧能力件本仓库还没有。

## 装配

扁平命名空间，一个一个独立入口，复制到 skills 根目录即可：

```powershell
Copy-Item -Recurse -Force .\skill\anc-distill,.\skill\anc-distill-targets,.\skill\anc-evolve,.\skill\kb-retrieval-acceptance,.\skill\anc-ingest-propose,.\skill\anc-onboard "$env:USERPROFILE\.agents\skills\"
```

macOS / Linux 用对应的 `cp -r`。装到哪个根目录由宿主决定，本仓库不假设。

## 与客户 vault 的 `skills/` 不是同一层

| | 本目录 `skill/` | 客户 vault 的 `skills/<name>/SKILL.md` |
|---|---|---|
| 是什么 | **产品能力库**：ANC 自带的方法件，给 FDE 与任意 harness 装 | **客户真相源**：这家公司自己长出来的 skill |
| 真相源 | 本仓库 `SPEC.md`（本目录只是派生视图） | 客户 vault 的 git |
| 分发 | 复制到 harness 的 skills 根目录（见上） | 由 `render` 按角色分发（SPEC §3.4：Skill N—N Role，个人可另加载） |

两层都叫 skill，但**授权与生命周期各归各的**：产品能力库升级走本仓库；客户 skill 的增长走客户 vault 的 git 与 eval 回归。**别把产品能力直接写进客户 vault，也别把客户 skill 反向塞回本目录。**

## 机制溯源

这三件是**读完公开素材后重写**的，没有复制原文段落。借用的机制与出处：

| 机制 | 出处 |
|---|---|
| 多源并行提取 + 调研质量检查点 | `nuwa-skill` |
| 三重验证（来源充分性 / 可执行性 / 任务增益）+ 四类分流 | `cangjie-skill` |
| 晋级门五判据 + 可发现入口软预算 8 | `cangjie-skill` |
| 能力卡 R / I / A1 / A2 / E / B 六段 | `cangjie-skill` |
| 9 维评分卡 + 失败模式必填 + 反例黑名单 | `darwin-skill` |
| 绝对分只排序、keep/revert 走成对比较 | `darwin-skill` |
| 对象模板化 + 授权前置 | `relic.skill` |
| 域包 = 1 个路由入口 + N 个原子卡 | `toprank` / `patent-disclosure-skill` / `Master-skill` |

**为什么要重写而不是直接用**：那批素材是**给人读**的（金句、故事、情绪钩子）；
这里的关键字段是 **trigger / 可执行步骤 / 判停 / 完成标准**，目标读者是执行者不是读者。

## 检查点与硬门（2026-10-12 拍定）

这 6 条原先笼统叫「门禁」、**待拍**。2026-10-12 拍定：**5 条是检查点，1 条是硬门**
（[[立项书-ANC-2026-10-08]] §3）。

| 位置 | 内容 | 定性 |
|---|---|---|
| `anc-distill` | 阶段 0 / 3 / 5 三处 CHECKPOINT | 检查点 —— 流程停顿点，不阻断代码 |
| `anc-distill` | 可发现入口软预算 8 | 检查点 —— 软约束，超预算只降级不删除 |
| `anc-distill` | 三重验证全过才进能力池 | 检查点 —— 质量门，未过项保留不删 |
| `anc-evolve` | `dry_run` 占比 > 30% → 评估失效 | 检查点 —— 结论可信度门 |
| `anc-evolve` | 绝对分禁用于 keep / revert | 检查点 —— 方法约束 |
| `anc-distill-targets` | **授权来源未经 human 确认不开工** | **硬门** —— 挡的是越权蒸馏 |

**怎么分的**：检查点是「停下来让人看一眼」—— 不拦、不改代码路径，所以前 5 条要改是**文档改动**，
不会引起重构。唯一那条硬门挡的是**越权蒸馏**（拿没获授权的素材去做蒸馏）—— 那是**安全边界**，
不是流程偏好，所以「硬」这一档只留它一个。

## 未验证

- `anc-distill` / `anc-distill-targets` / `anc-evolve` 这三件**没有跑过压力测试**，也还没写 `test-prompts.json` —— 它们自己尚未被 `anc-evolve` 评过。
- `skill/kb-retrieval-acceptance/` 已编译并通过 `anc-distill` 阶段 0–6。**阶段 7 压力测试**：触发测试与执行测试**已真跑**，但执行测试的结论口径被独立复核修正四处 → 记为「**走完、待回炉**」，按规矩不得标「通过」；基线对照部分有效（3 条干净 / 2 条被污染）。记录在 `skill/runs/kb-retrieval-acceptance/06-pressure.md`，原始凭证在 `skill/runs/kb-retrieval-acceptance/08-exec-test/`。
- `skill/anc-onboard/` **没有被真实客户端到端跑过** —— 它引用的每条命令都各自实测过，但阶段 1–3 的访谈脚本与阶段 4 的 IM 后台 checklist **还没有真人走完一遍**，不得当「已通过」讲。
- 机制溯源是重写而非引用；其中 `relic.skill` 的许可证未核实，若要引用其模板原文需先确认。
