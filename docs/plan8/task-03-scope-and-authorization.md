# P8-T03：统一范围解析与显式授权

状态：in_progress。前件：P8-T02。

## 设计与接点

- [02-domain-data-and-authorization.md](./02-domain-data-and-authorization.md)
- [06-dashboard-scope-and-integrations.md](./06-dashboard-scope-and-integrations.md)

已有/拟扩展接点：`internal/authorization`、`internal/telemetry`、`internal/dashboard`、`internal/alerting`、`internal/storage/postgres`、`web/packages/api-client`。新领域包和具体接口在 T01 固化；路径是定位入口，不代表本任务已实现。

## 实施清单

- [ ] 实现Namespace UID显式授权、Cluster全范围继承及独立运维日志能力；所有目录/查询/会话/工具执行相同范围，不借Helm或动态GVR扩权。
- [ ] 扩展 Application、MonitoringTarget、KubernetesNamespace 授权根，服务继承应用；Cluster/Namespace 范围与动作权限分别检查。
- [ ] 建立单个 ScopeResolver 与类型适配器，显式区分 All、空集合、未授权、未匹配、不适用与来源不可用。
- [ ] 实现关系分支中的环境推导、目标去重、服务范围和 K8s UID/Owner 过滤；执行 Collector 不扩大目标范围。
- [ ] 范围/候选/游标/缓存绑定主体、授权/关系版本、时间、来源能力与预算；冻结执行快照并在返回前复验撤权。
- [ ] 更新现有三语言查询、Data Catalog、Dashboard、导出和后台主体的接入接口，不保留 Host-only 旁路。
- [ ] 生成权限矩阵与授权管理入口；授权查询/候选计数不泄露不可见对象。

## 完成标准

- [ ] 真实数据库与查询引擎验证跨企业、跨应用、共享目标、混合类型、显式无权 ID、空范围、分页撤权和执行中撤权。
- [ ] 同一规范输入在资源页、Dashboard、告警与工具得到一致的资源/来源签名；不可因聚合后过滤漏出数据。
- [ ] 相关存储、接口/生成客户端、权限、门户/工具、审计、配置和文档保持一致；单文件小于 2000 行。
- [ ] 保存测试范围、失败迭代、源码/版本与证据；只完成 mock 或单层测试不计真实验收。

## 验收关联

对应 [验收矩阵](./09-acceptance-matrix.md)：P8-A01、P8-A02、P8-A03、P8-A04、P8-A46、P8-A47、P8-A48、P8-A49、P8-A50、P8-A51、P8-A06。

新增验收关联：P8-A65、P8-A66、P8-A70、P8-A71、P8-A73。

## 实施记录

当前实施与验证层级见 [实施记录](./14-implementation-log.md)。尚未完成本任务全部验收；同时更新 [progress.json](./progress.json)，不预填通过结果。

授权管理入口现支持 Host、Cluster、Dashboard、Application、MonitoringTarget 和 Namespace 六种授权根。目录使用 SQL 游标分页与搜索，门户仅加载当前类型/页，保存跨页变更差集；游标同时绑定请求主体和被授权主体版本。应用显示固定环境，Namespace 显示所属 Cluster 和真实 UID。直接授权与继承证据分别保留，角色绑定过期或角色停用不展示为有效继承。

继承来源查询也只查当前页资源及其父集群，不加载整个租户的角色绑定和授权对象；旧的全量目录实现已移除。角色编辑矩阵已补充应用、监控、各 K8s 管理能力、终端、转发和独立运维日志的中英文名称。

- [真实 PostgreSQL 目录验证](../../artifacts/plan8-authorization-catalog-postgres.log)：同名应用的环境区分、翻页、Namespace UID/父集群查找、直接授权与 Cluster 继承并存、同名 Namespace 重建不继承旧授权、过期角色绑定排除。
- [门户中英/明暗](../../artifacts/plan8-authorization-ui-e2e.log)：应用与 Namespace 授权、保存后回读、撤销和可访问性四场景通过；[旧仪表盘流程](../../artifacts/plan8-authorization-dashboard-regression.log)通过授权/撤销回归。两者为 mock 门户。
- [跨页与并发版本](../../artifacts/plan8-authorization-ui-unit.log)：保留跨页差集，不把继承转为直接授权；版本变化阻止保存。
- [契约与投影](../../artifacts/plan8-authorization-final-contract.log)：HTTP 校验器与领域类型注册表统一；Namespace-only 集群导航不返回集群节点统计和凭据/Connector 引用。
- [来源范围回归](../../artifacts/plan8-authorization-bounded-sources.log)：真实 PostgreSQL 验证有界目录来源和 Namespace 重建授权；[当前门户回归](../../artifacts/plan8-portal-current-regression.log)28 个 Plan8 mock 场景全部通过。

Namespace 目录目前由受信资源读取进行 UID 登记，后台持续同步与重建/删除的完整并发场景尚需完成。遥测行级 Namespace 范围、独立运维日志范围、全部查询入口/告警/Agent 的统一范围验收仍未全部完成，因此任务保持 in_progress。

Namespace 日志已加入统一来源解析，区分选择授权与实际存储身份；普通 Namespace 权限、应用授权、运维日志权限及自然到期分别验证。详见[查询范围记录](./16-namespace-log-query.md)。真实 PostgreSQL、ClickHouse 与受控查询分发各有独立证据；正式筛选、下钻/Workspace 与部署闭环仍需继续完成。
