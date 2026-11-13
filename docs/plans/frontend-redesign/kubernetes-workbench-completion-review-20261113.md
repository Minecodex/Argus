# Kubernetes 工作台第二次完成度复核

日期：2026-11-13。本文保留二次复核发现；所列 C01～C05 随后已完成修复和复验，最新状态见 [剩余项补齐验收](./kubernetes-workbench-closure-20261113.md)。以下“尚未完成”及状态表为修复前审查结论。

上一轮的实际部署、样式改进、28 个真实浏览器用例、健康检查和清理证据有效。但这些用例覆盖的主流程不能替代原计划的全部字段、关联和结束状态。本次以原计划条款逐项核对，确认下面五组剩余问题。

检查范围为 D:/workspace/go/Argus 当前代码。生产源码与冻结构建内容一致，现场仍为 backend v9 / Web v11，相关 Deployment Ready。本次只进行了只读现场核对、代码审查、隔离的 Go overlay 探针和 mock 浏览器复现；没有修改业务代码、重新部署或向真实集群造测试数据。

## 确认的剩余项

| 编号 | 级别 | 计划条款 | 问题与影响 | 依据 / 关闭条件 |
| --- | --- | --- | --- | --- |
| C01 | 中 | KWR-03、09、14：统一原生状态与字段 | DaemonSet 的 desired_replicas 在读取 desiredNumberScheduled 前就写入，通常固定为 1；原生期望 8 时详情同时出现 8/8 与期望 1。多容器 Pod 的后续普通等待原因还会覆盖前一个容器的故障原因。 | rollout_summary.go:51/54；summary.go 的 containerStatuses 循环。直接调用当前实现复现 healthy、ready=8/8、desired=1，以及 failed、reason=ContainerCreating 覆盖 CrashLoopBackOff。需修复字段赋值及原因优先级，并验证多节点和多容器组合。 |
| C02 | 中 | KWR-05、14～17、24：关联资源完整性 | Ingress TLS Secret、projected volume 中的 ConfigMap、StatefulSet serviceName 指向的 Service，均未形成关联；合法且已授权的对象对返回空关系。部分跨 Namespace 的 ServiceAccount/RoleBinding 引用也被固定在源对象 Namespace 的读取范围排除。 | resource_relations.go:148/207、related.go:58/111/185。前三种由生产函数探针复现；跨 Namespace 项为读取范围代码核查。需按资源矩阵补齐引用解析，保持目标授权和 UID 校验，并覆盖正/反向关系。 |
| C03 | 中 | KWR-06：可分享的分页/导航上下文 | URL 仅保存当前游标；重新加载第二页链接后仍显示第二页的数据，但分页变成“第 1 页”，上一页按钮禁用。 | workbench.tsx:99/253/570。浏览器复现 URL 与行内容保留、page_label=第1页、previous_disabled=true。普通未重载的返回操作在本次对照中正常，不能笼统说所有返回都失效。需明确保存/恢复页序与可达的前页，补复制链接/刷新/重新挂载场景。 |
| C04 | 中 | KWR-08、11；计划 6.3：Job 被回收后的反馈 | 即时 Job 结果跳转已实现；但从操作历史打开已不存在的 Job，仍走通用读取错误，没有计划要求的回收/不存在说明及原操作记录入口。 | workbench.tsx:672、operation-result.ts、resource-detail.tsx:135。浏览器模拟一个保留的成功记录和已移除结果对象，显示“暂时无法读取集群资源”，操作详情入口数量为 0。需携带操作上下文，并在不猜测删除原因的前提下解释结果对象已不存在，保留可达的原记录。 |
| C05 | 中 | KWR-07、15～17：完整类型化详情 | 已补 HPA、LimitRange、EndpointSlice、CRD，但尚未逐类兑现原矩阵：Role/ClusterRole 的只读详情没有 API 组/资源/动词规则表；Ingress 路径/地址、Node 角色/完整 Conditions 等仍需到高级清单查看。部分默认列表字段也未覆盖计划清单。 | resource-configuration.tsx、resource-facts.tsx、resource-model.ts，summary.go:177/251/311。Role 注入明确 rules 后的配置页无规则表（tables=0）；其余为字段映射核对。需逐类核对原矩阵，不能以通用 YAML 或可编辑表单替代约定的只读详情。 |

这些是功能/交互缺口。B10b 接入权限不足导致的“不完整”提示，以及 evaluation 环境的网络隔离边界，仍单独作为已有环境限制记录，不混在上述缺陷中。

## 复现方法和证据

证据目录：`artifacts/kubernetes-workbench-refactor/20261112-review2/`。本次复核从 11 月 12 日延续到 13 日，目录名保留创建时日期。

- `review_probe_test.go` + `overlay.json` + `probe.log`：通过 Go overlay 调用当前生产函数；不向 internal 写入测试文件。预期正确行为的断言失败，确认 C01 两例和 C02 三类关系缺失；这不是全量测试运行。
- `review.spec.ts` + `browser-probe-final.log` + `browser/`：仅使用本地 mock 模式及独立浏览器 context，模拟分页、只读规则和回收对象。没有将 mock 复现写成真实集群验收。
- 分页对照：不重载的返回保留第 2 页；重新加载相同 URL 后变为第 1 页且无法上一页，行内容仍是原第二页。
- Role 只读配置没有展示提供的规则表；已回收 Job 路径只有通用错误和请求 ID，没有本次操作入口。
- 再次核对现场 image、Ready 状态和上一轮冻结源码哈希，当前业务实现没有漂移。

## 计划状态调整

| KWR 范围 | 当前状态 |
| --- | --- |
| 01～04 | 支持矩阵和契约已有；C01/C05 的字段与状态语义待补齐 |
| 05 | 分页、授权、UID 校验已有；C02 的关系种类覆盖不完整 |
| 06～08 | 共享列表、详情、表单与进度已有；C03/C04/C05 待补齐 |
| 09～13 | 真实日志/终端、Deployment、Job、PVC 主流程已有证据；C01 的多容器原因及 C04 的 Job 回收后路径需补验 |
| 14～17 | 各类型操作能力已有；字段、只读详情及引用关系未全部满足原矩阵 |
| 18～20 | 既有质量门禁、部署和清理记录有效；完整验收仍需覆盖本次剩余项，文档已更正 |
| 21～23 | 仪表盘 v2、共享嵌入与按集群选择已有有效实现和证据 |
| 24～25 | 大盘与资源图型已部署；C02 的引用遗漏影响关系展示完整性 |
| 26～27 | 已有权限/布局/实页证据；整体交付不能继续标为全部关闭 |

建议先补齐 C01/C02，再完成 C03～C05，并按上述反例追加回归。原有完成记录保留为历史交付事实，最终关闭必须引用新的实现和验证结果。
