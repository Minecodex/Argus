# Kubernetes 清单编辑与版本冻结

状态：门户实现及受控交互验证通过，T11 整体继续实施。

资源新建、编辑使用 `@argus/ui` 共享 YAML/JSON 编辑器，支持格式切换、格式化、键盘操作和中英明暗。解析采用锁定的 [js-yaml 4.3.1](https://github.com/nodeca/js-yaml/blob/4.3.1/README.md)，只接受单个、可无损转换为 JSON 的对象；重复字段、未知 Tag、循环/过量别名、过深结构、非有限或无法精确表示的整数及超大内容拒绝。错误只保存类别与位置，不保存原文片段。

编辑器读取实际对象的公开投影，保留脱敏标记，隐藏服务端管理的 UID、resourceVersion、generation、时间和状态字段；身份和版本在清单之外单独冻结。修改仍按合并补丁语义提交，省略字段保持原值、null 删除字段，服务端继续核验类型、允许字段和版本，并执行原生 dry-run。

异步刷新不再让旧编辑内容自动使用新 resourceVersion：首次读取冻结编辑基线，后台刷新只提示变化，保留未保存内容并禁用预览；用户明确重新加载后才替换。预览前另行读取当前版本，补上“后台尚未刷新、对象已变化”的窗口。UID 被替换由原有对象身份检查拒绝，不能自动跟到同名新对象。

Secret 原值仍使用独立受保护字段，不进入 YAML/JSON、浏览器持久化、公开差异或审计。统一 PendingAction 确认、权限、版本校验、异步结果和操作历史保持原链路。

证据：

- [解析边界 11 项](../../artifacts/plan8-yaml-editor-parser-tests.log)：值类型往返、重复字段、Tag、循环、多文档、数字、大小和别名展开限制，诊断无原文。
- [版本变化两条路径](../../artifacts/plan8-kubernetes-edit-snapshot-tests-verified.log)：后台刷新和预览前变化均保留旧编辑、不提交；显式重载后显示新内容。
- [中英明暗四场景](../../artifacts/plan8-kubernetes-yaml-e2e-final.log)：YAML/JSON 切换、键盘选择、格式化、重复字段拒绝、对象创建/编辑/读取、Secret 私有提交、操作历史和可访问性。使用 mock API；不能替代已部署平台真实写入。

首次交互验证发现格式选择器采用无对应 Panel 的 Tab 造成无效 ARIA 引用，已改用共享分段单选控件，保留失败记录；单选测试通过键盘操作真实选择。没有移除可访问性断言或强制点击遮挡元素。真实 K8s 的版本/批准摘要/权限和原生写入验证见[写入边界](./18-kubernetes-mutation-privileges.md)。
