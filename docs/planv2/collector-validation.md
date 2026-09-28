# Collector 来源与发行包验证

本文件保留 2026-09-25 初次分段验收记录，覆盖当时的 Linux amd64 OCB 发行包。随后 Catalog 已升为 Revision 3，并加入 SkyWalking/Jaeger Receiver；真实协议、自监控及正式 Dashboard 链路的新增证据见 [自监控验收](./self-monitoring-native-traces.md)。Windows/arm64 运行验证、实际模型与 Workspace 完整部署验收仍未由本记录证明。

## 修复与权威目录

- `configbundle.DistributionComponents` 使用真实组件 ID。补齐目录漏列的 resource、health_check、k8s_cluster、argus_gateway_identity，修正 k8s-cluster Profile 的组件引用。CatalogRevision 升为 2。
- 发行版本、上游 Collector 版本、目录配置 Schema 和各平台组件清单集中声明；正常契约生成同步 API Client mock。mock 不再继续显示 0.132.0 和旧 Schema。
- 静态契约核对 Linux amd64/arm64、Windows amd64 三份 OCB 配置的版本及组件。真实二进制的 `components` 输出另与目录精确比对，不能仅凭手写列表宣称发行包支持。
- Windows 仍是 validation_pending；本次没有运行 Windows 或 Linux arm64 二进制，没有启用原生 SkyWalking/Jaeger 插件。
- 修复 collector-self 只启用 hostmetrics 的功能缺口：通过 prometheus/collector_self 每 30 秒抓取本机 127.0.0.1:8888 的 Collector 进程、Receiver、Exporter 等实际指标，监听不对外暴露。Host、K8s Agent 与 Gateway 分别生成来源身份，不与应用 Prometheus endpoint 或 hostmetrics 合并。Profile 只声明 metrics，不再错误声明日志能力；已安装采集器需应用新配置才能生效。

## 可重复的检查

构建当前源码发行包：`go run ./cmd/argus-dev collector build linux-amd64`。此命令重新构建本地二进制与压缩包，缓存指纹覆盖 OCB 配置和自定义组件源码；不会推送镜像或安装到受管资源。

普通测试：`go test ./internal/otelcol/configbundle ./tests/contract`。

真实二进制检查使用环境变量 `ARGUS_COLLECTOR_TEST_BINARY` 指定新构建的 Collector，并运行：

```text
go test ./internal/otelcol/configbundle -run 'TestRealCollector|TestRenderedTargets' -v
```

在 Windows 开发机可以交叉编译此 package 的 Linux 测试二进制，在独立 Kubernetes Namespace 的测试 Pod 中执行。测试 Pod 使用自己的 ServiceAccount，将节点 kubelet 的公开证书文件只读挂载到 `/var/run/argus-kubelet/pki/kubelet.crt`，不挂载私钥或整个宿主根目录。

| 检查 | 已验证内容 | 未包含内容 |
| --- | --- | --- |
| `TestRenderedTargetsLoadRealCollectorDistribution` | Host 常用插件、Host Gateway、K8s Agent、K8s Gateway 四类完整渲染配置通过实际 `validate`，含来源 Processor | 不启动认证/注册流程，不证明目标环境证书或授权有效 |
| `TestRealCollectorComponentsMatchDistributionRegistry` | 实际二进制组件与服务端目录一致 | 不证明各平台均已安装运行 |
| `TestRealCollectorPreservesReceiverSourcesOnWire` | 启动真实 Receiver、来源标记、Batch、OTLP Exporter；OTLP 三信号、应用 Prometheus 指标和 Collector 自身指标到达本地接收端；同名指标不同来源，伪造来源引用被覆盖 | 测试替换端点、TLS/注册和持久队列，并加快 self 抓取周期；不覆盖 Ingest 授权、Kafka、存储或 Workspace |

Kubernetes 执行环境为 `argus-p2-collector-20260925-g4c8`，均已通过。初次在 WSL 直接检查 K8s Agent 缺少挂载的 kubelet CA，改在具备真实只读证书/ServiceAccount 挂载的临时 Pod 中通过；没有关闭 TLS 校验来绕过。首次 wire 测试的本地接收夹具漏注册 gzip 解码器，补齐后完整重跑通过；正式 Ingest 原本已经注册 gzip，无对应生产修复。

测试结束后核验 Namespace、Pod 的 codex-planv2 所有者标签和空 PVC 清单，已删除该 Namespace；没有修改宿主证书文件，Kubernetes `/readyz` 正常。

## 初次分段验收时的发布门禁（2026-09-25 历史记录）

以下保留当时的环境和缺口；后续临时环境的镜像构建、签名包、Collector/Kafka、Dashboard/Chat/Workspace 与故障验收已完成，见 [2026-09-28 验收结论](./completion-review-20260928.md)。这不代表全部平台的 Collector 运行验证、长期正式部署升级或远端发布已经完成。

- 当前本地 Collector 内嵌版本仍是 0.1.0-m7，上游锁定 0.133.0。未发布新的远端镜像/签名产物，不把本地重建视为存量 Collector 已升级。
- Docker `version` 服务端调用在 10 秒限时内无响应；已结束此只读检查进程，没有重启 Docker/WSL 或变更共享集群。Kubernetes `/readyz` 正常，其他项目服务未暂停。
- 正式镜像构建、目录签名产物同步、Collector/Kafka 来源闭环，以及 Dashboard/Chat/Workspace 的完整部署验收继续保留在 [未完成清单](./implementation-status.md)。
