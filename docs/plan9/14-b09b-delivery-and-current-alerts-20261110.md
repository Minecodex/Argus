# B09b 投递与当前告警修复

运行 `p9-b09b-20261110-01`；保留原独立 Kind 集群、Collector、采集实例和业务负载。2026-11-10 按用户要求完成 B09b-D02 与 B09b-A01 修复、重新发布及完整复验，等待人工查看。原始失败证据保留，最终核验为 [final-review.json](../../artifacts/b09b-repairs-20261110/final-review.json)。

## 原因与修正

### 投递失败存在两条不同路径

原生 exporter helper 在关停时也会增加 `exporter_send_failed_*`，即使其持久队列明确保留该请求并在重启后重放。原配置的 `normal` 自监控等级合并掉 `error.type` 和 `error.permanent`，Argus 报告器因此无法区分这类失败与真正终止投递。

统一配置生成器现为强制回环指标开启 `detailed`，保留上游错误分类，并显式登记使用 `file_storage` 的持久导出器。仅当导出器在该登记范围内，且原生标签同时为 `Shutdown`、非永久错误时，本地台账记录为关停保留计数，不增加永久失败或最后失败时间。内存队列、缺失分类、永久拒绝、取消及五分钟重试耗尽仍按原规则记录。可选的 Collector 自身指标开关不影响该控制观察；外部报告协议继续使用 `argus.collector_delivery/v1`。

实际停采复验还暴露了真正的控制面失败：Direct Executor 已完成 Kubernetes 配置，但成功回执落库发生 `SQLSTATE 40001`。原代码只记录错误，操作随后变为 `result_unknown`，生效版本和来源准入未推进，排队请求最终被拒绝。

新增显式的 `Store.InTxRetry`，仅对序列化冲突和死锁做有界事务重试。Direct Executor 在远程操作完成后，使用它原子保存已有回执、配置版本、来源与路由投影；不重新执行远程安装或配置。原有操作 fence、当前期望版本、租户、资源和来源授权校验继续生效。普通 `InTx` 不改变语义，其他含外部副作用的调用不被自动重试。

复现阶段，累计指标点失败由 17 增至 **918**，最后记录为北京时间 **2026-11-10 21:41:48**。已通过正常 Preview/Confirm 修复异常配置并恢复采样；这 918 点是保留的历史累计证据，不能声称已补齐。后续复验必须保持这一累计值不再增加。

随后逐条日志核对还发现，普通停采将 `desired_enabled` 立即用于准入，拒绝了仍在原有效配置队列中的数据；部分终止错误发生在最后一次周期观察之后，因此“累计值没增加”不足以证明没有丢弃。现改为按已生效版本和分配代次准入，待运行时停采成功后才关闭旧版本，授权撤销仍即时生效。身份扩展在流水线停止、遥测监听器尚可用时，完成有界的最后一次观察并先持久化再上报。优雅关停已覆盖；强制终止或断电仍有原先的观察窗口限制。最终验收同时检查本地持久台账和实际终止丢弃日志，保留前轮已捕获但未进入台账的日志事实。

### 未触发的 no_data 条件状态混入当前告警

聚合告警的指纹包含参与来源集合。原退役判断只能发现来源消失，遗漏来源新增；停采后重新开启会留下旧分组，之后被更新为 `normal/no_data`。当前列表又以“生命周期异常或健康异常”过滤，把这些没有触发时间的内部状态展示为告警。

退役判断现比较完整的参与来源 ID/安装代次集合，同时保留资源与显式来源范围。来源增减和安装代次变化均通过原有 `source_changed` 路径关闭旧分组。当前列表、分页总数和概览统一只统计 `pending`、`firing`、`recovering`；真正已触发的无数据健康告警仍会显示，已触发事件历史继续保留。

### 完整一小时页面复验的查询修正

配置切换增加了历史来源版本，导致少量实际选中的指标也回退到大来源 JOIN。样本读取现使用已授权 FINAL 元数据实际选中的来源子集，完整授权集合仍先约束元数据。

同一表达式内、相同标签条件和查询提示的直方图/摘要 `sum/count` 共享一次元数据和样本读取，分别返回独立名称和游标的视图。仅当两个操作数都被请求才启用复用；普通同后缀指标和原生族并存、单独查询及不同过滤条件保持原语义。桶视图只读所需字段，完整摘要和指数直方图继续读取完整载荷。

旧索引每个标记默认覆盖 8192 行，在包含许多短配置版本的大保留分片中，即使匹配少量样本也会读取相邻指标和宽载荷。新增 `argus_payload_source_metric_time_128` 物理投影，保持相同原始值、顺序和身份，以 128 行粒度限制读取放大；旧两项投影保留。初始化、缺失检查和可恢复物化一并维护这项投影，整板 256 MiB 预算不变。投影独立设置来自已锁定 ClickHouse 版本支持的 [官方投影设置](https://presentations.clickhouse.com/2025-release-25.12/index.html)。

## 实现与发布

- Server、Worker、Direct Executor、Ingest：`host.docker.internal:5001/argus/argus-backend:local-20261110-b09b-repairs-v5@sha256:48dd9340cabbbbdcbc79fdb577cd8f9d4db149bb88199203d15ca0a2d3d053c3`。
- Query：`host.docker.internal:5001/argus/argus-backend:local-20261110-b09b-query-selection-v5@sha256:473324817af72fd76cdff1840f383bfc3a9387b8cad260321e0bd219800c6681`。
- Collector：`host.docker.internal:5001/argus/argus-otelcol:local-20261110-b09b-repairs-v2@sha256:f15e3b4f64d6d7202161ab392114e44d9c764cc11db4324c5d406cecd40a89d0`。
- 后端从当前线上 Schema 52 的冻结源码叠加本次修改构建，不引入仓库其他迁移。节点 containerd 的精确引用、实际 Pod imageID、正常 Collector Preview/Confirm 升级均核对；默认镜像和三个配置生成进程同步更新。
- Web、Writer 等其他服务保留；6 个正式 PVC/PV 绑定和本批身份 PVC 不变。临时回归数据库、原生队列测试容器及 Linux 验证 Namespace 按所有权清理。

## 验证与证据

已通过的组件检查：告警模块真实 PostgreSQL 44 组、Direct Executor 35 组、监控模块 39 组，后两模块各有一个不属于本批的独立实机门禁未配置而跳过；原生身份扩展 Linux 19 组无跳过，实际 Collector 加载 10 种完整配置，真实 CPU 采样在故障与关停后精确重放 3 轮。新增数据库测试真实触发 `40001`，确认回执事务回滚后重试、来源准入推进且版本只提交一次，并验证停采生效边界、归档与分配撤销。隔离 ClickHouse 37 组无跳过，包含成对标量、同后缀普通指标并存、固定共享粒度的宽载荷列裁剪，以及租户/资源/来源/Namespace 隔离。另有 Go vet、隔离契约生成、TypeScript 和 E2E ESLint 检查。

物理索引的最后一个保留分片约 1.37 亿行，补齐用时约 685 秒；没有删除或注入原始样本。原失败的固定窗口（UTC 13:12:58–14:12:58）与当前完整一小时，API Server 六个面板均完成并取得有限数值；整板实测分别扫描 244115668 / 254919718 字节，原 268435456 字节上限不变。固定窗口保留真实历史空档，不能据此说过去丢失已补齐。

部署复验使用真实 CoreDNS 停采/恢复、精确出口网络故障和 Gateway 重启，以及同一来源的 Scheduler 抓取告警。真实浏览器检查中英、明暗、五套默认仪表盘、30 个面板、列表与概览计数及页面停采恢复。最后继续观察完整五分钟重试窗口，不以刚恢复的新样本代替后续丢失检查。

最终 5 项真实浏览器 E2E 全部通过，无重试、无跳过。页面停采后继续观察 **326.91 秒**，包含同一 Scheduler 条件告警在北京时间 **23:31:00** 触发、**23:32:00** 恢复；所有临时故障规则已移除。8 项采集 healthy/fresh、30 个面板有有限数值，五条组件策略评估正常。终止失败、入队失败、接收拒绝增量均为 **0**；对最终阶段原生日志的独立核对没有终止丢弃。持久报告身份保持，本地台账另记录 1552 个持久队列关停保留点，未将其计为永久丢失；精确批次重放另由 3 轮原生 CPU 测试验证。

当前列表不再包含未触发的 `normal/no_data` 条件记录，列表、分页总数和概览一致。当前仍有 **1 条**保留的历史投递缺口事件，其累计已观察失败为 918 点；新的正常采样不会伪造该历史空档恢复。平台健康检查 **20/20** 通过，6 个原有 PVC/PV 绑定及非目标服务保留，临时测试资源均已清理。

证据目录 `artifacts/b09b-repairs-20261110/`：`final-review.json`、`source-audit-final.json`、`alert-postgres-regression.log`、`outcome-postgres-regression.log`、`stop-postgres-regression.log`、`native-replay-test.log`、`distribution-validation-v2.json`、`linux-identity-v2.log`、`linux-renderer-v2.log`、`clickhouse-integration-verified.json`、`release-v5.json`、`query-release-v5.json`、`collector-release-v2.json`、`fine-projection-release.json`、`hour-window-final.json`、`browser-review.json`、`real-browser/`、`health-final/verify.json`。首轮真实失败保留于 `final-live/`；正确停采与出口重启证据在 `final-live-v3/`，最终页面、326 秒观察和日志检查在 `final-watch/`。
