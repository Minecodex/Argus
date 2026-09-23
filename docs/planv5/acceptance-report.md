# PlanV5 实施与验收报告

状态：**尚未全量完成。R01～R17 已有修复与验收证据；R17 已通过工程、数据库和完整 P5，真实中文模型评测仍未运行。** 最新证据见 [R17 修复记录](./fixes-r17-2026-09-23.md)。

本轮工程、空库、数据库回归及完整 P5 b 通过；18 项关键证据 Hash 已核验。a 轮新 Schema 嵌入包未复制到镜像的失败已修复并保留记录。以下较早复核/验收按其实际日期保留，不当作本轮重新执行。

最新 P5 为 `p5-r17-20260923-b`：10 项真实浏览器、20 项安装器及完整业务/故障场景通过，实际退出码 0。七分类九组业务检索和完整 Describe 投影验证通过，业务变更为零；R16 异常用量、MCP、文件/隔离及恢复场景继续通过。18 项关键证据 Hash 匹配，本轮临时集群资源零残留，原有 9 个 PVC、27 个工作负载及就绪状态保持。独立数据库及匿名卷已清理。本轮未重跑独立 M4/P4/M7/M10。


此前 R12/R13 的 P5 为 `p5-r12-r13-20260922-d`：10 项真实浏览器（3 项身份、7 项业务）、20 项安装器检查和完整 P5 通过，实际退出码 0。新增导入来源撤权、复制/发布字节比对、文件与交付下载拒绝，以及缺 usage 的推理/压缩结算和评测拒绝均通过。14 项证据 Hash 已核验；归属资源零残留，原有 9 个 PVC、27 个工作负载/就绪状态保留。工程与数据库门禁通过，独立数据库和匿名卷已清理。本轮未重跑 M4/P4/M7/M10，以下记录按各轮实际日期保留。

此前 R09～R11 两轮为 `p5-r09-r11-20260922-a` 与 `p5-r09-r11-m4-20260922-a`：P5 的 9 项真实浏览器、M4 的 5 项真实浏览器及各 20 项安装器检查通过，两轮退出码均为 0。模型接收端验证 Workspace 删除/重建状态和引用失效，M4 验证异常动作结束后会话可继续及 Run 归属隔离。归属资源零残留，原有 9 个 PVC、27 个工作负载/就绪状态保留。以下较早证据按其日期保留。

此前 R06～R08 的 P5 c：9 项真实浏览器、20 项安装器与 Workspace 故障矩阵通过；对应 M4 a：5 项真实浏览器、20 项安装器与压缩/审批/执行回归通过。两轮退出码均为 0，归属资源零残留；原有 9 个 PVC、27 个工作负载/就绪数保持不变。该轮全量 Go、契约、SQLC、前端类型/单元/lint 和 mock/real 构建通过，未重跑 P4/M7/M10。历史证据按原日期保留。

P5 g 覆盖单次取消、未知结果恢复、Worker 强制退出、冷/热接管、空闲回收与文件交付；M10 g 覆盖新增 Collector 模型 Preview、实际执行及查询业务。两轮退出码均为 0，临时资源已清理，原有 9 个 PVC、27 个工作负载及就绪数保留。本次复核 19 项证据 Hash 全部匹配。逐项修复与失败历史见 [补齐记录](./fixes-2026-09-21.md)。

## 2026-09-22 收口证据

- P5：`artifacts/p5-e2e/p5-fixes-20260922-g`，8 项真实浏览器、20 项安装器和 Workspace 故障矩阵通过，退出码 0。
- M10：`artifacts/p5-e2e/p5-native-m10-20260922-g`，M3/M4/p5-native/M7/M10-query、23 项真实浏览器和 20 项安装器通过，退出码 0。
- 两轮清理、原有 PVC/工作负载/就绪数和证据 Hash 分别记录在 `cleanup-verified.json` 与 `evidence-index.json`；本地专用测试资源清理记录为 `artifacts/p5-e2e/final-temporary-cleanup-20260922.json`。
- 最新工程检查与 1193 个源码文件大小、旧 Card 零引用证据见补齐记录末节。以下既有门禁按原日期保留，不替换历史记录。

## 任务对应

| 任务 | 已实现内容 | 主要证据与剩余工作 |
| --- | --- | --- |
| Task 01 Agent 与 Context | 原生 Chat/Responses 工具消息、Provider 别名、Run 快照、跨 Run 事件、实际请求 Hash、增量压缩、任务 fence、派发状态、只读验证、24 次模型迭代上限 | R06 Preview 恢复、R07 容量边界和 R08 压缩失败终态已修复；最新 P5 c/M4 a 与数据库回归通过，已结束 Run 的手动/后台压缩继续有效；R09 异常收尾、R10 归属校验和 R11 Workspace 上下文已由最新数据库、P5 a/M4 a 关闭；真实模型评测仍待运行。 |
| Task 02 自有 Tool Gateway | 七分类、三个元工具、版本化目录、权限感知缓存、独立 Invoke 校验、隐藏 Commit、固定实现版本 | 1/500/1000 目录与恢复测试通过；正式 Preview 已扩展到 27 项，Schema/模板/隐藏 Commit 和 Bastion 数据库调用通过；新增 M7 Collector 网关集群回归在 M10 g 轮通过；R17 已补齐 52 个正式工具的业务元数据、确定性检索、版本与缓存约束，并通过完整 P5 b。 |
| Task 03 Tool 模板 | 同包 Builder/模板、私有源码与模型投影分离、独立 Template Origin、opaque iframe、nonce/序号、三种 Bridge 操作、宿主单次确认 | 80 项模板矩阵和 2 项 Runtime 隔离测试通过；27 项正式 Preview 已接入同包 Builder；新增业务网关路径与平台默认配置在 M10 g 轮通过。 |
| Task 04 MCP 与 Workspace | 企业连接/凭据/授权、会话选择、目录刷新、Remote MCP JSON/SSE、未知结果停止；专用 PVC、IO 角色、admission、离线四工具、上传与不可变交付 | 既有 P5 文件/隔离/安装矩阵通过；R02/R03/R04 已实现并有定向证据；b 轮已验证热复用、崩溃后恢复、自然空闲与模型文件删除。最后使用时间、并发结算、Worker 强制退出与旧租约收尾已有验证；R11 已在实际模型接收端验证删除状态、旧引用失效和新 Workspace 身份。 |
| Task 05 Card 删除 | 后端/前端旧包、API/DTO、数据库对象、生成入口、部署、导航、Mock 和旧运行时删除；Dashboard 保留 | 运行源代码与部署扫描无旧 Card 领域引用；主文档已切换，历史验收记录明确标记。切换登记见 `api/contracts/cutovers.yaml`。 |
| Task 06 验收与文档 | `argus-dev p5`、M7/M10 依赖调整、独立测试 Ingress、全局 Lease、原副本记录、按归属清理、开发/运维/使用手册、真实模型评测入口 | P5 g、M10 g 既有断言及资源恢复校验通过；R06～R08 已有正式回归；R09～R11 已纳入正式数据库及集群回归；真实中文模型评测仍缺实际配置，保留为未完成。 |

R12/R13 已闭环：Task 01/06 的用量记录和评测使用来源完整性，Task 04 在写入前登记整个 Workspace 的来源并统一鉴权；数据库、公开用量 UI 与最终 P5 d 实测通过。真实模型评测仍缺配置，不能据此将全部 Task 标为完成。

R14/R15 已闭环：Task 04 在传输解析入口阻止已知认证值返回；Task 01 记录缓存输入来源和实际摘要 ID/Hash；工程、数据库与 P5 b 验证通过。Task 06 的真实中文模型评测仍未执行。

## 历史通用检查（最新门禁见本文开头及 R17 修复记录）

| 门禁 | 当前证据 |
| --- | --- |
| 新数据库完整迁移链 | 00001/00002、重复/并发迁移及数据库约束已有通过记录。2026-09-20 空库完成迁移和 791 条 SQL 预编译；2026-09-21 再次使用 PostgreSQL 18.6 空库完成迁移，并校验新增真实模型统计 SQL。临时数据库均已删除，未执行历史数据转换。 |
| OpenAPI/protobuf/生成一致性 | 2026-09-21 完整重试通过，生成内容与源契约一致，证据 `build/p5-contracts-check-20260921-retry.log`。首次 Windows Node 退出断言异常保留为失败记录。 |
| SQLC | 2026-09-21 生成及声明级拆分通过，证据 `build/p5-sqlc-20260921.log`。 |
| Go | 最新安装模式、源码内置 MinIO Client、真实模型评测入口修改后的 `go test ./...` 通过，证据 `build/p5-go-all-20260921-final.log`。未配置环境的集成测试仍跳过；真实数据库和 Kubernetes 证据单独列明。 |
| 前端 | 2026-09-21 类型、lint、单元及 i18n、mock/real 构建通过，real 包无 Mock 标记，证据 `build/p5-web-{types,tests,lint}-20260921.log`、`build/p5-{mock,real}-build-20260921.log`。 |
| 桌面浏览器 | 原流程与 P5 Mock/隔离测试 37 项通过，移动端 1 项按项目范围跳过；最新模板/Runtime 82 项通过，证据 `build/p5-template-boundaries-20260920.log`。 |
| 平台离线 Profile | 新接口模型、只保存支持字段、禁用 Workspace 网络访问通过类型/lint/单元与契约检查；创建、刷新、停用在中英文和明暗主题的 4 项浏览器测试通过。 |
| 安装器 | `p5-20260921-final-d` 的 20 项检查全部通过，包含新镜像内置 Client 的对象读写、OpenSandbox 生命周期、PKI 和业务入口；保留 CA/SNI/Origin 校验。 |
| 现有 Kubernetes P5 套件 | `p5-20260921-final-d` passed、退出码 0。覆盖无 OpenSandbox 安装 → 文件 IO/自有工具/Remote MCP → 同 release 启用计算 → MCP/离线分析/不可变下载/硬额度/撤销旧写入者；M2 与 P5 共 6 项真实浏览器通过。两个 Workspace 均为 deleted，按归属清理通过。覆盖边界见逐项复核。 |
| P4/M7/M10 受影响回归 | P4 `p5-p4-20260920-e` passed、退出码 0，归属扫描无残留。M10 `p5-m10-20260920-i` 的业务阶段及安装器通过，包括 12 项 M7 真实浏览器；日志包含卸载完成标记，但原进程退出码及集群重建前零残留无法补证。 |
| 中文真实模型 | 六任务自然语言评测入口、配置/凭据校验、CSV 结果断言和 PostgreSQL 统计查询已实现并通过相关测试。实际运行尚未执行，需指定已有模型配置，不能以 Replay 或空结果替代真实评测通过。 |
| 源码边界 | 最新 1144 个 Go/TypeScript/样式源文件均不超过 2000 行，差异检查通过；运行/部署源码无已删除的 Card 领域引用。PlanV2 展示依赖和 M4/M5 历史入口已同步。 |

## 2026-09-21 P5 历史证据

运行目录为 `artifacts/p5-e2e/p5-20260921-final-d`，完整日志为 `build/p5-kubernetes-20260921-final-d.log`：

- `result.json`、`verify/verify.json`：业务和 20 项安装器检查通过；实际进程退出码 0 记录在 `cleanup-verified.json`。
- `agent-lite-install-config.yaml`、`agent-sandbox-install-config.yaml`：同一 release 的能力切换记录；测试卷使用 256 MiB，产品默认容量仍为 2 GiB。
- `state-workspaces.json`：两个测试 Workspace 均明确删除；工作负载回收没有代替文件删除。
- `build-image-digests.json`、`runtime-image-digests.json`：8 个构建镜像及实际运行镜像标识，覆盖 Workspace/egress 与固定存储组件。
- `p5-statistics.json`：确定性 Replay 共 17 个 Run，15 个成功、2 个故障注入按预期以结果未知停止；输入 1280、输出 320 Token，首个有效结果均值 3054 ms。这不是实际模型任务成功率或成本评测。
- `cleanup-verified.json`、`evidence-index.json`：本轮归属扫描、原有资源比对及关键证据 SHA-256。

## 自有 Gateway 本地延迟基线

Linux 隔离容器中各操作执行 100 次，未包含模型、数据库或网络耗时。该数据是本地函数调用基线，不能替代业务任务的端到端有效结果延迟。证据：`build/p5-native-latency-linux.log`。

| 目录规模 | 无缓存 Search P95 | 缓存 Search P95 | Describe P95 | Invoke P95 |
| --- | --- | --- | --- | --- |
| 500 | 489 μs | 87 μs | 4 μs | 9 μs |
| 1000 | 944 μs | 5 μs | 9 μs | 10 μs |

两个规模的核心工具数仍为 3，初始 Schema 均为 1296 字节；外接 MCP 工具 Schema 单独进入容量检查。

模板初始化到可见内容的预算为 15 秒，与宿主初始化上限一致。2026-09-20 本机开发服务器、双 Chromium worker 的 80 个样本为 P50 73.5 ms、P95 196.2 ms、最大 910.5 ms，零超限；包含 780034 字节、13 万数据点的样本。该观测不含业务 API 获取详情的时间，也不代表生产网络 SLA。逐项记录和汇总见 `artifacts/p5-e2e/template-boundaries-20260920/timing-summary.json`。

## 默认边界

- 自有工具经 `tool.search/describe/invoke` 使用；外接 MCP 直接注册到模型，不渲染客户 UI 扩展。
- Workspace 默认 2 GiB，企业预留池 20 GiB，单文件默认 100 MiB；配置权属于平台。
- RawFile LocalPV 固定 v0.15.1，ext4、thick、nodiscard、RWO、WaitForFirstConsumer；不自动扩容，不宣称跨节点 HA。
- 离线镜像预装 Python 3.12、Node 24、bash 和锁定版本的分析库；无运行时网络安装依赖。
- 仅明确删除 Workspace 或永久删除会话才清理工作文件和交付；不保证逐次修改回滚或进程内存保留。
- Skill 仅提供版本化上下文接口，stdio、旧 HTTP+SSE、编辑器和 Marketplace 不在首期范围。

## 资源与清理

测试使用独立 Namespace、独立 Ingress 和全局 E2E Lease。没有正式 Argus 部署被暂停；其他项目的 Namespace、Deployment 和 PVC 不作为本次清理对象。中断轮次按配置及 UID/resourceVersion 清理，所有额外恢复操作记录在实施日志。

2026-09-21 的专用 PostgreSQL 测试容器、匿名卷、独立 MinIO Client 验证镜像和本次临时镜像 tar 已删除。a/b/c 失败轮次及 d 成功轮次均完成归属清理。d 轮复核本轮 Namespace、PVC/PV、RawFile Driver、Registry、Lease 无残留；原有 9 个 PVC 的 UID、绑定卷、状态，以及 23 个 Deployment/StatefulSet 的 UID 和副本数保持一致，没有暂停正式服务。安装器按共享基础依赖保留 cert-manager/trust-manager；这些共享组件单独记录，不计为本轮专用资源。

2026-09-19 已核实 Docker 恢复，不再需要重启；P4 c 轮的 Registry、临时 Namespace 和 Lease 已按归属清理，详见实施日志。真实中文模型评测还需指定已有模型配置位置。

使用说明见 [使用手册](./user-guide.md)，开发与运维分别见 [开发手册](./developer-guide.md)、[运维手册](./operations-guide.md)。

2026-09-23 后续复核曾发现 R16 跨更新用量不一致，见 [发现记录](./review-2026-09-23-r16.md)。随后已补齐最终校验、诊断来源和无效统计隔离，并通过工程、数据库和完整 P5，见 [修复验收](./fixes-r16-2026-09-23.md)。真实模型评测仍未执行。

2026-09-23 完成度复核曾发现 R17，详见 [原复核与断言](./review-2026-09-23-r17.md)。随后已补齐正式业务元数据和检索，并通过工程、数据库与完整 P5 b，见 [修复验收](./fixes-r17-2026-09-23.md)。真实模型评测仍未执行。
