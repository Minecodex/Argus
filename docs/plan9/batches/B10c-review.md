# B10c 查看与运行记录

运行标识：`p9-b10c-20261113-01`。当前状态：**B10c 已由用户明确通过，并于 2026-11-13 按要求清理；当前运行批次为 [B11a](./B11a-review.md)**。下方是历史验收路线，原集群、Helm 源和专属工作负载已销毁，链接不再代表在线现场。正式 Argus 数据及六个 PVC、历史告警/审计均保留。

用户确认：“没啥问题，继续 plan9 下一轮，清理 B10c 后部署 B11a”。验收归档 `accepted-retirement-archive.json`、`cleanup.json` 与 `cleanup-verification.json` 保存在本批 C 盘产物目录；清理完成时间 2026-11-13 22:02:01 +08:00。此前三项界面反馈和资源列表趋势一起归入本次人工确认。

按用户“继续 plan9 下一轮检查”进入本批。B10b 记录为用户要求推进后的退役，不改写为明确人工通过；已先回收消费者、CSI PVC/PV，再卸载采集器并销毁其独立节点。旧专属运行容器为零，历史和正式 Argus 的六个 PVC 保留。

## 查看路线

1. 打开 [B10c 集群大盘](https://argus.dev/kubernetes/01a1d448-13b5-7728-afb9-3eabbf65e11b)。集群名称为 **B10c Helm Extensions and Node Maintenance**，实际为三个 Ready 节点。大盘展示当前授权快照与最近一小时趋势。
2. 打开 [业务工作负载](https://argus.dev/kubernetes/01a1d448-13b5-7728-afb9-3eabbf65e11b?tab=resources&kind=deployment&namespace_id=1843e815-6df1-5b4c-846c-05a112fe6607&namespace_name=b10c-workloads)，检查 `b10c-repo`、`b10c-oci`、`b10c-maintenance` 和 `b10c-activity`。四个 Deployment 均已恢复就绪；名称进入详情，列表 `…` 使用现有真实操作流程。
3. 切换 Pod，打开业务容器日志和终端。实际浏览器已读取 HTTP 请求日志，并在业务 Pod 的真实终端执行命令返回本批运行标识；会话验证后关闭。Collector 镜像不带 Shell，仍按既定范围说明处理。
4. [节点列表](https://argus.dev/kubernetes/01a1d448-13b5-7728-afb9-3eabbf65e11b?tab=resources&kind=node) 中查看 `p9-b10c-20261113-01-worker`，保留标签 `argus.plan9/maintenance=verified`。当前已恢复调度，临时维护污点已移除，PDB 的 `minAvailable` 恢复为 2。
5. 资源查询 → 扩展资源，选择 `checks.b10c.argus.test`，查看 `b10c-check`。**YAML / JSON** 中可见实际消息 `Updated through the normal Argus workbench` 和 `replicas: 2`。此 CR 无控制器，状态未知符合实际；不伪造 Ready。
6. 资源查询 → 访问控制，检查业务 Namespace 的 Role `b10c-observer`、RoleBinding 和 ServiceAccount `b10c-reader`。读权限已实际撤销/恢复；提权和控制节点修改均被原生权限拒绝。
7. [集群仪表盘](https://argus.dev/kubernetes/01a1d448-13b5-7728-afb9-3eabbf65e11b?tab=dashboards) 检查下表四套视图，再到 [告警中心](https://argus.dev/alerts) 的告警历史查看本批已恢复事件。

Plan9Check 需要选择 **`b10c-workloads`**；默认全部范围超过本批原生凭据的读取权限。[范围拒绝提示与选择恢复已修复](../21-b10c-namespace-scope-error-20261113.md)，该记录含可直接打开的业务范围链接。原有正向自检漏测默认全部范围的情况已单列补测。

Helm 沿用当前受支持的 **API/CLI** 路径验收，门户查看其生成的对象、操作进度与结果；不提供已移除的 Helm 专属页面。已保留真实 release：

| Release | 来源 | 当前版本 / 修订 | 已实际验证 |
| --- | --- | --- | --- |
| `b10c-repo` | 本批 HTTPS/Basic 仓库 | Chart 0.1.0 / revision 3 | 安装、升级 0.2.0、回滚 0.1.0、Hook 输出 |
| `b10c-oci` | 本批 HTTPS OCI Registry | Chart 0.2.0 / revision 1 | 保留在线供查看 |
| `b10c-offline-check` | 预览后关闭真实仓库 | 已卸载，保留历史 | 源持续关闭至确认执行成功，冻结制品安装成功 |
| `b10c-oci-lifecycle` | OCI | 已卸载，保留历史 | 安装、升级、回滚、卸载及清理关联对象 |

仓库与 OCI 的成功 Hook 由真实进程输出 `B10C_HOOK_OK`。临时 release 和 Hook 对象已删除，两个保留 release 持续运行。

## 指标、仪表盘和告警

三项原生采集均为健康且数据新鲜：集群/工作负载、节点/容器、Collector 自身。四套指标仪表盘的一小时执行结果共 **21 个面板**，逐个核对成功且有实际有限数值；另有默认嵌入的 **12 面板 Kubernetes 运维大盘**。

| 仪表盘 | 实际面板数 | 内容 |
| --- | --- | --- |
| B10c Helm 与节点维护 | 3 | 发布工作负载可用副本、CPU、内存 |
| Kubernetes cluster | 6 | 实际集群与工作负载指标 |
| Kubernetes nodes and containers | 6 | 实际节点/容器指标 |
| Collector | 6 | 实际采集器收发与运行指标 |

策略 **B10c released workload has no available replica** 监控 `b10c-repo` 实际可用副本。正常平台扩缩容 `2 → 0 → 2` 已触发并恢复：

- 触发：**2026-11-13 18:35:50**；恢复：**18:36:40**，均为 Asia/Shanghai。
- 事件 ID：`889d7762-f10a-4194-a641-5d85daf32b08`，当前无本批活跃告警。
- 120 秒、带 Deployment UID/副本前置条件的恢复定时器作为兜底；正常恢复后已停止。没有外部通知目标。

交付前额外观察 **312 秒**的网关持久投递账本：终止失败、入队失败、接收拒绝均无新增，序列推进；三项采集的样本时间持续推进，当前列表无未触发的普通 `no_data` 记录。该检查范围是网关与三项采集推进，不代表逐一验证所有节点 Agent 的持久账本。

## 真实操作、自检和发布

| 范围 | 实测结果 | 证据文件 |
| --- | --- | --- |
| Helm 仓库/OCI | 生命周期、Hook 和源离线完整执行通过 | `helm-final-releases.json`、`helm-oci-lifecycle.json`、`source-offline-execution-proof.json` |
| CR / RBAC | 实例 CRUD、读权限撤销/恢复、越权拒绝通过 | `extension-rbac-exercise.json`、各操作回执 |
| 节点维护 | PDB 阻塞 → 恢复预算 → 实际 Eviction/异节点重建 → uncordon；标签/污点通过 | `drain-pdb-held.json`、`node-maintenance-exercise.json` |
| 指标与告警 | 三项新鲜采集、21 面板和真实触发/恢复通过 | `checks.json`、`alert-cycle.json` |
| 持续投递 | 完整观察窗无新增失败，三项样本推进 | `delivery-watch.json`、`delivery-observations.json` |
| 真实页面 | 正常登录、严格 TLS，中英/明暗、1280/1920 桌面；详情、CR、RBAC、大盘、告警和真实日志/终端共 5 项通过 | `browser-review.json`、`browser.log`、`browser/` |
| 正式健康 | evaluation 配置的 20/20 检查通过，既有环境降级单列保留 | `formal-health-after-fix.log` |
| 数据保留 | 六个正式 PVC 的 UID/绑定保留；本批诊断资源已清理 | `final-preservation.json`、`custom-diagnostics-cleanup.json` |

实际扩展资源预览发现并修复 [B10c-CR01：审计租约与预览保存事务冲突](../20-b10c-custom-resource-preview-transaction.md)。修复不改变公开协议或 schema 54；真实 PostgreSQL 三项专项回归、WorkBench/Inventory/Reader/Resource 四包通过，并已在同批真实 CRUD 路径复验。

现场 Server/Worker 为 `local-20261113-b10c-resource-trends-v2@sha256:370baf15a64c2f3d552eed823d6f90095b89c378385671022dcbd10e0ee3beec`；Web 已更新为 `local-20261113-b10c-resource-trends-v5@sha256:c235e4f287553be168d64000b3eaf51b9e1c69f3a761f02165ad33e957a32baa`。均保存冻结源码、实际 imageID 和 rollout 证据；其余正式后端部署与数据保留。首版 Web 的原始发布证据仍保留。

最初真实浏览器有四个用例把 CR 的通用“配置”Tab 当作清单入口，断言失败；校正到产品已有 YAML / JSON 后完整重跑 5/5 通过。源准备阶段的私网 TLS EOF、旧 Helm 推送行为及预览事务失败也保留原始日志，不抹去失败记录。TLS 仓库和 OCI 发布最终使用严格证书校验、仅本机发布端口和显式 HTTPS ORAS SDK，没有放宽产品策略。

## 表格操作列颜色反馈

用户截图中的固定“操作”列表头与其余表头采用了不同的背景规则；固定单元格还覆盖了行悬停背景。已在共享表格样式中统一使用 `--bg-elevated` 表头底色；工作台固定列继承当前行的实色背景，常态与悬停均保持整行一致，横向滚动时仍遮挡下方内容。所有资源类型共用该修复，中英 × 明暗 × 1280/1440/1920 共 12 组表格外观检查及完整真实页面 5/5 通过。此项不改业务数据、资源授权或后端部署，交付证据为本批 `table-surface-feedback.json`，人工复查仍待用户确认。

## 列表趋势与渐进加载

已按确认方案增加 CPU、内存及适用的存储迷你趋势。列表与操作先显示，当前页指标独立渐进加载；镜像、副本细节与创建时间收进名称摘要，Job/CronJob 等不增加无对应概念的指标。实际口径、权限边界、临时进程场景与查看证据见 [渐进式资源趋势记录](../22-b10c-progressive-resource-trends-20261113.md)。新增真实流程 6 项及既有详情/日志/终端 5 项分别验收，人工查看仍待用户确认。

## 阅读数据时的边界

- 管理凭据只对一个专用 worker 开放维护写入；控制节点、系统工作负载写入保持拒绝。跨 Namespace 的 ConfigMap/Secret 不开放全量读取，因此默认大盘的相关聚合可能显示“结果不完整”。可以选择 `b10c-workloads` 查看授权范围；不为使页面全绿扩大权限。
- 当前快照可能保留部署、迁移及受限读取产生的历史 Warning Event。事件数量不等于当前异常工作负载数量；应结合实际 Ready 状态和事件时间查看。
- Kind 三节点共用 Docker VM；分别限制 3/2/2 GiB、每节点 2 核，节点报告的共享 VM 容量不能相加为独立物理容量。
- 正式 evaluation 环境已有 NetworkPolicy 拒绝探测未生效、未安装外部 Egress Gateway、共享容器沙箱运行时三项降级，本批未宣称通过生产网络隔离认证。B10c 的原生 RBAC 拒绝和 PDB/Eviction 均有独立实际证据。
- 低频 HTTP 活动最多运行 8 小时，结束后进入空闲，服务和采集器保留。实际管理令牌有效至 **2026-11-20 17:40:46 Asia/Shanghai**；超过时效需正常续期。源证书和认证文件保留在受限 private 目录中，供当前批运行及后续清理。

## 产物与下一步

运行包见 [B10c runbook](../runtime/B10c/runbook.md)。实际产物目录：`C:/Users/23574/.codex/artifacts/argus-plan9/p9-b10c-20261113-01/`。平台会话、源密码和管理 kubeconfig 不进入 Git；临时浏览器/诊断会话完成后退出。

继续保留 B10c 等待用户反馈。只有用户要求下一轮或销毁时才执行归属清理；顺序中的下一子批为 **B11a 堡垒机与远程连接直连验证**。
