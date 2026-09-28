# Collector 插件、厂商视图与两层过滤

调研日期：2026-09-24。依据 Collector 官方文档、Contrib 组件说明和 Argus 当前源码。上游 main 用于了解能力；Argus 构建清单仍锁定 0.133.0，不能将上游能力直接记为 Argus 已支持。

## 1. 用户最新方向与状态

- Trace 仪表盘按启用的采集插件/厂商组织，提供与该来源能力相符的展示，不将不同厂商强行混成一种通用分析界面。
- Logs 后续按采集插件接入，保留此前已确认的 KQL 方向。用户提到的“fotelp”项目名称待澄清，不擅自等同于 Fluent Bit、Fluentd 或 filelog。
- Metrics 通过 Collector 的插件采集，配置与图表能力围绕所选采集来源组织。
- 这修正了此前“统一 OTLP 数据后共用 APM 展示”容易产生的歧义。共用传输、存储、授权和基础组件不要求使用相同的厂商界面，也不能抹掉来源。
- Q35 已确认：基于接入数据对齐厂商仪表盘的功能与交互，同时保持 Argus 的统一样式和权限体系。按接收、存储、查询和展示的实际支持矩阵验收，不能仅凭使用 OTLP 推定全部专有能力已经具备。
- Q36 已确认：不同统计图可以分别绑定来源；Dashboard 顶部放通用过滤，统计图/Trace 分析视图内提供对应厂商的局部过滤。这里将用户所说的第二层理解为每张统计图，不新建另一层 Dashboard 对象。
- 本文保留 2026-09-24 的调研和设计基线。2026-09-28 已取得来源绑定、两层过滤、来源生命周期及首期常用 APM 的正式实现与真实验收证据，见 [验收结论](./completion-review-20260928.md)。采样口径已确认展示已接收样本、不推算全量；未明确的“fotelp”扩展不属于首期验收，不能据此宣称支持全部 Collector 插件或厂商专有能力。

## 2. Contrib 的组件能提供什么

用户所说的插件，对应 OpenTelemetry Collector 的组件。OTLP 是数据协议，Collector 可以通过 Receiver 接收 OTLP，也能接收其他原生协议，再转换成内部的 OTel 数据模型并导出。

| 组件类别 | 职责 | 与仪表盘的关系 |
| --- | --- | --- |
| Receiver | 接收/拉取数据及协议转换 | 确定输入格式与实际可获取的数据 |
| Processor | 过滤、转换、补充属性等 | 影响落库字段、数据范围与来源保存 |
| Connector | 连接信号管线，可从 Span 生成指标 | 可供服务指标、依赖关系等分析复用 |
| Exporter | 将数据送到后端 | 不等于被接收的数据源，也不提供后端查询能力 |
| Extension | 认证、持久队列存储、健康等辅助能力 | 不直接定义业务图表 |

组件需按支持的信号、稳定性、平台和发行版本选择，不能根据仓库中有同名目录就视为可启用。官方依据：[Contrib 仓库](https://github.com/open-telemetry/opentelemetry-collector-contrib)、[Collector 架构](https://opentelemetry.io/docs/collector/architecture/)、[完整 Receiver 目录](https://opentelemetry.io/docs/collector/components/receiver/)。

Receiver 的职责是输入与转换，不携带对应厂商的完整查询后端或 UI。Collector 扩展组件、Argus 扩展查询能力与厂商视图，需要分别登记和验收。

## 3. 代表性组件与展示方向

下表列与本轮设计直接相关的代表，不承诺一次性支持全部 Contrib 组件。

### Trace

| Receiver | 输入与数据 | 对 Argus 的意义 |
| --- | --- | --- |
| [otlp](https://github.com/open-telemetry/opentelemetry-collector/tree/main/receiver/otlpreceiver) | 标准 OTLP，支持三信号 | 通用接入协议，不能单凭 Receiver 判断原 SDK 或厂商 |
| [skywalking](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.133.0/receiver/skywalkingreceiver/README.md) | SkyWalking 原生 Trace，部分 JVM Metrics | 需提供对应来源视图与支持矩阵；不是 OAP 的完整替代 |
| [jaeger](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/jaegerreceiver) | Jaeger 原生 Trace | 适配接收和数据语义，查询与 UI 仍需实现 |
| [zipkin](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/zipkinreceiver) | Zipkin Trace | 同样是原生接收适配，不附带 Zipkin 查询后端 |
| [awsxray](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/awsxrayreceiver) | AWS X-Ray Trace | 属于可选来源扩展，不自动获得云厂商控制台能力 |

厂商视图可以有自己的检索维度、默认图表、详情布局及能力集，基础瀑布、表格和图形组件仍复用 `@argus/ui`。例如 SkyWalking 的服务/实例/接口组织与 Jaeger 的服务/操作、标签和 Trace 详情不必做成完全一样的交互。

“与厂商支持的一样”必须区分两种交付：

- 参考厂商的核心排查体验，在实际接收到并保留的数据上提供相应能力。
- 完整兼容厂商原生数据语义、Query API 或直接承载原生 UI。此时需要额外兼容层，必要时保留厂商扩展信息或引入其查询后端，不能用 Receiver 的支持状态证明兼容完成。

例如 SkyWalking 转换会映射 Trace/Segment/Span ID、引用和 Span 内日志；其 Span 内日志可成为 OTel Events，并不自动形成独立 Logs。具体映射见 [0.133.0 translator](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.133.0/pkg/translator/skywalking/skywalkingproto_to_traces.go)。Jaeger 自身也能接收 OTLP，同时维持自己的查询与展示模型，证明接入协议和展示产品并非一一对应，见 [Jaeger Features](https://www.jaegertracing.io/docs/2.21/features/)。

需要逐项验收的差异包括：SkyWalking 的多 reference 会进入 links，不能只画 ParentSpanID 树；Jaeger 的 Process 映射成 Resource、Span logs 映射成 events、其他 references 进入带引用类型的 links，不能宣称保留了原始 Batch 结构。见 [Jaeger translator](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.133.0/pkg/translator/jaeger/jaegerproto_to_traces.go)。基础组件需要支持这些关系与属性，不能以“统一”为由丢掉厂商能力。

### Logs

| 来源类别 | 代表 Receiver | 可组织的视图 |
| --- | --- | --- |
| 文件/容器日志 | [filelog](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/filelogreceiver) | 文件或业务日志、解析字段、关联资源 |
| 操作系统日志 | [journald](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/journaldreceiver)、[windowseventlog](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/windowseventlogreceiver)、[syslog](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/syslogreceiver) | 对应系统字段及默认过滤条件 |
| Fluent Forward | [fluentforward](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/fluentforwardreceiver) | Forward 协议接入，需保留来源与字段 |
| OTLP Logs | [otlp](https://github.com/open-telemetry/opentelemetry-collector/tree/main/receiver/otlpreceiver) | 应用或外部 Collector 上报的结构化日志 |
| Loki Push | [loki](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/lokireceiver) | 接收 Loki 写入协议；并不提供 LogQL 执行与 Loki 查询 API |

不同接收方式可以对应不同字段和默认视图，仍使用 Argus KQL 检索。不能把“接收到 Loki Push”推导成“已有 Loki 后端”，也不能把“来自 Fluent 系列”推导成固定的一种日志 Schema。

若用户所指是 Fluent Bit/Fluentd：Fluent Bit 可通过 [OpenTelemetry output](https://docs.fluentbit.io/manual/data-pipeline/outputs/opentelemetry) 输出 OTLP，也可走 Forward；Fluentd 的 [核心 out_forward](https://docs.fluentd.org/output/forward) 是 Forward 协议，OTLP 需要对应插件。应按实际传输选择 Receiver。当前 [fluentforward Receiver 文档](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/receiver/fluentforwardreceiver/README.md) 明确没有 TLS/Forward 握手支持，不能直接等同于发送端支持的 Secure Forward；这是后续接入选型需验证的具体兼容项，不预先决定使用哪种采集器。

### Metrics

| 采集目标 | 代表 Receiver | 展示组织建议 |
| --- | --- | --- |
| 主机 | [hostmetrics](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/hostmetricsreceiver) | CPU、内存、磁盘、网络等模板，按开启的 scraper 和指标支持配置 |
| Kubernetes | [kubeletstats](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/kubeletstatsreceiver)、[k8scluster](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/k8sclusterreceiver) | 节点/容器统计与集群指标；不自动扩展已确定的对象授权/绑定粒度 |
| 数据库与中间件 | [mysql](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/mysqlreceiver)、[postgresql](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/postgresqlreceiver)、[redis](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/redisreceiver)、[nginx](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/nginxreceiver) | 对应连接、缓存、吞吐等语义模板，按实际指标构建 |
| Prometheus 端点 | [prometheus](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/prometheusreceiver) | 多种 exporter/应用共用同一 Receiver，需区分 target、实例及指标语义 |
| SDK 推送 | [otlp](https://github.com/open-telemetry/opentelemetry-collector/tree/main/receiver/otlpreceiver) | 标准 Metrics，自描述指标目录与模板匹配 |

如 [MySQL metadata.yaml](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/receiver/mysqlreceiver/metadata.yaml) 包含指标、单位、类型、属性和默认启用状态，可以帮助建立配置与图表候选；它不包含完整仪表盘布局和业务查询，也不证明当前版本、目标平台、权限和采集配置下存在全部指标。动态 OTLP/Prometheus 数据还需结合真实目录。

Metrics 统一经 Collector 汇入可行，但部分数据仍来自 SDK、Prometheus exporter 或独立采集程序。版本差异的例子是 [JMX Receiver v0.156.0](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.156.0/receiver/jmxreceiver/README.md) 已标记 deprecated，并推荐独立 [JMX Gatherer](https://github.com/open-telemetry/opentelemetry-java-contrib/blob/main/jmx-metrics/README.md) 向 Collector 推送 OTLP；不能把“Collector 插件接入”限定为“所有采集工作都只在一个 Collector 进程内完成”。

## 4. 插件、厂商与业务来源不是一一对应

- `prometheus` 可采 MySQL exporter、Redis exporter 和应用指标；只有 Receiver 名称不足以选择业务模板。
- `filelog` 可以读取 NGINX、Java 应用和普通文本日志；业务类型取决于目标和解析配置。
- `otlp` 可能承载 OTel SDK，也可能承载前置 Collector 已转换的其他厂商数据。
- Receiver 类型、Receiver 实例、Collection Profile、SDK 声明、业务服务需分开记录。可以按插件组织入口，但不能把其中任意一个字段当作其他字段的可靠替代。

推荐的配置过程是：选择信号 → 选择采集集成/来源 → 选择授权资源及实例 → 在该来源的能力范围配置查询和视图。插件已安装、已启用、曾收到数据、当前时段有数据应是不同状态。

## 5. 当前 Argus 缺口与架构影响

| 当前事实 | 按来源展示所需补齐 |
| --- | --- |
| [Catalog](../../internal/telemetry/catalog.go) 保存 Distribution/Component/Profile 安装能力 | 新增可追溯的采集实例和配置版本关联，不能仅看启用列表猜来源 |
| [pipelineConfig](../../internal/otelcol/configbundle/config.go) 按信号合并多个 Receiver | 合并前标记各接收实例来源，中转时保留原始来源，不覆盖成最后一跳 otlp |
| [Gateway](../../internal/otelcol/argusgatewayidentity/processor.go) 和 [Ingest](../../internal/telemetry/ingest.go) 清除自报 argus.* 后重建身份 | 来源必须进入既有受控校验和保留链路，不能简单添加一个会被清除或可伪造的标签 |
| 三信号已存部分 scope 信息，但查询未普遍暴露 | scope 是生产库元数据，不是可信插件身份；需独立的来源字段与查询过滤 |
| [metricSeriesID](../../internal/telemetry/writer.go) 不含 receiver/source/scope | 同名同标签可能跨来源合并，需同步明确序列身份和重复采集处理 |
| 当前自定义 Trace GraphQL 只有基础查询 | 各厂商视图需要对应的查询契约和能力支持，接口名字为 SkyWalking 不等于原生兼容 |
| [Trace 表](../../internal/telemetry/tenant_schema.go) 的属性主要是字符串映射，Writer 会字符串化部分类型 | 厂商能力若依赖原类型，应调整存储与投影，不能只更换界面文案 |

这次要求会影响 Collector 配置生成、身份链路、落库与索引、Catalog、QueryTarget、缓存、AI 文件来源和前端视图选择。不能仅给现有 Trace 表格增加一个厂商下拉框。对象权限仍按 Dashboard/Host/Cluster，不增加按插件分权。

## 6. Q35/Q36：来源绑定与实现边界

一个采集集成可以把接收组件、字段映射、支持的查询、默认图表和专用详情关联起来。组件和业务配置采用版本化登记；统一传输与存储可以复用，Trace 查询和视图明确指定来源，不静默混合厂商。

一个 Dashboard 可放置分别绑定 SkyWalking、Jaeger 或 OTel 等来源的独立 Trace Panel。每个 Trace Panel 的展示 QueryTarget 继承同一来源绑定；跨厂商合并不是隐式默认能力。Q39 下钻单独发布目标信号、来源和参数映射，允许明确的跨信号关联。配置来源、查询、局部条件与下钻进入个人草稿，随 Dashboard 统一预览发布；Host/Cluster 页面导航绑定不限制查询全集。

厂商对齐指对应功能、信息结构、字段关系与交互，外观组件和对象授权继续遵循 Argus。Q35 没有要求直接嵌入厂商原生 UI 或原样开放其 Query API；若某项所需字段在转换/存储中缺失，应补齐链路或明确支持状态，不能仅换厂商名称。

原始采集实例的来源需与当前中转实例分开；同厂商跨 Host 的链路可以通过授权实例集合查询，不能把来源范围固定成单个 Collector 而破坏 Q31 的 A/B/C 展开。来源未知时不得猜厂商；具体通用入口或显式绑定策略需要在来源设计中固化。

Q37 已确认按来源类型＋顶部授权资源动态匹配，具体实例在局部筛选；停采保留历史，重装区分新旧来源。解析包含请求时间范围内的历史来源，每次执行冻结实际成员及配置版本，后续新查询才纳入新增成员。Q38 规定服务/实例不按名称自动合并，默认在各图筛选；共享业务条件必须明确候选来源及各图映射。

Q41 的 APM 以 Dashboard 专用统计图、展开详情和全屏视图承载，按厂商组织；不新增独立 APM 一级页面。标准下钻由平台按来源/图型生成，依 Q39 纳入发布配置，人工与 AI 共用；界面不增加导出配置。

新增能力应使用真实输入验证“接收 → 来源保存 → 字段映射 → 查询 → 展示”，包括多来源同名指标、同一批次多来源、中转、配置更新后迟到、跨资源授权和专有字段丢失。采样口径的前一轮问题保留，本轮先讨论来源与视图组织。

## 7. 两层过滤的交互与执行规则

| 位置 | 内容 | 作用范围 |
| --- | --- | --- |
| Dashboard 顶部 | 时间、授权 Host/Cluster，以及明确配置的共享变量 | 时间和系统资源强制作用于全部适用 Panel；环境/服务等共享变量继续只影响显式引用它的查询 |
| Panel/Trace 分析视图内 | 当前来源支持的服务、实例、接口/操作、标签、状态、时延等局部条件 | 仅当前 Panel，不反向修改顶部或其他 Panel |

布局：顶部保持紧凑；Panel 标明来源，常用过滤就近展示，高级条件折叠到“更多筛选”，有效条件以摘要显示。Demo 已提供相应交互；小尺寸 Panel 可通过“展开探索”查看过滤和详情。正式组件仍需按业务实现验收。

执行遵守既有边界：

1. 当前用户对象授权、顶部时间/系统资源、Panel 来源绑定、已发布查询、实际引用的共享变量及局部条件共同决定结果。局部条件进一步筛选，不能通过覆盖顶部条件扩大资源范围。
2. 顶部时间/资源或前置变量变化后，重新获取受影响 Panel 的候选与结果；局部条件变化只影响本图及其内部依赖，不刷新无关统计图。旧请求不能覆盖新条件结果。
3. 每个局部过滤项有稳定 ID、类型及发布配置中的查询参数绑定。服务端按语言和来源能力校验，不用任意查询文本拼接，也不在返回的第一页上冒充全量筛选。
4. 候选型局部值消失时沿用 Q22 自动回退“全部”，局部 All 仅解除本项条件；不能移除来源、对象授权、顶部条件或原查询约束。关键字、Trace ID、时延等手输值不因结果为空自动删除。
5. 有效条件组合后无匹配记录时显示无数据；不为得到结果而自动放宽顶部条件。相同显示名的不同厂商服务/实例不得自动视为同一实体。
6. 来源、过滤定义/默认值是发布配置；普通查看中调整过滤是运行态。保存新的默认值仍进入草稿发布，不修改他人当前视图。
7. Q31 的“明确打开完整链路”是独立的已授权详情展开，不是普通局部筛选；展开后仍检查当前 B/C 对象授权，不更改 Dashboard 顶部资源选择。
8. Chat 继续从已发布默认值和用户明确参数构建自己的条件。局部参数按 Panel ID 保存与继承，不能读取浏览器状态，也不能把一个厂商的条件自动套给另一个厂商。

## 8. 仍需澄清

- Q34 已确认展示已接收样本统计并标注来源，不推算全量；这与两层过滤共同生效。
- Q40 已确认：新取数采用最新发布版，旧文件保留原版本，单次查询冻结配置，跨轮仅继承兼容的明确条件。
- 日志“fotelp”的准确项目名称待用户澄清，不预先确定其接入协议或支持范围。
