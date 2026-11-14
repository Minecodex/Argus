# B10c 自定义资源预览的事务边界修复

2026-11-13，B10c 实际验证发现：CRD 已 Established，原生权限、SDK dry-run 与平台列表均正常，平台创建预览却返回 `KUBERNETES_API_UNAVAILABLE`。

仅对本批 `checks.b10c.argus.test` 的 create 请求启用 API Server 审计后，确认原生 dry-run 返回 201；正式 PostgreSQL 同时记录预览重试在 `LockAuditChain` 上持续发生序列化冲突。没有业务实例落库或原生写入，失败现场保留。

原因是普通类型从静态目录解析，自定义类型需要原生 metadata 读取并签发/消费凭据租约。预览保存事务和确认重验再次调用同一解析逻辑，在事务外推进了相同企业的租约审计链，使外层 serializable 事务每次都自失效。重试次数不能修复这一边界问题。

## 行为与边界

- 预览在保存事务外读取类型、验证 CRD UID 并执行原生 dry-run；事务内复用已验证定义，用绑定该事务的查询重新校验权限、Cluster 版本和 Namespace 身份。
- 确认保留受保护计划中带 CRD UID 的类型引用，并原子校验当前权限、范围和冻结策略，不在事务内打开原生传输或签发租约。
- 操作 Worker 在任何原生写入前重新解析实际类型并比较定义、CRD UID 和 Namespace UID；同名重建不会承接旧审批。对象版本、原生 admission 和逐写授权仍继续校验。
- 不降低事务隔离，不跳过审计、授权或 dry-run，不改变公开接口、协议版本或数据库 schema 54。

已新增真实 PostgreSQL 回归，覆盖审计型 metadata 的预览/确认、原生验证后版本改变，以及确认后 CRD 同名替换必须在写入前失败。三项通过；WorkBench/Inventory/Reader/Resource 四包完整回归及相关 PostgreSQL 用例通过。

修复已实际部署到 Server/Worker，镜像为 `host.docker.internal:5001/argus/argus-backend:local-20261113-b10c-custom-preview-v1@sha256:04e71a111d2c3ab1f1cd28746bec5d70e38d7273e6559b0c57190a7eb348f8c0`。现场核对实际 Pod imageID、rollout 和六个正式 PVC 的身份/绑定；其他后端组件保持原镜像，不需要 schema 迁移。

同批通过正常平台预览/确认完成 `b10c-check` 的创建和编辑，以及临时 CR 的创建/删除；Role/RoleBinding/ServiceAccount 的实际变更、原生读权限撤销与恢复、越权拒绝均通过。五项正常登录的真实页面/日志/终端用例及正式健康 20/20 通过，结果见 [B10c 查看记录](./batches/B10c-review.md)。临时诊断 Namespace、凭据副本及 tmpfs PostgreSQL 容器已清理，本批 API Server 的临时审计配置已还原。

证据目录：`C:/Users/23574/.codex/artifacts/argus-plan9/p9-b10c-20261113-01/`，包括 `custom-apiserver-audit.jsonl`、`custom-preview-regression-v2.log`、`custom-preview-full-go.log` 和保留的原失败预览。
