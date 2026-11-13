# Kubernetes 工作台资源能力矩阵

最新状态：二次复核的五组剩余项已补齐，当前关闭依据见 [剩余项补齐验收](./kubernetes-workbench-closure-20261113.md)。本文保留其原阶段的实施与验收事实。

> 原矩阵的字段、只读详情及引用缺口现已补齐，逐类对照和实际验收见 [剩余项补齐验收](./kubernetes-workbench-closure-20261113.md)。

本矩阵对应 [完整实现计划](./kubernetes-resource-workbench-refactor-plan-20261112.md)。发布与真实页面验收状态见 [本轮验收记录](./kubernetes-workbench-acceptance-20261112.md)。

## 统一边界

列表、详情和大盘使用 Kubernetes inventory 的同一 Summary 状态模型。UID、resourceVersion、generation 位于技术信息页，名称为详情入口。Namespace、名称、标签、状态、分页游标与详情身份进入 URL；列表手动刷新，操作收敛后失效相关查询。

动作列表同时受资源类型、当前原生状态和 Argus 权限约束。requires_check 表示可以进入预览，原生 RBAC、准入、不可变字段、同名重建与版本仍须通过服务端实时校验；该值不承诺写入必定成功。只读展示入口没有无效写按钮。

本轮补齐：requires_check 在菜单说明和直接动作提示中明确呈现；ConfigMap 文件值使用多行编辑，保留换行及未编辑项。新打开的操作表单强制先读取当前对象，避免将上一动作的缓存版本作为编辑基线；编辑开始后仍保留真实版本冲突与重新预览流程。

Deployment、StatefulSet、DaemonSet 的就绪情况与新版本更新进度分别显示，结合 observedGeneration、updated 副本、可用副本、StatefulSet 分区和 OnDelete 解释状态。HPA 当前/目标指标、LimitRange 默认/约束、EndpointSlice 就绪条件、CRD 版本与字段定义进入专用详情；内置趋势适配目前覆盖 Pod、Node、Deployment、DaemonSet，其余类型不显示误导性的“未接入监控”页签。

对象关联/事件使用 UID 绑定的有界分页接口，PDB 显示匹配的 Pod 与工作负载，CRD 可分页查看实例。操作反馈可直达本次记录和 UID 绑定的结果资源，原生结果折叠在技术信息内。

| 类型 | 主要字段 / 状态 | 常用编辑与动作 |
| --- | --- | --- |
| Pod | Ready、重启、所属资源、节点、容器等待原因 | 日志、终端、容器配置、删除；未运行容器不可执行 Shell |
| Deployment | Ready / desired、镜像、策略、HPA 关系 | 扩缩容、镜像更新、重启、回滚、暂停/恢复、配置表单 |
| StatefulSet | Ready、镜像、更新策略、Service | 扩缩容、镜像更新、重启、回滚、配置表单 |
| DaemonSet | Ready / desired、镜像、策略 | 镜像更新、重启、回滚、配置表单 |
| ReplicaSet / ControllerRevision | 副本、镜像 / 修订、拥有关系 | ReplicaSet 扩缩容；高级清单保留原生约束，修订用于回滚选择 |
| Job / CronJob | 活动、完成、失败、来源 / 调度、时区、最近运行 | 暂停/恢复；CronJob 调度表单、运行一次；完成 Job 不提供可执行的暂停动作 |
| HPA / PDB | 当前与目标副本、伸缩目标 / 健康数、可驱逐数 | 指标目标、范围、选择器、可用性预算表单；原生控制器收敛验证 |
| Service / Ingress / IngressClass | 类型、地址、端口 / 域名与后端 / controller | 端口、选择器、路径、TLS 引用等表单 |
| EndpointSlice | 地址类型、端点、Service 与 Pod 关系 | 只读 |
| NetworkPolicy | 策略方向、入站/出站规则 | Pod/Namespace 选择器、IP 范围、端口规则表单 |
| ConfigMap / Secret | 配置项数量、类型 | 键值与元数据；Secret 值独立保护输入，不进入历史和普通清单 |
| PVC | 请求/实际容量、StorageClass、PV、使用者 | 专用扩容；拒绝缩容，驱动与实际容量由后端验证 |
| PV / StorageClass | 容量、绑定、回收 / provisioner、绑定方式、可扩容 | 只读；PVC 扩容不代替修改存储基础设施 |
| Node | Ready、调度、可分配资源、kubelet 版本 | 禁止/恢复调度、受保护排空、配置与相关 Pod |
| Namespace | 原生 phase、资源目录 | 创建、元数据、高级清单及受控删除 |
| ServiceAccount、Role、RoleBinding、ClusterRole、ClusterRoleBinding | 规则、主体、角色引用 | 专用规则/主体表单；权限委派与原生 RBAC 再校验 |
| ResourceQuota / LimitRange | 配额和使用 / 默认请求与限制 | 资源配额、请求/限制表单 |
| Event | 级别、原因、消息、时间、关联 UID | 只读；详情按真实对象 UID 过滤 |
| CRD / 扩展实例 | 注册类型、scope、conditions、phase | 动态发现、通用详情与高级清单；不伪造任意扩展类型的业务状态 |

## 配置与并发

表单预填投影后的真实对象；只计算变更字段，保留其他原生字段。高级清单遵守 merge-patch 规则：省略不删除，显式 null 才删除。Secret 和敏感环境值继续遵守既有投影和独立私有输入规则。

版本改变时保留编辑内容。重新读取按字段合并未改内容；同字段冲突或已删除容器需要用户明确选择，再生成新的预览。不会自动重放写入。异步失败可重新打开保留的输入，仍需重读、预览、确认。

确认提交后立即在当前资源上下文显示进度。先跟踪 Execution，再跟踪 Kubernetes operation；区分排队、运行、等待生效、成功、失败、未知、等待审批和被后续变更取代。全局历史由用户主动打开。

日志与终端复用共享可调整工作区，支持键盘展开/恢复。打开日志即读取，向上查看历史暂停跟随，回到底部恢复；关闭、撤权、容器代次改变和到期继续终止原会话。无 Shell 镜像保持明确的普通终端支持范围，不自动创建调试容器。

## 当前验证边界

隔离数据库测试、真实 Kubernetes 领域流程和浏览器布局测试分别记证据，不能互相替代。目录中的只读类型无需为了证明界面而修改 PV、StorageClass 或 Event。正式存储、当前 B10b 资源和用户现有工作区改动均保留。


## 2026-11-13 列表和详情交互

全部资源行操作统一进入“更多”菜单，Pod 日志/终端和复制定位链接也使用同一入口。尚需检查的动作只在菜单顶部显示一条统一说明；已知不可用原因仍逐项展示，预览、确认及执行继续按服务端校验。详情右上角使用放大/恢复图标调整抽屉宽度，保留当前分区和对象身份，点击左侧遮罩可关闭。保存中的共享表单继续阻止关闭，避免丢失提交。详见 [菜单与抽屉验收](./kubernetes-resource-actions-drawers-20261113.md)。
