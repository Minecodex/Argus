# P8-T14：仪表盘范围、模板框架与共享视图

状态：in_progress。前件：P8-T02、P8-T03、P8-T06。

审计后已新增自身能力、控制面和多类服务内容，当前实现 27 套受管默认模板；应用 APM 视图另计。主机进程/系统服务 8 个、PostgreSQL/探测 14 个、缓存/Web 18 个、RabbitMQ/ZooKeeper 十二个及三个控制面十八个新增实际查询通过。控制面无样本分位数保持无数据，见[44](./44-control-metrics-authentication.md)。Windows 服务正向取数仍未验证；其他服务内容、Namespace 指标和正式部署验收未完成。详见 README 中对应专项证据。

2026-10-20 当前补齐记录见 [57](./57-completion-delivery.md)：默认模板已增至 32 套；新增四类数据库 24 个原生查询与 Namespace UID 指标范围/持续目录组件证据。新版应用/资源筛选已在正式部署中通过中英明暗浏览器检查，真实主机查询、模型与统一发布仍在执行，不能由组件结果抵扣完整验收。下方较早记录保留历史时点。

## 设计与接点

- [06-dashboard-scope-and-integrations.md](./06-dashboard-scope-and-integrations.md)

已有/拟扩展接点：`internal/dashboard`、`internal/telemetry/queryengine`、`internal/telemetry/data_catalog.go`、`web/packages/ui`、`web/packages/api-client`、`web/apps/enterprise/src/components/dashboards`。新领域包和具体接口在 T01 固化；路径是定位入口，不代表本任务已实现。

## 实施清单

- [ ] 本期暂缓目录按[专用插件复查](./13-direct-receiver-review.md)与progress.json同步；资源无专用采集组件时不新建采集、不提供可用默认仪表盘，门户/API/Agent一致。
- [ ] 应用筛选按面板自动解析自身或当前关联资源，能查关系建立前历史指标；默认入口自动升级且运行中查询固定版本，用户副本不覆盖。
- [ ] 改造 Dashboard 定义、构建器、运行态、候选、下钻和默认模板，使其使用统一 ResourceRef/Scope 而非 Host/Cluster 固定枚举。
- [ ] 实现搜索分页应用/服务选择器和资源树/集合、环境、来源、K8s 对象过滤，保留空集合与无权请求语义。
- [ ] 面板声明资源/信号/来源能力与维度映射，区分不适用、无样本、来源关闭、未配置和权限不足。
- [ ] 建立 IntegrationDefinition→模板版本→受管默认入口/用户副本的生命周期，升级不覆盖用户修改。
- [ ] 复用现有发布确认、Data Catalog、三查询引擎、APM/日志组件，保留真实统计口径与作用范围摘要。
- [ ] 交付模板框架及真实多类型/三信号代表面板；各服务默认内容由 T09/T12/T13 按本框架补齐并独立验收。

## 完成标准

- [ ] 真实数据验证全局/局部、关系环境、K8s 子对象、不兼容过滤、无权/空范围与历史来源。
- [ ] 模板更新、用户副本、资源导航绑定、发布/撤权、缓存/游标失效和下钻保持一致；纯参考 JSON 不算内容交付。
- [ ] 相关存储、接口/生成客户端、权限、门户/工具、审计、配置和文档保持一致；单文件小于 2000 行。
- [ ] 保存测试范围、失败迭代、源码/版本与证据；只完成 mock 或单层测试不计真实验收。

## 验收关联

对应 [验收矩阵](./09-acceptance-matrix.md)：P8-A46、P8-A47、P8-A48、P8-A49、P8-A50、P8-A51、P8-A52、P8-A59。

新增验收关联：P8-A65、P8-A66。

## 实施记录

范围/关联基础已实施，模板与真实查询验收待完成，见 [实施记录](./14-implementation-log.md)；同时更新 [progress.json](./progress.json)，不预填通过结果。

正式查看页已增加应用筛选、资源类型筛选和分页资源弹框；应用/主机/Cluster/Namespace/监控目标使用同一受权目录，不再仅加载主机与集群作为可选范围。应用筛选支持“不限制应用”“全部有权应用”和显式集合；资源/应用清空后均保留 explicit 空集合，候选请求和缓存身份携带相同模式及应用条件。

- [真实 PostgreSQL 分页目录](../../artifacts/plan8-dashboard-scope-directory-postgres.log)：环境区分、分页、搜索、隐藏未授权应用、游标不能跨类型/条件复用。
- [65 个应用分页](../../artifacts/plan8-dashboard-many-applications-unit.log)：30/30/5 分页、搜索、去重及显式空查询。
- [门户 11 场景](../../artifacts/plan8-dashboard-filters-final-e2e.log)：新筛选中英明暗与既有仪表盘/资源快捷入口通过；[中文应用选择](../../artifacts/plan8-dashboard-application-filter-zh.png)、[英文空集合](../../artifacts/plan8-dashboard-explicit-empty-en-dark.png)保留。属于 mock 门户。
- [前端 241 测试](../../artifacts/plan8-dashboard-scope-filter-unit.log)、[API Client 108 测试](../../artifacts/plan8-dashboard-filters-api-client-tests.log)、[四包真实集成](../../artifacts/plan8-namespace-query-full-integration.log)分别通过。HTTP 权限回归补充已登记来源夹具，验证撤权返回权限拒绝，见[HTTP/原生工具一致性](../../artifacts/plan8-scope-directory-http-permission-verified.log)。

受管默认模板的版本、自动更新、个人副本、数据库只读保护和前三种原生指标模板已实施。22 组真实目标面板经 Writer/ClickHouse/正式 Dashboard 运行时验证，事后关联、共享去重、拒绝无权目标和显式空集合通过，见[默认模板与来源准入](./17-managed-defaults-and-sdk-admission.md)。门户快捷入口与个人副本中英明暗四场景通过，属于 mock 门户。其余资源、三信号内容、部署后完整链路与 Agent 验收仍待完成，T14 保持进行中。

后续新增 DNS、ICMP、Kafka，当前共 27 套受管默认模板。九个网络目标面板、八个 Kafka 面板使用同批实际数据查询，全丢包时四个 RTT 面板即时结果为空，见 [53](./53-native-network-and-kafka.md)。其余服务内容、Namespace 指标、告警和正式部署闭环继续未完成。
