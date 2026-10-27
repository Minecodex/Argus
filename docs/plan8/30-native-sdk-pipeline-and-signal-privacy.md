# 真实 SDK 联合链路、查询入口与三信号脱敏

2026-10-19，新增实际 Go Trace / Log / Metric SDK 联合验证。测试中的三个 HTTP 服务实际发起调用，使用 TraceContext 传播上下文，经内置 OTLP TLS Receiver、真实 mTLS Key 鉴权、PostgreSQL 服务发现、Ingest、SCRAM Kafka、Writer、ClickHouse 与查询引擎完成取数。

这是隔离组件与真实后端联合验收。Collector 控制记录、证书颁发和可信会话由测试夹具创建；未经过正式主机安装/注册、证书轮换、部署后的独立 Query gRPC 服务或真实模型。因此不抵扣这些尚未完成的门禁，也不视为 Plan8 已发布。

## 已验证行为

- 生产、测试是两个不同应用。生产应用的 `payments/api` 与 `inventory/api` 无预建服务，由三信号上报自动登记；同名服务按 Namespace 区分，轮换 Key 不分裂服务或运行实例。
- SDK 自报其他应用和执行 Host ID 被原生身份处理与受信来源覆盖。环境由应用继承；业务数据不进入执行主机范围。
- 实际调用产生生产应用四个 Span、两条关联日志与两个服务计数器，另一个应用有自己的 Span、日志和计数器。相同 Trace ID 不跨应用泄露详情；部分链路明确缺少父节点。
- 服务、实例、接口、RED、拓扑、Trace 列表及详情使用正式查询文档。请求样本只计 SERVER / CONSUMER；客户端操作可有观测 Span，但没有入口请求时，错误率和请求时延保持 unavailable，不制造统计。
- 仅有 `application.read` 与一个应用数据授权的用户，通过真实会话和生成 HTTP 路由、原生 Tool 查询同一数据。服务 ID 变量有效；显式选择另一应用/Host 被拒绝，GraphQL 变量不能扩大范围，撤权后两入口拒绝读取。
- 三个信号均验证 Key 撤销返回 401，轮换新 Key 后恢复，服务和实例身份不重复。Key 原文不进入 Collector 配置、数据或诊断。

SDK 固定为 Trace / Metric `1.44.0`、Log `0.20.0`，与[上游同批发行依赖](https://github.com/open-telemetry/opentelemetry-go/blob/v1.44.0/sdk/log/go.mod)一致。原生 Collector 为 `0.158.0`，Linux amd64 二进制 SHA256 为 `fdea3b018f485f9d2014f90402dc5915105f036890829f0c549b870b84ac315b`。SDK 依赖位于主工程测试；Collector 自有组件为独立 Go module，未因此修改其冻结版本或引入外部 exporter。

## 实测发现与修复

首轮实际 Trace 查询发现密码属性会进入持久数据。原有 Kafka 前脱敏只覆盖日志，现将同一 `datapolicy` 规则抽为共享 OTLP 属性/值投影，并在 Ingest 成功核验来源后、序列化和 Kafka 发布前应用到三信号。

Trace 覆盖 Resource、Scope、Span 属性、事件、Links、状态文本及已识别的凭据文本；指标覆盖 Resource、Scope、Metric metadata、五种数据点属性及 Exemplar 属性。保留数值、整数精度、桶、时间、Trace/Span ID 和非敏感 Key 引用。读取侧投影继续保护历史及派生结果。这是已知凭据字段/文本规则，不宣称能够识别任意业务秘密。

真实 SDK 指标还发现 PromQL 把所有 `_count` / `_sum` / `_bucket` 名称当作虚拟直方图指标。修复后同时选择实际完整名称和类型匹配的虚拟序列；普通标量基名不能冒充直方图。没有改名规避或放宽断言。真实与虚拟同后缀共存、Summary、Histogram 及普通基名否定路径已用 Writer / ClickHouse 验证。

原生 Trace 工具补充 `operation_name` 和 `variables`，保持与门户参数化文档一致。变量最多 64 个、编码后不超过 64 KiB，仍在服务端已授权范围内求交。它们进入现有查询快照与缓存标识。

## 证据

| 范围 | 证据 |
| --- | --- |
| 三信号 SDK、实际 HTTP 调用、身份、数据、查询、撤销/轮换 | `artifacts/plan8-native-sdk-three-signals-verified.log`，通过 |
| 三信号队列前脱敏与整数/分布保真 | `artifacts/plan8-three-signal-prequeue-redaction.log`，通过 |
| 真实和虚拟指标后缀 | `artifacts/plan8-promql-real-and-virtual-suffix-metrics.log`，通过 |
| 原生 Trace 参数与界限 | `artifacts/plan8-parameterized-trace-tools.log`，通过 |
| 真实门户读取这批 SDK 数据 | `artifacts/plan8-native-sdk-real-portal-desktop-views.log`：中英/明暗四场景，各核对视图实际响应、调用链详情和日志/服务筛选，并在 1280/1440/1920 桌面宽度检查布局及可访问性；无 mock API 或截获返回替换样本 |

首次失败的 interval、entry-counts、redacted、three-signal-diagnostics 记录保留。每轮 Docker 容器和网络带本轮 owner 并按归属回收；ClickHouse 测试租户表回收。PostgreSQL 控制夹具留在明确的隔离测试数据库中，由本轮数据库环境统一清理，未触及业务库。

浏览器使用正式 real 模式门户、完整 HTTP 路由/会话与 OpenAPI 校验；会话及 CSRF 由本轮隔离数据库的真实记录提供，未把模拟身份直接塞入页面。Trace 录像关闭，避免将测试会话凭据写入网络录像。服务端与 Vite 进程在测试结束关闭。

仍待完成正式注册/接入与地址曝光、部署后的完整门户验收、其他 SDK/协议组合、重复埋点/跨来源统计、Agent / Workspace / 真实模型与统一发布。Windows 实机测试按[用户决定](./28-windows-testing-deferral.md)暂缓。
