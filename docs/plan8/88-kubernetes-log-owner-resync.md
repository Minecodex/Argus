# Kubernetes 日志的归属缓存恢复

日期：2026-10-22。运行组件仍为原生 OpenTelemetry Collector 0.159.0，没有 fork 上游插件。

R63 的 84 组检查全部通过，但补充轮次的 `TestLiveKubernetesLogsRolloutAndCheckpoint` 在元数据 API 断网恢复后，40 秒内未收到新 Pod 日志。原配置原断言重跑通过，因此没有把一次重跑当作故障已解决。

随后在未修改的上游 0.159.0 源码上复现事件顺序：先处理 Pod，再处理其 ReplicaSet，已缓存 Pod 的 Deployment UID 仍为空；调用正常 Pod 更新处理器后才补齐。该观察只在临时诊断模块执行，没有替换产品二进制或修改 Go module 缓存。对应原生 Collector 的日志规则会将缺少可信归属的批次退回重试，仍然不会错误关联应用。

上游提供 `watch_sync_period`，默认五分钟；它把本地缓存重新送入事件处理器，而不是定时全量拉取 Kubernetes API。默认重同步周期与本项目五分钟最大日志重试处于同一量级，缺少恢复余量。现在仅对 Kubernetes 日志规则显式配置十秒重同步，仍保留节点／Namespace 过滤、UID 校验、启动元数据等待、持久队列和五分钟重试上限。

候选配置沿用原生 Collector 和原测试的 40 秒接收断言，连续五次实际通过；覆盖滚动发布、元数据断网、首行连续性、接收端不可用和进程崩溃后的队列恢复。此候选结果不替代合入后新源码的正式验收。渲染回归同时要求重同步有明确时限、低于重试预算且仍限制于节点与 Namespace。

证据：

- 原失败：`artifacts/plan8-supplemental-20261022-r63/supplemental-live-workload-logs.jsonl`。
- 原配置复验：`artifacts/plan8-r63-k8s-log-reproduction.jsonl`，只说明该故障与事件顺序有关。
- 上游缓存顺序复现：`artifacts/plan8-upstream-k8s-owner-order.log`。
- 原断言连续五次候选实测：`artifacts/plan8-r63-k8s-log-resync-candidate-five.jsonl`。
- [上游 0.159.0 重同步选项](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.159.0/processor/k8sattributesprocessor/README.md#informer-cache-resync-period)。大型集群的缓存重算成本需按实际节点／Namespace／规则数量评估，本次没有宣称十万 Pod 容量已通过。
