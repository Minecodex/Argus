# 用户截图与 Demo 对照复核

日期：2026-10-07。状态：用户截图所指出的缺陷及本轮全局样式缺口已修正、验证并部署。当前正式版本 `portal-20261007-r4`。

用户截图暴露了此前验收遗漏的正式样式缺陷：审批分段选择挤成文字，收件箱记录内容超过固定按钮高度；主机筛选缺少统一的容器与对齐；统计图编辑页预览过高，常用查询项不在首屏，图型区域使用普通按钮并重复显示说明，与 `docs/planv2/editor-ux/demo.html` 不一致。

本轮从共享组件处理：Button 区分固定尺寸控件和内容自适应的记录/卡片操作；SegmentedControl 命中 HeroUI 实际 Radio.Content，统一背景、选中、焦点与间距；FilterBar 统一容器；共享图型卡片默认展示常用/推荐类型，全部类型可展开。输入标题、表格预览与查询区的基础布局向 Demo 对齐，样式变化仍不改写或执行查询。

进一步静态追踪发现 29 处未定义变量引用，涉及平台 MFA、设置与模拟结果、主机移除/接入、远程审批和终端。均直接改为现有 Argus token，没有引入旧变量别名。`check:styles` 新增变量声明/引用检查，识别 CSS、静态 React style 和显式 fallback；HeroUI 在 Select 弹层写入的 `--trigger-width` 仅在共享包装样式中作为有来源的运行时例外。新增 3 项检查回归通过，旧错误可稳定复现并在修正后全部清零。StatCard 统一正文字体并保留数字等宽排版，避免中文状态退回等宽字体的宋体字形。

对照页：[用户截图、Demo 与修正后正式组件](../../../artifacts/frontend-redesign/user-ui-review/comparison.html)。其中正式组件图片明确使用 mock；真实取数不由这些图片证明。

不改变数据查询、对象权限、草稿保存和一次确认发布边界。新验收须断言记录内容在点击区域内、不同控件对齐、查询基础项与主要动作在桌面首屏可见，并展示 Demo 与正式界面截图对照。旧通过证据保留为历史，不替代本轮验证。

## 当前验收

- 类型、全工作区单元、国际化、样式/表单、正式构建与包体积通过；共享 UI 新增图型选择回归，75 项通过。变量声明检查有 3 项回归。
- 新增布局专项三尺寸 × 中英 × 明暗共 12 项通过；全门户最后完整轮次 151 通过、1 移动端范围外跳过、1 弹层进入态对比度失败。修正选项字体继承，视觉检查等待有限动画结束后，12 项全部复验通过，构成所有 152 场景的通过覆盖。没有将分轮结果写成单轮 152/152。
- 查询筛选弹窗另有 1 项闭环通过：取消不改变已有预览，必填字段约束，应用后待运行，服务端草稿恢复和条件移除。与 HeroUI 隐藏输入及弹层退出动画相关的测试选择器错误已修正，保留失败日志；没有使用强制点击绕过真实操作。
- 实际集群首次未使用独立 Ingress，预检拒绝且清理恢复。独立轮次 `ui-20261007q` 真实浏览器 5 通过、4 新测试失败：新增测试误用了接口路径和响应包装。原有图型/编辑/发布 5 项、文件工具协议与 6 类在途故障通过；失败轮次保留。
- `ui-20261007r` 的新增脚本把“编辑”误写为“编辑仪表盘”，等待失败。现场复验进一步确认图型样例为语句配置，CPU 预设来源没有相应样本；改为作者通过正式界面配置已有 OTLP 指标，并等待元数据及草稿稳定后运行。旧定义执行结果均未记为通过，临时会话文件与转发已清理。
- 最新 `ui-20261007s` 真实页面 4/4 通过，覆盖三种尺寸、中英/明暗、单图真实执行、结果表格、样式不查询、草稿、发布预览与审批记录/资源过滤布局。连同 q 轮次 5 项，全部 9 个本轮所选真实场景已有通过记录；这些是选定场景验收，不能称为完整 PlanV2 浏览器套件已重跑。s 的文件/工具协议、6 类在途故障及安装 20/20 均通过，结果为 `selected_browsers_and_runtime: passed`，最终退出码 0。临时 Namespace 剩余 0，19 项暂停工作负载全部恢复。
- `portal-20261007-r4` 已升级本地正式前后端。正式安装复核 20/20，通过 8 组企业/平台 × 中英 × 明暗的 HTTPS、Axe、32px 控件检查；初始化仍为 `initialized`，所有工作负载就绪。保留现有数据，维护的安装配置已同步到 r4。Docker Hub 临时 OAuth EOF 的失败日志保留，重试成功；实际模型质量与生产强隔离不属于本轮声明。

证据目录为 `artifacts/frontend-redesign/user-ui-review`：`accepted-browser.log` 保留完整轮次结果，`layout-accepted.log` 为 12/12，`filter-verified.log` 为筛选闭环；`lint-accepted.log`、`typecheck-latest.log`、`ui-unit-latest.log`、`bundle-accepted.log` 与 `comparison-proof.json` 提供源码及对照验证。`k8s-isolated.log` 保留失败的真实轮次，`k8s-final.log` 记录重验。

最新真实成功轮次为 `k8s-accepted.log`、`k8s-accepted/result.json`、`k8s-accepted/verify/verify.json`。清理恢复见 `cleanup-proof.json`。正式交付见 `formal/images-build-r4-retry.log`、`formal/images-load-r4.log`、`formal/install.log`、`formal/verify/verify.json`、`formal/browser-proof.json` 和 `formal/workloads.json`。三组静态对照以及真实执行图片均已检查；对照页不是真实查询证明的替代。
