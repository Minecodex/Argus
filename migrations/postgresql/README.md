# PostgreSQL migrations

PostgreSQL Schema 由独立 Job 在 `argus-server` 和 `argus-worker` 就绪前安装。`00001_argus_baseline.sql` 直接创建当前最终结构、权限种子与运行角色授权；`00002_cancel_unregistered_onboarding.sql` 为已有本地环境增加安装任务 `cancelled` 终态，不清理现有业务数据。结构变化必须同步 SQLC、契约测试和全新数据库 `up → down → up` 验证。

`00020_workspace_results.sql` 增加可信业务结果映射，供 Workspace 结果业务路径和单一 `results/meta.json` 使用；新版本必须先完成该迁移再启动对应 Worker。该迁移不改动既有文件或目录。
