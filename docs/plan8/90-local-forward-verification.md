# 本地端口转发验收与集群别名

2026-10-22。SDK 局部诊断已实际通过 OTLP HTTP、OTLP gRPC 与 Jaeger 的上报、Key 撤销轮换和端点关闭。随后安装验证的 `connector-forward-tls` 失败：验证器仅允许名为 `docker-desktop` 的 kubeconfig context，而本轮隔离环境为 Docker 中运行的 kind／Calico 集群。

## 修正边界

Context 名称是用户可修改的连接别名，不用它判断端口转发的验证范围。显式转发验证要求同时满足：

- 配置为 `evaluation` 或 `local-hardening`，使用 `local-registry` 镜像模式。
- 转发目标为明确的 loopback 地址。
- 验证真实服务证书、信任链、TLS 1.3、客户端身份及 HTTP/2 settings 交换。

`production`、未知或空 profile、其他镜像模式、非 loopback 目标仍拒绝。验证报告继续记录 `EXTERNAL_CONNECTOR_LB_NOT_EVALUATED_LOCAL_FORWARD`，不能据此声称外部 LoadBalancer 已通过。此调整扩大的是本地验收对集群别名的适用范围，不改变生产安装前件。

## 当前证据

- `artifacts/plan8-local-forward-profile-regression.log`：两种本地 profile × Docker Desktop、kind、自定义别名的六次实际 mTLS／HTTP/2 握手通过；无客户端证书、错误服务名、生产／未知 profile、非本地目标拒绝。
- `artifacts/plan8-calico-sdk-diagnostic-20261022-r65/plan8-deployed-foundation.json`：明确标记为局部诊断，SDK 三协议通过。其安装验证仍为失败原始记录，不改写成通过。
- 前一完整轮次 SDK 关闭端点的 `PENDING_ACTION_INVALIDATED` 尚未确定具体变化来源。局部成功不等于已解释该冲突；下一完整轮次将由独立只读观察器记录相关版本，保持产品与验收操作不变。

修正后的真实集群验证、完整业务验收和发布仍需继续执行。原部署尚未发布。
