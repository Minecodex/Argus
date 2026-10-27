# 已有企业的权限注册表升级

日期：2026-10-22。本次修复沿用当前权限注册表与内置角色定义，不改变自定义角色策略或资源授权边界，也不增加数据库 Schema 版本。

## 实际问题

原实现仅在创建企业时登记权限并创建内置角色。实际隔离 PostgreSQL 从 Schema 19 升级至 49 后，已有 `enterprise_admin` 仍缺少 `application.read/manage`、`monitoring.read/manage`，新建企业却具有这些权限。仅验证空库安装不能覆盖这个缺口。

## 统一升级入口

新增授权领域服务 `ReconcileBuiltinRegistry`，在正式 `argus-migrate up` 完成 Schema 升级后以及 API 服务启动、开放请求之前执行。两个入口调用同一事务服务；新建企业也复用同一目录锁、写入顺序及版本检查。相同权限定义不重复更新数据库行，低版本登记不能覆盖高版本。原有公共角色编辑接口继续拒绝修改内置角色。

- 使用数据库事务与专用 advisory lock 串行同步，拒绝旧服务覆盖更高版本的权限注册表。
- 登记当前权限定义，只按 `builtin=true` 和稳定 `identity_key` 对齐内置角色；补建缺少的内置角色，保留既有 ID、名称与状态。
- 权限确有变化才更新角色版本，并复用既有授权失效、会话撤销与 Outbox 机制；直接绑定和部门绑定重合时，同一用户只更新一次。
- 自定义角色、不相关的用户和服务账号、以及既有资源授权保持原边界；拥有功能权限仍然不等于拥有应用、主机、集群或 Namespace 数据访问权。
- 写入具有变更前后权限内容的系统审计。审计失败会回滚权限目录、角色、主体版本和 Outbox；重复或并发启动不重复产生变更。

## 验证

补充七项真实数据库回归：旧库升级的权限与数据范围、重复／并发启动、注入审计失败后的原子回滚、更新版本的拒绝、实际迁移命令的入口闭环，以及新建企业与启动同步并发、新建企业拒绝更高版本目录。它们纳入 Plan8 `domain` 与 `full` 的 `authorization-registry-upgrade` 分组，全量组件注册数增加至 85。

最初缺口记录于 `artifacts/plan8-existing-role-upgrade-reproduction.log`。候选修复四项检查与公共内置角色不可编辑断言通过于 `artifacts/plan8-registry-reconciliation-candidate-v5.log`。候选结果不替代合入后的正式版本验收；最终实际状态以 `progress.json` 和补齐记录为准。
