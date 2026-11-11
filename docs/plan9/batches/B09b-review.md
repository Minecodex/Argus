# B09b Kubernetes 控制面组件查看记录

本轮 `p9-b09b-20261110-01` 已于 2026-11-11 退役。用户要求进入下一波，未据此代记明确人工通过；独立 Kind、Collector、策略、凭据授权和专属入口已清理，正式六个 PVC 与平台历史保留。[清理核验](../../../artifacts/plan9/p9-b09b-20261110-01/cleanup-verification.json)，当前查看 [B10a](./B10a-review.md)。

以下为退役前验证归档，旧集群页面已不可作为当前入口。2026-11-10 按用户要求完成 B09a 和旧复用 B09b 的退役，再创建全新的独立 Kind 集群。旧应用、自定义仪表盘已归档，Collector 已卸载，旧节点和专属网络入口已回收。当前集群名称为 **B09b Kubernetes Control Plane**。

投递丢失和未触发 no_data 混入当前列表两项反馈已完成修复、重新发布与真实复验。8 项采集、30 个面板、5 项真实页面流程、326 秒投递观察及健康 20/20 通过；[修复与版本记录](../14-b09b-delivery-and-current-alerts-20261110.md)。

## 退役前查看入口

- [组件采集配置](https://argus.dev/kubernetes/01a1c56d-27fb-7e35-91d7-2c2604782627#otlp-collector) 中五项组件均为 healthy/fresh。每项右侧「默认仪表盘」可打开对应六个面板。
- [五套默认仪表盘](https://argus.dev/kubernetes/01a1c56d-27fb-7e35-91d7-2c2604782627?tab=dashboards) 已绑定本轮新集群。
- [Scheduler 条件告警历史](https://argus.dev/alerts?tab=history&policy_id=5558e03c-ec12-5133-a468-f421b00b87c2) 最新复验在北京时间 **2026-11-10 23:31:00** 触发，**23:32:00** 恢复。

| 组件 | 当前仪表盘 | 已配置策略 |
| --- | --- | --- |
| API Server | [六个面板](https://argus.dev/dashboards/ba89e0b8-0204-5461-84a9-1931aed51b58?resource=01a1c56d-27fb-7e35-91d7-2c2604782627&resource_type=kubernetes_cluster) | B09b API Server metrics scrape unavailable |
| CoreDNS | [六个面板](https://argus.dev/dashboards/0c88b025-2a4d-5b9d-97e3-408569077f40?resource=01a1c56d-27fb-7e35-91d7-2c2604782627&resource_type=kubernetes_cluster) | B09b CoreDNS metrics scrape unavailable |
| Scheduler | [六个面板](https://argus.dev/dashboards/59eba15f-bcdc-5ef0-86aa-59145838262e?resource=01a1c56d-27fb-7e35-91d7-2c2604782627&resource_type=kubernetes_cluster) | B09b Scheduler metrics scrape unavailable |
| Controller Manager | [六个面板](https://argus.dev/dashboards/df0a6a46-cbdc-54d9-a4d0-41ac509368db?resource=01a1c56d-27fb-7e35-91d7-2c2604782627&resource_type=kubernetes_cluster) | B09b Controller Manager metrics scrape unavailable |
| etcd | [六个面板](https://argus.dev/dashboards/cda16bcf-f1fa-560b-a29d-ad2208a21b27?resource=01a1c56d-27fb-7e35-91d7-2c2604782627&resource_type=kubernetes_cluster) | B09b etcd metrics scrape unavailable |

## 本轮自检

五项组件各取得连续推进的真实目标样本，30 个面板均有有限数值。另有集群、节点/容器和 Collector 自身指标，共八项采集。五条抓取可用性策略保留集群资源标签，资源授权限定本轮集群，触发阈值 0.5、恢复阈值 1，每十秒评估，触发和恢复各持续二十秒。

真实证书名称错误、独立停采和恢复均已执行。配置变化的事件记录为 source_changed。随后对 Scheduler 抓取施加精确、有界网络故障，验证同一条件告警触发和恢复，其他四个组件持续采集，临时规则已全部清除。

五项真实浏览器流程覆盖中英、明暗、1280/1920 桌面、五套默认仪表盘及通过页面停采和恢复 CoreDNS。规则资源分组补充后，另执行一次相同页面停采/恢复流程。当前五条业务策略的评估均为 normal，组件条件告警为 0。

低速 DNS/API 请求和 1/2 副本变化继续运行，绝对截止时间沿用本轮八小时预算。平台部署、查询/模板及身份续期版本核对一致，六个原有 PVC 绑定一致。

## 修复结果与历史记录

- **B09b-D02：已修复，待人工查看。** 修正成功回执事务重试、正常停采的生效版本切换、持久队列 Shutdown 分类及关停最后一次采样。最终实际停采、出口中断和重启后，台账三类失败增量均为 0，日志也没有终止丢弃。故障复现把累计已观察失败从 17 增至 918，原日志另含周期观察之外的拒绝事实；历史空档没有伪造补齐，当前保留一条历史缺口事件。最初 [17 点观察](../../../artifacts/plan9/p9-b09b-20261110-01/collector-delivery-observation.json) 和 [最终核验](../../../artifacts/b09b-repairs-20261110/final-review.json) 分别保留。
- **B09b-A01：已修复，待人工查看。** 原来 4 条开始时间为空的 `normal/no_data` 内部状态已退出当前列表。来源新增、移除与安装代次变化均会退役旧分组；列表、分页和概览口径一致，真正触发的健康告警与历史保持。原 [告警状态证据](../../../artifacts/plan9/p9-b09b-20261110-01/alert-observations.json) 保留，真实中英明暗页面均通过。

## 执行与证据

- [独立部署、预算和清理流程](../runtime/B09b/runbook.md)
- [修复后最终核验](../../../artifacts/b09b-repairs-20261110/final-review.json) · [原部署核验](../../../artifacts/plan9/p9-b09b-20261110-01/checks.json)
- [连续真实样本](../../../artifacts/plan9/p9-b09b-20261110-01/continuous-samples.json) · [30 个面板](../../../artifacts/plan9/p9-b09b-20261110-01/dashboard-verification.json) · [五条告警](../../../artifacts/plan9/p9-b09b-20261110-01/alert-policies.json)
- [真实稳定来源故障与恢复](../../../artifacts/plan9/p9-b09b-20261110-01/stable-source-fault.json) · [告警时间](../../../artifacts/plan9/p9-b09b-20261110-01/alert-cycle.json)
- [五项真实浏览器流程](../../../artifacts/plan9/p9-b09b-20261110-01/browser-review.json) · [补充规则分组复验](../../../artifacts/plan9/p9-b09b-20261110-01/alert-scope-browser.json)
- [B09a 清理核验](../../../artifacts/plan9/p9-b09a-20261108-01/cleanup-verification.json) · [旧 B09b 退役](../../../artifacts/plan9/p9-b09b-20261109-01/transition-cleanup.json)

## 2026-11-09 历史运行（已退役）

运行 `p9-b09b-20261109-01`，沿用 B09a 的独立 Kind 集群和已登记资源身份。五项组件已开启，正式采集、30 个面板、5 项真实浏览器流程与故障恢复均通过，环境保留等待用户查看。

## 查看入口

- [采集配置](https://argus.dev/kubernetes/01a1b8f7-875a-721d-8084-3c8e6c80a5cc#otlp-collector)：五项组件与原有 Collector、集群、节点/容器指标均可独立启停。每项显示数据状态、最近样本和默认仪表盘。
- [集群仪表盘列表](https://argus.dev/kubernetes/01a1b8f7-875a-721d-8084-3c8e6c80a5cc?tab=dashboards)：五套组件默认仪表盘已绑定当前集群，保留原有仪表盘。
- [组件抓取告警历史](https://argus.dev/alerts?tab=history&policy_id=b223edf0-c2a0-55f1-aa9d-cf50f1d52909)：实际拒绝 Scheduler 抓取、解除后样本与同一条件告警恢复。当前故障规则已清理。

最近一轮条件告警于北京时间 **2026-11-09 23:19:10** 触发，**23:19:40** 恢复；相同 Collector 配置版本下完成，当前无本轮活动故障。

| 组件 | 默认仪表盘 | 已核对内容 |
| --- | --- | --- |
| API Server | [查看](https://argus.dev/dashboards/ba89e0b8-0204-5461-84a9-1931aed51b58?resource=01a1b8f7-875a-721d-8084-3c8e6c80a5cc&resource_type=kubernetes_cluster) | 抓取状态、请求速率、平均延迟、P95、处理中请求、进程内存 |
| CoreDNS | [查看](https://argus.dev/dashboards/0c88b025-2a4d-5b9d-97e3-408569077f40?resource=01a1b8f7-875a-721d-8084-3c8e6c80a5cc&resource_type=kubernetes_cluster) | 抓取状态、请求/响应、P95、缓存命中率、进程内存 |
| Scheduler | [查看](https://argus.dev/dashboards/59eba15f-bcdc-5ef0-86aa-59145838262e?resource=01a1b8f7-875a-721d-8084-3c8e6c80a5cc&resource_type=kubernetes_cluster) | 抓取状态、调度速率、P95、待调度 Pod、内存、协程 |
| Controller Manager | [查看](https://argus.dev/dashboards/df0a6a46-cbdc-54d9-a4d0-41ac509368db?resource=01a1b8f7-875a-721d-8084-3c8e6c80a5cc&resource_type=kubernetes_cluster) | 抓取状态、队列深度/入队速率/排队 P95、内存、协程 |
| etcd | [查看](https://argus.dev/dashboards/cda16bcf-f1fa-560b-a29d-ad2208a21b27?resource=01a1b8f7-875a-721d-8084-3c8e6c80a5cc&resource_type=kubernetes_cluster) | 抓取状态、Leader、Leader 变化、失败提案、WAL P95、数据库大小 |

五套共 30 个面板通过实际查询，每个面板含有限数值样本。DNS/API 请求与 1/2 副本变化来自 `b09b-components` 中的低速真实活动，截止时间使用绝对时间并在容器重启后保持不变。八小时后恢复一副本并空闲，人工查看前保留环境。

## 本轮修正与边界

修复 API Server 直方图因读取无关历史样本耗尽整板预算的问题，新增完整来源/时间投影和已有数据物化；保留原有预算、授权及原始样本。初始化仅补齐缺失部分，断线后接续现有任务，不重复物化已完成投影。页面显示简短抓取失败说明，停采不显示旧失败；配置应用期间缩短状态核对间隔。

错误证书配置与独立停采验证正常，其他组件保持采集。配置变化关闭的告警明确记录为 `source_changed`；另在 Collector 配置版本不变时，对网关到 Scheduler 的流量施加带 120 秒自动恢复的精确故障，证明同一条件告警实际触发、恢复。实际样本失败期间未冒充新鲜数据，临时规则已删除。

本轮仅修改独立测试节点的组件端点与证书准备；正式 Docker Desktop 控制面保留。Collector 的 metrics/system Pod Proxy 读取可用，业务 Namespace Proxy、Secret、exec 和 RBAC 写权限拒绝。本轮未登记节点主机，因此不宣称 Host/Node Claim 与日志责任去重已经实测。

## 发布与证据

- Web：`local-20261109-k8s-components-v2@sha256:1206939db57edbf246ed3b6a60d44bfc2ad35309ff8b4942afb7f5d6c2da58e0`。
- Query：`local-20261109-k8s-components-query-v3@sha256:c915fedf62e960f66ea175b30619d54d387e4c0aa89dcf0ec42ad68c8a1868c1`。
- Query 使用当前线上 Schema 52 源码基线加本轮查询/投影修正；其余服务镜像和副本保留，没有引入 PostgreSQL 迁移。节点 containerd 的精确摘要、实际 Pod imageID 和 Rollout 均核对。
- Go 全量、隔离 ClickHouse 的 Telemetry/QueryEngine 全量、类型与 ESLint 通过；8 项中英明暗 Mock 交互与 4 项运行包单测通过。隔离 ClickHouse 回归容器及其匿名卷已回收。
- 发布后 5 项真实浏览器流程无重试、无跳过通过：中英明暗、1280/1920 桌面、五项采集及五套默认仪表盘、集群绑定入口，以及实际 CoreDNS 停采后样本不再推进、其他七项继续、重新开启恢复。严格 HTTPS 和正常企业登录，未使用 Mock。
- 最终五项组件均为 healthy/fresh，30 个面板再次得到实际有限数值。稳定来源故障与同一告警恢复的完整命令复验通过，临时网络规则全部删除。平台健康检查 20/20，6 个原有 PVC/PV 绑定一致，除 Web/Query 外的部署镜像、UID 和代次保持一致。

证据目录 `artifacts/plan9/p9-b09b-20261109-01/`：`checks.json`、`continuous-samples.json`、`dashboard-verification.json`、`failure-and-stop.json`、`configuration-alert-closure.json`、`stable-source-fault.json`、`alert-cycle.json`、`runtime-authorization.json`、`release.json`、`platform-before-release.json`、`platform-after-release.json`、`query-regression-complete.log`、`histogram-projection-idempotent.log`、`go-full.log`、`query-regression-cleanup.json`、`browser-review.json`、`real-browser-final.log`、`health-final.log`。实际截图位于 `real-browser-final/`，首轮失败日志保留。

运行方式和回滚边界见[运行包](../runtime/B09b/runbook.md)，设计修正见[组件指标与直方图查询](../11-b09b-component-metrics.md)。B09a 原有资源和查看入口继续保留。

## 2026-11-10 一小时窗口反馈修正

用户发现完整一小时下节点网络、API Server 后部面板耗尽预算。已优化来源/指标/时间读取和直方图字段、标签展开，并把 API 普通请求延迟与 WATCH 等长连接持续时间区分。节点/API Server 的 12 个面板在当前及反馈历史窗口全部完成，4 项真实中英明暗 E2E、最新样本和健康 20/20 通过。API Server 受管模板现为版本 2、延迟单位 ms；个人副本不变。

当前 Web、Server、Query 版本及完整对照以[一小时查询修正记录](../12-kubernetes-hour-window-query-20261110.md)为准，上节保留 11 月 9 日原交付时点的记录。
