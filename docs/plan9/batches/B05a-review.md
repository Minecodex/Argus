# B05a RabbitMQ 查看记录

2026-10-28：用户要求“下一批”，本批已完成正常停采、卸载、退役与资源回收；2 容器、2 卷和网络无残留，历史保留。该指令按批次切换记录，不补写明确人工验收通过。详见[切换记录](../../../artifacts/plan9/p9-b05a-20261028-01/transition.json)、[清理记录](../../../artifacts/plan9/p9-b05a-20261028-01/cleanup.json)。当前批次为 [B05b](./B05b-review.md)，下文保留原交付快照。

运行 `p9-b05a-20261028-01`。B04 已经用户确认通过并清理，企业账号、共享评估身份和历史保留。本批实际 RabbitMQ 版本为 4.4.0，人工验收待用户进行。当前平台版本 `local-20261028-b05a-v1`：9 个面板、6 项真实浏览器 E2E、15 项翻译检查通过；平台健康 20/20，正式 6 个 PVC 与 2 个 OpenSandbox Pod 保留。当前样本新鲜、Collector 已收敛、没有本批活跃告警。

## 查看顺序

1. 打开[监控列表](https://argus.dev/monitoring)，搜索 `B05a`，检查 `B05a-RabbitMQ` 的正常采集、生效配置和新鲜样本。
2. 从目标行进入默认仪表盘：9 个面板展示节点内存、磁盘、文件描述符、运行时长、队列消息数、消费者数，以及发布、投递、确认速率。
3. 两个 vhost `p9-work`、`p9-control` 各有一个同名 `jobs` 队列。通过图例和 Tooltip 对比 vhost；正常状态各有 1 个消费者，每 2 秒真实发布一条并由消费者手动确认。
4. 在[告警历史](https://argus.dev/alerts?tab=history)搜索 `B05a`，查看“ready backlog”和“awaiting acknowledgement”两种告警的趋势、触发、恢复及可读时间线。

建议历史窗口为 **2026-10-28 20:38–20:43（Asia/Shanghai）**。当前压力已解除，队列已正常消费，不保留未确认消息或停用消费者。

## 实际发生的场景

| 场景 | 实际操作与结果 |
| --- | --- |
| 正常 AMQP | 两组均使用 publisher confirms、mandatory 路由及手动 ACK，初始化各 4 条消息实际确认发布并消费 |
| 权限隔离 | monitor 能读取管理统计但不能创建队列（401）；work 账号连接 control vhost 被拒绝 |
| 待消费积压 | 停止 work 消费者，确认发布 250 条；连同低频负载观测到 261 条 ready，触发告警；恢复消费者并实际确认排空后告警恢复 |
| 暂缓确认 | work 消费者接收消息但暂缓 ACK，prefetch=10，实际未确认数为 10；触发告警后解除标记，在原连接上逐条确认并恢复 |
| 健康对照 | 两次故障期间 control vhost 继续实际生产消费，未混入 work 的队列值或告警 |
| 独立停采 | 只停用 RabbitMQ 采集，两条策略暂停评估；主机固定采集继续新鲜，恢复后样本和评估重新正常 |

阈值在故障前冻结：ready > 100、恢复 <= 5；unacknowledged > 5、恢复为 0。每 10 秒评估，触发和恢复均持续 20 秒。没有配置外部通知收件人。

恢复采集在 20:42:38 发起；20:42:40–20:42:50 两条策略各产生过一次查询健康 `no_data` 提示，随后自动恢复。这是新样本到达前按 `no_data_behavior=report` 记录的短暂状态，历史保留，不与两次业务积压告警混为一谈。

每队列最多 500 条、1 MiB，超限拒绝发布，消息 TTL 15 分钟。暂停消费和暂缓 ACK 均有 180 秒恢复保护；正常低频生产/消费进程有 8 小时上限，到期只停客户端，保留服务与监控。不会自动进入 B05b。

本批为单节点 Broker，专用网络内明文 AMQP/Management，不包含多节点容灾、TLS 或 Kafka 场景。平台取数使用原生 RabbitMQ Receiver，没有向 Argus 数据库或遥测入口写预制指标。

## 证据与运行包

- [Compose、协议客户端与清理流程](../runtime/B05a/runbook.md)
- [真实资源与 ID](../../../artifacts/plan9/p9-b05a-20261028-01/resource-map.json)
- [初始化及权限验证](../../../artifacts/plan9/p9-b05a-20261028-01/protocol-preflight.json)
- [告警触发和恢复](../../../artifacts/plan9/p9-b05a-20261028-01/scenario-checks.json)
- [ready 原生状态对照](../../../artifacts/plan9/p9-b05a-20261028-01/backlog-native-comparison.json)
- [unacknowledged 原生状态对照](../../../artifacts/plan9/p9-b05a-20261028-01/unacked-native-comparison.json)
- [停采与恢复](../../../artifacts/plan9/p9-b05a-20261028-01/lifecycle-checks.json)
- [最终指标核验](../../../artifacts/plan9/p9-b05a-20261028-01/monitoring-verification.json)
- [真实浏览器检查](../../../artifacts/plan9/p9-b05a-20261028-01/browser-review.json)
- [最终检查汇总](../../../artifacts/plan9/p9-b05a-20261028-01/checks.json)

B03-D01 的历史诊断问题继续保留在原记录中，本批通过不代表该问题已关闭。

## 当前对象入口

- [RabbitMQ 默认仪表盘](https://argus.dev/dashboards/73542eae-5e0a-5aa3-8af8-b16ceba53c78?resource=652332a9-5600-5038-8afd-a1188755ee9e&resource_type=monitoring_target)
- [采集主机](https://argus.dev/hosts/01a1827e-7354-7b40-9c8d-661f2b9308d2)
- [B05a RabbitMQ ready backlog 历史](https://argus.dev/alerts?tab=history&policy_id=a1132066-e0c4-5b6d-bd7c-c9e81473c336)
- [B05a RabbitMQ awaiting acknowledgement 历史](https://argus.dev/alerts?tab=history&policy_id=8459479e-713c-57bf-98ba-ef77ac7249af)
