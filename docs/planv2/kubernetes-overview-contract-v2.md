# DashboardSpec v2 与 Kubernetes 运维大盘

## 版本与复用

新增 argus.telemetry_dashboard/v2；v1 的已发布仪表盘、草稿和历史修订继续读取，不批量重写。独立页与集群大盘共用 DashboardView、执行 Hook、网格、面板和放大视图，不使用 iframe。

data_source 为受控只读能力。省略 data_source 的 v1 遥测面板保持原语义。

| kind | 能力 | 展示类型 |
| --- | --- | --- |
| telemetry | 既有指标、日志、Trace 引擎 | 既有图型 |
| kubernetes_inventory | Kubernetes 工作台授权快照 | resource_summary、node_allocation、resource_distribution、resource_graph、resource_table、resource_events |
| alert_instances | 告警领域授权实例 | alert_summary、alert_list |

资源面板使用 resources 信号，告警面板使用 alerts 信号；没有任意 URL、凭据、DSL、详情查询或任意跳转定义。资源类型、状态和展示页大小由服务端校验。limit 控制列表视图页大小，不取消全局扫描预算。新图型在独立页、嵌入页和个人草稿编辑器可用。资源数据源的类型选择为内置目录；动态 CRD 实例沿用资源工作台的 UID 绑定清单与详情。

## 集群绑定与授权

默认受管模板为 kubernetes.operations-overview。默认只读，调整须创建个人副本并走现有发布流程。dashboard_bindings 增加 purpose=overview，每企业/集群仅一个大盘选择；普通 resource 绑定仍独立保留。恢复默认也更新选择版本，防止删除/重建导致 ABA。

服务端入口：

- GET /kubernetes/clusters/{id}/overview-dashboard
- GET /kubernetes/clusters/{id}/overview-dashboard/candidates
- POST /kubernetes/clusters/{id}/overview-dashboard/preview
- POST /kubernetes/clusters/{id}/overview-dashboard/execute
- GET /enterprise/kubernetes-clusters/{id}/inventory/snapshot
- 资源列表增加统一 state 过滤；游标签名包含过滤和授权范围。

更换/恢复通过 Pending Action 预览、确认、版本检查和审计。候选必须属于本企业、当前可读、已发布、未归档且所有面板适用于 Kubernetes。已选项失效时明确不可用，管理员可恢复默认，不静默换板。

嵌入执行锁定路径中的集群和当前授权 Namespace。客户端资源参数、应用参数、变量不能扩大该范围。Namespace-only 用户没有集群节点容量、其他 Namespace 数量或隐式关联读取权限。每轮结束重验授权与大盘选择。

## 快照、关系与预算

当前资源快照标注 observed_at；趋势使用用户时间范围。初始最近一小时，可见时默认每 30 秒刷新，隐藏和浏览器后台暂停，并取消旧请求。URL 保留时间和 Namespace。资源查询列表仍手动刷新。

同一轮 native 面板合并快照读取，不按行查详情。Kubernetes 快照最多 2,000 个对象、32 种内置类型、20 秒；局部关系图最多 200 个资源节点，超限要求缩小范围，不能静默截断。结果预算和 AI run 累计账本继续生效。

关系只连接当前授权快照内真实对象，使用 owner UID、同一 Namespace UID、Service 选择器、EndpointSlice targetRef、挂载/环境引用、HPA 目标和 PV claimRef UID。关系含义是拥有、选择、调度、路由、挂载或引用，不是应用调用拓扑。

各类读取失败记入不完整状态；无权限、未采集、失败、未知和真实零值不混淆。单个类型失败不影响不依赖它的节点/资源面板。告警摘要进入同一授权快照的分页列表，实例可进入告警详情。大量实例有扫描上限，并明确不完整。

## 默认布局

受管模板第 3 版统一面板内边距与网格间隔；摘要采用等宽统计块，趋势保留两列与足够高度，关系图独占一行。关系节点使用共享 UI 中可滚动的分层卡片布局，连线使用实际卡片坐标，避免密集关系图压缩重叠。历史零副本 ReplicaSet 和已完成 Pod/Job 可显式展开，折叠数量可见；服务端 200 节点上限仍然有效。

### 对象详情的分页读取

`GET /enterprise/kubernetes-clusters/{id}/inventory/related` 接受 `type/name/uid/namespace_id`、`view=relations|events`、`limit` 与 `cursor`。只读取当前主体有权查看的对象，源 UID 在读取前后核验。游标签名绑定企业、主体、授权版本、资源范围、源 UID/generation、视图及分页大小，不能跨范围或对象重建后复用。

一次最多扫描 8 个有界列表页，每页最多 100 个对象，15 秒读取预算。`next_cursor` 表示还可继续读取；`complete=false` 与类型化 `errors` 表示本次读取有失败，不将缺失内容当作零值。事件按真实 involved UID 筛选；关联包括 PDB 选择器匹配的 Pod/工作负载，以及 UID 绑定的 CRD 实例。详情不再下载 1,000 个对象后在浏览器过滤。

展开大盘的链接携带 `cluster_scope` 与完整 `namespace_scope`，独立页保持锁定范围及时间条件。范围参数始终由服务端重新授权校验；无效参数不得退回整个集群。

2026-11-13 补充：引用解析包括 Ingress TLS/IngressClass、projected 卷、工作负载模板、StatefulSet Service 和镜像拉取引用。ServiceAccount/RoleBinding 跨 Namespace 的候选读取使用当前主体的全部已授权 Namespace；最终仍按源引用、目标身份和当前权限匹配，不因知道引用名称而增加访问权限。

资源列表 URL 增加 `page_index` 与已有 `page_cursor` 配对。当前游标不是可逆游标；重新打开链接缺少前页缓存时，用户触发上一页后，按相同筛选条件从已知位置重建，每批最多 8 次、每次最多 30 个对象，允许取消及继续。范围变化取消恢复，过期/资源变化明确报错；不把部分恢复显示为目标页。没有页序的旧链接提示重新读取，不误标为第一页。

结果详情 URL 的 `resource_operation` 是客户端的原操作定位信息。显示原操作入口前重新读取并授权操作记录，并核对类型、名称、UID 和 Namespace。已不存在/身份变化的结果给出明确说明，保留原记录入口；不推断删除原因，不打开同名替代对象，权限错误继续独立显示。

状态摘要与活跃告警；节点/Pod 分布和状态构成；两列 CPU/内存使用趋势和请求趋势；局部关系、警告事件、异常资源及告警列表。可分配资源来自当前 Node，Pod requests 使用 Kubernetes 自身的资源计算规则，包含 init container 与 overhead；使用量来自真实遥测。无综合健康评分。

相关实现与验收：[资源工作台计划](../plans/frontend-redesign/kubernetes-resource-workbench-refactor-plan-20261112.md)、[资源能力矩阵](../plans/frontend-redesign/kubernetes-workbench-capabilities-20261112.md)、[本轮验收记录](../plans/frontend-redesign/kubernetes-workbench-acceptance-20261112.md)。
