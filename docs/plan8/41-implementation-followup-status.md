# 完成度审计后的实施进展

2026-10-19，响应继续完善剩余内容。**Plan8 仍在实施，未统一发布。** 本记录接续 [32 完成度审计](./32-implementation-audit.md)，把后续新增实现与旧缺口分开；不改写历史失败或验收状态。

本记录之后又完成 drain/PDB/取消及控制面成功采样/十八个查询，当前事实见[45 第二轮实施](./45-followup-delivery.md)。下列未完成列表和数量的验证边界按本记录时点保留。

## 本次已补内容

| 范围 | 当前事实 | 证据 |
| --- | --- | --- |
| Agent 新领域 | 20 个应用/监控/通用 K8s 工作台工具，Server/Worker 注册、分类协议 3、隐藏 Commit、严格公开输入、共享展示及带类型来源授权；Schema/权限等进入工具版本签名 | [33](./33-native-domain-tools.md) |
| 主机进程/服务 | 新增进程、Linux systemd、Windows 服务三个固定实例；Linux 实际状态/独立启停/隐私与 8 个默认查询通过，三平台构建通过 | [34](./34-host-process-and-service-metrics.md) |
| 统一验收入口 | e2e-plan8、具名测试、源码/Collector Hash、先脱敏后写盘、隔离后端归属检查；组件与正式验收分层，full 缺正式阶段返回 incomplete | [35](./35-acceptance-harness.md) |
| 控制面 | Scheduler/Controller Manager/etcd 配置、启停、模板和当前集群实际不可达/权限拒绝/诊断路径；没有伪造目标样本 | [36](./36-kubernetes-control-metric-availability.md) |
| PostgreSQL/探测 | PostgreSQL 17 只读身份、HTTP/TCP/证书期限实际目标，认证/故障/恢复/生命周期和 14 个查询；修复上游失败查询仍产生的零计数配置 | [37](./37-postgres-and-network-probes.md) |
| Workspace | 三类新资源只撤数据授权且不变认证版本，结果/文件/导出/目录/续约均拒绝；真实数据库，计算为明确 fake | [38](./38-workspace-typed-source-revocation.md) |
| 缓存/Web | Memcached 1.6.45、Apache 2.4.69、HAProxy 3.2.26 的具名原生组合、认证/生命周期及 18 个查询 | [39](./39-cache-and-web-targets.md) |
| 消息/协调 | RabbitMQ 4.3.6 monitoring-only、ZooKeeper 3.9.6 mntr、认证/生命周期及十二个查询 | [40](./40-rabbitmq-and-zookeeper-targets.md) |

当前固定指标定义 12 个、默认模板实现 24 套，应用 APM 视图另计。Windows 服务和三个控制面模板没有实际成功取数证明，不能因代码存在标为通过。原有与本次已验证面板累计 110 次实际目标查询包含 MySQL 双实例，不等于 110 个独立产品面板或正式门禁。

Collector 仍为 0.158.0，新增 systemd/windows service 使用上游内置组件，无 exporter 或 fork。当前 Linux amd64 二进制 SHA256 为 `ae399b246e920b1b8e14a85c70ac518b753c7103827bb08a70c54f83061b1ce9`；三平台构建不替代实际安装验收。

## 仍需开发及验收

1. K8s 的 Helm 生命周期、drain/PDB/存储影响、终端/实时日志/端口转发会话；成功 CSI 扩容与完整对象/正式 Connector 链路。
2. Kafka、MongoDB、Elasticsearch、SQL Server、Oracle、DNS、ICMP 等其余专用资源的实际支持组合与默认内容；IIS/Windows 实机继续用户暂缓。已有配置适配器不能抵扣这些交付。
3. Host/Node Claim、迁移/受控双采、完整失败回滚、正式注册/颁证轮换及 Gateway/Agent/Query 独立部署路径。
4. Namespace 指标可信对象身份、持续元数据权威同步和全部入口范围/撤权闭环。
5. 其他 SDK/协议组合、重复埋点/跨来源统计，Agent 的可信敏感宿主表单、实际 Workspace/PVC/模型和完整告警业务闭环。
6. 全部 76 项正式基础门禁、候选支持矩阵、适用版本/平台/拓扑五类门禁、迁移备份恢复、制品/镜像冻结、统一发布和回退。

11 类资源及 SkyWalking 原生协议继续按已有决定暂缓，没有新增豁免。Windows 实机未启动管理员准备、未安装服务/启用 IIS；仍为 deferred_by_user/not_verified。

## 验证与状态口径

十组源码冻结执行、resources/cache-web/brokers 分套件分别具有 manifest，路径见各专项记录。当前完整 full 明确 incomplete，即使全部已注册组件通过也不改正式 Axx 为 passed。

本次工程回归、门户 E2E、链接/单文件限制及最终自有资源核对见 progress.json 的最新检查；若记录仍为 pending，不从本文推断已完成。既有部署未替换，受保护业务数据库与命名空间未用于新目标采样。长驻的本轮隔离 PostgreSQL/ClickHouse 保留作为验收前件，自有临时目标和 Namespace 按 owner/UID 回收。
