# B10c 扩展资源的命名空间范围与错误提示

2026-11-13，用户在 Plan9Check 默认“全部已授权命名空间”入口看到“暂时无法读取集群资源”。请求 ID 为 `afbd31eb-9a75-44cc-bdf9-376e65db2fb9`，Server 实际记录 HTTP 403 / `KUBERNETES_API_FORBIDDEN`。

## 原因和边界

Argus 的资源数据授权和 Kubernetes 接入凭据的原生 RBAC 都必须满足。本批企业用户可查看整个集群；原生凭据通过 `b10c-workloads` 内 Role 获得 Plan9Check 实例权限，未获得这类实例的跨命名空间 list 权限。因此：

- `namespace_mode=all_authorized` 会走跨命名空间查询，原生拒绝，返回 403。
- `namespace_mode=explicit&namespace_ids=<业务 Namespace ID>` 可以读取真实 `b10c-check`。
- 原先五项真实页面自检覆盖了已选择业务 Namespace 的正向路径，漏掉默认全部范围的拒绝与恢复路径。本次补测，不把旧自检改写为已覆盖。

前端全局错误翻译只查 `errors.codes.<code>`，忽略后端已有的业务 `message_key`。该错误码的中文/英文文案已在 `kubernetesWorkbench.errors.forbidden` 注册，却落入了通用不可读提示。

## 修复行为

- 在共享 API Client 中统一错误翻译策略：先查稳定错误码，未注册时查已注册的业务文案；未知错误继续使用页面安全的默认提示，保留请求 ID，不展示任意异常消息。企业门户与平台门户使用同一实现。
- 资源工作台准确显示“当前用户或集群接入身份无权访问该范围”。对于命名空间资源的全部范围 403，补充命名空间选择引导和直接选择按钮，复用现有受授权目录、共享选择弹框及 UID 绑定。
- 显式选择 Namespace 后保留资源类型、名称/标签/状态筛选，重置旧分页游标和详情，发起新范围查询。未通过原生权限的范围继续拒绝；不吞掉错误伪装空列表，也不返回没有完整性说明的部分结果。
- 此项为展示和恢复流程修复，原生 Role/ClusterRole、平台数据授权、后端 API、schema 和 Server/Worker 镜像均不改变。

可以直接打开 [业务范围 Plan9Check](https://argus.dev/kubernetes/01a1d448-13b5-7728-afb9-3eabbf65e11b?tab=resources&kind=custom%2Fb10c.argus.test%2Fv1%2Fchecks%40fd508ad5-2737-44f2-84a1-a4dbacf478ff&namespace_id=1843e815-6df1-5b4c-846c-05a112fe6607&namespace_name=b10c-workloads)。默认全部范围仍按真实权限提示；需要扩大权限时应由管理员明确修改接入授权，不由读取错误触发自动扩权。

## 验证和发布

新前端镜像：`host.docker.internal:5001/argus/argus-web:local-20261113-b10c-scope-error-v2@sha256:42ae17fbc1943e24d5431fdddf05711017a02585ff27d40a8843916e7da9bab8`。冻结源码、完整 real build、实际 Pod imageID、rollout 和六个正式 PVC 的保留均已核对。

现场发布与核对已完成。共享错误展示单元 7/7、双语资源 15/15、类型编译/ESLint/样式/i18n 检查、真实页面 5/5、正式健康 20/20 通过；四套指标仪表盘 21 个实际面板复验通过。正常 API 仍为全部范围 403、指定业务范围 200，原生 `can-i` 同样证实全局拒绝、业务 Namespace 允许、`kube-system` 拒绝。六个正式 PVC、其他部署及全部后端镜像保留。

本批实际验证记录放在 `C:/Users/23574/.codex/artifacts/argus-plan9/p9-b10c-20261113-01/`：

| 验证 | 证据 |
| --- | --- |
| 原请求与全部/指定 Namespace 的正常 API 对照 | Server 日志、`custom-all-namespace-before.json` |
| 错误码优先、业务文案回退、未知文案保护与请求 ID | `scope-error-api-client-unit.log` |
| 双语资源及静态检查 | `scope-error-enterprise-i18n.log`、`scope-error-static.log` |
| 中英/明暗、真实 403 → 明确提示 → 选择业务 Namespace → 实际实例详情，保留名称筛选及 URL 上下文；真实日志/终端回归 | `browser.log`、`browser-review.json`、`browser/` |
| 新镜像与正式数据保留 | `web-release.json`、`final-preservation.json` |
| 正式环境复验 | `formal-health-scope-error-v2.log` |
| 本轮完整交付结论 | `namespace-scope-feedback.json` |

原有发布、成功页面及健康证据保存在 `before-scope-error-fix/`。最初单元命令从仓库根目录按路径后缀筛选，误扫描了历史冻结源码；改为在对应包目录运行，保留原命令日志，不将历史目录解析失败计为生产回归失败。B10c 继续保留运行，人工验收待用户确认。

后续表格操作列颜色修复 v3 已包含本项修复，现场 Web 版本以 [B10c 查看记录](./batches/B10c-review.md) 和最新 `web-release.json` 为准。
