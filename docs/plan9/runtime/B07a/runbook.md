# B07 真实应用运行包

B07a、B07b 和 B07c 共用同一套业务服务和验收脚本。以下命令默认运行 B07a。对每条 Python 命令统一追加 `--batch B07b` 或 `--batch B07c` 可选择相应的独立资源。B07b 使用原生 OTLP gRPC。B07c 使用 Jaeger gRPC `PostSpans` 发送真实请求的 SDK Span，并通过独立 OTLP gRPC 接入点发送指标和日志。运行结果见 [B07b](../../batches/B07b-review.md) 和 [B07c](../../batches/B07c-review.md)。

本批沿用当前 Plan11 部署与正常企业账号，构建独立 Debian systemd 主机和 Redis：`orders-gateway` 接收实际 HTTP 请求，经 W3C Trace Context 调用 `orders-worker`，后者执行 Redis SET/GET。两个服务运行在本批主机的独立进程中，Redis 运行在另一个容器。指标、日志和 Span 均来自服务实际处理请求时的 SDK 埋点。

## 资源与上限

- 每批各有一个 Collector 主机、一个 Redis、两个容器、两个卷和一个私有网络。
- 主机 1.5 CPU/1536 MiB，Redis 0.5 CPU/192 MiB，Redis 数据上限 8 MiB、最多 64 个业务 key、TTL 120 秒。
- 仅 SSH 绑定宿主机 loopback。B07a/B07b/B07c 分别用 22341/22342/22343。HTTP 业务端口 18080/18081 与 SDK 端口在容器内访问。B07a 使用 24318，B07b 使用 24317，B07c 使用 24250（Jaeger）和 24317（OTLP）。SDK 使用平台签发的接入点专用 Key、公开 CA、正确 SNI 和 TLS 1.3。
- 正常负载每轮间隔 0.5 秒，串行发请求，最多 60,000 次/8 小时。两个应用服务与原生监控继续保留；Key 有效 24 小时，到期需要正常轮换。没有外部通知渠道或真实模型调用。
- fault 文件只接受有界截止时间，错误、延迟、Redis 等待最多 180 秒，进程每次请求检查截止时间，超时自动恢复正常。

## 构建与接入

```powershell
python docs/plan9/runtime/B07a/manage.py prepare
python docs/plan9/runtime/B07a/manage.py build
python docs/plan9/runtime/B07a/manage.py up
$sessionPath = 'artifacts/plan9/reset-20261026/private/enterprise-session.json'
$caPath = 'artifacts/plan9/reset-20261026/trust-bundle.pem'
python docs/plan9/runtime/B07a/integration.py onboard --session $sessionPath --ca $caPath
python docs/plan9/runtime/B07a/integration.py collectors --session $sessionPath --ca $caPath
python docs/plan9/runtime/B07a/integration.py builtins --session $sessionPath --ca $caPath
python docs/plan9/runtime/B07a/integration.py targets --session $sessionPath --ca $caPath
python docs/plan9/runtime/B07a/integration.py application --session $sessionPath --ca $caPath
python docs/plan9/runtime/B07a/manage.py workload
python docs/plan9/runtime/B07a/integration.py services --session $sessionPath --ca $caPath
```

`up` 要求前一批已正常卸载且清理记录为 `destroyed`。B07a、B07b、B07c 分别检查 B06d、B07a、B07b。build 使用当前根模块锁定的真实 Go OTel SDK，镜像构建上下文放在本次 artifacts，不把二进制提交到 docs。普通 API 完成应用、SDK 接入点、一次领取 Key、Redis 采集与应用关联；服务由真实 SDK 自动发现，未手工创建假的服务记录。

prepare 生成随机 SSH/Redis 凭据，private 目录采用 Windows ACL，容器 SDK 配置目录 700、文件 600。Key、密码不进入 Docker 参数、日志或文档。Redis app 仅访问 p9:* 与必要的 SET/GET/BLPOP 等命令；monitor 用于 INFO/CONFIG GET/PING。未给业务账号管理权限。

## 数据与场景

服务实际处理请求时通过 SDK 记录请求数、错误数、活动请求和实际耗时直方图，并在同一 Trace 上发出业务日志。成功请求的错误增量为 0，这是该次真实请求的观测，不是给缺失来源补数据。P95 指标采用 SDK 直方图插值，与 APM 对入口 Span 的分位数统计口径不同。

| 场景 | 实际行为 | 固定告警与恢复 |
| --- | --- | --- |
| 正常 | gateway → worker → Redis SET/GET，返回相同 request ID、Trace ID 和 Redis 实读值 | 当前业务告警恢复 |
| 受控错误 | worker 执行真实 Redis 操作后返回 HTTP 503，gateway 原样传递，Span/日志标记错误 | gateway 最近 1 分钟错误率 > 25%，恢复 <= 5% |
| 数据库等待 | worker 对空列表执行 BLPOP，实际等待约 0.6 秒，出现 Redis 等待 Span | gateway 最近 1 分钟平均耗时 > 300 ms，恢复 <= 150 ms |
| 服务延迟 | worker 实际等待 500 ms，返回 200 | 单独保留 HTTP 收据和延迟观测 |
| 接入停用 | 先停止并刷新本批 SDK 进程，再通过正常 API 停用入口，确认被停用的监听端口关闭；Redis 继续采集。B07c 另验证 OTLP 接入点保持生效 | 恢复入口、服务后验证新 SDK 样本和规则健康 |

应用 SDK 停用不复用原生采集的 not_evaluated 语义：当前后台仍按窗口与 no_data 策略评估。脚本记录实际规则健康，不声称自动暂停；原先套用原生采集断言而失败的记录保留。没有样本不能判定服务健康。

两业务策略仅授予本批 application 范围。共用 Plan9 评估角色增加 application.read，资源授权仍显式限定本批应用、主机和 Redis，后续清理撤销本批数据范围。

```powershell
python docs/plan9/runtime/B07a/integration.py dashboards --session $sessionPath --ca $caPath
python docs/plan9/runtime/B07a/integration.py alerts --session $sessionPath --ca $caPath
python docs/plan9/runtime/B07a/integration.py exercise --session $sessionPath --ca $caPath
python docs/plan9/runtime/B07a/integration.py lifecycle --session $sessionPath --ca $caPath
python docs/plan9/runtime/B07a/integration.py verify --session $sessionPath --ca $caPath
python docs/plan9/runtime/B07a/browser.py
```

应用仪表盘为本批正常发布的 10 个面板：请求速率、错误率、平均耗时、P95、活动请求、运行时长、服务、拓扑、Trace、日志；Redis 默认图 11 个面板。应用详情另有服务、实例、接口、RED、拓扑、Trace、日志七个实际视图。浏览器覆盖中英/明暗、日志关联 Trace、真实告警时间线、两套图和 1280/1920 桌面。

## 当前更新的兼容与保护

B07a 在 Plan11 基线上发布了应用来源能力匹配、拓扑页有界预算和单次 PromQL 重复选择器复用，见[修正记录](../../09-b07a-application-source-capability.md)。B07b 和 B07c 复用该平台版本，使用正常 API 部署各自的运行环境，并核对平台 Deployment 和 PVC 绑定。

更新期间的原生接收/发送告警按实际保留；接收拒绝次数不等于最终丢失量，不清除历史记录制造“零告警”。具体事件与最终计数见本批查看记录。

## 清理

用户要求下一批后执行：

```powershell
python docs/plan9/runtime/B07a/retire.py --session $sessionPath --ca $caPath
```

先停止负载及 SDK，删除本批业务策略、停用本批接入点并逐个撤销 Key、解除 Redis 关联、归档本批仪表盘与应用、撤回应用数据授权，然后复用正常采集归档、主机卸载/退役和所有权核验后的 Compose 清理。保留共享定义与历史，下一批按用户指令启动。
