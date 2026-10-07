# 正式组件的界面证据

当前资源关联入口的[变化对照](../../../artifacts/frontend-redesign/resource-links-20261007/comparison.html)包含用户原图、正式 React mock 截图和临时 Kubernetes 真实关联/资源范围截图。文字清理后图来自 `visual-closed`，真实流程图保留相应轮次；本地正式 r7 已通过安装和 HTTPS 复核，详见[交付记录](./resource-links-20261007.md)。

当前查看条件与过滤项工作区见[本轮变化对照](../../../artifacts/frontend-redesign/dashboard-controls-20261007/comparison.html)，同时列出原图、正式组件 mock 布局和临时 Kubernetes 的真实接口截图。时间/刷新默认设置与自定义变量分开，资源范围是本次查询筛选；详细语义见[记录](./dashboard-controls-20261007.md)。

最新弹窗与统计图场景库见[变化对照页](../../../artifacts/frontend-redesign/dialogs-presets/comparison.html)。下图来自临时 Kubernetes 的实际创建接口与个人草稿页面；本地正式部署已更新同轮 r5。完整十一类图型选择和三种尺寸布局另见对照页，mock 与真实范围明确区分。

![紧凑居中新建表单](../../../artifacts/frontend-redesign/dialogs-presets/k8s-final/playwright-m2/m2-dashboard-dialogs-real--4e869--and-complete-panel-library-chromium/create-real.png)

![来源与采集要求明确的场景库](../../../artifacts/frontend-redesign/dialogs-presets/k8s-final/playwright-m2/m2-dashboard-dialogs-real--4e869--and-complete-panel-library-chromium/scenarios-real.png)

2026-10-07 用户反馈后的最新视觉对照见[用户截图、Demo 与当前正式组件](../../../artifacts/frontend-redesign/user-ui-review/comparison.html)。以下旧轮次截图保留其原有范围，不能代替新样式的验收。当前对照图来自正式 React 的 mock 状态；真实数据另由 `user-ui-review/k8s-accepted` 记录。

这些截图来自运行正式 React 组件的 Playwright 检查，API 模式明确为 mock；真实部署截图另在 Kubernetes 验收目录。它们用于查看交互变化，不代表真实数据或实际模型验收。

## 独立统计图编辑页

收尾版增加了结果形状推荐与具体不兼容原因。普通兼容图型保持简洁；指标元数据来自目录，计算选项依据实际类型限制。以下截图来自正式组件，使用明确的 mock 数据；真实状态和取数截图另由集群用例保存。

![中文浅色图型推荐](../../../artifacts/frontend-redesign/closure/editor-final/frontend-redesign-redesign-01c7f-y-style-restore-and-publish-chromium/chart-recommendations.png)

预览在上、查询在下，右侧按图型/样式/交互组织；先选择来源、指标、计算和拆分，高级设置按需打开。样式修改保留查询结果，最终回到个人草稿统一发布。

![中文浅色编辑页](../../../artifacts/frontend-redesign/route-browser/frontend-redesign-redesign-de423-y-style-restore-and-publish-chromium/editor.png)

![英文浅色编辑页](../../../artifacts/frontend-redesign/route-browser/frontend-redesign-redesign-1c81d-y-style-restore-and-publish-chromium/editor.png)

## 资源卡片

标题导航、状态、摘要和更多操作分开；搜索/筛选与主操作遵循相同的控件尺寸。

![主机卡片](../../../artifacts/frontend-redesign/route-browser/frontend-redesign-redesign-65d64-red-cards-controls-and-menu-chromium/hosts.png)

## 平台概览的真实契约与月度口径

概览改为服务端完整计数和近 12 个月的月度用量；正式模式不再生成日度演示曲线。以下为相同正式组件的 mock 回归截图，用于查看布局与口径文案；实际数据验证在本轮 Kubernetes 产物中另行记录。

![中文深色平台概览](../../../artifacts/frontend-redesign/overview-mock/frontend-redesign-redesign-090e9-mplete-desktop-route-matrix-chromium/overview.png)

## 公共控件

展示实际 `@argus/ui` 控件的 28/32/36px、选项、禁用、错误、搜索、日期与多选。

![共享控件](../../../artifacts/frontend-redesign/route-browser/frontend-redesign-redesign-3217a-lected-and-disabled-options-chromium/shared-controls.png)

## 本地正式安装

`portal-20261007-r2` 已部署到 Docker Desktop 的正式 Argus Namespace。以下登录截图来自正常证书校验的 Chromium，使用正式 real 镜像；平台已初始化，进入现有登录流程。业务内容的真实数据/编辑验证另在临时集群验收产物中。

![正式企业门户](../../../artifacts/frontend-redesign/overview-formal/enterprise-login.png)

![正式平台门户](../../../artifacts/frontend-redesign/overview-formal/platform-login.png)

其余中英×明暗的目录、详情、平台、初始化截图保存在 `artifacts/frontend-redesign/route-browser`；完整业务回归截图在 `accepted-browser`。所有截图均通过测试输出路径生成，复制后可直接查看。
