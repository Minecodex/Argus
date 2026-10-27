# P8-T05：多实例编译、责任与运行状态

状态：in_progress。前件：P8-T01、P8-T02、P8-T03、P8-T04。

## 设计与接点

- [03-collection-and-ingest.md](./03-collection-and-ingest.md)
- [07-integration-catalog.md](./07-integration-catalog.md)

已有/拟扩展接点：`internal/telemetry`、`internal/otelcol/configbundle`、`internal/collectormanager`、`internal/connector`、`internal/app/connector`、`deploy/otelcol`。新领域包和具体接口在 T01 固化；路径是定位入口，不代表本任务已实现。

## 实施清单

- [ ] 本期暂缓目录按[专用插件复查](./13-direct-receiver-review.md)与progress.json同步；资源无专用采集组件时不新建采集、不提供可用默认仪表盘，门户/API/Agent一致。
- [ ] 仅复用可内置的Collector组件，验证OCB清单、组件注册、配置、多实例及运行模型；移除独立exporter/JRE采集helper的安装和进程治理范围。
- [ ] 建立版本化集成目录、参数 Schema 与编译适配器，替换 Profile bool 集合对多实例配置的限制。
- [ ] 渲染稳定 Receiver 实例、原始来源标签和共享 pipeline，按 Distribution/平台检查组件与运行依赖。
- [ ] 实现 CollectionClaim 范围规范化/冲突、Host/Node 绑定、迁移 primary/epoch 和受控双采窗口。
- [ ] 实现整个 Bundle 校验、desired/effective revision、单实例诊断、回滚/CAS 与 Collector 级资源预算。
- [ ] 允许单实例启停和全部业务采集关闭，保持控制健康；离线/失败/无数据/等待分别观测。
- [ ] 真实发行制品进入签名/哈希清单；明确持久队列、文件 checkpoint 与更新期间的保留/恢复策略。

## 完成标准

- [ ] 同一 Collector 运行多条同类及异类任务，关闭/修改一条不错误删除其他配置。
- [ ] 坏配置不会替换有效版本；并发更新/重装/断线恢复不回滚到他人旧配置；实际状态可追溯。
- [ ] 相关存储、接口/生成客户端、权限、门户/工具、审计、配置和文档保持一致；单文件小于 2000 行。
- [ ] 保存测试范围、失败迭代、源码/版本与证据；只完成 mock 或单层测试不计真实验收。

## 验收关联

对应 [验收矩阵](./09-acceptance-matrix.md)：P8-A14、P8-A15、P8-A16、P8-A17、P8-A18、P8-A19、P8-A62。

新增验收关联：P8-A76。

## 实施记录

当前实施与验证层级见 [实施记录](./14-implementation-log.md)。尚未完成本任务全部验收；同时更新 [progress.json](./progress.json)，不预填通过结果。

已补齐 API/领域接受显式空 Profile，控制连接不再被迫开启某个数据来源；预览和提交要求实际 `nop` 组件。来源应用窗口及数据库不可用采用可重试响应，基础设施旧生效版本在切换后拒绝新写入，见[控制与准入](./20-collector-control-and-admission.md)。全平台生命周期、失败回滚和正式接入仍待完成。
