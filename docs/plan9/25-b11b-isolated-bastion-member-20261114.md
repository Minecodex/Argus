# B11b 私网堡垒机验收与安装前件

> 2026-11-15：本批已随[仅保留账号的维护](../local-account-only-redeployment-20261115.md)退役，本文仅保留当时的验证记录，不代表当前仍有这些资源。

2026-11-14，用户要求继续下一阶段。B11a 已封存并按归属清理，保留历史；新建 B11b 的堡垒机和私网成员，不依赖上一批存活环境。

## 真实路径

成员只连接 Docker internal 私网，实际无法直连平台 HTTPS/Gateway/Ingest。堡垒机接入两张网络，通过正常 Direct SSH 安装，其两个 TLS 密文 Relay Listener 发布私网地址；成员由 Bastion Connector 经 SSH 安装并正常注册为 `bastion_relay`。

Collector 采用已支持的组合：堡垒机 `edge_gateway/direct_argus/direct`，成员 `leaf/bastion_gateway/bastion_tunnel`。实际传输如实记录，不用“内网”标签或普通直连伪装中继。私网套接字拒绝与堡垒机同一端点成功均留有原生证据，独立于 DNS。

## 安装失败及原位重试

首次成员安装 `HOST_ONBOARDING_ARTIFACT_SIZE_MISMATCH` 原因为实验临时目录 64 MiB，小于实际签名二进制 72,507,576 字节。调整目录时的首次路径又被 Connector 原有只读沙箱拒绝。最终使用原可写数据目录下的私有 TMPDIR，通过固定 ExecStartPre 创建、0700 权限、argus-connector 归属，保留 `ProtectSystem=strict` 和原 ReadWritePaths。

失败操作、阶段、三次自动尝试及后续正常 `preview-retry` 均保留；成员 Host ID 不变，新的连接测试冻结当前 SSH Host Key、Bastion Relay 代数及凭据。未放宽不可变制品大小、摘要或签名验证，未改写数据库状态。

B11a 清理时还补齐授权变更后的正常重新登录，避免使用过期授权版本继续归档；受管仪表盘绑定幂等键加入目标 Host ID，防止两个主机复用同一待确认动作。均为运行包处理，不需要重部署正式后端。

## 检查与证据

两台主机共 8 项 healthy/fresh 采集，8 个资源范围视图、40 个非空面板。成员业务服务失败与恢复已触发真实告警；查询中堡垒机为 0、成员为 1，验证 Leaf 资源 ID 不被 Gateway 覆盖。

控制断连与采集暂停分别执行：前者记录成员离线而堡垒机在线；后者保持两个控制通道在线，触发真实 `no_data` 并恢复，说明连接状态和实际新样本必须分别判断。两条路径观察 120 秒，丢弃、入队失败与接收拒绝均为 0，未清零计数。

4 项严格 TLS/正常登录的真实页面用例通过，包含两台主机的 Shell/OpenSSH 真实输出、停靠和缩放、加密录像回放、监控视图及告警时间线。正式健康 20 项、六个 PVC、原部署均保留。详见 [B11b 查看记录](./batches/B11b-review.md) 和 [运行包](./runtime/B11b/runbook.md)。

本批采用 systemd 容器，共享 Docker VM 内核，容量按实际可见主机范围解释。此次是 Docker 私网路径隔离验收，不是恶意特权容器逃逸或生产全局 NetworkPolicy/Egress Gateway 验收。
