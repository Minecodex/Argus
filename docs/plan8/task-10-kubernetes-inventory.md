# P8-T10：Kubernetes 资源视图与读取

状态：in_progress。前件：P8-T02、P8-T03。

## 设计与接点

- [05-host-and-kubernetes.md](./05-host-and-kubernetes.md)

已有/拟扩展接点：`internal/kubernetesreader`、`internal/resource`、`internal/connector`、`internal/app/connector`、`internal/transport/httpapi`、`web/apps/enterprise/src/components/kubernetes`。新领域包和具体接口在 T01 固化；路径是定位入口，不代表本任务已实现。

## 实施清单

- [ ] 补齐Jobs/CronJobs、HPA、NetworkPolicy、RBAC、CRD/自定义资源和Helm Release的分页/详情/事件/动态类型，所有入口处理Namespace独立授权。
- [ ] 在现有 Direct/Connector 适配上增加 GVR/UID/resourceVersion、Namespace/Owner/container 身份、观测时间和完整性。
- [ ] 实现受控对象列表/详情、分页/continue、事件、watch 恢复、过期重列、背压、撤权与切换取消。
- [ ] 提供工作负载、Pod、Service/Ingress、ConfigMap/Secret 元数据、PVC/PV/StorageClass、Namespace、Node 的桌面列表与详情。
- [ ] YAML/对象投影按允许字段和敏感信息处理，Secret 原值不通过普通详情回传。
- [ ] 建立供日志规则和指标过滤复用的下级对象解析，避免各页面按名称维护平行缓存。
- [ ] 增加 Namespace UID 独立授权并保留 Cluster 全范围，沿用可信接入身份；撤权覆盖所有读取和会话。

## 完成标准

- [ ] 真实 API 和 Connector 路径的同一查询返回一致身份；分页不漏/重复，Pod 重建/换名不接管旧身份。
- [ ] watch 断开、410、撤权与页面切换后，旧请求不能污染新集群视图；查询超限显示完整性。
- [ ] 相关存储、接口/生成客户端、权限、门户/工具、审计、配置和文档保持一致；单文件小于 2000 行。
- [ ] 保存测试范围、失败迭代、源码/版本与证据；只完成 mock 或单层测试不计真实验收。

## 验收关联

对应 [验收矩阵](./09-acceptance-matrix.md)：P8-A27、P8-A36、P8-A37、P8-A40、P8-A59。

新增验收关联：P8-A65、P8-A67、P8-A68、P8-A69、P8-A70、P8-A71、P8-A72。

## 实施记录

已加入共享 Kubernetes 对象读取、31 个内置类型、UID 绑定的 CRD 类型发现、服务端分页、Secret 投影和有界 watch；Direct 与 Connector 使用相同对象契约，门户/API 已接入。Namespace 独立授权、显式空范围、分页游标及监听期间撤权经真实数据库验证；真实集群验证分页、Secret 屏蔽、Namespace 重建与 list/watch 衔接。完整 Connector 部署链路、全部类型、动态类型及写操作整体验收仍在进行，见[实施记录](./14-implementation-log.md)。
