# Task 06：端到端验收与文档收口

2026-09-23：R17 已完成工程、数据库及完整 P5 b 验收，见 [最新修复记录](./fixes-r17-2026-09-23.md)。Gateway 业务发现缺项已补齐，真实模型评测仍未运行，不能将全部任务清单整体标为完成。

2026-09-22：R12/R13 已修复并通过工程、数据库和完整 P5 d，见 [最新修复与验收](./fixes-r12-r13-2026-09-22.md)。真实中文模型评测仍未运行，下方任务清单不能整体视为完成。

## 目标

用单元、契约、浏览器和临时 Kubernetes Namespace E2E 验证三类能力：自有 Registry 的三个元工具及 Preview/模板、离线四基础工具及每会话持久目录、客户 MCP 直接模型工具及文字数据。当前实施和验收进行中，实际证据与未通过门禁见 [验收报告](./acceptance-report.md)。

本矩阵已同步 [已确认决策](./02-confirmed-decisions-and-open-questions.md) Q1～Q16：客户 MCP 首期仅 Remote Streamable HTTP，运行前容量预检失败明确提示，用户业务文件上传/模型产物下载属于本期正式路径。stdio 与旧 HTTP+SSE 不在本期门禁中；产品范围已确认，不表示实施或验收已完成。

## 测试矩阵

### P5-E01：Agent 与 Context

- [ ] 无 Tool 的普通对话。
- [ ] Search → Describe → Invoke 单轮和多轮调用。
- [ ] 一个 Turn 内多个元工具调用的顺序和原生 ToolCall/ToolResult 配对。
- [ ] 两个以上 Run 的 Conversation 历史连续恢复。
- [ ] Soft/Hard Compaction、多次增量摘要、Worker 重启和 Redis 清空。
- [ ] Tool Result Projection 后再压缩，Template Source 永不进入模型输入或摘要。
- [ ] Chat Completions 与 Responses 两种协议一致。
- [ ] 自有 Catalog 500/1000 Tool 时核心 Tool Schema Token 保持常数级；客户 MCP Schema 直接注册并独立计量。
- [ ] 核心三/七工具与客户 MCP N 个工具组合为 `3 + 可选的4 + N`，外接调用不经过 Search/Describe/Invoke。
- [ ] 自有确定性提交后可自动只读验证/总结；新的自有变更重新 Preview，验证阶段不能调用外接写工具冒充只读检查。
- [ ] 显式激活一个 Skill 只增加该 Skill Instruction，不加载全量 Skill Catalog，也不改变核心 Tool Schema。

### P5-E02：Tool Discovery 与权限

- [ ] 七个分类的检索、详情、调用和分页。
- [ ] 未知分类、未知 Tool、非法参数和超预算。
- [ ] RBAC、ServiceAccount Tool Allowlist 和 explicit resource authorization。
- [ ] Describe 后撤权、资源版本变化和 AuthorizationVersion 变化。
- [ ] 自有隐藏 Commit 通过搜索、详情、调用或外接同名工具均不可直接访问。
- [ ] Catalog Reload、Manifest 顺序变化和 Redis 清空保持确定性。
- [ ] 自有 Native Tool 使用自有 Gateway；四基础工具使用离线执行接口；客户 Remote Streamable HTTP 使用直接模型工具与独立 Adapter。三类身份和执行规则不可混淆。
- [ ] 客户 MCP 读写调用不创建 Argus Preview/Commit，不要求自有分类；连接身份、Schema、企业隔离、预算和审计仍正确。
- [ ] 企业管理员管理本企业连接/凭据并授权成员，普通成员不能管理连接或读取凭据；平台管理域不代管客户业务凭据。
- [ ] 用户只能选择已授权连接，会话保存选择；新增连接不自动加入旧会话，撤权/停用后旧 Run 工具调用立即被拒绝。

### P5-E03：Template Runtime

- [ ] Host、Kubernetes、Metric、Log、Trace、Connector 和 Preview 模板真实渲染。
- [ ] Light/Dark、中文/英文、空、错误、Partial 和大数据。
- [ ] Template Hash/Version、历史刷新、缓存命中与 Artifact 缺失。
- [ ] 错误 Origin/Nonce/Sequence、重复/乱序/超大消息和销毁后消息。
- [ ] DOM、Cookie、Storage、Network、任意 API 和任意 Tool 逃逸全部失败。
- [ ] PendingAction 确认、取消、双击、过期、审批、执行成功/失败/ResultUnknown。
- [ ] 自有模板只展示详情，宿主显示权威影响范围、确认/取消及审批执行状态；用户只确认一次，不出现第二次确认。
- [ ] iframe 发送 `confirm_action/cancel_action`、伪造影响范围或自动发起确认，均不能改变宿主预览或触发执行。
- [ ] 模板加载失败时，宿主仍可根据完整有效的公开预览显示动作和状态；预览缺失/失效禁用确认。
- [ ] 浏览器和模型输入中搜索不到私有 Token、冻结参数和生产凭据。
- [ ] 客户 MCP 即使返回 HTML/Dashboard/Presentation 元信息也不创建模板 iframe，仅使用支持的文字/结构化数据。
- [ ] 查询翻页、换时间由用户新消息触发新 Tool Call；没有服务端刷新/分页 Bridge，旧查询结果不被新数据静默替换。

### P5-E04：OpenSandbox 降级

至少运行两套安装矩阵：

| 模式            | OpenSandbox | 预期                                                               |
| --------------- | ----------- | ------------------------------------------------------------------ |
| `agent-lite`    | 未配置      | 自有三元工具与 Remote MCP 正常；四基础工具不可见，文件能力独立报告，不以离线计算不可用阻断普通会话 |
| `agent-sandbox` | 配置且健康  | 核心七工具可见，可追加已启用 Remote MCP；完整验收离线分析、持久目录、业务文件上传和产物下载 |

每种模式均包含无外接 MCP 和启用 Remote Streamable HTTP 的子矩阵；stdio/旧 HTTP+SSE 在所有首期模式均不可配置或执行，不把“核心工具数”误当“模型全部工具数”。

P5 使用 M2 身份初始化和自身的模型/角色/配额 fixture，不依赖重复执行 M3/M4 的完整业务流程；两个 Provider 的原生工具调用在 P5 内验收。M4、P4、M7/M10 的受影响回归继续作为独立必需门禁，不因 P5 fixture 解耦而删除。

故障注入：

- [ ] Sandbox Backend 启动前不可达。
- [ ] Agent 运行中 Sandbox API 中断。
- [ ] Profile 删除/停用和企业配额耗尽。
- [ ] 计算实例 TTL 到期、Supervisor 崩溃和 Worker Pod 删除后，Workspace 目录仍存在且可重挂载。
- [ ] 所有故障都不触发 Worker 宿主执行 fallback。
- [ ] Sandbox 故障不使普通 Agent/Native Tool Readiness 失败。
- [ ] 离线代码不能联网，客户 MCP 可以联网；不能借共享网络、进程凭据或管理通道使 `bash` 获得联网权限。
- [ ] 关闭页面、Run 结束、长期闲置和归档均不删除工作目录；只有明确删除 Workspace 或永久删除会话才清理。
- [ ] 存储满额后停止新增写入，旧文件仍可读且不自动删除；`bash` 写文件不能绕过额度。
- [ ] 写入/覆盖/删除 → 回收实例 → 重挂载，当前完整目录状态一致；不要求恢复每次修改历史。
- [ ] 明确删除与迟到命令/实例重建竞争时，旧写入不能复活已删除 Workspace。
- [ ] 用户真实上传业务文件/大查询结果 → 持久目录 → 离线脚本分析 → 会话下载产物 → 下次 Run 继续使用，完整浏览器/服务端链路成立。
- [ ] 上传仅在字节持久保存后成功；中断、重名、大小/容量超限和跨企业请求被正确处理，不损坏旧文件或伪造可读附件。
- [ ] 下载内容与交付 Hash 一致，页面刷新和实例回收后可下载；同名工作文件更新不改写历史交付内容，明确删除后不能取得已清理文件。
- [ ] 文件上传/下载属于宿主会话入口，不授予模板 iframe 下载、执行或网络权限。
- [ ] 镜像预装的语言/分析库可在无网络环境运行；环境能力清单与镜像一致，缺依赖不会联网或进入用户上传依赖安装流程。

### P5-E05：客户 MCP 直接模型工具

本期仅验收 Remote Streamable HTTP。stdio 和旧双端点 HTTP+SSE 明确不交付，不以其正向接入测试阻塞本期完成。

- [ ] Streamable HTTP JSON 响应。
- [ ] Streamable HTTP SSE 响应和断线取消。
- [ ] Session ID、重连、超时和服务端通知。
- [ ] TLS、认证、非法 Origin、DNS Rebinding、SSRF 和私网地址策略。
- [ ] 配置与执行拒绝不支持的 stdio/旧 HTTP+SSE，不自动降级；Streamable HTTP 的 SSE 响应正常。
- [ ] 客户 Result 的文字/结构化投影、大小和错误语义，不生成 Presentation。
- [ ] 上游 Schema 直接提供模型，别名稳定且不会覆盖核心 Tool；连接更新/撤权不能让旧调用静默执行新的工具定义。
- [ ] 所选连接的可用工具直接注入，用户调整会话选择影响后续 Run；实时撤权/停用不等待新 Run 才生效。
- [ ] 客户写工具直接执行，不套自有 Preview/Commit；调用回执丢失不自动声称未执行，不盲目重放无幂等保证的写调用。

## Kubernetes E2E

### P5-E06：临时 Namespace 流程

按照项目约束使用一次性 Namespace：

```text
doctor capability check
→ acquire global e2e lease
→ record original replica counts and optionally scale down owned normal test services
→ create unique namespace
→ install agent-lite profile and run suite
→ upgrade/reinstall agent-sandbox profile
→ run sandbox/mcp/fault suite
→ collect sanitized evidence
→ explicitly delete this test's workspaces/conversations and their processes/artifacts
→ uninstall release and delete namespace/PVC/lease
→ verify zero residue
→ restore recorded replicas and verify normal services and business entrypoint
```

- [ ] 测试前检查 Kubernetes Context、RuntimeClass、StorageClass、磁盘和镜像架构。
- [ ] 只有本次测试拥有的普通测试服务可以暂停；不得改动未知或生产 Namespace。
- [ ] Cleanup 在成功、失败、超时和中断路径都运行。
- [ ] 实例 TTL 清理不删除持久目录；E2E 清理必须显式删除本次测试拥有的 Workspace/会话，不触及其他会话文件。
- [ ] 暂停前保存原副本数，结束后恢复并验证就绪及业务入口；恢复失败作为明确验收失败记录。
- [ ] 证据只保存 Hash、状态、计数和脱敏截图，不保存 Secret、文件正文或私有 Tool Result。
- [ ] 最终扫描 Namespace、PVC、Cluster RBAC、Sandbox Session、进程、Artifact Fixture 和 Lease 零残留。

## 性能与容量

### P5-E07：预算门禁

- [ ] 无客户 MCP 时核心三/七工具 Schema Token 建立固定基线；外接 N 个工具另外统计 Schema Token 与总上下文。
- [ ] 500/1000 自有 Tool Catalog 不增加初始核心 Schema Token；外接 Tool 数量增长可以增加实际模型 Schema 成本，不宣称总成本恒定。
- [ ] Search P95、Describe P95、Native Invoke P95、Sandbox 冷/热启动和 Template 首帧建立预算。
- [ ] 使用中文跨领域真实任务验证成功率、总 Token、模型往返数和首个有效结果延迟，不能只测工具数量。
- [ ] 外接 Schema/必要上下文超模型容量时运行前明确拒绝，无模型或业务 Tool 执行；提示减少连接/换模型，保留消息/附件/选择，不静默丢弃工具或自动换模型。
- [ ] 用户减少连接或显式更换模型后重新预检可运行；单个连接工具就超限时同样给出明确容量诊断，不能靠压缩历史掩盖。
- [ ] Template 256 KiB、Presentation 1 MiB、Model Projection 64 KiB 和内联 Tool Result Artifact 4 MiB 上限都有边界测试；独立业务文件传输/交付按文件与 Workspace 额度验收，不受内联 Artifact 上限误限。
- [ ] SSE 背压、浏览器慢消费和 Tool Result Artifact 路径有容量测试。
- [ ] Sandbox Workspace/进程配额和 MCP 并发有企业隔离测试。
- [ ] 持久目录容量、冷启动重挂载、满额行为和文件交付大小分别建立预算；不沿用 4 MiB Tool Artifact 作为 Workspace 总大小上限。

## 文档更新

### P5-E08：权威文档收口

- [ ] `docs/README.md`：术语和决策改为自有 Tool Discovery/Template、客户 MCP 直接工具、离线 Sandbox 与持久 Workspace。
- [ ] `docs/00-decisions-and-invariants.md`：删除 Card Skill/Render Plan，全量 Tool 注入和 OpenSandbox 硬依赖不变量。
- [ ] `docs/01-product-and-architecture.md`：更新总体图、服务职责和依赖矩阵。
- [ ] `docs/04-agent-mcp-and-action-workflow.md`：区分自有三元工具/双投影/Preview 与客户 MCP 直接执行；保留自有执行后只读验证和总结。
- [ ] `docs/05-tool-templates-and-host-ui.md`：删除旧内容或重命名为 Tool Presentation Runtime。
- [ ] `docs/06-security-and-mvp-roadmap.md`：更新模板、Sandbox 和 MCP 威胁模型。
- [ ] `docs/08-model-and-sandbox-management.md`：增加持久 Workspace/临时计算、离线与联网隔离域、容量及明确删除语义。
- [ ] `docs/10-service-components-and-kubernetes-deployment.md`：增加 `agent-lite/agent-sandbox` 安装矩阵和 Readiness。
- [ ] `docs/12-technology-stack-and-code-structure.md`：更新包结构，删除 Card 领域和组件库。
- [ ] `docs/13-current-implementation-and-kubernetes-rollout.md`：更新实现盘点和测试证据。
- [ ] `docs/15-end-to-end-implementation-plan.md`：加入 PlanV5 E2E 与零残留门禁。
- [ ] `docs/16-agent-harness-and-context-management.md`：按小内核、原生 Tool Message 和跨 Run Context 重写。
- [ ] M4/M5 阶段文档标明被 PlanV5 哪些决策替换，避免继续作为运行时基线。
- [ ] `docs/planv2/`：替换对 Card Runtime 的预览/分析依赖，保留 Dashboard 独立业务域，不因删除 Card 删除 Dashboard。
- [ ] 主文档同步 Q1～Q16，包括企业连接/会话选择、预装环境、宿主确认、Remote-only 首期范围、容量拒绝和业务文件上传/产物下载。

### P5-E09：契约与运维文档

- [ ] OpenAPI/JSON Schema 记录元工具输入、Tool Result Envelope、Template Runtime 和稳定错误。
- [ ] 平台运维手册说明 OpenSandbox 未配置、故障、Profile/配额和 MCP 诊断。
- [ ] Tool 开发手册说明 Manifest、Projection、Template、风险和发布门禁。
- [ ] Template 开发手册说明 CSP、Design Tokens、Bridge、大小和可访问性。
- [ ] MCP 接入手册区分自有 Registry 与客户直接模型工具，说明企业管理员配置/授权、会话显式选择、撤权即时拦截以及读写直接执行和文字结果边界。
- [ ] Template 开发手册明确宿主拥有影响范围、确认/取消及审批执行状态，iframe Bridge 没有确认/取消能力。
- [ ] Skill 接入手册说明显式激活、Context Hash、自有三元工具与外接/离线能力的指导范围，不让 Skill 自行注册工具或模板。
- [ ] Workspace 手册说明完整目录最新状态、实例回收、明确删除、额度行为和大文件交付，不承诺逐次版本回滚或进程内存恢复。
- [ ] 用户手册说明真实附件上传与产物下载、容量不足时调整连接/模型，stdio/旧 HTTP+SSE 标明为未交付。
- [ ] 迁移记录明确旧 Card 数据和 API 不兼容且不提供转换。

## 最终删除检查

- [ ] 正式代码不存在 `card.render`。
- [ ] Agent 无业务 Tool 关键词选择器。
- [ ] 模型请求不包含业务 Tool 完整 Catalog。
- [ ] 企业门户不存在 Card 创建、内置卡片、自定义卡片和 Binding 页面。
- [ ] 数据库不存在 Card Catalog/Version/Instance/Presentation/Slot/Binding。
- [ ] Worker 宿主不执行 `bash/read/write/edit` 或 stdio MCP。
- [ ] OpenSandbox 未配置时 Agent 仍通过完整 `agent-lite` E2E。
- [ ] 主文档不再把旧 Card 架构描述为当前基线。
- [ ] 不再声明全部模型工具总数固定三/七个、所有 MCP 都走自有 Gateway、客户 MCP 返回模板、Workspace 闲置即删目录或所有写操作都必须 Preview。

## 完成标准

1. 单元、契约、数据库集成、前端 Playwright 和 Kubernetes E2E 全部通过。
2. `agent-lite` 与 `agent-sandbox` 两种模式均有真实证据和零残留清理记录。
3. 自有 Catalog 规模测试证明核心 Schema 不随自有工具数线性增长；客户 MCP 直接工具成本单独报告。
4. Template 与 Sandbox 安全逃逸测试全部 fail closed。
5. Card 领域已删除，PendingAction 安全闭环保持完整。
6. 主文档全部切换到 PlanV5 基线，不存在互相冲突的当前架构说明。
7. 客户 MCP 可作为模型工具直接调用并返回文字数据；离线四工具不能联网，持久目录在计算实例回收后完整保留。
8. 明确删除、满额、恢复与正常服务复原均有证据；企业连接管理/会话选择、预装依赖、容量拒绝和真实文件上传/下载验收完成。stdio/旧 HTTP+SSE 不作为本期交付。
