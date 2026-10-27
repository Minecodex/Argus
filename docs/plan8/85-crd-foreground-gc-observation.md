# 新 CRD 前台删除的验收观察

R59 完整 84 组检查全部通过，但补充轮次中相同的自定义资源删除观察再次超过 15 秒。诊断使用相同资源、真实 Kubernetes API、原始前台删除策略及原有 90 秒工作流总期限；两次实际删除分别耗时约 10.1 秒和 16.9 秒，最终 NotFound、CRD 重建及旧 UID 拒绝均成立，未移除 finalizer 或修改集群控制器。

[Kubernetes v1.37.0 控制器实现](https://github.com/kubernetes/kubernetes/blob/v1.37.0/cmd/kube-controller-manager/app/core.go#L636-L646)使用 30 秒的垃圾回收发现同步周期。[垃圾回收器](https://github.com/kubernetes/kubernetes/blob/v1.37.0/pkg/controller/garbagecollector/garbagecollector.go#L175-L240)在该同步中更新新资源类型及监视器。API 已建立新 CRD，并不表示垃圾回收器已经发现它。Argus 使用前台级联删除；新类型尚未加入回收图时，15 秒的测试窗口可能先结束。

修正仅作用于 `TestLiveCustomTypeDiscoveryAndRecreation` 的异步结果观察：每 250 毫秒查询，在原有 90 秒工作流总期限内等待实际 NotFound。真实创建、Schema 拒绝、批准字段更新、删除、重建 UID 和旧类型身份拒绝等断言保持不变。生产删除策略、集群参数、权限和重试规则不变；五分钟日志重试上限、容量及模型预算也不变。

原失败保留在 `artifacts/plan8-supplemental-20261022-r59/`。候选诊断为 `artifacts/plan8-r59-crd-gc-observer-candidate.log`，仅用于确认时序及修正，不能当作原源码通过。源文件修正后按新的统一指纹验收，不将 R59 的成功结果改写为新版本。
