# 展示深度、KQL 与完整 APM：Q27～Q34 对齐

## 状态

2026-09-24 用户明确：保留 KQL，目标是类似 Grafana/Loki 的展示和日志探索体验；继续以 OTel/OTLP 接入、ClickHouse 存储为基础，目标覆盖完整 APM。SkyWalking 是数据来源，不因参考其界面而引入 OAP。Q31 已明确入口资源筛选与授权链路展开规则。

Q32 已确定常用 APM 排查能力为首期验收范围，Profiling 后续考虑。Q33/Q34 明确后续通过 Collector 接收组件支持 SkyWalking、Jaeger 等来源，统一为 OTel/OTLP 数据后分析；来源选择已明确，Q34 原问题中的采样统计口径仍待澄清，不能将该回答记录为同意某种指标优先级或全量估算方案。

Q35/Q36 进一步明确 Trace 按 Panel 绑定采集来源，对齐厂商功能与交互，保留 Argus 样式和权限；顶部通用过滤与图内厂商局部过滤分层。本文的统一接入/存储不意味着将厂商视图混为一体，详见 [Collector 插件专项设计](./04-collector-plugins-and-source-views.md)。未回答的采样问题保留。

Demo 展示与网格增强已更新；它们使用模拟结果，不证明真实查询、Trace 拼接或 APM 后端已经实现。Q1～Q26 中的对象授权、个人草稿/发布、Chat 文件分析及无 Dashboard 导出功能继续有效。用户对“开启 Collector 就具备 APM”的理解属于需要澄清的技术事实，不记录为已验证能力。

## 1. 本轮原型增强

- Metrics：Time series、Stat、Gauge、Bar gauge、Bar chart、Pie/Donut、Histogram、Heatmap、State timeline、Scatter、Table，使用仓库已安装的 ECharts 6.1.0。
- 图例、悬浮提示、时序缩放、显示聚合、小数位、范围、堆叠与样式配置。
- Logs：当前结果内关键字/级别/标签筛选、日志量、结构化字段、同一结果集的上下文、关联 Trace 入口。
- Trace：服务/ID/时延/错误筛选、时延分布、样例 Span 树/瀑布、属性和事件。
- 布局：12 列，保存 x/y/w/h；标题把手拖动、右下角同时调整宽高、吸附、碰撞下推、键盘操作、个人草稿持久化。
- 所有结果内探索均不修改已发布查询或 Chat 条件。日志全量检索、实时 tail、服务拓扑和完整链路仍需下面的后端决策。

参考的是交互能力，不承诺复制相关产品全部功能：
[Grafana 图型](https://grafana.com/docs/grafana/latest/visualizations/panels-visualizations/visualizations/)、[Grafana 布局](https://grafana.com/docs/grafana-cloud/learn-and-build/visualizations/dashboards/build-dashboards/create-dashboard/)、[日志探索](https://grafana.com/docs/grafana/latest/visualizations/explore/logs-integration/)。

## 2. Q27：保留 KQL 可以实现丰富的日志界面

查询语言、存储和界面是三个不同层次。KQL 可以继续编译到受控的 ClickHouse 查询；结果模型和交互能力补齐后，可以实现日志量趋势、结构化字段、条件检索、上下文、关联 Trace 及多种统计图。无需为了类似的界面改用 LogQL 或 Loki。这里的 KQL 指现有 Argus KQL，不宣称与其他同名语言完整兼容。

事实：

- 当前 KQL 是字段表达式和受控 Pipeline，处理器保存单一 parser，按管道字符拆分阶段；见 [kql.go](../../internal/telemetry/queryengine/kql/kql.go)。
- 聚合主要是过滤窗口内 count() by field；数量趋势、数值聚合和更完整的逐阶段处理需要扩展 Argus KQL 语义并明确支持矩阵。
- 当前返回契约仅有 log_entries/log_streams；日志派生的时序与向量还需扩展结果模型，见 [m7.yaml](../../api/openapi/components/m7.yaml)。
- 目录、稳定历史分页、日志上下文、实时 tail、完整导出均有各自接口和存储要求，不能靠换 Parser 自动获得。
- 来源信号和结果类型需分离：日志查询可以产生日志行，也可以产生时序/统计值；可视化按结果形状选择，不把一切日志查询都当文本表格。

| 界面能力 | KQL/服务端需要提供 |
| --- | --- |
| 日志列表、字段侧栏、点击字段过滤 | 结构化字段、候选/分布目录、可组合的受控表达式 |
| 日志量曲线、错误数量趋势、分组统计图 | 时间分桶、聚合与明确的时序/表格结果类型，统计整个查询范围而非当前页 |
| 日志详情与前后文 | 稳定记录标识、同一来源的上下文读取接口 |
| 大结果翻页与 AI 文件分析 | 稳定游标、读取边界、去重、取消和完整性状态 |
| 日志跳转 Trace | 已采集的 trace_id/span_id、授权检查和详情接口 |
| 实时追踪（具体范围待定） | 增量读取/订阅、断连恢复、背压和独立预算；不是前端反复追加当前页 |

之前将路线直接分成 LogQL 子集、上游 LogQL 引擎或 Loki 的提议已被 Q27 修正，不继续作为默认选型问题。参考 [Grafana 日志探索](https://grafana.com/docs/grafana/latest/visualizations/explore/logs-integration/) 的能力组织，不承诺 LogQL 语法兼容。

企业隔离不能代替企业内部 Host/Cluster 授权。目录、聚合、上下文及未来 tail 均在读取前受可信对象范围约束；不能聚合完成后再裁剪。

## 3. Q28/Q30：OTLP 接入与 ClickHouse 存储

当前数据路径是应用/采集器 → Collector → OTLP → Argus Ingest → Kafka → Writer → ClickHouse。OTLP 规定遥测编码与传输，并不规定 ClickHouse 表结构，也不提供 APM 查询和界面。Writer 将收到的数据转换成 Argus 的事实表、索引及派生数据；后端还需提供查询和分析。依据：[OTLP 规范](https://opentelemetry.io/docs/specs/otlp/)、[现有遥测架构](../09-opentelemetry-observability.md)。

SkyWalking 来源的可行路径为：

~~~text
OTel SDK / 自动埋点 Agent ──OTLP───────────┐
                                         ├─ Collector ─OTLP─ Argus Ingest / Kafka / Writer ─ ClickHouse
SkyWalking 原生 Agent ─原生协议─ Receiver ┘                                  ↓
                                                               Argus 查询与 APM 分析层
                                                                         ↓
                                                              仪表盘 / 人工下钻 / Chat 工具
~~~

- 原生 SkyWalking 协议与 OTLP 不相同，普通 OTLP Receiver 不能仅靠改地址接收原生 Agent。可以使用兼容的 SkyWalking Receiver 转换，或由客户先转成 OTLP 后接入。
- 与仓库同版本的 Collector Contrib [SkyWalking Receiver v0.133.0](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.133.0/receiver/skywalkingreceiver/README.md) 声明 Java Agent 8.9.0+；Traces 为 beta，Metrics 为 development 且仅覆盖 JVM 数据，不据此承诺所有 SDK、插件、日志与 Profiling 兼容。
- Argus 的 [Linux amd64](../../deploy/otelcol/builder-linux-amd64.yaml)、[Linux arm64](../../deploy/otelcol/builder-linux-arm64.yaml)、[Windows amd64](../../deploy/otelcol/builder-windows-amd64.yaml) 构建清单锁定 0.133.0，均未注册该 Receiver，也没有 Span Metrics/Service Graph Connector；仅配置开关不能启用未编入发行包的组件。本轮核查的是源码和构建清单，未验证运行中二进制。
- 统一输出 OTLP 不会自动修复应用调用间缺失的上下文传播。混用 OTel 与 SkyWalking Agent 时必须验证跨进程 Trace/Parent 身份和传播协议；转换格式不能补造未采集的调用关系。依据：[OTel 上下文传播](https://opentelemetry.io/docs/concepts/context-propagation/)。
- 可信企业与 Host/Cluster 身份仍由 Argus 接入链路约束，不能仅凭客户端自报 service.name 或 host.id 授权。
- Q33 已明确阶段：本期以标准 OTLP 接入验证 APM；后续扩展 Argus 托管 Collector 的接收组件，以类似 OTel Collector 的方式支持 SkyWalking 等来源。扩展 Collector，不改变 OTLP 协议本身，也不另写一套重复采集器。
- Q34 明确 Trace 来源随启用的接收组件和应用 SDK 而定：OTel Trace SDK 可通过 OTLP Receiver 接入；SkyWalking/Jaeger 原生协议通过对应 Receiver 转换；已输出标准 OTLP 的来源直接复用 OTLP 入口。上游也有 [Jaeger Receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.133.0/receiver/jaegerreceiver/README.md)，具体协议/版本支持按后续发行版验收，不推定当前 Argus 已具备。
- 新 Receiver 沿用既有 Distribution/Component/Profile 和配置发布机制，按版本编入发行包后配置启用；数据发现与 APM 展示以实际收到的字段和语义为准，不因插件安装就假定有数据或所有能力可用。

## 4. Q29：完整 APM 需要采集与分析两部分

当前已有 Span ID、Parent ID、时间、服务、属性、事件与 links 等原料；但不能将当前查询叫作完整 SkyWalking 后端。

| 问题 | 当前证据与影响 |
| --- | --- |
| 跨批次组装 | [writeTraceDerived](../../internal/telemetry/writer.go) 只在当前 OTLP 请求的 spans 中找 root 与父服务；无 root 取首 Span，depth 固定为 1 |
| 摘要一致性 | [TraceSummary 表](../../internal/telemetry/tenant_schema.go) 按 resource_id/trace_id 替换摘要；不能据此声明任意多批次/迟到/跨 Host Trace 完整 |
| 查询范围 | [Trace resolver](../../internal/telemetry/queryengine/skywalking/resolver.go) 的实例/标签筛选子查询未同样添加 ResourceIDs，未授权 Span 可能影响命中；此项应独立修复 |
| API 覆盖 | [SDL](../../internal/telemetry/queryengine/skywalking/schema/trace.graphql) 只有少量 Trace 查询，没有完整服务/实例/端点目录、RED 与依赖拓扑 API |
| 缺失状态 | 缺 root、采样、迟到、预算截断、未埋点与授权裁剪须区分；unknown 不能被变成 ok |
| 日志关联 | LogRecord 已可保存 trace_id/span_id，但应用日志必须真的携带上下文，Collector 不能从任意正文恢复链路 |

上述生产代码本轮仅核查，未修改；不能用原型中的完整样例树掩盖这些缺口。

完整 Trace 排查至少包含跨批次拼接、真实层级、Span 属性/事件、慢错误定位和日志关联。完整 APM 还需要服务/实例/端点发现、请求量/错误率/时延、依赖图和排序定位。

### 基于现有 ClickHouse 扩展 Argus 分析层

保留 OTLP、可信 resource_id、租户存储和现有运维。新增跨批次组装/去重/迟到更新、完整性状态、实体目录与 RED/依赖聚合。这个工作相当于建设 APM 分析层，不能作为“加几个 GraphQL 字段”估算。

Collector 也能承担部分计算：[Span Metrics Connector](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/connector/spanmetricsconnector) 从收到的 Span 生成调用数、错误数和时延分布；[Service Graph Connector](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/connector/servicegraphconnector) 配对调用两端以生成依赖指标。后者依赖同一 Trace 的 Span 路由到可配对的实例，并受等待窗口限制。这些组件可复用，但需明确发行版、配置、身份维度、去重和统计口径，不能将两个层次同时计算的结果重复累加。

服务目录、历史存储与查询、完整链路拼接、对象授权和交互仍需 Argus 实现；收到采样后的 Span 不能自动恢复全量请求量。Q32 已确定下面的首期功能清单，采样统计口径仍待澄清。现有自定义 Trace GraphQL 不因此成为完整 SkyWalking 查询协议。

### Q32 已确认的首期 APM 验收范围

| 能力 | 验收内容 |
| --- | --- |
| 服务/实例/接口总览 | 从真实数据发现实体，按时间和授权资源范围检索，展示可下钻的概览 |
| 调用量、错误率、时延 | 提供趋势和时延分布，明确统计对象、时间范围、来源与覆盖状态；采样下的产品口径按 Q34 澄清后固化 |
| 服务拓扑 | 从有效调用关系展示依赖与对应统计，节点/边受对象授权约束，不把缺失服务推定为不存在 |
| 慢请求与错误 Trace | 按服务/实例/接口、时间、时延、状态及受支持属性查询和定位 |
| Span 详情 | 跨批次链路组装、父子树/瀑布、属性与事件；缺失、迟到、截断和授权裁剪明确可见 |
| 关联日志 | 使用实际采集的 Trace/Span 关联信息定位日志，缺少关联字段时显示不可关联 |

Profiling、方法级性能剖析留到后续，不作为本期退出门禁。上述 APM 能力属于正式交付范围，不能以现有 Trace 列表、单条模拟瀑布或 Collector 开关替代验收。

### 共同前提

- 应用/中间件埋点及上下文传播；仅安装 Host Collector 不能自动获得完整应用 Trace。
- 企业、环境、service.name、instance 与端点的稳定身份和规范化。
- 请求计数与内部 Span 区分；采样流统计不能无说明地冒充全量业务 RED。
- 入口 Host 筛选与跨资源链路展开范围明确区分，同时保留对象授权。
- 测试多批次、乱序、缺 root、迟到、重复、跨主机、部分授权与截断，不只验证同一批次 root+child。
- 实际 SPS、Span 大小、保留期和查询负载尚未测量，本轮不推定具体集群容量。

## 5. Q31：入口按 A 筛选，明确展开到授权的 B/C

- 从 Host A 快捷打开，列表及默认查询仍按 A 筛选，不因某条 Trace 跨主机而扩大仪表盘全局资源选择。
- 用户明确打开完整链路时，服务端针对同一 Trace 重新检查当前对象授权，可读取用户另有授权的 B/C Span；不以 A 的权限代替 B/C 的权限。
- 不可访问的部分不能返回实际 Span、属性或可识别的未授权对象信息；对可见链路说明完整性限制，避免宣称绝对完整。
- 入口范围与详情展开范围分别记录；详情缓存和后续读取按实际范围重新授权。属性/实例等筛选子查询同样受授权约束。
- AI 仍遵守已发布查询与 Q26：列表查询不会因这项 UI 展开能力自动扩大导出范围或补取未配置明细。

## 6. Q34：来源已确认，采样统计口径仍待澄清

接收组件决定输入协议和可转换字段；采样决定哪些请求被记录/保留。这是两个独立维度。SDK 或 Collector 均可能执行采样，而且可能按错误或时延选择保留，不能仅凭 SkyWalking、Jaeger 或 OTel 来源推定全量，也不能默认把计数乘以一个固定倍数恢复所有统计。依据：[OTel 采样说明](https://opentelemetry.io/docs/concepts/sampling/)。

待用户澄清的产品选择：客户只上报部分 Trace 且没有可用的完整请求指标时，是否按实际收到的请求 Span 展示样本调用量/错误率/时延并说明覆盖，而不估算全量？建议是；不能用任意内部 Span 数冒充请求数。若另有口径明确的应用请求指标，可作为单独、标明来源的指标展示，两种来源不能混算。

此前提出的“请求指标优先、Span 统计回退”自动选择策略尚未获确认，不写为既定规则。日志 tail、上下文边界、保留期和容量等后续细节仍需细化，不将未决定项写成已完成能力。

## 7. 不变的边界

- 普通查看和个人草稿分离，布局/查询改动统一预览发布。
- 当前选值消失回退全部，但对象授权和系统资源范围保留。
- Dashboard 页面没有导出按钮或导出配置；AI 在 Chat 通过工具内部交付文件后分析。
- AI 自行判断，不能将失败、缺数据或权限裁剪说成正常。
- 新语言/后端不能绕过企业和对象授权，不采用静默降级语法或第二套无审计执行路径。

