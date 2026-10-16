# 按分类完整列出工具

2026-10-14 用户确认将工具发现改为“分类列表 → 按名称读取详情 → 调用”。此决策替代原关键词 AND 检索及游标分页契约，工具标识仍为 `tool.search`。

## 输入与输出

搜索只接收一个必填字段：

```json
{"category":"alert"}
```

`additionalProperties=false`，`query`、`limit`、`cursor` 均已移除，传入旧参数返回 `TOOL_INPUT_INVALID`。

输出包含 `category`、`items`、`complete=true`、`catalog_revision`。一次返回当前分类中全部可用工具，按规范名称稳定排序，不返回 `next_cursor`。每项保留 `name`、中英业务标题、最长 256 字符的用途摘要、风险、是否需要宿主确认和版本；完整 Schema、前件、结果语义和示例由 Describe 提供。

模型从返回列表中选择名称，再提交：

```json
{"category":"alert","name":"event.query"}
```

成功收到这一工具的 Describe 响应后，模型在后续轮次构造 Invoke 参数。不得根据其他工具描述猜参数，不得在同一批次并行 Describe/Invoke 同一工具。

## 分类导航与能力边界

`internal/toolruntime/categories.go` 是模型分类名称和用途的共同来源。系统提示列出九类的职责和易混淆边界，`tool.search` 描述使用同一份导航，Schema 的枚举由同一份定义生成。名称不是所有用户均可用的能力承诺；调用时继续裁剪当前权限和 Run 业务范围。

| category | 用途 |
| --- | --- |
| host | 主机查询、接入、组件及主机采集器维护 |
| k8s | 集群、Kubernetes 对象、Pod 日志、采集器与节点主机绑定 |
| metric | PromQL 指标与遥测概览 |
| trace | 调用链、慢请求和 Span |
| log | KQL 日志检索、过滤及聚合 |
| connector | 连接代理、堡垒机、采集器清单和状态；客户 MCP 独立接入 |
| workflow | 操作状态/取消、Workspace 结果导入及文件交付 |
| dashboard | 仪表盘取数、条件、个人草稿准备、校验和发布预览 |
| alert | 告警/历史/通知/配置读取，策略、屏蔽、抑制、路由及处理预览 |

## 权限、版本和上下文

列表仍先应用当前功能权限、可见性和 Run 范围；隐藏 Commit 永不返回。分类完整列表每次重新应用当前范围，不使用可能在同一 Run 自动取得仪表盘后仍保留宽范围的列表缓存。Describe 的主体/版本缓存仍在授权检查之后使用。

三个元工具协议版本更新为 2；分类导航及协议版本参与 Catalog Revision。旧 Run 冻结的 Schema 不静默替换，不兼容快照按既有恢复规则返回受控版本错误。

列表标记为服务端发现元数据，Agent 投影保留全部条目和摘要，不按普通业务列表上限裁剪。Describe 的完整验证契约也不裁剪枚举、必填字段和示例。分类列表继续敏感字段过滤，原始结果 Hash/result_ref 与投影字节统计保留。完整消息仍受既有上下文组装及 Provider 请求容量门禁约束，不因目录过大而静默省略工具。

已删除的 19 个模型入口保持删除，管理门户和领域 API 保留。业务查询中的 PromQL/KQL `query`、对象筛选和其他目录分页保持其各自契约；此次只移除元工具搜索参数。

## 验证

回归覆盖仅 category 输入、旧参数拒绝、九类完整列表（含 38 个告警工具）、稳定顺序、撤权和同 Run 范围变化、隐藏 Commit、完整投影、大目录及系统提示导航一致性。既有 Replay、展示夹具和实际模型调用脚本同步新参数。

执行结果：

- 相关 Go 包及 API/JSON Schema 契约检查通过，支持 1/500/1000 工具完整分类列表及旧参数拒绝。
- 隔离 Kubernetes PostgreSQL 的 Gateway、领域、Agent、展示和 HTTP API 回归通过。首次 Gateway 测试执行在迁移前的既有 fixture 失败；迁移完成后整包复跑通过，原始日志保留。
- 新增 Agent 持久化链路回归用 80 个明确的合成工具完成分类列表 → 最后一项详情 → 调用，验证模型消息及不可变 ToolResult 完整性。这是控制场景，不代替真实模型自主选择证明。
- 全工作区类型检查、706 项模板 Runtime 回归及新增两项中英发现模板测试通过；变更前端文件 ESLint 通过。
- Playwright 桌面 1280 宽度下中英、明暗四组全部模板资产回归通过。
- GLM-5.3 真实模型尝试返回 HTTP 429，未记为通过，也未切换模型。
- 当前部署未升级；临时 namespace `argus-category-list-20261014` 已核对原始 UID 和所有权后删除，测试端口转发已关闭，未停止其他命名空间。

证据目录：`build/category-listing-20261014/`。含 `postgres.log`、`gateway-pg.log`、`agent-sequence.log`、`browser.log` 和 `model-smoke.log`。
