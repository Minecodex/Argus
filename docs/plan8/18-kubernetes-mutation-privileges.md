# Kubernetes 写入的显式权限与生效配置

状态：边界实现与分层验收进行中；T11 仍未完成，未统一发布。

原先的 `clusterWide` 同时承担范围选择和特权放行，会让“具有 Cluster 数据授权＋普通对象管理权限”的用户跳过工作负载与 RBAC 提权检查。现在只把它用于确定对象范围；新增功能权限分别表达额外能力，权限注册表升级为 16。

| 权限 | 作用 | 范围前件 |
| --- | --- | --- |
| `kubernetes.workload.privileged` | host namespace、HostPath、特权容器、HostProcess、特殊 capabilities、Unmasked proc 等 | 已授权整个 Cluster；Namespace-only 不放行 |
| `kubernetes.service_account.use` | 使用非默认 ServiceAccount、API Token 自动挂载、显式 projected token、服务账户 Token Secret | 已授权目标 Namespace 或 Cluster；实际可使用的是所选 ServiceAccount 的 Kubernetes API 权限 |
| `kubernetes.rbac.escalate` | 创建或修改角色、角色绑定，显式委派 Kubernetes API 权限 | 同时具有 `kubernetes.rbac.manage` 及 Cluster 范围授权 |

角色/绑定创建或修改统一要求明确委派权限，因为平台连接本身可能是 cluster-admin，不能拿该连接的权限冒充操作人的权限。删除仍经过相应管理权限、UID/resourceVersion 与作用域检查。新增权限不自动添加到资源操作员等普通角色，也不授予任何 Cluster/Namespace 数据范围。

普通新建 Pod/工作负载显式补上 `automountServiceAccountToken: false`，由原生服务端 dry-run 返回到差异预览。已有工作负载在扩缩容时不被悄悄改写；若其模板使用 API 身份，操作者需要相应使用权限。持有主机特权权限也不会自动取得 ServiceAccount 使用权限。

权限检查覆盖 Controller 模板、CronJob 模板、Pod/init/ephemeral container 和 admission 返回的生效对象。原生客户端在实际写入前再次 dry-run，复核策略与已批准的生效对象摘要；对象或 admission 结果变化需重新预览。敏感输入继续使用既有加密暂存及精确输入绑定，不把 Secret 内容转换成可猜测的摘要。

写命令统一升级为 `argus.kubernetes_mutation/v2`。Server、Reader、Gateway、Connector 与异步计划使用同一常量，旧版本不回退执行。计划冻结实际许可，PendingAction 复验和发送前逐项重新授权；限时权限自然到期，即使主体版本未递增，也不能继续发送已排队的写入。

## 已取得的证据

- [策略与领域单层回归](../../artifacts/plan8-kubernetes-explicit-privileges-unit-verified.log)：集群范围不等于主机特权，Namespace-only 不放行特权，ServiceAccount 身份独立，HostProcess/Unmasked proc 与 RBAC 委派受控。
- [真实 PostgreSQL 与真实 K8s API 工作流](../../artifacts/plan8-kubernetes-explicit-privileges-postgres-live-verified.log)：显式权限的作用域及自然到期、发送前拒绝，既有 ConfigMap/Secret 预览、确认、创建、更新、删除和结果收口回归通过。
- [真实 Deployment 与 Pod dry-run](../../artifacts/plan8-kubernetes-identity-policy-live-final.log)：普通新建的无 Token 默认值进入生效预览；错误批准摘要不写入；1→2 ready 副本、重启、版本冲突、删除验证；未授予身份权限时拒绝 Token，显式授予后允许 dry-run，未创建测试 Pod。

临时 Namespace 按 ownership 标签和 UID 清理。真实 API 测试仍由测试进程调用原生适配器，未替代已部署 Connector/门户/Agent 全链路验收。

本页没有声明 Namespace 是业务数据沙箱：显式工作负载管理本来就能改变应用执行内容并使用其配置；ServiceAccount 使用、终端、RBAC 委派等权限需按其实际执行能力分配。Kubernetes 自身 RBAC/admission 继续是第二层约束。动态 CRD/Operator 的业务副作用、Helm 逐对象授权、drain、存储影响、回滚及终端/转发会话仍在 T11 范围内待完成。
