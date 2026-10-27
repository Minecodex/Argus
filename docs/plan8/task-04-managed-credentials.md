# P8-T04：监控专用凭据与敏感输入

状态：in_progress。前件：P8-T02、P8-T03。

## 设计与接点

- [04-credentials-and-api-contracts.md](./04-credentials-and-api-contracts.md)

已有/拟扩展接点：`internal/secret`、`internal/action`、`internal/transport/httpapi`、`internal/storage/postgres`、`web/apps/enterprise/src`、`web/packages/ui`。新领域包和具体接口在 T01 固化；路径是定位入口，不代表本任务已实现。

## 实施清单

- [ ] 为既有 Secret/Credential 增加监控用途、owner 与版本化绑定，平台默认凭据目录不混入监控专用项；列表与按 ID 访问同时执行用途/归属授权。
- [ ] 在新建监控中直接填写集成支持的账号/密码/Token，删除用户预建全局凭据及下拉选择步骤。
- [ ] 建立加密短期草稿/测试租约，用户一次表单保存即可完成预览与正式关联；取消/超时/失败自动清理。
- [ ] 实现 keep/replace/remove 的明确更新语义；编辑留空沿用，替换创建新版本，删除释放自身引用且不影响他人凭据。
- [ ] 交付给固定 Collector/任务/配置版本的最小租约；验证轮换、撤销、重启、重试与明文材料清理。
- [ ] 从请求日志、Trace、Diff、错误、PendingAction、工具结果、浏览器持久状态、Workspace 和公共配置中排除原始认证输入。

## 完成标准

- [ ] 真实加密/租约链路证明跨用途、跨 owner、过期/撤销/重复兑换被拒绝，平台管理菜单不能越权代用。
- [ ] 门户/API/工具扫描无密码和 Token 原文；提交中断不残留可复用临时 Secret；UI 只有一次业务确认。
- [ ] 相关存储、接口/生成客户端、权限、门户/工具、审计、配置和文档保持一致；单文件小于 2000 行。
- [ ] 保存测试范围、失败迭代、源码/版本与证据；只完成 mock 或单层测试不计真实验收。

## 验收关联

对应 [验收矩阵](./09-acceptance-matrix.md)：P8-A07、P8-A08、P8-A09、P8-A10、P8-A13、P8-A56。

## 实施记录

当前实施与验证层级见 [实施记录](./14-implementation-log.md)。尚未完成本任务全部验收；同时更新 [progress.json](./progress.json)，不预填通过结果。
