# 正式基础链路与网络隔离验收

状态：r33 的 14 个功能阶段全部通过；整轮因部署 NetworkPolicy 校验失败退出，不能记录为整体验收通过或发布成功。

`artifacts/plan8-completion-deployed-r33/plan8-deployed-foundation.json` 记录主机正式接入、真实指标/仪表盘、文件轮转/checkpoint/故障恢复/Journald、K8s 规则和 CRI/Gateway 重建/独停/卸载/历史、多数据库与采集迁移、Redis/Nginx 默认面板、三协议 SDK、真实模型和 Workspace 的当轮结果。

真实 GLM 单实例暂停为 6 次调用、一次确认；主机仪表盘 Workspace 场景为 15 次调用。两个场景均终态 succeeded，输入/输出及缓存用量结算完整，各在冻结的 32 次/600 秒预算内。前者只暂停选定实例，后者使用实际查询文件核验结论。

安装后统一校验 `verify/verify.json` 的 passed 为 false，原因是 NetworkPolicy 探针没有观测到预期的强制阻断。Docker Desktop 当前 CNI 的这一结果不能用功能通过、告警提示或人为豁免替代。r33 最终退出 1，原环境恢复 passed、原 server 1/1、自有 Namespace 已清理。

下一轮准备使用本任务已有、所有权记录可核对的 Kind/Calico 测试集群，仍运行在当前 Docker Desktop，不修改现有业务集群 CNI。只读前检确认两个 v1.36.1 节点 Ready、Calico 3.33.0 运行、默认文件系统存储类可用，整体工具前检 ready。结果为 `artifacts/plan8-calico-full-preflight.json`；前检本身不证明完整安装、策略探针、模型出口或业务流程通过。

当前主机内存有限，正式测试可按既有授权暂停原 Argus 工作负载。环境驱动复用现有 E2E 的 Namespace/Workload UID、副本、卷基线和恢复核验；限定原 `argus-system`/`argus-observability`，保留入口与共享控制器。带有 scale-down 删除 PVC 策略的 StatefulSet 拒绝暂停。无论新测试成功或失败，都必须在 finally 恢复原副本并核验存储与健康。只清理本轮拥有的临时资源。

新增的真实模型安全表单场景已实现驱动并通过编译/浏览器类型检查，但尚未真实执行。它不计为通过；运行后还须继续其他五个模型场景、完整支持组合、K8s 其余操作、故障容量、76 项汇总与统一发布。


## r34 安装前件失败与恢复

r34 在自有 Calico 集群首次安装时，Strimzi 1.1.0 镜像尚未拉取完成，Helm 按固定 15 分钟上限失败并回滚；业务场景和 NetworkPolicy 探针均未执行，不计通过。原 Desktop 的 18 个工作负载已记录并暂停，失败后先提前恢复，再由外层 finally 幂等复验，副本、身份和存储均通过，server 1/1。

清理期间仅对本轮删除中的 Namespace 内、精确 UID、Pending 且没有任何已启动容器的一个 Strimzi Pod 进行 UID 前件删除，未去除 Namespace finalizer、未操作业务数据。失败日志、精确清理与恢复分别保存在本轮 artifacts。

同版本镜像最终由宿主机下载完成。首次直接导入因多架构清单缺内容失败，改为导出 Linux/amd64 归档后，两个自有 Kind 节点均确认存在原标签。现在先在原服务正常运行时准备锁定依赖缓存，记录原引用、摘要和每步结果；不更改版本来绕过故障。缓存完成后再开始下一整轮。

此外应用 APM/日志与共享数据库的第四个真实模型场景驱动已实现并通过包回归，尚未实际执行。初始样本来自同轮 SDK/服务证据，比较两个同名服务所属应用、实际样本/错误数和共享资源去重，要求模型实际取数、读取 Workspace 并不产生变更；负向授权验收仍另行保留。
