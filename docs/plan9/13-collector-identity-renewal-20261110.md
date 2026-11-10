# Kubernetes Collector 统一续期与长连接重建

## 问题与边界

2026-11-10 的真实组件仪表盘验收发现，Gateway 与 Agent 共享一个逻辑 Collector 身份，却分别调用续期接口。后一次续期把前一批证书限制在 15 分钟重叠窗口内，Gateway 最后持有未过期但已被注册表拒绝的证书，持续上报失败。上轮只是恢复现场，并未根治。

本轮保持一个 Collector 对应一个 Kubernetes 采集安装的既有边界。现有 Gateway 已是单副本、Recreate Deployment，并使用专用身份 PVC，继续承担唯一续期与固定 Secret 镜像职责；不增加 CA、身份授权、集群级权限或新进程。

## 修正

- Kubernetes Agent 显式启用跟随身份模式，不调用 enrollment、rotate 或自行更新 Trust Bundle。每分钟读取挂载 Secret 的同一投影版本，验证完整材料后同步至可写运行目录。Gateway 与普通 Host 的本地续期流程保持。
- 跟随更新核对 CA、摘要、epoch、Collector URI、DNS SAN、单一 EKU、有效期与密钥配对；拒绝倒退或不完整更新，保留最后有效材料并重试。证书最后写入作为本地提交标记，原始 Secret 始终只读。
- Gateway 的 OTLP gRPC 接收器和 Argus Ingest gRPC 服务均限制连接年龄为 5 分钟，给予 30 秒宽限，让客户端在 15 分钟旧证书重叠期内重新握手；保留每次请求的身份状态校验。导出器与接收器的证书热加载需一并验证。
- 真实换证还暴露了同一 Collector 的两跳序列号必须完全相等的问题。对同身份的转发，若序列号不同，Ingest 必须额外查询其活动证书注册状态，并核对 Collector、企业、资源、角色、URI 与授权版本完全相同；仅接纳合法重叠期，不接受已吊销或已到失效时间的旧叶证书。其他 Collector 的 Gateway 路由授权边界保持。
- 不延长或取消旧证书失效窗口，不接受已吊销身份，不用关闭 TLS 校验恢复采样。

## 验收计划

单测覆盖跟随者不续期、投影切换、多 Agent 同步、损坏/错误身份/回退拒绝、重启与权限；配置测试保证角色配置与连接上限一致。真实环境重新发布后触发 Gateway 正式续期，核对 Secret 与所有 Agent 的证书序列，跨越实际 15 分钟旧证书失效窗口持续检查 8 类指标，并验证仪表盘。

实现与发布证据保存在 `artifacts/collector-identity-20261110/`。

## 关联的一小时查询回归

轮换期间真实浏览器复验再次遇到 API Server 后部面板累计耗尽整板 256 MiB 扫描预算。ClickHouse `system.query_log` 显示已有排序投影已完整物化，但带外部表 JOIN 的样本查询没有使用投影。只读移除该 JOIN 的候选计划使用了 `argus_histogram_source_metric_time`；原始历史、授权与预算均未改动。

本轮一并调整共享 PromQL 读取：对已有小授权来源集合（最多 128 个来源键）的常量集合路径，使用来自已授权 FINAL 元数据的 UUID 参数精确筛选，并通过原生常量映射维持原先的来源优先级排序，避免外部 JOIN 阻断投影。大来源集合保留外部表路径。额外验证同点来源优先级、租户/资源/来源/命名空间围栏、Histogram/Summary 语义与最近完整一小时页面，不提高查询预算。

## 发布与回归

- Server、Worker、Direct Executor、Ingest 使用 `local-20261110-collector-identity@sha256:0f0688c28fbbeee0ee046adcf8ebc12296401fbcc70409aa9d5bd20af4ff3f14`。
- 目标集群 Collector 使用 `local-20261110-identity@sha256:ce954bb6c833b473b13e25755373ac2bfadeae96a33788e502b891881eb7b5b5`，通过普通企业 Preview/Confirm 升级并收敛至 revision 25。运行时默认镜像也更新到同一摘要，三个配置生成进程重新加载，避免后续安装拿到旧扩展。
- Query 使用 `local-20261110-identity-projection@sha256:c6b660841f30561aeb3b010b218eebd4f10d00ffcb2d216516b4ec7eaecaedf6`。仓库前缀分别为 `host.docker.internal:5001/argus/argus-backend` 与 `host.docker.internal:5001/argus/argus-otelcol`。
- 全部新 Pod 的精确 imageID 与源码指纹已核对，Web 保持上轮悬浮提示版本；6 个原有 PVC 绑定与资源身份保留。
- Go 相关测试 418 项通过，125 项因未提供各自独立实机依赖而跳过；Linux 真实 Collector 身份扩展另有 17 组通过、无跳过。类型配置由实际 Collector 验证 10 种场景。
- 临时命名空间内的隔离 ClickHouse 36 项通过，无跳过；不在本轮运行依赖其他新鲜原生接收器捕获的 12 项专项门禁。新回归包含 400 条逻辑序列的双来源优先级、80 万条交错无关样本，扫描 6978 行 / 675948 字节；跨租户、资源、来源、命名空间及大来源集合的既有围栏也通过。
- 严格 HTTPS、正常登录、无 mock 的真实页面 E2E 8 项通过，无重试、无跳过：中英明暗、8 类数据状态、5 个组件仪表盘共 30 个面板，以及实际工作队列悬浮框。节点与 API Server 的最近完整一小时 12 项冷查询均成功，实测约 0.74 / 2.64 秒，整板 256 MiB 扫描上限未变。这是当前窗口实测，不能拿此前未完成的响应与完整工作量直接比较速度。
- Go vet、TLS 安全检查、隔离契约生成检查通过。Windows 首次契约 lint 遇到 Node/libuv 退出断言；在 CI 环境设置下完整重跑通过，未绕过 lint 或生成比对。

证据：`release.json`、`collector-release.json`、`default-collector-image-release.json`、`query-release.json`、`source-audit.json`、`go-test-summary.json`、`linux-identity.log`、`linux-renderer.log`、`clickhouse-integration.json`、`projection-live-1h-summary.json`、`browser-review.json`、`real-browser/`、`final-contracts.log`、`final-tls-security.log`。

## 轮换验收的证据边界

首轮真实发布发现后台 Worker 未同步，旧配置在验收中被拦截；完整更新配置生成路径后重新升级。第一次实际轮换进一步暴露两跳序列号相等限制，随后补充了注册窗口与完整身份一致性校验。

后一次测量中，8 类样本持续推进，但最终导出失败计数增加，验收仍判失败并保留原日志/报告。持久台账记录最后一次失败为北京时间 18:56:19，位于该次启动约 5 分钟后，之后未继续新增；不能将整段结果说成零丢失。后续复验在队列和在途请求连续为空后建立新基线，原有累计失败记录和旧数据不清零；继续严格检查完整 17 分钟的增量、实际日志和旧证书拒绝结果。最终轮换结果以 `identity-renewal-review.json` 为准。

测试只临时提前 Gateway 本地续期阈值，使用真实签发器与真实 15 分钟注册重叠窗口，不修改时钟或服务端失效规则。`collector_identity_renewal.py --restore` 可恢复中断的测试；正常结束也会还原原始 Gateway 8h/5m 设置并确认所有 Agent 和 Secret 的证书一致。测试命名空间和只含公共叶证书的临时文件均按所有权清理，私钥不离开目标节点。

## 最终轮换结果

最终复验稳定运行 **1021.14 秒**，观察到 **18 代 Gateway 证书**，完成 61 次连续采样检查，8 类指标全部持续更新；最终导出失败与入队失败增量均为 0，实际日志没有身份拒绝或最终丢弃。Gateway 服务端证书在进程未重启的情况下完成热加载。

原证书及进程启动时的证书均仍在物理有效期内，但跨过注册重叠窗口后，严格 TLS 请求分别返回 **401 / 401**；当前证书继续通过。所有现有 Agent 最终与 Gateway、固定 Secret 的客户端与服务端证书一致。原始 8h/5m 配置已恢复，再次取得 8 类恢复后的新样本。

最终 `argusctl verify` 20 项通过；evaluation 环境仍保留 NetworkPolicy 未强制拒绝、未检测到外部 Egress Gateway 的原有能力提示。两套测试命名空间和临时公共证书文件均已删除，默认 Collector 镜像通过普通企业 API 核对，非本轮目标的部署与 6 个 PVC 保留。

最终证据：`final-review.json`、`identity-renewal-review.json`、`identity-continuity.jsonl`、`gateway-whole-renewal.log`、`gateway-continuous-renewal.log`、`identity-test-state.json`、`identity-restored-samples.json`、`final-collection-status.json`、`final-health/verify.json`。本轮修复面向后续正常轮换；之前停采或最终丢弃造成的历史空档不等于已恢复，累计失败台账保持。
