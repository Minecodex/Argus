# B09a Kubernetes 监控与容器日志查看记录

运行 `p9-b09a-20261108-01`。B08 已完成正常卸载和归档，专属容器与卷已回收。本批独立单节点 Kind 集群保留运行，供人工查看。

## 查看入口

- [B09a Kubernetes Workloads](https://argus.dev/kubernetes/01a1b8f7-875a-721d-8084-3c8e6c80a5cc) 选择 `b09a-workloads` 命名空间。Deployment 中有 orders 和 driver，另可查看 StatefulSet、DaemonSet、Job、Service 和 PVC。切换资源类型到 Pod，点击「查看日志」，再打开日志会话。
- [Namespace 与业务工作负载仪表盘](https://argus.dev/dashboards/f294713d-7229-4879-8500-c7ee523215de?resource=7ba1d021-8dc1-5e3b-825c-544c0eca6424&resource_type=kubernetes_namespace) 可查看当前命名空间的副本、重启、CPU 和内存。
- [容器持久日志仪表盘](https://argus.dev/dashboards/b0751543-c0ea-4d6e-a0d4-3d2bf9a6fb10?resource=8e72c0ec-07d1-5cb5-8edb-464407b43fe2&resource_type=application) 显示实际 stdout/stderr。展开完整字段可检查来源、Pod UID 和应用归属。
- [副本不足告警历史](https://argus.dev/alerts?tab=history&policy_id=5336646f-2dca-5103-945a-37b261f8982d) 于 **2026-11-08 10:54:50** 触发，**10:55:20** 恢复。时间均为北京时间。
- [集群默认仪表盘](https://argus.dev/dashboards/1764d344-bb5a-5b16-9f09-13f68a452bc9?resource=01a1b8f7-875a-721d-8084-3c8e6c80a5cc&resource_type=kubernetes_cluster)
- [节点与容器默认仪表盘](https://argus.dev/dashboards/c17f3435-0dc8-544f-bef4-e5e71c3bc381?resource=01a1b8f7-875a-721d-8084-3c8e6c80a5cc&resource_type=kubernetes_cluster)
- [Collector 默认仪表盘](https://argus.dev/dashboards/258c6833-6dce-5240-bddf-81bbf7c6f2c2?resource=01a1b8f7-875a-721d-8084-3c8e6c80a5cc&resource_type=kubernetes_cluster)

## 已验证场景

真实请求驱动 orders 和 ledger 服务，产生 CPU 计算、文件读写及 stdout/stderr。orders 正常为两副本，ledger 使用实际绑定的 64 MiB PVC。另有节点 DaemonSet 和完成的 Job。

三项原生指标采集均有新鲜样本。五套仪表盘共 24 个面板通过，Namespace 范围另执行 5 个面板。四组真实浏览器检查覆盖中英、明暗、1280 与 1920 桌面视口，以及实时 Pod 日志和持久日志。

故障演练将 orders 减少到一副本，固定告警阈值为 1.5，恢复阈值为 2，每十秒评估，触发和恢复各需持续二十秒。最终已恢复两副本，本批活跃告警为 0。容器重启后核对了相同 Pod UID、新重启代次及上次容器日志。随后删除该 Pod，核对替换 Pod 的新 UID，并确认原 Deployment 日志规则继续采集，旧日志仍可检索。

回看完整告警和重启场景可选择 **2026-11-08 10:49–11:00**。低速请求驱动每五秒一轮，最多运行八小时。平台静态接入 Token 有效 24 小时。

## 本次修正与运行范围

接入时修复了直连 HTTPS 目标解析、凭据租约事务并发重试和候选 Agent 的 CA 挂载。相关 Go 包检查与独立 PostgreSQL 回归通过。部署更新落在 direct-executor，告警 Worker 在证书续期后重启。六个原有 PVC 绑定核对一致。详见[修正记录](../10-b09a-direct-collector-target.md)。

平台使用独立节点的内部 API 地址接入本批集群。跨集群原生 mTLS 通过本批专属的 `argus-system/plan9-b09a-telemetry` Service 传输，该入口及其 UID 已列入清理清单。规则、来源和告警的失败历史保留。

## 可重复执行与证据

- [运行、预算与清理](../runtime/B09a/runbook.md)
- [最终核验](../../../artifacts/plan9/p9-b09a-20261108-01/checks.json)
- [真实工作负载身份](../../../artifacts/plan9/p9-b09a-20261108-01/workload-identities.json)
- [容器 stdout/stderr](../../../artifacts/plan9/p9-b09a-20261108-01/container-log-verification.json)
- [实际副本告警](../../../artifacts/plan9/p9-b09a-20261108-01/alert-cycle.json)
- [重启与 Pod 替换](../../../artifacts/plan9/p9-b09a-20261108-01/pod-lifecycle.json)
- [浏览器结果](../../../artifacts/plan9/p9-b09a-20261108-01/browser-review.json)
- [隔离 PostgreSQL 租约回归](../../../artifacts/plan9/p9-b09a-20261108-01/lease-regression.log)
- [证书续期恢复](../../../artifacts/plan9/p9-b09a-20261108-01/worker-certificate-recovery.json)

## 2026-11-09 接入凭据恢复

原静态接入 Token 于北京时间 2026-11-09 10:21:34 到期，资源读取被 Kubernetes 拒绝，门户返回 `KUBERNETES_API_FORBIDDEN`。已为原 ServiceAccount 续发相同 24 小时有效期的 Token，并通过正常企业管理员登录调用原 Secret 的轮换接口，将版本从 1 更新到 2；Cluster、Credential、Secret、Namespace 身份及 RBAC 保持原范围。

恢复后，Argus API 中 Namespace 目录、Pod、Deployment、StatefulSet、DaemonSet、Job、Service、PVC 共八类查询均返回 200。真实部署页面另通过中英 × 明暗四项 Playwright 验证，检查八类资源 Tab、手动刷新及没有实时更新 watch 请求。日志与结果见 `artifacts/kubernetes-resource-recovery-20261109/` 的 `api-verification.json`、`rotation.json` 和 `browser.log`。

新 Token 到期时间为北京时间 **2026-11-10 16:42:47**，仍为本批临时凭据。当前接入身份没有 CRD 列表读取权限，类型目录为 32 项且 `complete=false`；这与已恢复的普通资源读取分别记录。后续按用户要求移除页面上的扩展资源发现提示，已随 `local-20261109-k8s-hint` 发布，四项真实页面回归确认不再展示。

## 2026-11-09 采集续证与仪表盘恢复

另确认遥测证书在北京时间 10:42 左右到期：安装模块生成 PKCS#8 ECDSA 私钥，Collector 续证模块却只接受 SEC1，导致网关发送被拒绝。已修正解析兼容、使用原 PKI 维护流程修复身份，并通过正常预览/确认把同一 Collector 更新至修正版容器镜像，配置收敛到 Revision 11。

三项开启的采集重新产生新鲜样本；集群、节点/容器、Collector 与工作负载四套仪表盘共 23 个面板都有实际数据。采集配置主视图简化为内容开关和数据状态，维护细节移入次级入口，见[采集配置与恢复记录](../../plans/frontend-redesign/kubernetes-collection-controls-20261109.md)。发布后中英明暗的 8 项真实页面检查与平台健康 20/20 通过，临时维护 Job 和一次性输入 Secret 均已清理。故障期间终止丢弃的样本未作回填声明。

## B09b 组件指标续验

用户于 2026-11-09 要求继续完成组件指标和发布验证。本轮沿用本集群及已有 Collector，保留 B09a 工作负载和原有入口，另完成五项组件、30 个默认面板、独立启停、故障和告警恢复及部署后浏览器验证。当前查看入口和版本以 [B09b 查看记录](./B09b-review.md)为准，本页以上历史时点保持原记录。
