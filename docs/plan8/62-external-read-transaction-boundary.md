# 外部预检与监控提交事务

状态：修复已实现，真实隔离 PostgreSQL 验证及相关包最终回归通过（`artifacts/plan8-preflight-final.log`）；正式链路待重跑。

## 正式故障与已通过范围

r30 已通过主机文件日志初读、轮转、停启 checkpoint、强制 Gateway Pod 故障、旧命令跨代次补报、一次实际 Collector 启动、路由恢复后的新主机样本、Journald 系统归属查询和规则独立停止。K8s 工作台 ConfigMap 变更、Deployment 扩容、终端、端口转发和实时日志也已通过；正常 API 安装的 K8s Collector 收到了真实 `k8s_deployment_available=2`。

随后业务 `k8slog` 创建在后台执行阶段失败，Worker 报告 `SQLSTATE 40001`，五次任务重试耗尽。证据位于 `artifacts/plan8-completion-deployed-r30/`，包括 `plan8-deployed-host-logs.json`、`plan8-collector-reconnect.json`、`plan8-k8s-workload-metric-response.json` 和 diagnostics。CRI 持久日志和卸载未执行，不能把本轮称为全部 K8s 采集通过。原环境已恢复，原 server 1/1，本轮 Namespace 已清理。

原执行事务在锁定 Execution/Action 后，通过 Workbench 再次实时读取工作负载。Connector 查询会在独立事务内创建命令及适用的凭据租约并等待远端返回。将这种外部 I/O 放进 SERIALIZABLE 领域提交事务，会延长锁定范围并引入相互依赖。Namespace 权限解析本身只读，不能将故障归因于它刷新了授权目录。

## 修订边界

新增可选内部准备接口，在监控预览、人工确认、策略自动确认及后台执行各入口的提交事务前完成外部只读预检。现有扩展链向下转交准备能力，不增加模型工具、公开预检凭证或替代提交接口。

准备阶段从数据库读取不可变计划，核验规范 JSON 的 SHA-256 与授权版本。它不消费或暴露私有一次性 Token。K8s 读取仍经过现有 Workbench 的权限、Namespace UID、对象 UID、Owner 和容器解析，不接受客户端声称的归属。

实际查询结果只保存为本次请求内的私有 context 快照，绑定可信主体、集群、完整工作负载请求、操作 ID 和规范计划摘要，有效期 30 秒。预览快照不具有执行能力，不能传给另一个操作使用。

短 SERIALIZABLE 事务仍重新检查当前授权、Namespace 身份和版本、Collector 和配置版本、应用服务归属、规则冲突、计划摘要及私有 Token。事务内不再创建 K8s 查询命令。缺失、过期或绑定不同的快照直接拒绝；目标、采集实例、凭据绑定和 Outbox 仍在原事务内提交。停止或归档规则仍允许按既有权限退役已消失的工作负载，不为清退强制发起远程查询。

这项调整不降低事务隔离级别、不增加任务重试次数，也不放弃对象 UID 复验。外部系统与 PostgreSQL 不存在分布式原子快照；对象可能在预检后改变，后续 Collector 的 UID 匹配与实际生效诊断继续负责识别这种变化。

## 验证要求

真实 PostgreSQL 测试通过独立连接以 NOWAIT 获取 Action/Execution 锁，验证远端预检期间没有持有这些锁；正常预览、确认、执行各读取一次，已结束任务重放不再次查询。拒绝场景覆盖缺失/过期快照、Namespace 重建或版本变化、撤权、快照跨操作复用、计划篡改；拒绝后没有创建采集实例，私有 Token 保持未消费。

正式下一轮必须使用正常 API 创建业务日志规则，实际采集 CRI 数据，独立启停并卸载，再继续数据库、SDK、模型和发布门禁。组件测试不能替代这些正式结果。

## r31 完整扩展链纠正

r31 通过主机完整日志阶段和 K8s 实际工作负载指标，但日志确认返回 409。实际 Server 与 Worker 的最外层是 Workbench 扩展；最初的测试只覆盖 RemoteExecution → Monitoring，漏掉最外层可选准备能力的转交。缺失快照被确认事务正确拒绝，没有放宽校验。现补齐 Workbench 转交，并将真实 PG 夹具改为与 Server、Worker 相同的完整链；`artifacts/plan8-preparation-full-chain.log` 通过。

r31 原环境恢复 passed、原 server 1/1、本轮 Namespace 清零，尚未到达持久 CRI 样本和卸载。此次失败保留，下一轮同时验证完整链和 [Gateway 持久状态](./63-kubernetes-gateway-persistent-state.md)。
