# Plan9：分批真实场景与人工验收

创建日期：2026-10-26。当前阶段为 **B10a 已退役，B10b 存储、任务、真实 CPU HPA 和 NetworkPolicy 已部署，等待用户查看**。三项原生采集、四个仪表盘共 24 个面板、实际告警触发/恢复、六项部署页面 E2E 和正式健康 20/20 通过。

## 要完成的事情

每次搭建一小批真实环境，完成 Argus 接入、采集、仪表盘、告警及适用的聊天/远程连接，再交给用户进入正式门户检查数据、结果和样式。发现问题就在当前批次修复并复验；用户确认并要求下一批后，销毁当前批次的专属运行环境，继续下一批。

```text
列出批次 → 用户指定部署 → 准备并验证该批 Compose/脚本
  → 启动实际服务 → 正常接入 Argus → 实际负载与故障/恢复
  → 自动核验 → 保留环境，交付页面入口 → 用户查看
       ├─ 有问题：保留现场 → 修复 → 同批复验 → 再交用户查看
       └─ 通过且要求下一批：封存证据 → 清理本批 → 核验清理 → 下一批
```

这是一套按需执行的人工验收包。每个批次都必须有自己的运行记录，不能用 Plan8 的历史成功记录代替本次接入和取数。

## 本轮交付和后续交付

B09a 使用独立 Kubernetes 集群运行实际业务工作负载，接入原生集群与节点指标、持久容器日志、实时 Pod 日志及告警。平台入口与事件时间见 [B09a 查看与运行记录](./batches/B09a-review.md)。

B09b 已完成真实控制面组件验证并按用户下一批指令退役，修复结果和历史见 [B09b 记录](./batches/B09b-review.md)。

B10a 已清理，原始反馈与修复证据保留。[当前 B10b 查看入口](./batches/B10b-review.md) 提供真实 PVC 在线扩容、任务暂停/恢复/手动执行、CPU HPA、网络隔离、四个仪表盘和已恢复告警。

2026-11-10，按完整一小时反馈补齐[节点/API Server 查询预算与延迟口径修正](./12-kubernetes-hour-window-query-20261110.md)，12 个面板、4 项真实中英明暗页面及健康 20/20 通过，当前版本见该记录。

用户指定某批后，先在本目录补齐该批可运行文件并验证，再执行部署；无需用户另外手工安装中间件、填仪表盘或配置告警。缺少实际登录、模型凭据、VM 等必要条件时，明确指出缺什么，继续完成不依赖它的准备工作。

| 文档 | 用途 |
| --- | --- |
| [批次与场景目录](./01-batch-catalog.md) | 13 个主批次、子批次、仪表盘和实际故障 |
| [每批执行与清理流程](./02-execution-workflow.md) | 部署、接入、交付查看、修改、通过、销毁 |
| [Compose 与环境规范](./03-environment-and-compose.md) | 文件组成、主机/中间件/K8s/模型边界、资源与凭据 |
| [人工检查与证据要求](./04-review-and-evidence.md) | 用户看哪些内容，如何证明数据和结果真实 |
| [B01 Linux 主机详细方案](./batches/B01-linux-host.md) | 第一批的对象、负载、告警、查看路线和完成条件 |
| [B01 验收归档（已退役）](./batches/B01-review.md) | 新账号、实际页面、当前故障、时间线和自检 |
| [B01 修复与复验](./batches/B01-issues.md) | 查询、停采、仪表盘与告警样式反馈 |
| [B02a 验收归档（已通过并退役）](./batches/B02a-review.md) | 六个目标、当前 HAProxy 故障、历史窗口与真实验证 |
| [B02a 执行与问题记录](./batches/B02a-issues.md) | 已修复项与保留的历史投递缺口 |
| [B02a Compose 和运行流程](./runtime/B02a/runbook.md) | 可重复构建、接入、负载、故障、恢复与清理顺序 |
| [B02b 实际查看入口](./batches/B02b-review.md) | 十个探针、当前 HTTP/TLS 故障与历史窗口 |
| [B02b 运行包](./runtime/B02b/runbook.md) | 原生服务、测试证书、真实故障与恢复流程 |
| [B03 查看入口](./batches/B03-review.md) | 四数据库实例、真实告警历史与诊断缺口 |
| [B03 运行包](./runtime/B03/runbook.md) | 数据库、协议客户端、独立凭据与有界负载 |
| [B04 查看入口](./batches/B04-review.md) | 缓存命中、TTL、淘汰、历史告警与查看窗口 |
| [B04 运行包](./runtime/B04/runbook.md) | 真实协议客户端、有界压力、独立停采和清理 |
| [缓存视图和元数据扫描修正](./08-b04-cache-views-and-metadata-scan.md) | 空闲比率语义、全局查询优化和真实回归 |
| [B05a 查看入口](./batches/B05a-review.md) | RabbitMQ 队列、vhost、真实积压和确认告警 |
| [B05a 运行包](./runtime/B05a/runbook.md) | AMQP 客户端、有界队列、恢复保护和清理 |
| [B05b 查看入口](./batches/B05b-review.md) | Kafka Topic、固定组、提交位点和实际 lag |
| [B05b 运行包](./runtime/B05b/runbook.md) | SASL 权限、原生客户端、有界暂停与清理 |
| [B05c 查看入口](./batches/B05c-review.md) | ZooKeeper 会话、Watch、临时节点与告警历史 |
| [B05c 运行包](./runtime/B05c/runbook.md) | 原生客户端、有界压力、会话回收和清理 |
| [B06a 查看入口](./batches/B06a-review.md) | MongoDB CRUD、连接、操作和告警历史 |
| [B06a 运行包](./runtime/B06a/runbook.md) | 最小权限、原生客户端、有界负载与清理 |
| [B06b 查看入口](./batches/B06b-review.md) | Elasticsearch 索引、yellow 原因和真实告警历史 |
| [B06b 运行包](./runtime/B06b/runbook.md) | 权限、私有密码文件、有界场景及清理 |
| [B06c 查看入口](./batches/B06c-review.md) | SQL Server 事务、连接、锁等待和告警历史 |
| [B06c 运行包](./runtime/B06c/runbook.md) | 最小权限、真实 TDS 客户端、速率口径与清理 |
| [B06d 查看入口](./batches/B06d-review.md) | Oracle 事务、会话、提交速率和告警历史 |
| [B06d 运行包](./runtime/B06d/runbook.md) | 受支持认证组合、最小授权、有界负载与清理 |
| [B07a 查看入口](./batches/B07a-review.md) | 实际 HTTP 链路、SDK 三信号和告警窗口 |
| [B07b 运行归档](./batches/B07b-review.md) | OTLP gRPC 三信号回执、告警窗口和清理状态 |
| [B07c 运行归档](./batches/B07c-review.md) | Jaeger Trace、关联日志、告警窗口和清理状态 |
| [B08 查看与运行记录](./batches/B08-review.md) | 文件日志、Journald、轮转、续采与告警窗口 |
| [B08 运行包](./runtime/B08/runbook.md) | 独立规则、故障、实际浏览器与清理 |
| [B09a 查看与运行记录](./batches/B09a-review.md) | 集群、Namespace、容器日志与告警入口 |
| [B09a 运行包](./runtime/B09a/runbook.md) | 独立 Kind、原生接入、真实故障与清理 |
| [B09a 接入修正](./10-b09a-direct-collector-target.md) | HTTPS 目标、凭据事务和候选 Agent 挂载 |
| [B09b 查看记录](./batches/B09b-review.md) | 五项组件、30 个面板、独立启停及故障/告警恢复 |
| [B09b 运行包](./runtime/B09b/runbook.md) | 自有端点准备、只读权限、有界活动、正常 API 与真实浏览器 |
| [B10a 查看记录](./batches/B10a-review.md) | 常用对象、真实变更、日志/终端、23 面板与告警历史 |
| [B10a 运行包](./runtime/B10a/runbook.md) | 独立集群、正常预览/确认、实际会话、有界负载和清理 |
| [B10b 查看记录](./batches/B10b-review.md) | 存储扩容、Job/CronJob、CPU HPA、真实 NetworkPolicy 与告警历史 |
| [B10b 运行包](./runtime/B10b/runbook.md) | 固定版本 CNI/CSI/metrics-server、正常平台操作、有界活动及先回收卷再销毁 |
| [B10b 节点证书和操作结算](./19-b10b-storage-jobs-and-node-ca-20261112.md) | 受管公开 CA 文件、错误 CA 拒绝、后续编辑替代旧观察与正式发布 |
| [B10a 终端可用名单与展开选项](./18-b10a-terminal-matrix-and-log-options-20261111.md) | 17 个 Pod 的权限分析、四个真实 PTY 和参数区对齐 |
| [B10a 安静日志与操作栏复查](./17-b10a-quiet-pod-logs-and-actions-20261111.md) | 截图 Agent 的真实历史日志、默认 tail、对齐操作栏与终端前件 |
| [B10a 日志和终端反馈](./16-b10a-pod-logs-and-terminal-20261111.md) | 日志自动打开与滚动、原生终端拒绝诊断、实际 PTY 与重连 |
| [B10a 生命周期修复](./15-b10a-workbench-lifecycle-20261111.md) | Secret 引用、操作分页、凭据交付后的会话与短命 Namespace 归属 |
| [B09b 直方图查询修正](./11-b09b-component-metrics.md) | 大序列参数、完整物理投影和可恢复物化 |
| [一小时查询与延迟口径修正](./12-kubernetes-hour-window-query-20261110.md) | 同源多指标裁剪、直方图按需读取、长连接排除及正式复验 |
| [Collector 身份续期与连接重建](./13-collector-identity-renewal-20261110.md) | 唯一续期者、投影同步、注册窗口与真实轮换 |
| [B09b 投递与当前告警修复](./14-b09b-delivery-and-current-alerts-20261110.md) | 回执事务重试、持久队列失败分类和未触发状态过滤 |
| [B07 运行包](./runtime/B07a/runbook.md) | 服务、TLS/Key 接入、负载与清理 |
| [B07a 接入与查询修正](./09-b07a-application-source-capability.md) | 应用来源契约、拓扑预算与重复读取复用 |
| [状态清单](./progress.json) | 规划状态、支持范围映射及后续运行索引 |
| [本地业务数据重置与账号重建](./05-local-data-reset-20261026.md) | 用户授权范围、清理结果、新账号和验证 |
| [B01 默认仪表盘与查询修正](./06-b01-managed-dashboard-and-query-fixes.md) | 实际接入发现的问题、统一实现修正及证据范围 |
| [仪表盘与告警详情统一修正](./07-dashboard-and-alert-review-fixes.md) | 本轮人工反馈修复、真实取数与界面复验 |

## 建议批次顺序

| 批次 | 主要内容 | 拆批方式 |
| --- | --- | --- |
| B01 | Linux 主机、进程、systemd、Collector | 第一批先跑通最小完整链路 |
| B02 | Web/代理和网络探测 | B02a Nginx/Apache/HAProxy；B02b HTTP/TCP/TLS/DNS/ICMP |
| B03 | MySQL、PostgreSQL | 先单目标，再双目标筛选 |
| B04 | Redis、Memcached | 正常缓存、未命中、容量和恢复 |
| B05 | 消息队列与协调服务 | B05a RabbitMQ；B05b Kafka；B05c ZooKeeper |
| B06 | 扩展数据库/搜索 | B06a MongoDB；B06b Elasticsearch；B06c SQL Server；B06d Oracle |
| B07 | 应用、服务、APM、SDK 日志 | B07a OTLP HTTP；B07b OTLP gRPC；B07c Jaeger |
| B08 | Linux 文件日志、Journald | 真实文件写入、轮转、服务日志、停采/恢复 |
| B09 | Kubernetes 监控与日志 | B09a 工作负载；B09b 控制面可用性与主机/节点去重 |
| B10 | Kubernetes 工作台操作 | B10a 常用对象；B10b 存储/任务；B10c Helm/CRD/节点治理 |
| B11 | 堡垒机与远程连接 | B11a 直连；B11b 内网中继；B11c 隧道和恢复 |
| B12 | 真实模型聊天与操作闭环 | 主机/指标问答、Workspace、告警分析、确认后操作 |
| B13 | 多目标仪表盘、告警、权限与生命周期 | B13a 范围；B13b 告警；B13c 撤权/归档/清理 |
| E01（按需） | Windows、IIS、Event Log、RDP | 单独准备真实 Windows VM/主机 |
| E02（按需） | ARM 主机 | 单独准备真实 ARM 执行目标 |

默认只运行一个子批次。表中的组合不代表同时启动全部组件；子批次同样执行“交付查看—用户反馈—清理”循环。允许用户调换顺序，每批自行重建所需基础设施，不依赖上一批残留资源。

## 不变的执行规则

1. 数据来自实际服务、请求、SQL/协议操作、SDK 埋点、操作系统和远程会话。禁止直接写 Argus 业务库/遥测库、伪造告警或模型结果、mock API、回放旧样本冒充当前运行。
2. 正常安装 Connector/Collector，通过门户或正式 API/Tool 的预检、预览、确认和领域流程接入。平台返回的真实 ID、版本、状态才是依据。
3. 每批都有指标/来源核对、适用仪表盘、至少一个真实触发与恢复的告警、清晰的查看入口。日志/远程类批次复用本批主机或探测指标告警，不把未支持的日志告警当成已有能力。
4. 用户验收前保持环境和低负载数据源；有界故障自动恢复时保存绝对时间窗口，并说明当前已经恢复，不能为了等查看持续压满机器。
5. 当前正式 Argus、业务数据、共享 OpenSandbox、存储和网络基础设施保留。只处理本批清单内、所有权核实的测试资源。
6. 仅验收桌面 Web。中英、明暗、统一组件和样式都纳入每批检查。

## 与既有计划的关系

复用 [Plan8](../plan8/README.md) 的采集、应用、工作台和默认仪表盘，[Plan6](../plan6/README.md) 的告警，[Plan7](../plan7/README.md) 的 Workspace/会话，以及[主机接入](../19-cross-platform-host-onboarding.md)、[卸载生命周期](../21-host-and-bastion-removal.md)。不增加平行采集平台或绕开原有架构边界。

Plan8 的确认范围内功能实现已完成；现有文档仍单列 P8-A64 历史目录保留证明缺口。Plan9 不追溯修改其验收结论，也不把该缺口扩成每批重新执行整个 Plan8 的前置条件。

[2026-10-26 清理记录](../local-formal-deployment-20261026.md)说明旧测试主机和独立测试集群已删除，历史平台记录仍保留。本计划不使用旧资源 ID 或历史在线状态作为当前前件；部署每批时重新确认实际版本、登录、资源和网络。

## 用户指令与对应动作

| 用户表达 | 执行动作 |
| --- | --- |
| “部署 B01”或“开始第一批” | 补齐并验证本批运行包，完成接入/负载/仪表盘/告警，自检后交付查看 |
| “这里有问题，修改……” | 记录问题，保留环境，修复并验证同一场景，更新查看入口 |
| “这一批通过” | 记录用户通过，保留环境，等待清理或下一批指令 |
| “通过，下一批” | 封存并清理当前批次，核验后启动顺序中的下一子批次 |
| “销毁当前批” | 清理当前批次并验证，保留可审计历史与证据，不自动开始下一批 |
| “跳过这一批” | 记录 skipped 及原因，不记为通过；是否启动其他批次按用户指令执行 |

不以自动化通过代替用户确认，不因用户一段时间未回复而销毁环境、宣告通过或启动后续批次。
