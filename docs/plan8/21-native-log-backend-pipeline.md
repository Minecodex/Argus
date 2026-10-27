# 原生日志到真实存储的联合验证

状态：组件联合数据链路通过；正式部署、主机接入、门户/Agent 与真实模型验收未完成。

使用构建的 Collector 0.158.0、真实文件、mTLS Ingest、PostgreSQL 来源校验、Kafka 4.2.1 SCRAM-SHA-512、Redis、真实 Writer 和 ClickHouse。分别验证系统日志与关联应用/服务的业务日志，每组 12 条记录。

| 环节 | 已验证结果 |
| --- | --- |
| 配置尚未生效 | 每组实际收到 2 次可重试 Unavailable，没有提前确认样本 |
| Collector 崩溃 | 强制 KILL 后保留原生持久队列/checkpoint，使用同一状态目录重启 |
| 新旧记录 | 重启前 10 条与新增 2 条均入库，每组 12 条，当前受测时序无重复 |
| 数据脱敏 | 直接读取实际 Kafka Record 及查询结果，私有 canary 不出现；原生进程诊断也没有原文 |
| 资源归属 | 系统日志归 Host，业务日志归 Application，保留声明服务并更新实际首次/最近观测 |
| 执行者与业务资源 | 业务日志不能通过执行 Collector 所在 Host 的数据范围取出 |
| 运行状态 | 实际已接受的样本更新采集实例 last_sample_at |
| 清理 | 两组自有容器/网络结束后清理，未停止或更改其他部署 |

证据：[系统及业务两组联合日志](../../artifacts/plan8-native-mtls-kafka-system-business-pipeline.log)，[先前系统日志链路](../../artifacts/plan8-native-mtls-kafka-log-pipeline.log)。实现位于 `internal/collectormanager/native_pipeline_*_test.go`，通过 `ARGUS_MONITORING_NATIVE_PIPELINE=1` 显式启用，要求指定自有 PostgreSQL/ClickHouse 端点及构建的 Linux Collector 路径。Broker/Redis 只发布 loopback 端口，容器、网络使用唯一 owner 标签，清理前核验所有权。

本验证的控制记录和 CA 为隔离夹具，实际使用原生二进制、TLS、数据库认证与生产 Ingest/Writer 代码。没有伪装为已经正式登记的真实主机，也没有绕过权限校验来产生平台支持声明。它不替代 PKI 正式 enrollment/轮换、已部署 Connector、实际 Namespace/应用权限入口、门户、Agent 或模型场景；其中权限查询由其他分层测试验证，统一业务闭环仍是 T16 前件。
