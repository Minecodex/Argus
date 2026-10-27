# SDK 原生组件准入核验（2026-10-19）

本记录区分组件协议测试、平台验收和发布范围。2026-10-19 用户明确选择将 SkyWalking 原生协议列为新增暂缓项，不再作为 Plan8 发布门禁。原有 11 项资源监控暂缓保持不变，合计 12 个入口暂缓；其余 27 个候选及平台功能继续实施。SkyWalking 的 AUTH/LIFECYCLE 失败证据保留，不计为支持或验收通过。

## 已取得的证据

- [真实 Collector 协议测试](../../artifacts/plan8-native-sdk-auth-recovery.log)：实际 Go OpenTelemetry SDK 使用 OTLP/HTTP；Jaeger 与 SkyWalking 使用上游生成的原生 gRPC 客户端。四个接入的 Key/source 绑定独立，SDK 伪造资源归属被清除。认证控制端与 OTLP 接收端为隔离测试端点，不等于正式 Ingest/Kafka/ClickHouse/APM 链路通过。
- OTLP/HTTP 的认证服务临时不可用返回 503，撤销 Key 返回 401，恢复后可再次接收。Collector HTTP 认证层会把普通认证错误统一映射为 401，因此暂时性故障通过无授权身份的失败标记交给强制身份 Processor 拒绝，不能形成合法来源；该行为有模块与真实二进制测试。
- [真实数据库测试](../../artifacts/plan8-sdk-discovery-tests.log)：12 路并发首次发现只产生一条服务及实例记录；跨应用、错误环境、无效服务名拒绝，缺实例 ID 不制造默认实例。
- [门户测试](../../artifacts/plan8-sdk-ui-tests.log)：中英 × 明暗四场景创建接入、签发、一次领取、撤销 Key；Key 原文不进入 localStorage，未收到数据时保持等待样本。属于 mock 门户，不替代生产权限、网络与采集验收。

## SkyWalking 原生协议新增暂缓

官方 [v0.158 原生实现](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.158.0/receiver/skywalkingreceiver/internal/trace/tracing_report_service.go)、[v0.162 实现](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.162.0/receiver/skywalkingreceiver/internal/trace/tracing_report_service.go)和本次读取的 main 均有以下行为：

1. `Collect` 收到下游消费错误后关闭流并返回成功响应。
2. `CollectInSync` 忽略下游消费错误，继续返回成功，并直接输出序列化的原始 Segment。
3. HTTP 接收路径也未将消费失败返回为准确的失败状态。

真实测试故意让认证返回的 endpoint version 与 Collector 已应用版本不一致，身份 Processor 正确拒绝数据；SkyWalking 同步 RPC 仍成功，且测试用非敏感 canary 出现在 Collector 原始日志中。测试自身通过表示**缺陷成功复现**，绝不是该协议通过验收。

另有已知监听地址问题，官方 [issue #50787](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/50787)与锁定版本代码一致：gRPC 使用端口构造监听地址，不能依赖配置中的 localhost 限制监听。当前研究配置明确使用带认证的 TLS 监听，没有把它称为回环隔离。

本轮检索了官方版本、主干源码和相关修复记录，未取得可验证的已修复正式版本；不声称所有第三方实现均不存在。按照不自行 fork 的约束，当前 API、Key 验证和服务登记均拒绝启用 SkyWalking；门户展示准入原因，旧 Profile 改为 validation_pending。原生组件仍在研究发行版中用于复现，不能通过产品入口启用。

用户授权变更的是原生 SkyWalking 接入协议的发布范围，不删除现有 Trace 查询能力，也不影响采用 OTLP 接入的应用。重新纳入需使用修复后的正式组件，并重新通过认证、失败传播、日志保密和完整生命周期门禁。

## 待完成

正式 SDK 接入地址/TLS 配置指引、K8s 入口及回滚实测、逐 Resource 的正式 Ingest→Kafka→ClickHouse 链路、APM 逻辑来源去重、完整权限撤销、真实 Java 等 SDK 组合、告警/Agent 与统一发布均仍需完成。仅有正常上报样本不构成 AUTH/LIFECYCLE 通过。
