# B13a 查看与运行记录

2026-11-18：用户“下一轮”授权封存/退役并进入 [B13b](./B13b-review.md)。B13a 专属资源已正常卸载/清理，原三个问题和业务证据保留，不把推进指令写成明确人工通过。初始构建源码哈希表在一次批次目录复用错误中被覆盖；运行镜像按原清单重核验，原哈希表未伪造恢复，详情见 B13b 实施记录。下方为原交付时点，链接不代表专属资源仍运行。

2026-11-17，运行号 `p9-b13a-20261117-01`。状态为 **已部署、核心自检通过并保留已知问题、等待人工验收**。B13b/B13c 未启动，Plan9 未标记全部完成。

## 先看哪里

1. [B13a 范围与共享依赖大盘](https://argus.dev/dashboards/757061b0-9b91-46a3-b141-80b6cb46b897)：默认最近 15 分钟、30 秒刷新。应用筛选中选择 `B13a Orders`（production）或 `B13a Reports`（staging）以检查关联范围；两者一起选择时，Redis 只计一次。
2. [Redis 个人副本](https://argus.dev/dashboards/5e1fa5fa-6be6-4ec5-8f48-7947306debdd)：由受管 Redis v3 复制，已修改标题并通过正式预览/确认发布；原模板保持独立。命令速率单位为 `ops/s`。
3. [Memcached 归档历史](https://argus.dev/dashboards/f8cd2937-1919-5871-a126-d3d6166aaf0c?resource=811e6b51-3474-5ee3-a9f2-eb1c72f07d6e&resource_type=monitoring_target&time_from=2026-11-17T13%3A35%3A07.010317%2B00%3A00&time_to=2026-11-17T13%3A47%3A07.010317%2B00%3A00)：固定为归档前实际时间段。该目标和采集已归档、当前清单不显示，9 个历史面板仍可读，归档前后原始结果完全相同。
4. [主机列表](https://argus.dev/hosts)：`B13a-Orders-Host`、`B13a-Reports-Host` 均在线。管理员已有两台主机限定的 Shell/SSH 账号及七天终端授权，保留命令审计和录像。
5. [应用列表](https://argus.dev/applications)：两应用处于不同环境，当前都关联各自主机与同一个 Redis；Reports 的 Memcached 关联在归档测试中已结束。
6. [告警中心](https://argus.dev/alerts)：`B13a Reports service failed` 已真实触发并恢复。初始化投递缺口另行保留，不自动伪装成已恢复。

“不限制应用 / 全部授权资源”允许查询当前主体可读的保留期历史，包括已归档来源；这不代表来源仍在采集。检查 B13a 当前关联时请使用应用筛选，查看归档数据时使用上面的绝对时间链接。

## 实际环境

| 对象 | 真实行为 | 当前状态 |
| --- | --- | --- |
| Orders 主机 / 应用 | 每五秒执行真实 HTTP→共享 Redis 请求 | 在线、production 应用有效 |
| Reports 主机 / 应用 | 真实 HTTP→Redis/Memcached 请求 | 在线、staging 应用有效 |
| B13a-Shared-Redis | 原生 Redis receiver、独立监控凭据 | 采集中，两个应用共享同一个目标身份 |
| B13a-Reports-Memcached | 原生 Memcached receiver | 已完成目标归档验证，保留历史 |
| B13a Orders 只读成员 | 独立普通登录、角色和资源授权 | 仅 Orders 应用/主机、共享 Redis 和明确授权仪表盘；无终端授权 |
| 后台评估主体 | 与只读成员分离 | 仅新增本轮主机范围，无外部通知 |

两台逻辑主机共享 Docker Linux 内核；其 CPU/内存不能相加解释为两台独立物理机容量。本轮真实 HTTP 流量最多八小时，缓存键带 TTL；容器和数据在用户验收前保留。

## 自检证据

- 默认仪表盘 60 面板曾全部取得实际数据；当前 8 项主机内置采集 + Redis 共 9 项健康且新鲜。Memcached 9 面板通过历史链接查看。
- 单/双应用、类型和目标范围、显式空范围、面板局部条件均通过。两应用合并前 4 个唯一资源，归档后当前关联为 3 个；共享 Redis 数量为 1。
- 只读成员的“全部授权”保留当前授权范围；显式请求 Reports 应用、主机、Memcached 三类目标均被拒绝。真实界面资源候选不出现 Reports，清空选择后保持空范围。
- 真实 Reports 服务退出 → 告警触发 → 自动恢复保护/主动恢复 → 同一告警恢复；Orders 与 Redis 在故障窗口保持可用。
- 8 项真实浏览器 E2E，覆盖 1280/1440/1920、中英、明暗、键盘刷新、可见自动刷新、离开页面停止请求、只读和空范围；未跳过 TLS。
- 真实临时 PostgreSQL 上 `internal/monitoring`、`internal/dashboard` 测试通过，临时数据库容器已清理；隔离契约生成检查及前端类型检查通过。
- 正式健康检查 20/20；六个正式 PVC 和所有 Deployment UID 保留。后台精确镜像为 `host.docker.internal:5001/argus/argus-backend:local-20261117-b13a-target-archive-v2@sha256:80c72cc902a051af336c64321a0a83f91c75bfc643e4c574167b7a7ccb2592fd`，Helm revision 109，没有新增数据库迁移。
- 本轮未调用模型，不消耗 APIMart Token 或 Workspace 额度。B12 模型、会话、报告、账单和审计继续保留。

## 已知问题，不能写成全程零失败

- **B13a-D01**：初始化时两台 Collector 曾报告 `SOURCE_REJECTED`，Orders 永久丢弃 1,642、Reports 568 个指标点，总计 2,210。日志证实拒绝，与连续配置切换同窗；尚未以受控复现证明完整根因。最新 120 秒观察没有新增丢失、receiver 拒绝或入队失败，原实例 ID 和计数保留。公共接入脚本已改为等待每个内置配置生效后再提交下一项；首次接入和切换排空效果仍需后续实际验证，不放宽服务端来源栅栏、不清零历史告警。
- **B13a-D02**：本轮容器把凭据文件绑定在 `/root` 下，Collector 服务隔离后 filesystem scraper 对此挂载路径记录读取警告。CPU/内存、缓存和已有磁盘序列可读，但不宣称整机所有挂载点采集无警告；后续测试包装需避免此路径，不能为此放松 Secret/ProtectHome 隔离。
- **B13a-D03**：本次目标归档经过正式 API 的预览、确认和执行验证。独立目标归档的门户入口、完整撤权/生命周期交互留在 B13c，不以 API 通过冒充门户全流程通过。

## 修复与交接

新增修复和契约见 [B13a 实施记录](../36-b13a-dashboards-scope-and-archive-20261117.md)，运行方式见 [runbook](../runtime/B13a/runbook.md)。

证据根目录：`C:/Users/23574/.codex/artifacts/argus-plan9/p9-b13a-20261117-01`。主要文件：`handoff.json`、`scope-acceptance.json`、`personal-copy-check.json`、`archival-acceptance.json`、`alert-cycle.json`、`delivery-watch.json`、`known-findings.json`、`current-state.json`、`postgres-tests.json`、`contracts-check.log`、`frontend-typecheck.log`、`browser-delivery.log`、`formal-health.json`、`template-release.json`。截图位于 `browser-delivery/`；密码/会话及初始原始日志位于 ACL 限定的 `private/`，不进入仓库。

用户本次“继续下一轮 B13a”已作为 B12 接受/退役授权记录。B12 专属主机正常卸载，Redis 采集与目标已归档，Compose 容器/卷/网络及专属遥测入口已清理。B13a 现在等待用户查看，后续须在接受本轮并选择下一轮后才退役；不自动推进 B13b。
