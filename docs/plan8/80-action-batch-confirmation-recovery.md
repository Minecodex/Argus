# 预览批次收尾与确认恢复

r55 的真实 K8s 模型在第七次调用后，同一模型轮次返回 `object.change.preview` 和 `object.list`。预览成功，后一个调用停在 prepared；Run 一直 running，十分钟到期。本轮未进入采集队列故障阶段，不能据此宣称 Namespace 指标恢复已经通过。原部署与卷身份恢复40.51秒通过，server1/1、自有Namespace清零。

根因是 Agent 在预览返回 ActionRef 后立即退出，但全部调用意图已一次持久化。确认恢复要求整个工具消息组完成，看到尚未结束的调用就不进入等待确认；正常任务却已返回，没有继续调度。真实 PostgreSQL 补测复现为只有一个结果、后续只读调用执行次数为零。

现按既有“完整工具批次后确认”契约收尾：预览出现后，已声明的剩余调用采用现有 verification/read-only 约束。只读调用正常完成，原生确认预览仍只生成待批准动作，外部写调用在 durable dispatch 之前拒绝。全部结果及 Step 收尾后再进入 waiting_input，不提前执行任何待确认动作。Plan 与用户补充输入仍保留原来的硬暂停屏障。

中断恢复也查询本 Run 的实际 PendingAction，按同一限制完成剩余调用，并在批次结束后恢复确认状态。预览已经提交但 ToolResult 尚未提交，以及 ToolResult 已提交而后续调用尚未处理，两种窗口均验证没有重复动作、没有隐式确认、外部写未派发，三个工具结果完整配对。未知副作用结果优先停止，不因批次含预览就掩盖未知状态。

真实数据库回归验证正常路径为三个结果、一个只读调用、零个外部写；两种崩溃窗口恢复到等待确认且不可重复恢复。既有一次确认/一次执行、计划屏障、四种模式下补充输入屏障继续通过。完整 Agent 回归和真实模型复验单独记录，未以受控模型事件补测代替真实 Provider。

完整组件驱动新增 action-batch 组，合计74组；域测试补入两企业同名身份隔离、目标/凭据绑定/分配/Outbox失败回滚及退役后的稳定目标复用。有效的 failed/blocked/incomplete/not_run 观测与格式损坏、来源不匹配的证据分别报告；只有 passed 且全部冻结断言满足才通过。组件失败时仍汇总正式证据，不能因此漏报剩余项或标为可发布。

证据：`artifacts/plan8-action-batch-before-fix-r2.log`、`plan8-action-batch-after-fix.log`、`plan8-action-batch-recovery.log`、`plan8-calico-focused-20261022-r55/`。完整支持矩阵、76项逐条证据、最终部署和发布仍在执行。Oracle长密码范围决定仍待用户答复，Windows实机沿用用户暂缓决定。
