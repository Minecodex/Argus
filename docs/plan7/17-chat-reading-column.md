# Chatbox 计划与问答布局统一

日期：2026-10-18。状态：已修复、验证并更新本地 Web，argus-platform revision 37。

完整计划和已回答问题摘要原先从 ChatMessageItem 的独立分支直接返回，绕过了普通执行卡片使用的 argus-chat-activity 容器。消息流允许宽结果视图，因此这两类交互会铺满消息区，与正常聊天列错位。

现在所有持久化交互复用相同的活动容器和 chat-text-width token。完整计划、问答摘要与正文、执行卡片、文件区域等宽居中，助手标题与其左边界一致。共享组件补齐长文本的换行和 flex 最小宽度约束，长代码仍在代码块内滚动。计划继续完整展开，高度随内容变化。

## 验证

- 交互／输入框浏览器回归 17 项通过，覆盖中英文、明暗主题、1280×800／1440×900／1920×1080，增加长路径、长回答、长命令和几何对齐断言。
- 使用 kakj 的既有真实会话进行只读浏览器复核，12 个组合全部通过；252 个测量项的左边界差和宽度差均为 0，聊天列宽为 832 CSS px。
- Enterprise 234 项、共享 UI 120 项单测通过；类型检查、ESLint、样式检查与 real Web 构建通过。
- 首轮 17 项回归中 1 项在创建仪表盘菜单点击阶段发生元素脱离／超时，保留原日志；相同源码完整重跑 17 项通过，没有跳过用例或放宽布局断言。

本轮仅修改前端布局，不新增模型调用或远程执行；此前业务流程和模型验收继续对应 [r02 收尾版本](./16-interaction-closure.md)。后端仍为 chat-interactions-20261018-r02，数据库、会话内容和执行记录保留。

源码指纹：`4d6257920b2f4e68e5f86aab8fa12d04e126ef2f733914b4677dd0135074fb6a`，构建前与验收后一致。Web 镜像：`chat-layout-20261018-r01@sha256:8807954a7faa133ed1c074f11a1c9120a1e5bb16ad2edf5c25fac6e426300ea3`。Server／Worker／Web 均为 1/1，Deployment UID 和副本数保持。

证据：[布局测量](../../build/chat-layout-20261018-r01/layout-evidence.json)、[交互回归](../../build/chat-layout-20261018-r01/browser-r02.log)、[真实页面复核](../../build/chat-layout-20261018-r01/real-browser.log)、[计划截图](../../build/chat-layout-20261018-r01/real-browser/chat-reading-column-real-e-1e0a2-s-execution-cards-and-files-chromium/plan-alignment.png)、[问答截图](../../build/chat-layout-20261018-r01/real-browser/chat-reading-column-real-e-1e0a2-s-execution-cards-and-files-chromium/question-alignment.png)。
