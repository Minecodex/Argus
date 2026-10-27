# P8-T11：Kubernetes 在线变更与会话

状态：in_progress。前件：P8-T03、P8-T04、P8-T10。

2026-10-20 当前结论见 [54 完成度复核](./54-completion-audit.md)：日志、终端/转发及上传 Chart 的 Helm 生命周期均已有实现和分层证据。原计划的 Helm 仓库/OCI 来源接入已由 [55](./55-helm-repository-and-oci.md) 补充实现及分层验证；drain 取消模型工具、CSI 成功扩容及完整竞态/部署验收继续未完成。下列节点保留各自历史时点，不能将其旧缺项直接当作当前状态。

## 历史实施节点

本次新增 [47 Pod 日志会话](./47-pod-log-sessions.md)：Namespace/Pod/容器/重启代次、一次性票据、当前/上次增量日志、撤权及暂停检查、清退和门户已实现并有分层验证。下文未完成项保留各自历史时点；终端/端口转发和正式部署会话验收仍待完成。

最新补充见[43 Node drain](./43-node-drain.md)：已实现 PDB/存储影响、冻结 UID 的驱逐、取消、撤权和失联继续，以及真实双节点、数据库和双人审批门户验证。新增 PDB 标准类型，写协议已升级 v5；下文 v4/更早实施记录保留历史时点。Helm、授权会话、CSI 成功扩容及正式 Connector/Agent 全链路仍未收口。

最新原生补充见[存储、Job 与分配字段](./26-kubernetes-storage-jobs-and-allocations.md)：实际 PVC 不支持扩容/禁止缩容、Job 随机 dry-run UID、Service 自动分配和 PVC 控制器绑定均有对应验证；CSI 成功扩容和全部工作台闭环仍待完成。

Deployment、StatefulSet、DaemonSet 的历史修订选择、Pod 模板回滚及真实审批工作流已补充，见[范围与证据](./29-kubernetes-workload-rollback.md)。当前写协议已随 [CronJob 手动运行](./31-cronjob-manual-run.md)升级为 v4；v3 为历史记录。正式 Connector/门户/Agent 全链路及其他工作台功能仍待完成，当前缺口以[完成度核对](./32-implementation-audit.md)为准。

## 设计与接点

- [04-credentials-and-api-contracts.md](./04-credentials-and-api-contracts.md)
- [05-host-and-kubernetes.md](./05-host-and-kubernetes.md)

已有/拟扩展接点：`internal/action`、`internal/resource`、`internal/kubernetesreader`、`internal/connector`、`internal/remoteaccess`、`internal/remoteexecution`、`web/apps/enterprise/src`。新领域包和具体接口在 T01 固化；路径是定位入口，不代表本任务已实现。

## 实施清单

- [ ] 实现Jobs/CronJobs、HPA、NetworkPolicy、RBAC、Node cordon/uncordon/drain/污点、CRD及实例CRUD、端口转发与Helm安装/升级/回滚/卸载。
- [ ] Helm锁定Chart/values/渲染及hooks，逐对象授权；CRD定义与实例分权；drain验证PDB/存储/恢复，转发固定目标并撤权关闭。
- [ ] 实现已列对象的类型化创建/更新/删除、工作负载扩缩容/滚动重启/回滚、PVC 受控扩容与依赖预览。
- [ ] 服务端 dry-run、类型 Schema/受控字段校验和完整 Diff，冻结 UID/resourceVersion/身份/授权版本；冲突后重新预览。
- [ ] 实现异步提交与就绪观察、操作状态/取消/unknown、幂等与重启恢复；不把 API 接受当业务就绪。
- [ ] DaemonSet 不接受普通 replicas；StatefulSet/Namespace/PVC 删除展示存储和对象影响。
- [x] 实现实时/上次 Pod 日志会话与受控容器终端，固定 Pod UID/container/重启代次，独立短期票据与审计；组件验证见 [47](./47-pod-log-sessions.md) / [49](./49-kubernetes-stream-sessions.md)，完整部署门禁尚待执行。
- [ ] 沿用实际 K8s 接入身份；Secret 写入使用敏感输入机制，通用 YAML 不回传 Secret 原文；动态 GVR 按注册类型、Schema与作用域授权，RBAC提权字段独立检查。

## 完成标准

- [ ] 真实集群执行创建、改镜像、扩缩容、重启、回滚、删除和 PVC 能力允许的扩容；以实际状态和事件核对结果。
- [ ] 双编辑者冲突、UID 替换、权限拒绝、超时未知、Worker/Connector 重启、票据撤销不造成重复副作用或越权。
- [ ] 相关存储、接口/生成客户端、权限、门户/工具、审计、配置和文档保持一致；单文件小于 2000 行。
- [ ] 保存测试范围、失败迭代、源码/版本与证据；只完成 mock 或单层测试不计真实验收。

## 验收关联

对应 [验收矩阵](./09-acceptance-matrix.md)：P8-A38、P8-A39、P8-A40、P8-A41、P8-A42、P8-A43。

新增验收关联：P8-A65、P8-A66、P8-A67、P8-A68、P8-A69、P8-A70、P8-A71、P8-A72、P8-A73。

## 实施记录

已实现第一批对象变更流程：类型与权限校验、服务端 dry-run、UID/resourceVersion 冻结、公开差异预览、PendingAction 确认、独立异步队列、发送前撤权复核、原子领取及只读结果恢复。对象创建/合并补丁/删除、工作负载扩缩容/重启及 Node cordon/uncordon 已接入同一核心；操作历史有独立授权与分页。成功响应先记录已接受，再观察对象存在/删除和工作负载实际就绪；后续修改会标记 superseded。

- [真实数据库与真实 API 工作流](../../artifacts/plan8-kubernetes-operation-workflow-live.log)：授权 Namespace 中 ConfigMap 的预览、确认、创建、更新、删除及 Execution/审计收口通过；隔离 Namespace 已删除。后端注入真实 API 适配器，不等于已部署 Connector/浏览器全链路。
- [真实 Deployment 变更](../../artifacts/plan8-kubernetes-mutations-live-verified.log)：原生 Kubernetes API dry-run、ready 副本扩缩容、重启、版本冲突和确认删除通过。
- [数据库恢复门禁](../../artifacts/plan8-kubernetes-mutation-orchestration.log)：未知结果不重放、执行前撤权拒绝、进程中断保持 unknown、资源授权撤销后历史不可读。未以模拟后端的结果替代实际集群状态。
- [中英/明暗门户](../../artifacts/plan8-kubernetes-mutation-ui.log)：创建、编辑、差异确认、操作记录与可访问性四场景通过，属于 mock 交互验证。
- [生成契约修复](../../artifacts/plan8-kubernetes-contract-repair.log)：权限注册表版本同步；写命令拆为独立 proto 源，生成文件保持不超过 2000 行。
- [Secret 真实写入](../../artifacts/plan8-kubernetes-secret-workflow-live-verified.log)：在隔离 Namespace 创建 Opaque Secret，实际值正确，公开对象/预览/计划/审计/结果无原文，终态清理加密暂存。首次发现审计接收者类型不符合数据库约束，已修复并保留初次失败日志。
- [私有输入授权与重放](../../artifacts/plan8-kubernetes-private-input-authorization.log)：输入绑定操作者、操作、目标与 Namespace UID；目标变化和撤权拒绝解密；相同幂等请求不重复预检，不同私有值复用 Key 拒绝；取消后清理。
- [Secret 门户四场景](../../artifacts/plan8-kubernetes-secret-ui.log)：多行内容默认遮蔽，单独提交并在预览成功后清空表单；浏览器持久化状态中无原文。属于 mock 门户证据。

Secret 输入复用 Secret Keyring 的信封加密，临时密文置于专用表中，不进入平台凭据目录。私有材料仅在 Direct 调用或 Connector 发包时解密，命令持久化只存受控引用；Connector 共享结果工厂注册了新的写命令，公开投影去除内部 marker，含敏感字段的对象不持久化可猜测原值的 digest。首次真实验证覆盖 Opaque Secret，未宣称所有 Secret 类型与正式 Connector 路径均已通过。

仍需完整交付 YAML 编辑、通用类型依赖/存储影响、RBAC/工作负载提权约束、Namespace 重建与写入竞态边界、rollout 回滚、drain、PVC 扩容、Helm、终端/转发会话、Agent、正式 Connector 路径与全类型真实验收；私有输入还需覆盖接入身份/配置变更、租约撤销及各 Secret 类型。当前未勾选整体完成项，未发布。

已补上[显式写入权限边界](./18-kubernetes-mutation-privileges.md)：Cluster 数据范围不再直接放行工作负载/RBAC 提权；主机特权、ServiceAccount 身份和 RBAC 委派分别授权。写协议统一升级到 v2，发送前复验限时许可，实际写入前再次原生 dry-run 并核对批准摘要。真实 PostgreSQL、K8s API 和既有 Secret 工作流回归通过。完整 CRD/Helm、存储/Node 影响、会话、YAML 界面和部署后验收仍待完成。

共享 YAML/JSON 清单编辑、真实对象投影预填、格式/别名边界、Secret 私有字段与编辑基线冻结已加入门户。后台刷新及预览前版本变化均阻止旧内容使用新版提交，用户可显式重载。解析 11 项、版本变化 2 项和中英明暗 4 场景通过，见[清单编辑证据](./19-kubernetes-yaml-editor.md)。这仍是门户/受控 API 验证；完整 K8s 功能和部署后真实全流程继续待完成。


CronJob 列表新增运行一次，固定源 UID/RV 和 Namespace UID，通过现有审批/Worker 创建独立 Job；源更新后的失败及重预览成功均在真实 PostgreSQL/API 收口，私有模板不进入公开记录。写协议统一为 v4，见 [手动 Job 证据](./31-cronjob-manual-run.md)。


2026-10-19 已补独立 Pod 终端及 Pod/Service TCP 转发：复用底层传输、独立领域状态与授权，不创建虚假 Host/账号；支持一次性票据、原子执行认领、持续撤权检查、元数据审计、共享门户终端与本机 CLI。真实受限 ServiceAccount、HTTP/WebSocket/TCP/mTLS 分层证据和默认只读 API 身份的权限前件见 [49](./49-kubernetes-stream-sessions.md)。T11 继续保留 Helm、CSI 正向扩容和正式部署全链路等未完成范围。


2026-10-20 继续实现 Helm 安装、升级、回滚、卸载与历史：同一原生 SDK、逐对象权限/dry-run/CAS、审批/队列/围栏、逐写复验、受保护输入、受限渲染及共享门户，见 [51](./51-helm-workbench.md)。真实 API 的 hooks、随机值恢复、新 Namespace/CRD 与保留行为有专项证据；正式 Connector 部署、全部图表、正向 CSI、完整 Namespace 竞态与 drain 取消模型工具仍未完成。
