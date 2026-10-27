# K8s Gateway 持久状态与 Pod 替换

状态：实现已合入，相关四个包最终回归通过（`artifacts/plan8-state-preflight-final.log`）。独立真实 K8s 的非 root 文件写入、Pod 替换、相同 PVC 内容恢复及 Namespace/PV 清理已通过；正式 Collector 账本和应用日志链路待下一轮验证。普通文件恢复不代表实际发送批次已经跨 Pod 恢复。

## 存储边界

Gateway 原先使用 EmptyDir 保存身份文件、持久发送队列和故障报告账本。它只能跨容器重启保留，不能跨 Pod 删除保留。本轮为每个受管 K8s Collector 在受控 Namespace 创建固定名称 `argus-otelcol-gateway-state` 的 1 GiB 文件系统 PVC，交给集群已有默认 StorageClass 提供存储。没有可绑定存储时安装失败，不回退 EmptyDir，也不自行安装存储服务。

跨节点、节点或存储故障时的恢复能力由实际 StorageClass 决定，本轮本地卷结果不扩大为节点故障切换证明。生产前件是该 Namespace 可创建并绑定单写文件系统卷，通常需要合适的默认 StorageClass、配额和 CSI/本地卷能力。1 GiB 是申请容量，不是已测吞吐、磁盘使用承诺或保存时长。实际故障与容量仍按冻结规格验收。已有同名 PVC 必须属于相同 Collector、未删除/丢失、容量不小于请求且支持适用文件系统及访问模式；不能通过改标签认领他人的数据。

Gateway 继续单副本和 Recreate 更新，避免两个进程争用同一队列数据库。配置验证 Job 只解析候选，使用独立 EmptyDir，不挂载运行中的单写卷，避免校验任务阻塞当前服务或写入运行账本。Node Agent 不共享 Gateway PVC；K8s 日志 Agent 的 Collector 专属 HostPath checkpoint 机制保持原有边界。

Connector 安装脚本新增该 Namespace 内 PVC 创建及固定名称的读取、删除权限，不增加集群 PV 管理权限。卸载前将 PVC 加入完整所有权检查集，确认所有目标后才开始删除，并用 UID 前件删除同一对象。卷的最终回收遵从集群 StorageClass/PV 策略；Retain 策略保留的物理卷不由平台擅自删除。

Collector 上游和正式二进制仍为 0.159.0 / `0.3.2-plan8`，本次修改在管理器和部署层，不 fork Receiver 或发送队列实现。

## 分层证据

- 候选及正式源码分别运行真实 PVC 测试。UID 10001 在两个不同 Pod 中读取完全相同的内容，PVC UID 和物理卷名称不变；本轮 Namespace 与动态卷全部删除。正式证据为 `artifacts/plan8-k8s-state-final.json` 及 `plan8-state-capacity-final.log`，仅证明 K8s 文件系统持久性。
- 单元验证覆盖跨 owner、删除中、Lost、块设备、容量不足、只读卷、缺 owner 标签，以及校验任务隔离、他人卷阻断卸载且不发生部分删除。
- 正式驱动新增真实 Gateway Pod 按 UID 删除与重建。由于产品镜像为 distroless，使用本轮已有测试镜像作为非 root 只读观察 Pod，仅以 subPath 挂载 `delivery-state.json`，不挂载身份目录、证书、密钥或队列。前后核验同一报告身份、单调序号、相同 PVC，并随后实际产生新的工作负载日志，通过应用范围查询验收。该步骤尚未运行，不能预先计通过。
- 32 个专用 HTTP Receiver 的 60 秒、384 MiB 冻结子项已作为正式源码测试交付。正式源码证据 `artifacts/plan8-capacity-dedicated-final.json` 记录连续 60,186 ms、32 个独立监听端点和峰值 RSS 112,852,992 字节，故障与恢复都要求转变时间之后的新指标，独立启停及全部停采通过；它使用受控 OTLP 捕获，不能抵正式接入、查询、磁盘队列、网络/TLS、配额或完整 A62。

新增具名 `k8s-state` 组后共 73 组组件门禁；SDK 组纳入 32 目标子项。全部 76 项正式验收、支持组合、模型业务、故障容量与统一发布继续保留各自门槛。
