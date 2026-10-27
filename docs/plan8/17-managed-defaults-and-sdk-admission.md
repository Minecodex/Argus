# 受管默认仪表盘与 SDK 来源准入

状态：实现与分层验收进行中；尚未统一部署发布。本页记录 2026-10-19 的新增证据，不替代 T07/T14/T16 的完整验收。

## 默认模板

模板定义、不可变版本、当前版本指针和企业默认入口分别存储。默认入口由系统管理，读取时自动更新到兼容的最新版本；发布过的旧 Revision 保留，已开始的查询继续使用冻结定义。编辑默认入口创建独立个人草稿，保存模板来源和版本；修改副本不会覆盖默认入口。用户不能直接发布、归档或通过普通数据库写入修改系统默认项。

系统固定定义的读取只要求 Dashboard 读取功能，普通 Dashboard 继续检查对象授权。该例外不授予主机、集群、应用或监控目标的数据权限；查询仍经统一 ScopeResolver。

首批定义使用专用 Receiver 的实际指标名称：

| 模板 | 面板 | Grafana 参考 | 当前验证 |
| --- | --- | --- | --- |
| MySQL | 运行时长、线程、缓冲池用量/上限、操作速率、行锁 | 7362 | 两个真实实例，12 组目标面板 |
| Redis | 运行时长、连接数、内存/RSS、命令速率、阻塞连接 | 763 | 真实实例，6 组面板 |
| Nginx | 当前连接、请求速率、接收/处理连接速率 | 12708 | 真实实例，4 组面板 |

Go 定义同时生成门户注册表和 `api/contracts/dashboard-templates.json`，避免验收查询与产品公式各维护一份。模板安装不生成样本，也不把安装成功记成真实数据通过。监控列表能直接打开该目标的默认仪表盘；门户的应用和资源弹框继续使用分页、搜索及显式空集合语义。

证据：

- [真实 Receiver 采样与独立启停](../../artifacts/plan8-native-template-capture-final.log)：同 Collector 的两个 MySQL、Redis、Nginx，无独立 exporter。
- [真实 Writer / ClickHouse 表达式](../../artifacts/plan8-native-template-view-load-clean.log)：22 组目标面板返回有限数值，错误来源不返回数据。
- [正式 Dashboard 运行时](../../artifacts/plan8-native-template-runtime-clean.log)：真实 PostgreSQL 授权和来源目录、ClickHouse 查询；无权目标拒绝，事后应用关系能查历史，共享目标不重复计数，显式空集合不扩大。
- [版本/只读边界](../../artifacts/plan8-managed-templates-audit-postgres.log)、[个人副本](../../artifacts/plan8-managed-dashboard-copies-postgres-verified.log)：不可变版本、旧服务不降级、新版自动可见、用户副本不覆盖。
- [门户中英明暗四场景](../../artifacts/plan8-managed-dashboard-shortcut-e2e-final.log)：新建、目标快捷入口、系统默认、个人副本和可见面板高度；使用明确标识的 mock API。
- [原生二进制配置验证](../../artifacts/plan8-application-defaults-native-config-passed.log)：应用 OTLP/Jaeger、Host、Gateway、K8s Agent/Gateway 和组件清单。配置验证使用只读 CA 及显式无效测试 Token 满足构建前件，没有据此声称认证或实际 Kubelet 取数通过。

真实数据验证目前是认证测试 Sink → 原生 OTLP → Writer → ClickHouse → 正式查询运行时。控制面记录为受控数据库夹具，尚未覆盖部署后的门户、PKI、Kafka、Agent 与真实模型业务闭环。其余资源和主机/K8s/APM 默认内容仍需逐项补齐。

## SDK 来源准入

每个新进入的 Resource 块必须携带已登记来源及配置版本；缺失来源直接拒绝，不再接收为 unknown。旧 Kafka 记录中的 unknown 历史标记仍原样保留，不给其补造应用归属。

来源分成三种互斥类型，由迁移 37 约束：基础设施来源、带执行分配的采集实例来源、应用接入点来源。实际数据库复验发现旧约束漏掉第三种，使应用接入点无法登记来源；已修复，并显式要求非空采集版本/执行代次，避免 SQL CHECK 的 NULL 语义放过不完整记录。

所有 Trace 必须使用受信应用接入点。OTLP/Jaeger 的旧免 Key Profile 退役，新建业务接入使用应用 Endpoint/Key；主机页移除免 Key 接收开关并提供应用入口。Key 由 Ingest 再次检查，Collector 缓存不能延长撤销期限。应用环境来自应用，服务由上报 namespace/name 幂等发现。

SkyWalking 保持用户确认的新增暂缓项：应用接入、旧 Profile 编译、目录和 Ingest 均不能启用；暂缓总数仍是 11 类资源加 1 个协议，OTLP/Jaeger 的认证接入继续实施。

- [真实 PostgreSQL 来源准入](../../artifacts/plan8-sdk-source-admission-postgres-verified.log)：OTLP/Jaeger 来源登记、服务复用、环境继承、错误环境拒绝、Key 撤销。
- [严格来源单层回归](../../artifacts/plan8-strict-source-admission-unit-final.log)：旧入口不能用伪造应用属性获得 Trace 归属。
- [四领域真实集成回归](../../artifacts/plan8-managed-source-full-integration.log)：监控、Dashboard、Telemetry、告警；不等于全部真实产品场景通过。

初次测试的企业 code、目标 kind 和重复采样 ID 夹具错误记录保留；修正夹具后重新采集并通过正式运行时验证。应用来源约束失败是真实实现缺陷，修复记录与首次失败保留在 `artifacts/plan8-sdk-source-admission-postgres.log`。没有通过放宽来源校验来让测试通过。
