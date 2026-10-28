# B02b 网络探测真实运行包

一个正式接入 Argus 的探测主机，加两个独立目标容器 A/B。原生 Collector 实际发出 HTTP、TCP、TLS、DNS 和 ICMP 请求；Python 只承担被测 HTTP/HTTPS/TCP 服务，DNS 使用 dnsmasq，没有 OTLP 生成器或业务库造数。

## 拓扑

| 探测 | A | B（健康对照） | 实际故障 |
| --- | --- | --- | --- |
| HTTP/HTTPS | HTTP 8080 `/health` | HTTPS 8443 `/health` | A 实际返回 503 |
| TCP | 9000 Echo 服务 | 9000 Echo 服务 | 关闭 A 的监听进程，连接被拒绝 |
| TLS | 8443，独立测试 CA 签发 | 8443，正常证书 | A 换成 2 天证书及已过期证书，再恢复正常证书 |
| DNS | dnsmasq 5353/UDP | dnsmasq 5353/UDP | 移除 A 的本地记录，返回 NXDOMAIN；恢复后返回真实容器 IP |
| ICMP | target-a | target-b | 仅在 A 网络命名空间丢弃 Echo Request，随后撤销规则 |

两个 DNS 服务都查询 `app.b02b.test`，分别返回各自真实容器地址。只修改本批 DNS 配置，不改宿主或正式平台解析。

探测主机通过 SSH 22241 正常注册并安装 Connector/Collector。两目标没有发布宿主端口，只有本批网络可达。探测主机因 systemd 使用 privileged/private cgroup；目标容器只额外具备 NET_ADMIN/NET_RAW。总上限为 2.5 CPU、1792 MiB；不挂 Docker Socket 或可写宿主 cgroup。

## TLS 与 ICMP 前件

`pki.py` 在受限的 `artifacts/<run>/private/pki` 中生成本批独立 CA、目标私钥以及 normal/short/expired 三套真实证书。公钥证书清单可公开，私钥不进入仓库或日志。探测主机只获得 CA 公钥，目标只获得自己的叶证书/私钥，CA 私钥不挂载进容器。

- 正常证书约 90 天，临期证书约 2 天，过期证书 NotAfter 在过去；**不改变系统时钟**。
- HTTPS-B 使用正式 HTTP Receiver 的严格 TLS 校验，从实际探测主机用 curl/OpenSSL 复核 CA 与主机名。
- TLS Receiver 的指标是证书到期时间观察，不将它等同于链路信任验证；过期证书同时用 OpenSSL 严格校验，预期拒绝。
- ICMP 在探测主机的独立网络命名空间配置 `ping_group_range`，并检查正式 Collector 的真实丢包/RTT 输出。开发机 ping 成功不替代此验证。
- 同机 Docker 网络中的延迟不是物理网络性能承诺。DNS 原生耗时为整数毫秒，亚毫秒响应可能显示 0 ms，不补造延迟。

## 启动与正常接入

前件：B02a 已按正常生命周期退役，Docker Desktop Kubernetes 与 Argus 保持健康；Python 3、requests、cryptography、Docker Compose 可用。基础主机镜像 ID 必须与锁文件一致。

```powershell
docker build --tag argus-plan9-probes:b02b-20261027 --file docs/plan9/runtime/B02b/Dockerfile docs/plan9/runtime/B02b
python docs/plan9/runtime/B02b/manage.py prepare
python docs/plan9/runtime/B02b/manage.py up
python docs/plan9/runtime/B02b/integration.py onboard --session $sessionPath --ca $caPath
python docs/plan9/runtime/B02b/integration.py collectors --session $sessionPath --ca $caPath
python docs/plan9/runtime/B02b/integration.py builtins --session $sessionPath --ca $caPath
python docs/plan9/runtime/B02b/integration.py targets --session $sessionPath --ca $caPath
python docs/plan9/runtime/B02b/integration.py dashboards --session $sessionPath --ca $caPath
python docs/plan9/runtime/B02b/integration.py alerts --session $sessionPath --ca $caPath
python docs/plan9/runtime/B02b/integration.py review --session $sessionPath --ca $caPath
```

所有资源/采集/授权/策略经正常 API、预览与确认创建。共享 `common/hosts.py`、`common/monitoring.py` 和 `common/api.py`；只给共享 Plan9 评估身份授权当前批次的一个主机、十个目标，没有外部通知渠道。

## 故障、恢复与保持

```powershell
python docs/plan9/runtime/B02b/manage.py fault --kind http
python docs/plan9/runtime/B02b/manage.py recover --kind http
python docs/plan9/runtime/B02b/manage.py fault --kind tcp
python docs/plan9/runtime/B02b/manage.py fault --kind dns
python docs/plan9/runtime/B02b/manage.py fault --kind icmp
python docs/plan9/runtime/B02b/manage.py fault --kind tls
python docs/plan9/runtime/B02b/manage.py fault --kind tls_expired
```

CLI 故障默认有 180 秒自动恢复守护，显式恢复会取消守护；场景驱动还有 finally 恢复。`retain` 明确保留低风险的 HTTP 503 和临期证书，其他协议恢复正常，不保留额外压力负载。探测低频持续运行，等待用户查看后再清理。

告警固定为：HTTP/TCP/DNS 成功状态 `< 0.5`、恢复 `>= 1`；TLS 剩余 `< 7` 天、恢复 `>= 14` 天；ICMP 丢包 `> 50%`、恢复 `0%`。评估间隔 10 秒，触发/恢复持续窗口 20 秒。阈值在实际故障前冻结。

## 状态解释与自检

HTTP 状态码、DNS RCODE/解析地址变化会产生不同标签的序列。默认模板 v2 按资源与稳定检查身份选择最新观测，避免旧标签在 lookback 期间混入“当前状态”；原始指标与历史均保留，PromQL 引擎语义不变。

100% 丢包时，上游 RTT 的零值不作为快速回复绘制。测试检查当前 RTT 查询为空，恢复后真实 RTT 再次出现。

真实 E2E：`web/apps/enterprise/e2e/plan9-b02b-real.spec.ts`，覆盖中英/明暗、十目标类型化跳转、五类默认模板、资源隔离、告警详情及 1280/1920 桌面宽度。具体入口、时间窗口和结果见 [B02b 查看记录](../../batches/B02b-review.md)。

## 清理

收到下一批/销毁指令后使用 `retire.py`：保存证据、停止本批策略和采集、撤销本批数据授权、正常卸载探测主机、禁用其 SSH 凭据；再验证容器 ID/run 标签，清理本项目三个容器、三个卷和网络。共享平台、默认模板、评估身份与历史记录保留。

官方接入依据：[TLS](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.159.0/receiver/tlscheckreceiver)、[DNS](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.159.0/receiver/dnscheckreceiver)、[ICMP](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.159.0/receiver/icmpcheckreceiver)。
