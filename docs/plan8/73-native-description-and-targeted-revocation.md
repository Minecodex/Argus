# 原生工具描述门禁与精准授权失效

日期：2026-10-22。状态：r49 已通过四个真实模型场景；安全表单与后续权限场景暴露新问题，修复和重验进行中，整体未发布。

r49 通过主机/日志/断线恢复、完整 Kubernetes、多数据库迁移、Redis/Nginx、SDK，以及 K8s 恢复（10 次调用）、独立暂停（6 次）、Key 交付（6 次）、应用 APM/共享数据库（10 次、400.868 秒）。APM 查询与独立基线一致，最终读取 Workspace，真实通过旧故障门禁。证据在 `artifacts/plan8-calico-completion-20261021-r49`。

安全表单模型返回了正确的待用户输入表单，但在没有此前对应 Describe 的情况下成功调用 `monitoring:collector.choices`。验收没有放行：现有运行时只对 alert 强制持久化描述历史，新类目没有复用该门禁。修复将相同的当前 Run/主体/工具版本及更早模型轮次检查推广到所有原生业务类目；缺描述、同轮描述、历史读取失败均不执行业务，返回既有可纠正错误。保留严格发现验收要求，不给模型硬编码测试流程。

告警准备给自有 ServiceAccount 绑定只读角色后，原实现递增全企业用户和 ServiceAccount 的授权版本，导致不相关管理员后续收到 `AUTHORIZATION_VERSION_STALE`，影响了撤权、Workspace 与 PostgreSQL 验收。依据[现有身份授权设计](../02-identity-authorization-and-data-permission.md)，修复为复用已有主体失效服务：单条绑定只影响其用户、部门成员或 ServiceAccount；角色修改影响其绑定成员。审计与授权变更 Outbox 仍在同一事务提交，相关用户的 Session、租约和运行时对象仍撤销，不扩大权限或延用旧授权快照。

这不会把“权限改变使相关 Session 失效”的现有规则改为无条件刷新。修改自己授权的验收需要正常认证取得新会话，再在原 Conversation 继续验证素材拒绝；不能直接更新 Session 版本或把整个请求被拒当作全部文件撤权已经验证。无关主体的会话应保持可用，并用真实 PostgreSQL 测试覆盖用户、部门、ServiceAccount、角色成员及无关主体。

本轮后四个模型和追加 PostgreSQL 正式链均未通过。曾达到这些场景不等于验收通过，原失败保留。原环境已恢复，server 1/1、临时 Kind Namespace 清零后才修改源码。
