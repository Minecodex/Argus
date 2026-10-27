# 真实模型业务门禁与驱动修正

日期：2026-10-21。实现、实际执行和发布分别计数。

## r38 结果与定位

r38 使用正常门户、API、主机接入、Collector 和查询链路，7 项真实门户测试、主机指标及仪表盘、文件轮转/checkpoint/Journald、主动断线与恢复通过。进入新增 K8s 模型场景前，夹具将已有 Deployment 从 2 缩为 0；其确认请求复用了前一次扩容的幂等键，平台按合同返回 `IDEMPOTENCY_CONFLICT`。这是验收驱动错误，尚未发出该场景的真实模型请求，不归为集群资源版本冲突。

共享确认驱动现使用运行 ID 与不可变 `action_ref` 生成固定长度摘要键：同一动作重试保持相同键，另一动作使用不同键，显示名称不参与网络键。r40 曾发现第一次修正保留长场景名前缀，叠加摘要和运行 ID 超过接口 128 字符上限，日志故障确认因此被正确拒绝，模型尚未发出请求；该失败保留。新增回归直接加载当前 OpenAPI 的长度与字符合同，覆盖长运行 ID、超长/中文显示名、改名重试、不同动作及 CSRF 保留。完整驱动包回归通过，见 `artifacts/plan8-fixed-length-confirm-product-regression.log`。没有修改平台幂等或授权规则，也没有对失败动作自动放行。

同轮复查发现新驱动误用了内部 MCP 名称作为公开分类。真实 Tool Gateway 对外提供 `metric/query`、`log/query`、`trace/query` 和 `k8s`。应用、表单及 K8s 模型断言已按该合同修订；共享查询断言同时要求显式 `resource_ids`，查询文本碰巧包含 ID、失败调用或其他目标均不能计数。回归使用当前共享分类清单验证这些约束。

r38 原环境恢复记录为 passed，server 1/1，自有 Namespace 已清零。失败和恢复记录保留在 `artifacts/plan8-deployed-model-flows-r38`，没有将未执行的模型场景标为通过。

## 八个业务场景

每场最多 32 次真实 Provider 调用、600 秒；多轮对话按整场合计。确认后的续跑也取最终用量，不以等待确认时的初步用量代替终态。

| 场景 | 核验边界 |
| --- | --- |
| K8s 零副本恢复 | 自有 Namespace 取证、精确 scale2、确认前无副作用、一次确认、同 UID/模板、实际 2 Ready |
| 单实例暂停 | 一个数据库停采，另一实例继续产生新样本，不能改凭据或其他配置 |
| 应用与共享依赖 | 当轮两个实际 SDK 应用分别取 APM、日志及共享数据库指标，Workspace 计算与独立查询一致，共享目标只计一次 |
| 安全监控表单 | 模型请求表单不创建配置；浏览器私密输入，一次确认后实际采集；同会话自然追问取数 |
| SDK 配置和 Key | 模型读取实际接入配置、预览一个新 Key；用户确认并经专属 HTTP 接口领取一次；实际 SDK 发数据并验证撤销/轮换 |
| 停采、认证与告警 | 两个独立真实 MySQL 目标，一项停用、一项错误认证；实际原生失败、后台 NoData 评估与模型只读解释，不从无数据猜测原因 |
| 同会话撤权 | 实际 SDK 查询和真实 PVC 文件；通过正式授权 API 撤权后旧下载拒绝，模型不能借旧结果扩权，另一应用仍可读；结束恢复该夹具授权 |
| 仪表盘 Workspace 分析 | 正式主机仪表盘取数，实际结果文件、来源、范围和统计结论一致 |

新增三场完整驱动已经编译并接入正式流程，尚未运行，不计为模型验收通过。SDK 的密钥领取只通过用户身份专属接口，原值不会作为模型工具结果；事件 canary 检查的范围是持久化公共工具和会话事件，不冒充 Provider 线级审计。

相关包回归：`artifacts/plan8-r39-model-drivers-regression.log`。当前前端无新增业务组件，继续复用已有安全表单和正常门户。

## 独立真实行为证据

`artifacts/plan8-runtime-fixed-20261021-r38-1321722096/manifest.json` 记录固定源码下两项通过，源码前后相同：

- Calico 中通过 PostgreSQL 预览/确认/Worker 执行网络策略，实际流量依次允许、拒绝、选择性恢复及删除后保持允许。
- Minikube 中通过同一领域流程建立 HPA，实际 CPU 指标导致扩容，降低负载后实际缩容。

两项的原生传输仍为受控适配器，不能代替整套已部署 Connector 工作台验收。CSI 三镜像在两个自有 Calico 节点的六项内容检查完整，见 `artifacts/plan8-calico-csi-dependency-ready-check.json`；这只证明安装前件，实际 CSI Ready、隔离部署和八场模型业务仍须执行。

全部支持组合、故障/容量、76 项总矩阵、最终源码及制品冻结与统一发布继续保留门禁。

## PVC 真实绑定与环境前件

K8s 操作组件 r40 先因节点缺 `alpine:3.21` 缓存失败；补齐同版本镜像后，r41 的三个控制器实际回滚及 PostgreSQL 工作流通过，剩余两个 PVC 用例仍失败。隔离集群的 `standard` 为 `WaitForFirstConsumer`，测试原先没有消费 Pod，未调度的 PVC 本来就不会绑定。

两项夹具现仅在该绑定模式下创建本轮 Namespace 内的真实消费 Pod，UID 10001、只读挂载、零终止宽限且不挂 ServiceAccount Token；Immediate 模式保持原路径。没有改 StorageClass、放宽绑定断言或延长 35/40 秒窗口。实际重跑两项通过，见 `artifacts/plan8-pvc-wffc-product.log`；不支持扩容和缩容仍被拒绝，所有权清理通过。这是原生操作组件证据，后续完整固定源码集及正式 Connector 链路分别验证。

r42 操作组件整组在冻结源码下通过：`artifacts/plan8-operations-fixed-20261021-r42-2141087819/manifest.json`，八项原生用例与后台工作流全部通过，源码前后一致。领域/工具/来源撤权、连接恢复与上下文门禁同轮也分别通过，原始清单保留各自验证层级，不抵全部部署或真实模型业务。

## 隔离集群的工作区设备前件

r42 完整安装和七项门户验收通过，进入业务前被沿用的 Plan7 预检拦截：原函数仅允许 Docker Desktop 的单个 CSI Pod。八场模型尚未运行；原环境恢复和所有权清理通过。

Plan8 现将两种环境分开：Docker Desktop 保留原有范围限定的设备别名修复；本轮拥有的 Kind 安装仅执行只读检查。后者要求 `plan8` 套件、受限上下文、当前测试 Namespace 的 release 标签、CSI Helm 归属、所有已调度节点的 Pod/DaemonSet UID、运行镜像摘要和 `/dev` 挂载全部匹配。每个 Pod 按名称执行固定只读脚本，结束复验 UID；不使用选择器任取一个节点，不自动修改 Kind 的设备。拒绝外部 Namespace/Driver/控制器/节点、未知镜像、缺节点及未就绪的回归已通过，见 `artifacts/plan8-kind-device-check-tests.log`。完整安装中该只读路径仍待重跑。

独立准备阶段发现两个临时 Kind 容器原本各有 8 个别名。按 `owner.json` 核对工作区、集群名、Docker 容器 ID、容器 label，确认 `/dev` 是容器自己的 tmpfs 且不是外部挂载后，创建缺少的 `/dev/loop8`～`/dev/loop63`，并核对所有设备的类型及主次设备号。没有附加/格式化/卸载/删除实际卷或改动 backing file；完整创建清单见 `artifacts/plan8-owned-kind-device-preparation.json`。该准备记录不能替代下一轮 CSI 内的只读验收回执。

r43 的新校验拒绝了导入后的镜像索引表示，工作区和模型尚未执行，恢复与清理通过。实际节点返回 `import-2026-10-21@sha256:4f34…`，该 OCI 索引引用锁定的 Linux/amd64 清单 `sha256:9f804…`，与原始上游索引 `sha256:eb99…` 指向相同内容，另含导入附加的 attestation。

现复用已有镜像描述符验证：逐个 SHA256 验证节点读取的索引/清单原始字节，按已验平台选择清单，并比较锁定平台清单和配置摘要。仅在核对 Kind 节点所属集群后只读获取内容，不按导入名或任意 `sha256` 放行。三个公开描述符固定为测试数据，篡改单字节、其他平台及未知摘要均拒绝。实际两个节点的产品验证方法通过，见 `artifacts/plan8-rawfile-node-content-product.json/.log`；临时 overlay 仅增加测试入口，没有替换产品实现。完整 CSI 内只读脚本和八场业务仍待下一轮。

## 多节点传输适配

r44 的完整安装、门户和 M4 通过；新工作区前检在两个实际 CSI Pod 中通过，回执 `artifacts/plan8-calico-completion-20261021-r44/plan8-loop-device-prerequisites.json` 逐节点记录 UID、运行镜像、64 个设备别名、控制设备及零变更。工作区配置阶段随后通过，尚不等于真实查询文件已经交付。主机接入被旧传输的单 Desktop 节点假设拒绝，后续模型未执行；原部署恢复和临时 Namespace 清理通过。

已横向检查本轮复用的 Plan6/7/8 路径。设备修复仍限定原 Docker Desktop；桥接与节点镜像读取改用同一本地环境解析器。Docker Desktop 保持其单个既定节点；Kind 要求 Plan8 上下文、当前工作区内的受控所有权记录、集群名、精确节点容器 ID 和运行时集群标签。支持 PowerShell 写入的 UTF-8 BOM 记录，但不放宽任何归属字段。节点候选按 Ready 状态筛选并稳定优先工作节点，随后核对 Kubernetes InternalIP 与 Docker 网络地址。

桥接继续使用本轮容器在客户外部网络与节点共享网络间转发原始 TLS 字节；没有伪造主机、指标或模型结果，不终止 TLS，不允许删除共享网络。证明字段改为实际上下文、节点名、节点 UID 和容器 ID，避免把 Kind 写成 Desktop。

错误工作区/集群/容器 ID、重复/缺失记录、未就绪节点、非批准上下文和地址不一致的拒绝回归通过。真实两节点的归属与网络一致性、镜像内容验证通过，见 `artifacts/plan8-local-environment-live-product.log`、`artifacts/plan8-owned-node-network-product.json`。这些是只读节点组件证据；真实桥接和正式主机接入继续在完整流程验收。


## r47 故障注入前置与异步 Outbox

r47 的主机正式接入和指标/仪表盘通过，文件日志阶段因“Collector fault injection window was not observed”提前失败，模型未执行。清理和原环境恢复通过，server 1/1、本轮 Kind Namespace 清零。

检查发现旧查询在提交后立即按 Collector 当前 desired_revision 找命令，但配置编译经异步 Outbox；编译尚未前进时可能读到上一版已经成功的命令，提前认定故障窗口错过。本轮未留该次命令快照，不能声称竞态时序已独立复现。现先冻结提交前 revision，故障查询只接受更高 revision 的命令；证据保存前后版本，错误也显示安全的版本/状态。原 12 秒准备窗口、20 秒进程延迟、重连断言及真实模型预算未放宽，下一轮正式验证。

同轮另用实际 PG/API 通过四类 RBAC 批准变更和两层授权拒绝，见 [71](./71-kubernetes-rbac-live-acceptance.md)。
