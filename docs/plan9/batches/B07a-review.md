# B07a OTLP HTTP 应用查看记录

2026-11-04，按用户指令完成 B07a 清理并切换至 [B07b](./B07b-review.md)。以下内容保留当时的验收结果。

2026-11-04，运行 `p9-b07a-20261104-01`。基于当前 `be89e55` / Plan11 和原 `local-20261102-plan11-review` 更新为 `local-20261104-b07a-v3`，未回滚用户的新实现。B06d 经正常卸载、权限收尾和所有权检查后清理，原账号、模型配置、业务数据与历史保留。

两个真实进程 `orders-gateway` → `orders-worker` 实际经 HTTP 调用，工作服务执行 Redis SET/GET；真实 Go SDK 经 TLS 1.3/应用 Key 上报指标、日志和 Trace。应用和两个服务通过正常 API 与实际 SDK 发现生成，没有直接写 Argus 数据库、手工造遥测或调用模型。

应用 10 个面板、Redis 11 个面板及应用七视图已核验；六项真实门户测试覆盖中英、明暗、日志关联 Trace、真实告警历史和 1280/1920 桌面宽度。Go 查询/仪表盘/Collector 配置回归、Web 构建和 20 项平台健康检查通过。178 条原保护业务记录前后无差异，6 个 PVC/PV 身份保留。当前正常请求和监控持续运行，人工验收待用户进行。

## 查看入口

- [B07a 应用与性能视图](https://argus.dev/applications/93ed96ba-55a7-56be-9528-558457152aa1)
- [B07a 应用仪表盘](https://argus.dev/dashboards/0271a6f6-731c-4902-ae5c-fc089020722c?resource=93ed96ba-55a7-56be-9528-558457152aa1&resource_type=application)
- [Redis 默认仪表盘](https://argus.dev/dashboards/6f0cd210-ab49-5d65-be71-f63eb4535beb?resource=eae25e57-c51a-54ca-abda-bebcc1a8bdc5&resource_type=monitoring_target)
- [B07a HTTP error ratio high 历史](https://argus.dev/alerts?tab=history&policy_id=e2cc20de-d324-5a00-96b2-a62cc8e5187f)
- [B07a HTTP dependency latency high 历史](https://argus.dev/alerts?tab=history&policy_id=631af0f4-1b64-5947-997c-3f03b9723701)

在[应用列表](https://argus.dev/applications)搜索 `B07a`；在[监控列表](https://argus.dev/monitoring)查看 `B07a-Redis`；两套仪表盘均已正常发布/绑定。

建议最新场景窗口：**2026-11-04 14:35–14:40（Asia/Shanghai）**。默认应用图显示最近 15 分钟；需要查看更早记录时选最近 1 小时或对应绝对时间。当前故障已恢复，保留低频正常请求。

## 已实际验证

| 场景 | 结果 |
| --- | --- |
| 正常业务 | HTTP 返回 200；gateway/worker 的 request ID、Trace ID 与 Redis 实读值一致 |
| 原生 SDK | 指标包含真实请求计数、错误计数、活动请求、实际耗时直方图、运行时长；日志携带同一 Trace ID |
| 服务与依赖 | 自动发现 2 个服务及实例；服务拓扑有 2 个节点、1 条跨服务边；Redis 通过正式应用资源关联呈现 |
| Trace 与日志 | 普通请求至少 4 个真实 Span，Redis 等待场景增加 BLPOP Span；同一 request ID 可查到两个服务日志，并在页面跳转到对应 Trace |
| 错误率告警 | worker 实际返回 503，gateway 传递；最近 1 分钟错误率实测约 80.55%，超过 25%，恢复到 <= 5% 后关闭事件 |
| 延迟告警 | 实际执行 Redis BLPOP 空列表等待约 0.6 秒；gateway 平均耗时约 625 ms，超过 300 ms，恢复 <= 150 ms 后关闭事件 |
| 服务延迟 | 另实际执行 500 ms 处理延迟，保留 HTTP 收据，未篡改指标值 |
| SDK 入口停用恢复 | 先停止并刷新 SDK 进程，再停接入点；HTTP listener 确认关闭，Redis 仍有新样本；恢复后重新接收到 SDK 数据、服务身份保持稳定 |

错误和延迟阈值在场景前冻结，10 秒评估、触发/恢复持续 20 秒，无外部通知渠道。源于真实 SDK 的错误日志级别可由 `severity_number=17` 和正文 HTTP 503 核对；不能将空闲或未上报对象补成成功值。P95 面板是实际 SDK 直方图的插值估计，APM 的入口 Span 分位数是另一统计口径。

最新两次业务触发/恢复在修正后的 v3 版本重新执行，之前 v2 的真实场景证据保存在 `scenarios-before-query-fix/`。历史中可能看到多次真实测试事件，并非手工生成或重复伪造。

## 对新版本的适配和修正

1. 应用 SDK 来源登记的 `application/v1` 与仪表盘发现的查询能力 `v1` 含义不同。共用来源解析显式接受已授权 application 的 OTLP/Jaeger 已知组合，保留原始版本证据，未知类型/版本仍拒绝。
2. 应用拓扑要读取原始 Span 关系后再聚合。页面从列表的 5,000 行预算改用引擎已有的 50,000 行有界预算；其他视图、超时、扫描和输出字节预算保留。真实页面已覆盖超过 5,000 个 Span 的查询。
3. Redis 命中率公式在一次查询中重复读同一指标。引擎对同一 Querier、相同选择条件复用读取，独立游标不互相影响，跨请求不共享。固定历史窗口 11 个面板结果完全相同，命中率面板扫描量由 79,621,842 降至 38,904,603 字节，未增加仪表盘预算或改公式。

详见[接入与查询修正](../09-b07a-application-source-capability.md)。本次构建保持当前 Plan11 代码及签名 Collector 发行包，实际签名/哈希已校验，未关闭校验或替换固定包。没有进行新的真实模型调用，也未把本次结果扩成 Plan11 模型质量放行。

## 保留的观察与边界

**B07a-D01：更新期间的接收拒绝记录。** 平台仍保留一条 Collector 共享管道事件 `9edc9919-5726-499b-a4c6-46c41c8295e3`。最后观察到拒绝为 14:26:32，计数为 692 条日志尝试、275 个指标点尝试、2,638 个 Span 尝试；最终发送失败计数均为 0。拒绝可能重试，不能据此宣称丢失了这些数据，也不能由零最终失败证明整批完全无丢失。最终观察计数未增加，当前采集及 SDK 新样本正常；记录未删除、未替用户确认。

**B07a-L01：应用 SDK 停用的告警语义。** 当前应用入口停用没有原生 Host/中间件采集的自动 not_evaluated 语义，后台仍依据当前时间窗口与 no_data 策略评估。脚本最初套用原生采集断言而失败，已保留该事实。实际 listener 关闭、原生 Redis 继续采集、SDK 恢复和规则重新正常均已验证，不把它说成“告警自动暂停已通过”。

普通负载每轮间隔 0.5 秒，最多 8 小时/60,000 次；故障每次最多 180 秒。应用 Key 有效期见资源清单（24 小时），后续继续发送需正常轮换。服务、监控和历史在用户要求清理前保留；不会自动进入 B07b。仅本轮 OTLP HTTP 已验证，未据此宣称 OTLP gRPC、Jaeger 或真实模型场景完成。

## 运行与证据

- [运行包、边界与清理](../runtime/B07a/runbook.md)
- [真实资源与 Key 到期时间](../../../artifacts/plan9/p9-b07a-20261104-01/resource-map.json)
- [SDK TLS 连接说明](../../../artifacts/plan9/p9-b07a-20261104-01/sdk-connection.json)
- [实际发现的服务](../../../artifacts/plan9/p9-b07a-20261104-01/discovered-services.json)
- [最新触发/恢复](../../../artifacts/plan9/p9-b07a-20261104-01/scenario-checks.json)
- [最终 HTTP 收据](../../../artifacts/plan9/p9-b07a-20261104-01/final-http-receipt.json)
- [最终 Trace 与日志关联](../../../artifacts/plan9/p9-b07a-20261104-01/final-trace-proof.json)
- [入口停用恢复](../../../artifacts/plan9/p9-b07a-20261104-01/lifecycle-checks.json)
- [同窗口数据和扫描量对照](../../../artifacts/plan9/p9-b07a-20261104-01/selection-memo-live-comparison.json)
- [最新真实浏览器结果](../../../artifacts/plan9/p9-b07a-20261104-01/browser-review.json)
- [原业务与卷保护](../../../artifacts/plan9/p9-b07a-20261104-01/business-final-v3.json)
- [原签名发行包验证](../../../artifacts/plan9/p9-b07a-20261104-01/signed-artifact-validation.json)
- [最终状态与保留事件](../../../artifacts/plan9/p9-b07a-20261104-01/final-state.json)
- [检查汇总](../../../artifacts/plan9/p9-b07a-20261104-01/checks.json)
