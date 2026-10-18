# 业务域（演示租户）

一块业务一行。`slug` 是机器标识；`who` 填岗位，人名由 members/ 现算；`data` 指向 vault 顶层数据目录。

| slug | name | what | who | data | sources | terms |
|---|---|---|---|---|---|---|
| cs-ops | 客服接待与人效 | 客服工作台、人机协同接待、人效指标与验收 | cs-owner | reports | 工单系统 / 周报 | 保留英文：FTE、DSH |
| delivery-ops | 交付工单与流转 | 交付工单的达标判定、状态流转与预警 | delivery-owner | delivery | 项目看板 / 多维表格 | 保留英文：SLA、PIC |
| agent-bpo | Agent 与 Prompt 工程 | Prompt 归因、调优与指标链路探查 | eng-owner | knowledge | 代码托管 / 指标平台 | 保留英文：prompt、harness |
| sales-enablement | 标杆店铺与案例 | 标杆店铺查询、案例匹配与销售赋能 | cs-owner | delivery | 多维表格 | 保留英文：TOP10、KA |
| anc-platform | ANC 平台工程 | ANC 本体的接入、上下文治理与运行风险 | agentops | platform | 评审台账、运行日志 | 保留英文：MCP、vault |
