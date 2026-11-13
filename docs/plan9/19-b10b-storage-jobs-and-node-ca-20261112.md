# B10b 存储、任务与节点证书适配

2026-11-12，本轮按用户“直接进行下一轮验证”指令清理 B10a，搭建 B10b 独立真实环境。入口见 [B10b 查看记录](./batches/B10b-review.md)，操作顺序见[运行包](./runtime/B10b/runbook.md)。

## 节点证书修复

B10b 启用 kubelet serving certificate bootstrap 后，旧默认路径 `/var/lib/kubelet/pki/kubelet.crt` 不存在。原 Collector 配置对该文件的固定依赖导致候选校验失败。没有通过复制私钥、关闭证书校验或伪造健康绕过。

`kubernetes_node_metrics` 复用现有 `component_tls` 配置模型和门户组件信任表单，接受公开 `ca_pem`。上游原生 kubelet ServiceAccount 客户端只按文件读取 CA，不能依赖其内联 CA 或服务名字段。配置编译使用固定的 `/etc/argus-otelcol/kubelet-ca.pem`；部署器从已校验的 Collection 配置提取公开 PEM，加入同一候选 ConfigMap 及既有大小预算、原生校验与回滚。节点始终按节点名验证，API 拒绝服务名覆盖，门户隐藏该输入。未配置时保留既有默认。固定节点端点、ServiceAccount 认证及 `insecure_skip_verify: false` 保持原有边界。

公共 CA 仍经原有 PEM 数量/大小、私钥拒绝和名称校验；不新增任意文件路径、任意端点、凭据字段或数据库迁移。[上游 kubelet 客户端](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.159.0/internal/kubelet/client.go)在 ServiceAccount 模式读取 CAFile 并保留节点名称和证书链验证。独立原生接收器用错误 CA 文件连接本轮 kubelet，实际得到 unknown authority，测试 Job/ConfigMap 随后删除。实际 Agent/Gateway 原生配置校验通过，三项采集恢复 fresh/healthy。

## 被后续变更取代的操作

真实长 Job 在创建观察完成前被暂停、恢复，后续写入替换了操作 marker。原观察无法再匹配自己的 marker，旧创建记录一直停在等待生效。

共享 Worker 现在仅在已经收到写入接受回执、原操作处于 observing、原生观察返回结果未知时，补读该确切对象。必须同时满足已确认 UID、同一 Namespace UID、名称/类型一致以及 generation 严格增加，才结算为既有 `KUBERNETES_OPERATION_SUPERSEDED`。未知写入、同 generation、替换对象/Namespace、读取失败均保持原来的未知处理；不再次发写入。该逻辑适用于所有工作台对象，未改变两阶段确认和只写一次的架构边界。

十组单元场景覆盖这些证据条件；已部署 Worker 对本轮真实旧 Job 创建记录完成结算。后续 Job 暂停/恢复、真实完成以及浏览器手动 Job 各自保留独立成功记录。

## 发布与证据

后端基于已部署 B10a Schema52 冻结源码，仅叠加节点 CA 与操作观察两项修复，没有发布当前工作区 Schema53 或其他功能。镜像为：

`host.docker.internal:5001/argus/argus-backend:local-20261112-b10b-native-ca-v3@sha256:d6fe64f7f8f4712b58fd30e445e518366d11b16b6019033bc2b0f692e02c28a6`

Server、Worker、Direct Executor、Connector Gateway 更新并验证精确 digest。共享证书表单以冻结 Web 基线叠加两处组件/文案及生成能力目录，发布 `local-20261112-b10b-node-ca-v1@sha256:1c03d749735569219fe05df3ab4c278eebd68c8406cff54d44a11c837d81b93d`。Telemetry Ingest/Query/Writer、PKI 和正式六个 PVC 绑定保留。

本轮产物位于 `artifacts/plan9/p9-b10b-20261112-01/`：

- `storage-exercise.json`、`jobs-exercise.json`、`hpa-exercise.json`、`network-policy-exercise.json`：真实协议、挂载、任务、CPU 与通信结果。
- `superseded-create-verified.json`：原创建操作的实际结算。
- `delivery-watch.json`：完整五分钟重试周期，没有新增终止、入队或接收拒绝，历史计数保留。
- `browser-review.json`、`real-browser/`：实际门户中英明暗与日志/终端、仪表盘证据。
- `formal-health.log`、`checks.json`：正式健康、最终对象和绑定、告警周期与保留范围。
- `backend-unit-tests-v3-all.log`、`contracts-retry.log`：相关单测和隔离契约生成检查。首轮 Windows Node bundler 的 UV_HANDLE_CLOSING 异常日志保留，重跑成功才计通过。

原生指标中未出现该 Rawfile PVC 的 kubelet volume stats，因此没有把宿主文件系统容量或零值当作 PVC 使用率。实际卷容量由对象状态与容器 statvfs/df 共同验证。人工验收仍为 pending。
