# 工作负载日志的可写配置与观测信息

状态：修复已实现并完成契约生成；相关 Go 包、客户端测试和门户类型检查通过，正式链路待重跑。

r32 已正式通过主机完整日志、K8s 工作负载指标、业务日志规则创建、Gateway Pod 按 UID 替换、相同 PVC 上报告身份保留与序号 7→8，以及替换后的新 CRI 业务日志匹配。随后停采预览返回 HTTP 400，本轮整体失败，尚未执行卸载与历史日志门禁；原环境恢复 passed，原 server 1/1，本轮 Namespace 清零。账本证据为 `artifacts/plan8-completion-deployed-r32/plan8-k8s-gateway-state-restart.json`，批次队列恢复仍明确未验证。

问题是驱动把查询返回的日志配置整份作为更新输入。工作负载响应包含服务端解析的 `namespace`、`namespace_uid` 和 `ancestors`，它们是只读观测字段，HTTP 请求校验正确拒绝。应用作为采集目标本身早已支持；无需扩大应用与资源绑定的目标类型，也不应取消只读限制。

现在将 `MonitoringLogWorkloadInput` 与观测响应类型分开。输入只包含 Namespace 稳定 ID、对象类型/名称/UID 和容器列表；`MonitoringCollectionConfigInput` 引用该类型，采集实例输入使用此配置。响应继续保留完整观测字段和 readOnly 声明。服务端仍独立解析 Namespace UID、对象及 Owner，并复验权限，不能信任客户端归属。

OpenAPI、Go DTO、Schema、生成客户端与原生工具输入一起更新。共享 API Client 的请求转换由 real/mock 两个适配器共用，构造新的五字段工作负载对象，保留其他配置并避免修改原始查询结果。Mock 在自己的可信验证层生成观测信息，使用响应类型表示它。纯启停使用专用 `set_enabled`，不携带目标、认证信息或完整日志配置；编辑/迁移时才构造完整的可写配置。

回归覆盖带应用目标的业务日志更新、观测字段不会进入输入、手工添加这些字段仍被拒绝、对象 UID 不被删除，以及 Agent 描述中不暴露只读输入。证据包括 `artifacts/plan8-writable-config-go.log`、`plan8-writable-config-client-test-r2.log`、`plan8-writable-config-typecheck-r2.log`。原资源关联模型、数据授权、Collector 0.3.2/上游 0.159 和迁移 49 均不改变。

下一正式轮必须验证独立停采、卸载及历史查询，并继续后续数据库、SDK、模型及发布。不能因本轮已查到实际日志就提前关闭整个 K8s 采集门禁。
