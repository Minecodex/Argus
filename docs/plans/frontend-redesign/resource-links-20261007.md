# 资源关联入口正式重构

日期：2026-10-07。状态：用户确认的重构已实现、验证并交付，本地正式版本为 `portal-20261007-r7`；临时环境已清理，19 项工作负载就绪。

本轮将 [资源入口示例](../../planv2/resource-links-ux/README.md) 的推荐交互迁入正式组件：仪表盘不再显示通栏反向关联区，标题操作区通过“更多 → 关联资源”打开只读抽屉；Host/K8s 详情增加“仪表盘”页签，使用共享资源卡片展示快捷入口，并逐项关联/解除、预览、确认。绑定领域、对象授权、PendingAction 和查询语言保持既有边界。

## 实现

- `@argus/ui` 新增共用 DrawerPanel，FormDrawer 复用同一边框、标题、正文、页脚与字段边界。表单通过原生 form ID 关联页脚提交按钮；只读抽屉不伪装成表单。基础框架的单元覆盖独立页脚提交与只读无提交行为。
- DashboardLinkedResources 仅在抽屉打开时查询现有绑定 API。加载显示状态，失败显示错误及重试；只有成功返回空数组才显示无关联。列表与计数使用后端已授权结果，仪表盘侧不新增绑定写入入口。
- ResourceDashboardLinks 复用 ResourceGrid/ResourceCard；没有新建逐卡遥测轮询，也没有将缺失的信号摘要或 Revision 编号当成真实字段。管理列表可搜索，一行执行一次 attach/detach，完整保留当前 Dashboard、Resource、Binding 版本校验和确认。
- 快捷链接保留 `resource` 并补充受限 `resource_type` 导航参数；DashboardEntrySource 使用当前可访问资源的 GET/缓存显示标题来源。来源按入口 ID 保留，查看者改变顶部资源筛选不改变它；读权限失败/撤销时不展示该资源名称。
- 反向导航及来源返回链接进入 `?tab=dashboards`。Host/K8s 复用既有 Tabs；Collector hash 入口继续打开原安装页签。没有 Dashboard 读取功能权限时隐藏仪表盘页签。
- 新增双语 dashboard-links 模块，样式只使用既有语义 token。原型保留作设计对照，正式交付状态以本记录为准。

## 验收范围

产物目录 `artifacts/frontend-redesign/resource-links-20261007`。验证抽屉只读、按需查询、无关联/失败、资源页发现性、逐项取消/确认、资源入口预选、切换到授权全集后保留来源、解除后仪表盘仍可查询、中英/明暗/三种桌面尺寸、Axe 与焦点恢复。相关原有仪表盘、过滤和弹窗场景一并回归。

真实部署使用 PlanV2 临时 Kubernetes，选择本轮资源入口用例及原有 Host/K8s 快捷入口用例，并执行既有 Runtime、文件、工具协议和在途故障验证。浏览器注入 503 仅证明 UI 错误处理；实际取数、权限和发布由真实接口及领域测试分别证明。测试结束清理本轮命名空间、恢复暂停工作负载，然后部署本地正式 r7。未执行或失败的项目不标为通过；不是完整 PlanV2 或实际模型质量套件重跑。

当前证据：类型、样式、国际化与单元通过，UI 79、API 99、企业 164、平台 17、模板 2；相关 dashboard 与契约 Go 测试通过，dashboardaccess 无独立测试文件。桌面首轮 37/43，通过组合包含旧仪表盘/过滤/弹窗；新抽屉底部按钮在六个深色组合触发 Axe 对比度不足，修复公共 `--accent-soft-foreground` 映射后，新增入口 12＋公共控件 12 共 24/24 复验通过。因此 43 个所选场景已有分轮通过覆盖，另有 12 个公共控件组合，不称为单轮 43/43。最新文字清理的聚焦复验 1/1；正式构建/包体积通过。真实环境和正式安装仍在进行中。

证据：`browser.log` 保留首轮失败，`browser-accepted.log` 为 24/24；`visual-closed.log` 为最后文字清理后的 1/1；`types-closed.log`、`lint-closed.log`、`unit-accepted.log`、`backend.log`、`build-closed.log`、`bundle-closed.log` 对应源码检查。原图与正式组件的[变化对照](../../../artifacts/frontend-redesign/resource-links-20261007/comparison.html)已建立，真实结果后续在本记录追加。

真实轮次 `planv2-ui-links-20261007a` 已完成浏览器 5/5：中英 × 明暗四组实际 attach/detach、按需反查、独立入口来源、授权全集、失败重试，加上原有 Host/K8s 真实指标快捷入口用例。文件下载校验与跨会话拒绝为 true，工具协议通过，6 类在途故障报告 passed=true。`k8s.log` 与 `k8s/playwright-planv2/`、`planv2-files.json`、`planv2-tool-protocol.json`、`planv2-inflight-faults.json` 保留证据；实际模型未执行。最终安装与恢复结果仍待关闭，不把阶段通过写成整体已交付。

本轮完整验收进程退出码 0，`k8s/result.json` 准确记录 `selected_browsers_and_runtime: passed`；临时安装 20/20。包含辅助堡垒机的本轮测试 Namespace 剩余 0，原 19 项 Deployment/StatefulSet 副本与就绪均已恢复，见 `cleanup-proof.json`。此前阶段“恢复待关闭”的状态保留为过程记录，以本段最终复核为准。

| 范围 | 结果 | 证据（本轮 artifacts） |
| --- | --- | --- |
| 源码、国际化、正式构建与包体积 | 通过 | `types-closed.log`、`lint-closed.log`、`unit-accepted.log`、`build-closed.log`、`bundle-closed.log` |
| 所选桌面与公共控件 | 43 项分轮通过覆盖，另 12 项公共控件组合；最新复验 24/24 | `browser.log`、`browser-accepted.log` |
| 真实浏览器与运行时 | 5/5，文件/协议/6 类故障通过，未评测实际模型 | `k8s.log`、`k8s/result.json`、各 `planv2-*.json` |
| 临时安装与恢复 | 20/20，退出 0，Namespace 0，19 项就绪 | `k8s/verify/verify.json`、`cleanup-proof.json` |
| 正式镜像 | 最新构建和节点加载通过，包含文字清理 | `formal/images-build-closed.log`、`formal/images-load-closed.log` |
| 正式交付 | r7 安装/复核 20/20，19 项就绪，企业/平台 × 中英 × 明暗 8 组 HTTPS、32px、Axe 和初始化状态通过 | `formal/install.log`、`formal/verify/verify.json`、`formal/workloads.json`、`formal/browser-proof.json` |

## 最终交付

正式访问为 `https://argus.dev` 与 `https://platform.argus.dev`，原初始化仍为 initialized，现有数据保留。维护的 `deploy/.cache/argus-install-consolidated.yaml` 已同步 r7，前一份配置保留于 `formal/install-config-before-r7.yaml`。最终汇总见 `closure.json`，过程中的部署/恢复待完成声明以本节和表格最终结果为准。

本轮没有改变旧 PendingAction/Object 授权架构、查询定义、绑定与资源范围的关系；也没有新增批量绑定事务。实际模型质量及全部 PlanV2 浏览器套件未重跑。本地正式使用继续采用既有 evaluation Profile，不将此次 UI 交付扩大为生产强隔离或 HA 验收。

浏览器注入 503 用于错误/重试状态，原有指标用例检验真实取数范围。最新两项文字清理不改变接口/执行行为，聚焦检查及最终正式镜像已覆盖；真实截图来自文字清理前的验收镜像，该范围在对照页注明。
