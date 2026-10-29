# B05c ZooKeeper 查看记录

2026-10-29：用户明确要求清理 B05c 后部署 B06a，本批已完成正常停采、卸载、退役和资源回收；三个容器、七个卷和网络无残留。账号与历史保留，切换不补写明确人工验收通过。见[切换记录](../../../artifacts/plan9/p9-b05c-20261029-01/transition.json)、[清理记录](../../../artifacts/plan9/p9-b05c-20261029-01/cleanup.json)。当前批次为 [B06a](./B06a-review.md)，以下保留原交付快照。

运行 `p9-b05c-20261029-01`。用户指定“下一阶段”后，B05b 已正常卸载并清理，企业账号、共享模板和历史保留。本批是两个独立 ZooKeeper 3.9.6 standalone 实例，不构成高可用 ensemble。当前部署版本 `local-20261029-b05c-v1`：两个实例共 12 个面板、6 项真实浏览器 E2E、15 项翻译检查和 20 项平台健康检查通过，正式 6 个 PVC 及 2 个 OpenSandbox Pod 保留。当前样本新鲜、Collector 已收敛、没有本批活跃告警；人工验收待用户进行。

## 查看顺序

1. 打开[监控列表](https://argus.dev/monitoring)，搜索 `B05c`，查看 `B05c-ZooKeeper-A`、`B05c-ZooKeeper-B` 的生效配置和样本新鲜度。
2. 分别进入默认仪表盘：六个面板显示活动连接、ZNode 数量、Watch 数量、正在处理的请求、平均延迟和数据包速率。
3. A 为压力测试对象，B 为健康对照。正常时各有一个低频客户端会话、临时节点和 Watch；每两秒实际写入并读取有界数据。
4. 在[告警历史](https://argus.dev/alerts?tab=history)搜索 `B05c`，查看连接数和 ZNode 数量两条策略的触发、恢复及时间线。

建议历史窗口为 **2026-10-29 08:15–08:20（Asia/Shanghai）**。当前压力已解除，临时压力节点已回收。

## 已发生的真实场景

| 场景 | 实测结果 |
| --- | --- |
| 节点协议 | A/B 均实际创建、读取和更新节点，更新触发 `CHANGED` Watch 通知 |
| ACL | 业务子树使用 digest ACL，匿名读取返回 NoAuth，未给 Collector 伪装不支持的监控认证 |
| 会话临时节点 | 校验 ephemeralOwner 属于真实会话，关闭会话后节点自动消失，没有调用 delete 冒充回收 |
| 连接数压力 | A 新增 12 个真实会话，各有临时节点及 Watch；活动连接从基线 2 到观测值 14，告警触发，关闭会话后恢复 |
| ZNode 压力 | A 的一个会话创建 80 个临时节点，ZNode 总量从基线 11 到观测值 91，告警触发；关闭会话后服务器回收全部 80 个节点并恢复 |
| 健康对照 | 两次压力期间 B 保持正常读写及采集，指标未串源 |
| 独立停采 | A 停采时两条策略暂停评估，B 继续有新样本，A 重新启用后恢复采集和评估 |

阈值在压力开始前冻结：connections > 8、恢复 <= 4；znodes > 60、恢复 <= 20。每 10 秒评估，触发与恢复均持续 20 秒。没有配置外部通知接收人。

压力最长 180 秒，另有 systemd 190 秒上限。正常客户端最长 8 小时，到期关闭会话并回收自己的临时节点，服务及监控保留；不会自动进入 B06a。

监控通过专网内原生 `mntr` 统计，业务节点使用 digest ACL；本批不包含 TLS、多节点选主/容灾，也不与正式平台 Keeper 混用。节点数和连接数来自实际 ZooKeeper 状态，没有向 Argus 数据库或遥测入口写预制数据。

## 证据和运行流程

- [Compose、协议客户端和清理流程](../runtime/B05c/runbook.md)
- [真实资源 ID](../../../artifacts/plan9/p9-b05c-20261029-01/resource-map.json)
- [A 协议/Watch/ACL/会话回收](../../../artifacts/plan9/p9-b05c-20261029-01/a-protocol-preflight.json)
- [B 协议验证](../../../artifacts/plan9/p9-b05c-20261029-01/b-protocol-preflight.json)
- [真实告警触发与恢复](../../../artifacts/plan9/p9-b05c-20261029-01/scenario-checks.json)
- [连接数原生对照](../../../artifacts/plan9/p9-b05c-20261029-01/connections-native-comparison.json)
- [节点数原生对照](../../../artifacts/plan9/p9-b05c-20261029-01/nodes-native-comparison.json)
- [临时节点回收核验](../../../artifacts/plan9/p9-b05c-20261029-01/nodes-session-cleanup.json)
- [停采及恢复](../../../artifacts/plan9/p9-b05c-20261029-01/lifecycle-checks.json)
- [最终指标核验](../../../artifacts/plan9/p9-b05c-20261029-01/monitoring-verification.json)
- [真实浏览器检查](../../../artifacts/plan9/p9-b05c-20261029-01/browser-review.json)
- [最终检查汇总](../../../artifacts/plan9/p9-b05c-20261029-01/checks.json)

B03-D01 继续作为原批次未关闭诊断问题保留，不因本次切换声明已修复。

## 当前对象入口

- [B05c-ZooKeeper-A 默认仪表盘](https://argus.dev/dashboards/746036ac-804b-5e89-8256-5269f1918a2e?resource=80de99c1-9950-5162-a8f5-8365edd8ff1a&resource_type=monitoring_target)
- [B05c-ZooKeeper-B 默认仪表盘](https://argus.dev/dashboards/746036ac-804b-5e89-8256-5269f1918a2e?resource=452c9b8a-0a9e-5b27-8395-d364e941cef0&resource_type=monitoring_target)
- [采集主机](https://argus.dev/hosts/01a18500-32f4-75a1-993f-8ec92aca50a3)
- [B05c ZooKeeper connections high 历史](https://argus.dev/alerts?tab=history&policy_id=7a89bbe3-5ffe-509c-b53d-36727d7dbe2c)
- [B05c ZooKeeper znodes high 历史](https://argus.dev/alerts?tab=history&policy_id=7b011a70-07c9-5616-b4d5-efd9df072bf1)
