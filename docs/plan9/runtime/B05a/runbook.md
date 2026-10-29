# B05a RabbitMQ 真实场景运行包

运行 `p9-b05a-20261028-01`。B04 经用户确认通过后，通过正常 API 停采、卸载、退役和回收；保留企业账号、共享评估身份、历史和以前的诊断记录。

## 组成与边界

- 1 台正常 SSH 注册的 `B05a-MQ-Collector`，正式安装 Connector/Collector。
- 1 个独立 RabbitMQ 4.4.0 管理插件实例；镜像版本/ID/摘要写入 `images.lock.json`，运行按实际 image ID 锁定。
- `p9-work`、`p9-control` 两个 vhost，各有一个同名 `jobs` 队列，用于验证 vhost 维度与健康对照。
- 每个队列最大 500 条、1 MiB，超限拒绝发布；消息 TTL 15 分钟，故障注入另有 180 秒恢复保护。单消息为带真实 ID/时间的少量 JSON，突发最多 250 条。
- 每个 vhost 一个真实 AMQP 生产者和消费者，publisher confirm、mandatory 路由和 manual ack 均实际启用。消费者 prefetch=10，校验消息 ID 与所属 vhost。
- monitor 只有 monitoring 标签和空队列权限；实测不能建队列。两个业务账号只拥有各自 vhost 的 `jobs` 所需权限，实测 work 账号不能连接 control vhost。

总共 2 容器、2 卷、1 专网，CPU 上限 2.5，内存上限 2048 MiB。RabbitMQ 内存水位 256 MiB、磁盘余量门槛 64 MB、Erlang 调度器有界。只发布主机 SSH `127.0.0.1:22271`，AMQP 与 Management 端口不向宿主发布。本批协议为专网内明文，不包括 TLS 或 RabbitMQ 多节点高可用验收。采集主机为独立 systemd 容器，使用 private cgroup，无宿主 Docker socket。

密码随机生成，位于受限 private 目录；主机客户端材料通过 stdin 写入 root-only 目录，不进入镜像、命令参数或报告。RabbitMQ 初始化使用正式 Management API/AMQP，不改 Broker 数据文件，更不向 Argus 数据库/遥测入口写演示数据。

## 启动与正常接入

```powershell
python docs/plan9/runtime/B05a/manage.py prepare
docker build --tag argus-plan9-rabbitmq:b05a-20261028 --file docs/plan9/runtime/B05a/Dockerfile docs/plan9/runtime/B05a
python docs/plan9/runtime/B05a/manage.py up
python docs/plan9/runtime/B05a/manage.py initialize
python docs/plan9/runtime/B05a/manage.py workload

$sessionPath = 'artifacts/plan9/reset-20261026/private/enterprise-session.json'
$caPath = 'artifacts/plan9/reset-20261026/trust-bundle.pem'
python docs/plan9/runtime/B05a/integration.py onboard --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05a/integration.py collectors --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05a/integration.py builtins --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05a/integration.py targets --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05a/integration.py alerts --session $sessionPath --ca $caPath
```

`up` 要求 B04 清理状态为 `destroyed`，已有容器检查准确 ID/归属，不重建已注册身份。首次构建前需具有已核验基础镜像和 RabbitMQ 镜像。会话过期通过正常企业登录刷新。

持续负载每个 vhost 每 2 秒发布一条确认送达的消息，消费者实际确认；上限 8 小时。消费者在发布者之前启动，队列有硬上限，超限 NACK 不被当作发布成功。运行时统计只记录真实客户端操作，平台指标仍来自原生 Receiver。

## 故障、恢复与核验

```powershell
python docs/plan9/runtime/B05a/integration.py exercise --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05a/integration.py lifecycle --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05a/integration.py verify --session $sessionPath --ca $caPath
```

两条策略在故障前冻结，评估 10 秒，触发/恢复持续 20 秒，仅选择 `p9-work/jobs`：

| 场景 | 实际动作 | 策略 | 恢复 |
| --- | --- | --- | --- |
| 待消费积压 | 停止 work 消费者，确认发布 250 条；control 继续生产消费 | ready > 100，恢复 <= 5 | 重新启动消费者，实际确认后排空；180 秒定时器兜底 |
| 未确认消息 | work 消费者实际接收后暂缓确认，prefetch 将未确认上限限制为 10；再发布 20 条 | unacknowledged > 5，恢复为 0 | 解除标记后在原连接上逐条确认；标记 180 秒自动过期 |

每阶段对照 RabbitMQ 原生队列状态与 Argus 数值，检查同名队列没有跨 vhost 串值。恢复匹配本次实际 firing ID，不能人工创建或改告警状态。未设置外部通知接收人。

独立停采只禁用 RabbitMQ 采集，等待两条策略暂停评估、主机固定采集继续新鲜，再恢复配置、样本和评估；业务客户端继续运行。测试完成不保留压力和未确认消息，低频业务负载保留供用户查看。8 小时上限只停止业务进程，不自动销毁环境。

RabbitMQ 受管模板 v2 有 9 个面板：节点内存、磁盘、描述符、运行时长，队列消息/消费者，以及真实发布、投递、确认速率。队列和 vhost 标签保留；图表、单位、标题提示沿用共享组件，中英/明暗均验证。

真实门户 E2E 为 `web/apps/enterprise/e2e/plan9-b05a-real.spec.ts`。设置 `ARGUS_PLAN9_B05A_E2E=1`、正式 origin、当前 resource map、B04 retired map 与正常登录会话，覆盖 4 个语言/主题组合、全部面板/真实来源、告警历史和时间线，以及 1280/1920 桌面宽度。最终结果见 [B05a 查看记录](../../batches/B05a-review.md)。

## 用户要求下一批之后清理

```powershell
python docs/plan9/runtime/B05a/retire.py --session $sessionPath --ca $caPath
```

停止恢复定时器和客户端，解除 hold 并通过 AMQP 手动确认剩余消息，再经正式 API 收尾策略、采集、凭据和授权，卸载主机，按精确容器 ID/run 标签回收两个容器、两个卷和网络。共享模板、评估身份、企业账号和历史保留。
