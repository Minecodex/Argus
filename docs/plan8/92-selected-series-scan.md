# 已选序列索引过滤与默认视图预算

2026-10-23。R67 的完整业务序列已通过全部真实模型场景，但 Kafka 和 ICMP 默认视图在累计数据增多后返回 `QUERY_BUDGET_EXCEEDED`，整体验收仍未通过。

## 实际证据

Kafka 的 broker、分区和副本面板有实际数据，但每个查询读取约 46 万行、40 多 MB，后三个消费组面板因预算不足被拒绝。ICMP 的实际丢包率为零；loss 面板扫描约 45 MB，包含两个选择器的 RTT 查询扫描约 91 MB，随后面板触发预算。原始响应保存在 R67 目录的 `plan8-extra-dashboard-{kafka,icmp}-timeout.json`。

这是查询扫描放大，不能归结为 Receiver 缺样本，也不能通过提高预算或显示零值绕过。更早 R66 的 ICMP 超时未保存完整响应，仍保留其无法精确追溯的范围。

## 修正

PromQL 存储层先按资源、来源及 Namespace 权限筛选序列，再通过外部 UUID 表读取样本。原样本 SQL 的 JOIN 等式没有充分触发 `series_id` 主键裁剪，导致读取同一企业的大量无关样本。

在样本 WHERE 中增加对同一已选 UUID 外部表的显式 IN 过滤。原 JOIN 保留，用于选择顺序和同时间点来源优先级；时间精度、来源权限谓词、重复样本处理、排序和预算值均不改变。不修改存储数据或表结构，不引入其他采集服务。

## 回归与验收

- 新真实 ClickHouse 场景插入 40 万条其他资源／来源的受控样本，并通过 Writer 写入选中目标的实际测试点。
- 原查询在高预算下返回正确值，但扫描 400006 行、19600869 字节；4 MiB 预算下失败。
- 候选查询连续三次返回相同值和资源身份，扫描 8199 行、336757 字节，在原 4 MiB 预算内通过。
- 遥测和全部查询引擎包回归通过，覆盖既有时间精度、去重、类型和来源隔离路径。
- 全量组件增加 `metric-scan-pruning`，合计 87 组。先定向验证 Kafka 和网络探测的正式接入及默认视图，再执行最终完整业务和发布验收。

原始失败、候选和合入回归分别记录在 `plan8-series-scan-red-v3.log`、`plan8-series-scan-candidate.log`、`plan8-series-scan-telemetry-regression.log`、`plan8-series-scan-merged-regression.log`。本记录不声明已经发布。
