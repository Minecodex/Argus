# B01 实际环境查看入口

状态：**2026-10-27 按用户切换到 B02a 的请求完成退役。以下为历史验证记录，未补记独立人工通过。当前查看 [B02a](./B02a-review.md)。**

运行 `p9-b01-20261026-01`，版本 `local-20261026-b01-review-v3`。以下均为本次实际接入和实际执行产生的结果。

## 登录

- [平台门户](https://platform.argus.dev)：平台管理员 `admin`。
- [企业门户](https://argus.dev)：`test` 企业管理员 `kakj`。
- 邮箱：`kakjzzw@gmail.com`。两组账号密码均使用用户指定值，文档不保存密码；正常 API 和实际门户登录均已验证。
- 旧业务数据与旧会话已清理，使用新账号重新登录。

## 当前保留的场景

- A/B 两台真实 Linux 容器主机、正常 Connector 与 Collector；共 8 项采集均为 fresh/healthy/effective。
- 每台主机关联主机、进程、systemd、Collector 四类受管仪表盘。
- 专属 ServiceAccount 只通过只读角色获得两台主机范围，配置两条真实策略，无外部通知渠道。
- 服务故障和 CPU 负载均已实际触发并恢复；内存分配实测约 181 MiB，文件 I/O 上限 64 MiB，网络通过真实跨主机 HTTP 请求产生。
- 单项停采时 B 继续有数据；A 全关指标时 Connector/Collector 服务均保持 active；随后四项采集已恢复。
- **当前故意保留 A 的 `plan9-web.service` 故障和一条正常 firing 告警，供查看；B 保持正常。CPU/内存/I/O/网络压力任务已结束。**

Docker 主机共享 WSL Linux 内核，CPU/内存/磁盘口径按实际可见范围展示；进程、服务与网卡按容器可见范围观察，不作为独立 VM 硬件证明。

> 以下 B01 资源链接为当时的记录；主机已退役，实时入口可能不再可用。历史数据和本文件末尾的证据仍保留。

## 建议查看顺序

1. 看 A/B 主机资产、范围和观测时间，比较概览与组件列表。
2. 看四项采集的配置/生效版本、健康及最近样本。
3. 查看四类仪表盘并切 A/B，先使用最近 15 分钟，再检查默认 1 小时时间窗及查询错误提示。
4. 看当前专用服务告警，再看 CPU/服务恢复历史。
5. 切中英、明暗，检查标题、图例、表格、错误卡片和跳转。点击面板标题旁的说明图标，检查单值居中、展开、告警表达式、趋势坐标和时间线。

- [B01-Linux-A 主机详情](https://argus.dev/hosts/01a177f0-1c9f-74c2-9e7d-7a386b9f7e50)
- [B01-Linux-B 主机详情](https://argus.dev/hosts/01a177f1-b814-76a1-b2cf-bfd2629e53e4)
- [Collector · 主机 A](https://argus.dev/dashboards/258c6833-6dce-5240-bddf-81bbf7c6f2c2?resource=01a177f0-1c9f-74c2-9e7d-7a386b9f7e50)
- [Host · 主机 A](https://argus.dev/dashboards/cee08ea9-0beb-5c7b-a006-e27d11e61d39?resource=01a177f0-1c9f-74c2-9e7d-7a386b9f7e50)
- [Host processes · 主机 A](https://argus.dev/dashboards/c270e46c-5b78-5c92-8f5f-ea3b1009ff44?resource=01a177f0-1c9f-74c2-9e7d-7a386b9f7e50)
- [Linux system services · 主机 A](https://argus.dev/dashboards/526b50e6-f324-5727-994f-cbdf0814881c?resource=01a177f0-1c9f-74c2-9e7d-7a386b9f7e50)
- [当前服务告警](https://argus.dev/alerts?tab=active&policy_id=8416e50e-6a46-58cb-8c44-2345cc8fb285)
- [服务历史](https://argus.dev/alerts?tab=history&policy_id=8416e50e-6a46-58cb-8c44-2345cc8fb285)
- [CPU 真实负载恢复历史](https://argus.dev/alerts?tab=history&policy_id=e4b825b1-5a43-5a39-93a1-fbdb997f8d73)

## 当前自动复验

- 账号保持原设定，企业 `test`、管理用户 `kakj`；平台标准健康 20/20，通过。
- A/B 两台主机、8 项采集正常；资源、策略、历史和 6 个 PVC 的身份保留。
- 15 分钟和默认一小时分别执行 8 组真实 API 查询，各 40 个面板全部 success 且有数值，共 80 次检查通过。
- 门户 6 项 E2E 通过：中英/明暗四组合及 1280/1920 桌面宽度；包括面板/展开说明、数字位置、查询完整性和告警详情/时间线。
- 实际关闭 A 全部四项指标后，两条策略均暂停评估，无新增 NoData，B 保持新鲜；恢复后八项采集正常、两条策略 normal。
- [修复清单](./B01-issues.md)与[统一修正记录](../07-dashboard-and-alert-review-fixes.md)包含范围和证据。此前失败记录作为历史保留，人工验收等待用户重新查看。

## 本次实际时间线

以下均为 Asia/Shanghai。

| 时间 | 实际事件 | 对象/负载 |
| --- | --- | --- |
| 2026-10-26 17:13:19 | service_failed | host-a plan9-web.service |
| 2026-10-26 17:13:20 | service_recovered | host-a plan9-web.service |
| 2026-10-26 21:31:34 | service_failed | host-a plan9-web.service |
| 2026-10-26 21:32:13 | service_recovered | host-a plan9-web.service |
| 2026-10-26 21:35:33 | load_started | host-a cpu |
| 2026-10-26 21:38:12 | load_started | host-a memory |
| 2026-10-26 21:39:18 | bounded_load_completed | host-a memory |
| 2026-10-26 21:39:19 | load_started | host-a disk |
| 2026-10-26 21:40:25 | bounded_load_completed | host-a disk |
| 2026-10-26 21:40:25 | load_started | host-a network |
| 2026-10-26 21:41:32 | bounded_load_completed | host-a network |
| 2026-10-26 21:43:26 | service_failed | host-a plan9-web.service |
| 2026-10-26 21:43:52 | review_fault_retained | host-a plan9-web.service |

历史负载结束后，查看对应绝对时间窗口；当前仍可直接查看保留的服务故障。用户反馈后在本批继续处理；用户明确通过并要求下一批后才清理和推进。

## 证据

- [当前真实状态](../../../artifacts/plan9/p9-b01-20261026-01/live-handoff-state.json)
- [故障/恢复与启停记录](../../../artifacts/plan9/p9-b01-20261026-01/scenario-checks.json)
- [当前自动复验结果](../../../artifacts/plan9/p9-b01-20261026-01/checks.json)
- [真实门户 E2E 结果](../../../artifacts/plan9/p9-b01-20261026-01/browser-review.json)
- [Compose 与操作说明](../runtime/B01/runbook.md)
