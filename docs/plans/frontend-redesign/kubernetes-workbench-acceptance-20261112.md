# Kubernetes 工作台与大盘交付验收

最新状态：二次复核的五组剩余项已补齐，当前关闭依据见 [剩余项补齐验收](./kubernetes-workbench-closure-20261113.md)。本文保留其原阶段的实施与验收事实。

当前完整状态见 [第二次完成度复核](./kubernetes-workbench-completion-review-20261113.md)；已确认仍需补齐 C01～C05，以下历史验证和部署记录保留。

本文保留第一轮交付记录。首次复核后追加的缺口与样式修复现已完成，最终状态和当前镜像以 [补齐与样式复验](./kubernetes-workbench-repair-acceptance-20261112.md) 为准。B10b 批次未切换。
证据目录：artifacts/kubernetes-workbench-refactor/20261112-01/。

## 上轮实现汇总（完整状态以复核为准）

| 任务 | 交付 |
| --- | --- |
| KWR-01～05 | 完整资源能力矩阵；统一原生状态、动作能力、UID 引用、状态筛选、分页与有界快照 |
| KWR-06～08 | 类型化列表、URL 上下文、详情侧栏/完整页、控制字段表单、并发输入保留、异步原位置反馈 |
| KWR-09～13 | Pod 排障、Deployment 更新/回滚、CronJob 实际运行、PVC 扩容与数据保留 |
| KWR-14～17 | 其他工作负载、网络、配置、Secret 明确键删除、HPA/PDB、节点、存储只读、权限引用、Namespace 和 CRD/扩展对象 |
| KWR-18～20 | 全局质量门禁、实际发布、文档与用户查看路线 |
| KWR-21～25 | DashboardSpec v2、可复用嵌入组件、按集群选择、资源图型/编辑器、受管运维大盘 |
| KWR-26～27 | 授权/预算/可见刷新边界、真实页面、实际镜像、清理及保留核对 |

## 验证证据

| 验证 | 结果 |
| --- | --- |
| 桌面矩阵与既有完整操作链 | 57 个通过，覆盖 1280/1440/1920、中英、明暗、可访问性；e2e-complete-desktop.log |
| 显式 Secret 键删除、最终详情回归 | 通过；secret-intent-browser-final.log、final-detail-regression.log |
| 真实浏览器 | 10 条流程分组通过：大盘/节点下钻 4 条；真实操作 1 条；Namespace-only 1 条；镜像/PVC/撤权 3 条；真实发布/选板/恢复 1 条 |
| 最终镜像页面复验 | 4 个正常登录实页用例通过；shipped-real-browser.log |
| 隔离 PostgreSQL | 仪表盘、工作台、授权、绑定版本与普通绑定共存通过；final-domain-regression.log、httpapi-final-regression.log、namespace-runtime-regression.log |
| 真实 Kubernetes 领域流程 | 7 条主流程通过：HPA、日志重启/撤权、工作负载/回滚、Namespace UID、CSI、RBAC、NetworkPolicy；native-operations.log |
| 独立双节点维护 | PDB、驱逐、丢失响应、撤权、取消与恢复通过；drain-live.log |
| CRD 与不支持扩容 | 真实发现/Schema/同名重建、存在实例时拒绝 CRD 删除、PVC 缩容和不支持驱动拒绝且无写入；native-extended-resource-checks.log |
| 实际数据证明 | UI 扩缩容后 2/2、镜像 busybox:1.37、HTTP 响应、Job stdout、PVC 32Mi→64Mi 且原文件保留；actual-post-ui-proof.json |
| 刷新/冲突/范围 | Hook、merge-patch、同字段冲突与重读、显式 Namespace 不扩大、撤权清空数据通过 |
| 前端、契约与样式 | 全包测试通过（企业端 69 文件/293 测试），类型、lint、表单语义、token、隔离契约生成、diff 与文件 2,000 行限制通过 |
| 迁移与数据 | 正式备份恢复到隔离 PostgreSQL，52→54、54 down/up；13 类核心表数量与内容摘要保持一致 |
| 投递与健康 | 312 秒观察：terminal/enqueue/receiver 失败增量均为 0，三项采集持续更新；正式健康检查 20/20 通过 |

浏览器证据分别位于 real-browser、real-operations-browser、real-reader-browser、real-lifecycle-browser、real-publish-browser 和 shipped-real-browser。真实领域测试使用真实 API 对象/控制器与隔离控制数据库；上述正常登录实页另行验证了产品接入、权限、实际指标、会话与操作，二者未混为一种证据。

## 发布与修复

完整当前源码升级至数据库 54，保留 v1 DashboardSpec 读取及历史修订。最终运行镜像见下表，所有容器实际 imageID 与摘要一致，rollout ready：

| 组件 | 镜像 |
| --- | --- |
| 全部 8 个后端 Deployment | host.docker.internal:5001/argus/argus-backend:local-20261112-kwr-v8@sha256:a462723947e17f435e4b8bf0ce3067ef5054bbd4f2f3bdb7b702d71d61469d5b |
| 企业/平台/模板 Web | host.docker.internal:5001/argus/argus-web:local-20261112-kwr-v5@sha256:03f96ea6f8f8c409de574c00ff8dd748e5ad19e40ff3c63275eb448d3d779efe |

实际验证阶段修复了两个跨层问题，并追加回归：
1. CPU/内存请求面板必须绑定实际 k8s_cluster 来源；资源面板预览不能走遥测来源校验。默认受管模板已升级为版本 2。
2. Namespace 范围校验必须只读取显式 Namespace IDs；不得将所属集群 ID 当作整集群授权条件展开兄弟 Namespace。

另外补齐了权限读取原因的标题提示、失效关系对象重新选择、RBAC/修订/Namespace 关联、受控 Pod 删除说明、StatefulSet 更新说明及 Secret 键删除意图。

## 清理与保留

- 本轮临时 Pod、PVC 和 PV 已通过真实工作流删除并确认回收；扩容后原文件已先核验。
- 独立 Kind 测试集群已销毁；平台测试集群已退役；专用连接凭据和 Secret 已撤销。
- 临时账号已禁用、授权已撤销、专用角色已删除；部门禁用，审计所需历史身份保留。
- 测试仪表盘已归档，测试集群选板已恢复默认后退役。
- 隔离 PostgreSQL 及其匿名卷已删除；备份保存在 private 子目录用于回滚，不进入版本控制。
- B10b 原 Namespace/业务卷仍在，b10b-data 保持 128Mi；正式六个 PVC、业务账号、网关 guacd sidecar 和其他服务配置保持不变。
- 原工作区修改保留；没有创建提交或 PR。

## 用户查看路线

1. 打开 [B10b 集群](https://argus.dev/kubernetes/01a1cd21-4008-7b21-97fc-b808792615ab)，默认进入“大盘”。
2. 查看节点/Pod 分布、状态构成、真实使用/请求趋势、关系、事件和告警。点击节点/Pod 或状态进入同一资源详情；标题旁提示提供读取原因。
3. “资源查询”中点名称查看详情，使用行菜单进行具体动作；日志/终端可展开，提交操作后在当前上下文看进度。
4. 管理员可通过“选择大盘”选择适用已发布仪表盘，或恢复默认；调整默认内容需创建个人副本再发布。

B10b 原生连接的读取权限对部分资源类型有限，分布/关系面板会明确标识不完整并说明原因，不把无法读取显示成零值。指标来自实际 Collector；无 Collector 场景已另行实测，不生成示例指标。

当前正式环境仍为 evaluation 单机拓扑。健康报告保留既有 NetworkPolicy 未观察到强制拒绝、未部署外部 Egress Gateway 的降级说明；本次没有将其描述为强隔离生产部署。
