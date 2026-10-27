# 安装完成记录与真实门户会话交接

日期：2026-10-22。此记录描述发现及修复，不能替代修复后的正式验收。

## 实际发现

R62 完整部署中，资源采集及八类真实模型场景通过，门户检查在浏览器启动前失败。API 登录使用平台入口，其企业 Cookie 保存在 API Origin 的 CookieJar；原驱动却从企业浏览器 Origin 读取。修复从实际 API Origin 读取，且只允许企业 Session/CSRF 两个 Cookie 转入本次受控部署的企业入口。平台会话不会转移，CSRF 单独存在不算登录，HTTPS、Strict 与 HttpOnly 约束保留。

随后 R62 独立发布演练实际执行旧制品安装：MinIO 初始化 Job 在 09:28:11 UTC 完成，OpenBao 镜像拉取到 09:33:48 UTC 才结束。Job 的五分钟 TTL 已删除完成对象，Helm 聚合等待返回 `argus-minio-bucket-init: NotFound` 并回滚该次数据层安装。这是安装资源生命周期问题，不是业务采集组件缺失。

## 修复范围

安装器需要读取完成状态的四个固定名称 Job（MinIO 初始化、PostgreSQL 迁移、ClickHouse 迁移、目录同步）取消独立 TTL。普通 Job 继续由下一次安装的重建流程及卸载管理；目录同步 hook 保留 `before-hook-creation`。不会随着运行次数累积新的 Job 名称，也不会放宽失败或就绪判断。临时网络探测及 DLQ 重放任务继续保持各自清理策略。

新增两种部署 profile 的真实 Helm 渲染回归，检查四个完成对象及 hook 替换策略；门户会话回归覆盖 API/浏览器不同 Origin、平台 Cookie 隔离及 CSRF-only 拒绝。后续仍需实际旧版安装、升级、兼容快照恢复和门户浏览器验收。

## 证据及后续

- 原始业务失败：`artifacts/plan8-calico-completion-20261022-r62.log`。
- 原始安装失败：`artifacts/plan8-release-rehearsal-20261022-r62.log` 与该目录 `release-rehearsal.json`。
- 原环境恢复：`artifacts/plan8-release-rehearsal-20261022-r62-desktop-pause/restore.log`。
- 此次修复改变源码快照。旧快照及旧制品证据保留其原版本身份，不能重新标为新版本已通过；最终状态继续由 `progress.json` 与补齐记录汇总。
