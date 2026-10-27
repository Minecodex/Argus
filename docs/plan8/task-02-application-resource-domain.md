# P8-T02：应用、服务与稳定监控目标

状态：in_progress。前件：P8-T01。

## 设计与接点

- [02-domain-data-and-authorization.md](./02-domain-data-and-authorization.md)

已有/拟扩展接点：`internal/resource`、`internal/storage/postgres`、`migrations/postgresql`、`internal/transport/httpapi`、`web/packages/api-client`、`web/apps/enterprise/src`。新领域包和具体接口在 T01 固化；路径是定位入口，不代表本任务已实现。

## 实施清单

- [ ] Application保存唯一固定环境，生产/测试分别建应用；服务按应用+namespace+name受控发现，资源关系按当前绑定查询历史指标。
- [ ] 新增单环境应用、按应用登记/发现的服务、运行实例观测和监控目标领域；统一 ResourceRef 适配已有 Host/Cluster，避免双份权威状态。
- [ ] 实现应用/服务与目标的版本化多对多关系及生命周期，保持共享目标唯一身份。
- [ ] 普通新建原子建立目标与采集上下文；已有对象从资源详情带入，编辑保留 target ID。
- [ ] 环境继承应用唯一环境；多个应用可形成环境集合，未关联为空；当前资源关系与业务观测历史分别保存。
- [ ] 实现真实 API 的应用/服务/资源目录、详情、新建、归档、关系管理与运行实例状态；配置与观测字段分开。
- [ ] 提供面向 T03 的授权根解析和面向 T05/T07/T08 的目标/服务解析接口。

## 完成标准

- [ ] 真实 PostgreSQL 验证唯一约束、事务、并发更新、重复提交、归档/删除依赖。
- [ ] 同名服务跨应用/环境分离；应用关联变更不复制指标、不覆盖历史、不授予目标权限。
- [ ] 相关存储、接口/生成客户端、权限、门户/工具、审计、配置和文档保持一致；单文件小于 2000 行。
- [ ] 保存测试范围、失败迭代、源码/版本与证据；只完成 mock 或单层测试不计真实验收。

## 验收关联

对应 [验收矩阵](./09-acceptance-matrix.md)：P8-A02、P8-A04、P8-A05、P8-A07、P8-A49。

新增验收关联：P8-A75。

## 实施记录

已实施单环境应用、稳定服务身份、运行实例、监控目标与关系，SDK 自动发现及同名不同 Namespace 的真实数据验证已有证据；原“尚未开始”记录已过时。完整生命周期、依赖/归档、迁移与跨入口验收仍待收口，任务保持 in_progress。当前范围见[完成度核对](./32-implementation-audit.md)与[SDK 联合验证](./30-native-sdk-pipeline-and-signal-privacy.md)。
