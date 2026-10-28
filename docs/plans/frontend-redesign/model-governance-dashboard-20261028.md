# 模型治理迁入内置仪表盘

日期：2026-10-28。状态：已实现，已完成真实 APImart 调用与隔离数据链路验收；已发布到本机 evaluation 环境，当前版本 `local-20261028-model-controls-align`，见[本地部署记录](../../model-governance-local-deployment-20261028.md)。模型设置不再维护独立治理 Tab。第 2 节保留迁移前核查，第 7 节记录当前实现与验收边界。

## 1. 产品结构

- “观测 → 仪表盘”提供受管内置仪表盘“模型治理”，使用现有 Dashboard Spec、统一图表、布局、查询状态和刷新能力。
- “AIOps 设置”保留“模型设置 / MCP 连接”两个一级 Tab；模型设置内部移除“模型 / 治理仪表盘”二级 Tab，直接展示模型列表。
- 列表顶部“查看治理仪表盘”打开全部授权模型；行内同类操作打开相同仪表盘并预选该模型。URL 携带的模型、部门和时间只收窄范围，不产生授权。
- 内置定义受版本管理、只读、随系统升级；用户不能编辑、复制或将其内部查询目标移入自定义仪表盘。其他资源默认仪表盘继续执行现有复制规则。
- 模型与额度的修改继续放在设置页；企业管理员管理模型与额度，部门额度管理员看到可管理范围内的模型及额度操作，纯用量读取者进入只读仪表盘。不能因删除治理 Tab 丢失部门额度入口。

## 2. 迁移前代码核查

| 能力 | 当前事实 | 迁移要求 |
| --- | --- | --- |
| 图表配置 | `internal/dashboard/validation.go` 支持 stat、timeseries、gauge、bar、table 等；已有 PromQL 编译及直方图分位数查询模板 | 复用配置和图表，不嵌入旧 `GovernanceView` |
| 内置模板 | `template_registry.go` 有不可变版本、稳定企业实例 ID、受管升级；模板要求关联现有采集集成 | 增加明确的内部模板类别和绑定，不能虚构采集集成 |
| 查询范围 | Dashboard ScopeResolver 解析受管资源；模型不在 `ApplicableResourceTypes`、ResourceRef/Source 的现有支持范围中 | 增加内部模型治理数据域，独立解析企业/部门/模型范围；不借用主机、应用身份 |
| 时间范围 | `validation.go`、`runtime.go`、目录和分析参数目前限制最多 7 天 | 定义自然月聚合与历史月份查询，按数据域设置预算；不能仅放宽前端控件 |
| 真实模型统计 | `model_calls` 有状态、延迟、Token 来源、缓存 Token、金额快照及调用种类；当前 `ListModelUsage` 仅按企业/模型/月聚合部分字段 | 建立完整统计与可靠投影，补齐真实趋势、失败和维度 |
| 真实门户适配 | `adapters/real/agent.ts` 的 `usage()` 将有记录时的成功率写为 1，延迟和多类错误数写为 0，排行 points 始终为空 | 删除固定补值；未观测/未实现指标明确 unavailable，不能当真实零或成功 |
| 部门隔离 | 当前 `ListModelUsage` 调用链传入 enterprise、month、model，没有部门/主体参数；查询本身不按部门收窄 | 新数据域和保留的模型用量 API 都必须落实部门范围，不能只隐藏下拉选项 |

入口证据：

- [当前治理页](../../../web/apps/enterprise/src/pages/settings-ai-page.tsx)、[真实适配](../../../web/packages/api-client/src/adapters/real/agent.ts)。
- [用量 HTTP](../../../internal/transport/httpapi/model_handler.go)、[模型领域服务](../../../internal/model/service.go)、[聚合 SQL](../../../internal/storage/postgres/queries/model.sql)。
- [模板注册](../../../internal/dashboard/template_registry.go)、[配置校验](../../../internal/dashboard/validation.go)、[运行范围](../../../internal/dashboard/runtime_scope.go)。

## 3. 数据链路与边界

```text
ModelCall / 用量来源 / 调用时价格 / 配额与结算（PostgreSQL 事实）
  → 事务内 outbox + 可重放的内部投影
  → 指标服务的私有模型治理数据域（共享服务与查询引擎，独立逻辑表/访问规则）
  → 服务端核验内置模板版本 + 当前企业/部门权限 + 模型与时间条件
  → 固定内部来源的 PromQL 查询
  → 通用 Dashboard Spec / 查询结果 / 图表渲染
```

这是对现有“受管资源遥测”边界的显式扩展，不增加第四种查询语言或另一个图表系统。数据域标识、内部模板身份和强制权限范围必须由服务端生成并传递到查询服务，不能相信浏览器提交的 `internal=true`、指标名前缀或任意模板 Key。

指标服务承接分析投影；额度预留、扣减、限额判断和账务事实继续以 PostgreSQL 为准。投影延迟或中断不能影响结算，仪表盘展示数据截止时间、投影状态与完整性，不把未追平的金额称为精确账单。

投影以持久事件/调用 ID 去重，支持进程重启、批次重试、历史回填和多副本。调用 ID 只用作内部幂等键，不作为时序标签。不能仅用进程内 Counter 承接账务数据，也不能将采样后的累计计数差额当作精确月账单。

维度按业务需要受控：模型 ID、调用种类（inference / compaction）、状态、用量来源、部门，以及确有授权需求的用户 ID。企业与部门隔离由查询执行层强制执行，不能依赖可编辑 PromQL 标签条件。用户排行须设基数、分页/Top N 和查询预算；不写入提示词、响应、凭据、任意 URL 或会话内容。

## 4. 仅供内置仪表盘使用

- 内部数据只经服务端确认的内置“模型治理”模板查询；普通 `/observe`、通用指标/标签目录、用户仪表盘、告警、自有工具、MCP 和 Workspace 导出均不获得该来源。
- 内部数据域采用独立逻辑存储/路由与来源访问规则。即使用户知道指标名称、复制查询语句、伪造模板标识，也不能通过公开查询路径读取。
- `telemetry.dashboard.read` 不能自动授予模型用量权限。目录、详情、变量候选、执行、刷新、历史版本及缓存均要求模型治理访问策略；部门管理员始终限定本部门。
- 模型治理权限不能自动赋予其他资源仪表盘访问权。纯模型治理用户应能看到该内置入口，而不需要获得所有普通仪表盘的权限。
- 平台管理员不因维护指标服务获得企业业务模型用量的产品访问权。
- 查询请求、缓存键与结果复验绑定数据域、企业、主体、部门范围、授权版本、模板 Revision 和参数。撤权后不能复用旧结果访问内部数据。
- 保留的旧模型用量 API 同步按主体和部门授权，避免仪表盘隔离完成后旧接口仍可越范围读取。

## 5. 仪表盘配置内容与统计语义

| 内容 | 通用图表 | 数据语义 |
| --- | --- | --- |
| 调用数、成功率、错误数 | stat | 基于实际模型调用终态；明确 inference/compaction 范围和成功率分母；零调用时成功率 unavailable |
| 输入/输出/缓存 Token | stat、timeseries | Provider 实报与 estimated/missing/invalid 分离；缓存为输入子集，不重复加总 |
| 金额与月额度 | stat、gauge | 调用时价格快照与结算投影；未配置额度表示无限，不参与有限额度除法 |
| 平均延迟、P95/P99 | timeseries | 由有效实际时延形成统计；无样本不补 0；分位数不能平均各组分位数 |
| 模型、部门、用户排行 | bar、table | 在强制权限范围内聚合、Top N/分页；名称解析也在授权范围内 |
| 错误类型及用量完整性 | bar、table、stat | 分类由实际记录定义；Tool Calling/结构化输出失败需先确认可靠分类字段，否则显示不可用 |

趋势时间范围和自然月额度不是同一语义：趋势受所选时间控制；“当月金额/额度”必须标明月份、企业时区和截止时间，不能用最近 7 天代替月累计。历史月份由可重建的月度聚合读取。需要月级趋势时，新增私有聚合粒度和服务端预算后再开放更长范围。

模型筛选使用仪表盘变量；当前深链接为 `/dashboards/{id}?model={model_id}&month=YYYY-MM`。内部数据域契约、OpenAPI、共享 API Client、后端和 UI 已同步，具体实现见第 7 节。

## 6. 配置能力验证与实施门禁

2026-10-28 已运行配置级探针：累计调用 stat、Token rate 趋势、额度 gauge、部门 Top N bar、模型 table、P95 趋势六个面板，现有 `Validate` 与 `CompileTarget` 全部通过。探针为单独隔离图表层暂用已支持的 application 类型，未创建应用、未注册内部来源、未查询或写入遥测；这不证明内部数据域已支持。将类型替换成 `internal_model` 后六个面板均被现有验证器拒绝；31 天默认范围也被拒绝，与源码核查一致。

另运行并通过既有的内置模板编译、展示配置和查询步长/结果类型测试。探针及结果在本地 `build/model-governance-dashboard-review/`，不是生产模板。

正式迁移按以下依赖顺序完成，不先把模型按钮指向空壳仪表盘：

1. 固化内部模板、数据域、主体/部门权限、自然月和缺失值契约，补齐当前统计接口缺口。
2. 实现真实事实投影、去重/回填、数据截止时间及受限来源查询；保留现有结算路径。
3. 用标准 Spec 注册模型治理内置仪表盘，统一目录/运行态/候选/缓存授权；禁止内部来源复制与公开查询。
4. 模型列表增加全局与单模型跳转，移除二级 Tab 和硬编码 GovernanceView；额度操作保留在设置侧。
5. 自动化覆盖真实调用成功/失败/取消/Compaction、实报/估算/缺失 Token、月边界、投影重复/延迟/重启、无数据、跨企业/部门、撤权、伪造模板、公开目录和原始查询绕过、复制与工具导出拒绝。
6. 桌面中英明暗 E2E 验证从模型设置跳转、模型筛选与返回，并用隔离环境实测 PostgreSQL → 指标服务 → 仪表盘结果与事实对账后，再移除旧治理展示。配置探针和 mock 都不能代替此门禁。

## 7. 当前实现与验收

### 实现

- 内置 Key 为 `argus.model-governance`，沿用受管模板版本、企业稳定 ID、标准 `DashboardSpec`、`DashboardGrid`、`PanelTile` 与 `ObservationPanel`。19 个面板均由配置定义；调用数/Token 使用整数，金额保留 6 位小数，避免真实小额用量被显示为 0.00。
- 模型设置只有列表和查看按钮；部门管理员保留本部门成员额度操作，纯用量用户仍能进入内置仪表盘。内置条目不会进入 Chat 引用或资源绑定候选，也不允许复制、修改、归档或将其私有来源保存为自定义草稿。
- 月份、模型与刷新按钮使用字段行底边对齐布局，按钮与两个输入控件保持同一高度基线；不通过固定像素偏移补偿标签。1280/1920 宽度、中英文及深浅主题的 8 项几何对齐与刷新交互检查通过。
- PostgreSQL migration 51 新增 `model_metric_outbox`。终态调用、额度策略、预留/结算/释放状态均在业务事务内写入无凭据、无提示词的快照；旧记录可回填。Writer 使用 `FOR UPDATE SKIP LOCKED` 分批投影，只有 ClickHouse 写入成功才确认；未知事件类型不确认。
- ClickHouse 私有表为 `argus_internal_model_calls_v1`、`argus_internal_model_quotas_v1`、`argus_internal_model_reservations_v1`。事件版本去重和 `FINAL` 读取避免重启重放重复计数；这些表不在公开 TenantTableRouter、指标目录或外部采集路径中。
- 指标服务新增 `ModelGovernanceService.QueryModelGovernance` 内部 RPC，只接受控制面 Server 的 mTLS 身份。请求只能选择月份、模型与固定面板，不接收任意查询语句；服务端检查内置契约 Hash、当前主体/企业/部门和授权版本，返回前再次验证。普通 Worker 身份不能调用，内部结果不使用跨请求缓存。
- `internal/modelmetrics` 将授权范围内的事实映射为内部数值序列，使用现有 Prometheus PromQL 库执行内置配置；没有新增查询语言或专用前端统计卡片。每天的聚合按企业当地日历边界生成，P95 从实际有效延迟计算，不平均各组 P95。
- 调用数和金额按完成时间查询自然月；额度占用按预留记录的账期计算 `active.reserved_amount + settled.settled_amount`，释放记录不计入，不用完成时间重新推算账期。新预留使用企业时区确定月份，历史账期不改写。所选历史月份对照的是**当前已知部门限额**，不宣称重建历史限额。
- 实报/估算/缺失用量、缓存完整性、同步待处理数和投影更新时间明确展示。缺失观测不补 0，空成功率不补 100%。Tool Calling 与结构化输出失败暂无可靠独立分类，保持不可用。额度判断和扣减继续使用 PostgreSQL。
- 单次自然月查询最多读取 100,000 条调用和 100,000 条预留记录，最多 5,000 个调用维度组合；目录最多 1,000 个模型、10,000 个成员/部门。超预算明确失败，不静默截断。排名以 ID 与名称共同分组，避免同名用户合并。

### 证据

真实集成测试：[model_governance_integration_test.go](../../../internal/transport/httpapi/model_governance_integration_test.go)。测试在自有临时命名空间运行 PostgreSQL / ClickHouse，以真实 HTTP 路由、真实双向 TLS RPC、生产 Agent Loop 与生产投影代码验证，不 mock 数据库或查询返回值。

已验证：自然月超过 7 天的查询与边界、企业时区、实报与估算分离、真实失败/成功计数的受控组合、预留/结算额度口径、重复投影、跨企业/部门拒绝、撤权、普通仪表盘权限不能读取模型数据、私有定义复制拒绝、公开 PromQL/指标目录不出现内部数据、Worker RPC 身份拒绝。Query 数据库角色不能读取模型凭据，Writer 不能读取原始模型调用表。

真实模型验收使用用户指定的 APImart `glm-5.3-flash`，入口为其[官方文档](https://docs.apimart.ai/)说明的 OpenAI Compatible API。模型接入的基础/流式/工具/结构化检查通过后，通过 HTTP 创建会话和发送消息，生产 Agent 完成调用，再将 PostgreSQL 调用与结算记录、私有投影、PromQL 结果及页面逐项对账。测试价格为每百万输入 1、输出 2，金额仅验证配置价格快照的计算，不代表 APImart 的实际扣费或报价。

浏览器验收：[model-governance-real.spec.ts](../../../web/apps/enterprise/e2e/model-governance-real.spec.ts)，覆盖中文/英文 × 浅色/深色、列表按钮跳转、单模型/月份深链接、刷新、返回目录、不可复制、无二级 Tab、真实结果和小额金额显示；检查严重/关键无障碍问题及横向溢出。另有 18 项导航/权限 Mock E2E 和 API Client 回归。

本地证据位于 `build/model-governance-e2e/`：`live.log`、`live-receipt.json`、`final-controlled.log`、`api-client-tests.log`、`final-go-tests.log` 与 `browser-live/` 截图。最终真实轮次为 1 次成功调用、实报输入 3678 / 输出 3 Token、估算 Token 为 0、用量完整；按测试配置价格得到金额 0.003684，与仪表盘一致。4 项真实浏览器、18 项导航 Mock E2E、133 项 API Client 和 260 项 Enterprise 单元测试通过；相关 Go 包与契约测试通过。

网络失败轮次单独保留在 `live-network-eof*.log`、`live-partial-usage-observed.log`，没有当作干净调用通过。验收要求至少一次成功调用，且每个成功调用都有供应方完整用量；如果出现失败调用，则逐项对账失败数、成功率、实报/估算来源和不完整标志，不因最后一次成功而删除失败记录或强行标记完整。

凭据不写入报告；测试使用随机生成的单独加密密钥及临时凭据文件，验收后清理。宿主存在 Fake-IP DNS 和直连 EOF，验收使用公开 DNS 返回的公网地址及测试进程 `GODEBUG=tlsmlkem=0`，保留原有公网地址策略、固定 IP 拨号、TLS 1.3 和主机名/证书验证。没有修改正式环境网络配置，也没有声称默认网络路径已稳定；正式部署接入模型时仍需确认 DNS 与 TLS 路径。

### 发布条件

本机已按后续授权更新 `argus-system` 并执行 migration 51；evaluation 沿用原有共享数据库账号。后续发布仍需一起更新 Server、Telemetry Query、Telemetry Writer 并先执行 PostgreSQL migration 51 / Runtime Role 权限同步。Query 使用既有 Schema 角色创建私有表；Writer 在表暂不可用时保留 outbox 并重试。旧 Query 不支持新 RPC 时明确不可用，不回退旧固定统计或 mock。

隔离验收阶段收尾：拥有者标签匹配的临时命名空间 `argus-modelgov-e2e-20261028-a21f` 已删除；临时 API Key 文件已删除，报告/源码凭据扫描通过。现有 `argus-system` 的 20/20 个 Running Pod 全部 Ready，当时主环境仍为 schema 50；后续已升级至 51，数据保留与部署后健康证明见独立部署记录。
