# 范围、决策与当前差距

> 最新执行决定（2026-10-19）：用户暂缓 Windows 实机测试。保留 Windows 11/Server 实现及构建，正式接入、服务生命周期、主机指标、事件日志和 IIS 实测均记为“用户暂缓、未验证”，不执行管理员准备。Windows Server 实机验收仍不作为本期发布门禁；此次暂缓不表示 Windows 能力已通过，也不作为永久验收豁免。Journald 允许调用系统自带 journalctl，仍禁止独立 exporter 和外部 JMX 采集服务。

## 已确认产品基线

| ID | 决策 | 实施要求 |
| --- | --- | --- |
| P8-D01 | 新增监控模块，按服务/组件组织 | 分类 Tab＋类型＋实例列表；Prometheus 抓取作为内部接入方式 |
| P8-D02 | 两批服务全部进入开源选型审查 | 有适用方案的组合完成采集、状态、仪表盘与验证；无合格内置方案的类型/模式按 D23 暂缓列缺项 |
| P8-D03 | 同类采集可创建多个实例 | 每个实例独立配置、启停、版本、诊断；同一 Collector 可监控多个 MySQL 等 |
| P8-D04 | 目标、采集实例、Collector 分离 | 执行位置不等于目标部署位置；迁移采集执行者保留目标身份 |
| P8-D05 | 增加应用模块，每个应用只有一个环境 | 同一业务的生产、测试分别创建独立应用；服务、实例、接入、APM、日志和资源关系在应用内管理 |
| P8-D06 | 业务 Trace/日志强制关联应用服务 | SDK、业务文件与业务容器日志均适用；系统/组件日志归属主机或集群 |
| P8-D07 | 外部服务关联应用可选且可多选 | 多应用共享资源不复制目标或遥测，也不授予额外权限 |
| P8-D08 | 监控不填写生产/测试 | 环境跟随应用/服务；未关联目标不默认生产，不按执行 Collector 推断 |
| P8-D09 | 应用和服务使用列表弹框选择 | 服务端搜索分页、跨页多选、取消不改草稿、应用变化收窄服务候选 |
| P8-D10 | 普通新建自动建立目标 | 不提供只有“新建资源”一个选项的对象下拉框；已有对象从资源入口带入 |
| P8-D11 | 认证信息在监控中直接填写 | 自动托管为监控用途凭据；平台凭据默认目录不混入监控记录 |
| P8-D12 | Trace 接入提供地址与 Key，SDK 上报发现服务 | 新建不要求预选服务；Key 固定应用、其环境及信号，服务从有效上报幂等登记；逐协议验证认证 |
| P8-D13 | 主机与 K8s 固定自身指标直接启停 | 内部仍有稳定采集实例和配置状态；动态日志使用规则列表 |
| P8-D14 | Kubernetes 提供在线资源管理 | 工作负载、Pod、网络、配置、存储等详情与操作，采用既有授权/审计 |
| P8-D15 | 每类采集配套默认仪表盘 | 参考 Grafana 社区配置，按实际指标合同转换和验证 |
| P8-D16 | 仪表盘增加应用及多类型资源筛选 | 时间/环境/应用/服务/目标/来源/K8s 对象＋面板适用性，服务端统一求交 |
| P8-D17 | 业务数据与基础设施权限分开 | 应用读权限不自动获得共享数据库或所在主机/集群权限 |
| P8-D18 | 原型与生产验收分开 | 模拟交互通过不计为真实集成、授权、采集或在线操作完成 |
| P8-D19 | 完整 K8s 在线管理纳入同次交付 | 加入 Jobs/CronJobs、HPA、NetworkPolicy、RBAC、CRD/自定义资源、节点维护/污点、端口转发及 Helm 管理 |
| P8-D20 | Namespace 可独立授权 | Cluster 全范围与 Namespace 显式授权并存；集群级对象另查 Cluster 对应操作权限 |
| P8-D21 | 显式运维日志访问 | 普通集群查看不自动读业务日志；独立授予的运维日志能力在其基础设施范围内读取，终端/转发另行授权 |
| P8-D22 | 默认仪表盘自动更新，应用筛选按面板生效 | 应用面板查自身，资源面板自动查有权依赖；受管默认入口跟随已验证新版，用户副本不覆盖 |
| P8-D23 | 仅采用内置Collector组件，无合格方案暂缓 | 资源指标需现成的专用Collector Receiver直接采集；通用Prometheus抓取、通用SQL或SDK接入不算该资源已支持。不使用独立exporter、外部JMX采集进程，不自研/fork补目录；缺项留证 |
| P8-D24 | 全部完成后统一发布 | 开发/测试可分批；全部纳入支持的组合与平台功能验收完成后统一发布，D23 缺项单独披露且不计通过 |
| P8-D25 | 当前关联可查询资源历史指标 | 新关联目标可查当前授权下保留期历史；业务日志/Trace 保持观测时归属，关系变更留痕 |

## 作为本计划执行基线的工程细化

这些细化用于让需求可实施，不是新增已发布能力：

- 一个应用只有一个环境，生产/测试是两个 application_id，不增加 ApplicationEnvironment 授权层。应用保存后环境固定；需要其他环境时新建/复制应用，不重写原数据归属。
- 服务按 application_id＋service.namespace＋service.name 幂等登记，协议/Collector 不改变服务目录身份；来源与重复埋点独立识别。缺有效服务名给出接入诊断，缺实例 ID 显示实例未知。
- Namespace 授权绑定 Cluster＋Namespace UID；Cluster 授权覆盖下级，Namespace 授权不扩展到集群级对象。应用遥测按 Application 授权；运维内容访问有独立能力及基础设施范围。
- 对数据库等外部目标新增显式监控目标授权；资源关系只用于归属/查询，不成为权限继承渠道。
- 集成数量多时用类型注册表、配置 Schema、编译适配器和能力描述驱动；不建立各页面自己的采集逻辑或平行组件库。
- 固定组件开关只改变 Argus 的采集期望状态；不因“开启监控”擅自修改远端组件的监听地址、权限或集群配置。需额外开通端点的步骤要明确展示并走对应变更契约。
- 同一进程多实例并不提供进程级故障隔离；整个配置需先校验、逐版本应用与回滚，必要时按资源预算拆分 Collector 分配。

## 当前代码事实与改造接点

| 当前事实 | 证据 | Plan8 变化 |
| --- | --- | --- |
| Collection Profile 仍作为启用键集合生成配置 | [config.go](../../internal/otelcol/configbundle/config.go) | 以 CollectionInstance＋参数＋绑定版本编译，Profile 成为模板定义 |
| 当前 Linux 发行清单锁定 Collector 0.133.0，未编入两批多数服务 Receiver | [builder](../../deploy/otelcol/builder-linux-amd64.yaml)、[Catalog](../../internal/telemetry/catalog.go) | 重新锁定发行矩阵；逐组件真实验证，按平台声明能力 |
| 来源资源必须等于 Collector 认证资源 | [source_identity.go](../../internal/telemetry/source_identity.go)、[SQL](../../internal/storage/postgres/queries/telemetry_sources.sql) | 按已登记的目标责任绑定逐记录校验，支持一个 Collector 的多个目标 |
| Dashboard 范围仅解析 Host/Cluster | [runtime_scope.go](../../internal/dashboard/runtime_scope.go)、[bindings.go](../../internal/dashboard/bindings.go) | 通用 ResourceRef、授权根及查询维度适配 |
| Secret 已有信封加密与凭据代用/租约 | [secret](../../internal/secret/service.go) | 增加监控用途/归属、事务创建和最小交付，不重写另一套 Vault |
| K8s Reader 已有 Direct/Connector 查询与有界 Pod 日志 | [reader.go](../../internal/kubernetesreader/reader.go) | 增加稳定对象 UID、分页/watch、类型化变更和独立会话 |
| APM 已有服务/实例/接口/RED/拓扑查询 | [APM 契约](../planv2/apm-query-contract.md) | 从 Host/来源名称分组演进到受信应用服务身份，保留来源与采样口径 |
| 三种查询引擎已确定 | [Query ADR](../adr/telemetry-query-engine.md)、[锁定清单](../../deploy/query-parsers.lock.json) | 复用 PromQL、Argus KQL、只读 Trace GraphQL，不新增 Grafana/Loki/Tempo/OAP 后端 |

此表是编制时的源码盘点，不代表本轮复验了现网、Collector 收敛或历史验收环境。

## 需要同步演进的既有文档

- [系统不变量](../00-decisions-and-invariants.md)与[显式授权 ADR](../adr/explicit-resource-data-authorization.md)：由 Host/Cluster 扩展到应用和监控目标，企业隔离与显式授权原则不变。
- [监控链路](../09-opentelemetry-observability.md)：多实例、业务目标身份、凭据用途与采集状态。
- [资源管理](../03-connectors-and-resources.md)：K8s 在线资源操作和日志/终端边界。
- [PlanV2 决策](../planv2/02-confirmed-decisions.md)：Q41 的 APM 入口扩展到应用；Q37/Q38 的来源与服务身份需以受信映射演进，不能按同名自动合并。
- [对象查询授权](../planv2/object-query-authorization.md)、[Plan6](../plan6/02-architecture-and-contracts.md)、[PlanV5 工具发现](../planv5/category-only-discovery-20261014.md)：统一新范围和授权失效。
- 远程命令遵循 [Plan7 当前命令授权](../plan7/18-markdown-plan-command-consent.md)，不恢复已被替代的固定命令列表规则；类型化平台业务变更继续 Preview/Confirm。

## 不由本计划默认扩大的产品边界

不新增企业内 Project/Workspace 租户层级、KubeSphere 应用商店/计费/集群安装产品、任意采集 YAML/SQL/Shell、公网监控平台聚合后端、移动端验收、Profiling 或方法级分析。Helm Release 与通用 CRD/自定义资源管理已纳入，不等于应用商店或每种 Operator 的专用 UI。Node 维护、驱逐和污点采用独立动作与验收，不随通用 YAML 权限隐式开放。

数据库集群拓扑按现成内置Collector组件能力逐模式选型，RAC/Always On/副本集/分片/Cluster/Sentinel 分别记录来源、版本与实测范围。没有合格内置方案可按 D23 暂缓；有方案但缺环境或测试未通过仍待验收，不能冒充上游缺项。

问答收敛与上游事实见 [讨论收敛与开源选型](./11-grilling-decisions-and-upstream.md)，最新单环境模型替代初版多环境应用假设；[内置组件约束](./12-collector-native-support.md)和[专用插件复查](./13-direct-receiver-review.md)替代独立exporter及通用Prometheus兜底路线。

## 技术前件，不以未知代替支持承诺

| ID | 前件 | 收敛任务 / 结果 |
| --- | --- | --- |
| P8-V01 | Collector 发行版、组件成熟度、OS/CPU 架构与依赖 | T01/T05：锁定清单、签名制品、支持矩阵 |
| P8-V02 | Oracle/SQL Server/IIS/Windows 等真实可用环境 | T01/T12/T13/T16：版本、账号、权限、测试证据；缺失记外部前件未就绪 |
| P8-V03 | 原生 SkyWalking/Jaeger SDK 认证与身份映射 | T07：协议兼容测试；不能把 OTLP Header 套用所有 SDK |
| P8-V04 | 控制面端点可达、证书和 K8s 权限 | T09/T10：逐组件状态；托管环境不可访问要明确不可用 |
| P8-V05 | 每个 Grafana 模板的确切版本、字段和规则依赖 | T12/T13/T14：锁定 ID/revision/source hash 和能力映射 |
| P8-V06 | 多目标身份、日志选择器重叠、重装/迟到/转发来源 | T05/T06/T08：真实混批、故障和跨权限测试 |
| P8-V07 | 查询成员规模、指标基数、磁盘队列和接收配额 | T03/T05/T14/T16：在编码前定预算，实测后冻结验收阈值 |

当前不需要为继续编制计划重新确认已认可的产品范围。技术任务发现必须改变上述产品语义时，给出具体影响并同步决策记录；普通实现选型在既有边界内自主推进。
