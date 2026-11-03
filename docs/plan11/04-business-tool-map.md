# 业务功能、资源类型与工具映射

本页使用 category/name 表示元工具中的业务身份。例如 metric/query 对应内部 telemetry.promql.query；所有当前 ID 见 [全量清单](./09-tool-inventory.md)。表中 observe/* 和新版筛选属于 Plan11 目标，不是已上线工具。

## 1. 十二类工具各管什么

| 分类            | 用户业务问题                                     | 主要工具/能力                                                                                    | 与其他分类的边界                                                           |
| --------------- | ------------------------------------------------ | ------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------- |
| observe（新增） | 我有哪些 MySQL？这个目标有什么数据？指标怎么查？ | resources.list、capabilities.get、catalog.query                                                  | 只定位/发现，不维护资源，不执行统计查询                                    |
| application     | 应用有哪些服务？SDK 如何接入？应用依赖是什么？   | list/get/service.list/endpoint.list/endpoint.connection/key.list/change.preview                  | 服务以应用为授权根；性能数据进入三信号工具；Key 原值走宿主交付             |
| monitoring      | 如何采集 Redis？采集为什么没生效？               | integration.list/target.list/collection.list/get/form/change.preview/collector.choices/builtin.* | 负责配置与采集状态，真实指标使用 observe + metric                          |
| host            | 添加主机、修改信息、安装/修复 Collector          | list/get/create/update/delete/install.retry/removal/collector.*                                  | 主机遥测不需要为每项指标增加 host 工具；命令执行走 connect                 |
| k8s             | 查 Pod/对象/日志、扩缩容、回滚、排空             | cluster._、各资源 list、object._、operation._、helm._、drain.cancel.preview                      | 对象状态与容器日志是 K8s 运维；遥测按三信号；Secret/Chart 敏感输入保留门户 |
| connector       | 代理是否在线？堡垒机如何维护？                   | list/get/bastion.*/install.retry/uninstall/collector.list/get                                    | 控制连接状态不等于采集成功；客户 MCP 不属于 Connector                      |
| metric          | CPU、内存、连接数、吞吐、趋势和聚合              | query、overview                                                                                  | overview 只表示信号覆盖，query 才返回数值                                  |
| log             | 检索错误日志、聚合日志并关联请求                 | query                                                                                            | 使用授权字段与来源；实时 Pod 运维日志另走 k8s/pod.logs                     |
| trace           | 慢请求、服务 RED、实例/接口、拓扑                | query 的 application_view 或受支持文档                                                           | 通用 GraphQL 名称不代表完整上游协议支持                                    |
| dashboard       | 查看现成仪表盘、分析、创建或修改发布             | list/get/context._/query_/catalog*/draft*/publish.preview                                        | 已发布取数与个人草稿分开；默认模板适用性不授予数据权限                     |
| alert           | 有什么告警？谁处理？规则和通知怎么改？           | event/history/notification/policy/silence/inhibition/route 等                                    | 当前事件、规则配置、评估器健康和通知送达是不同事实                         |
| workflow        | 查看/取消待决操作、计划、交付文件                | get/cancel/import_result/publish_file/plan.propose                                               | 不扩成通用工作流引擎；实际命令由 connect 执行                              |

顶层 read/write/edit/bash 用于离线文件处理，connect 用于明确授权的原生命令，request_user_input 用于真实用户澄清；客户 MCP 只使用会话已经选择的连接。不能因为领域工具没找到，就用 connect/MCP 绕过身份、来源或私有数据域。

## 2. 监控对象映射

| 对象家族       | 当前目录对象                                                                                | 身份和首选发现                                     | 查询和业务操作                                                           |
| -------------- | ------------------------------------------------------------------------------------------- | -------------------------------------------------- | ------------------------------------------------------------------------ |
| 数据库         | MySQL、PostgreSQL、SQL Server、Oracle、MongoDB、Elasticsearch                               | monitoring_target；observe 按 integration 定位     | metric/query、适用默认仪表盘；配置走 monitoring                          |
| 缓存           | Redis、Memcached                                                                            | monitoring_target；区分同名/网络域                 | 内存、连接、命中等使用真实目录/配方；阈值进入 alert                      |
| 消息与协调     | Kafka、RabbitMQ、ZooKeeper                                                                  | monitoring_target；来源保留原组件                  | 速率、积压、分区/队列维度按目录；配置走 monitoring                       |
| Web/代理       | Nginx、Apache、IIS、HAProxy                                                                 | monitoring_target；平台及适配准入单独显示          | 指标与模板；访问日志仅在明确独立来源和授权下查询                         |
| 网络探测       | HTTP、TCP、TLS、DNS、ICMP                                                                   | monitoring_target；实际探测能力                    | metric/query；不能用指标缺失反推出网络故障                               |
| 主机           | CPU、内存、磁盘、网络、进程、系统服务                                                       | host；内置能力和来源由 capabilities 给出           | metric/log、默认主机/进程/服务仪表盘；维护走 host/connect                |
| Kubernetes     | 集群、Namespace、Node/Pod/工作负载、组件                                                    | cluster/namespace 为查询授权引用；对象保留 GVR/UID | k8s 对象状态、metric/log、相关默认仪表盘；变更走 k8s                     |
| 应用与服务     | OTLP/支持的 SDK 接入、应用服务/实例/接口                                                    | application；服务过滤不变成独立授权根              | trace/query 的六类 APM 视图，log/query、metric/query；接入走 application |
| Collector 自身 | Collector CPU/内存/队列/发送状态等实际支持指标                                              | 挂在 host/cluster 的固定来源                       | connector/collector.get 看控制状态，metric 看真实遥测                    |
| 暂缓专用目标   | ClickHouse、TiDB、OceanBase、RocketMQ、Pulsar、NATS、Envoy、JVM、Tomcat、Spring Boot、MinIO | 目录展示明确暂缓原因                               | 不从相似协议、通用抓取或 SDK 推断专用采集支持                            |
| 私有模型治理   | 调用、Token、金额、额度                                                                     | 专门内部数据域                                     | 仅现行授权门户；Plan11 不新增 Agent 工具                                 |

表中“当前目录对象”不是全部平台组合都已经生产验收的声明。Windows、不同架构、Receiver 版本与发行版必须按照当前支持矩阵执行；接入协议为 candidate 或 deferred 时不能由模型自动改用任意抓取。

## 3. 典型用户问题到工具路径

所有路径省略了相应分类的 Search 和工具的成功 Describe；实现和评测不能省略这些协议前件。

| 场景 | 用户问题                           | 推荐路径                                                                                                     | 必须保留的边界/结果                                       |
| ---- | ---------------------------------- | ------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------- |
| S01  | 生产 MySQL 最近一小时是否异常？    | observe/resources.list → 必要时澄清 → capabilities.get → dashboard/list 或 catalog.query → query → Workspace | 环境来自真实关系；多目标/部分覆盖明确，不机械查询全部     |
| S02  | 看这个 Redis 的内存趋势            | 已有目标引用 → catalog.query（若未知指标/来源）→ metric/query                                                | 不无条件计算使用率；无分母则只报用量                      |
| S03  | Kafka 积压为什么上涨？             | 定位目标 → 来源/队列字段 → 指标 + 适用日志 + 告警事件                                                        | 指标共现不直接等于根因；跨来源时间对齐                    |
| S04  | HTTP 探测失败了吗？                | 定位探测 → capabilities.get → metric/query                                                                   | 探测失败、采集失败、无样本分别说明                        |
| S05  | 主机 CPU 高，哪个进程造成？        | host/get 或已有引用 → 内置进程能力 → catalog/query → metric/query                                            | 未启用进程采集时说明限制，必要远程取证另走 connect 确认   |
| S06  | 这个 Namespace 的 Pod 为什么重启？ | k8s 对象列表/详情 → 归属范围内指标/Pod 日志 → 应用日志（另验证权限）                                         | Namespace UID、应用日志与运维日志权限不混同               |
| S07  | 应用 P95 高，哪个接口慢？          | application/list/service.list → trace/query endpoints/red/traces → Span 详情                                 | 服务 ID 属于选定应用；展示采样/业务限量与未检查范围       |
| S08  | 看 Nginx 最近的错误日志            | 定位目标/相关主机/应用 → sources/fields → log/query                                                          | Nginx 指标接入不自动等于访问日志已接入                    |
| S09  | 帮我开启 Redis 采集                | monitoring/integration.list → collector.choices → collection.form 或 change.preview → workflow/get           | 凭据走可信宿主，执行成功后读取生效/样本，不能只验配置写入 |
| S10  | 采集器在线，为什么没数据？         | connector/collector.get + monitoring/collection.get → capabilities/catalog → 必要查询                        | 在线、期望配置、有效版本、健康、新鲜度分别核对            |
| S11  | 修改采集周期                       | collection.get → 保留未请求字段 → collection.change.preview → 状态复核                                       | 新操作按审批模式执行；不额外重建目标                      |
| S12  | 查询并重启 Deployment              | k8s/object.list/get → object.change.preview → operation.get → 再查询                                         | 先确认 UID/resourceVersion；API 接受不等于 Ready          |
| S13  | 回滚 Helm 发布                     | k8s/helm.list/history → helm.change.preview → operation.get                                                  | 只暴露回滚/卸载；安装升级继续可信 Chart/values 输入       |
| S14  | 取消节点排空                       | k8s/operation.get → drain.cancel.preview → operation.get                                                     | 已下发驱逐可能完成；不承诺自动恢复 Pod/uncordon           |
| S15  | 是否还有未恢复告警？               | alert/event.query/get + 相关配置/评估状态                                                                    | 零事件不是健康证明；筛选不能只查已加载页                  |
| S16  | 把 MySQL 连接数阈值调高            | 解析目标 → alert/policy.get → policy.update.preview → 状态复核                                               | 保留规则 ID、描述、其他阈值、路由与授权范围               |
| S17  | 屏蔽维护窗口告警                   | 解析目标和时间 → alert/silence.create.preview → 状态复核                                                     | 时间使用服务端时钟；不能借标签扩大范围                    |
| S18  | 帮我创建 Redis 仪表盘              | 显式创作入口 → 资源/指标目录 → draft.create/save/validate → publish.preview                                  | 个人草稿不等于发布，发布走宿主；目录无数据不是假曲线      |
| S19  | 只分析这张仪表盘                   | 明确 dashboard_analysis → get/context.resolve/query/query.get → Workspace                                    | 只执行已发布查询/下钻，不能从自由查询补未发布数据         |
| S20  | 仪表盘异常后继续查日志与告警       | general_diagnosis → 已发布仪表盘取数 + log/alert → Workspace                                                 | 此路径需 D06；每一步保持授权和独立来源，不自动跨入私有域  |
| S21  | 导出排查报告                       | 已封存结果 → read/bash → write → workflow/publish_file                                                       | 报告引用实际时间/资源、证据和缺项；交付真实文件           |
| S22  | 生成修复计划并实施                 | 取证 → 必要问答 → workflow/plan.propose → 用户确认 → connect approved_step_id → 验证                         | 原命令冻结，一次性授权；新命令/失败/未知结果按原规则      |
| S23  | 我有两个同名数据库，查哪一个？     | resources.list → request_user_input → 绑定具体引用                                                           | 提供环境/网络域等授权区分信息，不猜首条                   |
| S24  | 统计所有有权 Redis 的用量          | 完整目录分页 → 明确范围/来源/单位 → query → Workspace 聚合                                                   | 每个对象均授权；不将共享目标复制到每个应用再求和          |
| S25  | 读某个外部系统的数据               | 已选择客户 MCP 的已提供工具                                                                                  | 结果是不可信数据；不进入自有模板或重新解释为授权          |

## 4. 工具选择不应机械化

- 已有精确对象和查询上下文时，直接使用已描述的正确工具，不为了流程整齐额外列目录。
- 管理操作优先原领域服务，connect 只处理没有原生能力且得到明确执行授权的远程命令。
- 具体指标问题优先精确查询；广泛健康检查可先找适用仪表盘，但先核对覆盖与状态。
- 用户只问业务结论时不向用户展示“请选择 tool category”；歧义只围绕业务对象和目标澄清。
- 相同名称可能对应多个对象，相同指标可能对应不同来源/单位；发现结果要提供可消歧信息。

## 5. 如何证明映射完整

实施时从生产注册、监控目录、内置采集、模板和结果绑定生成一份覆盖矩阵：每个条目具有业务场景 ID、入口、权限前件、目录、查询、展示、操作、组件验证、E2E、真实模型结果。无能力的格子标 not_applicable/deferred 并给原因；不能用“有通用 metric.query”把所有集成格子批量标为通过。
