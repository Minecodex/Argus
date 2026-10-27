# P8-T06：多目标身份、存储与来源链路

状态：in_progress。前件：P8-T03、P8-T05。

## 设计与接点

- [02-domain-data-and-authorization.md](./02-domain-data-and-authorization.md)
- [03-collection-and-ingest.md](./03-collection-and-ingest.md)

已有/拟扩展接点：`internal/otelcol/argusgatewayidentity`、`internal/telemetry`、`internal/storage/postgres/queries/telemetry_sources.sql`、`api/proto/argus/telemetry`、`deploy`。新领域包和具体接口在 T01 固化；路径是定位入口，不代表本任务已实现。

## 实施清单

- [ ] 自动发现服务后仍逐记录固定应用身份和来源；同一逻辑服务多协议进入同目录不等于重复累加Span/RED数据。
- [ ] 替换所有遥测必须归属 Collector Host/Cluster 的硬约束，按控制面分配的 collection/target/source binding 校验。
- [ ] 支持一批三信号中多个目标/服务，逐资源段处理身份与拒绝策略；转发保留原始身份。
- [ ] 同步 Kafka 元数据、Writer、租户表 Schema、索引、指标序列身份与可信查询列，不接受客户端自报 argus.*。
- [ ] 实现来源重装、配置变更、迟到、重试、重复转发与 assignment epoch 的处理和诊断。
- [ ] 为服务级 Trace 去重/聚合提供可信逻辑身份，同时保留厂商/原始来源，不按名称隐式混合。
- [ ] 更新查询 RPC/能力握手、迁移 readiness、目录发现和权限验证，旧节点缺字段时失败关闭。

## 完成标准

- [ ] 真实 Collector→Ingest→Kafka→ClickHouse→Query 三信号验证混合目标、伪造身份、网关转发、迟到和重装历史。
- [ ] 查询结果的目标/应用/来源与登记一致，同名指标不串资源；重复重试不造成错误聚合。
- [ ] 相关存储、接口/生成客户端、权限、门户/工具、审计、配置和文档保持一致；单文件小于 2000 行。
- [ ] 保存测试范围、失败迭代、源码/版本与证据；只完成 mock 或单层测试不计真实验收。

## 验收关联

对应 [验收矩阵](./09-acceptance-matrix.md)：P8-A20、P8-A21、P8-A22、P8-A23、P8-A24、P8-A31、P8-A61。

新增验收关联：P8-A75。

## 实施记录

当前实施与验证层级见 [实施记录](./14-implementation-log.md)。尚未完成本任务全部验收；同时更新 [progress.json](./progress.json)，不预填通过结果。

基础设施旧配置围栏、应用/采集生效窗口和控制错误分类通过真实 PostgreSQL 验证，见[控制与准入](./20-collector-control-and-admission.md)。系统及业务日志各 12 条经过实际原生 Collector、mTLS、Kafka、Writer、ClickHouse，并覆盖未生效重试、强制退出恢复、脱敏和业务日志不归执行 Host，见[联合链路](./21-native-log-backend-pipeline.md)。控制登记与 CA 为受控夹具，未把组件验证当作正式平台验收完成。
