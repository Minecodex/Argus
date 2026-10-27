# K8s 存储、Job 就绪和服务端分配字段

状态：约束、原生实际操作和分层验证已补充；T11 整体仍未完成。

## PVC 扩容

创建 PVC 保持 Kubernetes 的正常语义。已有 PVC 修改 storage request 时禁止缩小，等价单位不视为变更；增加必须读取原 StorageClass 的实际 `allowVolumeExpansion`。权限不足、没有能力证明或明确不支持时拒绝。输入与原生 dry-run 返回对象都受这一检查，实际写后异常不能被报告为成功。

异步就绪判断要求 PVC Bound、实际 status capacity 达到请求容量，并且没有 Resizing / FileSystemResizePending。API 接受或 spec 增大都不等于底层容量已经完成；Lost 明确失败。

- [12 个存储策略/就绪场景](../../artifacts/plan8-pvc-expansion-readiness-unit-verified.log)。首次测试缺少对象 GVK，正确触发协议拒绝；修正夹具后重跑，未放宽校验。
- [实际已绑定 PVC 的拒绝和清理](../../artifacts/plan8-pvc-native-unsupported-shrink.log)：`argus-monitoring` 的 `standard` 为 `k8s.io/minikube-hostpath`，未声明扩容能力。增加和缩小都没有改变实际 1Mi 请求；本轮 PVC、PV、Namespace 清理通过。
- 真正的成功扩容仍需支持扩容的 CSI/后端验证，当前 Minikube 结果不能代替该门禁。

## Job 实际状态

Job 不再沿用通用对象“写入接受即完成”的分支。就绪来自实际 Ready Pod 数满足并行度、Complete 条件或已经生效的暂停；失败条件明确失败。仅 active 或仅创建成功不能证明 Pod 已经就绪。

每次 Job dry-run 都会获得不同 UID，其自动生成的 controller UID 选择器也随之改变。摘要规范化仅移除与该对象 UID 相等的自动选择器值，保留手工选择器及其他标签；实际对象及选择器没有被改写。

- [Job 摘要和 8 类就绪状态](../../artifacts/plan8-job-digest-readiness-unit.log)
- [真实 Job 创建与完成](../../artifacts/plan8-job-native-approval-completion.log)：重复 dry-run 摘要一致、确认写入成功、实际完成可观察，独立 Namespace 清理通过。该原生组件测试不代表正式门户/Connector/Agent 全链路。

## Service 分配与 PVC 绑定

Service 的自动 ClusterIP/ClusterIPs/NodePort，以及 PVC 后续控制器绑定不能使原本批准的请求永久进入 conflict/unknown。摘要按输入是否显式指定分配值处理：自动分配可变化，明确选择和其他字段仍冻结。PVC 保留 Controller 拥有的绑定注解；用户不能通过通用清单新增、修改或删除这些注解。

- [字段规范化、明确输入与注解保护](../../artifacts/plan8-service-pvc-allocated-fields-unit.log)
- [实际 ClusterIP、NodePort、PVC 绑定](../../artifacts/plan8-service-pvc-native-allocations.log)：多次预览、实际写入及控制器绑定后的只读结果匹配通过；相关资源与卷按拥有的 Namespace/UID 清理。

这批工作没有覆盖 Helm、drain、工作负载回滚、实时会话或所有服务转换形式。剩余类型及路径继续按 T11 门禁逐项验证。
