# ClickHouse migrations

本目录是遥测 Schema 版本与引导 SQL 的权威来源，部署时同步打包到 `deploy/helm/argus-telemetry-pipeline/files/`。租户表的实际创建、校验与升级由 Query 内的 `TenantSchemaManager` 在租户锁内执行，查询引擎使用独立只读连接。

Schema v5 增加数值样本的来源/时间投影，并同步物化已有观测；ReplacingMergeTree 合并重建投影。原始表、TTL、来源/资源身份和直方图语义保留，不通过插入虚假业务样本处理查询性能问题。详见[查询引擎 ADR](../../docs/adr/telemetry-query-engine.md)。

Schema v6 增加 metric_series.assignment_epoch 内部物化列，表达式仍来自可信 resource_attributes 中的 argus.assignment.epoch。Query 在租户锁内同步物化历史行，校验完成前不标记 ready；不删除原始 Map 或样本。指标归属排序无需为单个标量读取无关的宽属性列，来源版本与 assignment 优先级不变。Plan11 的真实 ClickHouse 回归覆盖旧表升级、重复执行、原始行保留和预算边界。
