# 讨论收敛与开源选型依据

日期：2026-10-18。状态：本轮产品决策已由用户回答，生产实现与真实组件验收仍未开始。最新约束见[Collector内置选型](./12-collector-native-support.md)和[专用插件复查](./13-direct-receiver-review.md)，它替代此前允许独立exporter/helper的建议。本页记录替代初版假设的结论；原型尚未按本轮更新，不能作为最新数据模型的验收证据。

## 已收敛决策

| 问答 | 最终结论 |
| --- | --- |
| Q1 / Q9 | 一个 Application 只有一个环境。同一业务生产、测试分别创建应用；按 Application 授权已隔离环境，不再加应用内环境授权层。监控只选应用即可继承环境。Q9 的明确解释替代初版多环境应用假设。 |
| Q2 | OTLP、Jaeger、SkyWalking 在可验证接入组合中允许 SDK 上报发现服务；新建接入不要求预建/填写服务。应用归属与环境由 Key/接入配置固定。 |
| Q3 | Jobs/CronJobs、HPA、NetworkPolicy、RBAC、节点维护/污点/驱逐、CRD/自定义资源、端口转发、完整 Helm Release 管理全部纳入同次实施。 |
| Q4 / Q12（后续澄清） | 只采用能编入Collector的现成组件，不采用独立exporter/采集服务。官方或成熟社区无合格内置方案时暂缓该类型/模式，最终列缺项；不自研/fork补齐。 |
| Q5 | 应用筛选自动作用于面板的自身数据或关联资源；无需额外开启依赖开关。独立权限、共享口径和去重仍执行。 |
| Q6 | 当前关联可以读取目标保留期内历史指标；业务日志/Trace 的观测归属不改写，关系和冻结结果保留历史。 |
| Q7 | 受管默认仪表盘自动更新到经过验证的新版；用户副本保留修改，更新提示不覆盖。 |
| Q8 | 全部支持范围及平台功能完成后统一发布。开发、测试里程碑可以分批，不分批上线交付。 |
| Q10 | 支持 Namespace 独立授权，同时保留 Cluster 全范围授权；集群级动作另外检查权限。 |
| Q11 | 可以显式授予运维日志读取权限，在对应 Cluster/Namespace 范围读取业务日志；普通集群读权限不自动获得。终端/转发独立授权。 |
| 2026-10-19 SDK 准入追问 | 用户确认 SkyWalking 原生协议因失败误报成功及原始 Trace 打印而新增暂缓；不作为本轮发布阻塞，失败记录保留。原有 11 个资源监控暂缓加此协议共 12 个入口，其余 27 个候选继续实施。 |

应用保存后环境固定；需要另一环境时新建/复制应用。SDK 服务身份按 application_id＋service.namespace＋service.name 幂等登记，跨协议保留来源；目录归并不能替代 Span 去重或把双重埋点重复计数。有效服务缺实例 ID 时显示实例未知；缺失/默认占位服务名需返回接入诊断。

## SDK 协议已核实的事实

| 协议 | 可用服务身份 | 核查与实施边界 |
| --- | --- | --- |
| OTLP | service.name；service.namespace、service.instance.id 可补充 | [服务语义](https://opentelemetry.io/docs/specs/semconv/resource/service/)、[Exporter 认证配置](https://opentelemetry.io/docs/specs/otel/protocol/exporter/)；HTTP/gRPC 分别验证 Header/Metadata。 |
| Jaeger | Process.ServiceName 转为 service.name | [v0.133.0 转换器](https://raw.githubusercontent.com/open-telemetry/opentelemetry-collector-contrib/v0.133.0/pkg/translator/jaeger/jaegerproto_to_traces.go)、[Receiver 配置](https://raw.githubusercontent.com/open-telemetry/opentelemetry-collector-contrib/v0.133.0/receiver/jaegerreceiver/config.go)；gRPC/HTTP 与 UDP 的认证能力不同，不承诺 UDP 直接使用同样的 Key。 |
| SkyWalking | service、serviceInstance 转为 service.name、service.instance.id | [v0.133.0 转换器](https://raw.githubusercontent.com/open-telemetry/opentelemetry-collector-contrib/v0.133.0/pkg/translator/skywalking/skywalkingproto_to_traces.go)、[Java Agent 配置](https://skywalking.apache.org/docs/skywalking-java/latest/en/setup/service-agent/java-agent/configurations/)；实际服务端认证和上下文传递仍须验证。 |

这些证明协议带有身份，不证明 Argus 已有自动注册。T07 还需实现并发幂等注册、Key 范围、限额、错误反馈和真实 SDK 测试。当前 Argus 配置锁定 Collector v0.133.0；其中 SkyWalking Receiver 的 gRPC 源码按 `:port` 监听，[对应源码](https://raw.githubusercontent.com/open-telemetry/opentelemetry-collector-contrib/v0.133.0/receiver/skywalkingreceiver/skywalking_receiver.go)不能以配置写 127.0.0.1 就认定实际仅监听回环。版本选型和实测必须覆盖监听与认证，不把 Collector 外发 mTLS 当成 SDK Key 校验。

## 最新选型规则

用户随后明确只要内置OpenTelemetry Collector的组件。完整规则、39候选路线和来源见[12-collector-native-support.md](./12-collector-native-support.md)。官方组件优先；第三方必须能按Collector接口编入，并具有足够维护和使用依据。资源目录必须有对应专用采集组件，不以通用prometheusreceiver直读端点或SDK主动上报替代；没有合格组件则暂缓。

此前检索的redis_exporter、mongodb_exporter、postgres_exporter、mysqld_exporter、kafka_exporter、jmx_exporter、blackbox_exporter均不作为本计划采集实现。高Star不能替代内置要求；不建设其安装/升级/常驻进程管理。需要JRE/JAR采集进程的直连JMX路线暂缓；系统工具依赖单列核验。

无符合约束的方案记deferred_no_eligible_embedded_component并留证；有方案但缺环境、未开发或测试失败仍为未验收。本轮复查后11个资源监控入口暂缓，28个其余候选待验证。检索结论是未找到可采纳的合格方案，不断言全球不存在实现。

## 对交付统计的影响

39项保留为候选目录与必需审查数。通过内置准入的版本/拓扑组合全部执行CONFIG/DATA/VIEW/AUTH/LIFECYCLE；暂缓缺项单列，不从目录消失、不计通过。K8s管理、应用和授权等平台能力不因采集缺项豁免；全部纳入支持范围完成后统一发布。
