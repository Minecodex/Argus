# 模型治理本地构建与部署

2026-10-28，按用户授权将模型治理、AIOps 设置和导航调整部署到现有 Docker Desktop 本地环境。

## 刷新按钮对齐补丁

当前本机版本为 `local-20261028-model-controls-align`，平台 revision 75。月份、模型和刷新按钮所在行改为字段控件底边对齐，保持原有间距及组件尺寸；不改变其他普通按钮行。

本轮只重新构建 Web，其他镜像复用前版内容。1280/1920 桌面宽度、中英文及深浅主题的 8 项几何对齐和刷新交互检查通过；类型、样式和定向 ESLint 通过。发布后健康检查 20/20、9 个应用容器摘要匹配，99 条原有记录及 6 个 PVC/PV 前后核对一致，Schema 保持 51。Web OCI 索引为 `sha256:6fdfdf17266ac3f34d27adba5c73d7c8b2429b2bbccb33f4cf1ecd7045ae51a1`，证据位于 `artifacts/model-controls-align-20261028/`。

下文保留初次模型治理发布的范围与验收记录。

- 版本：`local-20261028-model-governance`，前版 `local-20261028-query-toolbar`。
- 集群：`docker-desktop`；Namespace：`argus-system`；沿用现有 `evaluation` 单机配置。
- 企业入口：[AIOps 设置](https://argus.dev/settings/aiops)、[仪表盘](https://argus.dev/dashboards)。模型治理为仪表盘目录中的系统内置条目。
- Helm：平台 revision 74、数据服务 45、遥测管线 40，均为 deployed。

## 构建与发布

冻结 3,681 个构建输入文件，内容指纹为 `352f84e8659cc8e184accc1c663d05c84503f85512fdf299be37951ddb0cb028`。独立构建上下文排除本机环境文件、依赖目录、测试输出和私有部署配置；没有重置或提交当前工作区源码。

重新构建 real API Web 和生产后端，后端没有 E2E 构建标签。其他五种配套镜像复用前版不可变内容。通过本轮源码构建的 `argusctl` 执行 preflight、plan、images load、install 和 verify。

| 镜像 | OCI 索引摘要 |
| --- | --- |
| argus-backend | `sha256:50fe8f9b7d9fd69e89c3700ce7472a425c0026b5d8906633a05a9105c4f695d9` |
| argus-web | `sha256:f3912775edbacf15480f1a044c6a1512050d843210a358cffb24b9a98f5f990b` |

## 核验结果

- 标准健康检查 20/20 通过；9 个应用容器 Ready，imageID 与本次不可变 OCI 索引或 Linux amd64 清单匹配。
- PostgreSQL 从 Schema 50 迁至 51，三个 outbox 触发器安装完成，待投影记录为 0。
- `argus.model-governance` 内置模板 head 为 version 1；ClickHouse 已创建调用、额度策略和预留账本三张私有表。
- 18 张表中 97 条原有业务记录的保护字段摘要一致；6 个 PVC/PV 的身份、配置和绑定一致。
- 升级前保存约 17 MB 的 PostgreSQL custom-format 备份及原安装配置，保存在受限 `private/`。卷身份检查不等同于全部文件内容校验或完整灾备证明。

沿用 evaluation 的共享 `argus` 数据库账号；本轮没有切换到 `local-hardening`、OpenBao 或专用数据库账号。应用/API 的私有来源、mTLS RPC、企业/部门授权隔离已发布；专用 Query/Writer 的列级数据库权限证明来自此前隔离 E2E，不能视为本机 evaluation 已启用该角色布局。

实际 HTTPS 页面能正常加载，当前检查浏览器停在带 `/settings/aiops` 返回地址的企业登录页。本轮没有可用的企业浏览器登录态，没有重置账号或创建代用会话；登录页不算登录后页面验收。真实模型和中英明暗页面的前置验收见[实现记录](./plans/frontend-redesign/model-governance-dashboard-20261028.md)。

## 证据

`artifacts/model-governance-deploy-20261028/` 保存构建日志、`source.json`、`deployment.json`、数据保护前后记录、`running-images.json`、`model-governance-verification.json` 和标准健康检查。敏感安装日志、数据库备份和原配置仅保存在受限 `private/`；前版镜像及业务数据未清理。
