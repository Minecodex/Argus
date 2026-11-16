# B11c 用户查看路线

运行 `p9-b11c-20261115-01`。状态：**用户已确认通过，2026-11-16 正常卸载并销毁，以下为历史查看记录**。用户原话“没问题，开始下一轮”；当前已进入 [B12](./B12-review.md)。B11c 的会话、告警、录像、失败和原自检证据保留，不将销毁前状态当作当前在线状态。

## 最新执行复查（2026-11-16）

按“执行 B11c”复用原两台主机，重新开启最长八小时的真实 HTTP 低负载；当前八项采集新鲜、八个视图共 40 面板正常。本次四条真实 Shell/SSH、加密录像和中英/明暗/三种桌面宽度检查通过，正式健康 20/20。与上一轮持久计数比较以及本次连续 120 秒观察均无新增丢弃；两条历史缺口诊断继续保留。

11 月 15 日的凭据轮换、自然过期、撤权和故障/恢复记录继续作为原始历史证据，未冒充今天重放。新证据：`C:\Users\23574\.codex\artifacts\argus-plan9\p9-b11c-20261115-01\rechecks\recheck-20261116-01`；四条新会话可在历史会话中按本次时间查看。环境继续保留，B12 未启动，人工验收仍待用户确认。

## 从这里查看

1. 打开 [主机列表](https://argus.dev/hosts)，沿用企业 `test` 的原账号 `kakj` 登录。当前两台都是本轮新建并保留的真实 systemd/OpenSSH 主机；列表 CPU/内存/磁盘渐进加载。
2. 进入 [B11c-Tunnel-Bastion](https://argus.dev/hosts/01a1dd5d-8cba-7244-ba1d-585fc3078343) 和 [B11c-Private-Member](https://argus.dev/hosts/01a1dd60-ab79-76ae-b8a7-42c2953301a4)。查看详情中的实际控制路径与采集路由、采集配置和四套默认仪表盘。
3. 在主机“终端与会话”中选择 `root`、`Shell / PTY` 或 SSH，填写事由后建立会话。可运行 `hostname`、`id`、只读 `ps`，查看底/左/右停靠和调整尺寸。Shell 实际执行用户为 Connector 服务账号，SSH 为 root；不能将账号选择当成 Shell 的 sudo 授权。录像和命令审计必需，未开放文件上传或剪贴板。
4. 打开 [远程会话](https://argus.dev/remote-sessions) → 历史会话，搜索 `B11c`。四条中英/明暗自动验收会话已有真实命令输出与加密录像，可打开回放。撤权测试原会话为 `invalidated`，正常关闭会话为 `terminated`；用户授权已恢复。
5. 打开 [业务告警历史](https://argus.dev/alerts?tab=history&policy_id=54aaf870-589a-5963-8781-5d47724bb821) 与 [数据健康历史](https://argus.dev/alerts?tab=history&policy_id=0e0fd25d-54c1-5e4a-b803-6cdc05939046)。业务和 no_data 测试异常均已恢复；当前列表保留 **两条 Collector delivery data gap 历史缺口诊断**，这是修复前真实丢弃的未处理证据，不能因恢复采样而伪装消失。查看当时趋势请使用下面的绝对时间。没有发送外部通知。

## 八个资源范围视图

| 主机 | 已真实执行的四套仪表盘 |
| --- | --- |
| 堡垒机 B11c-Tunnel-Bastion | [Host](https://argus.dev/dashboards/cee08ea9-0beb-5c7b-a006-e27d11e61d39?resource=01a1dd5d-8cba-7244-ba1d-585fc3078343) / [Host processes](https://argus.dev/dashboards/c270e46c-5b78-5c92-8f5f-ea3b1009ff44?resource=01a1dd5d-8cba-7244-ba1d-585fc3078343) / [Linux system services](https://argus.dev/dashboards/526b50e6-f324-5727-994f-cbdf0814881c?resource=01a1dd5d-8cba-7244-ba1d-585fc3078343) / [Collector](https://argus.dev/dashboards/258c6833-6dce-5240-bddf-81bbf7c6f2c2?resource=01a1dd5d-8cba-7244-ba1d-585fc3078343) |
| 私网成员 B11c-Private-Member | [Host](https://argus.dev/dashboards/cee08ea9-0beb-5c7b-a006-e27d11e61d39?resource=01a1dd60-ab79-76ae-b8a7-42c2953301a4) / [Host processes](https://argus.dev/dashboards/c270e46c-5b78-5c92-8f5f-ea3b1009ff44?resource=01a1dd60-ab79-76ae-b8a7-42c2953301a4) / [Linux system services](https://argus.dev/dashboards/526b50e6-f324-5727-994f-cbdf0814881c?resource=01a1dd60-ab79-76ae-b8a7-42c2953301a4) / [Collector](https://argus.dev/dashboards/258c6833-6dce-5240-bddf-81bbf7c6f2c2?resource=01a1dd60-ab79-76ae-b8a7-42c2953301a4) |

共八项原生采集、八个资源范围视图、40 个非空面板。当前状态以实际采样和刷新时间为准。两台容器共享 Docker VM 内核，系统容量口径不等同于两台物理机；资源身份、进程、服务和隧道均分别验证。

## 北京时间故障与恢复

| 场景 | 实际时间 | 已验证结果 |
| --- | --- | --- |
| 修复后 OS 密码与 Secret/Credential v2→v3 | 2026-11-15 12:54:01～2026-11-15 12:54:47 | 旧 v2 预览拒绝、新 v3 后台重连成功；原 Host/Connector/Collector/Host Key 保留；无新增丢弃 |
| Root 控制 SSH 中断与自动重连 | 2026-11-15 13:01:42～2026-11-15 13:03:04 | Root 控制离线时数据仍推进；成员隧道受控制租约影响；恢复后八项来源均新鲜，无新增丢弃 |
| 成员服务真实退出失败 | 原生故障 2026-11-15 12:56:37；告警 2026-11-15 12:57:10～2026-11-15 12:58:00 | `B11c private service failed` 已恢复；同一查询 Root=0、Member=1 |
| 主动停止 Root Collector | 原生暂停 2026-11-15 12:58:02；告警 2026-11-15 12:58:30～2026-11-15 12:58:40 | 两台控制仍在线，真实 `Query health: no_data` 触发并恢复 |
| 自然过期预览 | 2026-11-15 12:32:50 创建，12:47:50 到期，12:47:59 拒绝 | 没有改时钟/TTL；无执行，成员采集仍启用、版本不变 |

## 已修复问题与保留的历史缺口

本轮发现并修复了 PostgreSQL jsonb 格式化导致重连证明被误拒绝的问题，server 与 worker 已同步部署。修复前的中断超过 300 秒重试上限，Root 历史丢弃 **2562** 指标点、Member **448** 指标点。计数未清零，缺口未伪造回补。修复后重新完成整轮轮换和故障验证，检查无新增丢弃；不能将它表述为全程零丢失。详见 [B11c-D01](../27-b11c-tunnel-reconnect-20261115.md)。

## 自动自检与保留环境

正常登录、严格 TLS 的真实浏览器已通过四条正向会话链路和一条撤权链路，覆盖中文/英文、浅色/深色及 1280/1440/1920 桌面宽度；录像事件包含真实命令输出。正式平台健康 **20/20**；六个 PVC 与 Deployment UID 保留。发布证据明确记录修复导致的镜像/代际变化，不声称正式部署完全未变。

原始证据位于 `C:/Users/23574/.codex/artifacts/argus-plan9/p9-b11c-20261115-01`：`checks.json`、`server-fix-release.json`、`credential-rotation.json`、`expiry-refusal.json`、`connection-recovery.json`、`alert-cycle.json`、`data-pause-cycle.json`、`delivery-maintenance-gap.json`、`delivery-watch.json`、`browser/`、`revocation-browser-r3/`。私有目录不对外分发。

授权截至 `2026-11-22T04:14:03.262854Z`，仅限当前用户、两台 Host、两个托管账号及 Shell/SSH terminal。真实 HTTP 低负载最长八小时，结束后仅停负载，环境继续保留。自动自检不代表用户已接受。

后续由用户确认并要求下一批后按 [运行包](../runtime/B11c/runbook.md) 成员先行退役。借用的 evaluator 账号、角色及共享后台评估设置保留；只撤销本批 Host 数据授权。不会在本轮自动清理 B11c 或启动 B12。
