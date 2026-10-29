# B06a MongoDB 查看记录

2026-10-29：用户要求清理 B06a 后部署 B06b。本批已完成正常停采、卸载、退役及回收，三个容器、五个卷和网络无残留，账号及历史保留。首次归档遇到临时 MONITORING_UNAVAILABLE，核对状态后按检查点继续并完成收尾，未绕过版本保护或改库。见[切换记录](../../../artifacts/plan9/p9-b06a-20261029-01/transition.json)、[清理记录](../../../artifacts/plan9/p9-b06a-20261029-01/cleanup.json)。当前批次为 [B06b](./B06b-review.md)，以下保留原交付快照。

运行 `p9-b06a-20261029-01`。B05c 已按用户指令清理，账号、共享模板及历史保留。本批是两个独立 MongoDB 7.0.16 standalone 实例，人工验收待用户进行；当前部署版本 `local-20261029-b06a-v1`：两个实例共 14 个面板、6 项真实浏览器 E2E、15 项翻译检查和 20 项平台健康检查通过，正式 6 个 PVC 与 2 个 OpenSandbox Pod 保留。当前样本新鲜、Collector 已收敛，没有本批活跃告警。

## 查看顺序

1. 打开[监控列表](https://argus.dev/monitoring)，搜索 `B06a`，检查 `B06a-MongoDB-A`、`B06a-MongoDB-B` 的采集配置、托管凭据、生效状态和新鲜样本。
2. 分别打开默认仪表盘。每实例七个面板：连接数、进程内存、数据库数据量、数据库存储量、操作速率、网络接收速率、缓存操作速率。
3. 连接数按实例/类型展示，不把同一 serverStatus 值的数据库副本相加。数据量和存储量保留数据库维度，缓存图是真实 hit/miss 计数速率。
4. 在[告警历史](https://argus.dev/alerts?tab=history)搜索 `B06a`，查看 connections high 与 update rate high 两条策略的触发、恢复和时间线。

建议历史窗口为 **2026-10-29 11:08–11:13（Asia/Shanghai）**。操作告警按一分钟 update 窗口计算，默认操作/缓存图使用五分钟窗口；告警详情图与其规则窗口一致。

## 实际执行的场景

| 场景 | 实测结果 |
| --- | --- |
| 正常业务 | MongoDB 协议实际完成 insert、find、update、delete，保留 A 200 条、B 80 条固定文档及索引 |
| 监控权限 | monitor 仅 clusterMonitor，读取/插入业务文档均被拒绝；app 不能读取管理员用户数据 |
| 连接压力 | A 打开 12 个 MongoClient，含实际驱动监测连接，指标观测到 42 个 current connections；触发告警后关闭客户端恢复 |
| 更新压力 | 对既有文档实际完成 799 次 updateOne，观测到一分钟更新速率约 13.3 ops/s；停止后窗口自然回落并恢复 |
| 数据边界 | 压力结束文档数仍为 A 200、B 80，未不断插入文档制造负载 |
| 健康对照 | B 保持低频真实读写，未混入 A 的指标和告警 |
| 独立停采 | A 停采时策略暂停评估，B 仍有新样本；A 恢复后配置、样本和评估正常 |

阈值在负载前冻结：current connections > 20、恢复 <= 12；一分钟 update rate > 5 ops/s、恢复 <= 1 ops/s。每 10 秒评估，触发和恢复持续 20 秒。没有配置外部通知收件人。

恢复采集与发布时段还记录了 3 条短暂 `Query health: no_data`：11:15:20–11:15:40，以及 11:18:10–11:18:40（按各策略实际时间记录）。均以 `query_recovered` 恢复并保留历史，不与两次业务超阈值事件混为一谈。

当前压力已解除，正常负载为 A 每 2 秒、B 每 7 秒一次更新和读取；每进程最多 8 小时，到期只停止客户端，数据库和监控保留，不自动进入 B06b。

本批不包含副本集、分片、多文档事务或 TLS 验收。MongoDB 与正式 Argus 存储完全独立，所有场景数据来自实际协议请求，没有向 Argus 数据库或遥测入口写预制曲线。

## 本批修正与证据

共享 MongoDB 默认模板升级为 v2，新增真实缓存操作速率，按实例/类型去重连接统计，并补齐全部中英文标题。原始指标保留，未改变查询预算、来源权限或私有副本生命周期。

- [Compose、业务客户端与清理流程](../runtime/B06a/runbook.md)
- [真实资源 ID](../../../artifacts/plan9/p9-b06a-20261029-01/resource-map.json)
- [A 真实 CRUD 和权限](../../../artifacts/plan9/p9-b06a-20261029-01/a-protocol-preflight.json)
- [B 真实 CRUD 和权限](../../../artifacts/plan9/p9-b06a-20261029-01/b-protocol-preflight.json)
- [告警触发与恢复](../../../artifacts/plan9/p9-b06a-20261029-01/scenario-checks.json)
- [连接数与原生状态对照](../../../artifacts/plan9/p9-b06a-20261029-01/connections-native-comparison.json)
- [更新计数与原生状态对照](../../../artifacts/plan9/p9-b06a-20261029-01/operations-native-comparison.json)
- [实际压力操作完成记录](../../../artifacts/plan9/p9-b06a-20261029-01/operations-client-completed.json)
- [独立停采](../../../artifacts/plan9/p9-b06a-20261029-01/lifecycle-checks.json)
- [最终指标核验](../../../artifacts/plan9/p9-b06a-20261029-01/monitoring-verification.json)
- [真实浏览器检查](../../../artifacts/plan9/p9-b06a-20261029-01/browser-review.json)
- [最终自检汇总](../../../artifacts/plan9/p9-b06a-20261029-01/checks.json)

B03-D01 的历史诊断问题仍保留于原记录，不因本批部署而声明已关闭。

## 当前对象入口

- [B06a-MongoDB-A 默认仪表盘](https://argus.dev/dashboards/11d91cb6-20b6-5549-bee2-50df90d47eaf?resource=513a1d2d-dd6f-5fa8-82a8-34dfe5258e28&resource_type=monitoring_target)
- [B06a-MongoDB-B 默认仪表盘](https://argus.dev/dashboards/11d91cb6-20b6-5549-bee2-50df90d47eaf?resource=55438360-9f36-5ec2-a794-ab6c03b556c2&resource_type=monitoring_target)
- [采集主机](https://argus.dev/hosts/01a1859b-9415-7107-9511-11f5e2bdf6db)
- [B06a MongoDB connections high 历史](https://argus.dev/alerts?tab=history&policy_id=c4dbf03b-90e9-5dbb-af38-b710861ef71f)
- [B06a MongoDB update rate high 历史](https://argus.dev/alerts?tab=history&policy_id=0793b666-7dfc-5b89-8d36-c6048b63cb22)
