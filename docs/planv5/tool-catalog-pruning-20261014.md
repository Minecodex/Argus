# 模型工具目录收敛

2026-10-14 用户确认移除前一轮建议清单中的全部模型入口。当前工作区完整目录从 122 个业务工具收敛为 103 个，仍通过 `tool.search`、`tool.describe`、`tool.invoke` 使用。此记录描述代码范围；不表示已有部署已升级。

## 边界与影响

移除的是自有模型目录中的注册、Schema、发现元数据、隐藏配对占位、名称和展示夹具。管理门户、HTTP API、领域服务及确定性 Action Executor 继续提供账号、通知渠道、消息模板、合并设置和 Windows RDP 配置。功能权限、资源授权、Preview/Commit 与宿主确认保持原职责。

本次明确收窄 AI 配置范围：模型可读取已有配置、选择账号和通知渠道，继续管理策略、屏蔽、抑制和通知路由，执行告警处理及通知测试/补发；不能通过自有工具修改企业评估账号、通知渠道、共享模板、通知合并设置或启用 Windows RDP。未增设配置助手或新的会话模式。

## 移除清单

| 分类 | 工具 | 数量 |
| --- | --- | ---: |
| workflow | `pending_action.list` | 1 |
| host | `host.windows_rdp.enable.preview` | 1 |
| alert | `alert.settings.create/update/delete.preview` | 3 |
| alert | `alert.channel.create/update/delete/enable/disable.preview` | 5 |
| alert | `alert.grouping.create/update/delete.preview` | 3 |
| alert | `alert.template.create/update/delete.preview` | 3 |
| dashboard | `telemetry.dashboard.convert` | 1 |
| dashboard | `telemetry.dashboard.draft.drilldowns` | 1 |
| dashboard | `telemetry.dashboard.budget.get` | 1 |
| 合计 | 斜杠表示多个独立操作名称 | 19 |

旧名称在 Search 中不存在，Describe/Invoke 返回 `TOOL_NOT_FOUND`；具备全部权限的模型也不能调用。`pending_action.get/cancel`、四类告警配置列表、通知测试/补发和 Workspace 导入/文件发布继续保留。

## 转换和下钻替代能力

共享 `DashboardDraftInput` 新增可选 `preparation`。`draft.create/save` 和 HTTP 草稿服务通过同一领域实现调用编辑器的无损转换和平台标准下钻生成器。选项包括：

- `conversions[]`：明确的 `panel_id` 和目标 `mode=builder|dsl`。
- `drilldowns[]`：明确的 `panel_id` 和 `signal_sources`，仅引用已支持的来源。

未提供选项时不改写传入定义。转换先执行，下钻随后执行；失败不保存部分结果。草稿保存仍核对编辑者、资源权限、生命周期和 `expected_version`，所有准备工作与正文在一次版本更新中持久化。发布仍需独立校验与宿主确认，模型不得提交 Commit。

## 预算替代能力

Agent 原有 `dashboardcontext.Facts` 在每轮模型上下文组装时读取当前 Run 的数据库预算，并提供 `run_budget_status` 和 `run_budget_observed_at`。有账本时状态为 `ready`；未发生预留时为 `not_started`，不伪造余额。数据库失败继续返回错误。

查询结果里的预算是返回时点快照；下一轮，包括文件分析、查询失败和恢复后的轮次，使用新读取的服务端事实。累计预算、失败消耗、在途预留和新 Run 隔离规则不变。移除独立预算工具不放宽额度。

## 当前完整目录

| category | 数量 |
| --- | ---: |
| host | 12 |
| k8s | 19 |
| metric | 2 |
| trace | 1 |
| log | 1 |
| connector | 11 |
| workflow | 4 |
| dashboard | 15 |
| alert | 38 |
| 合计 | 103 |

加上三个元工具后，展示覆盖声明及生成夹具共有 106 个绑定。Sandbox 四基础工具与客户 MCP 仍独立接入，不计入业务目录。

## 验证

新增注册目录回归覆盖九分类分页、全部 19 个旧入口的 Describe/Invoke 拒绝及保留入口。草稿回归覆盖省略选项、共享转换/下钻、无部分修改、一次版本保存和旧版本拒绝。真实 PostgreSQL 回归覆盖最新预算事实、错误后预算读取及草稿持久化。

前端继续使用同一 OpenAPI 生成类型，Mock 草稿路径支持可选 preparation；Mock 数据不作为真实查询证据。检查结果：

- Go 相关包与契约检查通过；隔离 PostgreSQL 中的 Gateway、草稿保存/冲突、预算事实、告警、会话、展示与 HTTP API 回归通过。
- 前端全工作区类型检查通过；API Client 原 104 项及新增 preparation 用例通过，Template Runtime 665 项通过；变更文件 ESLint 通过。
- Playwright 桌面端仪表盘及告警中心 11 项通过，包括宿主确认发布、渠道默认配置和已有通知目标保留。此项使用明确的 Mock 门户数据。
- GLM-5.3 真实模型检索验证尝试受到 HTTP 429 限流及一次连接超时影响，未获得完成证据；未切换免费模型，不能记为真实模型通过。
- 未重新部署现有 Argus 服务。临时 PostgreSQL namespace `argus-tools-prune-20261014` 已核对原始 UID 与所有权标签后删除，并确认不存在；未停止或清理其他命名空间。

本次证据位于 `build/tool-catalog-prune-20261014/`，包括 `postgres-integration.log`、`alerting-integration.log`、`native-boundary.log`、`browser.log`、`model-smoke.log`。首次 PostgreSQL 全组日志含一个展示测试失败：公开结果是正确的 JSON 数组，旧断言只接受特定 Go 切片类型；改为按 JSON 契约校验资源名称后，告警全包复跑通过。原始失败日志保留。
