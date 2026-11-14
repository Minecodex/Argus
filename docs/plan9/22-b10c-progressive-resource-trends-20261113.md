# Kubernetes 资源列表趋势与渐进加载

2026-11-13，按用户确认实施 [资源列表用量趋势计划](../plans/frontend-redesign/kubernetes-list-resource-trends-20261113.md)。B10c 继续保留运行，不推进下一批。

## 当前行为

- 原有列表先显示名称、状态、副本与操作；指标不参与列表请求。当前页稳定后准备一次有界上下文，CPU、内存、存储三个批次独立读取，先返回的先显示，失败仅影响对应单元格。
- 支持 Pod、Deployment、StatefulSet、DaemonSet、ReplicaSet、Namespace、Node 的 CPU/内存；Node 本地文件系统、Pod/工作负载临时存储及适用的 PVC 卷另行标注。Job/CronJob、ControllerRevision、配置、网络、RBAC 等列表不加无意义指标。
- 默认最近 30 分钟、30 秒点距。工作负载按当前 Pod 的真实控制器 UID 链与 Namespace 身份汇总；不把 Namespace 总量套给每个工作负载，也不声称曲线代表已删除的历史成员。
- 完整上限使用上限比例，只有完整申请时显示申请利用率，可超过 100%；无完整分母时展示绝对用量。历史申请/上限取对应历史指标，缺失时不拿当前配置套算过去。当前有效申请/上限在提示中另外说明。
- Node CPU/内存按本次原生快照容量作参照，另外显示可分配量与当前 Pod 申请；Kind 节点共用 Docker VM 的容量边界沿用 B10c 原记录，不能相加为独立物理容量。
- 存储区分临时存储、节点文件系统和卷；共享卷按当前 PVC 身份去重。宿主路径/存储驱动未提供卷统计时显示无样本，不补零。
- 名称悬浮/聚焦显示镜像、更新与副本细节、创建时间，可复制镜像；点击名称仍进入详情。摘要为非模态；打开详情或其他工作台弹窗时收起，保留退出动画后的单一详情上下文。
- 列表只保留适用的一个副本列；镜像、重复副本字段和指标列表的创建时间移入悬浮摘要/详情，留出三列趋势空间。
- 手动刷新、翻页、切换范围或页签会取消旧请求；浏览器后台暂停，返回后准备新上下文，不复用过期令牌。不同用户、资源、范围和列表刷新有独立缓存键。

## 接口与权限

新增只读 `POST inventory/metrics/prepare` 和 `POST inventory/metrics/read`，使用 OpenAPI 生成 Go/TS 契约。准备请求含原列表条件与当前页 UID，最多 50 行；服务端重读本页并校验身份，Pod/ReplicaSet/PVC 扫描有界，达到预算时明确不完整。

上下文签名绑定企业、用户、授权版本、集群与当前范围，2 分钟有效，不含凭据。每次读取及返回前复验资源、Namespace UID、遥测授权与来源；服务账号沿用原有查询工具白名单。没有新增数据库表、遥测协议或任意 URL 查询能力。

静态 Pod 的 API UID 与 kubelet 指标 UID 可能不同。仅接受原生 `config.source=file` 和 `config.mirror` 的映射，指标按 Namespace、名称、UID 联合匹配，剪除对象创建前的 lookback 样本。已有完整 Cluster 数据授权的用户使用其已授权的 Cluster 来源并由服务端固定选择器收窄；仅 Namespace 授权的用户始终使用 Namespace 投影，不能借上下文扩大为 Cluster。

## 真实验证

产物目录：`C:/Users/23574/.codex/artifacts/argus-plan9/p9-b10c-20261113-01/resource-trends-v1/`。

| 范围 | 证据 |
| --- | --- |
| 归属、签名防重放/过期、撤权、有效请求量、缺口/零值、历史分母、静态 UID 与作用域边界 | `go-final-unit-v2.log`，9 项专项测试 |
| 有界资源服务与既有领域/HTTP 集成 | `go-regression.log`，四包及独立 tmpfs PostgreSQL 回归 |
| 渐进返回、单项失败、取消旧页面、隐藏暂停/恢复、Job/CronJob 不请求 | `frontend-regression.log`，含 4 项加载测试和双语回归 |
| 生成契约、类型、样式及 i18n | `contracts-check.log`、`typecheck-summary-dismiss.log`、静态检查记录 |
| Deployment/Pod/Namespace/Node/ReplicaSet/DaemonSet 实际 CPU、内存、存储 | `native-checks.json`、18 组实际 API 数据文件 |
| 实际无上限、仅申请、部分配置进程场景 | `fixtures.json`、`fixture-metric-checks.json`；经正常预览/确认创建 Pod，进程真实写 1 MiB 文件，CPU 活动最多 180 秒 |
| 正常登录、严格 TLS，中英/明暗、1280/1440/1920、曲线检查、镜像摘要、仅延迟/中断真实传输的渐进与隔离验证 | `browser-review.json`、`browser.log`、`browser/`，新增 6 项通过 |
| 既有详情、放大/关闭、CR/RBAC、告警、真实日志/终端 | 上一级 `browser-review.json` 与 `browser.log`，最终结论见交付记录 |
| 最终镜像、六个正式 PVC、临时资源清理及健康 | `final-review.json`、`formal-health-current.log`、清理记录 |

所有指标来自实际 Collector；传输延迟/断连测试不替换响应数据。首轮取消用例采用整页跳转和暂停路由下的 `requestfailed` 观察，后改为真实 SPA 资源切换并直接观测 AbortSignal；原始日志保留为 `browser-first.log`。名称摘要与详情叠层在回归中发现，已加非模态和显式关闭约束；既有色彩测试改为悬停纯文本单元格，避免触发新增摘要，详情断言等待退出动画结束。失败证据保留，不抹去历史。

Server/Worker 发布为 `local-20261113-b10c-resource-trends-v2@sha256:370baf15a64c2f3d552eed823d6f90095b89c378385671022dcbd10e0ee3beec`；Web 为 `local-20261113-b10c-resource-trends-v5@sha256:c235e4f287553be168d64000b3eaf51b9e1c69f3a761f02165ad33e957a32baa`。其余正式组件保留；现场 rollout、实际 Pod imageID 与冻结生产源码另行核对。

正式 evaluation 环境既有的网络隔离、外部 Egress Gateway 与共享沙箱降级继续单列，指标列表改动不宣称补齐这些环境能力。

最终交付核对：新增真实页面 **6/6**、原有详情/日志/终端 **5/5**、正式健康 **20/20**、18 组实际指标批次与契约检查通过；三项临时 Pod 和独立 tmpfs 数据库均已清理，六个正式 PVC 保留。B10c 保持运行，人工验收待用户查看。
