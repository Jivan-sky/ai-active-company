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

## 装配

扁平命名空间，三个独立入口，复制到 skills 根目录即可：

```powershell
Copy-Item -Recurse -Force .\skill\anc-distill,.\skill\anc-distill-targets,.\skill\anc-evolve "$env:USERPROFILE\.agents\skills\"
```

macOS / Linux 用对应的 `cp -r`。装到哪个根目录由宿主决定，本仓库不假设。

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

- 这三件**没有跑过压力测试**，也还没写 `test-prompts.json` —— 它们自己尚未被 `anc-evolve` 评过。
- 机制溯源是重写而非引用；其中 `relic.skill` 的许可证未核实，若要引用其模板原文需先确认。
