# 领域、数据与授权契约

## 身份分层

```text
Enterprise
├─ Application(单一固定环境) → Service(namespace＋name) → RuntimeInstance
├─ Host / KubernetesCluster → KubernetesObject(UID)
└─ MonitoringTarget(数据库/缓存/队列/Web/运行时/存储/探测)

Application/Service ──依赖关系── MonitoringTarget
CollectionInstance ──采集目标── ResourceRef
CollectionAssignment ──执行责任── CollectorInstance
TelemetrySource ──数据来源── Assignment/Receiver/安装代次/版本
```

统一 `ResourceRef {kind,id}` 和各领域适配器，不把现有 Host/Kubernetes 的业务状态复制到另一套资源表。目标类型描述业务实体，集成类型描述如何采集；一个 Receiver 可能服务多个业务类型，一个目标也可能有多种采集方式。

### 建议逻辑实体与约束

表名和 API 字段在 T01 固化为正式 Schema；下表是必须满足的领域语义。

| 实体 | 核心字段 | 约束 |
| --- | --- | --- |
| Application | enterprise/id/key/name/environment_id/owner/status/version | 每个应用一个固定环境；生产/测试分别建应用，不引入新租户层级 |
| ApplicationService | id/application_id/service_namespace/service_name/runtime/version/status | 应用内 namespace＋name 唯一，环境继承应用；允许受控 SDK 自动登记 |
| RuntimeInstance | service_id/instance_id/observed_at/placement_refs | SDK/工作负载身份可信映射；不以 Collector ID 充当服务实例 |
| MonitoringTarget | id/kind/engine/identity_config/status/version | endpoint 只是身份线索；实例名、网络域、Oracle Service 等进入明确身份契约 |
| ApplicationResourceBinding | app_id/optional service_id/target_ref/valid_from/valid_to/version | 当前关系用于资源历史指标范围，历次关系用于审计；不复制数据或授予权限 |
| IntegrationDefinitionVersion | key/version/signals/platforms/config_schema/capabilities/renderer/ref_templates | 类型定义版本不可变，支持状态不能从有同名目录推定 |
| CollectionInstance/Revision | target_ref/integration_version/typed_config/credential_ref/desired_state/revision | 同类型多实例；SDK 接入必选应用、服务可发现；文件/工作负载日志规则需受控服务映射 |
| CollectionAssignment | collection_id/collector_id/role/ownership_epoch/effective_time/status | 执行迁移保留 collection/target ID；物理来源另记 |
| CollectionClaim | target/physical_identity/signal/selector/owner/epoch | 防 Host 与 DaemonSet 或多规则重复采集；重叠按规范化范围判断 |
| TelemetrySource/Revision | logical collection/origin installation/receiver/source type/config revision | 区分原始来源与转发跳；不可伪造或用最后一跳 OTLP 覆盖 |
| MonitoringCredentialBinding | secret_ref/purpose/owner_ref/version/status | 复用 Secret/Credential；监控默认目录隔离，授权仍强制检查 |
| ApplicationIngestKey | app_id/environment_snapshot/optional service_allowlist/signals/key_digest/status/version | 默认应用级写入凭据，环境来自应用；允许受控发现服务，不能读平台 API 或跨应用 |
| CollectionObservation | desired/effective revision/health/last_attempt/last_sample/error/observed_at | 无观测用 unavailable，期望启用不等于采集健康 |
| KubernetesOperation | target UID/GVR/namespace/resourceVersion/frozen request/action/execution/status | 复用 Action/Outbox/Lease；变化必须重新预览，不重放未知副作用 |

配置持久化只进 PostgreSQL。Redis 仅做短租约/缓存/通知，重启或清空不得丢失配置、来源、授权、密钥撤销或操作记录。

## 新建、复用与生命周期

- 普通创建一次事务建立 target＋collection＋私有 credential binding＋outbox，失败不能留半个活动目标/凭据。预览草稿和短期凭据暂存按明确 TTL 回收。
- 从资源详情新增采集必须带入受信 target_ref 与 expected_version；前端不提供“新建/已有对象”占位下拉框。编辑采集保持 target ID，修改共享目标的身份配置属于单独可预览的资源变更。
- 重复目标识别先使用可信身份和网络域；只有 IP/名称相同不能自动合并。检测结果可提示复用已有目标，用户不用先手工建立资源对象。
- 关闭实例停止该实例采集，不卸载共享 Collector；保留保留期内数据。配置全部关闭时允许 Collector 保留控制健康能力，取消旧“至少一个业务 Profile”的强制条件。
- 删除采集先释放责任/凭据代用，再清理无引用的私有凭据；删除目标要检查仍依赖它的采集、仪表盘关联、告警和应用关系，不能静默级联业务数据。
- 卸载 Collector 显示全部受影响的采集实例，要求迁移或明确停采；受信来源历史保留。在线/离线与实例采集错误分开。

## 环境归属

Application 持有一个固定 environment_id；同一业务的生产、测试分别新建应用和独立 ID，权限按应用授予，不增加应用内部环境授权层。应用保存后变环境采用新建/复制及显式迁移接入，不原地改写身份。服务继承所属应用环境。

监控配置不接受独立 environment 覆盖值。仅选应用即可继承其唯一环境，不必再选服务或应用环境上下文。共享目标关联多个应用时可以得到多个环境，但每条关系始终指向确定应用；未关联目标保持未关联，不从执行主机/集群推断。

需要区分两类数据：

1. 应用遥测由确定服务产生，记录观测时可信 service/environment 身份。
2. 共享数据库等资源指标只有目标身份，应用环境是**关联关系筛选**；不能为每个应用环境复制一份指标，或把共享实例用量标成某应用专属用量。

基础设施自身采集保留 Host/Cluster 实际元数据。关联资源指标按查询开始时的当前绑定和当前目标权限取范围，可读绑定建立前的保留期历史，并展示关联时间及共享实例口径。解绑后不再经该应用的当前依赖列表纳入；目标详情在当前授权下仍可查历史。

业务日志/Trace 保留观测时应用/服务身份，不因资源关联变化转移。历次关系、已冻结查询、导出与告警事件保留原版本，不回写。新查询冻结当前关系 revision、目标成员和授权，执行途中不改变范围。

## 数据授权

复用 user/department/role/service_account、RoleBinding、DataAuthorizationGrant 和授权版本。不新增标签授权或独立 IAM 产品。

| 遥测目标 | 授权根 | 规则 |
| --- | --- | --- |
| 应用服务及其业务 Trace/日志/指标 | Application | 覆盖该单环境应用的服务；生产/测试分别授权不同应用，不增加 Service 级 Deny |
| 数据库/中间件/探测目标 | MonitoringTarget | 显式目标授权；关联应用不继承数据权限 |
| 主机自身指标/系统日志 | Host | 保持现有 Host 授权 |
| K8s 基础设施指标/组件日志/对象视图 | KubernetesCluster 或 KubernetesNamespace | Cluster 授权覆盖下级，Namespace 仅覆盖该 UID 下对象；集群级指标/Node/CRD 定义仍需 Cluster 对应能力 |
| K8s 业务容器日志 | Application；或显式运维日志能力＋基础设施范围 | 常规读取按应用授权；运维路径只读其 Cluster/Namespace 内来源可验证的日志，不获得整个应用或 Trace 权限 |
| 共享依赖统计图 | 参与目标的授权集合 | 不因能看应用仪表盘获得依赖数据；部分授权不得伪装成完整聚合 |

功能权限按领域操作注册并生成门户矩阵，例如 application.read/manage、monitoring.read/manage、collector.use、K8s 具体操作能力；最终名称在 T01/T03 与现有注册表收敛，不能绕过既有 host/kubernetes 功能权限。创建行为复用“创建成功给实际创建主体写入只读对象授权”的既有规则。

统一 ScopeResolver 保持功能权限与数据授权分离：资源页仍检查相应 `host.read` / `kubernetes.read` / `application.read` / `monitoring.read`；仪表盘入口沿用 `telemetry.dashboard.read` 加每个目标的显式数据授权，不因查看已授权仪表盘而要求进入资源管理菜单。命名空间和业务日志的行级限制对所有入口同样生效；功能权限不能替代数据授权。数据库事务冲突按事务错误重试，不能转换为授权拒绝后吞掉。

分开检查：读取应用遥测、配置采集、使用执行 Collector、使用私有凭据、管理目标、执行 K8s 操作。知道凭据 ID/Collector ID 或有全局管理菜单不等于被授权使用。

Namespace 授权贯穿分页/watch、目录、实时/持久日志、查询、终端、端口转发、Helm 和工具；删除重建同名 Namespace 不继承旧 UID 授权。终端/转发独立授权与票据，普通集群读权限不等于运维内容权限。底层执行能力可能读取容器内数据，权限说明必须明确该边界。

K8s 指标的对象归属无法由可信 UID 库存验证时，保留为已验证来源的 Cluster 数据，不授予 Namespace 行权限；已知身份矛盾仍拒绝，控制面故障仍重试。Namespace 查询明确只包含归属已验证的观测，详见[归属与采集可用性](./79-unverified-namespace-attribution.md)。

## 遥测与查询存储

- 维持企业物理表路由与现有三信号存储；`enterprise_id` 来自受信认证，不接受用户拼表名或伪造租户字段。
- 新增/调整受信 target kind/id、application/service ref、collection ref、来源版本/安装代次、部署位置、观测环境与关系版本。高频筛选字段是否物化由 T01/T06 的基数与查询验证确定，不把所有字段塞进任意属性 Map。
- Metrics 序列身份保持目标、来源/采集实例、指标语义、属性与 instrumentation scope 的明确区分；重复责任由 Claim/epoch 控制。业务聚合不能把重复采集当双倍吞吐。
- Trace 的逻辑服务身份与物理 Collector 分开；重复转发/重试按稳定 Trace/Span 身份和受信逻辑来源去重。跨厂商或未知来源不因名字一致自动混合；具体去重键由真实重复/迟到/迁移样本验证后冻结。
- 原始行的可见性在聚合前执行；Data Catalog、候选、缓存、游标、Workspace 清单和告警后台主体使用同一范围签名。
- 迁移采用版本化 DDL 与内部协议能力握手，旧服务不能以缺字段为理由回退到全资源查询。

相关任务：[T02](./task-02-application-resource-domain.md)、[T03](./task-03-scope-and-authorization.md)、[T06](./task-06-trusted-telemetry.md)。
