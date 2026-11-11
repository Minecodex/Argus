# B10a 当前 Pod 终端名单与日志选项对齐（2026-11-11）

本轮按实际运行状态、接入账号的 Kubernetes 原生权限和平台 PTY 逐项核对。运行 `p9-b10a-20261111-01`；命名空间和 Pod 名以本次检查为准，后续重建可能改变后缀。

## 当前可以连接的 Pod

在门户先选择命名空间 **b10a-workloads**。以下四个 Pod 都已从正式 Argus 页面实际连接 `sh`，执行 `id -u; command -v sh; command -v bash; pwd`，取得真实 uid=10001、Shell 路径及工作目录，再正常关闭会话，双向字节数均大于 0。

| Pod 当前名称 | 容器 | 建议 Shell | 平台终端 |
| --- | --- | --- | --- |
| b10a-api-6f44c8b8b9-rrkdz | http | sh | 已实测 |
| b10a-driver-96b8b5559-lsnnv | http | sh | 已实测 |
| b10a-ledger-0 | http | sh | 已实测 |
| b10a-node-observer-pwfsq | http | sh | 已实测 |

四个业务镜像同时包含 `/usr/bin/sh` 和 `/usr/bin/bash`；本轮平台实测使用默认 `sh`。powershell/cmd 不在这些 Linux 业务容器中。

操作步骤：资源查询 → 命名空间改为 b10a-workloads → Pod → 打开终端 → 容器 http、Shell sh → 填写操作原因 → 创建会话。打开弹窗是准备连接；填写原因并创建后才建立 PTY。

## 为什么感觉所有 Pod 都连不上

检查时共有 17 个 Pod，只有上述 4 个位于已授予 exec 的命名空间。其余 13 个的原生权限/状态如下：

| 命名空间 | Pod 数 | 当前限制 |
| --- | --- | --- |
| argus-telemetry | 3 | 接入身份没有 pods/exec；Agent/Gateway 的精简镜像没有普通 Shell，验证 Job 也已 Succeeded |
| b10a-infra | 1 | 接入身份没有 pods/exec |
| kube-system | 8 | 接入身份没有 pods/exec |
| local-path-storage | 1 | 接入身份没有 pods/exec |

最近的实际失败发生于 Collector Agent 和 local-path-provisioner，原因均为 `KUBERNETES_API_FORBIDDEN`。这与业务 Pod 的真实成功会话分别记录，未把管理员原生 exec 冒充平台连接成功。

这里有两层权限：平台用户的 kubernetes.exec/资源权限允许发起终端；集群连接 ServiceAccount 还要在对应命名空间拥有 create pods/exec。页面按钮可见表示平台层可发起，并不等于原生接入身份、Pod 状态和镜像 Shell 全部满足。本轮不扩大原生权限，继续保留已明确的 B10a 边界。

[17 个 Pod 与原生权限检查](../../artifacts/plan9/p9-b10a-20261111-01/pod-terminal-inventory.json) · [当前平台四项终端实测](../../artifacts/plan9/p9-b10a-20261111-01/pod-terminal-platform-proof.json)。

## 日志选项错位修复

上一轮只覆盖日志工具栏和弹窗底部操作栏，未覆盖展开的参数区。时间窗字段包含一行说明，原先将整块 Field 按底部对齐，导致它的输入框被顶高。

现在参数区使用共享栅格的标签、输入框、说明三行：两个输入框和“应用并重新读取”位于同一行，帮助/错误信息在单独一行，不再影响控件对齐。沿用现有设计 token，未更改表单和日志读取语义。

实际页面回归覆盖中英 × 明暗、1280/1920 桌面，检查两个输入框与应用按钮的 y 坐标、水平间隔、说明位于输入框下方、可访问性和完整截图。另逐个验证全部四个业务 Pod 的平台终端，最终共八项流程。

仅重新发布 Web，保留后端、Collector、原生 RBAC、正式服务和六个 PVC。人工验收仍 pending；当前环境继续保留。

[本批查看路线](./batches/B10a-review.md) · [本轮八项实际页面](../../artifacts/plan9/p9-b10a-20261111-01/terminal-options-browser.json) · [精确发布](../../artifacts/plan9/p9-b10a-20261111-01/terminal-options-audit/release.json)。
