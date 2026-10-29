# B06c SQL Server 查看记录

2026-10-29：用户指令“好的，下一波”后已按正常流程清理，转入 B06d Oracle。运行容器、卷、网络和当前采集配置无残留，历史保留；该指令记为推进，不代替明确人工通过。以下为交付时的记录。

运行 `p9-b06c-20261029-01`，部署版本 `local-20261029-b06c-v2`。B06b 已按正常流程清理，旧运行资源和当前采集配置无残留；企业账号、共享定义、业务数据和历史保留。

两个独立 SQL Server 2022 CU18 Developer 实例（实测 16.0.4185.3），A 执行负载、B 作健康对照。12 个面板、6 项真实浏览器 E2E、15 项翻译检查、dashboard/catalog Go 检查和 20 项平台健康检查通过，正式 6 个 PVC 与 2 个 OpenSandbox Pod 保留。最终 6 个采集配置 healthy/fresh、Collector 收敛，本批活跃告警为 0。人工验收仍待用户进行。

## 查看顺序

1. 打开[监控列表](https://argus.dev/monitoring)，搜索 `B06c`，查看 A/B 两个 SQL Server 目标及各自来源、托管凭据和新鲜样本。
2. 打开默认仪表盘：每实例六个面板——用户连接数、批请求速率、缓冲缓存命中率、页面预期生存时间、锁等待速率、SQL 编译速率。
3. 在[告警历史](https://argus.dev/alerts?tab=history)搜索 `B06c`，查看连接数、批请求的触发、恢复、指标趋势与时间线。

建议历史窗口：**2026-10-29 14:25–14:33（Asia/Shanghai）**。当前压力已停止，A/B 分别保持 200/80 行，正常事务负载继续运行。

## 实际场景与结果

| 场景 | 实际结果 |
| --- | --- |
| 业务协议 | 真实 TDS 建表、插入、读取、更新、删除，并验证提交持久化与回滚不改值 |
| 权限 | app 只可操作本批表，无创建数据库权限；monitor 可读性能状态，读取业务表返回 SQL 916 |
| 连接压力 | A 增加 12 个真实连接，采集值达到 15，超过阈值 10；关闭后回落到恢复阈值 5 以下 |
| SQL 请求 | 有界 SQL 查询使批请求约 24.34 次/秒，超过阈值 15；原始计数十秒近似差分约 26.24 次/秒，时间窗口与操作开销不同；停止后 <= 10 并恢复 |
| 锁等待 | 两个真实事务争用同一行，DMV 观察到 LCK_M_X，20.006 秒后释放并回滚；监控曲线出现约 0.02 次/秒的等待率 |
| 健康对照 | B 保持 80 行、低频正常事务，目标和来源隔离，未串入 A 的数据 |
| 独立停采 | A 停采时两策略暂停评估，B 继续有新样本；恢复后生效版本、样本和评估正常 |

阈值在场景前冻结，每 10 秒评估，触发/恢复持续 20 秒，无外部通知收件人。主要业务事件：连接数 14:26:20–14:27:00，批请求 14:28:00–14:29:20。连接压力客户端的实际保活查询也触发过批请求告警（14:26:20–14:27:20），因此该策略历史中有两次真实业务事件。

历史另保留两次已恢复的查询健康事件：14:25:20–14:26:00 位于版本重载后的采集恢复阶段；14:30:00–14:30:10 位于独立停采恢复阶段。它们与上述业务越阈事件分开记录，不视作 SQL Server 故障。

## 本批修正

- SQL Server 默认模板升级为 v2。锁定 receiver 的 SQL 路径把部分累计 DMV 计数输出为 rate 名称的 gauge，默认展示改为对批请求、锁等待、SQL 编译使用 `rate(...[1m])`。原始指标保留，未改写遥测样本。该修正适用于所有正常接入的 SQL Server 默认仪表盘。
- 页面生存时间只取实例级 Buffer Manager，避免与 Buffer Node 重复；补齐六个面板的中英文标题。
- 启动封装保留原厂 `mssql` 用户和主组，修复最初覆盖用户组导致数据目录无法写入的问题；实际服务以非 root 运行。
- 运行器权限检查改用服务器级 `HAS_PERMS_BY_NAME(NULL,NULL,...)`，没有放宽账号权限；依据 [Microsoft 权限函数文档](https://learn.microsoft.com/en-us/sql/t-sql/functions/has-perms-by-name-transact-sql)。
- 共用验证脚本的范围查询路径校正为正式 `/enterprise/metrics/query_range`，已通过真实锁等待查询验证。

## 运行边界

这是两个独立 Developer 测试实例，不是 Always On、高可用或 SQL TLS 验收。SQL 端口仅在本批私有网络可达，Argus API/门户仍校验正式 CA。业务通过真实 SQL 产生，没有直接写 Argus 业务库或遥测库。

压力最长 180 秒/2000 请求，另有 systemd 190 秒上限；正常事务 A 每 2 秒、B 每 7 秒，每进程最长 8 小时，到期仅停客户端，服务和监控保留，不自动进入 B06d。

当前客户端曾通过受控文件复制修正权限检查，运行文件 SHA256 与最终源码一致，已重新构建可复现的主机镜像；实际运行 image ID、上游摘要、客户端覆盖哈希和重建镜像 ID 均在镜像锁文件记录。

B03-D01 仍作为原批次未关闭的原生认证失败诊断问题保留，不因本批部署声明修复。

## 当前对象入口

- [B06c-SQLServer-A 默认仪表盘](https://argus.dev/dashboards/7486f32a-b56c-52a4-8c2a-f6dfb0a20093?resource=3702599b-ef51-5022-bb29-fbcb1c1dd4ee&resource_type=monitoring_target)
- [B06c-SQLServer-B 默认仪表盘](https://argus.dev/dashboards/7486f32a-b56c-52a4-8c2a-f6dfb0a20093?resource=9d40aab5-1276-5ab9-8479-c3d24b8a2c2d&resource_type=monitoring_target)
- [采集主机](https://argus.dev/hosts/01a1864d-8787-7e3e-b5f7-16730bcacf7e)
- [B06c SQL Server connections high 历史](https://argus.dev/alerts?tab=history&policy_id=b57f6600-e82c-5991-94c5-063f9f819c64)
- [B06c SQL Server batch requests high 历史](https://argus.dev/alerts?tab=history&policy_id=61446c04-9e01-598d-8451-5cd6a99d2945)

## 证据

- [运行包与清理命令](../runtime/B06c/runbook.md)
- [真实资源 ID](../../../artifacts/plan9/p9-b06c-20261029-01/resource-map.json)
- [A 协议与权限](../../../artifacts/plan9/p9-b06c-20261029-01/a-protocol-preflight.json)
- [B 协议与权限](../../../artifacts/plan9/p9-b06c-20261029-01/b-protocol-preflight.json)
- [告警触发与恢复](../../../artifacts/plan9/p9-b06c-20261029-01/scenario-checks.json)
- [批请求原始计数核对](../../../artifacts/plan9/p9-b06c-20261029-01/batch-rate-native-proof.json)
- [实际锁等待](../../../artifacts/plan9/p9-b06c-20261029-01/lock-monitored-checks.json)
- [锁等待监控查询](../../../artifacts/plan9/p9-b06c-20261029-01/lock-rate-query.json)
- [独立停采](../../../artifacts/plan9/p9-b06c-20261029-01/lifecycle-checks.json)
- [最终指标核验](../../../artifacts/plan9/p9-b06c-20261029-01/monitoring-verification.json)
- [真实浏览器检查](../../../artifacts/plan9/p9-b06c-20261029-01/browser-review.json)
- [最终状态及历史事件](../../../artifacts/plan9/p9-b06c-20261029-01/final-state.json)
- [最终检查汇总](../../../artifacts/plan9/p9-b06c-20261029-01/checks.json)
