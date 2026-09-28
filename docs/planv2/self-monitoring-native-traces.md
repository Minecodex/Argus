# Argus 自监控与原生 Trace 接入

## 本轮范围更新

2026-09-25 用户要求继续完善，并用 Argus 项目自身验证 SkyWalking/Jaeger 的效果。本轮把此前后续范围中的**原生 Trace 接收及自监控试验**纳入实施；应用接入仍以 OTel/OTLP 为主，查询仍为只读 Trace GraphQL，权限仍按 Dashboard/Host/Kubernetes 对象处理。

不把 Receiver 当作 SkyWalking OAP 或 Jaeger 查询后端。JVM Metrics、远程采样、Profiling、厂商完整管理 API 和所有原生 UI 兼容不属于本轮新增范围。

## 本轮结果（2026-09-26）

`p2-selfmonitor-20260926-k` 验收及清理通过，Harness 退出码为 0：真实 SDK 接收、Argus API→Query 自监控、三来源 18 张图全部取数成功，正式浏览器 8/8 通过。整个 PlanV2 仍在进行中。

| 验收项 | 实际结果 |
| --- | --- |
| 原生协议 | GO2Sky gRPC 与 Jaeger Go Thrift HTTP 均收到真实 Receiver 确认；各 4 个 Span，查询中各得到 2 个入口请求样本和 1 个受控错误 |
| Argus 自监控 | 首次冻结查询观察到 `argus-server` 14 个入口样本、`argus-telemetry-query` 31 个；服务拓扑确认 API→Query 的 31 个目标入口样本和 1 个受控查询错误 |
| 来源隔离 | 18 张图按 OTLP、SkyWalking、Jaeger 分开执行；返回行的来源 ID 属于各自冻结来源集合 |
| real UI | 中英文×深浅色共 8 个测试：显示、新鲜度、变量依赖、已发布下钻；个人草稿编辑、刷新恢复、样本、一次确认发布和历史列表 |
| 可访问性 | 四种展示组合的 axe serious/critical 检查均无违规；不等同于全产品键盘/读屏验收 |
| 回归 | UI 61 项通过；相关 Go、类型、ESLint、i18n、样式与完整 contracts check 通过；409 个变更源文件均未超过 2000 行 |

自监控查询本身产生后续 HTTP/gRPC Span，因此截图中的计数高于首次冻结查询是正常的；这些小样本不作为性能基准或全量错误率。

查看真实产物：

- [正式仪表盘截图](../../artifacts/planv2-e2e/p2-selfmonitor-20260926-k/playwright-planv2/planv2-real-PlanV2-real-se-8f1fc-s-and-variable-dependencies-chromium/self-monitoring.png)
- [SkyWalking 服务样本](../../artifacts/planv2-e2e/p2-selfmonitor-20260926-k/playwright-planv2/planv2-real-PlanV2-real-se-8f1fc-s-and-variable-dependencies-chromium/skywalking-services.png)、[Jaeger 服务样本](../../artifacts/planv2-e2e/p2-selfmonitor-20260926-k/playwright-planv2/planv2-real-PlanV2-real-se-8f1fc-s-and-variable-dependencies-chromium/jaeger-services.png)
- [Jaeger 已发布 Trace 下钻](../../artifacts/planv2-e2e/p2-selfmonitor-20260926-k/playwright-planv2/planv2-real-PlanV2-real-se-8f1fc-s-and-variable-dependencies-chromium/native-trace-detail.png)
- [真实发布历史](../../artifacts/planv2-e2e/p2-selfmonitor-20260926-k/playwright-planv2/planv2-real-PlanV2-real-se-e2d2c-ishes-one-reviewed-revision-chromium/real-revision-history.png)
- [冻结查询与拓扑摘要](../../artifacts/planv2-e2e/p2-selfmonitor-20260926-k/selfmonitor-summary.json)、[完整取数快照](../../artifacts/planv2-e2e/p2-selfmonitor-20260926-k/planv2-selfmonitor.json)
- [清理与健康核对](../../artifacts/planv2-e2e/p2-selfmonitor-20260926-k/cleanup-summary.json)、[外部 OpenSandbox 保留核对](../../artifacts/planv2-shared-opensandbox/preservation-result.json)

本轮追加范围的关闭清单：

- [x] Receiver 发行清单、目录与来源身份接通。
- [x] OTel/OTLP 应用 HTTP→Query 自监控及隐私回归。
- [x] 两套原生 SDK 的真实接收、落库、来源隔离和错误样本。
- [x] APM 汇总列、来源分块布局和正式页面 8/8 验收。
- [x] 草稿审计竞争修复通过真实发布流程。
- [x] 临时资源清理与外部依赖共存最终核对。

最终核对：本轮 Namespace（含测试 `argus-telemetry`）均已删除，专用镜像和测试拥有的 CRD 已清理；Kubernetes `/readyz` 为 `ok`，现存 10 个 PVC 均 Bound，40 个 Running Pod 均 Ready。外部 `judex-poc-d15` OpenSandbox 控制器及三份 CRD 的 UID、Spec、标签与 Helm 所有者均与测试前一致，控制器可用副本为 1；未接管、升级或删除外部安装。

以上为 2026-09-26 的阶段性结果。后续 Workspace/PVC 故障组合和受限规模验证已完成，Task 01/02 当前非模型完成状态及实际模型暂缓范围见 [2026-09-28 验收结论](./completion-review-20260928.md)；[早期缺口清单](./remaining-work-20260925.md) 仅保留历史状态。

## 两条验证路径

1. **实际 Argus 服务链路**：`argus-server` 的真实 HTTP 请求，经 Query gRPC 客户端到 `argus-telemetry-query`，生成有父子关系的 OTel Span，再通过本机 Collector 的 OTLP 入口进入既有 Gateway→Ingest→Kafka→Writer→ClickHouse 链路。
2. **原生 SDK 自检**：测试专用 `argus-telemetry-e2e` 使用 GO2Sky 1.5.0 和 Jaeger Go 2.30.0 对本机 Argus 执行真实、只读的 HTTP 检查，发送原生 SkyWalking gRPC 和 Jaeger Thrift HTTP。服务名称明确为 `argus-selfcheck-skywalking`、`argus-selfcheck-jaeger`；不是把手造遥测数据当作实际生产服务统计。测试等待 SkyWalking 上游流式 ACK 与 Jaeger 原生 Thrift HTTP 成功响应，再检查发布查询中的真实落库结果。

原生 SDK 自检的 Entry/Exit Span 表示检查流程；Argus 服务内部的 HTTP/gRPC Span 仍在 OTLP 来源。两者按来源分别组织，不按服务名称或 Trace 名称自动拼接成一条跨来源链路。

## 配置与身份

- Collector 继续锁定 `0.133.0`，三个平台的 OCB 清单加入 `skywalking`、`jaeger` 组件，Catalog Revision 升为 3。
- 新增 `skywalking-receiver`、`jaeger-receiver` Trace profiles。监听本机回环的 `11800`、`14250`、`14268`，不开放 UDP 或公网明文入口。Gateway 转发继续使用既有 mTLS 和来源注册，不信任 SDK 自报企业或资源身份。
- 原生来源分别进入独立 pipeline，接收后、batch 前盖上服务端注册的 Source ID/Revision。Dashboard Catalog、构建器和执行端使用同一来源能力定义。
- 正式程序仅在设置 `ARGUS_SELF_TRACE_ENDPOINT` 后启用导出，例如本机 Collector 的 `http://127.0.0.1:4318`。默认采样率 0.1；可通过 `ARGUS_SELF_TRACE_SAMPLE_RATIO` 设置 0–1。本次 E2E 使用 1，以便验证受控请求，统计仍标记为已接收样本。
- `ARGUS_SELF_TRACE_ENVIRONMENT` 填入 `deployment.environment.name`；实例取服务运行的主机/Pod 名称。HTTP 使用注册路由模板及标准方法名，不记录 URL 查询、请求体、Cookie、Authorization 或用户对象 ID；gRPC 仅记录固定方法和状态码。
- 导出器只接受经过审核的 `argus.http` / `argus.grpc` instrumentation scope，避免全局 Provider 顺带导出 PromQL 等依赖库的内部查询上下文。
- 运维自监控应送至运维专用企业/资源，不按请求中自报信息改变归属。临时 E2E 只使用自己创建的企业、Cluster、Collector、证书和 Namespace，不开启长期正式环境的全局导出。
- 测试 Sidecar 复用该临时 Cluster 已注册的来源配置与证书，移除身份轮换 Extension，避免同一身份被多个进程竞争轮换。实际安装的主 Collector 继续管理身份。Sidecar 配置和 Secret 随拥有它们的测试 Namespace 一并清理。

## 正式页面与验收入口

本次接入能力按实际测试区分：

| 路径 | 实际输入 | 已覆盖 / 待覆盖 |
| --- | --- | --- |
| OTel / OTLP HTTP | Argus HTTP 服务与 Telemetry Query gRPC | 真实应用父子关系、服务拓扑、样本统计；不是全部服务自动埋点 |
| SkyWalking gRPC | GO2Sky 1.5.0，入口/出口 Span | 原生协议 ACK、转换落库、实例、受控错误和来源隔离；其他语言 SDK、多 reference 特殊场景仍需各自验证 |
| Jaeger Thrift HTTP | Jaeger Go 2.30.0，入口/出口 Span | 原生 HTTP ACK、转换落库、实例、受控错误和来源隔离；不是所有旧客户端兼容证明 |
| Jaeger gRPC | Receiver 配置已启用 | 配置验证覆盖；本次原生 SDK 实际发送走 Thrift HTTP，未单独完成 gRPC 客户端验收 |
| JVM Metrics / Profiling / 远程采样 | 未启用 | 本轮没有对应实现或通过声明 |

两套原生 SDK 各请求一次正常地址和一次受控 404；图中的错误样本用于证明错误路径可见，不代表 Argus 正常业务发生相同比例的故障。

新增 `argus-dev e2e run --suite planv2`，复用 M2/M3/M4/P5-native/M7/M10 的环境和 API 初始化/校验，再执行自监控和正式 Dashboard 页面。专项浏览器只运行 `planv2-*` 用例；基础门户页面保留在各自 suite，B/C 轮已各通过其 23 项真实浏览器检查。

验收仪表盘为三个来源分别配置服务、实例、接口、RED、拓扑及 Trace 列表，共 18 张图；标准下钻与统计图一起发布。Environment 变量只被 OTLP 的六张图引用，原生 SDK 图保持自己的条件和数据。

界面按来源分块，服务与 Trace 总览占十二列，其他图成对展示，并增加图卡高度。共享 APM 表格突出服务/实例/接口、请求样本、错误样本、样本错误率和平均/P95 耗时，使用双语字段、百分比与毫秒格式；不再默认铺开时间窗、空分组字段和内部 ID。缺失统计保持“—”，原始结果及下钻身份保留。

浏览器使用正式镜像、real API、真实 MFA 登录，覆盖中英文×深浅色的数据展示、查询新鲜度、来源隔离、已发布下钻、变量依赖、草稿刷新恢复、样本校验、一次确认发布和版本历史。协议/页面通过不代表已完成真实模型或 Workspace 故障验收。

本机没有默认 nginx IngressClass 时使用独立测试入口：

```powershell
$env:ARGUS_E2E_ISOLATED_INGRESS='1'
go run ./cmd/argus-dev e2e run --suite planv2 --run-id <unique-run-id>
```

## 验证记录

当前已通过新增自监控隐私/父子传播测试、构建器/来源契约测试、Collector 组件清单检查和 SDK 测试程序编译。Linux amd64、Linux arm64 和 Windows amd64 Collector 发行物已重新构建；这不替代各平台运行验证。

`p2-selfmonitor-20260925-a` 在部署预检阶段因缺少默认 nginx IngressClass 失败，未创建业务测试 Namespace。`b` 轮使用独立 Ingress，通过 23 项真实浏览器及 M10 前置验收，在原生 SDK 接收判定超时：SkyWalking Receiver 0.133.0 不提供夹具原先假定的 obsreport accepted-spans 指标。已改为协议 ACK，并补成功/失败回归；`b` 轮资源已清理。`c` 轮原生 SDK 协议 ACK 已通过，随后因专项夹具误加 `/enterprise` 路径前缀而在创建草稿时收到 404；正式 Dashboard API 路径位于 `/api/v1/dashboard-drafts`。已修正并增加实际 OpenAPI 请求体/路径契约回归。后续各轮进展如下。

## 上游依据

- [SkyWalking Receiver 0.133.0](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.133.0/receiver/skywalkingreceiver/README.md)
- [Jaeger Receiver 0.133.0](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.133.0/receiver/jaegerreceiver/README.md)
- [GO2Sky 1.5.0 SDK](https://github.com/SkyAPM/go2sky/tree/v1.5.0)

Jaeger 旧版 Go SDK 仅用于原生协议验收夹具，生产服务继续使用 OTel SDK。为解决其间接测试依赖拉入旧 Google Cloud 单体模块与拆分后的 metadata 模块冲突，模块图将 Cloud 根模块提升到已拆分版本；未改变业务云服务接入。


### 后续验证进展

- d 轮已能创建发布并查询，暴露出 Query 容器名为 `telemetry` 导致夹具未注入埋点环境变量。已按程序入口识别容器并校验必须唯一匹配，补回归。
- e 轮 18 张图真实取数通过，来源行 ID 校验通过；实际观察到 API→Query 的服务拓扑、两个原生 SDK 来源及受控错误。四个语言/主题展示、已发布下钻和变量依赖用例通过；发布流程用例因未打开“基本信息”抽屉而失败，已修复测试路径及保存响应等待。另补齐发布预览的描述字段。
- f 轮期间其他项目安装了 OpenSandbox，导致 CRD 已有外部 Helm 所有者，安装器拒绝接管。已补兼容共享依赖发现：仅当三份 CRD、版本、约束、外部控制器版本/镜像/就绪状态符合要求时复用；允许无破坏性的可选字段和说明扩展，拒绝新必填项或隐式默认值。发现只执行 GET，不修改外部所有权或部署。单测及本机只读兼容性检查通过。
- 上述为各轮中间结果；最终通过范围以本文件顶部 k 轮记录为准，其他 PlanV2 退出条件继续独立跟踪。


### 发布与共存回归收尾

h 轮成功复用外部 OpenSandbox 控制器，完成真实安装、自监控取数、来源隔离及四种界面展示/下钻验证。前三个发布用例已实际完成保存、恢复、样本、确认发布，在历史版本文字定位上失败；已按真实 DOM 修正定位。第四个编辑入口捕获 PostgreSQL `LockAuditChain` 的 40001 并发更新：草稿写事务与查询审计争用，被错误映射成编辑版本冲突。现对仅包含数据库操作的草稿事务进行最多 5 次有界重试，每次重新校验权限、生命周期和版本；真正的版本冲突/唯一约束不重试，重试耗尽作为暂时不可用。对应回归通过。

i 轮滚动更新后 Query 连接超时，定位为测试 Sidecar 的 Collector 专用 NetworkPolicy 意外隔离了原本未限制出口的业务 Pod。已改为仅在相应方向原本已被策略隔离时补充 Collector 放行规则，不为未隔离 Pod 新建隔离；Gateway 入口规则选择具体部署。新增回归确认不改变原有隔离状态，并在滚动更新后用只读查询验证就绪，仅对暂时依赖不可用作有界等待。

j 轮网络和来源验证通过，浏览器 7/8 通过；第一组编辑入口仍遇到连续五次审计链序列化冲突，另外三组已完成发布与历史验证。PostgreSQL 日志确认冲突均在 `LockAuditChain`，重试窗口仅约 150ms，不足以覆盖多图查询的审计写入。最终改为最多八次、20ms 指数退避（累计等待最多 2.54s），保留取消及真实版本冲突，不降低事务隔离；草稿锁查询也不再把数据库冲突误映射为不存在。新增回归验证持续审计竞争结束后仍识别实际版本冲突。

`p2-selfmonitor-20260926-k` 已通过该修复及 APM 展示调整的 8/8 real UI；共享 UI 61 项测试、相关 Go、类型、样式及 i18n 检查通过。
