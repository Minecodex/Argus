# 正式环境 Linux 主机监控与 GLM 会话配置（2026-10-12）

> 2026-10-13 复查：下文是此前配置时的验收快照。当前绑定 SSH 凭据已更新至版本 2，冻结版本 1 的控制/遥测隧道拒绝重连，主机离线；演示低阈值策略仍待正式登录后停用。新列表、资产采集及安全恢复入口已部署，实际闭环缺口见[资源列表与主机资产验收](../plans/frontend-redesign/resource-lists-and-host-inventory-acceptance.md)，不能把历史在线与旧样本视为当前监控已恢复。

用户要求在现有环境持续保留主机、采集、仪表盘、实际告警及真实 GLM 分析，通知暂不配置。本次通过正常企业/平台 API、Preview 与确认完成配置，没有写 SQL 绕过领域命令。数据库查询仅用于只读核对。

## 已保留资源

| 内容 | 当前配置 |
| --- | --- |
| Docker 主机 | `argus-test-host`，Ubuntu 24.04.5 LTS，root SSH `127.0.0.1:2222`，`unless-stopped` |
| Argus 主机 | `01a13032-168b-7cfc-aed9-300599ca2b99`，online，Executor Tunnel，经正式 SSH/回连预检、安装与注册 |
| Collector | `01a13033-8206-7eec-b42e-3674fca60107`，converged，desired/effective revision 一致 |
| 采集配置 | host-basic、linux-journald、file-log、collector-self、otlp-receiver、jaeger-receiver、skywalking-receiver |
| 已观测系统指标 | 14 个实际指标族，CPU、内存、负载、文件系统/inode、网络连接/收发/丢包/错误；实际 journald/filelog 日志也已入库 |
| 主机仪表盘 | `argus-test-host · Linux 主机监控`，`59efd1c2-4abe-4487-8980-6a6b4699935f`，10 图，与该 Host 绑定 |
| 通用仪表盘 | `Linux 主机通用监控总览`，`c60a5552-93dd-4550-916a-f59f71457fe0`，6 图，支持授权主机资源筛选 |
| 常规告警策略 | `82bfc4df-1cc5-58c0-8d64-bf43c08da110`，CPU > 85%、内存 > 85%、文件系统 > 90%，本轮评估正常 |
| 演示告警策略 | `84d72453-6c6f-5da9-88bb-50087ca736e8`，CPU > 1% 持续 10 秒，已产生实际 firing 实例；低阈值明确标注演示用途 |
| 告警实例 | `d4553837-5d72-4d21-8499-1e27a9ed1032`；常规告警与演示告警均未配置渠道/路由，不发送推送 |
| 评估权限 | 专属 ServiceAccount 通过专属只读角色继承这台 Host 的资源授权，没有授予其他主机范围 |
| GLM | `glm-5.3`，`01a13043-7f0b-7e19-8cb6-363095300a20`，官方 Coding Plan endpoint，四项兼容性通过，真实 provider 用量已核对 |
| Workspace | 正式 OpenSandbox 后端、l01 已验收镜像及离线 agent_workspace profile；该企业并发 3、月度 86400 秒额度 |

Docker 容器共享 WSL Linux 内核。CPU、内存和负载为该内核视角，文件系统和网卡为容器可见范围，不代表独立物理机/VM 或容器 cgroup 专用资源计量。OTLP/Jaeger/SkyWalking 接收能力已开启；未创建业务应用或伪造链路数据。Prometheus 独立目标与 Kubernetes 专用采集不适用于这台普通 Linux 主机。

## 可直接查看

- [主机详情](https://argus.dev/hosts/01a13032-168b-7cfc-aed9-300599ca2b99)
- [主机仪表盘](https://argus.dev/dashboards/59efd1c2-4abe-4487-8980-6a6b4699935f?resource=01a13032-168b-7cfc-aed9-300599ca2b99)
- [通用仪表盘](https://argus.dev/dashboards/c60a5552-93dd-4550-916a-f59f71457fe0?resource=01a13032-168b-7cfc-aed9-300599ca2b99)
- [当前告警](https://argus.dev/alerts?tab=active)
- [仪表盘检索会话](https://argus.dev/?c=01a1304f-b35f-7bbb-a1db-4cdd9ec04767)
- [核心指标报告会话](https://argus.dev/?c=01a13057-6a4c-72b0-a497-dc862f633672)
- [主机告警诊断会话](https://argus.dev/?c=01a1305a-3c1f-7b45-9af4-ed05fa7cc153)

指标报告实际选中通用仪表盘，限定本 Host 最近 5 分钟，取数并执行真实 bash/Python 文件分析。独立下载核对 6 个数据文件的 bytes、SHA256、Host/Source 与报告值；3 个成功 Run 的真实 provider 用量、工具调用及 6 个门户场景均已核对。两份最终模型报告另存为会话 Workspace Markdown，便于继续查看。Query manifest 的 `analysis_status=not_analyzed` 是取数制品原字段，本次没有伪改为 analyzed；实际文件分析和报告以 bash、模型调用、封存文件与独立核对为证据。

## 本地 DNS 配置

原集群 DNS 将 `open.bigmodel.cn` 解析为 `198.18.*` Fake-IP，模型端点策略正确拒绝。本次在 argus-system 创建专属 `argus-model-dns` CoreDNS/Service/ConfigMap 与两条窄范围 NetworkPolicy，仅该域名经 AliDNS DNS-over-TLS 动态解析真实公网地址，其他查询仍转原集群 DNS；仅 Argus Server/Worker 改用此 resolver。保留 URL、Host/SNI、CA、公共端点校验，不固定模型服务器 IP，不改共享 CoreDNS 或 Judex。

该 resolver 为本地正式 GLM 接入所需常驻配置，不按临时验收清理。所有权与 UID 见 `artifacts/formal-host-onboarding-20261012/model-dns-ownership.json`，声明见 `model-dns-manifest.json`，两个原 Deployment 备份在该目录 private 下。后续部署应核对/保留该本地 DNS 设置。

## 保留的失败与未关闭产品问题

1. ServiceAccount 直接数据授权 POST 返回 409、`invalid subject type`，当前 `invalidateSubject` 未覆盖该分支；本次使用已支持的专属角色继承授权完成，保留原失败证据，未写表绕过。
2. 第一轮同时分析 16 图已完成实际取数和文件读取，但 Run 在上下文压缩阶段失败（`CONTEXT_COMPACTION_FAILED`）。原失败会话与 Run 保留；新独立核心 6 图/5 分钟分析和告警诊断成功。没有把新 Run 成功解释为长上下文缺陷已修复。

本次用户配置任务完成，这两个产品边界仍需专项修复/回归，历史 l01 完整 required 通过证据不覆盖此次新暴露分支。

完成证据：[final-setup-verification.json](../../artifacts/formal-host-onboarding-20261012/final-setup-verification.json)。截图、独立文件核对、两份报告和接续脚本保存在同目录；敏感 API 会话位于 private，仅当前用户/SYSTEM 可读，完成后清除临时 Cookie/CSRF 文件。既有 `test` 仪表盘与业务数据保留。
