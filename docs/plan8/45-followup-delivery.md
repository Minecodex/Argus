# 完成度复核后的第二轮实施

2026-10-19，接续[42 完成度复核](./42-completion-audit.md)。**Plan8 仍在实施，未统一发布。** 本次已补实际功能与真实组件验证，未将全部任务或正式门禁改为完成。

第二轮之后的逐任务复核与状态摘要校正见[46 当前完成度复核](./46-completion-audit.md)。

| 范围 | 本轮结果 | 证据 |
| --- | --- | --- |
| Node drain | 冻结 Pod/Namespace/控制器/存储，原生 dry-run、PDB 阻断和恢复、分批 Eviction、撤权停止、取消及失联恢复；保留 cordon，无强删或丢数据开关 | [43](./43-node-drain.md) |
| PDB 工作台 | 新增标准 PDB 类型，复用 Namespace 授权和审批变更，等待控制器 observedGeneration；内置对象类型变为 32 个 | [真实工作流](../../artifacts/plan8-drain-pdb-workflow-live.log) |
| 门户/Agent/API | drain 预览接入现有原生变更工具；门户双人审批和逐 Pod 状态、取消 API/界面；八个中英明暗及双宽度浏览器场景通过 | [八场景](../../artifacts/plan8-drain-browser-final.log) |
| 受保护组件指标 | 修复 API Proxy 移除 Authorization 导致匿名访问的问题，Scheduler/Controller Manager 改为直接 HTTPS、ServiceAccount 和严格证书校验；etcd 保留有认证 API Proxy | [44](./44-control-metrics-authentication.md) |
| 信任配置 | 按类型统一解析公开 CA/server_name，拒绝私钥、任意目标和跳过校验；四个中英明暗门户场景验证保存、拒绝、独立启停及版本刷新 | [四场景](../../artifacts/plan8-component-trust-browser-fresh-version.log) |
| 安装授权 | 仅在可审查的接入清单加入指标读权限和 kube-system 代理权限，运行时不创建 ClusterRole；真实 Token 验证业务日志/终端/Secret/写入仍拒绝 | [冻结执行](../../artifacts/plan8-control-complete-20261019-2550694980/manifest.json) |
| 默认面板 | 三个控制面实际取数及十八个查询通过；无样本分位数过滤 NaN，保留无数据语义；现有 24 套模板中仅 Windows 服务尚无正向取数证明 | [真实查询](../../artifacts/plan8-control-template-finite.log) |
| 验收驱动 | 自有 kind 双节点准备、实际 Docker ID/清单 Hash/证书准备、原配置恢复和所有权清理；具名 full 注册 23 组，新增两类分套件冻结记录 | [35](./35-acceptance-harness.md) |

当前新领域工具仍为 20 个、固定指标定义 12 个、受管默认模板 24 套。原有 110 次实际面板查询加本轮控制面十八次为累计 128 次目标查询，不等于 128 个独立面板或正式验收项。组件与查询证明不能扩大为正式版本/平台/拓扑支持矩阵。

## 尚未收口

- T11：Helm 全生命周期、按新授权模型实现的终端/实时日志/端口转发、CSI 成功扩容与全对象/正式 Connector 验收。drain 的独立取消尚未接入模型工具；已下发批次可能完成，不承诺撤回已有副作用。
- T03/T05/T06/T09：Namespace 指标可信身份与持续元数据同步、Host/Node Claim、迁移/受控双采、完整回滚及正式注册/颁证/独立部署链路。
- T12/T13/T14：Kafka/MongoDB/Elasticsearch/SQL Server/Oracle/DNS/ICMP 等其余实际组合和默认内容，以及全部支持矩阵门禁。
- T07/T08/T15：其他 SDK/协议、跨来源和重复埋点统计、完整日志组合、可信敏感宿主表单、真实 Workspace/PVC/模型及告警业务闭环。
- T16：76 项正式基础验收、内置能力及支持组合门禁、迁移备份恢复、制品冻结、统一部署发布与回退。

11 类资源和 SkyWalking 原生协议继续按决定暂缓。Windows 实机仍 deferred_by_user/not_verified，未执行管理员准备、服务安装或 IIS 启用。

## 最终检查

工程回归、生成契约、门户和所有权清理在本轮结束前统一记录到 progress.json。已有冻结记录仅指各自执行期间源码未变；后续文档或功能变化不自动获得旧运行的通过结论。先前失败日志和历史审计均保留。

[本轮最终检查](../../artifacts/plan8-second-followup-final-checks.json)：Go 全量回归、前端 1404 项、类型/lint/样式/i18n、生成契约检查和二十个当前门户场景通过。自有测试集群恢复原监听、确认 Pod Ready 后删除，实际节点容器剩余为 0；保留本轮隔离 PostgreSQL/ClickHouse 作为后续前件。argus-monitoring 与 docker-desktop 的 API readyz 均为 ok。正式模型/部署及全量基础门禁没有据此改为通过。
