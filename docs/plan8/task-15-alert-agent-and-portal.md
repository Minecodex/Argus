# P8-T15：告警、Agent 与门户整合

状态：in_progress。前件：P8-T07、P8-T08、P8-T09、P8-T11、P8-T12、P8-T13、P8-T14。

完成度核对之后，Server/Worker 已接入 20 个新的应用/监控/通用 K8s 工作台工具、分类说明及共享 Presentation，实际数据库及 SDK 目录隔离与撤权通过，见[原生领域工具](./33-native-domain-tools.md)。敏感宿主表单、Workspace/真实模型、完整告警及尚未交付的 K8s 能力仍需继续完成；不能将通用对象工具等同完整工作台。

2026-10-20 当前领域工具合计 25，可信宿主敏感表单、drain 取消入口、Namespace 指标与多实例执行责任已补齐组件验证；正式部署又修复了模型预检/首请求上下文不一致及 Gateway 的旧免 Key Profile，引导采集后按稳定逻辑实例核对数据。详见 [57](./57-completion-delivery.md)。真实 Provider 的创建/修改/分析与完整告警链路仍未完成，任务维持进行中。

## 设计与接点

- [04-credentials-and-api-contracts.md](./04-credentials-and-api-contracts.md)
- [06-dashboard-scope-and-integrations.md](./06-dashboard-scope-and-integrations.md)

已有/拟扩展接点：`internal/alerting`、`internal/toolgateway`、`internal/toolruntime`、`internal/workspace`、`internal/presentation`、`web/packages/ui`、`web/apps/enterprise/src`。新领域包和具体接口在 T01 固化；路径是定位入口，不代表本任务已实现。

## 实施清单

- [ ] 本期暂缓目录按[专用插件复查](./13-direct-receiver-review.md)与progress.json同步；资源无专用采集组件时不新建采集、不提供可用默认仪表盘，门户/API/Agent一致。
- [ ] 统一单环境应用、Namespace权限和显式运维日志能力；受管默认模板自动升级后新分析用新版本，旧Workspace/告警快照不重写。
- [ ] 把新目标/应用/来源范围接入 Plan6 策略、后台 ServiceAccount、事件/聚合/通知和默认建议，不额外开启生产通知。
- [ ] 明确停采、恢复采集、NoData/Error、目标异常与采集异常的评估行为，保留策略/事件冻结范围。
- [ ] 扩展原生工具分类目录、Describe、Preview/Commit 和共享 Presentation；模型输入不包含密码、Key 原文或私有提交材料。
- [ ] 通过受控宿主敏感表单完成凭据输入，AI 可在确认后读取诊断/查询结果继续分析，不能以业务变更工具绕过 Plan7 执行授权。
- [ ] 查询文件、Workspace 元数据和下载/导出授权纳入新资源/关系/来源签名；真实结果按既有文件工作流分析。
- [ ] 完成各模块导航、数据表、选择器、异常/空态、双语主题、共享组件、OpenAPI 客户端及 real/mock fail-closed 对齐。

## 完成标准

- [ ] 同一操作在门户/API/工具使用同一服务与权限；工具仅传 category 发现、先 Describe 后 Invoke，隐藏 Commit 不被发现。
- [ ] 真实告警、模型、Workspace 和门户验证多目标取数/分析/创建/修改与撤权，结果与实际配置/样本相符。
- [ ] 相关存储、接口/生成客户端、权限、门户/工具、审计、配置和文档保持一致；单文件小于 2000 行。
- [ ] 保存测试范围、失败迭代、源码/版本与证据；只完成 mock 或单层测试不计真实验收。

## 验收关联

对应 [验收矩阵](./09-acceptance-matrix.md)：P8-A53、P8-A54、P8-A55、P8-A56、P8-A57、P8-A58、P8-A59、P8-A60、P8-A06。

新增验收关联：P8-A65、P8-A66。

## 实施记录

当前实施与验证层级见 [实施记录](./14-implementation-log.md)。尚未完成本任务全部验收；同时更新 [progress.json](./progress.json)，不预填通过结果。


原生 Trace 工具现支持有界 operation_name / variables，真实 SDK 数据经会话 HTTP 与原生 Tool 的应用隔离、服务筛选、详情、日志关联和 PromQL 已通过，见 [联合证据](./30-native-sdk-pipeline-and-signal-privacy.md)。真实模型、Workspace 完整闭环和正式门户仍单独验收。


2026-10-20 新增三个 Helm 工具（列表、历史、回滚/卸载预览）及受保护门户流程，领域工具总数 23；图表和 values 不进入模型，见 [51](./51-helm-workbench.md)。受控门户和原生 API 证据分开记录，完整模型/Workspace/告警与部署链路仍未完成。
