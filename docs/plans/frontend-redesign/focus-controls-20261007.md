# 统一表单焦点边框

日期：2026-10-07。状态：共享样式和浏览器验证已完成，本轮尚未重新部署本地服务。

用户截图中的搜索框使用细边框和浅色光晕；添加主机弹窗中的输入框同时显示 `--brand-highlight` 深色外轮廓与 HeroUI 焦点环，形成明显较粗的深色边框。

`@argus/design-tokens` 新增 `--control-focus-border`、`--control-focus-ring` 和 `--control-focus-width`，分别复用搜索框原有的 `--accent`、`--accent-soft` 与 2px 尺寸。`@argus/ui` 在共享 foundation 中统一 Input、Textarea、SearchInput、Select、ComboBox 与 DateTimePicker 的焦点状态：细边框、紧贴控件的浅色光晕，并移除供应商叠加的焦点阴影。HeroUI 的 `--focus` 与 `--field-border-focus` 也映射到共享焦点边框 token。

普通输入和选择触发器覆盖原生 focus 与 React Aria 焦点属性；复合控件用 focus-within 将提示放在外框。无效字段仍显示错误边框。企业、平台和初始化流程消费同一个共享样式，不增加页面级覆盖或改变业务流程。

## 验证

产物目录：`artifacts/frontend-redesign/focus-controls-20261007`。

- Chromium、1440×900，中英文 × 浅深色四组检查通过。实际添加主机弹窗的自动焦点、鼠标点击、文本域和 Select 鼠标/键盘焦点，以及组件展示页 Input、ComboBox、DateTimePicker 的边框、光晕、宽度、偏移和阴影均与同主题搜索框一致；错误字段保留 `aria-invalid` 和错误边框。计算结果见 `after.json`。
- 既有 Playwright 回归六项通过：`dialogs-presets.spec.ts` 的四个 1440px 语言/主题组合，以及 `host-removal.spec.ts` 的两个 offline SSH host offers record removal 场景。包含弹窗位置、选值对齐、Axe、焦点轮廓不被裁切和移除预览。
- `pnpm check:styles` 和三个修改样式文件的 Prettier 检查通过。修改文件均低于 2000 行。

视觉对照：[修改前](../../../artifacts/frontend-redesign/focus-controls-20261007/before-dialog.png)、[浅色修改后](../../../artifacts/frontend-redesign/focus-controls-20261007/after-dialog-zh-light.png)、[深色修改后](../../../artifacts/frontend-redesign/focus-controls-20261007/after-dialog-zh-dark.png)。四组语言/主题截图与其余控件状态保留在产物目录。

本轮浏览器使用正式 React 组件与 mock API。没有重新运行 Kubernetes 全流程或更新本地部署；此前正式版本及其验收记录保留原有范围。
