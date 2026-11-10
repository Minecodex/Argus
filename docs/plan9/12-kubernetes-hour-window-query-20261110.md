# Kubernetes 一小时仪表盘查询修正

2026-11-10，用户反馈节点网络、API Server 后部面板显示 `QUERY_BUDGET_EXCEEDED`，同时 API 延迟图显示数十秒。

## 实际复现

使用正常企业登录，对已有集群执行与门户相同的最近一小时请求。节点仪表盘的前几个查询每项读取约 52–58 MB，累积耗尽 256 MiB 整板预算；API Server 平均延迟面板单次读取约 145 MB，耗时约 1.57 秒，后面的查询被预算围栏中止或跳过。原始指标仍在持续采集，两个既有投影也完整物化。

前次短历史验收覆盖了真实采集与面板取数，但未覆盖同一来源积累完整一小时、多指标并存后的读取成本。本轮补充这种数据分布及真实一小时页面回归。

另按 verb 核对实际样本：当时普通请求平均约 2.5 ms、P95 约 17 ms，WATCH 连接平均持续约 501 秒。原总体延迟将长连接持续时间一起聚合，不能把图中的数十秒理解为普通 GET/LIST 请求耗时。

## 统一读取路径

- 由授权且经 FINAL 选定的序列元数据提取真实物理指标名，并作为类型化参数与序列 ID、来源、时间共同裁剪样本。直方图虚拟 `_sum/_count/_bucket` 仍对应其基础物理指标，普通同名指标保留原语义。
- 新投影按 `(source_key,metric_name,timestamp,series_id,ingest_key)` 排序，覆盖数值与完整 Histogram/Summary 两种载荷。原有来源/时间投影保留，以支持滚动发布中的旧查询程序；原始样本、字段契约、TTL、权限和预算不变。
- Histogram/Summary 的虚拟 sum/count 只读取数值和 count/sum，省去未使用的桶、分位数与指数直方图数组；直接查询 Summary、真实名称恰好以 `_sum/_count` 结尾及原生 Histogram 的路径保持完整载荷。
- 直方图展开复用单次查询内不可变的来源标签和已创建序列，避免为每个样本、每个桶重复分配相同标签。桶边界和 Summary 分位数发生变化时仍创建对应的新序列。

这属于同一 Query 服务的访问路径与分配优化，不增加进程或查询引擎，不改变授权裁剪，不上调整板预算。

## API 延迟模板

`kubernetes.apiserver` 升级为不可变模板版本 2。平均耗时/P95 明确排除 WATCH、WATCHLIST、CONNECT、PROXY，以毫秒显示；中英文标题标明“不含长连接”。请求总量保持其原有统计口径，原始长连接样本仍可查询。系统受管仪表盘随模板升级，个人副本维持独立。

指标语义参考 [Kubernetes Metrics Reference](https://kubernetes.io/docs/reference/instrumentation/metrics/)，物理排序与列裁剪参考 [ClickHouse Projections](https://clickhouse.com/docs/data-modeling/projections)。数值、来源与时间以本轮实际采样为准。

## 验证与证据

隔离 ClickHouse 覆盖同来源同一小时混有 80 万其他指标样本的读取，及宽桶 Histogram 的 sum/count、小预算、真实后缀名称、变化的桶边界/分位数和模板长连接过滤。受控数据只写入独立随机租户并清理，不作为线上采集证据。

正式验证覆盖节点、API Server 两套仪表盘最近一小时的 12 个面板、中英明暗桌面页面和发布后的冷查询耗时/扫描量。证据目录 `artifacts/kubernetes-query-20261110/`；完成后的版本、对照与清理结果在本记录补记。

## 发布后结果

| 检查 | 修正前 | 发布后实测 |
| --- | --- | --- |
| 节点一小时仪表盘 | 4/6 完成，后部触及预算 | 6/6 实际取数，完整响应约 0.57 秒 |
| API Server 一小时仪表盘 | 3/6 完成，后部触及预算 | 6/6 实际取数，完整响应约 1.79 秒 |
| API 平均延迟面板查询 | 约 1.568 秒、145 MB | 约 0.210 秒、62 MB |
| 延迟显示口径 | 所有 verb，含长连接 | 普通请求，以 ms 显示，标题明确排除长连接 |

以上是本次数据分布和窗口的实测结果；总响应包含已完成的全部面板，不能把修正前的未完成响应当成同等工作量。整板 256 MiB 上限保持不变。另对截图对应的北京时间 **2026-11-10 07:35–08:35** 历史窗口执行，12 项同样完成，原始历史数据未重写。

Query/Server 发布为 `local-20261110-k8s-query-window@sha256:6e3993b420b6ca71eb09fd4efaa9d73f55eb70c42940be39467b03ffab88cb8a`，Web 为 `local-20261110-k8s-query-window@sha256:413eb96c8de36eaeced4082f2563cc7f493cfa36aaf90b15d9f6abef6c846aaf`。在当前线上 Schema 52 的源码基线上只叠加查询、投影和内置模板变更，未引入 PostgreSQL 迁移。三个 Deployment 的精确摘要、实际 imageID 与 Rollout 核对；其余部署和 6 个 PVC 绑定一致。

- 4 项真实浏览器 E2E 无重试、无跳过通过，覆盖中英明暗、1280/1920 桌面，核对请求实际为 3600 秒、两个仪表盘共 12 个有限数值面板、没有预算错误、模板版本 2 和 ms 单位。
- 全量 Go、隔离 ClickHouse 的完整 Telemetry/QueryEngine/Dashboard、417 项相关前端单测、类型、ESLint 和隔离生成契约检查通过。
- 最终 12 项查询的原始最新样本均在一分钟内，8 项采集 healthy/fresh。健康 20/20；隔离数据库容器和匿名卷已按身份回收。

主要证据：`checks.json`、`before-1h-summary.json`、`final-live-1h-summary.json`、`after-reported-hour-summary.json`、`request-latency-by-verb.json`、`projections-materialized.json`、`browser-review.json`、`real-browser.log`、`release.json`、`backend-source.json`、`web-source.json`、`health.log`、`backend-tests-final.log`、`go-full.log`、`frontend-tests.log`、`contracts.log` 和 `isolated-database-cleanup.json`。实际截图位于 `real-browser/`。首次回归失败与后续通过日志保留。

同日 Collector 轮换复验发现外部表 JOIN 仍会阻断当前 ClickHouse 的投影选择；完整物化不能替代实际执行计划验证。后续修正与真实一小时回归见 [统一续期与投影读取复验](./13-collector-identity-renewal-20261110.md)。
