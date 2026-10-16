# 资源列表与主机资产验收（2026-10-13）

列表、资产采集、只读刷新、固定目标隧道恢复和真实集群状态投影已实现并部署。完整正式监控闭环尚未完成，不能以本页的源码、Docker 本地采集或 Minikube 工作负载测试替代它。

## 已验证

- 主机、待接入堡垒机、Kubernetes、仪表盘目录、资源关联仪表盘改为共享列表；默认列表约定写入 AGENTS.md 和设计入口。
- 主机资产快照通过本机 Connector 心跳、PostgreSQL、OpenAPI、企业门户和 host.get 工具贯通。刷新使用只读类型化队列命令；缺失字段明确展示。删除页面伪随机概览和创建时间推算的 uptime。
- 当前 Docker 主机以实际 argus-connector 用户调用同一采集器，与 /proc、hostname、uname 对照：16 个逻辑核、16292970496 bytes 共享内核内存、4 个文件系统、2 个网络接口；硬件 UUID 权限不足明确报告，云实例信息未虚构。
- Kubernetes 版本/节点统计使用实际 Connector Discovery/Nodes 心跳，记录 statistics_observed_at；尚未观测时不显示初始化的 0。DaemonSet 副本来自真实资源状态；Direct 与 Connector 查询共用健康/容量白名单投影，删除固定 60% 进度和把 platform 当分发版本的展示。
- 隧道恢复只验证既有 SSH 目标、账号、固定 Host Key 和平台回连，经 Preview/确认重验当前 Host、凭据与测试版本，重建控制及遥测隧道；保留 Host ID 和关联，不绕过版本校验。
- Docker 驱动的 Minikube `argus-monitoring` 为 v1.37.0，1/1 Node Ready。实际临时 Pod 执行、日志与 API 测试通过，测试 Namespace 已删除；原 kubectl 默认 context 仍为 docker-desktop。
- Go 8 个相关包检查通过；API client 104、企业门户 191、共享 UI 84 项单元通过；全部工作区类型、OpenAPI/protobuf、国际化、样式及受影响文件 ESLint 通过。
- 桌面列表中英×明暗×三尺寸，加主机卸载、接入失败，共 38 个 Playwright 场景通过。先前的登录定位和旧卡片选择器失败已保留日志并在最终轮次修复通过。
- Helm revision 14：backend `resource-lists-20261013-r4`、web `resource-lists-20261013-r7`，均固定 digest。5 个核心 Deployment 就绪，UID、副本和既有模型 DNS 保留；4 个 HTTPS 入口通过原 CA 的 Host/SNI 验证。迁移 12/13 已应用，无业务数据清空。两个临时 Namespace 均已清理。

## 正式闭环待完成

1. 现有 Docker 主机离线：2026-10-12 14:32 UTC 的 credential.update 将绑定凭据从版本 1 更新到 2，两个隧道仍冻结版本 1，后台拒绝重连。本轮已补恢复入口，尚未取得有效企业登录进行验证、预览和确认，PostgreSQL 仍无该主机的新资产快照。
2. “演示：CPU 使用率超过 1%”是此前建立的真实低阈值策略，并非伪造样本。其停用与当前事件验收尚需登录后通过正常 API 完成；不改写历史会话/告警事实。
3. Minikube 本身已可用，但尚未通过企业 API 注册资源、安装 Connector/Collector、确认收敛和实际遥测，再验证正式列表、资产刷新与仪表盘。正式旧登录会话已失效，这是当前配置与验收的必要输入。

证据：[final-verification.json](../../../artifacts/resource-lists-20261012/final-verification.json)、[Docker 实值对照](../../../artifacts/resource-lists-20261012/docker-inventory-verification.json)、[Minikube 实际工作负载](../../../artifacts/resource-lists-20261012/minikube-verification.json)。最终测试日志及固定镜像引用均在同一产物目录。不得将 real_monitoring_loop_complete=false 标成全功能已验收。
