# RabbitMQ 与 ZooKeeper 真实目标

2026-10-19，新增 RabbitMQ 4.3.6 和 ZooKeeper 3.9.6 的原生实际目标、认证/生命周期以及十二个默认查询。统一 brokers 两组通过，源码/Collector 指纹相同；正式平台五类门禁及发布仍未完成。

## 目标与权限

- RabbitMQ：官方 rabbitmq:4-management-alpine，本轮镜像内版本 4.3.6、Image ID `sha256:3505b6096cdbc083bcc363e74366f575ecd0c1a55be4dadcbc60fa08e34647e1`。内置 rabbitmqreceiver 0.158.0 直接读取 Management API，不安装 exporter。监控身份仅 monitoring tag，vhost configure/write/read 均为空匹配；whoami/nodes 能读取，合法的持久队列创建被拒绝。
- ZooKeeper：官方 zookeeper:3.9，本轮镜像目录 apache-zookeeper-3.9.6-bin、Image ID `sha256:5bad433c71701a1d788b4df86c921d91bfb65c23a1443bcb18a80f40373ae9d9`。内置 Receiver 使用已有 mntr 只读统计命令；只在本轮服务配置开放 mntr/ruok/stat，不改业务实例。

RabbitMQ 上游文档仍列 3.8/3.9；本轮只是实际验证 4.3.6 的具名组合，不能扩大成任意 4.x 支持。ZooKeeper 当前为 standalone，未将它算作 ensemble 拓扑验收。

默认 RabbitMQ 只开启队列指标，没有队列时无法给出节点观测。现通过上游 metrics 配置启用 mem_used、disk_free、fd_used、uptime，配合已有 queue 消息/consumer 指标。本轮具有真实持久测试队列，monitor 身份不能创建队列；管理账号只用于自有测试环境准备。

原生测试区分认证与授权：先证实 monitor 身份读取成功，再使用合法创建请求验证拒绝。当前 RabbitMQ Management 对该拒绝返回 401，测试接受已独立证明认证成功后的 401/403，不把任意 4xx 或无效队列参数当权限证明。

## 真实链路与模板

同一 Collector 的 RabbitMQ/ZooKeeper 保持独立目标和来源。错误 RabbitMQ 密码后没有目标点，恢复后采集重新出现；关闭 ZooKeeper 不影响 RabbitMQ，全部关闭后控制运行时继续运行。

新增 rabbitmq.overview 与 zookeeper.overview，当前模板实现共 24 套。RabbitMQ 内存/磁盘/FD/uptime/消息/consumer，ZooKeeper 连接/znode/watch/请求/延迟/packet 的十二个查询均由同批实际采样经正常化、Writer/ClickHouse 和 PromQL 验证。分别参考 [10991 RabbitMQ](https://grafana.com/grafana/dashboards/10991-rabbitmq-overview/)及 [10465 ZooKeeper](https://grafana.com/grafana/dashboards/10465-zookeeper/)，没有沿用 exporter 查询假定。

证据：

- `artifacts/plan8-broker-coordination-native-queue.log`：实际两目标及生命周期通过。
- `artifacts/plan8-broker-coordination-template-queries.log`：十二个实际查询通过。
- `artifacts/plan8-brokers-auth-20261019-686309455/manifest.json`：原生认证/授权与查询两组通过，前后指纹相同。

镜像准备慢导致的启动前件超时、RabbitMQ 非持久队列参数失败、固定只认 403 的失败记录保留。没有放宽实际读取、无权写入或目标样本要求。测试容器、匿名卷、网络按本轮 owner 回收；启动清理在 dispatch 前注册，即使客户端未取得容器 ID 也按固定名称及 owner 复验清理，防止未知启动结果残留。

仍未验证正式接入/部署、TLS、集群/ensemble、其他平台/版本、全部授权入口及模板发布业务闭环。本记录不把暂缓资源算通过，不替代候选支持矩阵冻结。
