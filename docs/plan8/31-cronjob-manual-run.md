# CronJob 模板手动运行

2026-10-19，K8s 工作台的 CronJob 列表新增“运行一次”。用户填写独立 Job 名称，系统使用当前源对象生成差异预览、确认并异步创建 Job；源 CronJob 的计划和暂停状态保持原值。

源引用固定名称、UID、resourceVersion，且与目标 Job 使用同一个已授权 Namespace UID。完整模板在执行端读取，不经过浏览器/模型/公开计划往返。源对象被替换、更新或删除时拒绝写入，重新读取并预览后才能执行。继承模板仍检查当前主机特权、ServiceAccount 身份等许可。

Job 复制模板 spec、labels 和 annotations，添加手动实例标记及 CronJob controller owner；不添加需要额外 finalizer 权限的 blockOwnerDeletion。结构参考 [kubectl 原生实现](https://github.com/kubernetes/kubectl/blob/master/pkg/cmd/create/create_job.go)，本轮通过原生 API 库执行，无额外 CLI 服务。

手动创建不等待调度时间，也不自动等待、替换已有 Job。暂停和定时任务并发策略的影响在表单中说明；操作“已生效”表示 Job 创建并达到现有就绪观察条件，实际作业成功/失败继续查看 Job 状态。本轮实际测试额外等待了 Job Complete，不能用 API 2xx 代替该结果。

新增 `cronjob_source` 契约，K8s 写协议统一为 `argus.kubernetes_mutation/v4`；v1～v3 在 Connector API 执行前拒绝。v3 是[回滚阶段](./29-kubernetes-workload-rollback.md)的历史记录，本次未建设双轨。OpenAPI、生成客户端、领域快照、私有计划、原生执行和门户同步。

| 范围 | 证据 |
| --- | --- |
| 源引用、模板复制、私有字段投影、旧特权拒绝 | `artifacts/plan8-cronjob-manual-unit.log`，通过 |
| 实际 API、暂停源、预览无副作用、旧版本拒绝、Job Complete | `artifacts/plan8-cronjob-manual-native.log`，通过，临时 Namespace 已回收 |
| PostgreSQL 确认后源变化、failed Execution、重新预览与实际 Job 完成 | `artifacts/plan8-cronjob-postgres-api-workflow.log`，通过；原有 Service/Job/三种回滚/Secret 同轮回归 |
| 门户与三类回滚 | `artifacts/plan8-cronjob-rollback-portal.log`，中英/明暗 8 个 mock 场景通过 |
| API Client 与静态检查 | `artifacts/plan8-cronjob-rollback-client-unit.log`、`artifacts/plan8-cronjob-typecheck-verified.log`、`artifacts/plan8-cronjob-web-lint.log`，通过 |

审批流程中已有模板的私有环境值保留在实际 Job 执行对象中，公开差异、操作结果、私有计划及审计中均没有 canary 原文。失败与成功执行分别收口，不把源变化失败自动重放成另一次任务。

这是原生组件、真实后端工作流与 mock 门户的分层验证。正式 Connector、部署后门户/Agent、完整 Helm、drain、会话、CSI 扩容和其他工作台操作仍未完成，Plan8 继续实施。
