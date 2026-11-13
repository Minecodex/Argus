# B10b Kubernetes 存储与任务查看记录

当前运行 `p9-b10b-20261112-01`，集群 **B10b Kubernetes Storage and Jobs**，命名空间 **b10b-workloads**。B10a 已按本次下一轮指令清理，历史保留；本批环境运行中，人工验收待进行。正式环境六个 PVC 绑定保留。

## 查看路线

1. 打开 [B10b 资源工作台](https://argus.dev/kubernetes/01a1cd21-4008-7b21-97fc-b808792615ab?tab=resources)，选择 `b10b-workloads`。
2. 「存储」查看 `b10b-data` PVC，实际容量为 128 MiB，StorageClass 为 `b10b-expandable`；查看绑定 PV 和只读 StorageClass。已经完成 64 → 128 MiB 的在线扩容、重启保留文件、缩容拒绝，以及临时卷删除回收。
3. 「工作负载」查看 `b10b-job`、`b10b-pausable-job` 和 `b10b-cron`。已完成暂停/恢复、真实定时执行与手动运行；`b10b-ui-*` 是浏览器通过正常操作创建的真实任务，可能随后按 CronJob 的最近两条历史策略回收，执行记录保留。对应 Pod 的日志应显示 `job-completed`；业务 `b10b-storage-*` Pod 支持 Shell，可查看 `/data/receipt.json`、`/data/jobs.jsonl` 和 `df -k /data`。
4. 「弹性伸缩」查看 `b10b-autoscale` HPA，目前恢复 1 副本。实际 CPU 压力已完成 **1 → 3 → 1**，可以在仪表盘选择最近一小时查看历史波动。
5. 「网络」查看 `b10b-storage-allow-client` NetworkPolicy，当前保持生效。`b10b-allowed` 可以访问存储服务，`b10b-denied` 被真实 Calico 拒绝；已验证删除策略后恢复、重建后再次阻断。
6. 打开 [本批仪表盘](https://argus.dev/dashboards/35843509-f3aa-4b5d-bc09-fa384a5bfd03?resource=01a1cd21-4008-7b21-97fc-b808792615ab&resource_type=kubernetes_cluster) 或[四个绑定视图](https://argus.dev/kubernetes/01a1cd21-4008-7b21-97fc-b808792615ab?tab=dashboards)：本批可用副本、HPA 当前/期望副本、Job 完成数、CPU/内存，以及集群、节点和 Collector 默认视图。
7. 在[集群详情](https://argus.dev/kubernetes/01a1cd21-4008-7b21-97fc-b808792615ab?tab=resources)切换「采集配置」，三项原生采集应为健康、新鲜。节点行「证书信任」展示已配置公开 CA；节点名称匹配始终来自固定节点端点。
8. [告警中心](https://argus.dev/alerts) 的告警历史搜索 `B10b storage deployment has no available replica`。实际触发于 **2026-11-12 08:46:00**，恢复于 **2026-11-12 08:46:30**（北京时间）。当前本批活跃告警为 0；没有配置外部通知收件人。

## 本轮结果与范围

- 三项采集健康，四个仪表盘共 **24** 个面板取得真实数据。
- 六项部署页面 E2E 通过，覆盖中英明暗、桌面宽度、七类对象、真实 CronJob 手动执行、Pod 日志/终端、节点公开 CA 表单及仪表盘。
- 完整重试周期持续观察 **311.7 秒**，没有新增终止投递、入队失败或接收拒绝；本轮累计终止丢失指标点为 **0**。台账没有清零。
- 正式健康 **20/20** 通过，六个原有 PVC 保留。正式环境原有 NetworkPolicy 探测和无外部 Egress Gateway 降级提示仍记录；本批独立 Calico 的阻断测试已实际通过。
- 节点 CA 文件已由实际 Agent 使用；独立错误 CA 测试被原生 TLS 拒绝，测试 Job/ConfigMap 已清理。长 Job 原始创建观察被后续编辑替代的旧记录，已明确结算为 `KUBERNETES_OPERATION_SUPERSEDED`；后续暂停、恢复和完成分别有成功记录。

Rawfile 在本环境没有提供该 PVC 的 kubelet volume stats。PVC 对象、实际挂载 statvfs/df 和保留文件均已验证；没有生成虚假的 PVC 使用率曲线。详细原因、源码及发布范围见[修复记录](../19-b10b-storage-jobs-and-node-ca-20261112.md)。

两个低速客户端每 30 秒运行一次，后台活动截止 **2026-11-12 16:38:30（北京时间）**。CronJob 在同一时刻由精确 UID 保护暂停；环境和监控保留。查看通过并要求下一批后，先回收消费者/PVC/PV，再退役 Collector、集群与专属入口，进入 B10c。

[运行与清理流程](../runtime/B10b/runbook.md)。证据目录：`artifacts/plan9/p9-b10b-20261112-01/`，最终状态见 `checks.json`。


## 2026-11-12 工作台升级

本批场景未切换。平台已升级至 Kubernetes 资源工作台与运维大盘版本：默认进入大盘，资源名称打开详情，具体动作使用行菜单，提交后原位置显示进度。查看路径、最终镜像、真实指标和会话验证见 [交付验收](../../plans/frontend-redesign/kubernetes-workbench-acceptance-20261112.md)。临时验收集群、卷和连接已清理，B10b 原资源保留。

## 工作台重构后查看

B10b 场景保持运行，默认进入 [运维大盘](https://argus.dev/kubernetes/01a1cd21-4008-7b21-97fc-b808792615ab)。本轮专用临时集群和扩容测试卷已清理；原有 b10b-data 仍为 128Mi。样式、分页、动作和范围修复的完整记录见 [补齐验收](../../plans/frontend-redesign/kubernetes-workbench-repair-acceptance-20261112.md)。
