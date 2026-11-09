# B09a Kubernetes 监控与日志运行包

本批建立独立单节点 Kind 集群，运行两个业务服务、请求驱动、DaemonSet 和一次性 Job。请求实际产生 CPU 运算、文件读写和 stdout/stderr。平台通过专属 ServiceAccount 接入集群，正常安装原生 Collector Gateway 与 Agent，启用集群、节点/容器及 Collector 自身指标，并创建应用容器日志规则。

## 执行

从仓库根目录运行，正常企业登录会话保存在私有路径。

```powershell
$session = 'artifacts/plan9/reset-20261026/private/enterprise-session.json'
$ca = 'artifacts/plan9/reset-20261026/trust-bundle.pem'
$manage = 'docs/plan9/runtime/B09a/manage.py'
$integration = 'docs/plan9/runtime/B09a/integration.py'
python $manage prepare
python $manage cluster
python $manage build
python $manage workloads
python -c "import sys; sys.path.insert(0,'docs/plan9/runtime/B09a'); from manage import Run; from bootstrap import bootstrap; bootstrap(Run())"
python $integration enroll --session $session --ca $ca
python $integration install --session $session --ca $ca
python $integration builtins --session $session --ca $ca
python $integration logs --session $session --ca $ca
python $integration alerts --session $session --ca $ca
python $integration exercise --session $session --ca $ca
python $integration pod_lifecycle --session $session --ca $ca
python $integration dashboards --session $session --ca $ca
python docs/plan9/runtime/B09a/browser.py
```

Kind 使用官方 v0.32.0 工具及 Kubernetes v1.36.1 节点摘要。工具下载校验和记录在本批 artifacts。版本和摘要来自[官方发行说明](https://github.com/kubernetes-sigs/kind/releases/tag/v0.32.0)。

## 范围与上限

- Kind 节点上限 3 vCPU、3072 MiB。业务容器各请求 20m CPU/32 MiB，上限 200m CPU/96 MiB。
- `orders` Deployment 正常两副本，`ledger` StatefulSet 单副本配 64 MiB PVC，另有 `driver` Deployment、`node-agent` DaemonSet 和 `receipt` Job。
- 业务 Namespace 为 `b09a-workloads`。平台和来源按实际 Namespace UID、Deployment UID 关联。
- 请求驱动每五秒访问两项真实服务，每八轮包含一次受控 HTTP 503，最多运行八小时。文件每次覆盖少量内容。
- 通过减少 orders 的实际副本验证告警，阈值 1.5、恢复阈值 2，每十秒评估，触发和恢复分别持续二十秒。节点上的一次性定时器在 120 秒后恢复两副本，驱动也执行恢复。
- 容器重启保留同一 Pod UID，检查重启代次与上次容器日志。删除 Pod 后验证新 UID，持续日志规则跟随 Deployment。历史日志继续检索。
- 跨集群 mTLS 网络使用专属的 `argus-system/plan9-b09a-telemetry` Service，并核对所有权和 UID。共享 Kind 网络及平台依赖列为借用资源。

本次接入发现的修正与部署范围见[修正记录](../../10-b09a-direct-collector-target.md)，实际结果和查看入口见 [B09a](../../batches/B09a-review.md)。

## 临时接入凭据到期与恢复

本批 `plan9-access/argus-kubernetes-connector` 的静态接入 Token 有效期为 24 小时。2026-11-09 已确认原 Token 在北京时间 10:21:34 到期；同一集群的 Namespace、Pod、Deployment 等资源请求共用该凭据，因此均会收到认证拒绝。平台服务健康通过不代表该临时凭据仍有效。

恢复时为**同一个 ServiceAccount、同一套 RBAC**签发新的 24 小时 Token，验证原生读取后，通过正常企业管理员登录或现有授权 API Token 调用 Argus 的原 Secret 轮换接口。沿用原 Cluster、Credential 和 Secret ID，使用当前资源版本执行轮换，让既有审计、凭据版本及租约撤销链路生效。

不要只替换本地 kubeconfig 文件：已登记的值在 Argus 托管 Secret 中，必须正常轮换后才影响页面查询。重跑当前 bootstrap 也不会自动更新已有 Secret。企业自动化登录会话与上游 Kubernetes Token 分别有自己的有效期，轮换前需确认两者。

完成后应通过 Argus API 验证 Namespace 目录，以及 `b09a-workloads` 范围内的 Pod、Deployment、StatefulSet、DaemonSet、Job、Service 和 PVC。保留新旧版本号、到期时间及读取结果；Token、密码和 Cookie 仅存私有材料，不写入普通日志。

## 清理

```powershell
python $integration retire --session $session --ca $ca
```

先删除策略、停采和归档日志规则，再归档本批应用及自定义仪表盘。Collector 走正常卸载流程，集群通过预览确认退役，随后撤销凭据与授权，删除专属遥测网络 Service。最后检查 Kind 容器的原始 ID 和集群标签，再销毁本批集群。共享模板、网络和历史记录按既有保留策略处理。
