# B05b Kafka 真实场景运行包

运行 `p9-b05b-20261028-01`，在用户“下一批”指令后清理 B05a 并部署。切换不替代人工验收结论，历史与企业账号保留。

## 环境与权限

- 独立 Apache Kafka 4.2.1，单节点 KRaft；一个 Broker/Controller、两个 Topic、两个固定消费组。
- `p9-work` / `p9-control` 各 3 分区、1 副本，消费组分别为 `p9-work-consumer` / `p9-control-consumer`。
- `B05b-Kafka-Collector` 通过正常 SSH 流程安装 Connector/Collector；业务客户端使用实际 librdkafka 协议。
- 外部客户端监听仅在 Compose 网络内，使用 SASL/PLAIN 独立随机账号。monitor 只有 Cluster Describe、Topic Describe/DescribeConfigs、Group Describe；业务账号只有自己 Topic/Group 所需权限。
- 内部管理和 Controller 的明文监听绑定 Broker 容器 loopback，宿主只发布主机 SSH `127.0.0.1:22281`。本批不验证 TLS、多 Broker 选主/容灾或 JVM/JMX 指标。

两个容器、两个命名卷、一个专网；CPU 上限合计 2.5，内存上限 2048 MiB，Kafka JVM 堆 128–256 MiB。Broker 以容器内 root 初始化其专属卷，无宿主 Docker socket 或无关目录挂载。主机使用独立 systemd/private cgroup。

业务生产者每 2 秒发送一条，固定轮转三分区，使用 `acks=all` 并等待真实 delivery callback。消费者处理和校验消息后同步 commit，禁用自动提交和自动 offset store。首次位点通过实际消费建立，不调用 reset-offsets 或写提交位点假装已消费。

每次突发最多 240 条；生产者每 10 条检查自身组 lag，达到 1000 停止生产。每进程最长 8 小时。Topic 配置每分区 4 MiB 保留、1 MiB 段及 1 小时保留时间；Kafka 按段和检查周期回收，不把该配置宣称为瞬时精确磁盘硬上限。消息及运行时间另外有界。

## 启动和接入

```powershell
python docs/plan9/runtime/B05b/manage.py prepare
docker build --tag argus-plan9-kafka:b05b-20261028 --file docs/plan9/runtime/B05b/Dockerfile docs/plan9/runtime/B05b
python docs/plan9/runtime/B05b/manage.py up
python docs/plan9/runtime/B05b/manage.py initialize
python docs/plan9/runtime/B05b/manage.py workload

$sessionPath = 'artifacts/plan9/reset-20261026/private/enterprise-session.json'
$caPath = 'artifacts/plan9/reset-20261026/trust-bundle.pem'
python docs/plan9/runtime/B05b/integration.py onboard --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05b/integration.py collectors --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05b/integration.py builtins --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05b/integration.py targets --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05b/integration.py alerts --session $sessionPath --ca $caPath
```

首次运行需存在已核验的基础和 Kafka 镜像；`up` 使用验证后的 image ID，要求 B05a `cleanup.json` 为 `destroyed`。重跑检查已有容器/身份，不重建当前注册身份。配置和密码仅位于受限 private 文件，凭据通过 stdin 写入客户端 root-only 目录，不放进命令行或报告。

本次 Debian 包是 Python confluent-kafka 1.7.0 + librdkafka 2.8.0；使用其实际支持的 `list_groups` API 查询原生组成员。部署时的客户端脚本兼容调整通过受控文件安装记录在 `client-runtime-install.json`，源码 SHA 与运行文件核验；不能仅凭初始镜像 SHA 推断最终脚本。后续完整构建会直接包含当前源码。

## 实际故障与恢复

```powershell
python docs/plan9/runtime/B05b/integration.py exercise --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05b/integration.py lifecycle --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05b/integration.py verify --session $sessionPath --ca $caPath
```

阈值在故障前冻结，评估间隔 10 秒、触发/恢复持续 20 秒，仅针对 work 组：

| 场景 | 实际动作 | 告警 | 恢复 |
| --- | --- | --- | --- |
| 消费积压 | 调用 Consumer.pause，继续 poll/心跳保持组成员；确认写入 240 条、每分区 80 条 | group 总 lag > 100，恢复 <= 5 | resume 后实际处理并逐条提交；暂停标记 180 秒自动过期 |
| 成员退出 | 停止 work 生产者与消费者，消费者正常 leave group | members < 1，恢复 >= 1 | 重启真实客户端重新加入消费组；180 秒定时器兜底 |

control 组持续生产消费作为健康对照。lag 必须逐分区与 Broker 查询的 log-end minus committed 对照；暂停期间已提交位点不能变化，恢复必须是实际消费推进。另保留 Kafka 自带 consumer-groups CLI 的位点观察，不用生产速率推断 lag。

只停采 Kafka 时，两条策略暂停评估，主机固定采集继续新鲜；重新启用后等待配置、样本及评估恢复。`no_data_behavior=report` 可能在新样本到达前产生短暂查询健康提示，按实际结果记录，不删除历史。

默认模板 v2 共 9 个面板，保留 Broker、Topic 分区/副本/同步副本、最新位点、已提交位点、分区 lag、成员数，并增加消费组总 lag，避免将分区值与总积压告警阈值混淆。标题支持中英，使用共享渲染和主题组件。

真实门户检查：`web/apps/enterprise/e2e/plan9-b05b-real.spec.ts`，在正常 API 取数/关联完成后运行，覆盖中英/明暗、完整面板、真实目标范围、历史告警和时间线、1280/1920 桌面宽度。结果见 [B05b 查看记录](../../batches/B05b-review.md)。

## 收尾

只有用户指定清理或下一批后执行：

```powershell
python docs/plan9/runtime/B05b/retire.py --session $sessionPath --ca $caPath
```

停恢复定时器和业务进程，解除 pause，实际消费提交残余消息；通过正常 API 收尾本批策略、采集、凭据、授权和主机，再按准确 ID/run 标签删除本批容器、卷、网络。正式平台 Kafka、企业账号、共享模板、评估身份与历史保留。本批完成不自动进入 B05c。
