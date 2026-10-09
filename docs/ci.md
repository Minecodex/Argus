# PR 与 CI 合并规则

本仓库默认主分支为 `main`。日常开发从最新主分支创建功能分支，将功能分支推送到组织仓库，再通过 PR 合并。

```sh
git fetch origin
git switch -c feature/your-change origin/main
git push -u origin feature/your-change
```

主分支由仓库规则集保护：必须通过 PR，必须通过 `CI`、`contracts` 检查，合并前必须同步最新主分支，禁止强制推送和删除。规则没有管理员绕过名单；目前不额外要求他人批准，PR 上的审查讨论必须解决。

`CI` 是固定名称的汇总检查。它在所有 PR 上运行，任一必需任务失败、取消或意外跳过都会失败，避免工作流名称或矩阵版本变化导致保护规则失效。检查来源限定为 GitHub Actions。

## 自动检查范围

`CI` 汇总 Ubuntu / Windows / macOS 全部 portable 开发门禁；`contracts` 始终检查契约 lint、生成一致性、breaking change、Go 生成包和 TypeScript 客户端。契约工作流取消 PR 路径过滤，确保必需检查总会报告。

工作流也支持主分支 push 和手动运行。手动运行使用 Actions 页面的 Run workflow，选择待检查的分支；功能分支首次引入新工作流时，先创建 PR 触发检查。

完整集群、真实模型和外部接入的验收仍按项目原有 E2E 规范执行；本门禁不把外部基础设施验收视为已通过。

CI 失败会阻止合并。修复失败后在同一个功能分支继续提交，重新运行检查；不要通过删除必需检查或设置管理员绕过来把失败当作通过。

首次 CI 发现查询引擎锁清单引用的 THIRD_PARTY_NOTICES.md 缺失；现按锁定的上游版本和原始许可说明补齐该文件，不更改解析器版本或验证规则。

Sandbox Chart 的 TOML 配置由 argusctl 根据安装配置生成，裸 Chart 没有有效默认 serverConfig。CI 使用相同的安装 Values 执行真实 Helm lint，并验证缺少配置的直接 Helm 安装会被明确拒绝；不提供与实际安装路径不同的占位 TOML。
