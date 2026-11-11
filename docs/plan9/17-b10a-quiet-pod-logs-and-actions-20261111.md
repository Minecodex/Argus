# B10a 安静容器日志与操作栏复查（2026-11-11）

用户再次反馈截图中的 Collector 日志为空、终端仍无法进入，以及按钮未对齐/缺少间隔。此次直接使用截图中的 Agent Pod 验证，不用有持续输出的业务 Pod 代替安静容器的日志结果。

## 实际原因

原生 `kubectl logs --tail=8 --timestamps` 能读到 Agent 的启动日志，最后输出时间约为 UTC 09:23，早于查看时间数小时。页面默认 `since_seconds=300` 将这些真实历史记录排除，因此 HTTP 读取成功但正文为空。上一轮的业务 Pod 持续输出测试没有覆盖这个场景。

用户最近的 Agent 终端尝试仍由 Kubernetes 以 `KUBERNETES_API_FORBIDDEN` 拒绝。接入身份没有该命名空间的 exec 权限；该 Collector 又是不带 `/bin/sh` 的精简镜像。用户已明确选择先标清普通终端支持范围、验证业务 Pod，而不增加临时调试容器。

## 共享读取与界面修正

- 默认按最近 100 行读取保留日志，不预设五分钟时间窗。原有 `since_seconds` 增加 0 的明确语义：不加时间过滤；显式正值仍按指定时间窗读取。OpenAPI、生成契约、表单约束与 Mock 保持一致。
- 原生首次请求使用 Kubernetes 的 tail，避免从早期大量日志的前 256 KiB 中错误截取所谓“最近行”。为了保留同一秒内重复行的准确续读，再有界读取末尾秒的完整前缀，计算既有私有 HMAC 水位；并发新增进入当前快照，轮转/身份变化继续拒绝。超过边界预算明确停止跟随。
- 行数 1～1000、单次字节上限、十五分钟会话、UID/容器代次固定、读取前后身份校验、当前授权复验和脱敏均保持。没有读取权限扩大或回填数据。
- 真正没有输出时显示独立空状态，不把“没有日志”伪装为第 1 行正文。显式时间窗下的空状态说明该范围，并提示设为 0 查看保留历史。
- 日志和终端内容使用统一纵向间距；日志下拉、上次日志开关和选项按钮按控件底边排列。创建、重连和关闭统一放入共享 `Dialog.footer`，同排、右对齐并使用既有 action-gap。
- 终端在创建前明确展示 Shell 和原生 exec 权限两个前件，无 Shell 的 Collector 不作为可交互业务容器。业务 Pod 的实际 Shell 继续验证；没有放宽 RBAC 或安装调试容器。

布局复用 [HeroUI v3 Modal 的 Body/Footer 组合](https://heroui.com/docs/react/components/modal.mdx) 和项目现有共享封装，颜色、间距及控件高度使用设计 token。

## 发布与验收范围

后端基于冻结 Schema52 叠加日志读取修复，未运行正式迁移；Web 基于上轮冻结源码的共享界面修复构建。运行镜像和精确摘要：

- 后端 `local-20261111-b10a-quiet-logs-v1@sha256:e69ac4b3601cdcd06b1a84b76dde4111cd19ee9459a00b7491bf2de6b33dd1e9`，仅滚动 Server、Worker、Direct Executor、Connector Gateway。
- Web `local-20261111-b10a-quiet-logs-v1@sha256:300bfd909d138033929b427cb86f027b6217a1f35daa012d79b02cffa9e27673`。
- Ingest/Query/Writer、原生 Collector 和六个正式 PVC 绑定保留。

原生 tail 回归覆盖安静历史、同时间重复行、首读期间新增、轮转、边界预算和身份拒绝。临时 PostgreSQL 另验证默认 0、公开 HTTP/CSRF/一次性票据、游标和撤权；临时数据库结束后删除。

最终实页矩阵覆盖中英明暗的原 Agent 启动日志、时间早于五分钟的真实正文、工具栏/底部按钮几何间距、原生拒绝提示，另验证业务 Pod 的真实命令与退出重连。自动化使用正常登录、严格 TLS、不注入日志/PTY mock、不制造 Collector 输出。

首次实页轮次遇到已过期的自动化登录态，随后正常登录刷新；另一个开发轮次纠正了选择器触发器的角色定位。失败日志保留，最终轮次单独记录，不把中间失败写成通过。

[最终查看记录](./batches/B10a-review.md) · [原生读取与 PostgreSQL 门禁](../../artifacts/plan9/p9-b10a-20261111-01/latest-tail-postgres.json) · [本次实际页面](../../artifacts/plan9/p9-b10a-20261111-01/quiet-pod-browser.json) · [精确 Web 发布](../../artifacts/plan9/p9-b10a-20261111-01/quiet-pod-repair/release.json)。
