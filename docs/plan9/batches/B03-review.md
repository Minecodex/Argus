# B03 查看记录

2026-10-27：用户要求“清理 B03 后部署 B04，把 Redis 和 Memcached 的真实场景、监控、仪表盘和告警都接好，我再去平台看。”本批已通过正常 API 完成策略/采集收尾、软件卸载、主机退役和授权撤销，5 个容器、5 个卷及网络无残留。历史与 B03-D01 保留，切换不视为该诊断问题已验收或修复。见[清理记录](../../../artifacts/plan9/p9-b03-20261027-01/cleanup.json)。以下保留交付时的快照；当前批次见 [B04](./B04-review.md)。

运行 `p9-b03-20261027-01`。B02b 已由用户确认通过并清理；本批为 MySQL 与 PostgreSQL 的真实业务、原生采集和告警场景。当前平台版本 `local-20261027-b03-v1`；四目标新鲜采集、24 个面板、两条真实告警触发/恢复、独立停采、错误凭据恢复及 6 项真实浏览器 E2E 通过。中英文翻译检查 15 项、平台健康 20/20；正式 6 个 PVC 和 2 个 OpenSandbox Pod 保留。当前无本批活跃告警，也未观察到新的投递缺口。因 B03-D01 仍未闭合，状态为 `deployed_with_known_issues`，等待人工查看。

## 场景与查看顺序

1. 打开[监控列表](https://argus.dev/monitoring)，搜索 `B03`：应有 MySQL-A/B、PostgreSQL-A/B 四个独立目标。
2. 检查配置、已托管凭据、生效版本、新鲜度；四个目标使用同一 `B03-DB-Collector`，业务数据分别位于独立数据库和卷。
3. 从各行进入默认仪表盘，对比 A/B。MySQL 看运行时长、线程、缓冲池、操作速率和行锁；PostgreSQL 看连接数/上限、数据库大小、提交/回滚和块读取速率。
4. 打开[告警历史](https://argus.dev/alerts?tab=history)，搜索 `B03`。两条连接数告警都经历实际触发和恢复；检查详情趋势、数字、查询说明和可读时间线。

专用客户端真实执行业务 SQL：A 每 2 秒、B 每 7 秒更新/读取固定数据并提交或回滚；A/B 分别最多 200/50 行，最长 8 小时。超时只停止业务负载，数据库和监控继续保留供查看。当前不保留压力连接或锁，正常连接数约 2；MySQL 原生只读核验会临时多打开一个连接。

## 已执行的真实场景

- MySQL 8.4.13 / PostgreSQL 18.6，各两实例、独立卷和独立随机账号；正常 SSH 接入采集主机，原生 Receiver 取数。
- A 实例增加 10 个实际业务连接后，监控连接数升至 12；B 对照保持 2。告警阈值在负载前固定为 `> 8`、恢复 `<= 4`，触发/恢复持续 20 秒、评估周期 10 秒。
- 两类数据库分别持有并释放 20 秒事务锁，原生状态均确认真实阻塞，等待连接实际持续约 20.01 秒；无永久锁。
- 数据库监控用户不能读业务表或创建业务表。测试业务用户与监控用户相互独立；没有操作正式 Argus 数据库。
- 本批目标均在私有 Compose 网络中，数据库端口不发布到宿主；本次数据库协议为明文测试连接，不包含数据库 TLS 验收。

告警历史建议窗口（Asia/Shanghai，2026-10-27）：**16:06–16:10**。MySQL 场景 16:06:25–16:08:01，PostgreSQL 场景 16:08:01–16:09:32。查看实时最近一小时也能看到这些尖峰及恢复段。

## B03-D01：原生数据库认证失败诊断未闭合

错误采集凭据确实被数据库拒绝，受影响目标没有新样本，其他目标继续采集；恢复原始凭据后重新得到真实样本。未改变业务或数据库管理员密码。

但当前平台仅显示健康“未知”，随后数据“过期”，没有显示具体认证失败代码。认证拒绝可以在 Collector 的真实日志中确认。原因是原生数据库 Receiver 失败时不输出指标，而现有采集诊断主要根据收到的指标更新；不能把无数据猜成认证失败，也不能伪造 `up=0`。

最初要求“120 秒内平台显示明确失败代码”的检查未通过，保留在 `credential-diagnostic-gap.json`。实际拒绝、无样本、对照隔离和恢复另行验证，不把调整后的观测冒充原诊断检查通过。本批因此有已知诊断问题，**不标全部验收通过**。

## 文件与证据

- [Compose 与可重复流程](../runtime/B03/runbook.md)
- [资源与真实 ID](../../../artifacts/plan9/p9-b03-20261027-01/resource-map.json)
- [连接告警与恢复](../../../artifacts/plan9/p9-b03-20261027-01/scenario-checks.json)
- [原生计数对照](../../../artifacts/plan9/p9-b03-20261027-01/metric-native-comparison.json)
- [凭据失败、对照与恢复](../../../artifacts/plan9/p9-b03-20261027-01/credential-checks.json)
- [独立停采与恢复](../../../artifacts/plan9/p9-b03-20261027-01/lifecycle-checks.json)
- [最终真实取数](../../../artifacts/plan9/p9-b03-20261027-01/monitoring-verification.json)
- [浏览器自检](../../../artifacts/plan9/p9-b03-20261027-01/browser-review.json)
- [自检汇总与部署入口](../../../artifacts/plan9/p9-b03-20261027-01/checks.json)

本批环境等待用户查看，未自动进入 B04。后续清理使用本批 `retire.py`，保留账号、默认模板、共享评估身份和历史记录。

## 当前对象入口

| 对象 | 页面 |
| --- | --- |
| B03-MySQL-A | [默认仪表盘](https://argus.dev/dashboards/04c0f993-e5ba-5b1c-8501-85faae7bb81f?resource=a8224b1d-9d69-5fce-a3db-139f5b82e0a1&resource_type=monitoring_target) |
| B03-MySQL-B | [默认仪表盘](https://argus.dev/dashboards/04c0f993-e5ba-5b1c-8501-85faae7bb81f?resource=87c7c733-379d-53c3-9967-2abea72b2cd8&resource_type=monitoring_target) |
| B03-PostgreSQL-A | [默认仪表盘](https://argus.dev/dashboards/86813150-51d7-5a66-9eab-fd9a97cc0b9e?resource=642e26c2-0e15-5ee1-b0ff-da99953bcc0a&resource_type=monitoring_target) |
| B03-PostgreSQL-B | [默认仪表盘](https://argus.dev/dashboards/86813150-51d7-5a66-9eab-fd9a97cc0b9e?resource=bb2efa9f-1da7-5cf9-92e3-a1c4777eef9b&resource_type=monitoring_target) |

[采集主机](https://argus.dev/hosts/01a17c61-9e33-7d5b-9445-6601fb05127d)

- [B03 MySQL connections high 历史](https://argus.dev/alerts?tab=history&policy_id=d2730e5f-fbaa-5dfe-bf50-110ce722dac7)
- [B03 PostgreSQL connections high 历史](https://argus.dev/alerts?tab=history&policy_id=3303c48e-f784-5178-84cc-4460689f6f1f)
