# CRD 删除依赖与稳定故障前件

日期：2026-10-21。状态：CRD 原生真实 API 与依赖单元验证通过；Helm 模板 CRD 的真实专项通过；完整部署、模型和发布继续验收。

## CRD 删除

真实专项先复现：有自定义实例时，原生删除预览返回成功。失败保留在 `artifacts/plan8-crd-delete-existing-instances-before.log`；测试仅执行 dry-run，并按归属清理自有对象。

现在同一个依赖检查用于原生 CRD 删除、Helm 模板中 CRD 的删除预览和每次实际删除。读取该 CRD 一个已服务版本的全部 Namespace 实例集合，优先存储版本；同一 CRD 的服务版本对应相同底层实例集合，查询只需一项即可确认有依赖。发现实例返回 `KUBERNETES_CRD_HAS_INSTANCES`；无可读取版本、权限不足、读取失败或不完整结果均拒绝删除。

真实原生专项已观察到：有实例拒绝预览，预览后创建新实例会拒绝实际写入，清空实例后允许删除。证据 `artifacts/plan8-crd-delete-existing-instances-after.log`（10.19 秒）；拒绝/不完整读取/跨 Namespace 和错误码往返单元验证见 `artifacts/plan8-crd-dependency-unit-fixed.log`。HTTP 返回 409，门户提供中英文原因。检查在每次写入前重做，不将 Kubernetes 跨对象读取和删除描述为原子事务。

内部 Mutation 写协议升至 v6、Helm 协议升至 v3，服务端角色能力清单复用当前 Helm 版本常量；旧执行器拒绝新协议，不保留长期双轨。需要同步部署 Server、Gateway 和 Connector。Collector 发行版及三平台二进制不因本次 Kubernetes 写入保护而改动。Helm `crds/` 中原有安装后保留策略继续适用，模板内的 CRD 也不能绕过依赖检查。

## 故障前件

r48 主机日志故障准备匹配到新 revision 6，但状态为 `result_unknown`，整轮失败且模型未运行。其控制日志在切换正常后端后仍出现旧 Gateway 的 draining 断连。原环境恢复 passed、server 1/1、本轮 Kind Namespace 清零后才修改源码。

Deployment 的副本计数可能不计仍在执行 preStop 的旧 Pod。验收等待现同时核对真实选中 Pod：无旧 Pod 正在删除，数量符合期望且全部 Ready。故障窗口还要求独立观察本轮 Collector 恰好一次启动并处于 `start-pre`，不只依赖易变化的数据库命令状态；未知状态并不等于完成。仍保留原 12 秒准备、20 秒启动延迟，以及最后原命令成功、connection epoch 前进、实际一次启动和新主机样本的全部断言。

准备失败时，先持久保存命令、版本、进程子状态、启动次数和不含私有载荷的命令历史，再清理，避免 Namespace 删除后失去证据。日志故障专项失败仍计失败；只有真实控制收敛和新主机指标均确认后，才继续没有该日志前置依赖的 K8s、数据库和 SDK/模型场景。整轮保持非零退出，不能因此获得发布资格。

针对性结果：`artifacts/plan8-draining-fault-guards-targeted.log` 通过。完整真实重跑仍待执行，不能用单元验证抵扣 r48 的正式失败。


## Helm 与自定义资源补验

`artifacts/plan8-helm-crd-dependencies-live-r2.log` 实际通过 8.63 秒：安装模板内 CRD、已有实例拒绝卸载预览、预览后新实例阻断实际删除，CRD/实例 UID 保留。初次真实测试已阻止删除，但 Helm 的文本聚合丢失了具体错误；现从独立记录的已知写入拒绝恢复安全错误码，未知结果保持更高优先级，不传递未知或私有错误。Helm 已写发布元数据时仍明确报告部分写入，不能声称原子回滚。

`TestLiveCustomTypeDiscoveryAndRecreation` 现包含真实预览无副作用、新建、Schema 拒绝、更新、等待删除完成及旧类型 UID 拒绝；`artifacts/plan8-custom-resource-mutations-live-r3.log` 通过 16.07 秒。测试初次缺显式 Namespace、后又误把删除请求接受当完成，均已修正夹具/观测断言，没有放宽生产校验。

新六包回归的四包通过后，两项旧验收夹具缺 Pod/selector 导致失败；已补合实际对象契约，`artifacts/plan8-crd-rollout-final-two-packages.log` 的 Inventory 和 Argus Dev 全包分别通过 0.871/18.453 秒。旧失败日志保留；这不是新源码的全仓库最终验收记录。

## 追加真实服务链

已将专用服务的实际创建、私有凭据、取数、默认面板与启停抽为同一验收方法，并在模型之后新增 PostgreSQL 正式阶段，避免新集成失败遮住模型。PostgreSQL 夹具真实启动、受限监控身份、实际服务版本/镜像、数据工作负载和归属清理已通过 `artifacts/plan8-postgresql-actual-fixture.log`；该结果不冒充正式 PKI/Kafka/平台取数链，新增阶段仍须实际运行。
