# 真实模型验收的显式推理档位

2026-10-26。本次调整补齐验收配置对既有模型 API 的可选参数透传，不改变 Agent 的压缩上限、提示、调用次数、时限或工具权限。

## 事实与对照

R86 默认档位完整复验保留实际失败：应用与共享数据库场景的压缩调用达到 4096 输出令牌；Workspace 最后一次推理达到所配置的 8192 输出令牌，Run 返回 MODEL_OUTPUT_INCOMPLETE。前者有供应方用量，后者十一项调用也均有完整用量，不能归为额度耗尽或本机资源不足。

使用当前拥有的隔离环境重建失败请求，重建的实际请求 SHA256 与原 model_calls.projection_hash 完全相等。原摘要指令及相同输入的对照中，4096 和 8192 输出上限均返回 length，且没有可见摘要。仅澄清历史数据边界的提示对照同样未完成，两个候选均不采用。

保持原输入、原摘要指令、4096 上限及 GLM-5.3，改为显式 low 的诊断请求约 23 秒完成，供应方输出 1308 令牌，stop 结束且得到非空摘要。此诊断不修改原模型版本、Run、任务或正式验收记录。敏感输入仅在内存和标准输入传递，未保存凭据或摘要正文。

上游 [GLM-5.3 文档](https://docs.z.ai/guides/llm/glm-5.3)说明该模型保持推理，未指定时使用 max，支持 low、high、max。Argus 既有模型 API、兼容探测和不可变模型版本已经支持显式推理档位；验收配置此前没有此字段，无法表达这个既有选项。

## 实现与边界

在真实模型配置中增加可选 reasoning_effort，经统一 NormalizeReasoningEffort 验证，使用正常模型创建 API 传入。省略时保持原供应方默认行为，不对所有模型自动设 low。保留当前协议边界：Chat Completions 支持已认可档位；不支持的协议组合和未知值明确拒绝。API Key 继续只通过环境变量引用，不进入配置或证据。

部署验收记录增加 model_reasoning_effort，区分省略参数与显式配置。当前验收另建 GLM-5.3 low 配置，保留原配置文件和失败轮次，先经过正常兼容探测，再由新模型版本固定该档位；不得在运行中的旧版本上覆盖参数。

回归验证默认行为、显式档位透传、既有 OpenAPI 模型输入、未知值及不支持协议拒绝、预算保持和凭据隔离。旧源码不能加载有效 low 配置，候选通过。正式源码应用后重新执行全仓检查和本轮完整门禁；诊断成功不能替代八个真实业务模型场景。

Schema 49、前端、采集组件、Collector 三平台包、35 个默认／共享视图和监控产品 API 均保持。每场景最多 32 次调用和十分钟、推理最大 8192 输出、压缩最大 4096 输出继续保持。旧默认 max 的失败不改记为通过，也不把选定 low 的通过推广为所有模型、档位和任意上下文均通过。

## 证据

- [同一失败请求的输出上限对照](../../artifacts/plan8-r86-exact-compaction-cap-comparison.json)
- [同一输入的提示对照](../../artifacts/plan8-r86-exact-compaction-prompt-comparison.json)
- [同一输入的显式 low 对照](../../artifacts/plan8-r86-exact-compaction-low-profile-comparison.json)
- [旧配置加载回归](../../artifacts/plan8-real-model-profile-before.log)
- [候选配置与既有契约回归](../../artifacts/plan8-real-model-profile-candidate.log)

暂缓资源、Oracle 已验证认证组合、Windows 实机暂缓及 production 的两个架构前件继续按已有决定记录；这次模型验收配置补齐不扩展这些支持声明。
