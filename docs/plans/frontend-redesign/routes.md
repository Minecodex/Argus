# 路由改造与回归清单

更新：2026-10-07。源码路由、动态详情和编辑子路由均纳入范围。mock/视觉通过不替代真实接口、遥测或部署通过。

本表是改造与验证索引。正常路由现覆盖三种桌面尺寸×中英×明暗；主要数据页面覆盖加载/空集合/失败/403/恢复，详情、编辑和认证使用对应任务状态验证，见[状态矩阵](./state-matrix.md)。统计图候选、元数据与推荐缺口已修复并验证，见[复核记录](./review-20261007.md)。

再次复核所发现的组织/Sandbox 非默认 Tabs、平台概览和统计图编辑初始读取缺口已补齐；新增真实状态与恢复用例、完整聚合、APM 单次扫描及本地正式升级均已验证，见[补齐记录](./implementation-20261007.md)。本表与状态矩阵共同给出实际覆盖，不把未配置的模型或生产强隔离推定为通过。

| 门户 | 路由 | 改造与验证 |
| --- | --- | --- |
| Enterprise | `/login` | 公共认证表单；登录、MFA、回跳及门户隔离回归 |
| Enterprise | `/` | Chat 双区外壳、消息宽度、输入、引用、工具卡片；流式/一次确认回归 |
| Enterprise | `/dashboards` | Folder、资源卡片、草稿、历史、归档和恢复；中英×明暗、发布目录截图 |
| Enterprise | `/dashboards/:id` | 图型、时间/资源、变量/局部筛选和详情；十一图型、三信号与资源入口回归 |
| Enterprise | `/dashboard-drafts/:id` | 个人画布、模板、过滤项管理、预览发布；恢复、指针/键盘布局和发布隔离 |
| Enterprise | `/dashboard-drafts/:id/panels/:id` | 独立预览/查询/设置、按图运行和下钻；中英×明暗、样式不查询、刷新、长标题和放大 |
| Enterprise | `/hosts`、`/hosts/:id` | 卡片/菜单、接入/采集状态、分区详情、资源关联；中英×明暗、卸载/取消/重试 |
| Enterprise | `/kubernetes`、`/kubernetes/:id` | 卡片、节点/采集/绑定状态、安装向导与分区详情；中英×明暗、三种尺寸与关联流程 |
| Enterprise | `/tasks`、`/approvals` | 紧凑记录/详情；审批域 Tabs 和单选范围；中英×明暗、深链接、拒绝和确认 |
| Enterprise | `/remote-sessions` | 会话/终端/RDP 详情和共享播放滑轨；中英×明暗、播放器单元检查 |
| Enterprise | `/settings/org` | 用户、部门、角色、对象授权、治理表单；中英×明暗及 CRUD/授权回归 |
| Enterprise | `/settings/ai` | 基础配置与可展开高级配置；中英×明暗、模型测试创建 |
| Enterprise | `/settings/mcp`、`/settings/secrets` | 紧凑列表、认证、Secret/Credential/账号关系；中英×明暗、公共表单与状态 |
| Enterprise | `/settings/audit`、`/account` | 记录/筛选、密码/MFA/偏好；中英×明暗；真实审计另由集群验证 |
| Enterprise | `/demo`（仅开发） | 实际共享控件、28/32/36px 尺寸与选项状态；中英×明暗、焦点和 Axe |
| Platform | `/login`、`/setup` | 公共认证/初始化、凭据接收、预检/重试；中英×明暗及完整初始化回归 |
| Platform | `/`、`/enterprises` | 平台域标识、概览、企业卡片/菜单和管理员/配额详情；中英×明暗及详情/配额 |
| Platform | `/admins`、`/sandbox` | 管理员列表，后端/镜像/Profile/配额/会话；中英×明暗，真实运行另由集群验证 |
| Platform | `/audit`、`/pki`、`/account` | 审计、信任/证书、账号/MFA/偏好；中英×明暗，真实接口另由部署验证 |
| Template Runtime | 独立 iframe Origin | 宿主用共享组件；HTML 同步 token/排版/状态；保留 CSP、Hash、Bridge 和单次确认 |

共同规范：HeroUI 3.2.6、唯一 `@argus/ui`、语义 token、公共 Field/RHF/Zod、原生文件输入明确例外、公共控件尺寸不允许被业务 CSS 覆盖。ECharts/xterm/Guacamole 保留专业渲染器，统一容器/工具栏/状态。桌面导航支持收起；200% 放大时编辑任务堆叠，不建立移动端产品导航。

`web/apps/setup` 保留历史入口说明，实际初始化在 Platform SetupGate。真实集群复验和清理状态以验收报告为准，实际模型质量与生产强隔离不是此次视觉重构的结论。
