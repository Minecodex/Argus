# 应用、监控与工作台原生工具接入

2026-10-19，在[完成度审计](./32-implementation-audit.md)之后补充实现。本记录只覆盖原生工具及其结果边界，不表示 T15 或 Plan8 已全部完成。

## 已实现

- 新增 `application`、`monitoring` 两个分类，与既有 `k8s` 分类共用 `tool.search(category) → tool.describe → tool.invoke`。发现协议修订为 3；同一双语分类说明同时用于模型导航、元工具和目录签名。
- Server、Worker、工具名称生成器和模板测试目录统一注册新适配器。新增 20 个可发现工具及 4 个隐藏配对 Commit；隐藏 Commit 只作配对声明，实际提交仍由冻结的领域计划及 Action Executor 执行。
- 每个工具的版本签名同时包含输入/输出 Schema、权限、风险、执行模式和投影契约；即使人工版本号没有调整，修改这些契约也不能复用旧的描述授权。
- 直接复用 `monitoring.Service` 和 `kubernetesworkbench.Service`，不维护另一份授权或状态。服务端重新读取用户/后台账号状态、授权版本和后台账号允许工具列表；未知主体类型拒绝。

| 分类 | 工具范围 |
| --- | --- |
| application，6 个 | 应用列表/详情、服务列表、接入点列表、Key 元数据列表、配置变更预览 |
| monitoring，8 个 | 目标列表、实例列表/详情、Collector 选择、集成目录、实例变更预览、自身能力列表/启停预览 |
| k8s，6 个新增 | 类型发现、任意已注册对象列表/详情、操作列表/详情、对象变更预览 |

应用环境仍不可原地修改；SDK 接入仍不要求预建服务。目录结果保留分页和显式 Namespace 空集合；一页不代表完整资源目录。实例继续分别表达期望、生效、健康和数据新鲜度。

## 敏感输入与结果

公开工具 Schema 不包含认证输入、Secret 写入值或私有草稿通道；嵌套输入继续检查统一凭据规则及 name/value 字段。K8s Secret 写入、监控认证替换通过可信门户完成，不能用通用对象工具代填。工具可创建/撤销 Key 的预览，但不能读取 Key 原值；交付仍走用户的一次性结果。

公开预览的 `action_ref` 供宿主确认卡片使用，与私有 Commit 能力不同。初次验证发现通用展示脱敏误删此引用，现仅在可信预览结果的结构化根节点保留；展示数据仍不携带它，也不返回不可变计划、提交 Token 或私有暂存。

新结果复用现有受隔离模板、分页、公开字段详情和预览卡片。Presentation 授权快照新增应用、监控目标、Namespace 的带类型授权；数据库资源名称解析与 resources.context 同步支持这些类型。应用撤权后保存的展示快照失效，应用授权不能变成监控目标或集群授权。应用资源可从会话打开正式详情页。

## 已有证据

| 验证 | 结果与范围 |
| --- | --- |
| `artifacts/plan8-native-domains-go-regression.log` | 8 个包回归通过；后续自身监控新增内容另行回归 |
| `artifacts/plan8-native-domains-process-postgres.log` | 实际 PostgreSQL 应用目录隔离、无权/撤权拒绝、公开预览及无副作用、展示授权失效；自身实例既有工作流同轮回归通过 |
| `artifacts/plan8-native-domains-sdk-real-portal.log` | 实际 SDK 服务自动发现，经分类发现/描述/调用读取目录、同应用限制、结果绑定与撤权；原三信号/real 门户四场景同轮通过 |
| `artifacts/plan8-native-domains-template-runtime-final.log` | 897 项模板运行时测试通过，覆盖新工具的共享模板及公开 DTO；属于受控展示验证 |
| `artifacts/plan8-native-domain-template-browser-verified.log` | 新资源列表/详情共享模板在中英、明暗、三种桌面宽度共 12 个实际浏览器场景通过；属于明确的 mock 宿主及展示数据 |

SDK 联合验证依旧使用明确的控制注册/颁证及可信会话初始化夹具，见[原验证边界](./30-native-sdk-pipeline-and-signal-privacy.md)。本次没有声明真实模型调用、正式 Collector 注册、独立部署 Query RPC 或部署后全平台验收通过。

## 尚未完成

后续补充了应用/目标/Namespace 撤权对结果、文件、导出、目录和续约的真实数据库证明，见[Workspace 来源验证](./38-workspace-typed-source-revocation.md)；这是明确的 fake 计算，不替代实际 PVC/模型链路。

尚需完成模型侧可信敏感表单的宿主发起与回填、完整 Workspace/来源和文件交付、后台主体全组合、真实模型业务验证，以及 Helm/drain/会话等尚未交付功能的工具接入。通用对象变更工具只覆盖当前领域服务已经实现的能力，不能据工具名称推定完整 K8s 工作台已完成。Namespace/目标详情导航及完整部署闭环继续收口。
