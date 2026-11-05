# B07b OTLP gRPC 查看与运行记录

B07b 已于 2026-11-04 17:32（北京时间）退役。应用和仪表盘已归档，接入点与 Key 已停用，专属容器、卷和网络已回收。历史记录保留。当前查看入口见 [B07c](./B07c-review.md)。

运行 `p9-b07b-20261104-01`。B07a 应用与仪表盘已归档，旧接入点已停用，密钥已撤销，专属容器、卷和网络均已回收。旧批投递事件按清理流程确认，事件历史保留。

## 平台入口

- [B07b 应用性能与日志](https://argus.dev/applications/06e31ae5-fc14-5953-99c1-cb2586a3036b)
- [B07b 应用仪表盘](https://argus.dev/dashboards/585b3b61-f2fb-45fb-982f-c6aa3b18169f?resource=06e31ae5-fc14-5953-99c1-cb2586a3036b&resource_type=application)
- [B07b Redis 默认仪表盘](https://argus.dev/dashboards/6f0cd210-ab49-5d65-be71-f63eb4535beb?resource=543b5335-8316-5eb8-a9b7-32a2c1155902&resource_type=monitoring_target)
- [错误率告警历史](https://argus.dev/alerts?tab=history&policy_id=cf728b8d-b7b0-5861-8ae2-bc620f972d15)
- [延迟告警历史](https://argus.dev/alerts?tab=history&policy_id=f4dc3df4-80f3-5928-b761-b2fb399381fb)

## 场景与监控

业务链路仍使用 `orders-gateway` → `orders-worker` → Redis。两个真实服务通过 Go SDK 向 Collector 的 `grpcs://127.0.0.1:24317` 上报指标、日志和 Trace。两服务的三个 Export RPC 都取得了响应，记录了 TLS 1.3、`h2` 和证书链校验结果。

应用详情包含服务、实例、接口、请求与耗时、拓扑、Trace 和日志。应用仪表盘 10 个面板，Redis 默认仪表盘 11 个面板。应用图的 P95 来自 SDK 直方图插值。日志可按同一 request ID 查找两个服务的记录，并通过页面关联查询打开对应 Trace。

错误场景由 worker 实际返回 HTTP 503。错误率规则使用 1 分钟窗口，触发阈值为 25%，恢复阈值为 5%。依赖等待场景实际执行 Redis `BLPOP`，延迟规则使用入口请求平均耗时，触发阈值为 300 ms，恢复阈值为 150 ms。两规则每 10 秒评估，触发与恢复均需持续 20 秒。另保留真实 500 ms 服务延迟收据。

## 验收结果

场景窗口为 **2026-11-04 15:44–15:49（北京时间）**。错误率告警在 15:45:20 触发，15:46:40 恢复，场景观测值为 65.45%。延迟告警在 15:47:50 触发，15:48:50 恢复，场景观测值为 616.12 ms，Trace 中 Redis 等待为 602.76 ms。两个场景各自核对了两个服务的真实日志。三次服务延迟请求返回 200，耗时约 502–503 ms。

6 项真实浏览器检查通过，覆盖中英、明暗、日志关联 Trace、仪表盘、告警时间线和两种桌面宽度。接入点在 15:49 停用后，两条规则产生 `no_data` 健康事件（B07b-L01）。恢复后，两服务再次收到三个 gRPC Export 的响应，健康事件也已恢复。最终两个业务规则正常，本批活跃告警数为 0。13 个平台 Deployment 的身份、配置代数及镜像引用核对一致，6 个 PVC 绑定核对一致。应用和负载已留在平台等待人工查看。

部分早期样本的 request ID 使用共享驱动原有的 `b07a-` 前缀，应用和 Trace 均归属 B07b。负载脚本已改为从运行主机读取批次，新的请求使用 `b07b-`。

## 验收证据

- [三信号 gRPC Export 与 TLS 回执](../../../artifacts/plan9/p9-b07b-20261104-01/grpc-export-receipts.json)
- [故障场景与恢复](../../../artifacts/plan9/p9-b07b-20261104-01/scenario-checks.json)
- [接入点停用与恢复](../../../artifacts/plan9/p9-b07b-20261104-01/lifecycle-checks.json)
- [实际浏览器结果](../../../artifacts/plan9/p9-b07b-20261104-01/browser-review.json)
- [B07a 清理核对](../../../artifacts/plan9/p9-b07b-20261104-01/predecessor-cleanup-verification.json)
- [最终状态和事件时间](../../../artifacts/plan9/p9-b07b-20261104-01/final-state.json)
- [验收汇总](../../../artifacts/plan9/p9-b07b-20261104-01/checks.json)

## 执行与清理

共享运行包位于 [B07 运行包](../runtime/B07a/runbook.md)。`manage.py`、`integration.py`、`browser.py` 和 `retire.py` 选择 `--batch B07b`。`up` 校验 B07a 清理完成后启动，SSH 使用本机 22342，SDK 走容器内 24317。业务负载每轮间隔 0.5 秒，最多运行 8 小时或 60,000 次请求。故障最长 180 秒，应用 Key 有效 24 小时。

用户要求下一批时，使用 `retire.py --batch B07b` 和正常企业会话清理本批。接入点停用会关闭监听，应用告警仍按既有窗口和无数据策略评估。
