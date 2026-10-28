# B04 缓存视图与序列元数据扫描修正

2026-10-27 的真实 Redis/Memcached 场景发现默认图缺少命中/未命中信息，以及完整仪表盘在累计查询预算内不能完成的问题。两项修正统一进入共享模板和 PromQL 存储适配层，不增加平行图表组件，不放大查询预算。

## 缓存指标与空闲语义

基于本次原生 Receiver 实际观测，以及锁定的 v0.159.0 receiver 元数据定义，Redis 默认模板增加命中、未命中、淘汰、到期速率和命中率；Memcached 增加 GET 命中、未命中速率及命中率。两类模板升级为不可变版本 2，生成契约、前端模板与中英文标题同步。

命中率按资源分别计算五分钟窗口内命中速率 / 总 GET 速率。总 GET 速率为零时保留无有效观测，不用 0% 或 100% 替代。首次接入后，真实累计计数保持不变的空闲窗口已通过正常查询证明：命中/未命中速率均为零，命中率没有序列。启动真实负载后才出现约 50% 的命中率。

## 大字段在过滤前被读取

真实页面中，前六个面板查询正常，后续面板报 `QUERY_BUDGET_EXCEEDED`。检查表明序列元数据查询的 `FINAL` 路径读取了同一 granule 中其他资源的 labels/resource_attributes 大字段。单次查询扫描约 40 MB，多个面板会耗尽原有 256 MiB 整板预算。

修正只把主键中不随替换变化的 `metric_name`、`resource_id` 条件提前到 `PREWHERE`。完整 WHERE、来源与 Namespace 权限条件、FINAL 去重、指标类型选择、逻辑序列合并与 Kafka 冲突顺序均保留。直方图后缀预过滤使用精确指标名或基础名的宽集合，类型判断仍在 FINAL 之后进行，避免把普通标量基础指标当成直方图。

这属于现有查询读路径的优化，无存储 Schema/权限边界变更，不修改实际指标、采样步长、允许资源集或预算上限。

## 验证边界

- 真实 B04 元数据只读对照：相同查询结果由 35,954,465 bytes 降至 10,712,510 bytes；保留各步骤观察，不将探测当成完整仪表盘验收。
- 隔离 ClickHouse 租户用同一 granule 中约 64 MiB 无关宽字段验证预算。原实现失败，修正后在 16 MiB 受控读取预算内返回同一值 7 和同一资源身份。块读取有固定开销，测试预算与实际产品的 256 MiB 整板预算分开。
- 原数值投影、已选序列、直方图后缀、来源集合、Namespace 和 PromQL 回归通过。受控测试只写独立随机租户并清理；B04 业务数据仍全部来自实际缓存协议。
- 用 B04 已采集的真实来源只读执行 Redis 11 个、Memcached 9 个面板，共 20 项在各自原有整板预算内完成。最终部署后的 API、浏览器和平台健康核验见 [B04 查看记录](./batches/B04-review.md)。

证据位于 `artifacts/plan9/p9-b04-20261027-01/`：`metadata-scan-probe.json`、`metadata-regression-baseline.json`、`metadata-regression-after.json`、`live-budget-after.json`、`idle-ratio-checks.json`、`before-metadata-fix-dashboard-checks.json`。首次失败的页面结果保留，不覆盖为成功。

本次发布使用隔离构建副本，保留已部署的 B03 基线修正并加入 B04 所需改动；同时进行的分页和应用/API 改动未打包或覆盖。构建文件清单见 `build-snapshot.json`。
