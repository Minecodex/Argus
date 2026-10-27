# Plan8 实施记录

开始日期：2026-10-18；继续实施：2026-10-19。当前阶段：T01～T07 的领域、授权、凭据、多目标运行时和 SDK 接入，以及 T12/T13/T14/T15 的集成与门户基础正在实施。未达到全量发布条件。

## 已确认的执行补充

- 官方 Journald Receiver 可使用系统自带 journalctl；独立 exporter 与外部 JMX 采集服务仍不纳入。
- Windows 11 正式接入纳入本期，使用本机真实验证。Windows Server 保留实现，本期 Server 实机项单列未验证，不是发布门禁。
- 开始时工作区已有本轮 Plan8 与原型文档，保持原位置与未提交内容，不回退其他工作。
- 本机当前非管理员，IIS/服务安装前件尚不满足；不以受控组件测试替代正式接入验收。

## 进行中的改动

- 新增 00021 数据库迁移及应用、服务、目标、关系、Namespace、采集实例/版本、执行分配、接入 Key、凭据草稿查询。
- 应用环境不可原地变更，跨企业关系由复合外键/约束校验；迁移回退在已有 Plan8 数据时明确拒绝静默丢数据。
- 监控凭据沿用 Secret 的加密与 owner 隔离机制，增加 monitoring 用途，不建立另一套明文凭据存储。

## 验收状态

尚无本期生产门禁通过结果。以下结果分别记录验证层级，不能替代 76 项基础场景及支持组合真实验收。

| 范围 | 结果与证据 | 验证层级 |
| --- | --- | --- |
| 应用、环境固定、服务身份、租户 FK、Namespace UID 与继承授权 | disposable PostgreSQL 的 monitoring 集成测试通过 | 真实数据库，受控领域场景 |
| 两个 MySQL 采集实例的创建、独立启停、事务回滚、私有凭据、重复请求、归档回收、Outbox | [测试日志](../../artifacts/plan8-collections-telemetry-tests.log)；运行时交付在这些事务测试中显式替身 | 真实数据库；不证明真实 Collector 采集 |
| 应用门户 | 4 个桌面 E2E：zh/en × light/dark，创建生产/测试应用、编辑、详情、无服务状态、严重可访问性问题为零；[截图目录](../../artifacts/plan8-application-ui-20261018) | mock API 门户；不算真实平台验收 |
| API、生成契约、领域包 | [API 与服务测试](../../artifacts/plan8-collections-api-tests.log)；客户端/企业门户 typecheck、i18n/style 检查通过基础轮次 | 源码与组件检查；后续改动需复验 |
| Collector 统一版本 | v0.158.0，Linux amd64 原生构建成功；[二进制组件清单](../../artifacts/plan8-collector-components.log) | 实际构建与组件注册，不等于各资源准入 |
| MySQL×2 / Redis / Nginx 配置 | [实际 Collector validate 通过](../../artifacts/plan8-native-config-tests.log) | 原生配置校验，尚未证明真实样本 |
| MySQL×2 / Redis / Nginx 真实采集 | [测试日志](../../artifacts/plan8-native-data-tests.log)、[样本指标与镜像摘要](../../artifacts/plan8-native-samples.json)：MySQL 两个独立实例各 21 种、Redis 26 种、Nginx 4 种指标；关闭一个 MySQL 后其余 3 个继续上报；含 `$` 的凭据保持原值 | 原生组件真实 DATA/独立启停；接收端为有认证的测试 OTLP sink，未验证正式企业链路 |
| 监控租约与版本围栏 | [真实 PostgreSQL 测试](../../artifacts/plan8-runtime-domain-tests.log)：错接收者、重复兑换、关闭后的旧租约拒绝；迟到失败不覆盖新版 | 真实存储与领域流程，执行传输另行验收 |
| Collector 身份私有文件 | [Windows DACL 测试](../../artifacts/plan8-identity-module-tests.log)、[Linux 文件权限测试](../../artifacts/plan8-identity-linux-tests.log)均通过 | 子模块真实操作系统文件权限检查，不算主机正式接入验收 |
| Windows 11 预检 | 增加 ProductType/Build 区分，客户端允许 build≥22000，Server 单独；单元测试通过 | 预检实现，不算本机服务安装/IIS 验收 |

独立数据库：`argus-plan8-postgres-20261018`，标签 `argus.owner=plan8-20261018`，仅回环端口，迁移到 00027。另有独立 ClickHouse `argus-plan8-clickhouse-20261019`，标签 `argus.owner=plan8-20261019`，与仓库锁定版本一致；各测试创建并清理自己的租户表。原生采集测试的目标容器与网络均已按独立 owner 清理。未迁移生产数据库、未发布镜像、未停止既有 argus-system 工作负载。

## 正在接通的运行时

- `monitoringcatalog` 作为服务端与 Collector 编译共用目录，39 项保留、11 项明确暂缓、28 项仍待完成全部支持门禁；20 个原生资源/探测类型已有配置适配器，目录未预填生产支持通过。
- Collector Bundle 升至 v3：配置携带多实例、实例 revision/assignment epoch、独立目标；关闭项不创建 Receiver。K8s 无启用项时不再隐式开启节点采集。
- Collector 自身身份与每个 Resource 的目标身份分离；Writer 使用受信标准化版本逐个 Resource 写入；旧结果不能把新 desired revision 标成生效。
- 监控凭据使用独立短期租约，绑定 Collection/Collector/配置版本/epoch；配置材料只在执行通道交付，运行端单独落权限受限的私有配置，公共 Bundle/PendingAction 不含密码。
- 新应用、采集实例、目录、目标 API 已进入 OpenAPI/生成客户端；完整监控门户、共享选择器及实际执行闭环继续实施。
- 新增共享 ScopeResolver，已接入仪表盘执行/创建目录、资源查询授权与告警求值；可解析应用及原生监控目标、显式空集合、当前应用依赖、目标去重和部分依赖无权状态。关系/目标版本进入范围签名，签名进入仪表盘查询缓存及目录游标；内部目录 RPC 升为 v2。
- [范围与继承专项测试](../../artifacts/plan8-scope-filter-tests.log)通过；[分包串行真实 PostgreSQL 回归](../../artifacts/plan8-scopes-postgres-serial.log)通过。共享库同时运行多个包会触发可重试 SERIALIZABLE 冲突，失败日志保留；未把串行结果冒充跨进程并发业务验收。

新增进展：

- [监控表单 E2E](../../artifacts/plan8-monitoring-ui-tests.log)：zh/en × light/dark 四场景通过，覆盖应用/Collector 弹框选择、无独立环境输入、自动创建目标、确认与逐实例启停；密码不进入 localStorage。属于 mock 门户验证。
- [凭据草稿清理](../../artifacts/plan8-monitoring-credential-cleanup-tests.log)：未到期的已取消草稿、过期草稿自动清理；已绑定监控凭据和平台凭据保留。清理复用 Worker 租约巡检。
- [真实发行版完整配置校验](../../artifacts/plan8-control-only-validation.log)：控制扩展单独运行、Gateway 与原生 Trace 配置通过。零业务采集使用官方 nop Receiver/Exporter，不采集也不制造指标。
- [应用/监控资源仪表盘关联](../../artifacts/plan8-dashboard-resource-bindings-tests.log)：真实 PostgreSQL 验证关联/解绑与权限、版本检查。默认仪表盘模板本身尚未交付。
- Linux 本机与 SSH、Windows 服务已加入候选校验、重启及回滚代码；K8s 加入发布 Lease、双配置槽、校验 Job、Gateway/Agent 就绪检查及回滚围栏。文件回滚/锁与 Kubernetes fake client 测试通过，真实系统和集群生命周期仍待验收。
- 接入层仅在 Kafka 接收成功后更新实例样本状态；观测写入失败不将已接收消息变成重复重试。控制面配置成功不伪造采集健康。

尚需闭合的关键边界：Namespace UID / 运维日志的行级查询限制、原始来源与配置版本在历史查询中的去重、真实平台的配置失败回滚、其余集成准入矩阵、完整 UI/Agent/发布路径。只通过当前组件测试不能完成上述条目。

## 2026-10-19 继续实施

- [19 个 Linux 原生集成配置校验](../../artifacts/plan8-all-native-config-tests.log)通过；SQL Server DSN、Kafka SASL、HTTP Basic 认证使用各 Receiver 的私有字段。Oracle 当前适配器不开放 TLS 参数，避免声称加密却实际忽略。MongoDB 节点/副本集、Kafka Topic/消费组、ES 节点/索引、DNS 与 ICMP 参数进入统一 Schema 和生成表单。
- [Windows Collector 配置校验](../../artifacts/plan8-windows-native-config.log)覆盖 IIS、主机指标与 Application 事件日志；这是本机执行真实 Windows 二进制的 validate，不证明已安装服务、已采到 IIS 或 Windows Server 已验收。SDK 身份组件后续加入，Windows 最终发行版仍需重新构建冻结。
- [四类扩展表单 E2E](../../artifacts/plan8-native-forms-extended-ui.log)通过 Oracle、Kafka、DNS、Apache 的创建与私有凭据；原有中英/明暗四场景也通过。均为 mock 门户。
- [ClickHouse 序列验证](../../artifacts/plan8-metric-identity-clickhouse-retry.log)通过 Pod Resource 维度、采集版本切换、迁移 epoch、重放去重和历史来源过滤，并通过 PromQL 既有一致性与三信号来源隔离测试。首次执行发现 UUID 排序参数类型不一致，修复显式转换后通过；首次失败日志保留。
- 物理序列保留 source/revision，查询按授权来源过滤后合并同一采集实例的逻辑序列；Resource 与 instrumentation scope 属性进入指纹，避免 Pod/服务实例混淆。不同未关联来源仍分开，不以服务同名合并。完整 Kafka 链路及所有指标类型的迁移仍需后续验收。
- [应用 Key 生命周期](../../artifacts/plan8-ingest-key-domain-tests.log)及 [12 路并发服务发现](../../artifacts/plan8-sdk-discovery-tests.log)通过真实 PostgreSQL。Key 仅存摘要，由签名密钥独立域派生；创建人可在 10 分钟内经独立门户接口领取一次。模型工具不返回 Key，PendingAction/审计不保存原值。
- SDK 认证使用原生 Receiver 的认证扩展契约；新增 Argus 应用认证/身份组件只处理 Key 和归属，不替代资源指标插件。共享协议 Receiver 在 batch 前固定 endpoint/source/key 引用，Ingest 重新核验并固定应用、环境、服务和实例。缺有效服务名、环境不符和不允许的服务拒绝；缺实例 ID 保留未知。
- [SDK 完整配置与实际组件清单](../../artifacts/plan8-sdk-native-config-current.log)通过。v0.158 的 `host_metrics`、`file_log`、`kubelet_stats`、`windows_event_log`、`otlp_grpc` 工厂名称与受支持配置别名分开核验；首次清单差异没有被当作通过。
- SDK 接入、Key 元数据及一次领取接口已进入 OpenAPI；应用门户的接入/Key 创建、撤销与领取正在联调。真实协议请求、可用地址/TLS 名称、K8s Service 暴露、拒绝诊断、APM 去重、完整平台与真实模型尚未验收。
- [SDK 准入补充记录](./15-sdk-admission-findings.md)：正常原生协议请求已取得组件证据；SkyWalking 消费失败误报成功及原始数据打印已由真实 Collector 复现。该候选已由用户确认新增暂缓，禁止启用并移出发布门禁，失败状态和证据保留；没有将失败变成通过。

## 发现与修复

Kubernetes 读取与门户继续实施：

- 新增 `kubernetesinventory` 共享读取/投影及 `kubernetesworkbench` 授权范围服务，Direct 与 Connector 使用版本化 v2 对象契约，旧响应缺身份/版本时明确拒绝。
- [真实集群读取和 watch](../../artifacts/plan8-kubernetes-inventory-watch-live.log)：三页查询不漏不重复，Secret 在传输前屏蔽，重建同名 Namespace 拒绝旧 UID，list 后创建的对象能由对应 RV watch 读到；测试 Namespace 已确认删除。
- [真实数据库授权](../../artifacts/plan8-kubernetes-watch-authorization.log)：Namespace-only 不查询整个集群再过滤；显式空范围不访问后端，Node 等集群对象拒绝，游标和 watch Token 绑定主体/授权，监听期间撤权不释放事件。
- [门户四场景](../../artifacts/plan8-kubernetes-watch-ui.log)：中英 × 明暗，Namespace 弹框、资源类型分组、Secret 详情屏蔽和可访问性通过。属于 mock 门户；早期 ARIA 面板身份问题已修复并保留失败日志。
- Pod 日志旧入口和 Agent 工具均改为显式 `kubernetes.operational_logs.read`；普通 `kubernetes.logs` 不自动获得跨应用日志读取。日志与旧资源读取路径也应用统一凭据投影。
- 动态 CRD 类型的键绑定 CRD UID，未注册或被替换的类型不沿用旧引用。类型发现受限时保留可读的内置类型并返回不完整标记；没有把权限不足解释成没有任何资源。
- 当前页 watch 以签名 Token 固定查询、Namespace UID 和来源 RV，最多四个范围并发、每批最多 200 个事件和 1 MiB；410 要求重新列举，取消/切换终止旧监听。新增对象不强行塞入旧分页快照，页面提示刷新。
- 第一批 K8s 写操作进入实现，见 [T11 实施记录](./task-11-kubernetes-operations.md)。已完成原子领取的操作队列、确认/授权复核、UID/RV 冲突检查、实际就绪观察及只读未知结果恢复；私有 marker 和 digest 不进入对象公开投影或操作结果。
- [真实平台领域服务与 Kubernetes API](../../artifacts/plan8-kubernetes-operation-workflow-live.log)贯通 ConfigMap 创建/更新/删除和审计，Namespace 按 UID/owner 清理；[真实 Deployment](../../artifacts/plan8-kubernetes-mutations-live-verified.log)覆盖副本就绪、重启和版本冲突。两者仍不代表已部署 Connector/门户全链路。
- [变更门户四场景](../../artifacts/plan8-kubernetes-mutation-ui.log)通过中英、明暗和可访问性，mock 表单沿用共享组件/确认流程。真实 API/权限与 UI mock 分别记录。
- [回归修复](../../artifacts/plan8-kubernetes-contract-repair.log)处理权限版本断言和 proto 生成文件 2000 行约束；未放宽检查。
- Secret 私有输入已贯通专用表单、现有 Keyring 加密暂存和执行时解密；[真实 Opaque Secret](../../artifacts/plan8-kubernetes-secret-workflow-live-verified.log)、[授权与幂等](../../artifacts/plan8-kubernetes-private-input-authorization.log)、[门户中英明暗](../../artifacts/plan8-kubernetes-secret-ui.log)分别通过。公开差异遵守 PendingAction 数组契约，超限拒绝而不截断审批内容；不把 mock 数据作为真实密钥写入证明。
- Helm、完整依赖与权限检查、私有输入全部路由/Secret 类型、会话、Namespace 遥测行级范围、组件监控和日志规则仍未完成，本任务尚不能计整体通过。

- 统一错误码契约最初缺少新监控错误码，已登记并通过 contract/HTTP 回归。
- Go 模块镜像的 checksum 下载连接中断，切换单次命令的官方 GOPROXY 后完成构建；没有关闭 checksum 验证。
- Redocly 在 Windows 出现一次 libuv 退出异常，重新运行整套生成成功；未手工替代生成文件。
- `argusidentity` 子模块在 Windows 的两条旧测试使用 POSIX mode 位断言而失败；已补上实际文件 DACL 限制及独立操作系统校验，Windows 与 Linux 回归均通过。Gateway 子模块通过。
- 原生采集测试的 OTLP sink 首次缺少 gzip 解码器，Collector 明确拒绝导出；修复测试接收器后真实采集通过，初次失败记录保留在 `artifacts/plan8-native-data-tests-initial.log`。

剩余工作仍包括真实资源样本、全链路运行状态、配置失败回滚/迁移、统一 ScopeResolver、SDK/日志/APM、完整 K8s 工作台、其余原生适配器、仪表盘、Agent 与真实模型、Windows 11 管理员前件和最终统一发布。

## 授权目录与门户分页

六种授权根已贯通领域目录、OpenAPI、HTTP 校验、生成类型和正式授权编辑器；目录采用数据库 keyset 分页/搜索，应用环境与 Namespace Cluster/UID 可见，跨页选择作为差集保存。请求者和目标主体版本进入游标绑定；返回前复验授权，版本变化阻止旧草稿覆盖。旧仪表盘授权流程保持通过。详见 [T03 实施记录](./task-03-scope-and-authorization.md)。

共享选择器与受保护多行输入的 CSS 已归并到统一样式入口，修复 Node/Playwright 导入业务模型时直接加载 CSS 的失败。真实 PostgreSQL、浏览器 mock、完整 Go 回归分别记录；这仍不等于完整平台和全部业务范围已通过验收。

## 资源类型贯通查询

- Server/Worker 到 Query 的 RPC 现在传递真实资源类型、源版本和范围签名；SQL 来源匹配使用类型＋UUID。共享的 `resourceref` 契约避免把应用、数据库目标或 Cluster 自动当 Host。
- [真实 PostgreSQL 范围验证](../../artifacts/plan8-typed-sources-postgres-verified.log)覆盖应用依赖、显式空集合、同 UUID 跨类型来源隔离及告警冻结范围；[ClickHouse](../../artifacts/plan8-typed-sources-clickhouse.log)验证概览来源过滤和序列版本；[RPC](../../artifacts/plan8-typed-rpc-contract.log)验证资源类型/主体/来源/关系签名、缓存隔离和旧协议拒绝。
- 下钻和查询文件保存资源/关联版本；真实查询、目录、Native HTTP、工具与告警采用同样的类型化来源解析。返回前按冻结类型/签名复查，撤权不能把相同 UUID 的其他类型替代为原资源。
- 联合回归发现并修复：空资源集合叠加应用选择的误拒绝；告警状态机测试需携带真实冻结范围；旧缓存测试混用了 all_authorized 与 explicit；权限错误映射需保留领域错误类型。没有删除相关断言，增加了两种范围不共用缓存的验证。
- 测试 ClickHouse 的 768 MiB 容器上限触发内存限制，确认本轮所有权和机器余量后仅调高该测试容器到 1536 MiB，并将数据库/查询包串行验收。正常部署和其他业务容器未调整。首次失败记录保留。
- [完整仪表盘 PostgreSQL/ClickHouse 集成](../../artifacts/plan8-dashboard-scopes-full-integration.log)和[完整告警 PostgreSQL 集成](../../artifacts/plan8-alerting-scope-full-postgres.log)通过；监控领域与三信号 ClickHouse 验证见[分包联合日志](../../artifacts/plan8-scoped-runtime-integration-verified.log)，该联合日志仍保留修复前仪表盘/告警失败，不能整体标通过。

## 日志规则与服务声明

文件日志多规则已进入领域服务、配置编译、来源注册、OpenAPI 和正式门户。服务声明使用相同幂等身份，静态声明不冒充已观测数据。来源版本保存服务及 Namespace UID 字段；入库前按受信来源还原业务归属，已识别敏感内容在 Kafka 前脱敏。

真实文件读取验证发现 `262144B` 不是原生 Receiver 接受的大小格式，改成该版本支持的字节数文本后通过轮转、重启续读和独立启停。初次失败记录保留。SQL 规则验证使用真实数据库；门户四场景使用 mock，不能合并声明正式平台验收完成。详见 [T08](./task-08-log-collection.md)。

旧固定日志 Profile 的覆盖范围参与冲突判断，避免新规则和旧开关重复采集；这只是迁移期间的防重保护，旧模式仍需整体迁移。Journald 单元名规范化 `.service` 后再检查覆盖。K8s 日志、运维日志权限和完整查询闭环继续实施。

K8s 独立日志规则已取得真实 Pod/原生 Collector 的滚动发布、元数据 API 分区恢复、接收端故障与强制退出后续传证据，见 [T08 更新](./task-08-log-collection.md)。固定节点指标不再隐式读取所有 Pod 日志。Namespace-only 的规则管理、同名 Namespace 重建、孤立规则停用及门户 12 个场景分别通过；运行时字段和完整平台部署仍需继续验收。

## Namespace 持久日志查询

统一来源解析开始贯通 Namespace 选择、原生查询、仪表盘、候选值及角色到期检查，见[边界与证据](./16-namespace-log-query.md)。选择范围保持 Namespace 身份，查询数据保持应用/集群身份，不把权限转换成应用详情或 Trace 访问。候选值的纯选项级联逻辑与生产授权发现回调分开，取数前后都执行来源复核。

新范围复验会直接拒绝未授权的 Host 来源，因此同 UUID 跨类型测试同步加强为领域拒绝加直接 SQL 夹具存在性校验，原有“不泄露其他类型来源”的断言保留。日志解析错误路径发现 Stanza 默认策略会打印原始 entry，改为锁定 quiet 策略与 info 诊断后，真实二进制验证错误记录仍可上报且诊断无原文。

## 正式仪表盘筛选

查看工具栏加入分页应用选择、多类型资源选择与资源类型筛选。新建 Scope Directory 只返回当前授权元数据，游标绑定主体、版本、类型和搜索；不依赖主机或 K8s 菜单权限，也不提前加载全部对象。应用和资源的显式空集合贯通请求与候选缓存，移除旧的“数组为空即全部”推断。相关测试见 [T14](./task-14-dashboard-templates.md)。

最新四包 PostgreSQL/ClickHouse 集成与 15 个门户回归通过；筛选改造后 11 个门户场景及 65 应用分页验证通过。API/工具权限测试最初缺少已登记来源，已增加受控来源夹具；回归同时修正撤权与普通范围变更的错误类型区分。未放宽来源检查，没有把受控后端当真实采集验收。

## 默认模板与应用来源准入

实现不可变模板版本、受管默认入口、自动升级和独立个人副本。MySQL×2、Redis、Nginx 的原生采样进入真实 Writer/ClickHouse，22 组目标面板再经正式 Dashboard 运行时验证；门户四种语言/主题组合覆盖目标快捷入口和个人副本。图表高度按网格实际行高调整，避免只显示标题。详见[验证层级和证据](./17-managed-defaults-and-sdk-admission.md)。

严格来源复验发现应用 Endpoint 来源无法通过旧数据库约束，新增迁移 37 修复三类互斥来源。迁移 36 退役免 Key SDK Profile；新数据缺少已登记来源或试图把 Host Trace 当应用数据时拒绝。OTLP/Jaeger 的实际 PostgreSQL 复验通过。SkyWalking 继续暂缓，既不能创建应用入口，也不能借旧 Profile 启用。完整平台和统一发布继续保持未完成。

## K8s 显式权限、清单编辑与生效对象

移除 Cluster 范围直接跳过提权检查的旧分支，新增独立主机特权、ServiceAccount 使用和 RBAC 委派权限，迁移及权限注册表为 38/16。写协议升级到 v2，提交前复验限时权限与批准的生效配置；真实 API 验证错误摘要不写入、普通新建的无 Token 默认值、显式身份路径和既有 ConfigMap/Secret/Deployment 工作流，见[边界与证据](./18-kubernetes-mutation-privileges.md)。私有输入绑定在原生预检之前完成，敏感对象不追加内容摘要，避免破坏精确输入绑定。

门户使用统一 YAML/JSON 编辑器，实际对象投影与编辑基线分离，后台刷新不覆盖未保存内容或提升其 resourceVersion。格式切换改为共享分段单选，修复初次可访问性失败；解析、版本变化和四种语言/主题场景见[清单编辑](./19-kubernetes-yaml-editor.md)。Helm、drain、PVC、会话、全类型真实结果、正式 Connector/门户/Agent 路径仍未完成。

## 原生日志发送与 K8s 自身样本

实际 Journald info/debug、游标及持久发送队列在接收端故障和 KILL 后恢复。发行默认启用上游 Stanza 同步发射开关，并将默认值文件加入制品指纹；未 fork Receiver，Windows/ARM 与全量回归仍需适用验证。详见[Journald 证据](./24-journald-and-native-log-durability.md)。

真实 K8s API、Kubelet 保持 TLS/ServiceAccount 鉴权，集群/节点固定实例分别启停、全关闭保留运行，新增资源清理通过。实际原生数据经 Writer/ClickHouse 跑通 12 项默认查询；门户加入 Cluster 的默认面板快捷入口。模板生成物按定义拆分，保持单文件限制。见[K8s 样本与未完成边界](./25-kubernetes-own-metric-samples.md)。包含 Journald、Host/K8s 面板和 YAML 的 12 个 mock 桌面场景通过。正式平台、剩余组件和统一发布仍未完成。

新增 API Server/CoreDNS 实际采集、权限故障/恢复和 24 个默认查询；抓取失败诊断不能制造新鲜目标样本。同步发射的新二进制通过文件日志、K8s 日志和 mTLS/Kafka/Writer/ClickHouse 恢复回归，三平台已构建，Windows 本机仅验证二进制开关，未当作正式接入或 IIS 验收。

新增实际 Job、Service 和 PVC 的批准摘要、自动分配、绑定、就绪及否定路径，并贯通 PostgreSQL PendingAction/Worker/审计；见[存储与工作流](./26-kubernetes-storage-jobs-and-allocations.md)。CSI 成功扩容、Helm、drain、回滚、会话等仍未完成。

应用新增七视图、稳定服务过滤和 Trace/日志下钻；同名不同 Namespace 的 APM、日志和拓扑以真实 Writer/ClickHouse 验证分离。HTTP/原生工具允许应用独立读取，撤权仍拒绝。三信号及候选目录的时间边界改为明确精度转换，纳秒/毫秒半开窗口和回归通过；见[应用入口与时间证据](./27-application-observability-and-time-boundaries.md)。跨来源去重、部署后 SDK/APM 和真实模型仍未通过。

2026-10-19 用户暂缓 Windows 实机测试，未执行管理员 Prepare；保留 Windows 实现和构建，正式接入、服务、指标、事件日志及 IIS 验收未验证。其余 Plan8 继续，见 [Windows 状态](./28-windows-testing-deferral.md)。

三类工作负载补充明确历史修订的模板回滚，原生 API/Pod、PostgreSQL 审批与 Worker、四种门户语言主题均通过；写协议为 v3，旧版本明确拒绝。共享长差异/代码区域增加键盘滚动入口。见 [回滚证据](./29-kubernetes-workload-rollback.md)。全量 Plan8 与统一发布仍未完成。

真实 Go 三信号 SDK、HTTP 调用链、原生 Collector、Key 鉴权、PostgreSQL 发现、mTLS Ingest、SCRAM Kafka、Writer/ClickHouse 及会话 HTTP/原生工具已联合通过。修复 Trace/指标队列前脱敏遗漏、普通指标后缀误判和工具 GraphQL 参数遗漏；见 [范围与证据](./30-native-sdk-pipeline-and-signal-privacy.md)。正式门户正在读取同批真实数据验收，尚未发布。

同批真实三信号 SDK 数据已进入 real 门户，四种语言/主题组合各核对实际图表响应、Trace 下钻、日志和服务筛选，并通过三种桌面宽度的布局与可访问性。注册/颁证及会话初始化仍是明确夹具，不替代正式主机注册或部署后验收。证据见 [SDK 记录](./30-native-sdk-pipeline-and-signal-privacy.md)。

CronJob 手动 Job、源版本围栏、现有私有模板的执行端复制、Job 实际完成和失败 Execution 收口通过。门户与回滚共 8 个中英明暗场景通过；写协议更新到 v4，旧版本在执行前拒绝，见 [CronJob 记录](./31-cronjob-manual-run.md)。

## 2026-10-19 完成度回顾

按当前工作区源码与已有日志完成 [16 项任务核对](./32-implementation-audit.md)。结论为尚未全部实现：Helm/drain/受控会话、部分自身指标、其余资源支持和默认模板、新领域 Agent 工具、Namespace 指标权威范围及计划级验收/发布自动化均有剩余工作。已有真实 SDK/门户、Journald、K8s 回滚和 CronJob 证据已计入当前范围，不再沿用早期“尚未实现”的表述。

修正 T01/T02 的过时实施记录，并将已有代码的候选集成和内置能力记为 in_progress；selection、正式基础/五类门禁与发布状态没有提升为通过。Windows 实机继续按用户决定暂缓。本次仅审查源码/证据及维护文档，没有重新执行全量测试、安装或发布。

## 完成度审计之后的持续实现

已补 20 个新领域原生工具、带类型结果/Workspace 撤权、主机进程与 Linux/Windows 服务、三个控制面配置与不可达诊断，以及 PostgreSQL/探测、Memcached/Apache/HAProxy、RabbitMQ/ZooKeeper 的具名真实组合与默认面板。当前固定指标 12 个、模板实现 24 套；新增 Linux 与服务面板有实际 Writer/ClickHouse 查询，Windows/三个控制面成功取数仍未验证。

新增 e2e-plan8 驱动，具名执行、源码/Collector 指纹、后端 owner 和空/Skip 拒绝进入同一结果。full 仍明确 incomplete；K8s 未交付功能、其余支持组合、正式接入/模型/告警、迁移恢复和统一发布继续未完成。完整事实、证据和剩余项见[后续状态](./41-implementation-followup-status.md)，各专项失败轮次保留。

## 第二轮后续交付

已补 [drain/PDB/取消](./43-node-drain.md)和[受保护控制面指标认证、证书信任及默认面板](./44-control-metrics-authentication.md)。两个分套件的具名冻结执行分别通过三组检查；真实双节点、实际数据库、原生 Collector、ServiceAccount 拒绝/恢复、Writer/ClickHouse 和十八个控制面查询均有证据。门户分别覆盖八个 drain 双人审批/取消场景和四个信任配置场景。

保留已有安装清单授权、运行时不创建 ClusterRole 的边界；修改的是显式接入清单，真实 Token 审查未获业务日志、终端、Secret 或写权限。本轮临时集群已恢复原监听并按 Docker ID/Namespace 所有权核验后删除，原业务集群 API 健康。Windows 未测试，整体未统一发布。当前功能、剩余项和检查口径以[45 第二轮实施](./45-followup-delivery.md)及 progress.json 为准，不把此前的旧缺口描述当作当前状态。


## 2026-10-19：Pod 日志会话与共享接入身份

补上 Namespace/Pod/容器代次绑定、一次性票据、当前/上次增量日志、暂停授权检查、并发容量/清退及门户；固定 API 地址并统一内联身份校验。五组冻结记录、实际 Pod/HTTP/数据库和最终门户及工程检查见 [47](./47-pod-log-sessions.md)。本轮临时 Namespace 已按 UID/owner 清理，正式基础门禁、支持矩阵和统一发布状态没有改为通过。
