# B13c 查看与运行记录

2026-11-18，`p9-b13c-20261118-01` 的撤权、轮换和生命周期真实自检通过，用户确认验收完成。Orders 主机、Orders 应用和共享 Redis 保留供查看。Reports 主机已卸载并删除，Reports 应用和 Memcached 目标已归档。

## 平台查看路线

1. 在「中间件（监控）→ 监控目标」查看 `B13c-Shared-Redis`。点击名称可查看采集实例。目标的归档操作集中在行末的更多菜单，服务端预览检查活跃采集与应用、仪表盘关联。[中间件（监控）](https://argus.dev/monitoring)
2. 查看仍在线的 `B13c-Orders-Host`，包括主机指标和管理员终端。当前共有五项健康、新鲜的采集实例。[Orders 主机](https://argus.dev/hosts/01a1ed2b-8cb8-7a7a-90aa-ed9884b860ec)
3. 查看轮换后重新发布的 Redis 个人副本。归档个人副本时其旧发布预览失效，受管 Redis 模板仍能执行 11 个面板。恢复副本后重新取得基线，再发布保存的改动。[B13c Redis 个人副本](https://argus.dev/dashboards/6f12900f-7a94-46f7-a284-1cfa64f55b7e?resource=a9727bf0-2a82-5799-bc45-35f6f43e6d51&resource_type=monitoring_target)
4. 使用固定的北京时间 14:02–14:14 查看 Memcached。归档前、归档后和 Reports 代理卸载后，9 个历史面板的结果一致。[Memcached 归档历史](https://argus.dev/dashboards/f8cd2937-1919-5871-a126-d3d6166aaf0c?resource=cbf1d464-d179-5bc4-8966-929aa02b31bd&resource_type=monitoring_target&time_from=2026-11-18T06%3A02%3A35.014202%2B00%3A00&time_to=2026-11-18T06%3A14%3A35.014202%2B00%3A00)

## 验收结果

| 项目 | 实际结果 |
| --- | --- |
| 撤权与控制组 | 同一成员预先打开 Reports 图表、查询、下载 733 字节的真实告警 CSV，并用非 root 账号建立 OpenSSH 终端。管理员移除 Reports 主机授权后，旧查询和下载返回 `AUTHORIZATION_VERSION_STALE`，图表清除，终端约 0.94 秒关闭。重新登录后仅保留 Orders 对应的查询和导出记录。 |
| SSH 轮换 | 实际修改本批主机密码与平台 Secret。旧密码和旧预览被拒绝，新密码重新认证成功。Host、Connector、Collector 和 SSH 主机密钥身份一致。 |
| Redis 轮换 | 本批 Redis 重启加载 ACL，平台替换采集专用凭据。同一个目标和采集实例取得新鲜指标，旧密码被真实拒绝，两个应用恢复请求。轮换过程包含短时重启。 |
| 采集与资源启停 | 主动停采时 Memcached 与应用仍正常。Memcached 实际停止时采集配置仍启用，超过新鲜度窗口后读数标记过期。原容器重启后采集恢复。 |
| 归档与删除 | 门户预览拒绝存在依赖的目标，解除本批依赖后通过门户归档。Reports 应用归档，远程授权和凭据已关闭，实际 Connector/Collector 卸载验证成功，随后删除主机记录。Reports 和 Memcached 容器已停止并登记为本批保留的停用现场。 |

共通过 7 项真实浏览器 E2E，覆盖桌面 1280/1440/1920、中英、明暗、页面归档和只读授权。正式健康检查 20/20，保留对照 Collector 连续 120 秒运行且投递失败计数无增长。六个正式 PVC 和 Deployment UID 已核对。B13b 专属环境已退役，验收证据和历史已封存。

运行证据位于 `C:/Users/23574/.codex/artifacts/argus-plan9/p9-b13c-20261118-01`。原始 `handoff.json` 汇总实际链接、部署摘要和终端关闭耗时。分项见 `revocation-acceptance.json`、`ssh-rotation.json`、`redis-rotation.json`、`collection-independence.json`、`dashboard-lifecycle.json`、`portal-target-archive.json`、`reports-decommission.json`、`archival-history-after-uninstall.json` 与 `browser-review/`。账号、Secret 和原始诊断保存在受限目录。

实施差异、失败保留和部署路径见 [B13c 实施记录](../38-b13c-revocation-rotation-and-lifecycle-20261118.md)。
