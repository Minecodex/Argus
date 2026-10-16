# P6-T05：会话原生工具与两阶段操作

状态：completed。前件：T01–T04 领域服务。

## 交付

- [x] 告警业务工具接入已有会话工作台的 Tool Hub 内部；模型自行通过 `tool.search`（工具查询）→`tool.describe`（工具详情）→`tool.invoke`（工具调用）发现和使用，不增加告警助手页面、模式切换或手动业务工具选择器。
- [x] 在既有 Registry 添加正式 discovery metadata：中英标题/描述、关键词、前件、输入/输出语义、示例、版本/hash。
- [x] 策略存在/配置、当前告警、历史和通知只读工具；通过现有指标工具查询实际值。
- [x] 创建/编辑/启停/删除策略及多规则的 Preview/Commit，单次原子提交。
- [x] 屏蔽/解除、抑制、通知路由、测试/补发和 AI 处理动作均成对提供隐藏 Commit。账号、渠道、消息模板和合并配置的模型写入口已移除，管理门户继续提供配置能力，见[目录收敛](../planv5/tool-catalog-pruning-20261014.md)。
- [x] 模型只看到公开影响与范围；模板只展示详情，确认由宿主完成，私有 Token/Secret 不进模型或浏览器。
- [x] 当前企业、功能权限、资源范围与执行前版本重新校验；取消/过期/撤权/冲突不提交。
- [x] 用户确认后确定性提交，幂等结果持久化，Agent 仅继续只读验证/总结。
- [x] 查询结果带最近评估、健康、静默/抑制、范围和完整性，避免无事件被解释为健康。

## 验收

在普通会话工作台输入中文自然语言，先验证模型自主的工具查询→工具详情→工具调用与真实数据库状态，再在实际批准模型配置下验证「创建 CPU+内存」「查询某指标是否告警」「检查规则是否存在」「屏蔽一小时」「修改阈值」「取消」「撤权」。不以预选具体告警工具或独立告警助手作为前件。记录三个元工具、实际业务 ToolCall、PendingAction、Execution、业务对象和审计关联；确认入口属于会话宿主。固定 Replay 和原型回复不能替代真实模型验收。

涉及：D04/D05/D06，AI 发起的 D07。

## 本轮完成证据（2026-10-11）

源码指纹 `78eb7ef13d9b2c51d7286592160ce696a9ca6c7961e0c22d9a3fb7288805ca10`，完整运行 `plan6-20261011-w45`；正式构建、同组镜像升级与所有权清理已通过。

- [final-plan6-acceptance.json](../../artifacts/plan6-continuation-20261009-w/final-plan6-acceptance.json)
- [complete-required-acceptance.json](../../artifacts/plan6-continuation-20261009-w/complete-required-acceptance.json)
- [serial-gates-final.json](../../artifacts/plan6-continuation-20261009-w/serial-gates-final.json)
- [final-owned-cleanup-verification.json](../../artifacts/plan6-continuation-20261009-w/final-owned-cleanup-verification.json)
- [plan6-real-model.json](../../artifacts/plan6-e2e/plan6-20261011-w45/plan6-real-model.json)
- [describe-authority-regression.log](../../artifacts/plan6-continuation-20261009-w/describe-authority-regression.log)
- [kubernetes-run-w45.log](../../artifacts/plan6-continuation-20261009-w/kubernetes-run-w45.log)

完整业务与真实模型在同组镜像的隔离环境验收；正式环境保留既有数据和配置，并完成页面、权限入口、迁移、运行digest与恢复核对。外部机器人群和真实邮箱独立待配置；本轮真实HTTP/SMTP接收器已通过。

## 2026-10-11 r02当前验收绑定

GLM-5.3十业务+四状态、确认前后配置/取消/Execution与审计关联、真实会话宿主卡片通过。 当前源码指纹`1f6cf72bbe381fa4146365b3e4b277ead5fdd972908553aa230b8e27ed00e0af`；r02单轮完整all退出0，同组六镜像正式revision9发布、数据/UID保留、恢复及所有权清理通过。remaining_gates为空，历史证据保留。

[本次最终验收](../../artifacts/plan6-review-20261011/final-plan6-acceptance.json) · [逐项证据](./progress.json)
