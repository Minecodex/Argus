# B10a 常用 Kubernetes 对象验证

本批创建自己的 Kind 集群，先完整退役 B09b，正式 Argus 与历史保留。

通过正常工作台预览/确认创建和管理 Namespace、Deployment、StatefulSet、DaemonSet、Service、Ingress、ConfigMap 和 Secret；核对原生 UID、resourceVersion、实际 HTTP 结果、配置更新、受保护字段脱敏、扩缩容、重启、回滚、Pod 替换和版本冲突。日志与 Pod 终端通过当前门户检查；端口转发按 2026-11-09 的既定产品边界使用现有 CLI/API，门户不重新添加入口。

平台接入身份只有本批命名空间的对象写入、exec 和转发权限；Namespace 创建权限仅在本批独立集群，更新/删除限定 b10a-transient。核对 kube-system 写入与 RBAC 委派被拒绝。数据来自实际 HTTP、容器日志、原生 Collector 与 Kubernetes 对象，不写 Argus 业务或遥测库。

保留 Deployment/StatefulSet/DaemonSet、配置和网络对象供用户查看。至少一次真实副本不足告警触发与恢复；当前低速请求每 15 秒一次，最多 8 小时，到期停止增长性负载但保留环境。节点 3 vCPU / 3072 MiB，业务容器各自设置资源上限。

操作本身通过 Argus 工作台；Kind、采集安装前置 RBAC、Ingress 控制器属于明确的运行基础设施，原生核对不代替平台操作证据。最终状态、链接和未通过项记录于 artifacts/plan9/p9-b10a-20261111-01。
