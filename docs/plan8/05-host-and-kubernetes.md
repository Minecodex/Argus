# 主机与 Kubernetes 工作台

## 主机自身监控

主机页面继续管理资产、Connector 和 Collector 生命周期。自身能力包括基础 CPU/内存/磁盘/文件系统/网络、系统日志、受控文件日志、进程/系统服务以及 Collector 自监控；各平台只展示已验证能力。

- 固定指标用内置采集实例开关，支持配置子项/范围；文件日志按路径/解析配置建立规则列表。
- Linux Journald 与 Windows Event Log 使用对应平台适配，不假设 Windows 具备 Linux 路径、权限或指标。
- Collector 在线/证书/路由/desired/effective revision 的管理观测始终可用；详细自监控指标可以配置。不能关闭自身指标后连管理健康状态也消失。
- “由本机执行的采集”单独展示远程数据库等任务，不能混同本机资源。Host 查询默认仅含本机数据。
- Host 与 K8s Node 共存继续验证强身份绑定/Claim，避免两条路径重复采集同一机器或日志。
- 主机盘点使用已有真实资产观测；无数据不添加模拟值。安装/升级/卸载及路由变更仍使用既有领域服务。

## Kubernetes 四个入口

资源管理、组件监控、日志采集、仪表盘共用一个 Cluster 上下文。应用归属独立于 Kubernetes Namespace/Helm Release，不能照搬 KubeSphere 的租户/Workspace 模型。

### 资源管理必需范围

| 对象 | 读取 | 本期写入/操作 |
| --- | --- | --- |
| Namespace | 列表、详情、对象分布、事件 | 创建、有限元数据变更、带依赖预览的删除 |
| Deployment | 列表/详情/Pod/状态/事件/YAML/修订 | 创建、允许字段更新、扩缩容、滚动重启、回滚、删除 |
| StatefulSet | 列表/详情/Pod/PVC/状态/修订 | 创建/更新、扩缩容、滚动变更/回滚、删除；显示有序和存储影响 |
| DaemonSet | 列表/详情/节点覆盖/Pod/状态 | 创建/更新、滚动重启/回滚、删除；副本由节点调度，不提供普通副本输入 |
| Pod/container | UID、容器/重启、Owner、事件、当前/上次日志 | 经独立授权的 exec、删除 Pod；不开放原始任意 API 代理 |
| Service/Ingress | 端点、选择器、路由、状态、事件 | 类型化创建/更新/删除，端口/路由/暴露方式在预览中明确 |
| ConfigMap | 受控数据、使用者和版本 | 创建/更新/删除；重载不自动等同应用重启 |
| Secret | 名称、类型、key 名、引用关系等脱敏元数据 | 受控创建/替换/删除；原值不经通用 YAML、模型或列表返回 |
| PVC/PV/StorageClass | 容量、绑定、挂载引用、回收策略/扩容能力 | PVC 创建、可支持的扩容、带数据影响预览的删除；PV/StorageClass 首期只读 |
| Node | 就绪、容量、角色、Pod、事件和资源观测 | cordon/uncordon、drain、标签/污点管理；显示 PDB、DaemonSet、本地数据与调度影响 |
| Job/CronJob | 任务、调度、历史、Owner、Pod/事件/日志 | 创建/更新/删除、CronJob 暂停/恢复、手动触发 Job；重试和并发策略明确 |
| HPA/NetworkPolicy | 伸缩条件、指标、目标及网络规则 | 类型化或 Schema 校验的创建/更新/删除，展示依赖与影响 |
| Role/RoleBinding/ClusterRole/ClusterRoleBinding | 规则、主体、绑定和作用域 | 创建/更新/删除；不因一般资源编辑获得提权/绑定/模拟身份能力 |
| CRD/自定义资源 | API discovery、Schema、scope、状态、事件、YAML | CRD 定义与实例分别授权，通用创建/更新/删除；不为每种 Operator 建专用 UI |
| Helm Release | Chart/版本/values/资源/历史/状态 | 仓库及 OCI Chart 接入、安装/升级/回滚/卸载、values 编辑与逐资源预览；包含 hooks、失败和恢复 |
| Pod/Service 端口转发 | 固定对象/端口/会话状态 | 独立授权、受控连接、超时/撤权关闭和审计，不复用终端票据 |

以上全部纳入本次统一交付。通用 CRD 编辑按发现的类型、Schema 与授权处理；Helm 是 Release 管理，不取代业务 Application，也不自动增加应用商店、计费或集群安装产品。

支持 Cluster 全范围和 Namespace 独立授权；Namespace 身份固定到 UID，重建同名对象不继承权限。Namespaced 对象、watch、日志、exec、转发与 Helm 都检查当前范围；Node、CRD 定义、ClusterRole 等集群级对象需 Cluster 对应能力。用户获得 Namespace 权限不能借 Chart 内嵌集群资源扩大权限。

节点日志使用原生 filelog/容器解析与 Kubernetes 元数据组件，读写位置由原生 file_storage 持久化。仅开启节点日志时，DaemonSet 以 UID 0 读取 kubelet 的 CRI 文件，禁用提权、移除全部 capabilities、只读根文件系统；节点日志、hostfs 和 kubelet 证书均只读，仅 Collector UUID 下的 checkpoint 目录可写。Gateway 维持非 root，不挂载节点目录。元数据只需要所选 Namespace 的 Pod、ReplicaSet 和 Job 只读权限；仍由接入准备/显式授权授予，运行时协调器不自行创建 ClusterRole。

Helm 安装/升级先锁定 Chart 来源、版本/摘要、values 与渲染结果，对实际对象和 hooks 逐一查动作、作用域、权限和敏感字段。预览至执行之间不得替换制品；回滚/卸载与 CRD、hook、PVC 保留语义分别验证。通用编辑从固定 GVR 白名单演进为受控类型能力注册，不能提供绕过权限的任意 API 代理。

常规业务日志按 Application 读取；显式运维日志权限可在已授权 Cluster/Namespace 内读取来源可核验的实时及持久日志，不自动授予整个应用或 Trace 权限。终端、转发分别授权，底层执行可能访问业务数据，权限说明和审计必须反映这一事实。

Cluster 数据范围不自动包含主机特权工作负载或 RBAC 委派；ServiceAccount 身份使用也独立授权。普通新建默认关闭 API Token 自动挂载，生效字段进入差异预览，已有模板不在扩缩容时自动改写。实际写入前再次 dry-run 并核验批准摘要，限时许可发送前重验；当前实现、协议 v2 与证据见[显式写入权限](./18-kubernetes-mutation-privileges.md)。

### 查询与 watch

延续 Direct/Connector 两条路径，统一目标/证书/当前身份解析。对象必须有 GVR、UID、namespace/name、resourceVersion、观测时间及完整性；名称相同但重建后的对象不能沿用旧操作授权。

列表采用服务端分页/continue，标签/名称过滤有界；watch 有起始版本、恢复、过期重列、背压、主体撤权和关闭语义。门户刷新/切换集群清理旧请求，旧响应不覆盖新上下文。对象统计不能用第一页数量伪装全集。

YAML 读取需要字段级投影和 Secret 屏蔽；用户输入的 YAML 不能绕过受控类型注册、Schema、作用域及动作授权。生产编辑先结构化解析和校验，再做服务端 dry-run/差异预览；Demo 的有限字段解析器不能直接迁入生产。

Secret 的新值通过独立的 write-only 字段进入受保护表单通道，通用对象内容只允许保留标记或删除字段。服务端复用 Secret Keyring 加密临时输入，绑定操作者、授权版本、操作、Namespace UID 和变更摘要；公开预览只列出变动的 key。私有材料仅在 Direct 执行或 Connector 发包时解密，不写入命令持久化、模型上下文或 Workspace，取消、过期与终态清理。此通道的当前实现和分层验收见 [T11](./task-11-kubernetes-operations.md)，实现记录不等同于全部路径已验收。

### 写操作

预览冻结企业、Actor、Cluster、接入身份版本、GVR、UID、namespace、resourceVersion、字段变更、依赖影响和有效期。真实提交前重验权限与前件，应用允许字段集；冲突重新预览，不能 force 覆盖他人的变化。

控制面提交成功与工作负载就绪是两个阶段，需持续记录 rollout/reconciliation 结果。失败、超时和未知结果分类，保留执行历史。回滚目标是明确的历史修订，不能假定“上一步”在并发操作后仍是同一版本。

PVC 只允许后端/StorageClass 支持的扩容；不得将输入变小处理为可执行缩容。删除 Namespace/StatefulSet/PVC/Secret 等必须展示相关依赖与不可恢复影响，复用既有确认/审批策略，不临时绕开授权。

### 日志与终端

实时 Pod 日志与持久日志采集分别建模。前者是受限会话读取，后者是持续采集规则。固定 Pod UID、container、重启代次，支持有界 tail/follow、截断提示和断线恢复；Pod 替换后不得悄悄切到同名新对象。

当前交互日志实现见[47](./47-pod-log-sessions.md)：一次性 attach 票据、固定 Namespace/Pod/容器身份、每两秒增量读取、暂停时继续授权检查，轮转缺口明确停止。目标固定到平台 Cluster API 地址；共享凭据解析禁止调用者文件、执行插件、代理和身份模拟。该交付不包含终端/端口转发，也不等同持续日志规则的完整验收。

终端使用独立短期会话/票据与动作权限，明确 API 身份、容器实际身份、时效和审计。复用底层连接适配器但不共用 Collector 身份、人工票据与 AI connect 的授权能力。Secret 原文、kubeconfig 和服务账户 Token 不进入模型上下文。

## Kubernetes 组件指标

| 内置能力 ID | 组件/范围 | 关键前件 |
| --- | --- | --- |
| P8-B01 | 集群/工作负载状态 | Kubernetes API；模板若需 kube-state-metrics，必须明确部署/采集依赖或改写查询 |
| P8-B02 | 节点/容器指标 | Kubelet/cAdvisor、节点身份和权限；与 Host Claim 去重 |
| P8-B03 | API Server | metrics 端点、证书/权限、版本化 Histogram |
| P8-B04 | Scheduler | 可达监听端点、访问权限和支持版本 |
| P8-B05 | Controller Manager | 工作队列/控制器指标及端点访问 |
| P8-B06 | CoreDNS | 实际部署、指标端口与版本 |
| P8-B07 | etcd | 成员端点、专用证书/权限；托管不可访问则 unavailable |
| P8-B08 | Collector 自身指标 | 接收、拒绝、队列、发送、丢弃、进程与配置状态 |

界面直接启停，系统创建/更新内置实例及对应模板引用。开启后先显示 applying/waiting，再由实际样本变成 healthy。模板存在或 DS Ready 不代表某组件已采到指标。

K8s 组件通常由 Collector 抓取暴露端点，用户不需新建“Prometheus 服务”。端点未开放时展示具体缺口，不改防火墙、控制面监听配置或权限来偷偷达成“开启成功”。

## 工作负载日志规则

选择 Cluster、Namespace、Deployment/StatefulSet/DaemonSet/Pod 或标签选择器、容器范围、解析配置，再绑定应用服务或声明组件日志。工作负载规则按 Owner 链持续跟随 Pod；固定 Pod 规则必须显示其生命周期限制。

采集在节点 DaemonSet 读取，Gateway 只接收/聚合；不能让集群网关直接读所有节点本地路径。日志规则与应用映射、目标对象、Claim、配置版本共同预览，容器范围变化/选择器重叠要校验。

执行任务：[T08](./task-08-log-collection.md)、[T09](./task-09-host-k8s-builtins.md)、[T10](./task-10-kubernetes-inventory.md)、[T11](./task-11-kubernetes-operations.md)。
