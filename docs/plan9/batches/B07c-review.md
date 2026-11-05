# B07c Jaeger gRPC 查看与运行记录

运行 `p9-b07c-20261104-01`。B07b 已于 2026-11-04 17:32（北京时间）退役。旧应用和专属仪表盘已归档，旧接入点已停用、密钥已撤销，专属容器、卷、网络均已回收。告警历史保留。

## 平台入口

- [B07c 应用性能与关联日志](https://argus.dev/applications/71d76d35-8fcb-5058-9e9d-9779708b087d)
- [B07c 应用仪表盘](https://argus.dev/dashboards/3bbae6b5-cbde-487e-81ee-58062c9c0793?resource=71d76d35-8fcb-5058-9e9d-9779708b087d&resource_type=application)
- [B07c Redis 默认仪表盘](https://argus.dev/dashboards/6f0cd210-ab49-5d65-be71-f63eb4535beb?resource=f45fdbcb-4a95-5697-b4ce-e0c34fea1a7f&resource_type=monitoring_target)
- [HTTP 错误率告警历史](https://argus.dev/alerts?tab=history&policy_id=37f211ce-ea2b-591c-8a7c-5cf3e703f5f4)
- [依赖延迟告警历史](https://argus.dev/alerts?tab=history&policy_id=0e583f67-b762-5e17-9345-47e672486923)

## 应用详情复验

本机正式版本更新为 `local-20261104-b07c-apm-v2`。应用详情包含「概览、服务、调用链、日志、SDK 接入」五个页签。概览显示请求与耗时和服务拓扑，服务页从服务清单进入运行实例和接口。观测页签共用仪表盘的时间选择器，固定起止时间在切换页签和刷新后保留。

同一实际一小时窗口（北京时间 20:23:54–21:23:54）返回 120 个趋势时间桶记录、2 个服务节点和 1 条依赖边，以及 7,173 条调用链。调用链前两页各 50 条，Trace ID 无重叠。列表查询只取摘要，详情仍加载 Span、瀑布图和图完整性。该窗口的拓扑返回了节点和依赖，同时保留 `incomplete_observations` 覆盖提示。已验证查询超限时的提示及真实日志到 Trace 的关联。

6 项正式门户浏览器检查通过，覆盖中英、明暗、固定窗口、实际翻页、关联日志、仪表盘和告警。20 项平台健康检查通过，6 个 PVC 绑定核对一致。正式 Collector 签名包沿用原锁定版本。

- [一小时真实查询结果](../../../artifacts/plan9/p9-b07c-20261104-01/application-repair/apm-final.json)
- [最终浏览器结果](../../../artifacts/plan9/p9-b07c-20261104-01/application-repair/browser-final.json)
- [发布与健康核对](../../../artifacts/plan9/p9-b07c-20261104-01/application-repair/deployment-final.json)

## 实际场景

本批沿用 `orders-gateway` → `orders-worker` → Redis 的业务链路。两个服务从真实请求创建 SDK Span，并用 Jaeger 原生 gRPC `PostSpans` 上报到 24250。同一应用的指标和日志通过独立 OTLP gRPC 接入点 24317 上报。两个接入点分别使用平台签发的 Key。两个服务的三类 RPC 均收到响应，且记录了 TLS 1.3、`h2` 和证书链校验结果。应用日志和 Jaeger Trace 可按同一请求和 Trace ID 关联。

应用仪表盘 10 个面板，Redis 默认仪表盘 11 个面板。应用详情包含服务、实例、接口、请求与耗时、拓扑、Trace 和日志。P95 取自 SDK 直方图插值。

## 告警与复验

以下时间均为 2026-11-04 北京时间。两条规则每 10 秒评估，触发和恢复各需持续 20 秒。

| 实际故障 | 告警条件与恢复阈值 | 触发 | 恢复 |
| --- | --- | --- | --- |
| worker 返回 HTTP 503 | 1 分钟错误率 > 25%，恢复 ≤ 5% | 17:39:20 | 17:40:40 |
| Redis `BLPOP` 等待 | 1 分钟平均请求耗时 > 300 ms，恢复 ≤ 150 ms | 17:41:50 | 17:42:50 |

场景查询实测错误率为 76.22%，延迟场景平均请求耗时为 628.78 ms，对应 Redis 等待 Span 为 677.99 ms。错误 Trace 有 3 个错误 Span。另执行了 3 次真实服务延迟请求，均返回 HTTP 200，耗时 503–505 ms。

6 项浏览器检查覆盖中英、明暗、两种桌面宽度、日志关联 Trace、两套仪表盘和告警时间线。短时停用 Jaeger 后确认 24250 关闭，OTLP 接入点保持生效，Redis 样本继续刷新。恢复后用新 SDK 进程的回执和实时查询核对三信号。停采演练先停止两个 SDK 服务，因此指标查询曾出现 `no_data`。历史保留了 2 条评估健康事件且全部恢复。最终两条业务规则评估正常，本批活跃告警数为 0。本次核对了 13 个平台 Deployment 和 6 个 PVC 绑定。

## 执行与证据

使用 [B07 共享运行包](../runtime/B07a/runbook.md)，对 `manage.py`、`integration.py`、`browser.py` 和 `retire.py` 传入 `--batch B07c`。SSH 使用宿主机 loopback 22343。环境保留供查看，自动负载每轮间隔 0.5 秒，每次运行最多 8 小时或 60,000 次请求。受控故障最长 180 秒，接入 Key 有效 24 小时。

- [两个服务的 gRPC 和 TLS 回执](../../../artifacts/plan9/p9-b07c-20261104-01/grpc-export-receipts.json)
- [实际故障与恢复](../../../artifacts/plan9/p9-b07c-20261104-01/scenario-checks.json)
- [Jaeger 停用和恢复](../../../artifacts/plan9/p9-b07c-20261104-01/lifecycle-checks.json)
- [实际浏览器结果](../../../artifacts/plan9/p9-b07c-20261104-01/browser-review.json)
- [B07b 清理核对](../../../artifacts/plan9/p9-b07c-20261104-01/predecessor-cleanup-verification.json)
- [最终状态和事件时间](../../../artifacts/plan9/p9-b07c-20261104-01/final-state.json)
- [验收汇总](../../../artifacts/plan9/p9-b07c-20261104-01/checks.json)
