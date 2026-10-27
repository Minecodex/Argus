# Kubernetes 自身指标的原生样本与默认模板

状态：集群状态、Kubelet、API Server、CoreDNS 实际组件采集及 24 项默认查询通过；正式接入、Namespace 指标授权、Node/Host Claim 和全部 K8s 组件尚未完成。

固定实例 `kubernetes_cluster_metrics` 使用原生 `k8s_cluster`，生产配置放在单个 Gateway；`kubernetes_node_metrics` 使用原生 `kubeletstats`，放在每个节点 Agent。它们分别表达期望、配置、生效、健康及新鲜度，不附带文件日志，也不需要应用或其他执行主机。

本次在 `argus-monitoring` 创建独立 Namespace、Deployment、短期 ServiceAccount Token 和拥有标签的只读集群 RBAC。Collector 保持 API/Kubelet 的 ServiceAccount 身份与实际证书验证，不使用 `insecure_skip_verify`。组件门禁在一个测试进程中合并两个放置位置并连接认证测试接收端；生产放置方式没有改变。这不证明生产 Gateway/Agent 之间的传输、正式登记和最终门户已贯通。

实际得到 Deployment/DaemonSet/Pod/节点状态、容器请求及重启，以及节点、Pod、容器 CPU/内存/文件系统/网络指标。关闭集群状态后节点采集继续；全部关闭后无隐式样本，控制运行仍在。测试等待 Namespace 删除，并以 UID 清理新增 RBAC；不删除原有工作负载。

## 默认模板

| 模板 | 原生来源 | 面板 | Grafana 布局参考 |
| --- | --- | --- | --- |
| kubernetes.cluster | k8s_cluster | Deployment 可用/期望、DaemonSet 就绪、节点就绪、容器重启/CPU 请求 | 16364 |
| kubernetes.nodes | kubeletstats | 节点 CPU/内存/文件系统、Pod 内存、容器 CPU、节点网络速率 | 21788 |
| kubernetes.apiserver | kubernetes_apiserver | 抓取状态、请求速率、平均耗时/P95、处理中请求、进程内存 | 12654 |
| kubernetes.coredns | kubernetes_coredns | 抓取状态、请求/响应速率、P95、缓存命中率、进程内存 | 15762 |

查询使用本次原生数据的实际名称与单位，不依赖 kube-state-metrics、外部 exporter 或 recording rules。每个面板经实际 Writer 写入 ClickHouse 后执行查询，并验证错误来源返回空。默认模板继续遵循系统自动升级、个人副本独立、查询版本冻结和当前资源授权。Cluster 自身实例表提供按当前 Cluster 进入默认模板的入口。

模板客户端生成物已拆成每个定义一个文件，索引统一导出；避免随着模板增多突破单文件 2000 行限制。权威定义仍在 Go 领域层，业务门户不维护平行模板。

- [原生采集、独立启停与清理](../../artifacts/plan8-kubernetes-own-native-metrics-cleanup.log)
- [实际原生样本](../../artifacts/plan8-kubernetes-own-native-metrics.json)
- [12 项 Writer/ClickHouse 查询](../../artifacts/plan8-kubernetes-native-template-queries.log)

此前两次组件数据成功但清理超时的失败记录保留；独立测试 Pod 改为短终止宽限期后，包含清理的整项重新通过。没有强删 Namespace finalizer 或放宽通过条件。

## API Server 与 CoreDNS

两项属于 Kubernetes 自身已暴露的组件指标，使用内置 `prometheusreceiver`，没有安装 exporter，也没有把这种抓取路线作为数据库或中间件专用插件的替代。

API Server 固定读取 `https://kubernetes.default.svc/metrics`，使用 Collector 的 ServiceAccount、实际 CA 和明确 TLS 服务名。`get /metrics` 必须由接入准备显式授予。真实撤销这一权限后原生 `up` 变为 0，恢复授权后回到 1；没有跳过鉴权或改监听。默认面板使用实际请求直方图计算平均值/P95。

CoreDNS 限定标准 `kube-system`、`k8s-app=kube-dns`、Running Pod 的 9153 端口，经 Kubernetes API Pod 代理读取其原有 `/metrics`。需要该 Namespace 的只读 `pods/proxy` 授权；测试通过 Namespace RoleBinding 限定范围，没有赋予整个集群的 Pod 代理权限。没有端点或端点不可读不能宣称采集成功。当前实际版本为 CoreDNS v1.14.6、Kubernetes v1.37.0；其他标签、Namespace、端口或组件版本仍需逐组合核验。

- [四类实际原生数据、权限撤销/恢复、逐项关闭及清理](../../artifacts/plan8-kubernetes-core-components-native-metrics.log)
- [实际四类样本](../../artifacts/plan8-kubernetes-core-components-native-metrics.json)
- [24 项 Writer/ClickHouse 查询](../../artifacts/plan8-kubernetes-core-components-template-queries.log)，包含两个真实直方图 P95 和缓存命中率。

新增 API Server 测试最初沿用数据库夹具的 100 条序列预算，无法容纳组件正常维度。联合查询现明确使用生产 Dashboard 原有的有界预算（序列、样本、扫描字节与超时），没有改变生产上限；初始失败 `plan8-kubernetes-api-template-queries.log` 保留。模板没有用 0 填补缺失指标。

## 采集健康与实际数据

抓取诊断 `up=0`、`scrape_*` 仍写入真实遥测，但不能据此刷新目标指标的 `last_sample_at` 或显示正常。领域观测区分失败、部分失败和目标数据；实际数据保留原采样时间，旧成功记录不能覆盖较新失败。恢复时清除抓取故障，配置应用失败仍保留自己的错误码。空 Resource 或只有 stale marker 不制造采集健康；该分类只作用于受信的组件来源，其他类型中同名业务指标不改变语义。

- [七类观测单测](../../artifacts/plan8-component-scrape-health-unit.log)
- [真实 PostgreSQL 状态、新鲜度、乱序与恢复](../../artifacts/plan8-component-scrape-health-postgres.log)

Namespace 指标仍不能借此读取整个 Cluster。原生多数工作负载指标只有 Namespace 名称，需进一步建立可信 UID 归属才能开放 Namespace 指标查询；目前保持拒绝/空范围。Scheduler、Controller Manager、etcd 仍需各自端点/证书/权限、真实指标和模板门禁。
