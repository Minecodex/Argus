# 未验证 Namespace 归属与采集可用性

r54 在真实 Gateway 重建、上游停止后仍有指标队列，固定错误码确认为 `NAMESPACE_OBJECT_METADATA_PENDING`。Kubernetes 库存通过异步读取建立，短命对象或静态 Pod 的原生指标 UID 不一定能匹配 API 对象 UID。不能依 Namespace 名称或用户可编辑的 mirror annotation 授予数据权限，也不能让缺失归属证据阻断同批其它可信 Cluster 指标。

本轮调整准入语义：Collector、source、配置版本和 Cluster 根仍须全部可信。对象 UID 尚未被可信库存观测，或其 Namespace 代次已经退役时，保留真实指标为 Cluster 范围，清除输入的保留属性，不写入 `argus.namespace.uid`，由平台写入 `argus.namespace.attribution_status=unverified`。名称存在而没有对象 UID 的观测采用同一语义。Node/Cluster 自身没有 Namespace 的指标不标为归属缺失。

每个对象 UID 都检查可信库存；存在但 Namespace 名称、类型或同块归属相互矛盾时永久拒绝，不能通过另一个未知对象掩盖矛盾。数据库不可用、真正的 source 配置待应用仍有限重试，不转为 Cluster 降级。直接携带 Namespace UID 的身份继续按原可信 UID 契约验证。

Namespace 授权的 source facet 仍在聚合、目录、概览前过滤，因此不读取未验证归属数据，也不把同名重建后的 Namespace 视为旧代次。Namespace 指标查询附固定覆盖说明：只包括已验证 Namespace UID 的观测；未验证归属数据只对相应 Cluster 授权可见。这是资格说明，不伪造缺口计数或把每次查询都标为预算截断。

验证分别记录真实 PostgreSQL 身份、实际 Writer/ClickHouse 聚合与候选隔离、准入失败不改写请求、查询覆盖说明、真实 K8s 故障和总队列排空。实现或组件回归不等于完整复验，原队列断言及三分钟恢复、五分钟重试边界保留。r54 已恢复原部署和存储身份，server 1/1、自有 Namespace 清零；整体仍未完成、未发布。
