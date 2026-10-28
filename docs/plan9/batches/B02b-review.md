# B02b 实际查看入口

运行 `p9-b02b-20261027-01`，验收版本 `local-20261027-b02b-v1`。用户于 2026-10-27 明确表示“B02b 没啥问题，清理后切换部署 B03 吧”，本批已验收通过并完成退役。三个容器、三个卷、网络与当前采集无残留；账号、历史及共享评估身份保留。详见[验收记录](../../../artifacts/plan9/p9-b02b-20261027-01/acceptance.json)和[清理记录](../../../artifacts/plan9/p9-b02b-20261027-01/cleanup.json)。当前批次见 [B03](./B03-review.md)。

以下为交付时的场景及历史证据，原故障已解除、测试端点已销毁。

## 当前场景

- 1 台正式接入的探测主机、2 个独立目标容器，10 个探测目标和 4 项主机固定采集。
- 当前保留 **HTTP-A 返回 503**、**TLS-A 使用约 2 天有效期证书**，对应两条活跃告警；其余协议已恢复，B 组保持健康。
- 已实际执行 HTTP 失败、TCP 拒绝连接、临期/过期证书、DNS NXDOMAIN 和 ICMP 100% 丢包，并逐一恢复。
- 过期证书的严格 TLS 校验确实失败，TLS 剩余时间为负；测试没有修改系统时钟。TLS 到期观察不等同于信任校验，HTTPS-B 同时验证实际 CA/主机名信任。

## 查看顺序

1. 打开[监控列表](https://argus.dev/monitoring)，搜索 `B02b`，看十个目标的配置、生效状态、采集健康和新鲜度。
2. 点击“默认仪表盘”，比较 A/B。HTTP-A 当前成功状态为 0、服务端错误为 1；TLS-A 剩余约 2 天、B 约 90 天。
3. TCP/DNS 当前均成功，ICMP 当前丢包为 0；按下表历史窗口查看故障与恢复。100% 丢包期间不将零 RTT 画成快速回复。
4. 查看当前 HTTP/TLS 告警和五条策略的恢复历史，再检查中英、明暗、图表与时间线。

| 对象 | 默认仪表盘 |
| --- | --- |
| B02b-HTTP-A | [http](https://argus.dev/dashboards/44099160-01d1-5e24-af29-e8c36c864726?resource=2f5c9d73-cba2-5df8-9d64-fcb7476e1236&resource_type=monitoring_target) |
| B02b-TCP-A | [tcp](https://argus.dev/dashboards/7f16f2fa-55fc-594f-a180-dd49ebc646e9?resource=d82b9659-ad51-50a8-8cae-6d5e39dcc6ad&resource_type=monitoring_target) |
| B02b-TLS-A | [tls](https://argus.dev/dashboards/0bb57835-cf1e-5ded-8775-d3051169ed75?resource=32f97179-e708-5c14-8094-2d2991d11604&resource_type=monitoring_target) |
| B02b-DNS-A | [dns](https://argus.dev/dashboards/08c73c9c-3a8b-529d-95e0-106db91c4fa0?resource=25b4ea79-efdf-5e0e-9196-ed54cbc45bf6&resource_type=monitoring_target) |
| B02b-ICMP-A | [icmp](https://argus.dev/dashboards/1e8fb4e1-da96-57da-afb3-b04e1308f102?resource=05c2695b-b82a-5f3f-a84f-18ff8b966b26&resource_type=monitoring_target) |
| B02b-HTTPS-B | [http](https://argus.dev/dashboards/44099160-01d1-5e24-af29-e8c36c864726?resource=32097de2-680e-56f5-84e0-20d37cf8a7d1&resource_type=monitoring_target) |
| B02b-TCP-B | [tcp](https://argus.dev/dashboards/7f16f2fa-55fc-594f-a180-dd49ebc646e9?resource=2a4dd331-8fd3-5322-9aac-30e4672fe29d&resource_type=monitoring_target) |
| B02b-TLS-B | [tls](https://argus.dev/dashboards/0bb57835-cf1e-5ded-8775-d3051169ed75?resource=610d2d8e-bf67-56af-ba52-3a6b5dfc7df2&resource_type=monitoring_target) |
| B02b-DNS-B | [dns](https://argus.dev/dashboards/08c73c9c-3a8b-529d-95e0-106db91c4fa0?resource=d478eb6a-626e-5013-a1cf-d1de3f4d951c&resource_type=monitoring_target) |
| B02b-ICMP-B | [icmp](https://argus.dev/dashboards/1e8fb4e1-da96-57da-afb3-b04e1308f102?resource=c9b12f2e-a1c4-5a9a-84aa-0af1248d1eb2&resource_type=monitoring_target) |

- [探测主机](https://argus.dev/hosts/01a17bf3-d269-7f08-8f88-5a57e52601fd)
- [当前 HTTP 告警](https://argus.dev/alerts?tab=active&policy_id=7298d282-8179-5fc8-90cb-b591cffc0563)
- [当前 TLS 告警](https://argus.dev/alerts?tab=active&policy_id=963e9dba-9c6c-5757-8548-1c846db5d074)

## 实际历史窗口（Asia/Shanghai）

| 场景 | 开始 | 恢复完成 | 历史入口 |
| --- | --- | --- | --- |
| HTTP | 2026-10-27 14:29:40 | 2026-10-27 14:30:41 | [告警历史](https://argus.dev/alerts?tab=history&policy_id=7298d282-8179-5fc8-90cb-b591cffc0563) |
| TCP | 2026-10-27 14:30:41 | 2026-10-27 14:31:41 | [告警历史](https://argus.dev/alerts?tab=history&policy_id=593f6f7b-a1f6-5ac4-93d5-610625799756) |
| TLS | 2026-10-27 14:31:41 | 2026-10-27 14:32:52 | [告警历史](https://argus.dev/alerts?tab=history&policy_id=963e9dba-9c6c-5757-8548-1c846db5d074) |
| DNS | 2026-10-27 14:32:52 | 2026-10-27 14:33:52 | [告警历史](https://argus.dev/alerts?tab=history&policy_id=a6183e12-5790-52da-93d9-d826f2c10cec) |
| ICMP | 2026-10-27 14:33:52 | 2026-10-27 14:35:12 | [告警历史](https://argus.dev/alerts?tab=history&policy_id=6af1103b-0ef2-5dbc-9d08-1aea0bd26aa5) |

TLS-A 临期证书真实到期时间：2026-10-29 13:57:36。证书不会自动续期，数值随真实时间下降。低频探测和环境持续保留，等待用户验收后再清理。

## 自动验证与修正

- 五类默认视图、30 个面板均有实际数据；6 项真实门户 E2E 通过，包含中英/明暗、十目标跳转、资源名称、告警详情和桌面宽度检查。
- 五类故障全部真实触发与恢复，B 组对照正常；独立停采 TCP-A 暂停评估，其余九目标继续新鲜采集，恢复后正常。
- HTTP/DNS 默认模板升级到 v2：按稳定检查身份选择最新观测，避免状态码/返回码标签变化让旧值混入当前状态。原始数据与 PromQL 引擎语义保留；隔离 ClickHouse 回归与本批实测均通过。
- 独立停采恢复时一次旧预览被配置版本保护拒绝；脚本等待配置收敛并重新预览后恢复，没有绕过版本或确认校验。
- 本批未观察到新的 Collector 投递缺口告警。B02a 的既有历史提示已在验收退役时确认并保留；不据此宣称任意时间窗口绝对完整。
- 平台健康 20/20；正式 PVC 身份保留；B02a 容器、卷、网络及当前采集条目清理完成，账号和历史保留。

证据：

- [检查结果](../../../artifacts/plan9/p9-b02b-20261027-01/checks.json)
- [当前状态](../../../artifacts/plan9/p9-b02b-20261027-01/live-handoff-state.json)
- [故障恢复](../../../artifacts/plan9/p9-b02b-20261027-01/scenario-checks.json)
- [过期证书严格校验](../../../artifacts/plan9/p9-b02b-20261027-01/tls-expired-verified.json)
- [指标与证书有效期对照](../../../artifacts/plan9/p9-b02b-20261027-01/certificate-comparison.json)
- [独立停采](../../../artifacts/plan9/p9-b02b-20261027-01/lifecycle-checks.json)
- [真实门户 E2E](../../../artifacts/plan9/p9-b02b-20261027-01/browser-review.json)
- [部署与健康](../../../artifacts/plan9/p9-b02b-20261027-01/deployment.json)
- [运行包与清理流程](../runtime/B02b/runbook.md)
