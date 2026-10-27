# 集成目录与逐项交付合同

状态：新增能力待实施/待验证。目录保留 39 个候选：第一批 13 项、第二批 18 项、公共接入/日志 8 项；另有主机与 K8s 内置能力。两批都必须审查，按开源能力冻结支持组合，全部完成后统一发布。

选型按[用户确认规则](./11-grilling-decisions-and-upstream.md)：资源指标仅采用能编入Collector并直接采集该资源的官方/成熟专用组件；不使用独立exporter或外部JMX采集进程，不自研/fork补齐。无合格内置方案的类型/模式可记 deferred_no_eligible_embedded_component，保留检索证据并最终列明。有方案但缺环境、未开发或验收失败不能豁免；本轮路线与暂缓项见[内置支持表](./12-collector-native-support.md)；当前11个资源入口及 SkyWalking 原生协议暂缓，27个其余候选待验证，详见[专用Receiver复查](./13-direct-receiver-review.md)。通用Prometheus抓取不算资源采集插件。

## 统一交付项

每行必须提供版本化配置、目标身份、认证方式、Collector 能力/权限、测试/诊断、独立启停/版本/生命周期、实际数据与字段目录、默认仪表盘、适用指标告警建议和运行文档。认证在表单直接输入并自动托管；环境跟随应用/服务，普通新建自动建立目标。

每个支持的版本/拓扑组合执行 CONFIG/DATA/VIEW/AUTH/LIFECYCLE，定义见[验收矩阵](./09-acceptance-matrix.md)。单实例、复制、集群、分片分别声明；慢 SQL/日志等按上游能力和实际权限验证，不因一种信号成功就承诺全部。SDK 创建不要求服务，入库前从受信应用接入和有效服务名解析归属。

## 第一批服务与探测

| ID | 入口 | 本期状态 | 信号与目标内容 | 采集前件/适配边界 | Grafana 参考 | 任务 |
| --- | --- | --- | --- | --- | --- | --- |
| P8-I01 | MySQL | 内置候选／待验证 | metrics；连接、事务、InnoDB、缓存 | 原生只读数据库采集；数据库版本/认证插件、MySQL/MariaDB 分别验证 | [14969](./grafana-references.md) | P8-T12 |
| P8-I02 | PostgreSQL | 内置候选／待验证 | metrics；连接、事务、锁、复制 | 只读统计视图；数据库范围、扩展和复制指标逐项声明 | [9628](./grafana-references.md) | P8-T12 |
| P8-I03 | SQL Server | 内置候选／待验证 | metrics；会话、事务、缓存、锁等待 | 直连查询与 Windows 性能计数器能力分开；实例名、权限、TLS | [1883](./grafana-references.md) | P8-T12 |
| P8-I04 | Oracle | 内置候选／待验证 | metrics；会话、表空间、资源限制 | Oracle Service/实例、只读动态视图和表空间权限；RAC 单列 | [13555](./grafana-references.md) | P8-T12 |
| P8-I05 | MongoDB | 内置候选／待验证 | metrics；连接、操作、缓存、存储 | 节点/副本集/分片分别声明；监控账号与受控自动发现 | [2583](./grafana-references.md) | P8-T12 |
| P8-I09 | Redis | 内置候选／待验证 | metrics；内存、命中率、连接、复制 | INFO 等批准命令；ACL/TLS；Cluster/Sentinel 单独能力 | [763](./grafana-references.md) | P8-T12 |
| P8-I11 | Kafka | 内置候选／待验证 | metrics；Broker、Topic、消费组积压 | Broker/SASL/TLS、Topic/消费组权限；Broker JVM 来源另列 | [7589](./grafana-references.md) | P8-T12 |
| P8-I12 | RabbitMQ | 内置候选／待验证 | metrics；队列、投递、确认、节点 | 管理/metrics 接口和 vhost 范围；节点 Receiver 不等于队列全量能力 | [10991](./grafana-references.md) | P8-T12 |
| P8-I17 | Nginx | 内置候选／待验证 | metrics；连接、请求、访问与错误日志 | 状态接口＋可选日志；状态码/时延不能从基础连接计数推算 | [12708](./grafana-references.md) | P8-T12 |
| P8-I22 | Elasticsearch | 内置候选／待验证 | metrics；集群、分片、索引、查询 | 集群/节点/索引统计端点；scope/权限/版本和查询成本 | [2322](./grafana-references.md) | P8-T12 |
| P8-I27 | HTTP / HTTPS | 内置候选／待验证 | metrics；可用性、响应时间、状态码 | 允许 URL/认证/方法/预期结果/超时；重定向与目标边界 | [7587](./grafana-references.md) | P8-T12 |
| P8-I28 | TCP | 内置候选／待验证 | metrics；端口连通性、连接时间 | 固定目标/端口/超时/周期；连接成功不代表应用健康 | [7587](./grafana-references.md) | P8-T12 |
| P8-I29 | TLS 证书 / Certificate | 内置候选／待验证 | metrics；证书有效性与到期时间 | 固定目标/端口/SNI/信任链；有效性/到期/校验错误 | [7587](./grafana-references.md) | P8-T12 |

## 第二批服务与探测

| ID | 入口 | 本期状态 | 信号与目标内容 | 采集前件/适配边界 | Grafana 参考 | 任务 |
| --- | --- | --- | --- | --- | --- | --- |
| P8-I06 | ClickHouse | **暂缓／不开放新建** | metrics；查询、写入、合并、复制 | 未找到合格的ClickHouse专用指标Receiver；存储exporter和通用SQL不作为替代 | [Official integration](./grafana-references.md) | P8-T13 |
| P8-I07 | TiDB | **暂缓／不开放新建** | metrics；TiDB、TiKV、PD、事务 | 未找到合格的TiDB专用指标Receiver，不能用MySQL协议兼容代替TiDB/TiKV/PD采集 | [12712](./grafana-references.md) | P8-T13 |
| P8-I08 | OceanBase | **暂缓／不开放新建** | metrics；租户、OBServer、SQL | 未找到合格的OceanBase专用指标Receiver；OBAgent、独立exporter及通用抓取不纳入 | [15215](./grafana-references.md) | P8-T13 |
| P8-I10 | Memcached | 内置候选／待验证 | metrics；内存、命中率、淘汰 | 批准协议/Receiver；实例、slab、命中/淘汰指标的支持范围 | [11527](./grafana-references.md) | P8-T13 |
| P8-I13 | RocketMQ | **暂缓／不开放新建** | metrics；Broker、Topic、消费延迟 | 未找到合格的RocketMQ直接指标采集Receiver；产品OTLP/Prometheus输出不等于专用采集 | [10477](./grafana-references.md) | P8-T13 |
| P8-I14 | Pulsar | **暂缓／不开放新建** | metrics；租户、Topic、订阅积压 | pulsarreceiver是遥测消息传输，不采集Broker自身指标 | [Official dashboard collection](./grafana-references.md) | P8-T13 |
| P8-I15 | NATS | **暂缓／不开放新建** | metrics；连接、消息、JetStream | 官方组件是遥测传输，已查Broker组件为实验项目且社区依据不足 | [14725](./grafana-references.md) | P8-T13 |
| P8-I16 | ZooKeeper | 内置候选／待验证 | metrics；会话、延迟、节点状态 | zookeeperreceiver原生候选；成员/会话与协议权限，不走JMX helper | [10465](./grafana-references.md) | P8-T13 |
| P8-I18 | Apache HTTP Server | 内置候选／待验证 | metrics；请求、Worker、连接 | server-status 权限与扩展统计；Worker/请求口径 | [3894](./grafana-references.md) | P8-T13 |
| P8-I19 | IIS | 内置候选／待验证 | metrics；站点、请求、应用池 | Windows Collector 与 IIS 性能计数器；站点/应用池身份 | [14532](./grafana-references.md) | P8-T13 |
| P8-I20 | HAProxy | 内置候选／待验证 | metrics；后端、会话、错误 | 受控 stats/metrics 端点；frontend/backend/server 维度 | [12693](./grafana-references.md) | P8-T13 |
| P8-I21 | Envoy | **暂缓／不开放新建** | metrics；上下游、请求、连接池 | envoyalsreceiver是日志接入；AppMesh StatsD集成与通用抓取不满足本轮直接资源采集要求 | [23239](./grafana-references.md) | P8-T13 |
| P8-I23 | JVM | **暂缓／不开放新建** | metrics；堆、GC、线程、类加载 | 直连JMX需要外部采集进程，暂缓资源入口；应用SDK接入单独保留 | [4701](./grafana-references.md) | P8-T13 |
| P8-I24 | Tomcat | **暂缓／不开放新建** | metrics；线程池、会话、请求 | 外部JMX路线不符合约束，暂缓资源入口；文件日志独立保留 | [8704](./grafana-references.md) | P8-T13 |
| P8-I25 | Spring Boot | **暂缓／不开放新建** | metrics；JVM、HTTP、连接池 | Micrometer/OTLP是应用主动上报，未找到合格的专用资源采集Receiver | [4701](./grafana-references.md) | P8-T13 |
| P8-I26 | MinIO | **暂缓／不开放新建** | metrics；容量、磁盘、S3 请求 | 未找到合格的MinIO专用指标Receiver，通用抓取及向S3写数据的exporter不能替代 | [10946](./grafana-references.md) | P8-T13 |
| P8-I30 | DNS | 内置候选／待验证 | metrics；解析结果、延迟 | 批准 DNS 服务器、域名/类型/期望答案；解析视角与超时 | [7587](./grafana-references.md) | P8-T13 |
| P8-I31 | ICMP | 内置候选／待验证 | metrics；可达性、延迟、丢包 | 执行 Collector 的系统权限与网络可达性；丢包/时延口径 | [7587](./grafana-references.md) | P8-T13 |

## 公共 SDK 接入与日志

| ID | 入口 | 本期状态 | 信号与目标内容 | 采集前件/适配边界 | Grafana 参考 | 任务 |
| --- | --- | --- | --- | --- | --- | --- |
| P8-I32 | OpenTelemetry / OTLP | 内置候选／待验证 | traces/metrics/logs；应用 Trace 与可选 SDK 日志 | 受认证 HTTP/gRPC；默认 Trace，可按 Key 允许信号接 SDK Metrics/Logs | [19419](./grafana-references.md) | P8-T07 |
| P8-I33 | SkyWalking | **新增暂缓／禁止启用** | traces；原生 Trace 接入 | 已复现消费失败误报成功和原始数据打印；用户已确认新增暂缓，保留失败记录，见[核验记录](./15-sdk-admission-findings.md) | [19419](./grafana-references.md) | P8-T07 |
| P8-I34 | Jaeger | 内置候选／待验证 | traces；原生 Trace 接入 | 原生 SDK/Receiver 与认证适配；不承诺远程采样/完整后端 | [19419](./grafana-references.md) | P8-T07 |
| P8-I35 | 文件日志 / File logs | 内置候选／待验证 | logs；业务文件日志、系统文件日志 | 本机允许路径/编码/多行/轮转/checkpoint；业务日志必绑服务 | [13639](./grafana-references.md) | P8-T08 |
| P8-I36 | Kubernetes 日志 / Logs | 内置候选／待验证 | logs；工作负载、Pod、容器日志 | DaemonSet/Owner UID/container；规则交集与业务映射 | [13639](./grafana-references.md) | P8-T08 |
| P8-I37 | SDK 日志 / Logs | 内置候选／待验证 | logs；SDK 主动上报业务日志 | SDK Logs 身份/Key/环境；保留 Trace/Span 上下文 | [13639](./grafana-references.md) | P8-T08 |
| P8-I38 | Journald | 内置候选／待验证 | logs；Linux 系统与审计日志 | 依赖journalctl，运行依赖待核验，不计为纯进程内可用 | [13639](./grafana-references.md) | P8-T08 |
| P8-I39 | Windows Event Log | 内置候选／待验证 | logs；Windows 系统事件 | Windows 通道/权限/Bookmark；默认系统归属 | [13639](./grafana-references.md) | P8-T08 |

## 主机与 Kubernetes 内置能力

主机固定能力包括 CPU/内存/磁盘/文件系统/网络、进程/系统服务和 Collector 自监控；系统/文件日志由上述公共日志条目承载。平台不支持或尚未验证的 scraper/指标明确不可用，不以模板中有图表就显示已经开启。

| ID | 主机内置能力 | 交付边界 |
| --- | --- | --- |
| P8-H01 | 主机基础指标 | 对照系统真实观测，声明 scraper/指标/平台支持 |
| P8-H02 | 进程与系统服务 | 受控选择范围和目标身份，区分权限不足/未观测 |
| P8-H03 | Collector 自身指标 | 详细自身指标可开关，管理连接/配置健康始终可见 |
| P8-H04 | 系统日志入口 | 复用满足内置约束的系统日志来源及门禁；Journald工具依赖未核验不计可用 |

Kubernetes P8-B01～B08 的范围、前件和对应默认视图见 [主机与 Kubernetes](./05-host-and-kubernetes.md)。组件开关是内置实例的简单操作；业务日志、外部数据库和 SDK 接入仍是可多实例的配置。

## 实施前必须锁定的支持矩阵

每个集成维护以下记录，不能只有一个 `supported=true`：

- 目标产品/发行版/版本范围，单实例或具体集群拓扑。
- 执行 Collector 的 OS/架构、发行版、精确Receiver版本、OCB组件注册、制品哈希与运行依赖核查。
- collection_route、upstream_component、Star 数与核查时间、维护状态、许可证、runtime_model/external_process_required、tested_topology、成员发现与故障切换覆盖。
- 选型状态 pending/selected/deferred_no_eligible_embedded_component、支持组合、检索证据及缺项理由；与验证状态分列。
- 信号、默认/可选指标与日志、字段/单位/语义版本、实际只读权限和认证方式。
- 配置 Schema、身份解析和发现范围、Claim/端口/路径冲突、采集间隔、基数和容量上限。
- 模板 ID/版本/源哈希、变量/指标/Recording Rules 对应关系、实际适用告警。
- 五类门禁结果、真实输入/目标、未覆盖/不可用原因和更新日期。

初始矩阵中的版本为未锁定、门禁为未运行。T01/T05 选取可获得且能真实验证的版本；现有 0.133.0 是源码基线，不强制未来所有集成继续用这一版本，也不直接把上游 main 能力视为 Argus 支持。

## 默认仪表盘内容边界

| 家族 | 默认视图 | 必须区分 |
| --- | --- | --- |
| 数据库 | 实例健康、连接/事务/查询、缓存、存储、适用复制 | 实例/数据库范围；基础指标与需要额外来源的慢查询/日志 |
| 缓存 | 内存、命中率、连接、命令/淘汰 | 单实例与集群拓扑；命中率分母与无样本 |
| 消息/协调 | 节点、Topic/Queue、消费者/会话、积压与适用时延 | 生产/投递/确认/消费等口径；不同组件来源 |
| Web/代理 | 请求/连接、上下游状态、适用错误/时延 | 基础状态接口与日志派生值；缺 Histogram 不伪造 p95 |
| 运行时 | 堆/GC/线程、容器/连接池/请求 | JMX、Micrometer、SDK 的字段与语义差异 |
| 存储 | 容量、节点/磁盘、请求/错误 | 对象存储 metrics 版本、账号与租户/桶范围 |
| 探测 | 可用性、响应/连接时间、证书/解析等 | 探测位置、目标状态、执行失败；不同协议不套同一图 |
| 应用/Trace/日志 | 服务/实例/接口/样本 RED、依赖、慢错误 Trace、日志探索 | 已接收样本、业务归属、来源、日志精确关联与时间窗口 |
| 主机/K8s | 系统/容器使用、容量/requests/limits、工作负载及组件 | Host/Node 去重、授权根与对象维度、控制面不适用过滤 |

[Grafana 参考表](./grafana-references.md)仅是模板研究来源。每项实际交付必须同时验证采集字段、查询结果和 UI；无法映射的面板显示能力缺口并补齐，不能保留好看的空图冒充完成。

## 专用环境与容量

Oracle、SQL Server、IIS/Windows、TiDB/OceanBase/Pulsar/MinIO 等分别准备实际目标和受控测试数据。能在临时 Namespace 运行的组件分组装载，较重组件按容量分批；外部/Windows 目标独立登记所有权和清理范围。缺环境仅说明前件未就绪，不能把对应条目从必需范围悄悄删去。

产品只展示已通过矩阵的组合为可用；未验证组合是 planned/validation_pending/unsupported_version。Plan8 完成要求 39 候选审查闭合、支持组合真实验收、缺项完整披露；不以删目录或自研空壳完成统计。
