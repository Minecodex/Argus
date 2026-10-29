# Collector 内置组件准入与候选路线

> 执行调整（2026-10-19）：Windows 实机测试由用户暂缓，保留实现/构建，IIS、事件日志及主机正式接入等未验证。它们仍有原生候选组件，不并入“无合格组件”的暂缓目录。Journald 可调用系统自带 journalctl；仍禁止独立 exporter/外部 JMX 采集服务。详见[Windows 状态](./28-windows-testing-deferral.md)。

日期：2026-10-18。最新用户约束：只采用可以内置到 OpenTelemetry Collector 的现成组件；不采用独立 exporter/采集服务，没有合适组件的资源类型或模式暂缓。本页替代初版允许独立辅助 exporter 的建议；最新[专用采集复查](./13-direct-receiver-review.md)进一步排除用通用Prometheus抓取补资源支持。

## 准入要求

1. 使用官方或有足够维护/使用依据的第三方 Collector 组件，经 OCB 编入 Argus Collector。只有 Go 包或高 Star 不够，必须实现对应 Collector 组件接口，并核查运行依赖。
2. 通过受控配置创建实例并在 pipeline 中启用；未编入组件须升级发行版，不在线下载任意插件。
3. 不部署 redis_exporter、mysqld_exporter、blackbox_exporter 等独立程序，也不开发它们的常驻进程管理器。不以“同安装包/同 Pod”冒充内置。
4. 需要额外 JRE/JAR 采集进程的 JMX 路线暂缓；把外部采集进程包在 Receiver 中也不因此满足约束。依赖系统命令的组件须单列核验，不能直接标为纯进程内实现。
5. 资源指标入口要求对应的现成专用Receiver/scraper。通用prometheusreceiver、sqlqueryreceiver加自编SQL、SDK/OTLP主动接入都不作为对应资源已支持的依据；无合格插件即暂缓。主机/K8s自身组件与应用SDK接入按各自合同保留，不删除现有通用组件。
6. 无符合要求的内置方案时，记 deferred_no_eligible_embedded_component，保留检索证据、缺少能力及原因；用户已授权暂缓。存在合格组件但缺测试环境或测试失败，仍是未验收，不能借此豁免。

[OCB 官方说明](https://opentelemetry.io/docs/collector/extend/ocb/)解释组件组装方式；[运行配置](https://opentelemetry.io/docs/collector/configuration/)区分配置定义与 pipeline 启用。项目[现有发行清单](../../deploy/otelcol/builder-linux-amd64.yaml)锁定 v0.133.0，并已包含 prometheusreceiver；本轮没有修改二进制、发行清单或部署。

## 上游盘点范围与限制

本轮读取 [Contrib Receiver 目录](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/117c83022ff71c8dd75acc0ee6804d1c0b2dc17b/receiver)及相关 README。main 核查提交为 117c83022ff71c8dd75acc0ee6804d1c0b2dc17b。下表为路线筛选，不是编译或真实采集通过清单；主干组件不等于现有 v0.133.0 包含它。T01/T05 仍需选择正式版本、检查各组件成熟度/平台/子进程、构建并逐项实测。

| 候选 ID | 资源/接入 | 内置路线候选 | 当前结论与边界 |
| --- | --- | --- | --- |
| P8-I01 | MySQL | mysqlreceiver | 原生候选；MariaDB、复制/集群能力逐模式验证 |
| P8-I02 | PostgreSQL | postgresqlreceiver | 原生候选；不由复制指标推断全部高可用拓扑 |
| P8-I03 | SQL Server | sqlserverreceiver | 原生候选；OS、连接和 Always On 分开验证 |
| P8-I04 | Oracle | oracledbreceiver | 原生候选；版本/驱动和 RAC 指标范围核验 |
| P8-I05 | MongoDB | mongodbreceiver | 原生候选；副本集/分片及成员发现按实际版本验证 |
| P8-I06 | ClickHouse | **暂缓，不采集该资源指标** | 未找到合格的ClickHouse专用指标Receiver；存储exporter和通用SQL不作为替代；见[复查记录](./13-direct-receiver-review.md) |
| P8-I07 | TiDB | **暂缓，不采集该资源指标** | 未找到合格的TiDB专用指标Receiver，不能用MySQL协议兼容代替TiDB/TiKV/PD采集；见[复查记录](./13-direct-receiver-review.md) |
| P8-I08 | OceanBase | **暂缓，不采集该资源指标** | 未找到合格的OceanBase专用指标Receiver；OBAgent、独立exporter及通用抓取不纳入；见[复查记录](./13-direct-receiver-review.md) |
| P8-I09 | Redis | redisreceiver | 单实例 INFO 候选；Cluster/Sentinel 自动发现及整体视图未确认，不用 redis_exporter 补齐 |
| P8-I10 | Memcached | memcachedreceiver | 原生候选 |
| P8-I11 | Kafka | kafkametricsreceiver | Broker/Topic/消费组候选；JVM/JMX 外部采集不纳入 |
| P8-I12 | RabbitMQ | rabbitmqreceiver | 原生候选，队列/vhost/节点范围按版本核验 |
| P8-I13 | RocketMQ | **暂缓，不采集该资源指标** | 未找到合格的RocketMQ直接指标采集Receiver；产品OTLP/Prometheus输出不等于专用采集；见[复查记录](./13-direct-receiver-review.md) |
| P8-I14 | Pulsar | **暂缓，不采集该资源指标** | pulsarreceiver是遥测消息传输，不采集Broker自身指标；见[复查记录](./13-direct-receiver-review.md) |
| P8-I15 | NATS | **暂缓，不采集该资源指标** | 官方组件是遥测传输，已查Broker组件为实验项目且社区依据不足；见[复查记录](./13-direct-receiver-review.md) |
| P8-I16 | ZooKeeper | zookeeperreceiver | 原生候选；命令/访问前件核验，不走 JMX helper |
| P8-I17 | Nginx | nginxreceiver | 原生候选；错误率/时延不能从基础状态计数推算 |
| P8-I18 | Apache | apachereceiver | 原生候选，核验 server-status 权限 |
| P8-I19 | IIS | iisreceiver | Windows 性能计数器原生候选，不使用 windows_exporter |
| P8-I20 | HAProxy | haproxyreceiver | 原生候选，核验 stats/目标版本 |
| P8-I21 | Envoy | **暂缓，不采集该资源指标** | envoyalsreceiver是日志接入；AppMesh StatsD集成与通用抓取不满足本轮直接资源采集要求；见[复查记录](./13-direct-receiver-review.md) |
| P8-I22 | Elasticsearch | elasticsearchreceiver | 原生集群/节点/索引候选 |
| P8-I23 | JVM | **暂缓，不采集该资源指标** | 直连JMX需要外部采集进程，暂缓资源入口；应用SDK接入单独保留；见[复查记录](./13-direct-receiver-review.md) |
| P8-I24 | Tomcat | **暂缓，不采集该资源指标** | 外部JMX路线不符合约束，暂缓资源入口；文件日志独立保留；见[复查记录](./13-direct-receiver-review.md) |
| P8-I25 | Spring Boot | **暂缓，不采集该资源指标** | Micrometer/OTLP是应用主动上报，未找到合格的专用资源采集Receiver；见[复查记录](./13-direct-receiver-review.md) |
| P8-I26 | MinIO | **暂缓，不采集该资源指标** | 未找到合格的MinIO专用指标Receiver，通用抓取及向S3写数据的exporter不能替代；见[复查记录](./13-direct-receiver-review.md) |
| P8-I27 | HTTP/HTTPS | httpcheckreceiver | 原生探测候选 |
| P8-I28 | TCP | tcpcheckreceiver | 原生探测候选 |
| P8-I29 | TLS | tlscheckreceiver | 原生证书探测候选 |
| P8-I30 | DNS | dnscheckreceiver | 当前 main 标记 development，正式版本/构建/能力仍待核验 |
| P8-I31 | ICMP | icmpcheckreceiver | 当前 main 标记 development，正式版本/系统权限/运行依赖待核验 |
| P8-I32 | OTLP | 核心 otlpreceiver | 已编入当前包；新应用 Key/服务发现仍待实施 |
| P8-I33 | SkyWalking | skywalkingreceiver | 用户确认新增暂缓；上游失败传播和原始数据打印缺陷，见 15-sdk-admission-findings.md |
| P8-I34 | Jaeger | jaegerreceiver | 已编入当前包；传输/认证与身份逐组合验证 |
| P8-I35 | 文件日志 | filelogreceiver | 原生候选，文件/权限/规则及归属独立验证 |
| P8-I36 | K8s 日志 | filelogreceiver＋k8sattributesprocessor 等 | 原生组件组合候选；Owner/容器和应用映射仍需平台实现 |
| P8-I37 | SDK 日志 | otlpreceiver | 复用应用接入与服务发现 |
| P8-I38 | Journald | journaldreceiver，存在 journalctl 依赖 | 运行依赖待复核，暂不计纯进程内可用；能读取现有系统文本日志时可用 filelog，但不宣称等价覆盖 journal |
| P8-I39 | Windows Event Log | windowseventlogreceiver | Windows 原生候选，通道/权限/Bookmark 验证 |

## 已查明的容易误判项

- SQL Server（2026-10-29 B06c 实测）：锁定的 sqlserverreceiver v0.159.0 在 datasource SQL 查询路径把 Batch Requests/sec、Lock Waits/sec、SQL Compilations/sec 的累计 DMV 值直接输出为 rate 命名的 gauge；Windows 本地性能计数器是另一条路径，不能混用口径。Argus 正常配置统一使用 datasource，因此 SQL Server 默认模板 v2 对这三个指标使用 `rate(...[1m])`，页面生存时间只取实例级 Buffer Manager，原始指标保留。真实计数差分、告警恢复和界面验证见 [B06c 运行包](../plan9/runtime/B06c/runbook.md)。仅收到指标并不证明数值语义正确。
- [Redis Receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/117c83022ff71c8dd75acc0ee6804d1c0b2dc17b/receiver/redisreceiver/README.md)定位单实例 INFO。分别采集若干节点不等于自动发现拓扑或完整故障切换监控；单实例可保留，集群模式缺能力单列。
- [NATS Receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/117c83022ff71c8dd75acc0ee6804d1c0b2dc17b/receiver/natsreceiver/README.md)与 [Pulsar Receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/117c83022ff71c8dd75acc0ee6804d1c0b2dc17b/receiver/pulsarreceiver/README.md)接收消息载荷中的遥测。第三方 [natsbrokerreceiver](https://github.com/m3nowak/otelcol-nats)确实是 Collector Broker 指标组件，但 2026-10-18 GitHub API 核查为 0 Star，README 标实验，未满足用户要求的社区采纳依据。本轮暂缓 NATS 自身监控，不声称全球不存在实现。
- [JMX Receiver v0.133.0](https://raw.githubusercontent.com/open-telemetry/opentelemetry-collector-contrib/v0.133.0/receiver/jmxreceiver/README.md)需要 JRE 和 JAR，内置 Go Receiver 还会调用外部采集进程，因此不能列为本方案的纯内置采集。
- [Journald Receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/117c83022ff71c8dd75acc0ee6804d1c0b2dc17b/receiver/journaldreceiver/README.md)使用 journalctl。系统工具与独立 exporter 服务不同，但该路径也不能直接称为单二进制、无额外进程；保持运行依赖待核验状态，不从产品名称推断满足约束。
- [DNS](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/117c83022ff71c8dd75acc0ee6804d1c0b2dc17b/receiver/dnscheckreceiver/README.md)、[ICMP](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/117c83022ff71c8dd75acc0ee6804d1c0b2dc17b/receiver/icmpcheckreceiver/README.md)有原生代码候选，不能因为此前 Grafana 表参考 blackbox_exporter 就默认需要外部探测服务；development 状态仍需选型验证。

## 计划及验收变化

取消辅助 exporter/helper 的安装、升级和常驻进程治理任务。A76 改为内置发行版与运行依赖验收，验证构建清单、组件注册、配置验证、多实例独立启停与实际进程/连接。不新增独立采集服务，不把 OCB 能编译等同能力完整。

Grafana 表保留原始模板依赖用于研究；采用时按内置组件真实指标重新映射。无法等价得到的面板应删减或明确缺能力，不为复刻模板而引入其原 exporter。最终缺项表区分：没有合格内置组件、只有外部进程路线、组件成熟度/社区依据不足、现有内置组件缺少某拓扑能力；缺测试环境/测试失败另列。
