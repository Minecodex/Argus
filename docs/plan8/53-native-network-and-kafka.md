# DNS、ICMP、Kafka 与 Collector 升级

2026-10-20，接续 [52 审计](./52-completion-audit.md)，实施中。保持内置专用 Receiver、共享采集实例/凭据/范围/模板的架构，不新增 exporter 或外部采集服务。

当前锁定的 dnscheckreceiver 0.158.0 的 factory 返回空实现，仅通过配置校验不能证明能够采集。上游 [0.159.0 factory](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.159.0/receiver/dnscheckreceiver/factory.go)已接入实际 scraper；本次选用正式 0.159.0，将三平台 Collector 的上游组件和自有组件依赖统一升级，发行标识改为 `0.3.0-plan8`。没有只替换一个 DNS 组件或复制上游源码。原 0.158 制品保存在本地验收 artifacts，供证据追溯。

本轮需交付真实 DNS/ICMP/Kafka 目标、独立启停、故障/认证恢复、默认模板、正常化及 Writer/ClickHouse 实际查询，并回归已有 Collector 能力。DNS/ICMP 上游稳定性仍为 development，不能因内置就标记为全面支持；具体版本、平台、权限和拓扑按实际证据记录。

Windows 仅构建及非管理员可执行的配置验证；实机安装、服务、事件日志和 IIS 仍按用户决定暂缓。正式平台门禁、全计划及统一发布的状态不因这次升级自动通过。

## 本轮交付与限定组合

| 对象 | 实际组合及行为 | 默认内容 |
| --- | --- | --- |
| DNS | dnscheckreceiver 0.159.0 → CoreDNS 1.12.4，Linux amd64；UDP/TCP、A 记录、缺失名称、服务停机/恢复、两个独立实例 | `dns.overview`：按域名/记录类型/服务器保留维度的状态及耗时 |
| ICMP | icmpcheckreceiver 0.159.0，Linux amd64；真实容器 IP、丢包/恢复、非 root 且移除 capabilities、ping socket 许可拒绝/恢复 | `icmp.overview`：百分比丢包及平均/最小/最大/抖动耗时；完全丢包时 RTT 为空 |
| Kafka | kafkametricsreceiver 0.159.0 → Kafka 4.2.1 单节点 KRaft；SASL/PLAIN 元数据只读身份、两分区/实际消息/消费组 offset、写入拒绝、错误密码/恢复及停采 | `kafka.overview`：Broker 数、Topic 分区、offset、副本/同步副本、消费 offset/lag/成员 |

Kafka 仅采用原生协议的元数据/消费组指标，不提供 JMX/JVM 进程指标，不部署 exporter。上游说明以 Kafka 2.x/3.x 为前件，本轮 4.2.1 只构成实际具名组合的组件证据，不能扩大成任意 Kafka 4.x 或多 Broker 集群已通过。PLAIN 只在隔离测试网络内验证；TLS/SCRAM 组合另行验证。

DNS 的 TCP-TLS、AAAA/其他记录、不同服务器，ICMP 的 Windows/其他权限方式，以及其余平台和拓扑没有由本轮测试抵扣。DNS/ICMP 上游 development 状态保留。

同一 Collector 的 DNS UDP/TCP 与 ICMP 独立启停，全部停采后控制运行时保持存活。新建继续复用分页 Collector 弹框、自动目标、应用关联、凭据私有通道和统一来源授权。共享选择器同时校验锁定版本、组件、平台和资源类型；旧 0.158 或未验证的其他版本即使声明同名组件也不能显示为兼容，服务端仍独立拒绝。

新增内容进入同一模板注册表与生成客户端，当前共 **27 套受管默认模板**；用户副本和版本更新仍使用既有机制。布局参考 [Grafana 7587](https://grafana.com/grafana/dashboards/7587-prometheus-blackbox-exporter/) 和 [7589](https://grafana.com/grafana/dashboards/7589-kafka-exporter-overview/)，表达式按原生数据改写。门户覆盖中英、明暗、三个新建表单、默认快捷入口和资源范围绑定；截图中的 mock 图表只证明布局，真实取数另有后端证据。

## 分层证据

- [DNS/ICMP 具名记录](../../artifacts/plan8-network-delivery-manifest.json)：真实目标及 Writer/ClickHouse 查询两组，九个正向目标面板；另一份实际全丢包采样验证四个 RTT 面板即时结果为空，避免零延迟误导。
- [Kafka 具名记录](../../artifacts/plan8-kafka-delivery-manifest.json)：实际 KRaft/只读身份/认证恢复/生命周期，以及八个原生数据面板查询两组。
- [本轮汇总](../../artifacts/plan8-network-kafka-final-checks.json)：三平台制品、原生配置、Windows 组件注册、六个独立 Go 模块、工程检查、版本准入及八个门户场景分别记录。完整 harness 当前注册 45 组，本轮没有宣称 full 通过。
- 升级后回归原有 MySQL 双实例/Redis/Nginx、PostgreSQL/HTTP/TCP/TLS、Memcached/Apache/HAProxy、RabbitMQ/ZooKeeper、主机自身/进程/systemd、Journald、文件日志、K8s API/Kubelet/CoreDNS，以及实际 SDK 三信号/Key 轮换和系统/业务日志故障恢复。各范围单列，不抵扣正式接入、部署、模型或发布。

首次 DNS 测试暴露 CoreDNS 镜像文件 capabilities 与 `cap-drop=ALL` 的启动冲突，只给本轮独立 DNS 测试服务保留 NET_BIND_SERVICE；Collector 的非 root/移除 capabilities 边界不变。初次契约生成发生本机 Node/libuv 退出异常，完整重试和最终契约检查另有记录，未把部分生成算通过。首次日志回归缺少显式启用变量而跳过两项，随后单独真实执行，不用 Skip 计通过。

测试容器/网络按 owner 回收，K8s 只创建临时 Namespace、ServiceAccount 和明确只读 RBAC，清理核验 UID；未暂停或修改业务工作负载，没有执行 Windows 管理员脚本或正式发布。

## 告警建议与剩余边界

| 来源 | 建议表达式与解释 |
| --- | --- |
| DNS | `dnscheck_status == 0` 持续若干周期，按域名判断；无样本单列，不视为成功 |
| ICMP | `ping_loss_ratio > 20` 或 `ping_loss_ratio == 100`，持续时间按业务设定；RTT 告警同时限定丢包率小于 100 |
| Kafka | lag 超过业务阈值；`kafka_partition_replicas_in_sync < kafka_partition_replicas` 判断副本落后；零成员是否异常取决于消费方式 |

以上为口径建议，未自动启用生产策略；告警仍需统一授权、预览确认和真实业务闭环验收。SQL Server、Oracle、MongoDB、Elasticsearch、IIS 默认内容及其余服务组合，Namespace 指标/持续同步、采集责任迁移、剩余 K8s/SDK/敏感宿主输入、正式验收和发布继续未完成。11 类资源及 SkyWalking 原生协议暂缓保持不变。
