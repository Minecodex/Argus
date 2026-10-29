# B05b Kafka 查看记录

2026-10-29：用户要求“下一阶段”，本批已完成正常停采、卸载、退役和资源回收；两个容器、两个卷和网络无残留。账号与历史保留，该指令按切换记录，不补写明确人工验收通过。见[切换记录](../../../artifacts/plan9/p9-b05b-20261028-01/transition.json)、[清理记录](../../../artifacts/plan9/p9-b05b-20261028-01/cleanup.json)。当前批次为 [B05c](./B05c-review.md)，以下保留原交付快照。

运行 `p9-b05b-20261028-01`。用户指定“下一批”后，B05a 已正常卸载并清理，企业账号、共享模板和历史保留。本批为独立 Kafka 4.2.1 单节点 KRaft，当前平台版本 `local-20261028-b05b-v1`：9 个面板、6 项真实浏览器 E2E、15 项翻译检查和 20 项平台健康检查通过，正式 6 个 PVC 与 2 个 OpenSandbox Pod 保留。当前两组均有 1 个成员、正常消费提交，样本新鲜，无本批活跃告警；人工验收待用户进行。

## 查看顺序

1. 打开[监控列表](https://argus.dev/monitoring)，搜索 `B05b`，检查 `B05b-Kafka` 的采集配置、SASL 凭据已配置状态和样本新鲜度。
2. 打开默认仪表盘：9 个面板包含 Broker、Topic 分区/副本、同步副本、最新位点、已提交消费位点、分区 lag、组成员数和消费组总 lag。
3. 对比 `p9-work` 与 `p9-control` 两个 Topic；各 3 分区、1 副本，对应固定组 `p9-work-consumer` 和 `p9-control-consumer`。正常时每组 1 个成员，每 2 秒真实发布一条并实际处理、同步提交。
4. 在[告警历史](https://argus.dev/alerts?tab=history)搜索 `B05b`，查看“consumer lag”和“consumer group empty”的实际触发/恢复及时间线。

建议历史窗口：**2026-10-28 22:43–22:48（Asia/Shanghai）**。分区 lag 与组总 lag 分开展示；总积压阈值不要与某一个分区的值直接比较。

## 已实际执行的场景

| 场景 | 实测结果 |
| --- | --- |
| 首次消费位点 | 每分区写入真实记录，由实际消费者处理后同步提交；未用 reset-offsets 或注入提交位点 |
| 权限隔离 | 只读 monitor 创建 Topic 被拒绝且未留下 Topic；work 账号订阅 control Topic 被 Broker 明确拒绝 |
| 消费暂停积压 | 调用 pause 后继续心跳，组成员保持 1，已提交位点冻结；确认写入 240 条、每分区 80 条，采集到总 lag 259 并触发告警 |
| 积压恢复 | resume 后处理消息、真实提交推进，lag 降低并恢复告警；Broker 的 log-end 与 committed 差值与监控对照 |
| 消费者退出 | 停止 work 生产者和消费者，Broker 报告组成员为 0，触发第二条告警；重新加入并消费后恢复 |
| 健康对照 | control 组在两次故障中继续生产消费，未混入 work 的指标和告警 |

冻结阈值：work 总 lag > 100、恢复 <= 5；work members < 1、恢复 >= 1。每 10 秒评估，触发与恢复均持续 20 秒。只配置平台内部告警，没有外部通知接收人。

正常状态保留低频生产、消费和提交，暂停与停止故障均有 180 秒恢复保护。每进程最长 8 小时，生产者还受 backlog 数量检查约束，达到 1000 停止继续生产；Topic 的保留大小、段大小和时间另有界。到期只停客户端，服务和监控保留，不自动进入 B05c。客户端到期退出后，成员数告警可能再次触发；需要继续看实时业务变化时，可按运行包重新启动有界负载。

本批仅声明单节点协议、元数据和消费组指标验证；不包含多 Broker 高可用、JVM/JMX 或 TLS 验收。Collector 的原生 Kafka receiver 使用限定的 SASL/PLAIN 监控身份，测试 Kafka 与正式平台 Kafka 完全独立。

## 证据与运行流程

- [运行包、权限和清理命令](../runtime/B05b/runbook.md)
- [真实资源 ID](../../../artifacts/plan9/p9-b05b-20261028-01/resource-map.json)
- [真实首次消费及权限检查](../../../artifacts/plan9/p9-b05b-20261028-01/protocol-preflight.json)
- [实际触发和恢复](../../../artifacts/plan9/p9-b05b-20261028-01/scenario-checks.json)
- [暂停前位点](../../../artifacts/plan9/p9-b05b-20261028-01/lag-before-offsets.json)
- [真实写入分区和 offset](../../../artifacts/plan9/p9-b05b-20261028-01/lag-confirmed-publish.json)
- [lag 与 Broker 位点对照](../../../artifacts/plan9/p9-b05b-20261028-01/lag-native-comparison.json)
- [恢复后位点](../../../artifacts/plan9/p9-b05b-20261028-01/lag-after-offsets.json)
- [独立停采与恢复](../../../artifacts/plan9/p9-b05b-20261028-01/lifecycle-checks.json)
- [最终指标与 Kafka CLI 核验](../../../artifacts/plan9/p9-b05b-20261028-01/monitoring-verification.json)
- [真实浏览器检查](../../../artifacts/plan9/p9-b05b-20261028-01/browser-review.json)
- [最终检查汇总](../../../artifacts/plan9/p9-b05b-20261028-01/checks.json)

B03-D01 的既有诊断问题仍保留于原记录，不因本批切换补写为已修复。

## 当前对象入口

- [Kafka 默认仪表盘](https://argus.dev/dashboards/37f7ec26-123d-5ced-8544-d1f7c2291583?resource=973d54c1-cb98-5fd3-8c95-8e62cd489d67&resource_type=monitoring_target)
- [采集主机](https://argus.dev/hosts/01a182f1-a2b6-7a56-b84e-58206d4065d3)
- [B05b Kafka consumer lag 历史](https://argus.dev/alerts?tab=history&policy_id=2072c231-9691-5283-ad51-3e5885a277b6)
- [B05b Kafka consumer group empty 历史](https://argus.dev/alerts?tab=history&policy_id=dcbdcc3e-a976-52f6-9287-6886ac5f447b)
