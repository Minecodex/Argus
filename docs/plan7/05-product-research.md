# 官方产品资料对照

核对日期：2026-10-15。来源为公开官方文档；这是设计参考，不是产品实机对比或完整市场调查。开发文档与 Preview 不能等同于所有已发布版本的能力。

| 产品 | 官方说明 | 对本计划的参考 |
| --- | --- | --- |
| HolmesGPT（开发文档） | Bash 使用允许／拒绝列表，未允许命令需确认，可单次批准或记住前缀；K8s remediation 同时有固定诊断与始终人工确认的通用入口。[Bash](https://holmesgpt.dev/dev/data-sources/builtin-toolsets/bash/)／[K8s remediation](https://holmesgpt.dev/dev/data-sources/builtin-toolsets/kubernetes-remediation-mcp/) | 通用远程执行与受控业务工具可并存；不采用其跨会话前缀记忆作为 Argus 批准语义 |
| Datadog Bits Remediation | 基础设施一键修复和 Guardrails 为 Preview；按具体动作、环境／资源与批准人应用 Ask/Deny。[Bits Remediation](https://docs.datadoghq.com/bits_ai/bits_remediation/) | 具体执行范围、用户资格与批准应由平台治理，不能从自然语言建议直接获得权限 |
| Datadog Private Action Runner | 执行预先配置并按名称引用的脚本，可设置动作允许列表与参数 Schema；不接受任意 inline command。这是 Runner 能力，不能推成 Bits 支持全部脚本。[Run a Script](https://docs.datadoghq.com/actions/private_actions/run_script/) | 后续可把重复验证的操作沉淀为模板；本轮不要求预先覆盖所有 Linux 命令 |
| Resolve AI | 调查默认只读，模型和执行引擎分离；告警静默、PR 创建需人工批准；GitHub Actions 工具处于 Preview，默认批准，也允许管理员逐工具设 Auto。[Security & Administration](https://docs.resolve.ai/security-and-administration)／[Mitigation Actions](https://docs.resolve.ai/mitigation-actions) | 保留提案与实际执行分离；不把该产品的具体动作范围推成任意 shell 安全能力 |
| Rundeck／PagerDuty Runbook Automation | Job 可以包含脚本，Job 运行／修改、Node 运行和 ad-hoc 命令权限分别控制；MCP Job 校验检查结构，不能证明任意脚本安全。[ACL](https://docs.rundeck.com/docs/administration/security/authorization.html)／[MCP capabilities](https://docs.rundeck.com/docs/mcp/capabilities.html) | 可按固定脚本或本次执行计划治理，不必把每个内部命令转换成业务 Tool |
| Dynatrace Workflows | 执行受 actor 权限约束；Request Approval 可暂停工作流等待批准，拒绝或超时失败。[Permissions](https://docs.dynatrace.com/docs/analyze-explore-automate/workflows/security)／[Request Approval](https://docs.dynatrace.com/docs/analyze-explore-automate/workflows/default-workflow-actions/approval-request-workflow-action) | 用户确认不能扩大执行身份的原始权限；审批等待有效期不等于未来免确认授权 |

HolmesGPT 明确说明，命令前缀只约束开头，不约束所有后续参数或网络目的地。因此前缀匹配不能替代资源范围和完整请求绑定，见其 [Prefix Matching](https://holmesgpt.dev/dev/data-sources/builtin-toolsets/bash/#prefix-matching)。

## Python／shell 解析的适用边界

Python 可通过原生文件和网络 API 产生副作用，未必调用 shell；动态导入、命令替换和执行时输入也会改变行为。Python 官方说明审计机制不是完整沙箱，见 [PEP 578](https://peps.python.org/pep-0578/#why-not-a-sandbox) 与 [sys.addaudithook](https://docs.python.org/3/library/sys.html#sys.addaudithook)。Bash 的变量、命令替换和文件名展开具有额外执行语义，见 [Shell Expansions](https://www.gnu.org/software/bash/manual/bash.html#Shell-Expansions)。

据此，本计划采用「完整请求授权＋辅助校验＋运行时权限与执行状态」；不将解析结果或 AI 风险说明当成全部副作用证明。Workspace 中的 Python 仍可自由分析和生成文件，但不能持有生产执行能力。

## 本次不直接照搬的内容

- 不建立管理员配置每个工具自动执行等级的界面。
- 不采用跨会话保存 shell 前缀作为授权复用。
- 不将外接客户 MCP 统一纳入 connect；其例外已在产品决定中明确。
- 不借用其他产品的只读凭据默认值，声称 Argus 已接入账号必然低权限。
- 不承诺任意脚本可自动回滚，或解析后即可无需人工确认。

上述取舍来自 Argus 当前讨论，不是这些厂商的共同实现保证。
