# P8-T12：第一批服务与探测集成

状态：in_progress。前件：P8-T04、P8-T05、P8-T06、P8-T14。

## 设计与接点

- [07-integration-catalog.md](./07-integration-catalog.md)
- [06-dashboard-scope-and-integrations.md](./06-dashboard-scope-and-integrations.md)

已有/拟扩展接点：`deploy/otelcol`、`internal/otelcol/configbundle`、`internal/telemetry`、`internal/dashboard`、`internal/alerting`、`web/apps/enterprise/src`。新领域包和具体接口在 T01 固化；路径是定位入口，不代表本任务已实现。

## 实施清单

- [ ] 本期暂缓目录按[专用插件复查](./13-direct-receiver-review.md)与progress.json同步；资源无专用采集组件时不新建采集、不提供可用默认仪表盘，门户/API/Agent一致。
- [ ] 第一批全部候选完成开源选型；支持组合交付并通过五类门禁，无合格内置组件者按D23保留具名证据，不自研补目录。
- [ ] 对以下候选完成选型并交付有合格内置Collector方案的组合：MySQL、PostgreSQL、SQL Server、Oracle、MongoDB、Redis、Kafka、RabbitMQ、Nginx、Elasticsearch，以及 HTTP/HTTPS、TCP、TLS。
- [ ] 逐项固化目标版本/平台/部署模式、连接参数、支持认证、只读权限、信号、指标和受限能力；特殊集群拓扑单列。
- [ ] 先以 MySQL＋Redis＋Nginx 打通通用路径，再扩展其余必需项；这是内部实现顺序，排除项仅适用无合格内置组件的暂缓规则。
- [ ] 每项交付多实例配置、测试、状态、错误诊断、生命周期、默认仪表盘和适用指标告警建议。
- [ ] 按已锁定 Grafana 模板映射真实字段、单位/函数、变量/规则；RabbitMQ 队列与 Nginx 日志派生能力分别补齐来源。
- [ ] 采集器版本与所有依赖进入制品清单；外部 Oracle/Windows 等缺环境时保留未验收状态，不挂空壳 supported。

## 完成标准

- [ ] 每个纳入支持组合的 CONFIG/DATA/VIEW/AUTH/LIFECYCLE 五类门禁均有实际目标与运行版本证据。
- [ ] 同 Collector 的两个 MySQL 与 SQL Server 并行采集且独立启停；仪表盘在新目标、停采、无数据、权限变化下正确。
- [ ] 相关存储、接口/生成客户端、权限、门户/工具、审计、配置和文档保持一致；单文件小于 2000 行。
- [ ] 保存测试范围、失败迭代、源码/版本与证据；只完成 mock 或单层测试不计真实验收。

## 验收关联

对应 [验收矩阵](./09-acceptance-matrix.md)：P8-A14、P8-A15、P8-A24、P8-A44、P8-A45、P8-A53。

新增验收关联：P8-A74、P8-A76。

## 实施记录

首批原生组件配置和真实采集已验证，平台全链路仍在实施；见 [实施记录](./14-implementation-log.md)，同时更新 [progress.json](./progress.json)，不预填通过结果。

后续已补 PostgreSQL 17、HTTP/TCP/证书期限的实际目标、认证/故障/恢复/独立启停及 14 个默认查询；发现并通过上游配置关闭认证失败仍产生的两个 PostgreSQL 零计数。见[范围及证据](./37-postgres-and-network-probes.md)。仍非正式创建/接入和完整五类平台门禁。

RabbitMQ 4.3.6 的具名原生组合、只读 monitoring 身份、错误凭据/恢复、独立生命周期及六个默认查询已通过组件/真实后端验证，见[消息与协调目标](./40-rabbitmq-and-zookeeper-targets.md)。其他版本/拓扑和正式平台路径仍未验证。

后续补齐 Kafka 4.2.1 单节点 KRaft/SASL 元数据只读组合、实际消费组 offset、认证恢复及八个默认查询，见 [53](./53-native-network-and-kafka.md)。Collector 统一升级 0.159.0，不将此扩大为任意 Kafka 4.x、多 Broker/JMX 或正式部署通过。其余数据库/中间件及平台门禁仍未完成。
