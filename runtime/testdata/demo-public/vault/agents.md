# 业务 agent（演示租户）

一行一个业务 agent。`domain` 挂业务域；`role` 代哪个岗位；`tools` 非空是红线（演示统一只给只读上下文）。

| slug | name | domain | role | tools | harness | model | description | app_id |
|---|---|---|---|---|---|---|---|---|
| receptionbot | 接待助手 | cs-ops | cs-owner | anc_read_context | dsh | | 承接客服接待会话，按需检索知识与 Skill | cli_demo_receptionbot |
| shopinitbot | 店铺初始化助手 | delivery-ops | delivery-owner | anc_read_context | claudecode | | 从工单取店铺初始化任务，校验后给出配置 | cli_demo_shopinitbot |
| workorderbot | 工单助手 | delivery-ops | delivery-owner | anc_read_context | codex | | 判定工单是否达标并流转，输出日报与预警 | cli_demo_workorderbot |
| bpobot | Prompt 调优助手 | agent-bpo | eng-owner | anc_read_context | hermes | | 从会话记录定位节点与 prompt 段落，产出调优建议 | cli_demo_bpobot |
| salecasebot | 案例匹配助手 | sales-enablement | cs-owner | anc_read_context | claudecode | | 查询标杆店铺并按类目匹配案例 | cli_demo_salecasebot |
| withdrawbot | 指标排查助手 | agent-bpo | eng-owner | anc_read_context | claudecode | | 排查应答 / 撤回指标落库链路 | cli_demo_withdrawbot |
| ancopsbot | 平台运维助手 | anc-platform | agentops | anc_read_context | codex | | 维护 vault、上下文投影与接入面运行状态 | cli_demo_ancopsbot |
