# Plan10 本地 Kubernetes 发布

2026-10-28，按用户后续授权将观测查询发布到现有本地环境，保留业务数据。

当前本机已通过[模型治理发布](../model-governance-local-deployment-20261028.md)升级到 `local-20261028-model-governance`（平台 revision 74、Schema 51），包含下述查询工具栏功能。

## 查询工具栏发布记录（前版）

该轮版本为 **`local-20261028-query-toolbar`**，平台 Helm revision **73**，数据服务 **44**，遥测管线 **39**。面板移除语言标签和面板内的临时查询提示，执行/取消按钮与构造器切换同排靠右；图表/表格切换仅在有查询结果时出现。目录筛选通过共享 FilterBar 紧靠左侧排列，时间与刷新靠右。

本轮仅在上次已验证的独立 Web 副本上叠加 4 个产品文件，保留工作区其他未完成修改。Web OCI 索引为 `sha256:d54ff9215433bdec798c7aee7b310aba3ee77f87e98fa57dc0125d288702a76a`。10 项浏览器回归、类型、样式、i18n、定向 ESLint、real 构建及 bundle 预算通过；发布后健康检查 **20/20**、9 个应用容器摘要核验通过。Schema 保持 **50**，13 张表的 **97 条**原有记录保护字段摘要及 **6 个 PVC/PV** 绑定前后一致。证据位于 `artifacts/plan10-query-toolbar-20261028/`。

## 前一轮图表、时间与折叠版本

前一轮版本为 **`local-20261028-chart-time-collapse`**。所有共享图表的鼠标详情使用 document.body 最高层浮层；观测查询复用仪表盘绝对/相对时间组件，三信号按所选窗口执行；查询卡片支持标题栏点击和键盘收起/展开，配置及结果保留。

本轮 12 项浏览器检查、16 项相关单元测试、类型、i18n、样式、定向 ESLint、real Web 构建及 bundle 预算检查通过。时间测试包括历史绝对范围、取消修改，以及三信号请求的相同 UTC 起止边界和自动步长；浮层测试包括遮罩、弹窗、裁剪区域与文本转义。

发布采用独立 Web 副本，排除另一任务尚未完成的模型治理 UI，保留其原工作区源码；10 个文件只在副本中恢复，5 个新增文件只从副本移除，原文件摘要前后相同。本轮 14 个功能文件与副本逐字节一致。后端镜像复用原内容，不执行未完成的 Schema 51 迁移。Web OCI 索引为 `sha256:a3edabcced0ce3ddfb2e4c21927140d4a7a54da6ad2ee7a5caf6c4e42051e103`。

证据位于 `artifacts/plan10-chart-time-collapse-20261028/`，包括发布状态、运行镜像、数据保留前后记录、`staging.json`、`feature-hashes.json` 和测试日志。

发布后平台 Helm revision **72**，数据服务 **43**，遥测管线 **38**。标准健康检查 **20/20**；**9 个**应用容器运行镜像摘要核验通过。Schema 保持 **50**，**13 张表的 97 条原有记录**保护字段摘要和 **6 个 PVC/PV** 绑定前后一致；工作区另一个任务的 15 个相关源文件摘要也保持一致。

## 前一轮行内构造器版本

前一轮版本为 **`local-20261028-inline-builder`**，平台 Helm revision **71**，数据服务 **42**，遥测管线 **37**。观测查询与仪表盘主编辑器同步使用紧凑的来源/指标/计算/分组行、行内标签条件，以及直接可见的查询方式和步长。Web OCI 索引为 `sha256:ac97d2859cfe6fa1af0d0c3e04e77d2b59d46af3f7e7ecc05b65a3f3d44c20a8`；后端及其他镜像继续复用原内容。

18 项浏览器回归、14 项相关单元测试、4 组真实采集链路浏览器测试通过；真实测试新增标签/值选择和带条件执行。定向 ESLint 零警告，类型、i18n、样式及 real Web 构建通过。临时测试 Namespace 和端口转发已清理。

发布后健康检查 **20/20**，**9 个**应用容器运行镜像摘要核验通过；前后 **13 张表的 97 条原有业务记录**保护字段摘要和 **6 个 PVC/PV** 绑定一致。证据位于 `artifacts/plan10-inline-builder-20261028/`，其中 `native-pipeline.log` 为真实链路，`browser-verified.log` 为隔离 mock 回归；未将 mock 截图当作正式用户数据。

## 前一轮排版更新版本

前一轮版本为 **`local-20261028-observe-layout-v2`**，入口仍为 <https://argus.dev/observe>。平台 Helm revision **70**，数据服务 **41**，遥测管线 **36**。

- 工作台资源提示移到资源按钮旁；指标构造器与仪表盘共用同一组控件；联动十字线、时间和执行全部放在共享 Card 中。
- 目录资源范围改为按钮加共享多选弹窗。目录的时间、名称、资源类型、范围、来源和已观测状态集中于同一筛选栏。目录翻页后修改时间、名称或资源条件会回到第一页，避免复用过期游标。
- Web 新 OCI 索引为 `sha256:cef80882b20b1f3cd018831039efa873652805bd8f356c6623639e50b316b1f8`；后端及其他五种镜像复用上一版相同内容。沿用标准安装事务，未新增迁移或清理业务数据。
- 最终 **9 项观测浏览器回归**通过，覆盖中英文、深浅色、多选/取消/空集合、搜索和分页。共享构造器还通过 **2 项仪表盘条件编辑与布局回归**。real Web 构建、类型、i18n 和样式门禁通过。
- 发布后标准健康检查 **20/20**；**9 个**应用容器实际镜像摘要核对通过；**13 张表的 97 条原有业务记录**保护字段摘要和 **6 个 PVC/PV** 绑定前后一致。

最终证据位于 `artifacts/plan10-catalog-layout-20261028/`，前一轮工作台排版证据位于 `artifacts/plan10-layout-20261028/`。浏览器截图来自隔离 mock 回归，未将它们当作本次部署后的登录用户查询验收。

## 首次发布入口与版本

- 观测查询：<https://argus.dev/observe>。首次打开需要企业登录，登录后返回该页面。
- 集群：`docker-desktop`；Namespace：`argus-system`。
- 发布版本：`local-20261028-observe`；前一版本：`local-20261027-catalog-scroll-v3`。
- Helm：`argus-platform` revision 68，`argus-data` revision 39，`argus-telemetry-pipeline` revision 34。
- 沿用 `deploy/.cache/argus-local-formal.yaml` 的单机配置；只修改镜像 tag，未改变拓扑、账号、密钥或存储配置。

使用当前工作区源码构建 real Web 与后端镜像，API、Query 及其他后端角色一致更新；其余五种产品镜像复用前版的相同内容摘要。现有工作区修改保留，没有提交或重置代码。内部 Catalog 协议 v5 已随 API/Query 配套发布。

| 镜像 | OCI 索引摘要 |
| --- | --- |
| argus-backend | `sha256:8761f348d51ccec4f0062446d09980b9983d9c9655dc2c2e5a36007c282e24aa` |
| argus-web | `sha256:beb913471fc173dcd144e6e8d43fd95c58873b1e83ace655f0fb9b80fa4936bf` |

## 发布核验

- 标准平台健康检查 **20/20** 通过，包含 HTTPS、证书、各服务健康、数据库/队列/对象存储往返和 Sandbox 生命周期。
- **9 个**应用运行容器就绪，实际 imageID 与本次构建的不可变 OCI 索引及 Linux amd64 清单对应；不是仅比较标签。
- 全部 **18 个** Deployment/StatefulSet 达到期望就绪副本数。
- 前后比较 **13 张业务表的 97 条原有记录**保护字段摘要一致；**6 个 PVC/PV** 的身份、配置与绑定一致。数据库保持 Schema 50，没有新增迁移，没有删除业务数据或卷。
- 本次没有全量读取历史遥测内容或 Workspace 文件做摘要，PVC 核验不等同于文件内容全量备份。

浏览器已打开实际 HTTPS 门户，当前停留在企业登录页并携带 `/observe` 返回地址。已有自动化登录态返回 401，本次没有重置账号、伪造会话或把登录页当成已完成的登录后查询验收。此前真实采集与页面测试结果见[实现验收记录](./implementation-status.md)；本次部署后的登录用户查询效果由重新登录后查看。

## 证据与回退材料

本机 `artifacts/plan10-deploy-20261028/` 保存 `deployment.json`、`running-images.json`、`business-before.json`、`business-after.json`、构建日志和 `platform-health/`。原配置及详细安装日志保存在该目录的受限 `private/` 中；前一版本镜像仍保留，没有执行镜像或数据清理。
