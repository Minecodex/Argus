# 凭据、接口、工具与共享交互

## 监控认证信息的自动托管

用户在监控表单中直接填写目标需要的用户名/密码或 Token；可选认证方式由集成版本声明，不能给所有数据库统一展示不支持的 HTTP Bearer 模式。使用本机运行身份、无需认证和 TLS 材料分别表达。

服务端复用 `internal/secret` 的信封加密、版本、租约、接收者绑定、失效与审计，增加 `purpose=monitoring` 和明确 owner。平台“凭据与密钥”默认查询只返回平台用途；监控域按采集对象管理。用途是权限和索引条件，不只是前端隐藏字段。

### 提交与取消

1. 表单通过专用敏感输入通道提交；请求日志、Trace、错误、PendingAction、Diff、ToolResult、Workspace 和通用客户端状态不得保留原文。
2. 连接检测需要凭据时建立短期加密暂存/租约，绑定企业、主体、草稿版本、目标、Collector 和用途；用户不需要额外访问凭据页面或多点一次保存。
3. 对外预览仅返回认证方式、已配置/将替换状态。私有暂存引用与提交能力留在服务端，普通模型工具不能读取或代填明文。
4. Confirm 复验版本与权限，在事务内创建/绑定正式凭据、实例与 Outbox。检测成功不等于配置已提交或已开始采集。
5. 取消、超时、提交失败和重复提交有明确清理/幂等规则，不能残留可被其他操作复用的草稿 Secret。

编辑操作用明确 `credential_update=keep|replace|remove` 契约，门户默认留空映射 keep；replace 必须有完整认证输入，remove 与无需认证配置一起校验。禁止把空字符串、占位星号或字段遗漏解释成意外清空/覆盖。

Collector 的短期交付绑定指定接收实例与配置版本；明文只在受控进程/本机权限边界内存在。公开配置、下载制品、命令行参数和审计不能包含密码。目标不可达、证书错误和认证失败分开；不自动关闭 TLS 校验来让测试通过。

租约过期/撤销阻止后续兑换，不等于自动吊销数据库侧账号。已交付给 Collector 的材料需要通过配置代次、停止旧任务、清理受控本地材料与生效回执完成撤换；如需废止目标系统的旧账号，属于独立的已授权目标变更。验收分别验证平台代用权限失效、Collector 实际停止使用旧配置和目标侧认证结果，不作超出信任边界的撤销承诺。

删除采集释放其私有凭据及未用租约；有明确共享引用时先引用计数/所有者校验，不删除他人凭据。正式后台列表与 by-ID 路径同样执行用途和对象授权。

## SDK 写入 Key

与访问数据库的监控凭据是独立用途。SDK Key 保存摘要/验证材料，原值只通过有权用户的受控创建/轮换交付显示；不进入 AI/日志/审计或普通列表。丢失后轮换，不提供无限制明文取回。

Key 支持独立禁用、轮换、有限交叠有效期、速率/字节配额与最后使用观测。授权、服务限制或接入绑定变化、删除接入和撤销要使接收端缓存按版本失效；应用环境不可原地更换；证明撤销在约定窗口内生效。Key 认证不能授予监控平台其他 API 权限。

## API 资源与一致性

下列为待在 T01 固化的语义，不把示例路径视为现有 API：

| 资源族 | 必需接口 |
| --- | --- |
| applications / services | 单环境应用搜索分页、创建/复制/归档、服务受控发现与详情、实例观测和依赖关系 |
| monitoring/targets | 按语义类型分页、详情、关系管理、从已有目标新增采集 |
| monitoring/integrations | 版本化能力目录、配置 Schema、Collector 匹配、前件/模板/支持状态 |
| monitoring/collections | 列表/详情、草稿、敏感认证暂存、测试、预览、确认、状态、版本、独立启停/迁移/删除 |
| application-ingest | 接入配置、创建/轮换/撤销 Key、示例 SDK 配置、接收/拒绝诊断 |
| telemetry scope / catalog | 类型化条件解析、候选分页、有效目标/来源与适用性、三信号执行 |
| Kubernetes objects | GVR/UID/version 受控列表、详情、事件、watch、日志流、具体动作预览/状态 |
| Kubernetes sessions | 终端/流式日志的创建、票据、心跳、关闭和撤权；不复用 Collector 身份 |
| Kubernetes authorization / operations | Namespace 显式授权、运维日志能力、Jobs/CronJobs/HPA/NetworkPolicy/RBAC、Node 维护、CRD/自定义资源、端口转发及 Helm 生命周期 |
| dashboard templates | 列表、兼容性/缺失能力、实例绑定、模板版本、草稿/发布与用户副本 |

OpenAPI/JSON Schema/protobuf/sqlc/权限注册表与生成客户端一起更新。错误沿用统一契约并补充 target conflict、unsupported integration、capability unavailable、scope incompatible、credential incomplete、revision conflict 等可定位错误。

变更统一 expected_version、幂等键、固定预览与执行状态；Confirm 不接受可变业务参数。长操作返回操作引用，Worker/Connector 回报真实结果；超时 unknown 不伪装失败后重放。

## 应用/服务选择器

- 共享 `@argus/ui` 列表选择弹框，支持单选/多选、搜索、分页、已选计数、跨页保留、取消/Escape、键盘焦点与双语主题。
- 应用列：名称、负责人、唯一环境、可见服务数；同一业务的生产/测试是两条应用。服务列显示继承环境、技术栈/实例摘要，同名服务可区分。
- 候选在服务端按企业、主体、授权版本和关联上下文裁剪。游标绑定查询/排序/上下文，不把完整目录塞入浏览器。
- Confirm 提交稳定 ID 集合并重验权限；分页期间撤权、删除或版本变化不得提交已失效选项。跨页选中不是跨权限选择。
- 监控创建不提供独立环境值、对象新建/复用占位框或全局凭据下拉框。SDK 接入只必选应用，收到有效数据后登记服务；文件/工作负载业务日志规则保留服务映射。数据入库必须有可信归属，不等于创建表单必须预选服务。
- 应用变化只清除不再合法的服务选择并明确提示；取消暂选保留父表单与敏感草稿。全局 Dashboard 空目标集合仍是空，不回退 All。

## Agent 与其他入口

本轮新增 `monitoring.collection.form` 可信宿主表单和 `kubernetes.drain.cancel.preview`，领域工具当前合计 25 个。表单请求只传公共上下文，用户在当前会话的宿主 UI 中填写认证，通过现有 API 创建预览再确认；模板和模型不接收认证。实现及验证边界见[本轮补齐](./57-completion-delivery.md)。

复用 `tool.search(category) → tool.describe(category,name) → tool.invoke(...)`，Search 仍只传 category；不要为数量增长恢复关键词搜索/分页。新增 application/monitoring 分类或现有分类调整必须修改共享双语分类说明、Registry 和模型上下文，版本化发布。

当前已加入 application/monitoring 分类及 20 个原生工具，共享说明、发现签名、公开 Schema、Server/Worker 与模板目录同步更新。具体实现/验证及尚未完成范围见[原生领域工具](./33-native-domain-tools.md)，不将此实现记录解释为已发布。

模型可查询目录/状态、选择已授权应用与目标、提出配置草稿与变更预览、读取诊断和已发布仪表盘数据；敏感输入由宿主安全表单完成。模型看不到密码、SDK Key 原值、私有 Secret 暂存引用和 Commit 能力。

人工门户、API、原生工具、告警后台和工作流调用相同领域服务与范围解析。K8s 类型化业务变更沿用 PendingAction；自由远程命令沿用 Plan7 当前 connect 授权规则，不能因用户打开了应用/集群页面而取得后续任意执行权限。

数据返回继续遵循“工具取数 → Workspace 文件 → read/grep/bash 分析”，携带固定应用/目标/来源/关系/授权版本。创建配置成功、Collector 应用成功、收到样本、完成分析和问题恢复分别记录；Agent 不根据开关或空结果声称监控健康。

执行任务：[T03](./task-03-scope-and-authorization.md)、[T04](./task-04-managed-credentials.md)、[T15](./task-15-alert-agent-and-portal.md)。
