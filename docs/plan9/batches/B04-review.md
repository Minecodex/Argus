# B04 Redis / Memcached 查看记录

2026-10-28：用户明确表示“B04 没啥问题，清理后部署 B05a，把 RabbitMQ 的真实场景、监控、仪表盘和告警都接好，我再去平台看。”本批已记录验收通过，并完成正常停采、软件卸载、主机退役及资源授权回收；3 个容器、2 个卷和网络无残留。账号、共享模板和历史保留。见[验收记录](../../../artifacts/plan9/p9-b04-20261027-01/acceptance.json)、[清理记录](../../../artifacts/plan9/p9-b04-20261027-01/cleanup.json)。当前批次见 [B05a](./B05a-review.md)。以下保留当时交付快照。

运行 `p9-b04-20261027-01`。B03 已按用户指令清理，账号、历史和 B03-D01 记录保留。当前版本 `local-20261027-b04-v2`，状态 `ready_for_review`：20 个面板、6 项真实浏览器检查（中英/明暗、桌面宽度）、真实故障恢复及来源隔离通过，平台健康 20/20，正式 6 个 PVC 与 2 个 OpenSandbox Pod 保留。两目标持续取得新鲜样本，当前没有本批活跃告警；人工验收仍待用户进行。

## 平台查看顺序

1. 打开[监控列表](https://argus.dev/monitoring)，搜索 `B04`，检查 `B04-Redis`、`B04-Memcached` 的配置、生效状态和新鲜样本。
2. 从目标行进入“默认仪表盘”。Redis 为 11 个面板、Memcached 为 9 个面板，包含内存、连接、请求、真实命中/未命中、命中率与淘汰；Redis 还展示到期速率。
3. 轻量负载每 2 秒真实执行一次写入、一次命中和一次未命中，正常窗口命中率约 50%。无 GET 请求的窗口不显示成 0% 命中率。
4. 打开[告警历史](https://argus.dev/alerts?tab=history)，搜索 `B04`，查看内存告警的触发、恢复、趋势和时间线。当前压力键已删除，不保留高内存压力。

实际版本 Redis 8.10.0、Memcached 1.6.45。两者为单实例，独立于正式平台缓存，共用 `B04-Cache-Collector`。Redis 使用受限监控账号；Memcached 使用专用测试网络内的原生无认证协议。本批不包含 Redis Cluster/Sentinel 或缓存 TLS/SASL 验收。

## 实际发生的场景

| 场景 | 实测结果 |
| --- | --- |
| 正常写读、未命中、TTL | 两种缓存均实际写入并读回值，读取不存在键返回未命中，2 秒 TTL 到期后读不到原值 |
| Redis 监控权限 | INFO 取数正常，读取业务键被 ACL 拒绝 |
| 空闲命中率 | 原生累计计数保持不变；命中/未命中速率为零，命中率为空，不伪造 0% |
| Redis 内存压力 | 1024 个 32 KiB 写入、120 秒 TTL，8 MiB 容量实际产生 864 次淘汰，内存告警触发并恢复 |
| Memcached 内存压力 | 相同有界写入实际产生 844 次淘汰，内存告警触发并恢复 |
| 对照与独立停采 | 压力期间另一缓存健康；Redis 停采时 Memcached 继续有新鲜样本，Redis 恢复后评估正常 |

告警阈值在压力前固定：内存高于 5 MiB 持续 20 秒触发，降到 2 MiB 及以下持续 20 秒恢复；每 10 秒评估。没有配置外部通知收件人。

历史查看窗口为 **2026-10-27 21:42–21:46（Asia/Shanghai）**：Redis 21:42:55–21:44:02，Memcached 21:44:02–21:45:01。淘汰速率使用五分钟窗口，恢复后仍会逐渐回落；累计淘汰计数不会因恢复归零。

持续负载有 8 小时上限、32 个循环键、12 秒 TTL；到期只停止客户端，服务和采集保留，等待用户反馈。可通过运行包重新启动有界负载；不会自动进入 B05。

## 本批修正与证据

新增缓存模板及中英文标题已统一处理。完整页面检查发现的元数据扫描开销已在共享查询层修正，保留原有权限和预算，详见[缓存视图和查询修正](../08-b04-cache-views-and-metadata-scan.md)。B03-D01 属于上批尚未关闭的诊断问题，本批切换不等同修复。

- [运行包与清理流程](../runtime/B04/runbook.md)
- [真实资源 ID](../../../artifacts/plan9/p9-b04-20261027-01/resource-map.json)
- [故障、淘汰与告警恢复](../../../artifacts/plan9/p9-b04-20261027-01/scenario-checks.json)
- [空闲比率](../../../artifacts/plan9/p9-b04-20261027-01/idle-ratio-checks.json)
- [独立停采](../../../artifacts/plan9/p9-b04-20261027-01/lifecycle-checks.json)
- [最终指标核验](../../../artifacts/plan9/p9-b04-20261027-01/monitoring-verification.json)
- [真实浏览器检查](../../../artifacts/plan9/p9-b04-20261027-01/browser-review.json)
- [最终检查汇总](../../../artifacts/plan9/p9-b04-20261027-01/checks.json)

## 当前对象入口

| 对象 | 页面 |
| --- | --- |
| B04-Redis | [默认仪表盘](https://argus.dev/dashboards/6f0cd210-ab49-5d65-be71-f63eb4535beb?resource=4c73e58a-975a-5781-8824-b66499be5129&resource_type=monitoring_target) |
| B04-Memcached | [默认仪表盘](https://argus.dev/dashboards/f8cd2937-1919-5871-a126-d3d6166aaf0c?resource=a8509436-ce9d-586f-8b17-2dd5bdf344b5&resource_type=monitoring_target) |

[采集主机](https://argus.dev/hosts/01a17d90-b94f-7867-b498-5d7a8594401c)

- [B04 Redis cache memory high 历史](https://argus.dev/alerts?tab=history&policy_id=02328905-3910-55a1-9434-fcbd16b062eb)
- [B04 Memcached cache memory high 历史](https://argus.dev/alerts?tab=history&policy_id=d1e5f727-a858-589b-9de6-d0e6e7d81350)
