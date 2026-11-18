# B13b 查看与运行记录

用户指定继续 B13c 后，B13b 专属环境已退役。告警、投递和失败证据已封存，原先后台评估配置恢复。当前环境见 [B13c 查看路线](./B13c-review.md)。以下链接记录原验收时点。

2026-11-18，运行号 `p9-b13b-20261118-01`。**已部署、自检通过、保留现场等待人工查看**。B13c 未启动，Plan9 未标记全部完成。用户“下一轮”授权封存和退役 B13a，不把其三个已知问题或人工验收写成已通过。

## 查看顺序

1. [当前 NoData 示例](https://argus.dev/alerts?tab=active&policy_id=d46838c2-e88f-5ffc-87e2-09b5f39c6772)：只有一条明确命名的 `B13b deliberate empty metric selection`。它刻意匹配不存在的指标标签，用来显示“无数据”，不是 Collector 故障、不是数值 0；`phase=health` 不匹配本轮通知路由，不向外发送消息。
2. [真实服务告警历史](https://argus.dev/alerts?tab=history&policy_id=e5fe246c-6969-581f-880d-c99176abc200&instance=8531689d-f00a-4981-a341-798fcccda1a9)：查看独立规则的首次触发、确认/指派/评论、恢复及时间线；当前服务均已恢复。独立、AND、OR 长故障共六个实际 Episode，短故障仅 Pending，没有触发。
3. [本地 TLS 通知与尝试记录](https://argus.dev/alerts?tab=deliveries&channel_id=9ad0a6dc-e44e-51fc-9b60-7c80dc764398)：首次通知有一次真实 SMTP 451，再实际重试取得 250；同一告警正文保持一致，每次物理发送批次号允许不同。消息分组、成员和恢复投递均留存。接收人只有本批收件器 `b13b@plan9.invalid`，不代表公网机器人或真实邮箱联通。
4. [AND 多指标策略](https://argus.dev/alerts?tab=policies&policy=c1be8364-39d1-5373-a9b9-66fc36914a0f)：失败状态与真实内存利用率组合；第二个内存条件使用 5% 作验证门槛，99% 条件作 AND 负控，不是生产阈值建议。
5. [屏蔽与抑制](https://argus.dev/alerts?tab=noise)：查看 `B13b Reports maintenance window` 与主事件抑制子事件规则。两者均已禁用；历史中保留实际 muted/inhibited 记录。Reports 被静默时 Orders 三条控制事件仍投递，解除后新故障四条事件正常投递。
6. [通知配置](https://argus.dev/alerts?tab=channels)：本批单独 SMTP 凭据、路由和分组。后台评估账号独立于通知凭据，曾实际撤销其 Reports 数据授权：评估被拒绝，管理员读取及显式本地协议测试仍可完成；随后仅恢复原本批授权。
7. [主机](https://argus.dev/hosts)及[应用](https://argus.dev/applications)：`B13b-Orders-Host`、`B13b-Reports-Host` 在线；Orders/Reports 应用分别为 production/staging，真实 HTTP 请求访问本批 Redis/Memcached。两应用共享一个 Redis 身份。默认仪表盘 60 面板均取得真实数据。管理员已配置两主机限定的七天 Shell/SSH 授权；终端全流程属于既有 B11/后续 B13c 验证，不以本次配置代替该验收。

两个逻辑主机共享 Docker Linux 内核，CPU/内存不能相加解释为独立物理容量。HTTP 流量和收件器最多八小时，故障都有自动恢复；当前所有故障计时器已清理，服务正常，只有上述空匹配 NoData 供查看。

## 实测与门禁

- 30 秒连续固定评估槽、20 秒恢复窗口；独立/AND/OR 与负控、短脉冲、重复评估去重、处理和旧处理版本拒绝均通过。
- 真实静默、同主机抑制、解除后的新周期通过；不删除指标或改成健康状态来冒充降噪。
- NoData 与合法 PromQL 的运行期多对多 Error 都没有伪造零值/成功时间；主动关闭 Memcached 采集得到 `not_evaluated / ALERT_COLLECTION_DISABLED`，恢复后同一目标得到新鲜实际样本。
- 独立只读用户正常登录，只能读取 Orders Episode；响应快照不含 Reports 身份，显式请求 Reports 详情被拒绝。后台评估权限、通知租约和路由各自验证。
- 7 项真实浏览器 E2E 覆盖 1280/1440/1920、中英、明暗、当前/历史/策略/通知/降噪与只读；严格 TLS，无 mock/重放、无模型调用。
- 独立临时 PostgreSQL 上告警领域及新回归通过，容器核对归属后清理。OpenAPI 隔离生成检查、企业门户类型检查、批次目录隔离测试通过。
- 当前两台主机 10 项采集健康新鲜，累计终态丢失/拒绝/入队失败均为 0；120 秒观察无增长、原实例 ID 和计数未重置。旧 B13a 的 2,210 指标点缺口仍保留，不能据此回写成当时零丢失。
- 凭据已迁出 `/root`，父目录权限 700，ProtectHome 保留；没有此前凭据挂载的 filesystem 警告。进程恰好退出时的原生采样竞争另行保留：`{'orders': 1, 'reports': 0}`，不宣称所有原生日志零错误。
- 正式健康 20/20，六个正式 PVC 和 Deployment UID 保留，Helm revision 113。后台精确镜像：`host.docker.internal:5001/argus/argus-backend:local-20261118-b13b-immutable-state-v4@sha256:d4f60f4be2cd8329201a0777488f51a42998b26ac438d3f495e99ba366e31aa1`。未新增数据库迁移、未改变全局 CA 或 URL 安全策略。

## 本轮修复及失败记录

见 [B13b 实施记录](../37-b13b-alerts-scope-and-notification-20261118.md)。修复了凭据级 CA、策略标签未进入路由、展示裁剪状态误写回及 RawMessage 缓冲区复用覆盖策略。原先评估写回失败/长期 Pending 的日志、失败任务和超时记录保留，修复后在相同实际主机上重新触发/恢复/静默/抑制通过，没有清表制造通过。

另有接入脚本目录隔离错误：新容器启动被旧库存检查拦住，但 B13a 初始构建源码哈希表曾被准备步骤覆盖；运行镜像 ID 依据原 `resource-map` 和实际缓存镜像重核验，不伪造恢复该源码哈希表。B13a 的业务、告警、数据、截图和报告历史保留，失败副本位于 `preflight-path-overlap/`。目录归属与 retired 写保护已补回归。

测试脚本的初次阈值/恢复就绪/SMTP 批次断言错误及 Windows Node bundle 崩溃也保留；后续门禁实际通过。上述历史失败不标成“本轮全程零报错”。

## 证据与后续

证据根：`C:/Users/23574/.codex/artifacts/argus-plan9/p9-b13b-20261118-01`。主要文件：`handoff.json`、`core-acceptance.json`、`handling-acceptance.json`、`grouping-retry-acceptance.json`、`noise-acceptance.json`、`health-acceptance.json`、`explicit-stop-acceptance.json`、`evaluator-permission-acceptance.json`、`delivery-watch.json`、`current-state.json`、`native-collector-log-check.json`、`formal-health.json`、`template-release.json`、`postgres-tests.log`、`contracts-check.log`、`frontend-typecheck.log`、`browser-chart-final.log` 和 `browser-chart-final/` 截图（已等待真实趋势 SVG 渲染）。密码、会话、服务端私钥和原始诊断放 ACL 限定的 `private/`，不进仓库。

B13a 专属容器/卷/网络/遥测入口已清理，应用/副本/目标归档、测试用户禁用及授权移除；原账号、模型、Workspace 和历史继续保留。B13b 环境等待人工验收，B13c 的门户目标归档入口、完整撤权/轮换/生命周期继续未执行。
