# 专用资源采集插件复查与暂缓清单

日期：2026-10-18。用户最新要求：不能仅因为 prometheusreceiver 可以接收/抓取指标，就把对应资源列为可监控；应检索能内置 Collector、直接采集该资源运行状态的现成插件，找不到合格方案时暂不采集并明确标记。本页替代 12 中曾允许用产品原生 metrics 端点补充资源目录的规则。

## 本轮结论

此前依赖通用抓取的 8 项资源监控全部改为**暂缓，不开放新建采集和配套默认仪表盘的可用声明**：ClickHouse、TiDB、OceanBase、RocketMQ、Pulsar、Envoy、Spring Boot、MinIO。本轮未找到符合当前约束且可直接采用的成熟专用 Collector 指标 Receiver；这不代表这些产品没有指标、技术上无法监控或全球不存在相关实现。

此前 NATS 自身监控、JVM/Tomcat 直连 JMX 监控继续暂缓。合计 11 个资源监控入口保留在候选/缺项目录，明确原因、日期和证据，不从需求历史中删除。后续用户确认新增 SkyWalking 原生协议暂缓；27 个其余候选仍待正式选型、构建及真实采集验收，不能据此标为全部支持。

应用 SDK 的 OTLP/Trace/日志接入仍按原契约实施，不把业务主动上报能力冒充 Spring Boot、JVM 或消息中间件的独立资源监控。主机/K8s 已明确的自身组件指标另按其原生组件、端点和权限合同验证。本轮不删除现有 Collector 的 prometheusreceiver，也不改变现有部署。

## 准入判定

- 必须具有对应资源运行指标的现成 Collector Receiver/scraper 实现，能编入受控发行版，且满足此前的成熟度、维护、版本和运行依赖要求。
- 通用 prometheusreceiver、SDK/OTLP 入口、消息传输 Receiver、存储 exporter 都不能作为该资源监控已获支持的依据。
- 通用 sqlqueryreceiver 加自编 SQL、借 MySQL 协议兼容去代替 TiDB/OceanBase 完整监控、脚本或自研 Receiver，本轮均不作为补齐路线。
- 某产品本身就有 Prometheus/OpenTelemetry 指标，并不一定需要独立 exporter。比如 Envoy 原生管理接口提供 Prometheus 格式输出；仍按用户本轮的**专用采集插件**要求暂缓该资源，而不是写成“所有 Prometheus 指标都依赖外置 exporter”。
- 暂缓仅针对对应资源指标入口；已定义的文件日志、应用 SDK、Host/K8s、自监控等能力按各自范围实施，不因产品名称相同而一并删除。

## 逐项记录

| ID | 资源 | 检索与识别结果 | 当前标记 |
| --- | --- | --- | --- |
| P8-I06 | ClickHouse | 官方目录未列专用 ClickHouse 指标 Receiver；clickhouseexporter 向 ClickHouse 写遥测。sqlqueryreceiver 支持数据库查询，但需另写 SQL/指标规则，不是该产品的现成采集插件 | 暂缓：未找到符合约束的专用内置组件 |
| P8-I07 | TiDB | 官方目录及本轮 GitHub/网页检索未找到可直接采用的专用 Receiver；不能以 mysqlreceiver 代替 TiDB/TiKV/PD 指标合同 | 暂缓：未找到符合约束的专用内置组件 |
| P8-I08 | OceanBase | 本轮未找到可直接采用的专用 Receiver；OBAgent/独立 exporter 和通用抓取不作为替代 | 暂缓：未找到符合约束的专用内置组件 |
| P8-I13 | RocketMQ | 官方资料涉及产品生成指标及 OTLP/Prometheus 输出，未核实到可采纳的独立资源采集 Receiver；主动输出不等于 Collector 直接采集插件 | 暂缓：仅查到输出/通用接入等路线 |
| P8-I14 | Pulsar | pulsarreceiver 消费 Topic 中的遥测，不采集 Broker 自身运行状态；产品自身 OTel 指标和通用抓取不能替代专用 Receiver | 暂缓：同名组件职责不符 |
| P8-I21 | Envoy | envoyalsreceiver 接收访问日志；原生 /stats/prometheus 是产品输出。还查到 Splunk AppMesh 集成，但需启用 StatsD sink，属于特定场景推送接入，不作为通用 Envoy 直接指标采集插件 | 暂缓：未找到符合当前直接采集要求的组件 |
| P8-I25 | Spring Boot | 官方方案以应用端 Micrometer/OTel/OTLP 为主；未找到可直接采用的专用 Collector 资源采集 Receiver | 暂缓资源监控；应用 SDK 接入保留 |
| P8-I26 | MinIO | 本轮未找到可采纳的专用 Receiver；Prometheus 端点或向 S3/MinIO 写数据的 exporter 不等于 MinIO 自身采集插件 | 暂缓：未找到符合约束的专用内置组件 |
| P8-I15 | NATS | 沿用 12 中证据：官方 natsreceiver 是遥测传输，找到的 Broker Receiver 为实验项目且 0 Star | 继续暂缓：未满足社区采纳要求 |
| P8-I23 | JVM | 沿用 12：直连 JMX 路线要求外部 JRE/JAR 采集进程 | 继续暂缓资源入口；SDK 接入保留 |
| P8-I24 | Tomcat | 沿用 12：依赖外部 JMX 采集路线，未找到符合本轮约束的专用直接采集方案 | 继续暂缓资源入口；文件日志能力独立保留 |

## 检索范围与证据

核查官方 Contrib Receiver 目录、按产品名组合的网页搜索及 GitHub Repository Search。目录提交为 `117c83022ff71c8dd75acc0ee6804d1c0b2dc17b`。网页检索包括产品名＋OpenTelemetry Collector＋metrics receiver/plugin，以及 clickhousereceiver、tidbreceiver、oceanbasereceiver、rocketmqreceiver 等名称；GitHub 查询为 `opentelemetry <product> receiver`。GitHub 仓库搜索只索引元数据，不等同代码全文检索；不能把零结果直接证明为不存在。

GitHub 8 次查询中，ClickHouse 命中一个 0 Star 的 Rust 遥测后端 `microph1/sonde`，属于接收遥测并存入 ClickHouse，职责不符；其他七个查询无仓库命中。网页结果及上游文档用于交叉核对，额外记录了 Envoy/AppMesh 与通用 SQL 的替代路线及不采纳原因。

主要一手资料：

- [官方 Receiver 目录](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/117c83022ff71c8dd75acc0ee6804d1c0b2dc17b/receiver)。
- [官方 scraping receivers 分类](https://github.com/open-telemetry/opentelemetry-collector/blob/main/docs/scraping-receivers.md)：区分通用 Prometheus/SQL Query 与带预定义资源指标的采集 Receiver。
- [ClickHouse exporter](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/exporter/clickhouseexporter/README.md)、[SQL Query Receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/receiver/sqlqueryreceiver/README.md)。
- [Pulsar Receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/117c83022ff71c8dd75acc0ee6804d1c0b2dc17b/receiver/pulsarreceiver/README.md)、[Pulsar OTel 指标](https://pulsar.apache.org/docs/4.2.x/reference-metrics-opentelemetry/)。
- [Envoy ALS Receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/117c83022ff71c8dd75acc0ee6804d1c0b2dc17b/receiver/envoyalsreceiver/README.md)、[Envoy 原生管理接口](https://www.envoyproxy.io/docs/envoy/latest/operations/admin)、[Splunk AppMesh 集成](https://help.splunk.com/en/splunk-observability-cloud/manage-data/available-data-sources/supported-integrations-in-splunk-observability-cloud/applications-networks/aws-appmesh-envoy-proxy)。
- [RocketMQ 可观测性方案](https://github.com/apache/rocketmq/wiki/RIP-46-Observability-improvement-for-RocketMQ)、[Spring 官方 OTel 接入说明](https://spring.io/blog/2025/11/18/opentelemetry-with-spring-boot)。

机器可读检索记录：[direct-receiver-search-evidence.json](../../artifacts/plan8-documentation-20261018/direct-receiver-search-evidence.json)。原始搜索结果不属于生产验收，本轮没有编译、启动 Collector 或变更目标服务。

## 目录、状态与验收

07 资源目录和 12 路线表均显示“暂缓”与原因；progress.json 将上述 11 项记为 `deferred_no_eligible_embedded_component`，required=false，保留未运行的采集门禁，绝不转成 passed。后续 SkyWalking 原生协议新增暂缓，27 个其余候选为 pending，支持矩阵尚未锁定。

产品实现时，暂缓资源在新建监控入口不可选择，或仅在带明确原因的“暂不支持”目录中展示；不生成该资源的空白默认仪表盘。API、门户、Agent、能力目录与参考仪表盘表使用同一准入状态。旧 Demo 未同步实现此筛选，不能用其 39 个可点击表单声明当前支持。
