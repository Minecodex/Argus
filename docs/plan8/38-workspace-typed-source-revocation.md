# Workspace 中应用及新资源来源的撤权

2026-10-19，补充[原生工具](./33-native-domain-tools.md)后，新应用、监控目标及 Namespace 授权已经进入共享 Presentation/结果授权签名。Workspace 沿用同一来源检查，本次验证该边界贯通持久目录。

新增真实 PostgreSQL 三组：application、monitoring_target、kubernetes_namespace。在保存有权来源及文件依赖后，**只删除对应数据授权，不增加用户 authorization_version**。重新获取用户身份后验证：

- 原会话结果、文件读取和导出均拒绝；
- 即使 workspace_files 元数据丢失，目录自身的 source_result_refs 仍阻止计算/文件访问；
- 拒绝发生在来源注册及挂载前，不产生 Pod/计算副作用；
- 已持有访问的续约会取消上下文，用户显式清理仍可以进行。

应用授权不能借相同 UUID 变成目标/Namespace 授权。Namespace 夹具使用实际数据库中的 Cluster 外键和独立 UID；重建同名 Namespace 的其他边界仍由既有领域用例验证。

证据：`artifacts/plan8-workspace-typed-source-revocation.log`，三个带类型授权场景及原认证版本场景通过。这里只使用真实数据库和存储授权服务，Kubernetes/计算为明确 fake；没有分配真实 PVC 或调用真实模型。因此不宣称 Workspace/模型业务全链路完成。
