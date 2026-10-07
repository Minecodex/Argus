# 弹层、选中值对齐与统计图场景库

日期：2026-10-07。状态：本轮弹窗、选值对齐和场景库已修正、验证并部署。当前本地正式版本 `portal-20261007-r5`；细粒度采集插件配置仍属讨论范围。

用户指出新建仪表盘抽屉空白过多、全局下拉选中值未垂直居中、添加统计图只有少数普通按钮、关闭按钮位于标题下方。本轮将短表单迁入内容自适应的共享 FormDialog；Dialog/Drawer 公共标题区明确横向排列；抽屉的定位容器占满视口，宽度应用到实际抽屉；Select 选中值的文字继承公共字号并垂直居中，选中状态不复制选项的描述行。

添加统计图采用 Demo 的场景卡片、信号过滤与搜索，另提供全部十一类指标图型。常用场景覆盖 CPU、内存用量/比例、负载、文件系统、网络/磁盘速率、K8s 节点/Pod、请求速率/P95、日志、Trace 与 APM。每个场景声明来源、真实查询和采集要求；可提前配置，但不能把模板存在或发行包包含组件显示为数据已就绪。选择模板仅写个人草稿，不自动修改任何资源的 Collector。

## 采集能力核对与讨论边界

- 图型是结果展示方式，十一类图型不要求十一种采集插件。直方图/P95 需要应用提供累计分桶；Trace/APM 需要接收 Span，继续采用已接收样本口径。
- 当前 Collector 锁定 0.133.0，实际发行包包含 hostmetrics、kubeletstats、k8s_cluster、OTLP、Prometheus、filelog 及已有 Trace Receiver。主机已有 Profile 级能力开关，K8s 向导会选择节点/容器、集群与采集器自身三个 Profile；不能宣称已经存在任意插件/指标级配置。
- host-basic 默认渲染 cpu、memory、filesystem、network，Linux 另有 load。CPU 和内存利用率在上游默认为关闭，当前配置未覆盖；本轮在同一已有 scraper 中显式开启这两个已支持指标，既有累计指标继续保留。存量资源只有走配置预览/确认或升级应用新配置后才会生效。
- disk scraper 组件存在于 hostmetrics，但当前 Profile 没有启用；磁盘读写场景明确提示这一要求。请求速率/P95 的应用指标名称只是通用模板约定，必须匹配实际 Catalog，不是假定所有应用都会上报。
- 建议下一轮配置分两层：先选采集集成/Profile，再配置 scraper、指标和目标；显示发行包支持、配置启用、历史已接收、当前有数据的独立状态。主机与 K8s 复用同一服务端目录与校验；接入参数、凭据、平台支持、RBAC/Collection Claim 和配置预算必须进入现有一次预览确认流程。数据库与中间件另按目标及凭据逐个接入，不增加不可执行的空开关。

依据：项目 `internal/otelcol/configbundle`、`deploy/otelcol/builder-*.yaml`，以及上游锁定版本的 [CPU metadata](https://raw.githubusercontent.com/open-telemetry/opentelemetry-collector-contrib/v0.133.0/receiver/hostmetricsreceiver/internal/scraper/cpuscraper/metadata.yaml)、[Memory metadata](https://raw.githubusercontent.com/open-telemetry/opentelemetry-collector-contrib/v0.133.0/receiver/hostmetricsreceiver/internal/scraper/memoryscraper/metadata.yaml)、[Kubelet metadata](https://raw.githubusercontent.com/open-telemetry/opentelemetry-collector-contrib/v0.133.0/receiver/kubeletstatsreceiver/metadata.yaml)。查询语言、资源授权、来源身份和个人草稿/统一发布边界不变。

## 验收范围与证据

本轮产物位于 `artifacts/frontend-redesign/dialogs-presets`。与用户截图的[变化对照](../../../artifacts/frontend-redesign/dialogs-presets/comparison.html)采用实际 React 页面，明确区分真实接口和 mock 图型选择。

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| 类型、样式、导入、表单与国际化 | 通过 | `typecheck-final.log`、`lint-latest.log`、`unit-final.log` |
| 单元与契约 | UI 75、API 99、企业 163、平台 17、模板 2；相关 Go 与契约通过 | `unit-final.log`、`backend-final.log` |
| 正式构建与包体积 | 通过 | `formal/images-build-retry.log`、`bundle.log` |
| 新布局矩阵 | 12/12；三种桌面尺寸 × 中英 × 明暗 | `layout-accepted.log`、`layout-accepted/` |
| 所选既有回归 | 首轮 113 通过、2 失败、1 移动端范围外跳过；对应等待失败的六个尺寸组合复验 6/6。115 个所选桌面场景均有通过覆盖，不是单轮无失败 | `regression.log`、`regression-retest.log` |
| 临时 Kubernetes 真实浏览器 | M2 身份 3 项＋本轮弹窗/草稿/场景库 4 项，7/7；实际创建接口 201、双语明暗、选值/关闭位置和 Axe serious/critical 为 0 | `k8s-final.log`、`k8s-final/playwright-m2/` |
| 安装、清理与恢复 | 20/20；结果 passed；本轮测试 Namespace 0，19 项原工作负载恢复就绪 | `k8s-final/verify/verify.json`、`k8s-final/result.json`、`cleanup-proof.json` |
| 本地正式交付 | r5 安装和复核 20/20；19 项工作负载就绪；企业/平台 × 中英 × 明暗 8 组 HTTPS、32px 和 Axe 检查通过，初始化仍为 initialized | `formal/install.log`、`formal/verify/verify.json`、`formal/workloads.json`、`formal/browser-proof.json` |

M2 验收覆盖真实创建与编辑入口，不代表本轮重新执行全部三信号取数、十一种图型的真实结果或实际模型质量评测；此前 PlanV2 的查询/文件/发布证明仍保留原有范围。Collector 利用率开关在渲染配置和三种平台测试中验证；本轮没有自动升级现有主机 Collector。

正式访问仍为 `https://argus.dev` 与 `https://platform.argus.dev`，原初始化状态与数据保留。维护的 `deploy/.cache/argus-install-consolidated.yaml` 已同步 r5，上一份 r4 配置保留在 `formal/install-config-before-r5.yaml`。这里的“正式”指当前本地使用部署，不改变 evaluation Profile 的网络隔离验收边界。

## 本轮失败与修复记录

- 第一次真实安装暴露最小 M2 的安装制品与本轮签名根不一致。安装器必需制品始终使用本轮临时签名根，增加独立签名和验签测试。
- 第二次真实安装暴露最小 M2 仍使用占位制品 URI。M2 依赖清单明确启用真实 TLS Artifact Server，安装制品按实际地址生成；第三次运行通过。两次失败日志分别保留为 `k8s.log`、`k8s-accepted.log`，不算成功证据。
- 场景库 CSS 原来依赖编辑页按需加载，首次打开会缺少三列布局。样式移入场景库自身入口，并用卡片实际坐标及固定底部操作检查首开状态。
- Docker Hub OAuth 网络超时的正式镜像失败日志保留，后续重试构建和节点加载通过。没有绕过 TLS、权限或可访问性断言。
