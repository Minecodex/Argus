# 结构化仪表盘结论的状态层级

2026-10-26。只澄清真实模型验收中的共享任务说明；不放宽事实比对，不改变监控数据、产品 API 或模型预算。

R87 已在 GLM-5.3 显式 low 新版本下通过应用、SDK、安全表单、告警、撤权和 K8s 模型场景。Workspace 场景的 Run 成功，十三项调用用量完整；实际 CPU 的序列数、47 个点、最小／最大／最新值与不可变文件全部相同，但结构化 JSON 将面板原始状态 success 写成 complete，严格比较正确拒绝。

执行任务完成、文件交付完成与面板查询成功是不同层级。[既有仪表盘模型验证记录](../planv2/real-model-validation-20260928.md)已经区分 query_status、delivery_status、analysis_status。共享任务文字仍使用“清单中的状态”，未指明应读取哪个层级；模型取用了任务／交付完成状态。

在共用的结论口径和 JSON 字段说明中，明确 status 来自执行清单对应 panels 项的原始查询状态字符串，原样保留，不使用异步任务或交付的 complete/partial，也不翻译或使用近义词。完全未执行仍为 not_checked；补充的查询、交付、分析状态分别使用各自权威字段。

严格比较保持：success 与 complete 不合并；原始样本、数值计算、覆盖、文件 SHA、权限及模型用量要求均不改。提示不提供预期仪表盘 ID、观测值或正确答案，也不增加绕过查询、文件读取或审批的路径。原始 R87 失败保留，不改记为通过。

此说明由 PlanV2、Plan6、Plan7、Plan8 的既有仪表盘模型任务共用，统一修正，不为本次单个资源加分支。Agent 运行时、压缩指令与 4096 上限、所选 GLM-5.3 low 配置、8192 模型输出上限、32 次／十分钟场景预算保持。Schema 49、前端、API、采集组件和 35 个默认／共享视图保持。应用后重新执行当前版本适用检查和完整真实业务门禁；不因本次只有一个枚举不一致而豁免验收。

证据为 R87 完整轮次中的 [Workspace 模型记录](../../artifacts/plan8-calico-completion-20261025-r87/plan8-dashboard-workspace-evidence.json)。后续状态见 progress.json 和 [交付记录](./57-completion-delivery.md)。
