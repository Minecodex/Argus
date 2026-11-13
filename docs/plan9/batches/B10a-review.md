# B10a Kubernetes 工作台查看记录

历史运行 `p9-b10a-20261111-01` 已于 2026-11-12 按用户下一轮指令完成退役。独立 Kind 节点、Collector、凭据授权和专属入口已回收，正式六个 PVC 保留，历史指标、告警、操作和证据保留。该指令不视为对每项反馈的显式验收通过。当前查看 [B10b](./B10b-review.md)。

2026-11-11 的日志/终端反馈已修复并发布：[打开即显示日志、滚动查看与终端诊断](../16-b10a-pod-logs-and-terminal-20261111.md)。在 `b10a-workloads` 的 `b10a-api` Pod 上可验证真实终端；Collector Agent 所在命名空间未授 exec，且该精简镜像没有 Shell。

后续复查已修正安静容器的默认日志范围和操作栏：[最近行读取、按钮对齐与普通终端前件](../17-b10a-quiet-pod-logs-and-actions-20261111.md)。Collector 的真实启动日志现在按保留行数显示，不再被默认五分钟过滤；普通终端测试仍选择包含 Shell 的业务 Pod。

当前哪些 Pod 能连终端以及操作步骤见 [四个业务 Pod 的完整实测名单](../18-b10a-terminal-matrix-and-log-options-20261111.md)；展开“日志选项”后的两个输入框和应用按钮也纳入独立对齐检查。

## 查看路线

1. 打开 [B10a 资源工作台](https://argus.dev/kubernetes/01a1c9ea-4bfa-74ef-86a0-f899784cdf5e?tab=resources)，选择 `b10a-workloads`。
2. 工作负载查看 `b10a-api` Deployment、`b10a-ledger` StatefulSet、`b10a-node-observer` DaemonSet；Pod 行可打开真实日志和终端。
3. 网络查看两个 Service 和 `b10a-http` Ingress；配置查看 `b10a-settings`、`b10a-demo-secret` 以及四组 `b10a-ui-*` 中英明暗创建/更新结果。Secret 内容应为脱敏占位。
4. 在「操作记录」检查创建、编辑、扩缩容、重启、回滚、删除、分页和最终状态；版本冲突保留为正常拒绝结果。
5. 查看 [本集群四个仪表盘](https://argus.dev/kubernetes/01a1c9ea-4bfa-74ef-86a0-f899784cdf5e?tab=dashboards) 和 [采集配置](https://argus.dev/kubernetes/01a1c9ea-4bfa-74ef-86a0-f899784cdf5e#otlp-collector)，三项采集应为 healthy/fresh。

| 仪表盘 | 内容 |
| --- | --- |
| [B10a 工作台 / Workbench](https://argus.dev/dashboards/740ef95f-5ae5-403c-99ef-e8041e6f60af?resource=01a1c9ea-4bfa-74ef-86a0-f899784cdf5e&resource_type=kubernetes_cluster) | 可用/期望副本、重启、CPU、内存，5 面板 |
| [Kubernetes cluster](https://argus.dev/dashboards/1764d344-bb5a-5b16-9f09-13f68a452bc9?resource=01a1c9ea-4bfa-74ef-86a0-f899784cdf5e&resource_type=kubernetes_cluster) | 工作负载、节点、CPU 资源请求，6 面板 |
| [Kubernetes nodes and containers](https://argus.dev/dashboards/c17f3435-0dc8-544f-bef4-e5e71c3bc381?resource=01a1c9ea-4bfa-74ef-86a0-f899784cdf5e&resource_type=kubernetes_cluster) | 节点/容器 CPU、内存、文件系统与网络，6 面板 |
| [Collector](https://argus.dev/dashboards/258c6833-6dce-5240-bddf-81bbf7c6f2c2?resource=01a1c9ea-4bfa-74ef-86a0-f899784cdf5e&resource_type=kubernetes_cluster) | 内存、运行时长、队列、发送，6 面板 |

默认查看最近 15 分钟即可；要看副本不足过程，切换到下述绝对时间窗口。

## 告警与真实行为

[B10a 副本告警历史](https://argus.dev/alerts?tab=history&policy_id=5954bcfa-2509-52dd-b20a-665f5dfafbb8) 对应实际通过 Argus 把 `b10a-api` 缩容到 0、再扩容到 1。北京时间 **2026-11-11 17:58:40** 触发，**17:59:20** 恢复，建议查看 **17:57–18:01**。当前 Deployment 为 1 个可用副本；平台内策略保留，未设置外部通知收件人。

ConfigMap 挂载当前 `mode=updated`；版本升级后已回滚到 v1。Secret 已实际轮换并验证挂载刷新，但不公开原值。Service 的 8080/8081 都指向实际 HTTP 容器，Ingress `b10a.test` 已从集群内通过真实请求验证，未增加正式公网域名入口。

原生容器继续产生实际日志和三项指标。HTTP 请求每 15 秒一次，截止北京时间 **2026-11-12 01:55:52**；到期请求停止，环境仍保留。StatefulSet 使用 emptyDir，持久存储留给 B10b。

Pod 终端已通过页面执行读取实际挂载文件，CLI 已完成 Service TCP 转发并正常关闭。门户按既定边界没有端口转发按钮；该项不是等待下一批补界面。

## 本轮修复和保留事实

已修复 Secret 引用被误判为明文、操作历史超过 20 条分页失败、凭据一次交付后终端/转发被错误关闭，以及短命 Namespace 指标等待库存导致整个批次超时四项共享问题，见 [修复记录](../15-b10a-workbench-lifecycle-20261111.md)。

**当前仍可能看到一条 Collector delivery data gap：它记录修复前真实丢失的 133 点，最后观察于北京时间 17:53:13。** 这是已发生的历史缺口，按设计保留到明确处理；没有清零、伪造补点或自动确认来隐藏。修复后已重复真实 Namespace 创建/删除，观察 311.8 秒，三类投递失败增量均为 0，原生采集持续推进且日志没有新的终止丢弃。当前列表与概览没有未触发的 normal/no_data 内部状态。

## 自动自检范围

- 八类对象及 Pod，实际创建/更新/删除、扩缩容、重启、回滚、Pod 替换、ConfigMap/Secret 挂载更新、HTTP 路由、过期版本拒绝。
- 三项持续原生采集，4 个仪表盘共 23 个面板均有有效数据，实际页面 canvas 尺寸和内容通过。
- 5 项工作台页面流程（中英、明暗、1280/1920 桌面、实际日志/终端），另 1 项仪表盘与当前告警页面流程；正常登录、严格 TLS、零重试、零 mock。
- kube-system 写预览实际返回禁止，平台接入身份不能写集群 RBAC；两种会话结束后关闭，端口转发有实际双向字节数。
- 正式环境健康 20 项通过，六个原有 PVC 绑定保留；原有 evaluation 网络隔离/外部 Egress 警告保留。

[最终核验](../../../artifacts/plan9/p9-b10a-20261111-01/checks.json) · [投递观察](../../../artifacts/plan9/p9-b10a-20261111-01/delivery-watch.json) · [资源清单](../../../artifacts/plan9/p9-b10a-20261111-01/resource-map.json) · [B09b 清理核验](../../../artifacts/plan9/p9-b09b-20261110-01/cleanup-verification.json) · [运行和清理流程](../runtime/B10a/runbook.md)。

当前只交付 B10a 查看，未自动进入 B10b，人工验收仍 pending。
