# PR 与 CI 合并规则

日常开发使用功能分支和 PR。默认分支要求 GitHub Actions 的固定 `CI` 检查、同步最新主分支并解决审查讨论；失败、取消或意外跳过不能通过汇总门禁。禁止强推和删除，不设置管理员绕过。

## 执行层级

默认主分支为 `main`，除 `CI` 外还要求固定 `contracts`。

| 入口 | 实际范围 |
| --- | --- |
| 普通 PR / main push | 三平台 portable、契约 lint/生成/breaking change、真实 PostgreSQL 迁移及持久化业务、关键共享门户流程 |
| v* Tag / 手动 release | 同一提交的 portable/契约/真实 PG 检查，以及完整共享门户浏览器矩阵 |
| 独立环境 | M8 ARM64、PlanV2/P4/TLS 双架构实际集群、安装/故障/备份恢复；真实 Windows Server 2019/2022 接入 |

发布工作流仅由版本标签或手动运行触发；PR 标签不触发完整发布，也不会在每次新提交时重跑。固定 `Release CI` 汇总托管软件检查，失败、取消、意外跳过都失败；本工作流不创建公开版本。

大集群和远程 Windows Server 验收已经从 GitHub Actions 移出，Actions 不需要 `ARGUS_CI_*_RUNNER`、`ARGUS_WINDOWS_HOST_*` Secret、专用 Runner 或 Environment。独立环境验收继续使用现有入口：
```sh
go run ./cmd/argus-dev e2e run --suite planv2 --kube-context YOUR_ISOLATED_CONTEXT --run-id YOUR_RUN_ID --artifacts .cache/e2e
go run ./cmd/argus-dev e2e windows-host --config YOUR_PRIVATE_CONFIG --run-id YOUR_RUN_ID --artifacts .cache/windows-evidence
```

其他集群套件为 `m8`、`p4`、`tls`。实际安装预检的 10 核 CPU、15 GiB 集群可分配内存和原生架构要求保持不变；没有环境时记录待验证。配置、凭据和资源必须属于独立测试环境，结束后只清理本次资源。

完整门户保留原有主题、语言、桌面和权限断言。mock 门户检查不替代实际集群、外部接入或真实模型验收；托管 CI 成功不改变 `local_hardening_complete` 或 Production Profile 的既有阻断状态。没有定时任务。
