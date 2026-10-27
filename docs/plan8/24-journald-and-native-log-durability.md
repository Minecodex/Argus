# Journald 与原生日志发送行为

状态：Linux 原生数据、真实 systemd 单元过滤门禁通过；正式主机接入与跨平台整体验收尚未全部完成。没有据此宣布 T08 完成。

Journald 使用发行包内的原生 `journaldreceiver v0.158.0`，调用目标系统已有的 `journalctl`，不安装独立 exporter，不 fork Receiver。监控目录开放规则创建；应用业务规则必须指定服务单元并绑定应用服务，系统规则归属主机。单元名称规范化后检查交集，`orders` 与 `orders.service` 不能重复采集。没有 `journalctl` 或读取权限时，原生启动失败应表现为实际配置失败，不能制造样本或标健康。

配置明确设置 `priority: debug`，避免上游默认的 info 门槛忽略调试日志。发送队列使用持久化并在满时施加背压。2026-10-21 按用户决定，当前日志接收及出口重试改为单批最多五分钟，并记录故障与缺口，见 [60](./60-bounded-retry-and-delivery-gaps.md)；下方旧证据仍保留原运行时点。业务配置保持 info 级诊断，避免调试路径输出日志正文。

检查锁定版本的 Stanza 代码发现，其默认发射器仍有内存批处理。仅移除 Argus 配置中的 batch 不能解决该层提前确认。发行构建现在复制版本受控的 `deploy/otelcol/runtime-defaults.go.tmpl`，通过上游 feature gate 启用 `stanza.synchronousLogEmitter`，不修改上游实现。模板本身进入所有平台的制品输入指纹；旧制品缓存不能沿用。该开关属于上游 Alpha 能力，必须随锁定版本执行故障验证，不能将一次测试扩大成所有故障情况下的严格一次交付保证。

## 证据

- [发行构建](../../artifacts/plan8-synchronous-collector-build.log)与[实际开关状态](../../artifacts/plan8-collector-runtime-feature-gates.log)：Linux 二进制默认启用同步发射；Windows/ARM 仍需重新构建和适用验证。
- [制品指纹](../../artifacts/plan8-collector-runtime-fingerprint.log)：组件源码或运行默认值改变都会使缓存失效。
- [实际 Journald](../../artifacts/plan8-journald-native-outage-current.log)：独立拥有的 Linux 容器运行系统 journald，实际写入 info/debug 日志，经 `journalctl`、原生 Receiver、持久发送队列交付；接收端故障期间强制 KILL，重启后游标与队列续传，三条标记各到达一次。接收端为认证测试服务，不是正式 Argus 后端。
- [真实 systemd 单元过滤](../../artifacts/plan8-journald-systemd-units.log)：独立容器在 private cgroup namespace 内运行 systemd，创建两个实际服务单元；原生规则只收到指定业务单元，另一单元没有进入采集。容器不挂载可写主机目录、服务目录或主机 cgroup 树，结束后按所有权清理。
- 初次测试在写入标记后才 flush runtime journal，导致系统自身产生不同游标的两条相同消息；改为写入前完成持久存储准备。失败记录 `plan8-journald-checkpoint-records.log` 保留，未用正文去重隐藏故障。

新二进制的[文件读取、路径保护及真实后端联合回归](../../artifacts/plan8-synchronous-native-log-regression.log)通过：8 条文件日志、系统/业务各 12 条 mTLS/Kafka/Writer/ClickHouse 恢复。剩余门禁包括目标主机依赖诊断、正式下发/回滚与后台链路、K8s 日志同步发射回归，以及 Windows Event Log 实机。当前原生接收器的崩溃边界仍按适用场景实测，不能承诺任意指令位置的无损或跨接收端严格去重。
