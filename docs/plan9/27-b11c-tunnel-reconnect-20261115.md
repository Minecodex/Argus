# B11c 隧道重连与凭据轮换验证

运行 `p9-b11c-20261115-01`，当前阶段与人工结论以 [progress.json](./progress.json) 为准。B11c 仍属于 B11，完成并交用户查看后才进入 B12。

## 发现与修复 B11c-D01

真实 `executor_tunnel` Host 的独立 SSH、平台回调均成功，但重连预览返回 `CONNECTION_TEST_REQUIRED`。只读核对确认主体、地址、端口、平台、Host Key、凭据版本、过期时间与回调都一致。

根因：`validReconnectTest` 直接哈希 `connection_tests.request_plan` 的数据库返回字节。该列是 PostgreSQL `jsonb`，会调整字段顺序与空白，与写入前 JSON 字节不同。安装流程原本会先解码完整类型，再用与写入端相同的序列化验证，因此安装成功而重连失败。

修复将完整 `ConnectionTestPlan` 和 `DecodeConnectionTestProof` 提取到资源领域，供安装与重连共享。仍校验完整计划哈希、解析结果及解析 IP 一致性；重连继续校验主体、凭据版本、Host/Connector/固定目标、Host Key、检测时效及真实回调。没有取消哈希、扩大权限或跳过预览。

- Go 资源与 Connector 包测试通过，补充 JSON 字段顺序/空白变化和未重新签名的回调地址篡改拒绝。
- 独立临时 PostgreSQL 18.6 上完成真实创建、持久化、检测完成与校验，验证数据库往返后的证明可用于重连。临时容器和 tmpfs 数据已删除，未修改正式业务数据。
- 首次只更新 server，预览已通过但旧 worker 执行仍返回 `ACTION_INVALIDATED`；保留该失败执行 `01a1dd87-143d-7cc4-a7d5-b67acd9e8387` 后同步发布 server/worker。
- 最终后端镜像 `local-20261115-b11c-proof-r2@sha256:d2094c8b39a842b728fb28b1d79a150ad5b5ec1c7002a525b29e47d3cd6f8f65`；其余二进制与 Web 内容保持原制品。正常 Helm 升级、node containerd 摘要预载和 rollout 均完成；六个正式 PVC 及所有 Deployment UID 保留。没有新增数据库迁移。

## 原失败和历史数据缺口

修复和重新发布期间的数据中断超过 Collector 原生 300 秒重试预算，产生不可回补的历史缺口：Root 丢弃 **2562** 指标点，Member 丢弃 **448** 指标点。未重置持久投递实例、未清零计数或伪造回补，记录在 `delivery-maintenance-gap.json`。此历史缺口不能写为“全程零丢失”。

修复后在原两台 Host 上再次完整执行 OS 密码与 Secret/Credential **v2→v3** 轮换：旧 v2 预览返回 `PENDING_ACTION_INVALIDATED`，新 v3 预览与后台执行成功，原 Host/Connector/Collector 和 Host Key 保留，八项采集恢复新鲜；本次完整轮换没有新增投递丢弃。

本项代码故障已修复，历史缺口永久保留为本次失败证据。后续断线、告警与持续投递检查比较同一持久投递实例的增量，同时保留上述绝对历史计数。

## 真实权限与期限

正常网页登录后打开真实 PTY，撤销本批精确授权，原会话进入 `invalidated`，旧会话票据拒绝；恢复授权供用户查看。没有创建额外人工用户或全局放开远程能力。

真实停采预览于北京时间 **12:32:50** 创建，原生到期 **12:47:50**；服务端 Date 为 **12:47:59** 时确认得到 `PENDING_ACTION_INVALIDATED`，待确认对象已是 `expired`，没有执行，成员采集仍启用且版本不变。未改时钟、TTL、数据库时间或授权版本来冒充自然过期。

## 路由与故障语义

Root 的控制 SSH 回环 18443/19443 与遥测 SSH 回环 14317/14318 使用独立 SSH 子进程。测试只终止前者并短暂拒绝新入站 SSH，保留 SSH 服务、遥测监听和自动恢复定时器。

成员 `bastion_tunnel` 由 Bastion Connector 管理，其数据隧道会受堡垒机控制租约影响。不能把“Root 数据转发仍在”推断为“成员持续上报”。检查分别保存两台主机的控制状态、原生监听、样本时间和队列，恢复后再核对全部八项来源；未将部分来源新鲜误算为全链路正常。

业务服务失败与 Root Collector 暂停分别实测：前者触发成员业务告警并核对 Root=0、Member=1 的不同资源身份；后者两台控制仍在线，触发真实 `no_data` 健康事件，恢复采样后结束。未发送外部通知。

运行包见 [runbook](./runtime/B11c/runbook.md)，最终页面与实际绝对时间见 [用户查看路线](./batches/B11c-review.md)。原失败、自检与后续复验均在 C 盘本批目录保留；人工通过仍待用户确认。
