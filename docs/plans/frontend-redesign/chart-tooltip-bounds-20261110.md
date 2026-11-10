# 图表悬浮提示边界与序列名称

2026-11-10，实际 Kubernetes Controller Manager 仪表盘悬停后，轴提示一次展开大量长标签，超过视口并遮住整页。

## 原因与统一修正

共享 ECharts 提示原先只约束最外层最大宽度，默认内容包含不换行的长名称，也没有高度与滚动边界。名称生成把所有业务/资源标签值连续拼接，Pod UID、重复的系统信息也进入显示名称。

本轮统一在 `@argus/ui` 处理：

- 提示保留当前 document.body 的浮层，继续支持裁剪卡片、弹窗与 iframe 文档边界；默认和数值定位按视口约束，重绘后仍持有正确图表元素。
- 最大宽度 440px、最大高度 320px，由共享设计 token 控制，小视口进一步收缩。时间与序列数量固定展示，列表独立纵向滚动，名称单行省略，数值单独对齐；鼠标可以进入列表滚动。
- 空内容浮层隐藏，避免给未激活的 ECharts Tooltip 添加 padding 后产生空白可见块。鼠标离开正常关闭，卸载/隐藏仍沿用现有生命周期。
- 数值轴内容使用 DOM textContent，颜色沿用图表语义，格式化保留单位、精度、真实零和缺失值。显式 formatter 以及 Heatmap、Graph、状态区间、Pie/Gauge 的原生内容保持。
- 名称优先采用真正变化的队列、Namespace、Pod、容器等维度，省略不用于区分的公共信息和完整 UID。明确 display_name 保留；可读名称冲突时添加稳定短标识，避免图例同名导致不同来源、Pod 代次或查询 Target 混在一起。

变化仅作用于显示。完整标签、返回样本、行顺序和钻取原始行保留，不改变查询、聚合、授权或接口契约。

## 验证

共享单测覆盖名称精简、冲突身份、原数据不变、文本转义、零/缺失格式化、视口边界及多维图型原生格式保留。浏览器使用 150 条长名称序列，覆盖中英明暗、窗口滚动、列表滚动到最后一条、长文本省略、悬停离开、卡片裁剪和弹窗覆盖。

部署后使用正常企业登录和严格 HTTPS，在真实 Controller Manager 工作队列图上检查名称、可见范围与滚动。证据目录：`artifacts/chart-tooltip-20261110/`。

- UI、企业门户与 Template Runtime 单测共 1383 项通过；类型、ESLint、样式检查通过。
- Mock 浏览器 6 项通过，真实部署中英明暗 4 项通过，均无重试、无跳过。真实数据包含 55 条工作队列序列，提示保持在视口内并可独立滚动，名称不再包含完整 UID。
- Web 镜像：`host.docker.internal:5001/argus/argus-web:local-20261110-chart-tooltip@sha256:dceb29f04a53cfe5adf7e69fcdca6b11e55c4d40dc33b3fc98502fa6e7e6ee12`。`release.json` 验证新 Pod 的 imageID，仅更新 Web，保留其他部署与 6 个 PVC；后端和 Collector 版本未变。
- 自动化与截图：`fixture-browser-final.log`、`real-browser-final.log`、`real-browser-final/`；构建源指纹为 `source.json`。
- 发布后 `argusctl verify` 的 20 项检查通过（`health/verify.json`）。这是 evaluation 环境验收；报告仍列出 NetworkPolicy 未强制拒绝与未检测到外部 Egress Gateway 的网络能力提示。

## 验收时发现的独立采集问题

真实复验最初没有当前数据：8 类指标在 2026-11-10 13:18（北京时间）停止，早于本次修改。现场日志为 `telemetry identity or payload rejected`，并伴随 Trust Bundle 同步 401，不能用 Tooltip 样式解释。

现有 Gateway 与 Agent 使用同一 Collector 身份，但分别续期。Agent 在 Gateway 续期约 9 秒后再次续期，服务端按主体收窄旧证书的重叠窗口，15 分钟后 Gateway 的证书被拒绝。证书有效期仍未到期，原有本地续期判断未及时恢复。

本次仅做现场恢复：核对证书链、身份、公钥一致性后，将同一身份已生效的 Agent 叶证书同步至 Gateway 的运行目录并重启该 Gateway Pod。未更换私钥、CA、身份或授权，未放宽证书校验。8 类指标均产生恢复时间之后的新样本，再执行上述真实浏览器验证；停采区间未伪造或补齐。证据：`certificate-recovery.json`、`collections-recovered.json`、`certificate-recovery.log`。

这是运行恢复，未在本轮 Tooltip 补丁中根治多副本续期竞争。后续 PKI 修正需要统一续期所有权与副本证书同步，并验证轮换后长连接重建；不能将本轮页面验收当作续期机制已修复。

后续轮次已完成统一续期、合法证书交接、长连接重建及真实失效窗口验证，详见 [Collector 统一续期与长连接重建](../../plan9/13-collector-identity-renewal-20261110.md)；本节保留当时仅做运行恢复的证据边界。
