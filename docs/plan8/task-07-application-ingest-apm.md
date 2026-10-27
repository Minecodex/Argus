# P8-T07：应用 SDK 接入、Key 与 APM

状态：in_progress。前件：P8-T02、P8-T03、P8-T04、P8-T06。

2026-10-21 补齐当前 SDK 接入配置/API/原生工具/门户、公开 TLS 交付、整批服务身份拒绝和认证失败诊断；三平台发行版为 0.3.1-plan8，上游 0.159.0。实际 OTLP HTTP/gRPC、Jaeger gRPC/Thrift HTTP 及权限/轮换已有组件证据，中英明暗门户通过。完整正式安装、模型与所有场景仍未通过，详见[本轮记录](./57-completion-delivery.md)；以下实施段落保留历史范围。

## 设计与接点

- [03-collection-and-ingest.md](./03-collection-and-ingest.md)
- [04-credentials-and-api-contracts.md](./04-credentials-and-api-contracts.md)

已有/拟扩展接点：`internal/telemetry`、`internal/telemetry/queryengine/skywalking`、`internal/dashboard`、`internal/secret`、`web/apps/enterprise/src`、`web/packages/ui`。新领域包和具体接口在 T01 固化；路径是定位入口，不代表本任务已实现。

## 实施清单

- [ ] 按协议验证SDK首次/并发自动发现、有效服务名、缺实例ID、认证Key及真实监听地址；原生转换存在不等于Argus注册/认证已完成。
- [ ] 创建应用级接入配置并交付 Endpoint/协议/写入 Key，新建不要求预填服务；Key 摘要存储、有限信号/环境/来源范围、轮换、撤销、配额和使用观测。
- [ ] OTLP HTTP/gRPC 与 Jaeger 分别实现/验证认证适配，保留 Leaf/Gateway/平台受控入口的网络边界。SkyWalking 原生协议经用户确认新增暂缓，保留失败证据与拒绝启用检查。
- [ ] 业务数据按可信服务身份归属，多运行实例可聚合；有效未知服务受控幂等创建，错误环境/越权身份及缺有效服务名明确拒绝。
- [ ] 扩展 APM 服务/实例/接口/RED/依赖/慢错误 Trace 与来源明细，复用现有 GraphQL 和 UI。
- [ ] 保持入口样本口径、真实分位数、Coverage、跨应用授权展开及 Trace ID/Span ID 日志关联。
- [ ] 应用接入页展示 waiting/received/rejected/credential revoked 的真实状态和可操作诊断，不以配置创建成功替代已接收数据。

## 完成标准

- [ ] 使用实际 SDK 的请求调用链和业务日志验证服务环境、Key 轮换撤销、不同副本及跨应用可见性。
- [ ] 同名不同环境不混合；换 Collector 不分裂逻辑服务；采样不冒充全量请求率；Key 原值不进入模型/审计。
- [ ] 相关存储、接口/生成客户端、权限、门户/工具、审计、配置和文档保持一致；单文件小于 2000 行。
- [ ] 保存测试范围、失败迭代、源码/版本与证据；只完成 mock 或单层测试不计真实验收。

## 验收关联

对应 [验收矩阵](./09-acceptance-matrix.md)：P8-A11、P8-A12、P8-A13、P8-A29、P8-A30、P8-A31、P8-A32。

新增验收关联：P8-A75。

## 实施记录

最新应用七视图、稳定服务身份、入口权限及精确窗口证据见[应用可观测入口](./27-application-observability-and-time-boundaries.md)。真实 SDK、跨来源去重和部署后完整 APM 仍是未完成门禁。

已加入应用接入点、Key 生命周期、摘要存储、独立一次领取接口、并发服务发现、Collector 认证/身份组件与 Ingest 复核。真实数据库及原生配置验证见 [实施记录](./14-implementation-log.md)。真实 SDK、平台链路、地址/TLS、APM 与门户仍在实施，不计本任务验收完成。

日志规则可引用预先声明的服务；声明不产生虚假首次/最近上报时间，SDK 首次发现相同 namespace/name 会复用身份。真实 PostgreSQL 证据见[声明与发现](../../artifacts/plan8-service-declaration-postgres.log)，门户四场景见[中英明暗验证](../../artifacts/plan8-service-declaration-ui-verified.log)。SDK 接入仍无需预选服务。

已修复真实复验暴露的应用接入点来源约束遗漏，新增来源三分支与严格登记检查。旧免 Key Profile 退役，SkyWalking 旧 Profile 也禁止编译；OTLP/Jaeger 的 Endpoint/Key、服务发现、固定环境和撤销复验通过。实现范围、首次失败和证据见[来源准入](./17-managed-defaults-and-sdk-admission.md)。地址/TLS、部署后 SDK/APM 与真实模型仍未验收完成。


真实 Go Trace/Log/Metric SDK 联合链路、应用自动发现、七份查询文档、HTTP/原生工具、跨应用隔离和三信号 Key 撤销/轮换已通过，见 [SDK 证据](./30-native-sdk-pipeline-and-signal-privacy.md)。正式安装注册、地址曝光、其他 SDK/协议、跨来源去重和真实模型仍未完成；真实门户正在单独验收。
