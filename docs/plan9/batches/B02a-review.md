# B02a 实际查看入口

运行 `p9-b02a-20261027-01`，验收版本 `local-20261027-b02a-v5`。2026-10-27 用户明确反馈“没啥问题切换到 B02b 把”，本批已记为验收通过并完成退役。下文是交付时的历史记录；当前查看 [B02b](./B02b-review.md)。

## 交付时场景（历史）

- 两台独立身份主机，每台一组 Nginx、Apache、HAProxy；六个中间件目标、八项主机固定采集、三类默认仪表盘和三条本批策略。
- Nginx 活动连接、Apache 忙碌 Worker、HAProxy HTTP 5xx 三种告警均已通过实际负载/故障触发和恢复。
- 当前仅保留 **A 的 HAProxy 独立后端 9001 停止**，低频真实请求产生 503 和活跃告警；Nginx/Apache 的 9000 后端与 B 组正常。采集入口仍健康。
- 所有压力任务已结束。低频请求有绝对截止时间，结束后可回看历史，不自动启动下一批。

## 查看顺序

1. 打开[监控列表](https://argus.dev/monitoring)，搜索 `B02a`，查看六项采集的生效版本、健康和新鲜度。
2. 点击各行“默认仪表盘”，比较 A/B；使用最近一小时查看交付前重新生成的连接变化；早期场景按下表绝对时间选择。
3. 查看当前 HAProxy 告警及三条策略的恢复历史，检查资源名称、趋势、阈值与时间线。
4. 切中英、明暗和资源范围；直接描述看到的问题即可。

| 目标 | 默认仪表盘 |
| --- | --- |
| B02a-Nginx-A | [nginx](https://argus.dev/dashboards/4e06bf4d-84aa-5800-a0a8-330ac77d34bb?resource=a0a1257e-b257-5fd0-886f-898c5f66f697&resource_type=monitoring_target) |
| B02a-Apache-A | [apache](https://argus.dev/dashboards/cec7614b-b83b-5e8c-a6e7-09dc853f4473?resource=8f14d324-f2c2-5128-a742-5f156d70f5ab&resource_type=monitoring_target) |
| B02a-Haproxy-A | [haproxy](https://argus.dev/dashboards/18382186-1fa1-51c3-af79-55334b48934c?resource=71f74fb3-3e1e-5806-95f7-ba1f572f968b&resource_type=monitoring_target) |
| B02a-Nginx-B | [nginx](https://argus.dev/dashboards/4e06bf4d-84aa-5800-a0a8-330ac77d34bb?resource=69ce4d50-2c13-5122-99f4-114136c99385&resource_type=monitoring_target) |
| B02a-Apache-B | [apache](https://argus.dev/dashboards/cec7614b-b83b-5e8c-a6e7-09dc853f4473?resource=71d528f4-36bb-5e32-8d88-50057abb1f25&resource_type=monitoring_target) |
| B02a-Haproxy-B | [haproxy](https://argus.dev/dashboards/18382186-1fa1-51c3-af79-55334b48934c?resource=d65c6051-7889-5432-9768-3aedbdf83045&resource_type=monitoring_target) |

- [当前 HAProxy 告警](https://argus.dev/alerts?tab=active&policy_id=5a4595f5-7e76-5491-9f86-216a18ea6192)
- [nginx 恢复历史](https://argus.dev/alerts?tab=history&policy_id=7c15e6c0-2d25-5e4c-805a-da527a40fd8f)
- [apache 恢复历史](https://argus.dev/alerts?tab=history&policy_id=0443a7ce-cdb8-5446-92f4-1ca6096d1f8e)
- [haproxy 恢复历史](https://argus.dev/alerts?tab=history&policy_id=5a4595f5-7e76-5491-9f86-216a18ea6192)

## 实际时间窗口（Asia/Shanghai）

| 场景 | 开始 | 恢复完成 |
| --- | --- | --- |
| nginx | 2026-10-27 08:12:18 | 2026-10-27 08:13:23 |
| apache | 2026-10-27 08:13:23 | 2026-10-27 08:14:32 |
| haproxy | 2026-10-27 08:29:12 | 2026-10-27 08:30:33 |
| nginx 交付前刷新 | 2026-10-27 09:44:29 | 2026-10-27 09:45:42 |
| apache 交付前刷新 | 2026-10-27 09:45:42 | 2026-10-27 09:46:53 |

低频请求截止：2026-10-28 07:53:35。截止后服务和 Collector 仍保留，5xx 速率随请求停止下降；故障与恢复证据在上述历史窗口。

## B02a-D01：真实历史投递缺口

Collector 上报 7 个指标点终止投递失败，发生时间与独立停采切换窗口重合。当前六目标与主机采集均新鲜；原报告不能确定具体目标或丢失样本时间，不能宣称整段时间完全无丢失。平台设计保留这类历史缺口告警，不自动伪造恢复；本批原样保留，供用户查看。
- 发生：2026-10-27 08:30:41；告警 ID `00062046-8c7c-44c3-9f6b-3aac52c52f0c`。

## 自动验证与记录

- 32 个中间件面板查询通过；6 项真实门户 E2E 通过，覆盖中英/明暗、A/B 类型化跳转、资源名称、告警详情及 1280/1920 桌面宽度。
- 三种真实告警均触发和恢复；独立停采暂停评估，其他五目标新鲜，恢复后正常评估。
- 原生 Nginx 计数与采样核对、六目标跨指标隔离、平台健康 20/20、正式 PVC 身份保留均通过。
- B01 两主机/软件/凭据/专属容器/卷/网络均完成退役，当前监控列表不再显示旧 B01 条目；历史记录、公共默认模板和企业账号保留。

证据：

- [当前真实状态](../../../artifacts/plan9/p9-b02a-20261027-01/live-handoff-state.json)
- [自动检查](../../../artifacts/plan9/p9-b02a-20261027-01/checks.json)
- [故障与恢复](../../../artifacts/plan9/p9-b02a-20261027-01/scenario-checks.json)
- [独立停采](../../../artifacts/plan9/p9-b02a-20261027-01/lifecycle-checks.json)
- [真实门户 E2E](../../../artifacts/plan9/p9-b02a-20261027-01/browser-review.json)
- [B01 清理记录](../../../artifacts/plan9/p9-b01-20261026-01/cleanup.json)
- [Compose 与操作流程](../runtime/B02a/runbook.md)
