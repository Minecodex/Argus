# Kubernetes 工作负载历史修订回滚

2026-10-19，Deployment、StatefulSet、DaemonSet 增加显式历史修订选择与 Pod 模板回滚。使用现有 Namespace 范围、工作负载权限、原生 dry-run、差异确认、PendingAction、异步 Worker、结果观察及审计流程。

门户使用共享分页列表弹框选择历史对象，展示修订号、名称及资源版本。候选仅包含同 Namespace UID、由当前工作负载 UID 控制的 ReplicaSet / ControllerRevision；不能根据同名对象或标签认领历史。搜索按历史对象完整名称，分页未结束时空页不表示没有其他候选。弹框取消保留父表单。

批准内容冻结目标工作负载及历史对象的 UID、resourceVersion、修订号。实际模板在执行端从 Kubernetes 读取，调用者不能提交一个模板冒充历史修订。历史被替换或修改后拒绝提交，需重新读取并预览。回滚模板仍检查主机特权和 ServiceAccount 身份许可，不能借旧修订绕过当前权限。

Deployment 从受控 ReplicaSet 取完整 Pod 模板，并去除控制器生成的 `pod-template-hash`。StatefulSet / DaemonSet 只接受 ControllerRevision 的模板替换形式，拒绝夹带其他 spec 或 metadata 变更的补丁。模板整体替换，当前副本数、更新策略和工作负载元数据保持当前值。此入口的范围是 Pod 模板；不把它描述成整个对象或数据卷恢复。

原生写协议升级为 `argus.kubernetes_mutation/v3`。Connector、控制面、执行端使用相同版本，旧版本在 API 执行前明确拒绝，不保留长期双轨。预览差异、确认标题和操作记录包含具体历史修订；敏感字段继续使用既有投影，不写入公开结果。

Kubernetes 的 [Deployment 回滚说明](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#rolling-back-a-deployment)及 [kubectl 原生实现](https://github.com/kubernetes/kubectl/blob/master/pkg/polymorphichelpers/rollback.go)作为模板/历史结构参考。StatefulSet 使用 OrderedReady 且坏版本 Pod 无法就绪时，单独恢复模板可能无法推进，需要明确的 Pod 操作；[官方说明](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#forced-rollback)所述情形不由本入口隐式删除 Pod。仍等待实际就绪，不把更新 API 成功当完成。

| 验证范围 | 证据与结果 |
| --- | --- |
| 原生回滚约束 | `artifacts/plan8-workload-rollback-unit.log`：三类模板恢复、完整替换、UID/RV/Owner/修订号、历史补丁注入及旧特权拒绝 |
| 原生实际 API / Pod | `artifacts/plan8-workload-rollback-live.log`：三类工作负载实际 Pod one→two→one；预览无滚动，伪造历史 UID 拒绝；自有 Namespace 清理 |
| PostgreSQL 审批到实际 API | `artifacts/plan8-rollback-postgres-api-workflow-settled.log`：三类对象经预览、确认、Worker、状态观察和 Execution 收口，通过 |
| 跨版本拒绝 | `artifacts/plan8-rollback-domain-protocol.log`：Connector 在读取凭据/连接 API 前拒绝旧协议 |
| 门户 | `artifacts/plan8-workload-rollback-portal-accessible.log`：中英/明暗四场景，每场覆盖三类对象、搜索、取消、确认、实际 mock 结果及可访问性 |
| 全局回归 | `artifacts/plan8-rollback-all-web-unit.log`、`artifacts/plan8-rollback-web-lint.log`、`artifacts/plan8-rollback-real-backend-regression.log`：前端单测/样式/i18n 与六包真实 PG/ClickHouse 回归通过 |

首轮真实工作流中 Deployment 旧 Pod 尚在退出，历史对象版本变化被正确拒绝；测试等待该受控滚动实际结束后重新读取并预览，成功记录保留在 settled 证据。没有放松 UID/RV 检查，也未重放未知写入。首轮门户可访问性检查发现长差异区域缺少键盘入口，已在共享 DiffViewer / CodeBlock 增加带名称的可聚焦滚动区域后重跑通过。

以上是原生组件、真实后端工作流和 mock 门户的分层证据；尚未覆盖正式注册 Connector、部署后门户/Agent 完整回滚链路、坏 StatefulSet 的人工修复、Helm 或其他尚未交付的工作台功能。Plan8 整体仍在实施，未发布。
