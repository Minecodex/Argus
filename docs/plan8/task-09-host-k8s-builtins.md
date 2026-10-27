# P8-T09：主机自身与 K8s 组件指标

状态：in_progress。前件：P8-T05、P8-T06、P8-T08、P8-T10、P8-T14。

审计后已补主机进程与 Linux/Windows 服务及三个控制面，当前 12 个固定指标定义；全目录 24 套受管默认模板。后续已在自有 kind 验证 Scheduler/Controller Manager/etcd 成功采样、认证拒绝/恢复及十八个默认查询，并修复安全组件 API Proxy 丢认证问题，见[44](./44-control-metrics-authentication.md)。[原不可达记录](./36-kubernetes-control-metric-availability.md)保留当时环境事实。Windows 实机仍暂缓；Host/Node Claim、Namespace 指标和正式部署验收未完成。

## 设计与接点

- [05-host-and-kubernetes.md](./05-host-and-kubernetes.md)

已有/拟扩展接点：`internal/telemetry`、`internal/otelcol/configbundle`、`internal/collectormanager`、`internal/resource`、`web/apps/enterprise/src/components/hosts`、`web/apps/enterprise/src/components/kubernetes`。新领域包和具体接口在 T01 固化；路径是定位入口，不代表本任务已实现。

## 实施清单

- [ ] 把主机固定自身指标、进程/系统服务与 Collector 自监控转成稳定内置实例开关；日志入口转到规则列表。
- [ ] 实现 P8-B01～B08 的能力发现、端点/证书/权限检查、启停、应用状态和真实样本观测。
- [ ] 区分 kube-state-metrics、Kubelet/cAdvisor、集群 API 和控制面 /metrics 的数据语义及模板前件。
- [ ] 在支持平台采集真实 CPU/内存/磁盘/网络/Collector 数据，保留资产快照与高频遥测分工。
- [ ] 内置能力关联默认模板；缺端点/权限/指标明确 unavailable，不自动改远端监听或植入演示数值。
- [ ] Host/Node 强身份绑定与 Claim 保证共存；窗口迁移、卸载与采集关闭的影响可预览。

## 完成标准

- [ ] 主机数值与实际系统观测对比；不同系统/容器可见范围注明，未观测字段不补 0。
- [ ] 逐组件验证启停→应用配置→样本→查询→图表；托管 etcd 等不可访问场景明确，不称监控成功。
- [ ] 相关存储、接口/生成客户端、权限、门户/工具、审计、配置和文档保持一致；单文件小于 2000 行。
- [ ] 保存测试范围、失败迭代、源码/版本与证据；只完成 mock 或单层测试不计真实验收。

## 验收关联

对应 [验收矩阵](./09-acceptance-matrix.md)：P8-A19、P8-A33、P8-A34、P8-A35、P8-A44。

## 实施记录

已开始将固定指标与日志规则分开：`k8s-node-container` 仅开启 kubelet 指标，不再附带整集群 Pod 文件日志。独立日志规则按 Namespace/工作负载 UID 生效；关闭全部日志后，Agent 不再需要日志 HostPath 或 UID 0。相关配置与节点部署边界测试见[回归](../../artifacts/plan8-log-runtime-regression-verified.log)。主机固定实例、K8s 组件逐项真实指标、默认仪表盘及正式门户仍待完成，不能将此切分作为整项验收通过。

四项基础自身指标现已接入固定采集实例、正式 API/门户及独立状态。Host 与 Collector 自监控原生启停、全关闭保留进程，以及 12 组 Host/Collector 默认查询通过；旧 Profile 在确认配置时物化，完整历史安装迁移仍是发布前件。详见[自身实例与证据](./23-fixed-own-metric-instances.md)。其余内置组件、进程/系统服务、Node/Host Claim 和正式全平台验收仍未完成。

真实 K8s API/Kubelet 的基础固定实例、独立启停、清理及 12 项原生默认查询已通过，见[实际样本与验证层级](./25-kubernetes-own-metric-samples.md)。其余组件及 Namespace 指标仍保持未完成，不能按 Cluster 样本授予 Namespace 全量数据。

后续已补 API Server 与 CoreDNS 原有指标端点、精确授权、实际权限故障/恢复和原生数据，总计 24 项 K8s 默认查询通过。抓取诊断与真实目标数据分开更新健康、新鲜度；其余控制面组件和 Namespace 指标仍未完成。实际范围仍见同一证据文档。
