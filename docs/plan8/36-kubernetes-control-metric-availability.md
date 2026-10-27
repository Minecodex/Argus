# Kubernetes 控制面指标与不可用状态

2026-10-19，补充 Scheduler、Controller Manager、etcd 组件的固定采集实例、启停、API/门户入口和默认模板。当前固定指标定义共 12 个，受管默认模板共 15 套；这不表示所有组件已有真实成功取数证明。

## 实现与限制

三个实例沿用 Kubernetes Gateway，通过现有 Kubernetes API 的 kube-system Pod 发现及受权 pods/proxy 读取既有指标端点。Scheduler/Controller Manager 使用已有安全端口；etcd 仅使用现有组件指标端口，不代用客户端/成员私钥。不会安装 exporter、创建远端监听、修改防火墙或调整控制面配置。

API 侧保持 CA、服务器名称和 ServiceAccount 验证。Pod 代理到后端的行为由 Kubernetes 自身实现负责，不将 API 侧 TLS 校验扩大成另行验证了组件自身证书。生产环境必须具备相应端点及明确的 kube-system 代理读取许可；托管、未安装、仅回环监听或没有权限时保持不可用。

| 固定实例 | 原有端点 | 默认模板参考 |
| --- | --- | --- |
| kubernetes_scheduler_metrics | kube-scheduler 的 10259 /metrics | [24353 Scheduler](https://grafana.com/grafana/dashboards/24353-kubernetes-system-kube-scheduler/) |
| kubernetes_controller_metrics | kube-controller-manager 的 10257 /metrics | [24352 Controller](https://grafana.com/grafana/dashboards/24352-kubernetes-system-kube-controller/) |
| kubernetes_etcd_metrics | etcd 的既有 2381 /metrics | [22236 etcd](https://grafana.com/grafana/dashboards/22236-etcd/) |

查询按[实际 Kubernetes 指标定义](https://kubernetes.io/docs/reference/instrumentation/metrics/)编写，Grafana 仅用于布局和指标参考。未观测到的计数器、Histogram 或状态不补造；当前环境没有相应真实成功取数时，不登记模板数据门禁通过。

## 不把诊断当目标数据

抓取型固定能力现在统一从 builtin 目录判定，而非仅硬编码 API Server/CoreDNS。Scheduler、Controller Manager、etcd 和 Collector 自身的 `up`/`scrape_*` 也按同一规则处理：失败只更新诊断，不产生目标样本时间，不把 `up=0` 当健康/新鲜数据。普通 SDK 或其他资源中名为 up 的业务指标仍按业务数据处理。

## 真实集群验证

`artifacts/plan8-control-components-native-verified.log`：实际 argus-monitoring 的三个控制面 Pod 均存在，但当前三个指标端点无法从网关代理路径访问。原生 Collector 的 up=0 与使用同一 ServiceAccount 的真实 API 探测一致，仅收到诊断，没有伪造组件指标。

同一测试撤销了仅属于本轮的 pods/proxy 授权，实际读取变为 Forbidden；随后恢复本轮许可，并验证三条来源独立关闭、全部关闭后 Collector 保持运行。只创建/回收自有 Namespace、测试工作负载及带 UID/owner 的临时授权；没有修改既有控制面 Pod 或监听设置。

配置测试覆盖 API TLS 不跳过验证、ServiceAccount、精确 kube-system 发现、Gateway 责任以及全部关闭；健康测试覆盖三个新增来源及 Collector 自身的失败诊断。证据文件 `artifacts/plan8-control-components-native-metrics.json` 保留实际诊断和可达性结果。

这证明实际不可达/拒绝与启停路径，**不是三个组件成功取数或 18 个模板查询通过**。仍需在既有端点可达且有权限的支持组合中验证真实指标、版本、Histogram 和完整默认模板；不以当前环境缺口替换支持矩阵结论。Host/Node Claim、Namespace 指标及正式安装部署路径也继续未完成。
