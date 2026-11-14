# B11a 主机直连与远程终端查看记录

运行标识：`p9-b11a-20261113-01`。企业 `test`，使用现有企业账号 `kakj` 正常登录。B10c 已由用户明确通过并清理；本批主机保留运行，人工验收待用户查看。

## 查看路线

主机列表已按新反馈移除控制路径/最近心跳，改为 CPU、内存、磁盘异步趋势；字段在详情保留。实现和真实页面记录见[本轮列表调整](../24-b11a-host-list-trends-20261113.md)。

1. [B11a-Linux-Direct 主机](https://argus.dev/hosts/01a1d539-af94-7a20-befd-47b6f8f763f3)：查看真实资产、在线状态及连接配置。通过真实 SSH 连接测试与安装，Host 为 `direct`。
2. **终端与会话**：选择 `Shell / PTY` 或 `OpenSSH`，登录账号选择 `root`，填写事由后建立会话。Shell 的实际执行身份是 `argus-connector`，OpenSSH 的实际身份是 `root`；可执行 `hostname`、`id`、`ps -eo comm | head`。测试环境不需要额外开启全局远程访问规则。
3. **组件与采集**：Collector 自身、主机基础、进程和 systemd 服务四项均已启用，10 秒采样，数据 healthy/fresh。Windows 服务采集不适用于本批 Linux，不启用。
4. **仪表盘**：已接好下表四套默认仪表盘，均锁定当前主机并实际查询，合计 20 个非空面板。
5. [远程会话](https://argus.dev/remote-sessions) → **历史会话**：查看事由以 `B11a real Shell` / `B11a real SSH` 开头的记录，列表 `…` → **查看录像**。录像可播放，后台按默认策略加密保存。验证会话均已结束，不留活动终端。
6. [告警历史](https://argus.dev/alerts?tab=history&policy_id=86946b52-3808-5b22-85de-5a88cd454196)：查看 **B11a direct host service failed**，点开详情检查真实曲线、阈值、恢复状态和时间线。
7. [企业审计](https://argus.dev/settings/audit)：查看本批正常主机接入、配置确认、远程访问请求、会话建立、凭据使用及录像读取记录。

| 仪表盘 | 面板 | 实际内容 |
| --- | ---: | --- |
| [Host](https://argus.dev/dashboards/cee08ea9-0beb-5c7b-a006-e27d11e61d39?resource=01a1d539-af94-7a20-befd-47b6f8f763f3) | 6 | CPU、内存使用率/用量、文件系统、磁盘读写、网络吞吐 |
| [Host processes](https://argus.dev/dashboards/c270e46c-5b78-5c92-8f5f-ea3b1009ff44?resource=01a1d539-af94-7a20-befd-47b6f8f763f3) | 4 | 本机真实进程 CPU、内存、虚拟内存、I/O |
| [Linux system services](https://argus.dev/dashboards/526b50e6-f324-5727-994f-cbdf0814881c?resource=01a1d539-af94-7a20-befd-47b6f8f763f3) | 4 | 活跃/失败服务、服务 CPU、重启次数 |
| [Collector](https://argus.dev/dashboards/258c6833-6dce-5240-bddf-81bbf7c6f2c2?resource=01a1d539-af94-7a20-befd-47b6f8f763f3) | 6 | 真实进程内存、堆、运行时长、队列、容量与发送率 |

## 实际场景与告警窗口

主机运行 `plan9-b11a.service` HTTP 服务，低频真实请求每 15 秒一次，后台负载最长 8 小时，服务本身持续运行。

实际创建启动失败标记并重启服务，原生 systemd 进入 failed，指标 0→1，告警持续命中后触发；移除标记并启动真实服务后，指标恢复为 0，告警恢复。

- 触发：**2026-11-13 22:34:10 +08:00**（14:34:10Z）。
- 恢复：**2026-11-13 22:34:40 +08:00**（14:34:40Z）。
- 规则阈值 `> 0.5`，持续和恢复窗口各 20 秒，评估周期 10 秒。
- 当前测试故障已恢复，未配置外部邮件/消息通知。查看历史趋势时可选择 **2026-11-13 22:25～22:45**。

## 已知问题 B11a-D01：启动期历史投递缺口

当前告警中保留一条 **Collector delivery data gap**：连续启用采集的初始配置切换阶段，原生出口返回 `PermissionDenied / SOURCE_REJECTED` 并永久丢弃 **1999 个指标点**，最后失败时间为 **2026-11-13 22:30:03 +08:00**。原始 Collector 日志和持久状态可交叉核对；无法精确归属丢失点的具体目标或时间序列。

这不是服务故障告警，也不是持续无数据。四项配置收敛后实际样本持续到达。`delivery-watch.json` 单独记录后续观察期间是否新增损失；它不能补回历史点，也不能证明配置切换问题已修复。本轮没有删除、确认或伪造恢复这条缺口告警，配置切换的完整根因修复仍作为已知问题保留，供用户决定后续处理。

## 真实路由与验收范围

本批是实际 Linux 用户态主机环境，SSH 安装、Collector 注册、原生指标、systemd 失败/恢复、远程 PTY 和录像均真实执行；没有写数据库制造资源或注入指标。

Collector 的 `direct_argus + direct` 使用原生 mTLS 4317/4318，不经过 Executor Tunnel 或堡垒机。旧主机安装包已按[签名制品修正记录](../23-b11a-direct-host-artifact-20261113.md)更新；已保留旧发行记录和 ARM/Windows 制品。

该 systemd 容器共享 Docker VM 内核。Host 内存、磁盘和 CPU 计量按当前可见主机范围解释，不能把 1280 MiB 容器内存上限当成 hostmetrics 的整机容量。普通终端授权有效至 **2026-11-20 22:31:41 +08:00**；之后需要正常延长本批授权。

## 证据与后续

运行文件：[B11a 运行包](../runtime/B11a/runbook.md)。实际产物：`C:/Users/23574/.codex/artifacts/argus-plan9/p9-b11a-20261113-01/`。

最终证据入口为 `checks.json`，包含四类新鲜采集、20 个非空面板、真实浏览器门禁、录像/审计、告警触发恢复和正式平台保留检查。`browser/` 保存正常登录后的中英、明暗页面和真实终端/回放画面。正式平台健康检查与六个 PVC 的 UID 保留单独核对。

用户确认并要求下一批后，再按归属清理 B11a；不会自动部署 B11b，不删除本批历史录像、审计或告警。
