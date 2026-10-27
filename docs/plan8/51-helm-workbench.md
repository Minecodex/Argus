# Helm 工作台实施

2026-10-20，接续 [50 完成度复核](./50-completion-audit.md)，T11 实施中。

沿用现有 Kubernetes 接入身份、PendingAction、操作队列与审计。使用仓库锁定的 Helm v4 SDK 管理 Release 和 hooks；不创建外部 Helm 服务，不把平台安装 CLI 当作工作台。

## 边界

- 上传打包 Chart 并填写受保护 values；Chart/values 临时信封加密，公开接口及模型仅接收受控引用和已脱敏结果。限制压缩/展开大小、对象数与结果大小，禁止路径逃逸、链接和缺失依赖的隐式下载。
- 预览离线渲染，不允许模板读取平台本地环境、文件或跨范围 Kubernetes 对象。冻结一次渲染的输出，执行不重新计算随机模板值或使用实时 lookup 扩大范围。
- 每个普通对象、hook、删除对象和 CRD 都检查其类型权限、Cluster/Namespace UID 范围和提权策略。`helm.manage` 不替代工作负载、网络、配置、RBAC、CRD 等权限。
- 实际 API dry-run、UID/resourceVersion 和差异进入不可变预览。Helm 自身的存储 Secret 单独限定名称和用途，普通 Chart 不得冒用该前缀；不回传 Release values、Secret 原文或 hooks 日志。
- 执行再次核验审批绑定、集群配置/路由、当前权限和对象版本；每次写入都受限时许可控制，撤权后停止后续写入。部分成功/失败和未知结果如实记录，不把 Helm 的多对象操作描述成原子事务，也不自动重放未知写入。
- Release 历史、安装/升级/回滚/卸载与对象就绪分别观测。卸载保留 Helm 历史用于诊断和结果恢复，受保护/保留对象明确展示。CRD 沿用 Helm 的保留语义，定义与实例分别授权；不能把新 CRD 尚未注册时的实例检查宣称为已完成服务端 dry-run。
- Direct 与 Connector 共享原生引擎与契约；门户、API、工具调用同一领域服务。默认安装身份保持只读，实际写入仍受接入身份的 Kubernetes RBAC 限制。

Helm 的 [hooks](https://helm.sh/docs/topics/charts_hooks/) 和 [CRD 行为](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/) 是实现依据；不改变现有共享授权与敏感输入边界。

## 本轮实现

| 层 | 交付 |
| --- | --- |
| 存储 | Migration 42：加密临时输入、应用/操作/Namespace UID 绑定、Release 执行围栏；补齐 Connector 固定日志命令类型约束 |
| 原生引擎 | Helm v4.1.0 SDK 安装、升级、回滚、卸载、历史；冻结随机值、普通对象和 hooks；原 Chart/values 仍保存到标准 Helm Secret 历史 |
| 预览/执行 | 一次渲染、逐对象授权与 dry-run、UID/resourceVersion、字段路径差异、审批、队列、逐次写入许可、观测与审计；SDK 字段管理迁移只能修改已批准对象的管理元数据 |
| 资源身份 | 沿用已有 `custom/<group>/<version>/<plural>@<CRD UID>` 类型契约；新 CRD 和 Namespace 分阶段校验，自定义类型重建不继承旧身份 |
| Direct/Connector | 共享引擎；新 `argus.kubernetes_helm/v1` 能力协商；敏感材料只在分发时注入，持久命令不包含 Chart/values；每次写入绑定 nonce、command、operation、epoch 及短时许可 |
| API | 显式 Namespace 的发布列表、版本历史和受保护新建预览；沿用确认和操作记录接口；登录、CSRF、幂等、`no-store` 与安全错误 |
| 门户 | Namespace 列表弹框、发布列表、历史版本、上传 Chart/受保护 values、四种操作、预览和审批；中英、明暗、桌面及共享 UI |
| Agent | 三个新增工具：`kubernetes.helm.list`、`kubernetes.helm.history`、`kubernetes.helm.change.preview`；模型只可预览历史回滚或卸载，安装/升级的敏感输入经可信门户提交 |

同一 Release 的并发执行由数据库围栏排他控制。未知操作不重放；显式恢复只有在六分钟的旧执行窗口过去后才可接管，旧执行者随围栏失效。权限丢失后无法确认实际结果时保留 `result_unknown`，不推测工作负载已经失败或成功。

## 渲染与敏感数据边界

原生 Helm 的默认 Schema loader 可访问 file/http，值合并也可能打印原值，因此不在服务主进程中直接渲染。Server、Worker 与 Connector 使用同一程序的内部子进程入口；不增加独立服务或用户安装项，也不启动外部 Helm CLI、插件或 exporter。

子进程仅收到 Chart、values、渲染所需的公开能力与 Release 信息；不继承平台环境、Kubeconfig、解密 Key 或 API 凭据。单进程一次渲染，限制约 128 MiB、5 秒 CPU、10 秒墙钟及有界输入输出；Windows 使用 Job Object，Linux 使用 rlimit。macOS 的 RLIMIT_DATA 包含共享可写映射，不能直接用作相同的内存占用预算，因此由系统 `/usr/bin/taskpolicy -m 128` 在 exec 前安装 active/inactive 致命内存上限，CPU 与文件大小仍由 rlimit 限制；工具或限制不可用时拒绝渲染，不回退到无约束进程。三平台 CI 在完整 CLI 编译前执行普通渲染、外部 Schema 拒绝、超量模板与取消回归；macOS 另验证触碰 256 MiB 内存的子进程被 SIGKILL，且不是父进程墙钟超时导致。SDK 日志在子进程中丢弃；Schema 保留校验但使用关闭外部加载的 compiler，支持 Schema 文档内的定义与标准 draft。模板的集群 lookup/DNS 不可用。

macOS 机制依据：[Apple taskpolicy 源码](https://github.com/apple-oss-distributions/system_cmds/blob/main/taskpolicy/taskpolicy.c)，使用 `POSIX_SPAWN_SETEXEC` 及 active/inactive fatal memory flags；执行路径和参数单独传递，不经 shell 拼接。

Chart 压缩上限 384 KiB、累计展开 8 MiB（包括递归打包依赖）、values 和 Schema 各 96 KiB、100 个对象。配置/密封计划及整个 Connector 消息还受组合大小限制，不能同时占满所有单项上限。远程 URL、缺失依赖自动下载、外部 Schema 引用、可执行插件及后置渲染器不在入口范围。Kubernetes 返回体及 Helm 历史解压也有独立限额；超限明确失败。

预览只包含对象标识、动作、保留/延后校验标记及变化字段路径；原值保留在加密计划中。执行结果另记录有界的写入阶段/状态，不返回对象原文。敏感字段不进入公开操作、审计、工具卡片或模型输入。

## 接入身份与实际权限

平台 `kubernetes.helm.manage`、具体资源权限和显式 Cluster/Namespace 授权同时生效。实际 Kubernetes API 身份也必须有对应权限；默认接入身份继续只读。标准 Helm 历史需要 Namespace 内 Secret 的读取权限，写操作还需要图表对象及 hooks 的对应权限。

[可选 Namespace Role 示例](../../deploy/examples/kubernetes-helm-rbac.yaml)仅覆盖简单配置、Deployment、Service 与 Job 图表，不会自动应用。CRD、Namespace、RBAC、ServiceAccount/提权等按实际图表另行明确授权；不能用该示例或 `helm.manage` 代替这些前件。

## 验证与当前边界

- [本轮冻结分组证据](../../artifacts/plan8-helm-delivery-manifest.json)：`helm` 七组具名检查；完整 harness 为 41 组，仍拒绝用 skip 或组件成功替代正式验收。
- [工程验证汇总](../../artifacts/plan8-helm-final-checks.json)：Go、前端单元/类型/lint/双语、契约和门户回归分别记录返回值与原始日志。
- 真实 Kubernetes：安装/升级/历史回滚/卸载、实际 Secret、冻结随机值、hooks、并发对象修改失效、新 Namespace、CRD 及自定义资源，验证卸载保留 CRD；只清理带本轮所有权及 UID 前件的对象。
- PostgreSQL/HTTP：加密输入、幂等、审批与后台队列、实例围栏、未知结果不重放、逐对象授权、Namespace UID、撤权、Cookie/CSRF 与 `no-store`；该层使用明确的受控原生适配器。
- 原生协议/门户：逐写 nonce/epoch/命令绑定及到期拒绝；新增四个中英明暗门户生命周期场景，回归既有对象变更、回滚和交互会话。浏览器用 mock，不冒充已部署端到端。
- Linux 受限容器验证 Schema/file/网络拒绝和超量模板终止；本机单元测试也验证受限渲染。它们不等于 Windows 主机监控、事件日志或 IIS 的正式实测。

尚未完成正式部署的 Connector/Gateway/Ingress/门户/Agent 全链路、全部生产 Chart/拓扑和异常组合、全部 K8s 类型，以及完整 Plan8 发布门禁。列表/历史受 200 条及响应预算限制，超限明确返回不可用，不宣称已覆盖无限量目录。正向 CSI 扩容、Namespace 写竞态的完整矩阵、drain 取消模型工具、其余采集/集成/默认内容与模型业务闭环继续实施。

T11/T15/T16 与整体 Plan8 保持 `in_progress`；76 个基础正式场景和 60 个内置子门禁没有因本轮组件测试改成通过。Windows 主机实测沿用户决定继续暂缓；本轮不发布。
