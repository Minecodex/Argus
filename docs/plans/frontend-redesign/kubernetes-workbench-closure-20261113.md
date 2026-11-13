# Kubernetes 工作台剩余项补齐

状态：**C01～C05 已完成实现、回归、真实验证、部署和清理。** 本轮已确认的剩余项全部关闭；原计划完整范围按本记录与保留的上轮证据核对。

范围依据 [第二次完成度复核](./kubernetes-workbench-completion-review-20261113.md)。运行证据目录为 artifacts/kubernetes-workbench-refactor/20261113-closure/。

| 项目 | 实现 | 已有验证 |
| --- | --- | --- |
| C01 | DaemonSet 期望节点数按原生字段赋值；故障原因优先于普通等待原因，涵盖 init 失败、重启统计与可重启 sidecar | 原失败探针转为通过，状态单测及相关 Go 回归 |
| C02 | 通用显式引用提取覆盖 TLS、IngressClass、projected 卷、StatefulSet Service、Pod/工作负载模板等；跨 Namespace RBAC 候选仍受当前授权限制 | 正反向关系、同名 Namespace 替换、授权与撤权集成测试 |
| C03 | URL 保存页序；有界重建缺失前页，支持取消/继续和范围变化终止 | 单测涵盖复制页序、每批 8 页上限、旧请求取消；原浏览器失败路径通过 |
| C04 | UID 绑定结果携带原操作上下文，原记录重新授权匹配；对象不可用时明确解释并保留入口 | 原回收对象 mock 浏览器路径通过，实际 TTL 回收与保留原操作入口验证通过 |
| C05 | 逐类增加只读规则、路由、端口、选择器、节点角色/Conditions/容量、容器状态、存储、配额和工作负载信息；扩充列表字段和共享数量格式 | Role 原浏览器复现通过，类型检查通过；真实中英明暗与桌面矩阵验证通过 |

实现继续复用现有领域读取、投影、授权、动作与共享 UI。没有新增资源状态库或写入通道。公开读取协议及数据库 schema 54 保持不变，客户端路由字段不作为服务端授权依据。

最终镜像、真实验证结果、清理与保留核对见下文。

## 验证与实际发布

- 相关 Go 四包回归及 PostgreSQL 授权/关联集成通过；`final-go.log`。原先失败的状态/引用探针全部转为通过，见 `original-probes-passed.log`。
- 前端全量 API client/UI/Enterprise 单测通过，分页专项 3 例覆盖复制链接、每批 8 页上限和范围变化取消；`frontend-all.log`、`use-inventory-pagination.test.tsx`。
- 38 项桌面与既有流程回归通过；`mock-desktop.log`。
- 正常登录、真实双节点集群的四组中英明暗完整用例及一组受限账号反例均通过；`real-v3.log`。包含实际 DaemonSet 数量、多容器故障、TLS/projected/Service/跨 Namespace 引用、规则和节点详情、第三页重开后的逐页返回，以及实际 TTL 回收后从结果页返回原操作。
- B10b 三种桌面宽度 × 中英 × 明暗共 12 项通过；`desktop-final.log`。
- 隔离契约检查通过；`contracts-final.log`。数据库保持 schema 54，无新增迁移。

后端运行 `local-20261113-closure-v3`（`sha256:f113a6986d9d809a18bbf4f6dbcf4ba4ff8e256a70ea56ab4396b4348d8b9b8a`）；Web 保留生产内容相同且 runtime digest 可直接核对的 v2（`sha256:4d013ad8477d133265d5b0b7270cc107a283fd4d729623f37ea0a4ba5e9b2ef9`）。Web v3 仅测试内容发生变化，生产源码与 v2 一致，未将容器运行时复用的旧 repoDigest误写成新镜像摘要。

B10b 原有 24 小时接入令牌在跨日验证时到期，导致所有 inventory 类型报权限错误。本轮通过正常 Secret rotate 为原 ServiceAccount 续期，保留同一 Secret ID、credential 和全部 RBAC 规则，读取已恢复；见 `b10b-token-renewal.json`。没有将这一到期状态当成业务读取为零。

初始 TTL 测试对象在观察完成前被回收，因此没有提前宣布操作成功。最终用例先确认原生执行与平台记录，再设置 TTL 触发实际回收并验证结束态；失败过程保留在早期日志中。

此前记录的环境边界继续有效：缺少某些类型的接入权限会明确显示不完整；本地 evaluation 网络隔离警告不等于本轮 UI/授权实现未通过，也不作为生产网络隔离认证。

## 清理与最终核对

`cleanup.json` 八个步骤全部完成：撤销临时用户授权，删除两个测试 Namespace，注销测试集群并禁用凭据和账号，删除专用双节点 Kind 及带唯一 ID/所有权校验的 PostgreSQL 测试容器与卷。禁用账号/部门和审计历史按产品规则保留。

B10b 保持运行，原有 b10b-data 仍为 128Mi，6 个正式 PVC 的身份和绑定没有变化。代理 API 会话已登出，临时认证文件及健康检查配置副本已脱敏，正式连接配置保留；`private-cleanup.json`。

最终四个趋势查询均有真实样本、12 个面板可执行，源码和冻结构建匹配，修改的代码文件均不超过 2,000 行；见 `final-verification.json` 和 `web-equivalence.json`。部署健康 20/20，原 evaluation 网络隔离边界仍保留，不作为生产网络隔离认证。

## 与原资源矩阵对照

| 范围 | 当前只读内容/关联 |
| --- | --- |
| Pod/控制器 | 主/初始/临时容器状态、原因、重启、卷、Conditions、镜像/资源配置；受控 Pod 与控制器 UID 关系 |
| Deployment/StatefulSet/DaemonSet/ReplicaSet/修订 | Ready/Updated/Available、当前/目标修订、策略、HPA 及受控对象；StatefulSet Service/卷模板与保留策略 |
| Job/CronJob | 活动/成功/失败、并发、目标完成数、耗时、周期/最近运行、历史限制；结果保留原操作，回收/替换不打开同名新对象 |
| Service/Ingress/IngressClass/EndpointSlice | 端口/选择器、路由、路径、后端、TLS 引用、Class/参数、实际负载均衡地址和端点条件 |
| NetworkPolicy/HPA/PDB | 选择器、方向/peer/端口规则、实际目标和指标、健康数/中断预算/受影响对象；策略存在不冒充阻断证明 |
| ConfigMap/Secret | 多行文件、键与投影值；只解析公开引用，不增加 Secret 原值读取 |
| Node | 角色、污点、地址、容量/可分配量、完整原生 Conditions、Pod、事件、已有指标 |
| PVC/PV/StorageClass | 请求/实际容量、模式、绑定、Conditions、参数、拓扑和引用；PV/StorageClass 维持只读 |
| Namespace/Quota/LimitRange | 原生状态、作用范围、资源配额的使用/配置/观测上限、默认值和约束、授权资源分页 |
| RBAC/ServiceAccount | API 组/资源/动词/名称限制/非资源 URL 的规则表，主体/角色引用与跨 Namespace 授权裁剪；不显示 token 原值 |
| CRD/扩展实例 | 版本、schema、实例分页、投影配置、原生 Conditions/可证明状态；没有通用状态则显示未知 |

列表 URL 的页序与游标配对，复制和重载后仍能返回前页；缺失前页恢复每批 8 次、每次 30 个对象，范围变化取消，过期或页序不存在明确提示。已有指标详情适配范围继续明确列出，未支持的趋势类型不被表述为未采集。

本轮真实验证 17 个不同场景：四个中英明暗完整流程、一个受限账号反例、12 个桌面矩阵；重跑不累计成新场景。初期测试失败及修正记录保留，不能将它们描述成一次无失败执行。当前五组反例及其正/反向回归均有通过证据。
