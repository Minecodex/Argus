# B11b 堡垒机与私网成员查看记录

> 2026-11-15：本批已随[仅保留账号的全局维护](../../local-account-only-redeployment-20261115.md)退役。两个容器、专属卷和网络已删除，后端主机、监控、会话、录像及告警历史均清空。以下是历史记录，资源链接不再作为当前入口；没有将清理指令改写为人工通过。

历史运行标识 `p9-b11b-20261114-01`。B11a 曾按用户“继续下一阶段的验收”封存、卸载并清理；未把推进指令改写为显式人工通过。B11b 在退役前自检通过、待人工确认，现已清理。企业 `test` 和账号 `kakj` 已在本次维护中保留。

2026-11-14 录像反馈见[播放器布局与主题修复](../26-terminal-recording-player-layout-20261114.md)。第二轮已发布并通过 8 项真实录像 E2E：默认原始字号、左上角固定，手动适应窗口时保持整段录像相同比例，并修正异步输出与 resize/seek 顺序。B11b 主机及采集保持运行，仍待人工确认。

## 查看路线

1. [主机列表](https://argus.dev/hosts)：分组 **B11b-Bastion-Gateway**，应有堡垒机及一个内网成员，均在线，CPU/内存/磁盘异步曲线可查看。
2. [堡垒机](https://argus.dev/hosts/01a1d825-c5af-75bd-a9af-171331e194b7) 与 [内网成员 B11b-Private-Linux](https://argus.dev/hosts/01a1d829-4ddc-7892-a31a-dc267db0e1c5)：概览查看真实资产和控制路径；成员是 `bastion_relay`，安装执行者为 Bastion Connector。
3. 两台主机的 **组件与采集** 各有 4 项已启用且 healthy/fresh；Collector 分别是 Edge Gateway 和 Leaf。成员遥测为 **bastion_gateway / bastion_tunnel**，不声称 direct 传输。
4. 各自的 **仪表盘** 可打开下表四套默认视图，分别绑定当前主机，合计 8 个视图、40 个有数据面板。
5. **终端与会话**：协议选择 Shell / PTY 或 OpenSSH，账号 `root`，填写事由。Shell 实际身份是 argus-connector，OpenSSH 是 root。可执行 `hostname`、`id`、`ps -eo comm | head`，检查 Dock 底/左/右停靠及缩放。
6. [远程会话](https://argus.dev/remote-sessions) → 历史会话：查找 `B11b member` 或 `B11b bastion` 开头的记录，`…` → 查看录像。实际测试会话已关闭，录像加密保存且可播放，创建/凭据使用/读取有审计。
7. [成员服务告警历史](https://argus.dev/alerts?tab=history&policy_id=0de9728f-32fd-5a81-9146-1e184d509966)：**B11b private member service failed**，已恢复，查看曲线和时间线。
8. [成员无新样本诊断历史](https://argus.dev/alerts?tab=history&policy_id=f775dee2-d10b-5158-a0d1-146feabe922d)：**Query health: no_data**，本次有意暂停采集产生，已经恢复，不是未触发记录或当前故障。

| 成员视图（堡垒机从其详情打开同名视图） | 面板 |
| --- | ---: |
| [Host](https://argus.dev/dashboards/cee08ea9-0beb-5c7b-a006-e27d11e61d39?resource=01a1d829-4ddc-7892-a31a-dc267db0e1c5) | 6 |
| [Host processes](https://argus.dev/dashboards/c270e46c-5b78-5c92-8f5f-ea3b1009ff44?resource=01a1d829-4ddc-7892-a31a-dc267db0e1c5) | 4 |
| [Linux system services](https://argus.dev/dashboards/526b50e6-f324-5727-994f-cbdf0814881c?resource=01a1d829-4ddc-7892-a31a-dc267db0e1c5) | 4 |
| [Collector](https://argus.dev/dashboards/258c6833-6dce-5240-bddf-81bbf7c6f2c2?resource=01a1d829-4ddc-7892-a31a-dc267db0e1c5) | 6 |

## 本次实际验证窗口

| 场景 | 实际时间 |
| --- | --- |
| 成员服务失败告警触发 | 2026-11-14 11:59:50 +08:00 |
| 成员服务恢复 | 2026-11-14 12:00:20 +08:00 |
| 成员控制离线，堡垒机仍在线 | 2026-11-14 12:07:37 +08:00 |
| 控制恢复后的新数据核验 | 2026-11-14 12:07:48 +08:00 |
| 有意采集暂停触发 no_data | 2026-11-14 12:08:20 +08:00 |
| 恢复采集后的 no_data 恢复 | 2026-11-14 12:08:30 +08:00 |

建议查看 **2026-11-14 11:50～12:15** 的历史窗口。现在两个控制通道、服务和采集均已恢复，当前本批活跃告警为 0。未向外部邮箱、群机器人发送测试通知。

服务故障时同一查询返回堡垒机资源值 0、成员资源值 1，证明 Gateway 没有覆盖 Leaf 身份。控制离线与数据无新样本分开记录：不能用父堡垒机在线推断成员在线，也不能把 PromQL 的评估点或配置收敛状态当作刚产生的新样本。

## 实际隔离与权限

私网 `172.28.114.0/24` 为 Docker internal 网络；成员仅有私网接口，没有宿主机端口映射。原生探测显示，成员无法直接连接平台 HTTPS、Gateway 或 Ingest；同一地址和端口从堡垒机可连接。安装、控制及遥测均通过已有正常平台能力，不写数据库造状态或注入指标。

本批为两个真实 Linux 用户态/systemd 环境，共享 Docker VM 内核。CPU/内存/文件系统按 Collector 可见范围解释，不是独立 VM 或物理机容量，也不宣称防御特权容器逃逸。

终端授权仅对本批两台主机、两个托管账号和当前用户生效，有效至 **2026-11-21 11:58:55 +08:00**。录像和命令审计 required，文件上传、剪贴板、端口转发、会话共享保持 disabled，未为演示扩大权限。

## 证据与差异

- 4 项实际中英/明暗浏览器验收通过，包含 1280/1440/1920 桌面、四个真实 PTY、停靠缩放、录像回放和告警时间线。
- 8 项新鲜原生来源、40 个非空面板；两条路径持续观察 120 秒，丢弃、入队失败、接收拒绝全部为 0，未重置计数。
- 正式健康 20 项通过，六个正式 PVC 身份和全部既有部署保留。
- 成员安装的临时目录容量/沙箱失败及原位正常重试均保留，见[实施记录](../25-b11b-isolated-bastion-member-20261114.md)。没有降低签名/尺寸验证或服务沙箱。
- B11a 原有 1999 点历史投递缺口保留，退休时按正常流程确认处理，未声明恢复丢失数据；不计入本批新建 Collector 的计数。

实际产物：`C:/Users/23574/.codex/artifacts/argus-plan9/p9-b11b-20261114-01/`。最终入口 `checks.json`、`final-preservation.json`、`browser-review.json`、`member-isolation.json`、`delivery-watch.json`。运行/归属清理步骤见[运行包](../runtime/B11b/runbook.md)。

用户查看后再修正或进入 B11c，本轮不自动推进或销毁 B11b。
