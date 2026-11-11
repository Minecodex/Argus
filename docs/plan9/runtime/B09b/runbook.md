# B09b Kubernetes 控制面组件运行包

当前运行 `p9-b09b-20261110-01`。本轮按用户要求先退役 B09a 及复用其集群的旧 B09b，再创建独立 Kind 集群。原平台的查询、模板和 Collector 续期修正继续用于新环境，历史证据保留。

五项组件为 API Server、CoreDNS、Scheduler、Controller Manager 和 etcd。每项都有独立采集开关、六个默认面板和一条抓取可用性告警。数据来自组件原生端点，经正式安装的 Collector、mTLS Ingest、Kafka、Writer 和正常仪表盘查询链路进入平台。

## 创建与接入

从仓库根目录执行，`--session` 指向通过正常企业登录取得的有效私有会话文件。

```powershell
$env:ARGUS_PLAN9_B09B_RUN = 'p9-b09b-20261110-01'
$session = 'artifacts/plan9/p9-b09b-20261109-01/private/enterprise-session.json'
python docs/plan9/runtime/B09b/deploy.py prepare
python docs/plan9/runtime/B09b/deploy.py cluster
python docs/plan9/runtime/B09b/deploy.py bootstrap
python docs/plan9/runtime/B09b/deploy.py foundation --session $session
python docs/plan9/runtime/B09b/run.py inspect
python docs/plan9/runtime/B09b/run.py prepare
python docs/plan9/runtime/B09b/run.py permissions
python docs/plan9/runtime/B09b/run.py activity
python docs/plan9/runtime/B09b/run.py enable
python docs/plan9/runtime/B09b/run.py verify
python docs/plan9/runtime/B09b/run.py alerts
```

新的集群、Namespace、Collector、凭据和运行记录均属于本轮。集群创建要求 B09a 的正常卸载与零残留核验完成。基础设施复用共享的 Kind、原生权限准备和正式接入逻辑，单节点上限为 3 vCPU、3072 MiB。

## 端点与权限

控制面监听及服务端证书准备只修改 Docker ID 和 Kind 标签均匹配的独立节点。每个清单保存原文及 SHA256，恢复时拒绝覆盖外部改动。

Scheduler 与 Controller Manager 保持 ServiceAccount 认证授权，使用本批集群 CA 签发的服务端证书直接 HTTPS 抓取。etcd 使用专用 metrics 端口，经授权的 kube-system Pod Proxy 读取。私钥保留在目标节点，Collector 的只读角色不包含 Secret、exec 或 RBAC 写权限。权限检查保存到本轮证据。

本轮未登记节点主机，Host/Node Claim 和日志责任去重保持单列验收边界。

## 实际活动、故障与页面复验

```powershell
python docs/plan9/runtime/B09b/run.py exercise
python docs/plan9/runtime/B09b/run.py availability
python docs/plan9/runtime/B09b/browser.py
python docs/plan9/runtime/B09b/run.py final
```

`activity` 在 `b09b-components` 中每十秒执行少量真实 DNS/API 请求，每三轮改变一个 Deployment 的 1/2 副本。绝对截止时间保存在 `activity-deadline.json`，八小时后恢复一副本并空闲。容器重启沿用同一截止时间。

五条告警使用各自采集的原生 `up`，阈值 0.5，恢复阈值 1，每十秒评估，触发和恢复各持续二十秒。资源授权明确限定本批集群。

`exercise` 验证错误证书名称、独立停采和重新开启。配置变化将旧事件标为 `source_changed`。随后 `availability` 在不改采集配置的情况下，精确拒绝当前 Gateway Pod 到 Scheduler 10259 的流量，核对同一条件告警实际触发与恢复，并确认其余四个组件继续采集。故障设有 120 秒自动恢复，驱动也在 finally 中核对清除本轮规则。

浏览器连接正式门户，验证中英、明暗、1280/1920 桌面、五项组件、30 个面板，以及通过页面独立停用/恢复 CoreDNS。运行参数显式选择本轮集群 ID。

## 清理

```powershell
$env:ARGUS_PLAN9_B09B_RUN = 'p9-b09b-20261110-01'
python docs/plan9/runtime/B09b/run.py retire
```

按正常预览确认删除本批策略、停用八项采集并卸载 Collector，随后退役集群、撤销凭据与数据授权。专属遥测 Service 按 UID 与运行标签核验后删除，再核对 Kind 容器原始 ID 和标签，销毁本批节点。共享模板和平台业务历史按既有策略保留。

旧运行 `p9-b09b-20261109-01` 的复用模式与原始结果保留在历史验收记录中。本轮实际入口见 [B09b 查看记录](../../batches/B09b-review.md)。
