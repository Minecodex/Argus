# Kubernetes 资源工作台布局

2026-11-09，根据资源查询页截图调整桌面交互。

## 页面行为

- 资源类别与资源类型分别使用一排共享 Tab。先选工作负载、网络等类别，再选 Deployment、Pod、Service 等类型；扩展类型较多时沿用共享 Tab 的横向滚动。
- Namespace 范围使用共享可搜索下拉框，默认“全部已授权命名空间”。候选通过现有资源目录按页读取，滚动继续加载，不把第一页当作全部候选。
- 命名空间、完整名称、标签选择器与唯一刷新按钮在共享 `FilterBar` 中保持单行。输入框按剩余空间收缩并设宽度上限；按回车或刷新提交名称与标签条件，回到第一页并重新读取，已有缓存也不能代替本次刷新。
- 操作记录和新建对象放在筛选栏最右侧，操作记录位于新建左侧。命名空间资源未选择单个 Namespace 时新建禁用，按钮说明提示先选择命名空间。集群级资源隐藏 Namespace 下拉。
- 移除实时更新开关、连接状态提示与资源列表自动 watch。列表只在切换范围、分页、提交筛选、手动刷新或完成操作时读取；底部显示观测时间与分页。
- 资源工作台移除 Helm 发布管理入口与其弹层挂载；Pod、Service 行移除端口转发入口。Pod 查看日志、管理对象与终端继续使用原授权和会话流程。
- 资源页不再展示“部分扩展资源类型暂时无法发现”的常驻提示；类型目录的完整性仍由 API 返回，实际查询失败继续走原错误处理。
- 控件、字号、间距和主题颜色复用 `@argus/ui` 与 design token，覆盖中英及明暗桌面体验。

## 范围与授权

本次调整企业门户资源页的功能入口。既有 Helm/端口转发领域服务、API、CLI、权限和审计仍保留，不修改 Kubernetes 接入和服务架构；Plan8 的原始统一交付范围及历史验收不代表当前页面仍提供这些入口。

Namespace 候选使用 `namespace_mode=all_authorized`，选中后传递后端返回的 UID 绑定资源 ID，资源请求使用 `namespace_mode=explicit` 和 `namespace_ids`。选择“全部已授权”显式恢复原范围；清空搜索文本本身不改变已选授权范围。分页、搜索失败和候选刷新均不自动扩大授权。

## 验证

新增桌面布局 E2E，覆盖 1280/1440/1920 × 中英 × 明暗，检查单行控件、按钮顺序和右对齐、类型 Tab、Namespace 切换与清空、名称过滤、新建和操作记录、Pod/Service 无端口转发、集群级类型隐藏 Namespace 及可访问性。

既有 Kubernetes 资源读取、变更、日志、终端、回滚、CronJob 和 drain 用例同步使用新控件；原 Helm 门户用例改为验证入口移除，领域/API 的生命周期验证继续独立保留。

- 66 个相关 E2E 场景全部通过，包含 12 种桌面布局、2 项 Namespace 下拉、8 项共享 ComboBox 分页，以及资源读取、变更、日志规则/会话、终端、回滚、CronJob、drain、授权和 Helm 入口移除。日志：[完整回归](../../../artifacts/kubernetes-resource-layout-20261109/e2e-final-r2.log)。
- 刷新动作统一重新读取第一页后，12 种布局与 2 项 Namespace 场景再次全部通过，未把重复执行另计为新场景。日志：[最终复验](../../../artifacts/kubernetes-resource-layout-20261109/e2e-refresh-final.log)；[中文浅色 Pod 工作台截图](../../../artifacts/kubernetes-resource-layout-20261109/refresh-final/kubernetes-resource-layout-86b21-source-layout-1440-zh-light-chromium/kubernetes-pod-workspace.png)。
- 共享 UI 149 项单元测试、Enterprise/UI 类型检查、相关 ESLint、双语覆盖、样式 token、表单语义和格式检查通过；所有修改的前端代码文件均低于 2000 行。
- Enterprise 的 real 模式生产构建退出码 0，见[最终构建日志](../../../artifacts/kubernetes-resource-layout-20261109/enterprise-real-build-final.log)。Vite 仍报告循环分块、Mock 模块静态导入及大分块提示；本轮没有把构建成功等同于全门户发布门禁通过。

首次布局轮次的浏览器验证使用独立 Mock 上下文，当时未部署到本地 Kubernetes，也未新增真实 API/Collector 全链路验收。Namespace 目录失败保留选值、搜索取消恢复名称的修正在共享 ComboBox 实现，既有分页、失败重试、旧请求隔离和自定义输入回归均通过。

## 移除实时更新与本地发布

同日根据后续要求移除资源页实时更新功能：删除唯一使用的 `inventory-watch.ts`、开关、状态文案和关联样式。底部保留观测时间，手动刷新继续重新读取当前筛选范围的第一页。

- 26 项桌面 E2E 通过，覆盖中英、明暗及 1280/1440/1920 布局、Namespace 下拉、资源读取/变更和 Pod 终端；新增断言确保实时更新开关及状态不存在。证据目录：`artifacts/kubernetes-resource-release-20261109/mock/`。
- 相关 ESLint、双语、样式 token 和差异格式检查通过；Docker 内三个门户 real 模式构建及 Mock 种子/占位端点扫描通过。
- 发布版本 `local-20261109-k8s-workspace`，镜像摘要 `sha256:6dcf5adce69453c56dced09ba9fab5fde70099e7e893ff965f2f8b4b8c886be3`。Docker Desktop 的 `argus-system/argus-web` Deployment revision 147，1/1 就绪，实际 imageID 与发布摘要一致。本次只更新 Web Deployment，Helm release revision 仍为 90。
- 前后核对 24 项工作负载/PVC 记录：除 Web 镜像外均一致，六个 PVC 的 UID 和卷绑定一致。实际 HTTPS 提供的 Kubernetes 页面资源与运行容器逐字节一致，页面资源已无实时更新文案引用。见[发布核对](../../../artifacts/kubernetes-resource-release-20261109/published-verification.json)、[镜像记录](../../../artifacts/kubernetes-resource-release-20261109/deployment.json)及[构建日志](../../../artifacts/kubernetes-resource-release-20261109/web-build.log)。

首次健康检查在滚动期间记录 19/20，未就绪原因是节点缺少规范化的 `repository@digest` 镜像别名；补齐与安装器一致的别名后滚动更新完成，重新执行的完整健康检查 **20/20 通过**，退出码 0。见[最终健康报告](../../../artifacts/kubernetes-resource-release-20261109/health-final.json)。

保存的真实测试登录态返回 `SESSION_EXPIRED`。Computer Use 又因无法可靠识别当前浏览器 URL 而停止，所以本轮登录后实页检查未完成；Mock 页面回归和实际部署资源核对分别记录，不互相替代。

同日后续通过正常管理员登录完成 [B09a 接入凭据恢复](../../plan9/batches/B09a-review.md#2026-11-09-接入凭据恢复)，八类真实资源查询返回 200，并补充通过中英 × 明暗四项实际部署页面验证（`kubernetes-credential-recovery-real.spec.ts`）。此次凭据恢复没有修改 Web 镜像；恢复证据补齐了上述发布轮次缺少登录后实页验证的部分。

## 移除扩展资源发现提示

同日根据截图要求，删除顶部“部分扩展资源类型暂时无法发现”徽标及其双语文案。目录 API 的完整性元数据和实际读取错误处理继续保留。

发布版本为 `local-20261109-k8s-hint`，Web 镜像摘要 `sha256:eb089e2cb54cbf4a0656a53fe8e4ab5494ffc2f9c57f60fa8c27902ceec7b49b`，Deployment revision 148，1/1 就绪。12 项桌面布局回归、4 项真实页面验证通过；真实验证在目录仍不完整的 B09a 集群上断言该提示不再展示。

三个前端 HTTPS 健康入口均返回 200，实际页面资源与新容器一致，其他工作负载和六个 PVC 前后核对一致。见[发布核对](../../../artifacts/kubernetes-partial-hint-removal-20261109/published-verification.json)与[真实页面回归](../../../artifacts/kubernetes-partial-hint-removal-20261109/real-e2e.log)。
