# Plan7 历史 F33 实现与验收报告

日期：2026-10-17。P7-T01～T09 已实现，并按用户确认的验收环境完成全部 required 门禁。

复查补充：本报告保留 F33 的历史通过结果。后续复查发现的文件类型与原环境前置快照两项问题，已经在 F40 修复并完成整轮验收，当前状态及证据以[最新报告](./11-follow-up-acceptance.md)为准；详见[复查记录](./10-completion-review.md)。不补造 F33 缺失的历史前置快照。

## 冻结版本与结论

| 项目 | 结果 |
| --- | --- |
| 完整运行 | `plan7-20261017-f33`，`--suite plan7 --plan7-phase all`，进程退出 0 |
| 源码指纹 | `417b000a100b6c5742723d6e7b05e9d85b75e2c82466e75554ba68c92ad58fee` |
| 源码前后核对 | before-build 与 after-acceptance 一致；包含本次工作区未提交代码，不能仅用 Git HEAD 代替此指纹 |
| required 矩阵 | **57/57**：A01～A38、M01～M11、U01～U08 |
| 证据完整性 | 矩阵引用的 42 个不同证据文件 SHA256 全部复核一致 |
| 真实浏览器 | **19/19**，无失败、跳过或 flaky；中英文、明暗主题、1280／1440／1920 桌面宽度 |
| 真实模型 | GLM-5.3：75 次 Agent 调用，其中推理 73 次、压缩 2 次；均成功，提供商用量和额度结算完整 |
| 工程门禁 | 全量 Go、Replay 协议、生成契约、前端类型、单测、样式／i18n lint、real 构建通过 |
| 实机补充 | 9 卷 Workspace 挂载／换 Pod 读取、Kubernetes 适配器与隔离边界、Linux runner／runtimewire 通过 |
| 部署自检 | Ingress、Kafka、MinIO、OpenSandbox、PKI、PostgreSQL、Redis、工作负载就绪均通过 |

本轮 Agent 用量为输入 868339、输出 81686 tokens；统计只覆盖该冻结验收运行，不含模型注册兼容性探测或此前失败迭代。模型输出不是验收依据的唯一来源：数据文件、实际远端效果、服务状态、批准记录、工具调用及请求 Hash 均独立核对。

## 交付内容

- 排查／计划模式及 Run 模式快照；connect 每批只生成冻结预览，所有自有远程读取和修改都需确认。
- 固定计划的描述与原生命令、一次批准、严格串行执行、意见修订、停止、失败／未知状态及新版本恢复。
- Linux Bash、Windows PowerShell、Kubernetes 受控 kubectl 与固定 Pod/container exec；接入身份绑定和当前权限复验。
- 共享会话、连接与解释器代次、上下文版本、已知目录／版本／前序批次；进程丢失后不盲目重放修改。
- 专用 input 自动封存和上传、output 自动取回、Hash／来源／完整性记录；文件恢复不重跑命令。
- 后台执行、刷新与重连恢复、明确停止；不同聊天可实际并发，活动事实只进入有权限的模型上下文。
- PostgreSQL 业务事实、Outbox／租约／围栏、独立停止容量；迁移 00015、00016，以及同步的协议、SQL、OpenAPI、客户端与权限注册。

## 验收环境与边界

Linux 使用本轮拥有的 Docker SSH/systemd 目标，并通过正式接入、Connector、Collector 与 OTLP／ClickHouse 链路验证。Kubernetes 使用隔离 namespace，同时覆盖 Connector 与 Direct Executor。Windows 使用本机 Windows 11 专业版、独立 Connector／执行进程与临时目录，核对实际账号 SID、PowerShell 状态、编码、错误和进程树停止。

**Windows Server 兼容性仍为单独待验证项**，本机结果不替代 Server 实机证据。客户 MCP 继续既有独立配置；本轮确认规则覆盖自有远程执行，不代表任意客户 MCP 或任意脚本已被静态证明安全。

这是隔离环境的实现与验收交付。既有 Argus 部署已恢复，没有将本轮新镜像发布到既有业务部署。

## 证据入口

- [验收摘要](./evidence/plan7-20261017-f33-summary.json)
- [57 项矩阵副本](./evidence/plan7-20261017-f33-matrix.json)：其中 evidence.path 相对于下列原始证据目录。
- [原始结果](../../build/plan7-evidence/plan7-20261017-f33/result.json)、[完整日志](../../build/plan7-all-f33.log)
- [构建前源码清单](../../build/plan7-evidence/plan7-20261017-f33/source-before-build.json)、[验收后源码清单](../../build/plan7-evidence/plan7-20261017-f33/source-after-acceptance.json)、[镜像来源](../../build/plan7-evidence/plan7-20261017-f33/image-provenance.json)
- [模型调用汇总](../../build/plan7-evidence/plan7-20261017-f33/plan7-model-usage-summary.json)、[真实修复](../../build/plan7-evidence/plan7-20261017-f33/plan7-service-recovery.json)、[未知结果恢复](../../build/plan7-evidence/plan7-20261017-f33/plan7-model-recovery.json)、[私有并发上下文](../../build/plan7-evidence/plan7-20261017-f33/plan7-model-activity.json)
- [主浏览器报告](../../build/plan7-evidence/plan7-20261017-f33/playwright-plan7/report.json)、[真实模型历史浏览器报告](../../build/plan7-evidence/plan7-20261017-f33/playwright-plan7-model/report.json)
- [工程检查](../../build/plan7-evidence/plan7-20261017-f33/plan7-engineering.json)、[部署自检](../../build/plan7-evidence/plan7-20261017-f33/verify/verify.json)
- [环境恢复与清理](../../build/plan7-evidence/plan7-20261017-f33/plan7-cleanup-verification.json)、[独立数据库夹具清理](../../build/plan7-evidence/plan7-20261017-f33/plan7-database-fixture-cleanup.json)

本轮测试 namespace、Docker 目标、Windows 测试进程和临时目录已清理。原 20 个工作负载的 namespace UID、工作负载 UID、副本数与就绪数均复核恢复，共享 OpenSandbox 控制器身份与就绪状态保持。独立 PostgreSQL 测试夹具也在核对容器 ID 与所有权标签后删除；保留源码、证据及无关业务数据。

## 运维配置

| 配置 | 默认值 | 说明 |
| --- | --- | --- |
| `runtime.remoteIdleSeconds` / `ARGUS_REMOTE_IDLE_SECONDS` | 1800 | 无活动执行才回收，保护未收集的唯一输出 |
| `runtime.remoteExecutionWorkers` / `ARGUS_REMOTE_EXECUTION_WORKERS` | 8 | 每个 Worker 的远程执行容量，范围 1～64 |
| `runtime.remoteControlWorkers` / `ARGUS_REMOTE_CONTROL_WORKERS` | 2 | 独立停止容量，范围 1～16 |

Worker 容量限制不等于按资源独占排队；同计划／同 shell 仍串行。新的远程命令始终重新确认；共享连接、模型压缩、恢复和文件重传均不产生通用授权。

此前 f01～f32 是诊断和修复记录，见[实施记录](./08-implementation-notes.md)，没有与 f33 的通过项混合计数。
