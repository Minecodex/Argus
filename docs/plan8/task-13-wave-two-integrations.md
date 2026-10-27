# P8-T13：第二批服务与探测集成

状态：in_progress。前件：P8-T12。

## 设计与接点

- [07-integration-catalog.md](./07-integration-catalog.md)
- [06-dashboard-scope-and-integrations.md](./06-dashboard-scope-and-integrations.md)

已有/拟扩展接点：`deploy/otelcol`、`internal/otelcol/configbundle`、`internal/telemetry`、`internal/dashboard`、`internal/alerting`、`web/apps/enterprise/src`。新领域包和具体接口在 T01 固化；路径是定位入口，不代表本任务已实现。

## 实施清单

- [ ] 本期暂缓目录按[专用插件复查](./13-direct-receiver-review.md)与progress.json同步；资源无专用采集组件时不新建采集、不提供可用默认仪表盘，门户/API/Agent一致。
- [ ] 第二批全部候选完成开源选型；支持组合交付并通过五类门禁，无合格内置组件者按D23保留具名证据，不以缺环境代替缺插件。
- [ ] 对以下候选完成选型并交付有合格内置Collector方案的组合：ClickHouse、TiDB、OceanBase、Memcached、Apache、IIS、HAProxy、Envoy、JVM、Tomcat、Spring Boot、RocketMQ、Pulsar、NATS、ZooKeeper、MinIO、DNS、ICMP。
- [ ] 按产品类型使用有现成资源指标采集实现的专用Receiver或原生探测Receiver，不以通用Prometheus/SQL/SDK接入补齐，内部抓取方式不变成另一套用户产品目录。
- [ ] 仅对通过专用插件准入的资源验证版本/运行依赖/指标合同；ClickHouse、TiDB、OceanBase、RocketMQ、Pulsar、NATS、Envoy、JVM、Tomcat、Spring Boot、MinIO暂缓，不实现采集及可用默认仪表盘。
- [ ] IIS/Windows 运行身份与 ICMP 权限单独验证；不以 Linux 容器测试宣称所有平台完成。
- [ ] 逐项交付同一套实例、凭据、状态、模板/告警、诊断与运维文档；避免新增应用内专用表格/样式/授权逻辑。
- [ ] 追加性能/基数与多目标组合测试，补充所有制品、模板、配置 Schema 的锁定版本。

## 完成标准

- [ ] 18 项均有选型结论，纳入支持组合完成五类门禁；缺项、未验证与版本/拓扑限制分别列明。
- [ ] 第二批与第一批及基础设施实例混跑，更新/停止/卸载不影响非目标采集；默认模板真实取数。
- [ ] 相关存储、接口/生成客户端、权限、门户/工具、审计、配置和文档保持一致；单文件小于 2000 行。
- [ ] 保存测试范围、失败迭代、源码/版本与证据；只完成 mock 或单层测试不计真实验收。

## 验收关联

对应 [验收矩阵](./09-acceptance-matrix.md)：P8-A24、P8-A44、P8-A45、P8-A59、P8-A62。

新增验收关联：P8-A74、P8-A76。

## 实施记录

Memcached、Apache、IIS、HAProxy、ZooKeeper、DNS、ICMP 的配置适配器和表单进入实现；原生配置校验见 [实施记录](./14-implementation-log.md)。各支持组合的真实数据、仪表盘、授权和生命周期门禁尚未完成，11 项既定暂缓保持不变。

后续已补 Memcached/Apache/HAProxy 三个原生实际目标、HAProxy 认证拒绝/恢复、独立生命周期和 18 个真实默认查询，统一入口两组通过。见[组合范围及证据](./39-cache-and-web-targets.md)；这仍不是正式平台五类门禁，其他类型与组合保持待完成。

ZooKeeper 3.9.6 standalone 的 mntr 实际数据、独立启停和六个默认查询已通过，见[协调目标证据](./40-rabbitmq-and-zookeeper-targets.md)。没有将 standalone 结果扩大成 ensemble、其他平台或正式部署通过。

后续补齐 DNS UDP/TCP/A 与 Linux 非 root ICMP 的实际目标、故障/权限恢复、独立启停及默认查询，见 [53](./53-native-network-and-kafka.md)。DNS 0.158 是空实现，已统一升级 Collector 0.159.0；development 稳定性、其他协议/记录/平台、IIS 实测和正式门禁分别保留。
