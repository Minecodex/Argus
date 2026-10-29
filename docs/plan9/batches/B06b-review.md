# B06b Elasticsearch 查看记录

2026-10-29：用户指令“下一轮”后已通过正常流程清理，容器、卷、网络及当前采集配置无残留，历史保留；转入 B06c。该指令记为推进，未替代明确的人工通过结论。下文为交付时的历史记录。

运行 `p9-b06b-20261029-01`。B06a 已正常清理，企业账号、共享定义与历史保留。本批为两个独立 Elasticsearch 8.17.0 单节点集群，不构成多节点高可用集群。当前部署版本 `local-20261029-b06b-v1`：两个实例共 14 个面板、6 项真实浏览器 E2E、15 项翻译检查和 20 项平台健康检查通过，正式 6 个 PVC 与 2 个 OpenSandbox Pod 保留。当前样本新鲜、Collector 已收敛、没有本批活跃告警；人工验收待用户进行。

## 查看顺序

1. 打开[监控列表](https://argus.dev/monitoring)，搜索 `B06b`，查看 `B06b-Elasticsearch-A` 与 `B06b-Elasticsearch-B` 的生效配置、托管凭据与新鲜样本。
2. 分别打开默认仪表盘：每实例七个面板，显示集群健康指示、节点数、主分片活动文档、JVM 堆、磁盘、索引操作速率和未分配分片数。
3. 健康指示为原生 green/yellow/red 三个 0/1 序列，**值 1 表示当前状态**；当前应为 green=1，其余为 0。
4. 在[告警历史](https://argus.dev/alerts?tab=history)搜索 `B06b`，检查 cluster not green 与 documents high 的真实触发、恢复、趋势和时间线。

建议历史窗口：**2026-10-29 12:13–12:16（Asia/Shanghai）**。当前副本数已恢复为 0，压力文档已删除；A/B 活动文档分别恢复为 200/80，正常低频更新与检索继续运行。

## 已实际执行的场景

| 场景 | 实测结果 |
| --- | --- |
| 索引与业务协议 | 实际创建索引，最多四并发 Bulk 写入，执行 get、update、delete、全文/关键词检索 |
| 权限边界 | monitor 不能搜索业务文档或创建索引；app 不能读取安全管理用户接口；operator 仅管理本批索引 |
| 单节点 yellow | A 将 `p9-business` 副本数改为 1，实际出现 1 个未分配副本、cluster=yellow，并触发告警；主分片仍可查询 200 条文档 |
| 健康恢复 | 将副本数恢复为 0，实际恢复 green，未分配分片回到 0，告警恢复 |
| 文档数量 | 实际增加 200 条压力文档，主分片活动文档从 200 到 400 并触发告警；删除这些 ID 后回到 200 并恢复 |
| 健康对照 | B 始终为 green，80 条文档保持正常读写，没有串入 A 的指标和告警 |
| 独立停采 | A 停采时策略暂停评估，B 仍有新样本；重新启用 A 后配置、采集和评估恢复 |

yellow 是本批单节点无法分配额外副本的真实结果，不表示主分片数据丢失，也不是高可用认证。阈值在场景前冻结：yellow/red 指示 > 0.5、恢复为 0；活动文档 > 300、恢复 <= 210。每 10 秒评估，触发/恢复持续 20 秒，没有外部通知收件人。

压力最长 180 秒，另有 systemd 190 秒上限。正常负载 A 每 2 秒、B 每 7 秒更新并检索原有文档，每进程最长 8 小时；到期只停客户端，服务和监控保留，不自动进入 B06c。

## 本次配置和视图修正

- 采用 Docker Hub 官方 8.17.0 镜像并核验服务版本；上游摘要及启动封装见镜像锁文件。
- Windows 文件绑定显示 777，原厂密码文件校验拒绝启动；已在容器私有 tmpfs 内复制为 UID 1000、600 权限，再调用原厂入口，未放宽检查。
- 默认模板 v2 使用主分片和具体索引，排除副本、`_all` 汇总重复计入；补齐未分配分片图与中英文标题。原始指标未改动。
- B06a 收尾首轮出现临时 MONITORING_UNAVAILABLE，核对后按检查点继续正常流程完成，旧运行资源无残留。

本批没有 TLS、多节点选主/容灾或内存/磁盘耗尽测试。Elasticsearch 与正式 Argus 存储完全独立，业务数据通过实际 API 产生，未向 Argus 数据库或遥测入口写预制指标。

## 证据和操作流程

- [运行包与清理命令](../runtime/B06b/runbook.md)
- [真实资源 ID](../../../artifacts/plan9/p9-b06b-20261029-01/resource-map.json)
- [A 索引/检索/权限](../../../artifacts/plan9/p9-b06b-20261029-01/a-protocol-preflight.json)
- [B 索引/检索/权限](../../../artifacts/plan9/p9-b06b-20261029-01/b-protocol-preflight.json)
- [告警触发和恢复](../../../artifacts/plan9/p9-b06b-20261029-01/scenario-checks.json)
- [yellow 原生状态对照](../../../artifacts/plan9/p9-b06b-20261029-01/health-native-comparison.json)
- [文档数量原生对照](../../../artifacts/plan9/p9-b06b-20261029-01/documents-native-comparison.json)
- [独立停采](../../../artifacts/plan9/p9-b06b-20261029-01/lifecycle-checks.json)
- [最终指标核验](../../../artifacts/plan9/p9-b06b-20261029-01/monitoring-verification.json)
- [真实浏览器检查](../../../artifacts/plan9/p9-b06b-20261029-01/browser-review.json)
- [最终检查汇总](../../../artifacts/plan9/p9-b06b-20261029-01/checks.json)

B03-D01 仍作为原批次未关闭的诊断问题保留，不因本批部署声明修复。

## 当前对象入口

- [B06b-Elasticsearch-A 默认仪表盘](https://argus.dev/dashboards/ae9c560b-b797-55b1-8b8a-e371580354a0?resource=8183bcd1-dbcb-553e-a7fe-5def4daf4593&resource_type=monitoring_target)
- [B06b-Elasticsearch-B 默认仪表盘](https://argus.dev/dashboards/ae9c560b-b797-55b1-8b8a-e371580354a0?resource=9580316e-8ea5-5089-b7b0-05ce393c25e0&resource_type=monitoring_target)
- [采集主机](https://argus.dev/hosts/01a185d4-0523-7044-b652-991a318a77fe)
- [B06b Elasticsearch cluster not green 历史](https://argus.dev/alerts?tab=history&policy_id=e277f7b7-1ef1-5051-9dd6-53714b0cd4df)
- [B06b Elasticsearch documents high 历史](https://argus.dev/alerts?tab=history&policy_id=4fe28647-327c-50bb-ad8f-91bce698cfe7)
