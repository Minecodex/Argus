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

PR 增加真实 PostgreSQL 的空库/重复/并发迁移、全部生成语句准备、持久化 Agent/授权/仪表盘回归，以及共享门户的登录/权限、主机接入、MFA、MCP、Workspace 和 Profile 关键浏览器流程。这些任务与三平台 portable 一起决定固定名称 CI 的结果；PostgreSQL 凭据仅属于当前隔离 job。

发布验收和手动 full=true 另执行完整共享门户浏览器矩阵，保留原有主题、语言与桌面界面场景。两个浏览器 worker 避免托管 CPU 争用，完整矩阵使用独立的 60 分钟 job 预算和每例 90 秒总预算；局部交互断言保持原有超时。CI 不依靠自动重试把失败掩盖为通过，并保存失败诊断。真实集群场景仍由其专属部署套件负责，mock 门户测试不替代集群验收。

每次 v* 版本运行 release.yml：复用同一提交的基础/契约门槛，再在原生 Linux amd64/arm64 的独立 Calico/Minikube 集群执行 M8、PlanV2、P4、TLS 的完整依赖闭包和现有实际 argusctl 安装、浏览器、故障、备份恢复及清理。不给 unit-only 或局部 browser grep 记完整通过。没有定时任务；PR 的 ci:release 标签可收集同一候选的 Linux 完整证据。

M8 保持现有 ARM64 本地硬化契约，不增加 AMD64 的 local_hardening_complete 声明；PlanV2/P4/TLS 的运行器按现有能力在 AMD64/ARM64 上验证。托管 CI 证据不解除 Production Profile 阻断，也不代替 ARM64 Docker Desktop 的既有最终硬化验收。全新 checkout 会由共享 Collector 构建路径先创建输出父目录，再调用 OCB，不依赖开发机器残留的 build 目录。

正式 Release CI 还要求实际 Windows Server 2019/2022 接入验收。受信任 Tag/手动运行读取 ARGUS_WINDOWS_HOST_CONFIG 和 ARGUS_WINDOWS_HOST_ENV 两个 Actions Secret：前者使用 tests/e2e/windows-host.example.yaml 的配置合同，后者是该配置引用的 ARGUS_WINDOWS_* 环境值 JSON。凭据不会上传，缺少环境明确失败。公开 PR 不执行带这些凭据的远程主机操作；标签候选的 Linux 证据单独汇总为 Linux candidate CI，不代表正式 Release CI 或 Windows 验收通过。工作流不创建公开版本。

当前缺少独立的 Windows Server 2019/2022 测试 VM，因此正式发布验收尚不能完成。需补齐该配置合同中的实际主机、接入和清理条件，并将凭据放入上述 Actions Secret 后，重新运行同一发布候选的完整门槛。不会用 GitHub Windows 桌面 Runner 或模拟测试代替 Server 接入验收。

`CI` 汇总 Ubuntu / Windows / macOS 全部 portable 开发门禁；`contracts` 始终检查契约 lint、生成一致性、breaking change、Go 生成包和 TypeScript 客户端。契约工作流取消 PR 路径过滤，确保必需检查总会报告。

工作流也支持主分支 push 和手动运行。手动运行使用 Actions 页面的 Run workflow，选择待检查的分支；功能分支首次引入新工作流时，先创建 PR 触发检查。

完整集群、真实模型和外部接入的验收仍按项目原有 E2E 规范执行；本门禁不把外部基础设施验收视为已通过。

CI 失败会阻止合并。修复失败后在同一个功能分支继续提交，重新运行检查；不要通过删除必需检查或设置管理员绕过来把失败当作通过。

首次 CI 发现查询引擎锁清单引用的 THIRD_PARTY_NOTICES.md 缺失；现按锁定的上游版本和原始许可说明补齐该文件，不更改解析器版本或验证规则。

Sandbox Chart 的 TOML 配置由 argusctl 根据安装配置生成，裸 Chart 没有有效默认 serverConfig。CI 使用相同的安装 Values 执行真实 Helm lint，并验证缺少配置的直接 Helm 安装会被明确拒绝；不提供与实际安装路径不同的占位 TOML。
