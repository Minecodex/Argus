# SDK 接收确认与持久发送队列

状态：配置修复和故障驱动已合入，正式源码的受控注册原生组件测试通过；新增磁盘满、TLS 和查询配额分项已按合入后的源码实际通过；完整正式容量与摄入配额仍待验证。

在冻结的 60 秒、每秒 1,000 点、共 60,000 点/4,096 序列条件下，先阻断实际 Ingest 出口，待 SDK 全部收到接收确认后强杀 Collector，再以同一持久目录启动并恢复出口。原流水线最终只存储 59,000 点，见 `artifacts/plan8-queue-capacity-candidate-r2.log`。失败保留，未降低数量、提高预算或延长恢复窗口。

上游 0.159 的 batch processor 将数据放入内存通道后即返回成功。因此应用接收端可能先确认数据，最后一批却尚未进入持久发送队列。固定源码位置、哈希及对照结果记录在 `artifacts/plan8-sdk-durable-ack-candidate-facts.json`。

应用 OTLP/Jaeger 的指标、日志、Trace 流水线现保留应用身份处理与 memory_limiter，直接交给标准 file_storage 发送队列，去掉中间内存 batch。未 fork Receiver、Sender 或引入独立服务，Collector 二进制和上游版本保持 0.3.2-plan8 / 0.159.0。文件日志、K8s 日志和下游 Gateway 身份转发路径本来已避免同类异步批处理，不把此次修复误记为这些路径的新增变更。主机/服务自主采样的批处理与远端 SDK 确认边界分别记录。

正式源码证据 `artifacts/plan8-sdk-durable-ack-product.json/.log`：

- 60,000 点全部存储、4,096 序列，故障期间存储为 0。
- SIGKILL 后恢复用时 10,341 ms，低于冻结的 120 秒窗口。
- 队列磁盘峰值 4,259,840 字节，Collector RSS 峰值 113,696,768 字节。
- 100 次查询 P95 331 ms，Key 撤销拒绝 1 ms。

同轮原有真实三信号 SDK、APM/Trace、关联日志、应用授权、Key 撤销和轮换回归通过。三平台配置单测确认四条 OTLP/Jaeger 信号路径都保留身份/准入与持久队列；它不代表 Windows 或 ARM64 实机已经通过。该试验仍使用受控 PKI/注册，不抵正式接入下的全部 A62。

## 磁盘满、TLS 与查询配额

新增原生故障测试及正常 SDK 组件组注册，不改变 Collector 发行版或组件实现。文件系统故障仅作用于本轮容器内的独立 8 MiB 队列挂载，身份、证书及控制报告仍使用原持久目录；不填满宿主、共享卷或平台数据库磁盘。8 MiB 是故障注入条件，原冻结容量门槛和工作负载没有调整。

候选实际执行已观察到：

- 出口中断时先确认 1,000 点，再实际写满专属队列文件系统。新请求被拒绝，Collector 仍运行；清除本轮填充文件并恢复出口后，先前确认的 1,000 点全部入库，新的 1,000 点也入库。原生日志有空间不足诊断且不含接入 Key。未确认请求的交付结果保留未知，不声称其一定没有到达。
- SDK 使用错误 TLS 服务器名或空信任根均被拒绝；正确固定身份恢复后，实际存储 1,000 点。请求序列上限为 1 时，真实查询返回预算错误；恢复至原冻结的 100,000 上限后查到 1,000 条实际序列。这只覆盖查询配额，不等同于摄入速率配额。

候选证据为 `artifacts/plan8-disk-full-candidate-r3.json` 和 `artifacts/plan8-tls-quota-candidate.json`。首轮磁盘试验的存储失败和 `UNKNOWN_TOPIC_OR_PARTITION` 保留；测试 Kafka 夹具原先只创建三个主主题，现补齐生产已有的三个死信主题，未在失败时绕过 Writer。后续非空前件的磁盘试验通过，不把零条已确认数据的恢复当作可靠性证明。

上述驱动已合入，正式源码无 overlay 的两项测试实际通过（合计 56.5 秒），证据写入 `artifacts/plan8-disk-full-product.json`、`artifacts/plan8-tls-query-quota-product.json` 与 `artifacts/plan8-disk-tls-query-quota-product.log`。这些使用受控注册、真实 Collector/Kafka/Writer/ClickHouse，不抵正式主机或 K8s 接入的全部故障、性能及队列跨 Pod 恢复门禁。
