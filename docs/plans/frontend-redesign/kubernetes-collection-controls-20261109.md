# Kubernetes 采集配置与仪表盘恢复

2026-11-09，根据实际集群页面反馈，采集页以用户选择采集内容为主。

## 交互与边界

- 集群详情的“OTLP 收集器”改为“采集配置”。主视图直接展示采集内容、开关、实际数据状态和默认仪表盘入口。
- 移除主视图中的运行状态大卡、绑定覆盖率、Claim 冲突数、版本、就绪副本进度、镜像、Revision、Profile 和出口信息。镜像、路由测试、升级、修复与卸载收进次级“收集器管理”弹窗。
- 未安装时先展示可采集内容与一个安装入口；安装弹窗默认使用已保存的集群凭据和默认容器镜像，自定义镜像及拉取凭据按需展开。安装完成后，通过各项开关独立启用指标。
- 主机与 Kubernetes 复用的 `BuiltinCollections` 统一精简为四列，显示内容说明、开关、最近样本状态和仪表盘操作。配置 Revision、独立健康列与技术诊断列移出日常视图；组件证书配置进入行操作菜单。
- 数据状态以采集实例的实际样本和新鲜度为准，不能把 Pod Ready 或配置已应用当成持续采到数据。

沿用既有“连接凭据（kubeconfig）/集群内 Kubernetes Connector → Gateway Deployment 与节点 DaemonSet Collector → 内置采集实例开关”能力。Connector 管理连接身份，Collector 管理遥测身份；不合并两种身份或扩大资源权限。所有安装、维护和开关变更仍经原 Preview/Confirm、授权、审计及运行收敛流程，不改变既定架构边界。

## 数据故障与修正

B09a 三项已启用采集的样本停在北京时间 10:42 左右。网关持续报告上报身份被拒绝；节点采集器报告续证失败。已确认安装模块生成 PKCS#8 ECDSA 私钥，而 Collector 续证模块只接受 SEC1 编码，导致首次证书到期前的自动续期失败。

续证模块现接受这两种 ECDSA 私钥编码，保持原密钥和原编码，不接受其他算法或损坏材料，也不偷偷替换现有密钥。新增回归复现原 PKCS#8 失败，并验证同一密钥签出的 CSR 与拒绝无效材料的行为。

另修正路由 API 响应遗漏 `transport` 和回环端口的问题，避免维护预览把空传输方式发回服务端。

当前安装已通过已有 PKI 身份修复流程取得新证书，并同步持久身份状态；通过正常管理预览与确认切换修正版 Collector。已确认三项采集重新产生新鲜样本，四套仪表盘 23 个面板返回实际数据。失败期间已终止丢弃的样本不宣称补回。

## 验证与发布

- 20 项桌面 E2E 通过：1280/1440/1920 × 中英 × 明暗的采集页验证、组件连接设置，以及主机/Kubernetes 共用采集开关与仪表盘入口回归。另有 1 项未安装集群的“安装 → 确认 → 收敛 → 可用开关”流程通过。
- 发布后 8 项真实页面 E2E 通过，覆盖中英明暗的采集页、三套默认仪表盘及八类资源读取。使用正常企业登录、实际 B09a 数据和正常 HTTPS 校验，不使用 Mock 或忽略证书错误。
- `argusidentity` 模块全量测试、Telemetry/CollectorManager/HTTP API/Direct Executor/Secret 五个相关包通过；类型、ESLint、国际化、样式 token、表单语义及格式检查通过。修改的前后端源文件均低于 2000 行。
- Web 发布为 `local-20261109-collection-controls-v2`。修正版 Collector 的 amd64/arm64 容器索引为 `sha256:2b2815e4fe768bfd2924cb0a209135ddea3503f031ebd7e1c9c7640238ece193`，已设为后续 Kubernetes 安装/配置的默认镜像。本次未发布新的主机签名分发制品。
- API Server 仅发布当前线上源码基线上的路由字段补丁；保留原运行程序及数据库 Schema 52，未混入工作区其他待发布迁移。Worker 沿用原镜像重启以加载默认 Collector 镜像。
- 平台健康检查 20/20 通过。核对 24 项工作负载/PVC 记录，除 Server/Web 镜像外，其余镜像、副本及 UID 一致，六个 PVC 绑定一致。三个维护 Job 和一个一次性输入 Secret 已按 UID 清理。
- B09a Gateway 与 Agent 均就绪，近五分钟无原私钥格式错误或上报身份拒绝；三项已开启采集持续产生新鲜样本，四套仪表盘 23 个面板均有非空实际样本。未开启组件仍显示“未开启”。

证据目录：`artifacts/kubernetes-collector-review-20261109/`。主要记录为 `dashboard-verification.json`、`final-freshness.json`、`collector-runtime-verified.json`、`frontend-pass.log`、`install-final.log`、`real-browser-final.log`、`health.json`、`deployment.json` 和 `server-patch-baseline.json`。真实截图位于 `real-browser-final/`。
