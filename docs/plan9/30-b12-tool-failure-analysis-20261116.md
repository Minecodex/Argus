# B12 会话工具失败根因核对

2026-11-16 按用户截图读取当前正式会话账本、历史上下文快照及对应模型调用绑定。**本次是诊断，没有修改执行逻辑、重放工具或调用模型。** 没有进行业务 SQL 写入；历史快照仅以绑定企业/会话的有界 `BEGIN READ ONLY` 查询核实。

## 结果范围

会话 `01a1e1f0-4809-78f5-bb8b-9b7b96e35a44` 共291条账本事件、62条工具结果。18条失败全部集中于补齐仪表盘取数的 Run `01a1e210-8621-7a95-9ebd-397f6ca00178`；该 Run 的35条工具结果为17条成功、18条失败。

| 实际错误 | 次数 | 账本 sequence | 原因 |
| --- | --- | --- | --- |
| TOOL_SEARCH_REQUIRED | 4 | 88～91 | 新 Run 直接 Describe get/query/context.resolve/query.get，没有先完成本轮该分类的 Search；旧 Run 的发现不能代替新 Run 的授权发现 |
| DASHBOARD_VERSION_CONFLICT | 3 | 105、106、122 | get 已明确返回 `analysis.condition_version=0`，模型却提交 `expected_version=1`；两张仪表盘的条件版本独立，不是另一张已递增就一起递增 |
| DASHBOARD_CONDITION_EVIDENCE_REQUIRED | 4 | 114、118、120、124 | Redis 证据引用并非当前用户原文的连续子串；还有一次对 `/resources` 提供两条 evidence，违反每个变更条件一条证据的要求 |
| TOOL_INPUT_INVALID | 1 | 116 | 把 dashboard_id、expected_version 放到 changes 内，把 time/evidence 放进 resources，违反已返回的 Schema |
| WORKSPACE_FILE_NOT_FOUND | 6 | 164～169 | 压缩摘要缩短真实路径，后续模型用缩写路径读取；文件实际存在 |

这些错误不能合并解释为服务宕机或数据库并发冲突，也不能把全部失败归为模型本身的问题。

## 已证实的平台侧缺陷：压缩损坏可执行引用

原生 `query.get` 在 sequence150/152 返回完整文件路径，两个作业均 `complete`。随后 sequence156 使用的快照 `01a1e215-40a8-7b8d-9a9e-3fa3d45b659c`（revision7）覆盖到152，将路径写成：

```text
Host: /workspace/queries/70c27d04…/741dc561…/{f953b1e5(cpu).json, ...}
Redis: /workspace/queries/c916bbdd…/a1e01dc0…/{7bbdefa6(memory).json, ...}
```

但原件目录和文件分别是完整 UUID，如：

```text
741dc561-54a5-4550-b329-52f396548fe1/
f953b1e5-fb8d-4deb-9f3f-512191a6d3e5.json
```

模型调用 `01a1e215-4179-75ce-9adf-6babb47035ea` 的持久化 `context_snapshot_id` 确实绑定此快照，`context_through_sequence=156`。紧接着 sequence163 将目录/文件缩为 `741dc561/f953b1e5.json` 等，导致六次读取失败。sequence174 的实际离线目录列表和后续读取确认完整原件均存在。

因此因果链为：**压缩把机器引用当叙述压缩 → 完整原结果离开上下文 → 模型直接将缩写当路径调用**。现有压缩提示虽然要求保留 relevant file paths，但800 Token的叙述摘要不足以保护这些引用；平台没有在这条仪表盘文件链路上给出独立于叙述摘要、可直接复用的完整引用保障。模型也没有先恢复完整路径。

现有 `query_evidence` 每轮事实只覆盖 `NativeQueryFile` 注册的原生 metric/log/trace 文件，不能据此宣称所有 Dashboard 作业文件引用都已获得同等保护。

## 版本与条件证据的具体问题

Host 和 Redis 的 get 结果分别在99/101返回 `analysis.condition_version=0`。首次两次提交均填1，正确被领域服务拒绝。Host 改0后在113成功；Redis 此时仍为0，后来再次填1又被拒绝。没有证据表明这三次错误由其他用户修改仪表盘造成。

本轮用户原话是 `查询最近5分钟 Host CPU/内存、Redis内存/命令/命中率`。模型构造的 `查询最近5分钟 Redis内存/命令/命中率` 不是连续原文，因而不能作为 message_interpretation 的 quote。sequence120虽然已把资源证据改为用户真实提供的完整 Redis 身份文本，但 `/time` 仍使用上述自写句子，因而再次失败；不能用模型自写摘要充当用户引用。

后续以两条合法 ai_selection evidence 和版本0在128成功；该次资源仍严格为用户指定的 Redis，时间为300秒，没有通过移除条件或扩大范围规避门禁。建议保留这些验证规则，修正字段提示与错误恢复，而不是取消版本/证据校验。

## 页面为何都是“工具调用 ×”

这些失败账本的 `display=null`，且 `target_tool_id=tool.invoke`。执行失败时结果未保留已验证的业务工具身份，`persistToolOutcome` 回退到元工具名称；前端时间线优先使用该字段，翻译后就都是“工具调用”。普通失败没有公开 summary，错误详情留在折叠的 output 中。

这同时存在两个问题：真正的调用失败，以及失败展示缺少具体业务名称与原因。只换红叉样式不会解决调用质量；只让模型重试也不能改善页面的解释性。

截图中的两条绿色 queued 表示当时的作业提交成功，不是取数已完成的证明；已核对后续150/152的真实状态均为 complete，并在179通过 Workspace 实跑计算。Run 最终在229 succeeded，不能据此把前面18次失败从质量验收中忽略。

## 建议的修复顺序（本次未实施）

1. **完整引用保真**：在现有授权、Workspace 身份及作业语义内恢复有界完整文件引用，独立于自然语言摘要；检测并拒绝把省略号/短 UUID 叙述当成可用路径。不得添加模糊前缀猜路径或跨 Workspace 自动重绑定。
2. **契约与恢复反馈**：明确 expected_version 对应 analysis.condition_version，提供可公开的字段级冲突/证据原因与重新读取指引。修正仍需遵守当前用户条件、权限及原有乐观锁；不能自动填0或放宽 quote 校验。
3. **失败信息与重试收敛**：保留已验证的业务工具名称和可公开错误说明；相同错误重复发生时要求重新读取契约/权威事实，保留完整失败历史和费用。页面可以呈现后续恢复关联，不能把历史失败改成成功。
4. **真实回归**：覆盖新 Run 的 Search→Describe→Invoke、两张仪表盘独立条件版本、逐字证据和错误嵌套、query.get 后强制压缩再精确读取文件，以及中英/明暗的失败卡片。新增付费模型回归应单独记录用量，不重放原已完成的运维操作。

证据位于 `C:\Users\23574\.codex\artifacts\argus-b12-tool-audit-20261116`：`failures.json`、`path-compaction-model-call-proof.json`；完整账本和快照仅保留在受限 `private/`。B12 保持等待人工验收，本次没有开启下一批。
