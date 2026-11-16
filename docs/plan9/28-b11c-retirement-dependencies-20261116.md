# B11c 正常卸载的历史依赖投影修正

2026-11-16 用户通过 B11c 并要求下一轮。成员先行正常 SSH 卸载并核实本机清理，随后删除记录；Bastion 在停采后还需先卸载 Edge Collector。原清理包遗漏这一步，已补齐。

Collector 已正常卸载后，历史 `telemetry_routes.status=active` 与已清理成员的 `telemetry_tunnels.status=down` 仍被算为活动依赖，使确认返回 `PENDING_ACTION_INVALIDATED`。没有通过 SQL 改状态、强制忘记目标或销毁后冒充成功。

修正共享 `ListBastionRemovalDependencies` 只读投影：已 uninstalled Collector 的历史路由不阻塞；成员必须同时满足 deleted、本机 cleanup=verified、Collector uninstalled、隧道 down 才忽略这个历史隧道。活动 Collector/路由、仍 established 的隧道、活动成员、未知清理和缺失身份仍阻止卸载；租户过滤保持不变。

通过独立 PostgreSQL 临时库和临时表验证以上正负边界及跨租户读取，Host Removal 包测试通过，SQLC 1.30 正常生成并拆分客户端。没有新增迁移或修改正式业务事实。临时数据库容器与数据已删除。

server/worker 使用同一修正版本正常 Helm 发布，后端为 `local-20261116-b11c-retirement@sha256:352e8e18d8cbc582cd981670084a2b0657e073fd7d42e847826d56d7e78b7301`；所有正式 Deployment/PVC UID 保留。随后通过新 SSH 检测、预览/确认完成 Bastion 正常卸载，实际单位 LoadState=not-found，最终删除记录、撤销本批授权/凭据、清理按 ID/标签/Service UID 核实的专属容器、卷、私网、Service 和目录发布 Job。

最终 `cleanup-verification.json` 为 destroyed，原历史保留；证据在 C 盘 `argus-plan9/p9-b11c-20261115-01/` 与 `argus-b11c-retirement-20261116/`。完整规则同步到 [卸载文档](../21-host-and-bastion-removal.md)。
