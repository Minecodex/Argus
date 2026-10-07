# 完成度复核后的补齐记录

日期：2026-10-07。对应[再次复核](./completion-review-20261007.md)的代码、验收和本地交付缺口。状态随实际结果更新，未执行项目保持待验收。

## 实现

- 新增平台管理员专用 `GET /platform/overview`。在 PostgreSQL 的只读 Repeatable Read 快照中取得完整企业计数、占用 Sandbox 配额的会话数、尚未登录的直接授权管理员数，以及近 12 个 UTC 月份的已记录 Sandbox 用量。聚合覆盖全部企业，不依赖明细接口的分页或 200 条上限。
- 使用现有 `sandbox_usage` 月度事实，未新增日度/CPU 推算。概览与 Sandbox 用量共用查询缓存和图表；口径改为月度，未记录时显示空状态，CPU 消耗未提供时显示未知。审计与概览分别显示查询状态。
- 共享 `QueryBoundary` 支持必需依赖、403、404、暂时失败、重试和能力不可用。组织部门的成员数、Sandbox 配额等组合结果必须在所有必需读取成功后展示；列表状态边界不销毁编辑表单。
- 组织部门/角色/服务账号、远程访问治理子页、连接凭据/托管账号，以及 Sandbox 镜像/Profile/配额/会话/用量接入共享查询反馈。模型治理摘要和遥测管理的组合查询采用同一规则。
- 草稿 workspace 统一处理首次读取、失败和重试；按草稿 ID 隔离编辑状态，重试不能覆盖已缓冲修改。统计图编辑直达/刷新不再无限显示加载中。
- 完整真实回归暴露 APM 覆盖统计和分组结果重复扫描同一批 Span，会浪费仪表盘共享扫描预算。现通过分组后的窗口汇总在一次查询中返回覆盖统计和图内行；过滤、显式 limit、缺失维度及计数保留原义，预算没有上调。实际 ClickHouse 回归验证仅一次扫描及既有样本语义。
- 审批策略使用 Workflow Domain 安装后的真实读写接口；加载失败或无权限时禁用新建，并显示共享反馈。进一步追溯确认，real.ts 的初始不可用方法会被 Workflow Domain 覆盖，不能据此把审批策略判为未实现；本记录已修正此前判断。

## 验收进度

| 检查 | 当前状态 |
| --- | --- |
| 契约生成、校验与漂移检查 | 通过；新增 OpenAPI、Go/TS 类型与 mock/real adapter |
| 类型、样式、表单和国际化 | 通过 |
| 前后端相关测试 | 通过；共享查询依赖/404、草稿加载重试与缓冲保护、API 聚合响应及失败保留已有证据 |
| 正式构建与包体积 | 通过 |
| 三种桌面尺寸、中英、明暗及业务回归 | 140 通过，1 移动端范围外跳过 |
| 真实 Kubernetes 的完整 PlanV2 和新增子页/编辑初始状态 | 60 个场景跨轮次关闭：完整轮次 44 通过；失败/未运行的 16 项复验 16/16 通过 |
| 超过 200 条数据的实际聚合与跨门户拒绝 | 通过；205 个企业、615 次会话、6355 秒均完整聚合，企业凭据拒绝，夹具已清理 |
| 临时资源清理与正式服务恢复 | 通过；两个集群轮次和 ClickHouse 语义测试 Namespace 剩余 0，19 项正式工作负载恢复就绪 |
| 本地正式升级与真实访问验证 | 通过；`portal-20261007-r2` 已安装，正式安装检查 20/20；两门户浏览器 HTTPS 200，登录提交按钮 32px，平台仍已初始化 |

新增真实浏览器用例覆盖组织与 Sandbox 非默认 Tabs、概览、读取依赖、加载/空结果/503/403/重试；中英与明暗分别运行。草稿初始读取覆盖 503/403/404 和恢复，确认没有发布请求。审批策略另验证实际服务端配置、失败、拒绝和恢复。

契约生成期间遇到 Windows Node 退出断言和暂时文件锁，保留失败日志；恢复生成目录后，完整生成、后端契约与漂移检查通过。证据位于 `artifacts/frontend-redesign/overview-*`，不覆盖此前失败轮次。

首次完整集群轮次 `ui-20261007n`：44 通过、12 失败、4 未运行。8 项新测试的草稿响应字段/审批 Domain 判断有误，已修正；4 项自监控因重复扫描触发预算，串行后继的 4 项未执行。`ui-20261007o` 复验对应 16 项全部通过，文件/工具协议、6 类在途故障及安装 20/20 通过，最终退出码 0。全部 60 项的通过覆盖由两轮共同构成，不能称为单轮 60/60 无失败。临时 ClickHouse 语义测试 Namespace 已删除。

## 正式交付与证据

当前正式部署保持 Docker Desktop 集群中的 `argus-system`、`argus-ingress`、`argus-sandbox`；保留原数据与初始化状态。企业门户 `https://argus.dev`，平台门户 `https://platform.argus.dev`。`argus-web` 和后端/遥测工作负载均为 `portal-20261007-r2`。

- 源码：`overview-types-final.log`、`overview-lint.log`、`overview-tests.log`、`overview-contract-check.log`、`overview-apm-backend-final.log`。UI 74、API Client 99、企业门户 160、平台门户 17、Template 2 项通过；289 个改动源文件无 2000 行超限。
- 桌面与构建：`overview-mock.log`（140 通过/1 范围外跳过）、`overview-build.log`、`overview-bundle.log`。新增 APM 代码不影响前端包，生产后端镜像已重新构建。
- 真实数据：`overview-k8s-retest/platform-overview.json`，`overview-apm-clickhouse-final.log`。源事实覆盖、去重、结果限制、错误率、分位数和缺失维度均验证；扫描预算没有放宽。
- 部署：`overview-k8s.log` 保留首次失败；`overview-k8s-retest.log`、`overview-k8s-retest/result.json`、`verify/verify.json`、`planv2-inflight-faults.json`；`overview-cleanup-proof.json` 证明清理/恢复。
- 正式安装：`overview-formal/images-build-r2.log`、`images-load.log`、`install.log`、`verify/verify.json`、`browser-proof.json` 和两门户登录截图。使用当前发布的 CA 通过 Python/Chromium 验证证书链；Windows curl 对私有 CA 使用撤销检查 best-effort，未关闭证书链验证。

本轮完成上述复核缺口及本地正式交付。实际模型质量未重新测试；本机 NetworkPolicy 强隔离、外部 Egress Gateway 和独立 Sandbox Runtime 保持既有范围声明，不推定为通过。
