# B10a Pod 日志与终端反馈修复（2026-11-11）

用户反馈日志应打开即输出，以及 Pod 终端连接失败。本轮只修正共享日志/终端体验，保留当前 B10a 环境、接入身份及 Kubernetes 原生权限。

## 日志打开与滚动

- 点击 Pod 行「查看日志」即读取当前容器的最近日志，不再先显示参数表单或要求第二次打开会话。
- 默认跟随最新内容；向上滚动查看历史时保留阅读位置，新日志仍接收；回到底部自动恢复跟随。共享 `LogViewer` 同步提供暂停/继续、复制和下载。
- 容器、上次终止容器日志和初始时间窗/行数保留在工具栏及「日志选项」。切换读取目标或应用选项时，关闭旧会话并创建新的 UID 绑定会话。
- 关闭窗口、切换 Pod、异步创建/读取迟到均正确处理：不泄漏未关闭会话、不将旧 Pod 响应混入新目标。页面内存保持有界，关闭后清空，票据不进入持久浏览器存储。
- 暂停滚动或冻结显示不停止授权复验；有界读取完成后仍检查会话，拒绝后清空已显示正文。

这项行为使用既有 create/attach/read/close API；未改变读取权限、脱敏、签名游标、十五分钟期限和数据源，也未增加平行日志采集链路。

## 终端实际失败原因

只读查询最近会话元数据，用户失败的对象为 `argus-telemetry/argus-otelcol-agent-fvc4n`，实际结束原因为 `KUBERNETES_API_FORBIDDEN`，传入/传出均为 0。当前测试接入 ServiceAccount 仅在 `b10a-workloads` 被授予终端权限，Kubernetes 正常拒绝采集器命名空间的 exec。

另外，对该自有集群使用管理员身份直接执行 `/bin/sh`，原生运行时明确返回文件不存在。当前 Collector 镜像是 distroless，并不包含普通 Shell。更换为用户具有平台权限的账号，也不能凭空给镜像增加 Shell。

界面现在显示原生权限拒绝、后端/Shell 前件、权限/身份变化、到期和空闲结束的相应提示；轮询持久会话原因可纠正传输层的笼统错误。Shell 选择项说明镜像必须包含所选 Shell；失败后立即显示断开状态，提供「重新连接」，不会停留在绿色等待连接或空白终端框。

普通业务 Pod `b10a-workloads/b10a-api-*` 已通过正式页面打开真实 PTY，执行 `cat /config/mode` 得到实际 `updated`，跨多个心跳周期继续执行，输入 exit 后重新连接获得新会话，再执行并正常关闭。原来用户报告的 Collector Pod 也通过真实页面复现拒绝并核对明确提示。

本轮没有扩大原生 RBAC、加入调试容器或改造 Collector 镜像。需要测试交互终端时，在 B10a 中选择业务 Pod；无 Shell 镜像的普通终端能力仍受原生镜像限制。

## 验证与发布

- 共享 UI 全套 157 项、日志自动创建/晚到清理/授权失效与暂停正文清退的 4 项回归通过。
- 中英 × 明暗的日志和终端 Mock E2E 共 8 项通过；这部分明确使用夹具，不作为真实连接证据。
- 另有 6 项实际部署 E2E：四种语言/主题的日志自动打开、真实 HTTP 产生日志、鼠标滚轮位置保持/恢复；实际 PTY 持续命令与退出重连；报告中的 Collector Pod 原生拒绝。正常登录、严格 TLS，均无重试、无 mock。
- 类型、ESLint、双语、样式、格式检查和三个门户 real 构建通过。生产 Web 源码从当前已部署 Tooltip 版本冻结，仅叠加七个会话 UI 文件；未附带当前工作区的无关变更。
- 最终 Web 镜像为 `local-20261111-b10a-sessions-v3@sha256:cd9c6aa391f8c9a320322be2effef1d578fff466e62d3cd4606b046f26337fe3`；精确镜像摘要和实际 Pod imageID 记录于 [发布证据](../../artifacts/plan9/p9-b10a-20261111-01/session-repair/release.json)。仅更新 Web Deployment，后端/Collector 镜像和六个原有 PVC 绑定保留；正式健康检查 20 项通过。

[实际页面证据](../../artifacts/plan9/p9-b10a-20261111-01/session-repair-browser.json) · [原生无 Shell 诊断](../../artifacts/plan9/p9-b10a-20261111-01/collector-terminal-diagnosis.json) · [本批查看路线](./batches/B10a-review.md)。
