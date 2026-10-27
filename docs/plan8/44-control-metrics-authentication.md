# 控制面指标认证与证书信任

2026-10-19，T09/T14 继续实施。真实 kind 集群已复现：即使 Scheduler 指标端口可达，经过 API Pod Proxy 的请求仍返回 `system:anonymous` 无权读取 `/metrics`。[API Server 的认证过滤器](https://github.com/kubernetes/apiserver/blob/master/pkg/endpoints/filters/authentication.go)会在认证成功后删除 Authorization 头，因此此前仅使用 API Proxy 的配置不能接入默认受保护的 Scheduler/Controller Manager。失败证据见[首次正向门禁](../../artifacts/plan8-control-positive-20261019-3972678327/manifest.json)，没有改成通过。

## 调整及影响

- Scheduler 和 Controller Manager 使用已有内置 prometheusreceiver，通过 Kubernetes SD 在 kube-system 发现固定组件，直接访问其 Pod IP 的 HTTPS 指标端口。继续使用 Collector 自身的 ServiceAccount；没有独立 exporter 或辅助采集服务。
- TLS 校验始终开启。默认信任集群 CA，证书身份默认为组件名；允许按固定实例配置额外公开 CA PEM 和 server_name。禁止任意端点、私钥、客户端密码、跳过验证或调用者指定 Token 文件。
- etcd 的原生 HTTP metrics 继续经有认证的 Kubernetes API Proxy 读取；不由平台修改客户组件的监听或认证配置。组件未暴露、网络受限或证书不受信任时展示实际失败。
- 主机和 K8s 固定自身能力仍直接启停。受保护组件增加可选的“证书信任”配置入口，保存到同一采集实例并走原有预览/确认；启停保留已配置的信任，不增加环境字段或平台凭据目录项。公开证书不是 SDK Key 或私钥。
- 服务器规范化、配置编译、门户和 Agent 都使用同一按类型解析规则；其他固定能力不能接受这些 TLS 字段。新增字段只影响相应实例的配置版本，固定实例 ID 不变。

## 真实验证要求

自有双节点测试集群使用受其 CA 签发的组件服务端证书，私钥在该节点内部生成和使用，继续启用原有认证/授权。不开匿名指标授权，不关闭证书校验，不修改既有业务集群。

原生组件测试要求三次成功抓取、实际 Scheduler 工作负载、撤销指标权限和 Pod Proxy 权限后各来源失败、恢复后真实样本及独立启停。相同新鲜捕获进入 Writer/ClickHouse，三个默认模板的十八个面板必须真实取数；无观测的分位数显示无数据，不补零。以下证据按这些要求执行，不扩大成正式支持矩阵或全量通过。

## 已执行证据

- [三个具名组同次冻结通过](../../artifacts/plan8-control-complete-20261019-2550694980/manifest.json)：bootstrap-rbac、control-positive、control-positive-views 全部 passed，source_unchanged=true，plan_complete/release_eligible 仍为 false。
- 原生 Collector 0.158.0 使用真实 ServiceAccount、校验通过的组件 HTTPS 证书和实际 Pod IP，三个组件均得到实际目标指标；受保护端点继续认证授权。撤权后每个来源 up=0，恢复后 up=1 和真实指标恢复，独立启停及进程存活通过。测试通过原生 Scheduler 放置工作负载，并使用实际 Controller/etcd 活动产生直方图样本。
- 同一新鲜捕获经 Ingest/Writer/ClickHouse，十八个默认面板查询通过。首次发现闲置 workqueue 的分位数 NaN 后，三个控制面分位数查询增加比较过滤：缺观测保持无数据，没有替换成 0；受管内容版本随内容变化更新，个人副本不改。
- 安装 shell 清单显式加入 `/metrics` 只读和 kube-system `pods/proxy`，不向 Collector 运行时发放集群 RBAC 管理权限。真实 Token/SelfSubjectAccessReview 验证允许指标、节点统计及系统代理；业务命名空间代理、pods/log、pods/exec、Secret 和写入均拒绝。原有“不在运行时收敛创建 ClusterRole”的测试继续通过。
- [四个门户场景](../../artifacts/plan8-component-trust-browser-fresh-version.log)验证中英/明暗、私钥拒绝、信任配置预览与确认、独立启停保留配置。修复确认后列表尚未刷新时立即启停可能携带旧版本的问题；对话框在当前版本刷新后关闭。

## 自有集群准备与恢复

[`plan8-kubernetes-local.ps1`](../../scripts/plan8-kubernetes-local.ps1) 的 PrepareControlMetrics/RestoreControlMetrics 仅修改本轮自有 Docker 节点，保存原清单和精确 Hash，复验 Docker ID，拒绝覆盖外部变化。测试证书在节点内部生成并由其 CA 签发，私钥不导出；保持组件认证和 TLS 校验，等到新配置的实际 Pod Ready 后运行。TestControlMetrics 复用这些前件；Cleanup 仍只删除所有权验证通过的集群。

本次正向测试为 Kubernetes 1.36.1 自有 kind 集群；原 `argus-monitoring` 控制面没有修改，之前的不可达结果仍按原时点保留。托管组件不开放或缺可信证书的实际目标仍应显示未观测。本记录不替代正式接入 Collector、全部平台/版本/拓扑或部署后模型验收。
