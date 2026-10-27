# 原生查询交付、告警契约和完整证据校验

2026-10-22，r50 结束后原 Docker Desktop 的副本、资源及存储身份恢复核验通过，server 1/1，本轮 Calico Namespace 清零。六场真实模型通过：K8s 恢复、独立停采、SDK Key、应用/共享数据库、安全表单和主机仪表盘 Workspace；告警和同会话撤权仍未通过。它们保持原始失败记录，不计入完成。

## 修复与架构边界

- 告警资源类型的权威源为 `api/openapi/components/alerts.yaml`。增加 application/monitoring_target 后重新生成 JSON Schema、OpenAPI 和客户端，不能只修改会被生成器覆盖的副本。新增契约回归对照领域校验，并拒绝未知类型。
- 已停采目标的授权回归使用真实历史样本时间窗和 `/enterprise/metrics/query_range`，不再把 `step_seconds` 发送给 instant 路由。拒绝、授权和撤权仍分别要求 403/200/403。
- 原生指标、日志和 Trace 查询保留完整不可变 Artifact、授权签名及 Presentation 的类型化资源引用。Agent 在提交结果后，用同一个受信 result_ref 自动导入 `queries/<signal>/<tool-call-id>.json`，文件有实际哈希和 source_result_refs。下一轮模型之前必须完成导入；Worker 中断后从已提交事件恢复缺失导入，不重做查询来伪造旧结果。
- 导入复用 Workspace 既有租约、配额、原子写、来源复验和元数据发布。重放核验现有字节及哈希，冲突不覆盖，撤权不能通过改副本洗掉。投影只提供由服务端生成的交付路径及读取要求；模型不能提交文件来源声明。此扩展使用内部交付端口，没有新增存储权威或绕过审批。
- Agent 原生查询目录显式要求现有 workspace.use 权限；不自动授予它。门户查询与后台 ServiceAccount 查询继续使用原领域权限。
- 摄入配额使用类型化错误；gRPC ResourceExhausted 必带 RetryInfo 和配额原因，HTTP 429 带 Retry-After。普通后端错误保持可重试 unavailable/503，Collector 每批五分钟重试上限不变。

## 新验收入口

`api/contracts/plan8-acceptance.json` 固定原矩阵 76 项的执行、预期和允许级别。新增 `argus-dev e2e-plan8 verify --evidence artifacts/<run>`，要求逐项具名断言、相同源码/Collector 指纹、合同摘要、实际起止时间及内容哈希一致的本轮文件。缺文件、低级别、Skip、跨版本及仅包测试成功均不计通过。

完整模式可通过 `--formal-evidence` 引用同一冻结源码的正式证据；不再永久硬编码“阶段未实现”。25 个非暂缓集成家族的精确支持组合、39 个选型结论、12 个内置能力、同组发布与恢复记录仍单独验证。尚无具名证据的条目保持 incomplete；新增校验器本身通过不等于 76 项通过。

## 当前证据

- `artifacts/plan8-r51-pg-contract-source-revocation.log`：真实 PostgreSQL 验证提交后交付屏障、交付失败的持久恢复意图、三类来源撤权，以及真实文件 RPC 的重复导入/孤立文件恢复；计算依赖为受控夹具，尚未抵扣真实 PVC/模型。
- `artifacts/plan8-r51-ingest-quota-product.log` 与同名 `.json`：真实 SDK 经原生 Collector 的请求、真实 mTLS Ingest、Redis/Kafka；受控注入本轮限流状态，gRPC/HTTP 拒绝及恢复通过。不是实际 1200 请求/分钟负载证明，不计作完整 A62。
- `artifacts/plan8-r51-formal-evidence-contracts.log`：76 原始合同一致性、失败/外来/弱级别/缺少引用拒绝，以及占位支持矩阵拒绝。

正式驱动追加 HTTP/TCP/TLS/DNS/ICMP 五个真实目标，复用平台创建、实际 Receiver、Kafka/Writer/Query、默认面板、授权和启停。各服务组互相独立时失败后仍继续，清理失败立即停止；每个失败保留，整轮仍失败。K8s 队列恢复补按 data_type 的积压及发送/失败计数，仍保留原恢复断言，不能用日志到达代替所有队列排空。

r51 仍待完整部署重验；支持矩阵、76 项、最终发布不可据此先报通过。Windows 实机继续按用户决定暂缓，11 个资源和 SkyWalking 原生协议继续暂缓。
