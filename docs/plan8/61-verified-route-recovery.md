# 已确认配置与隧道恢复的一致性

状态：实现及真实隔离 PostgreSQL、相关三个包回归通过；r30 正式验证恢复后新主机样本、文件日志与 Journald 全阶段通过，后续 K8s 业务日志另见 [62](./62-external-read-transaction-boundary.md)。

r29 已正式通过强制 Gateway 故障后的类型化补报：原命令在新连接代次核验成功，配置 6 生效，同版本只有一条写命令，Collector 实际启动一次，证据为 `artifacts/plan8-completion-deployed-r29/plan8-collector-reconnect.json`。但随后无法新建 Journald 规则；实际只读数据库观察为 Collector converged 6/6、隧道 established 且租约属于新连接代次 2、路由仍 degraded，叶子主机没有新样本。本轮最终失败，恢复 passed，原 server 1/1、本轮 Namespace 清零。

原先逻辑只在配置结果或尚未结束的 Action 对账中激活路由。隧道晚于配置证明恢复时，原 Action 已经完成，可信心跳只更新物理隧道，不再恢复逻辑路由。安装成功和隧道建立均不能单独作为数据已恢复的证明。

新增共享 `collectorstate.ActivateVerifiedTunnelRoute`，由 Connector 可信隧道心跳和 Direct Executor 的建立、续租路径调用。SQL 同时核验企业、Collector 当前意图及已验证生效配置、配置哈希、运输类型、隧道 ID/epoch/fence/owner、有效租约、当前凭据版本及密钥状态；Connector 还需有效当前连接代次。只有全部成立才恢复路由并完成适用 Claim 迁移。稳定心跳不反复增加路由版本，不伪造配置结果或采样健康。

真实 PG 场景覆盖未知/安装中/失败/已卸载配置、更新意图、旧 epoch/fence/owner、跨企业、过期租约、禁用或轮换凭据、禁用密钥、未验证配置和重复心跳，证据 `artifacts/plan8-route-recovery-final.log`。它是明确的存储投影夹具，不代表实际 SSH 隧道。

正式故障驱动现将路由与数据恢复纳入最终条件：除了核验原命令和一次实际启动，还要等待控制路径就绪，并观测故障恢复后新的主机指标。下一轮仍须继续 Journald、K8s 与后续全部场景，不因本次组件回归就宣称整体完成。
