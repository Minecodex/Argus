# P8-T01：契约、基线与技术前件

状态：in_progress。前件：已确认产品范围。

## 设计与接点

- [01-scope-and-decisions.md](./01-scope-and-decisions.md)
- [02-domain-data-and-authorization.md](./02-domain-data-and-authorization.md)

已有/拟扩展接点：`api/schemas`、`api/openapi`、`api/proto`、`migrations/postgresql`、`internal/authorization`、`deploy/otelcol`。新领域包和具体接口在 T01 固化；路径是定位入口，不代表本任务已实现。

## 实施清单

- [ ] 本期暂缓目录按[专用插件复查](./13-direct-receiver-review.md)与progress.json同步；资源无专用采集组件时不新建采集、不提供可用默认仪表盘，门户/API/Agent一致。
- [ ] 完成39候选的官方与成熟社区的内置Collector组件审查，区分无合格内置组件、未验证和缺环境，冻结实际支持组合及缺项证据。
- [ ] 记录源码、现有部署和数据清单；将原型证据与生产前件分开。
- [ ] 固化 ResourceRef、应用服务身份、环境关系、采集实例/版本、凭据用途和 Scope 的版本化契约。
- [ ] 核对 39 个候选集成及内置能力的目标版本、OS/架构、Receiver/运行依赖核验、采集权限、真实测试资源和模板依赖；收敛 P8-V01～V07。
- [ ] 制定 API 错误、分页、幂等、预览、长操作、未知结果和状态完整性契约。
- [ ] 冻结每类吞吐/基数/日志大小/查询成员/时延和撤权窗口预算，记录数值、测量方法及测试环境；不得先调低阈值再称通过。
- [ ] 按实际工具链生成/验证 OpenAPI、protobuf、JSON Schema、sqlc 和权限表；迁移序号在实施时分配，不预占猜测版本。

## 完成标准

- [ ] 契约样例覆盖同 Collector 多目标、生产/测试独立应用、共享依赖和未关联目标，无字段语义相互矛盾。
- [ ] 生成契约可重复，缺外部环境明确标记未验证并指定后续任务，不用 mock 顶替。
- [ ] 相关存储、接口/生成客户端、权限、门户/工具、审计、配置和文档保持一致；单文件小于 2000 行。
- [ ] 保存测试范围、失败迭代、源码/版本与证据；只完成 mock 或单层测试不计真实验收。

## 验收关联

对应 [验收矩阵](./09-acceptance-matrix.md)：P8-A04、P8-A07、P8-A45、P8-A60。

新增验收关联：P8-A74。

## 实施记录

已实施领域迁移、内部协议和生成契约，并通过当前契约一致性检查；原“尚未开始”记录已过时。全部候选版本/平台/拓扑、容量与可靠性门禁尚未闭合，任务保持 in_progress。当前范围见[完成度核对](./32-implementation-audit.md)，具体修改见[实施记录](./14-implementation-log.md)，不预填整体通过。
