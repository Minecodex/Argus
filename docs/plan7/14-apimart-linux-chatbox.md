# APIMart 与 Docker Linux 的 Chatbox 实测

日期：2026-10-17～18。用户要求使用 APIMart `glm-5.3-flash` 完成实际 AIOps 流程，并使用本地 Docker Linux。此前 R01～R03 的失败保留，独立 GLM-5.3 发布回归不替代此模型验证。

**最新结果：2026-10-18 通过 Docker Linux 的四类实际账号业务验收。** 采用同一冻结源码、部署及模型 revision 4，失败阶段独立重跑；最终通过案例共 **47/47 次模型调用成功**，提供方用量和结算完整，实际浏览器 **2/2**，服务修复、连续确认、断线恢复和私有并发上下文均通过。历史失败保持，不等同于所有轮次零错误，也不是一次新的 57 项跨平台 Flash 验收。[最终摘要](./evidence/plan7-apimart-linux-20261018-summary.json)。

## 本次验证范围

新增可重复的 `--suite plan7 --plan7-phase linux-chatbox`：正式接入 Docker SSH/systemd 主机及 Collector，使用实际遥测查询和 Workspace 分析；随后由真实浏览器在 Chatbox 中发送排查消息、查看并确认 connect、切换计划、提交修改意见、查看第二版命令、一次批准、刷新页面，再检查真实服务配置、备份、进程及健康。模型始终为配置选定的 `glm-5.3-flash`。

同轮继续验证连续两次取证确认、Workspace 中间分析、解释器丢失后的新确认恢复、未知修改不重放，以及仅模型可见的并发事实。使用生产后端和真实模型，不将 Replay 控制场景视为模型通过。

这是 Linux 完整业务链路验证，不替代跨 Linux／Kubernetes／Windows 的 57 项完整矩阵；输出明确带 `scope=linux_chatbox_real_model`、`complete=false`。此前冻结版本的完整发布验收仍按原记录保留。

## 诊断补充

上轮 HTTP 200 后的流错误尚无确定根因。本次给流读取和事件回调加可解包的来源标记，日志仅输出固定分类，区分响应读取、后端事件处理、HTTP/2 错误类型及数据库冲突；不输出错误原文、凭据或模型内容，不改变超时、工具执行或审批规则。

## 进度

`plan7-apimart-20261017-r05` 完成真实取数、Workspace 分析、现场取证及第一版修复计划，共 22 次提供方调用成功。浏览器脚本在填写修改意见后仍用原“确认执行”按钮定位整张卡片，按钮改名导致定位器失效；该轮被中止，不能计作完整通过。定位器已改为绑定不可变标题和版本。此问题属于本轮新增的测试驱动，不把它描述成已确认的业务功能故障。

中止后使用原清理实现重建本轮上下文，按所有权清理 Docker／namespace，并核对原 20 个工作负载及 9 个 PVC 恢复；[清理执行记录](../../build/plan7-r05-owned-cleanup.log)退出 0。原始模型和浏览器诊断保留。

用户随后明确要求在现有 `kakj` 账号下测试并保留界面历史。本轮改为显式启用 `plan7portal` 的真实账号验收，直接使用已部署的生产 API；不执行安装、租户重置或账号权限修改，也不修改 MFA、密码、原 Workspace Profile 或已有文件。新建且可核对归属的 Docker Linux／堡垒机、监控数据和会话留供用户在现有 Chatbox 查看。保留历轮文件所需的本地存储配额扩容单列如下。

真实账号的浏览器测试复用正常登录产生的会话 Cookie，并在服务端重新核对用户名；认证规则保持不变。Cookie 仅通过子进程环境传递，未写入报告；关闭该场景的浏览器 trace，避免将真实账号凭据录入测试轨迹。正式门户使用正常 HTTPS 校验。

为接入 APIMart，本地已有专属模型 DNS 仅新增 `api.apimart.ai` 的 AliDNS DNS-over-TLS 动态解析；仍保留 URL、SNI、CA 和公共地址校验，不固定服务器 IP，不修改共享 CoreDNS。其他域名及既有 GLM 路由不变。

接续各轮复用已按正式接入流程创建且核对归属的 Docker 资源。每次重新开始使用新的运行标识和幂等键，保留旧会话；不重放已经成功的修复。会话历史保留在 `kakj` 名下，未将隔离测试记录导入或伪造成该账号的新会话。

## 正式账号中发现的部署问题

实际账号首次流程在读取 Workspace 文件时失败：上次本地发布脚本把已经包含 `@sha256:...` 的冻结引用写入 `sandbox_images.image_ref`，同时保留独立 `digest` 字段。运行时、Broker 和 Admission 按既定 `image_ref + @ + digest` 约定组装，得到两次摘要，Kubernetes 返回 `InvalidImageName`。因此前次部署的健康核验并未证明正式账号的分析容器能启动。

已核对同一个受管镜像 ID、原引用、摘要及版本，通过条件更新只将 `image_ref` 改回不含摘要的目录引用；`digest`、Profile、PVC 和原文件保留。发布脚本同步按相同约定拆分并拒绝摘要不匹配或多次 `@`，避免再次写入无效记录。没有修改镜像内容、认证策略或审批语义。

旧 Run 冻结了修复前版本，因此后续工具返回 `TOOL_VERSION_UNAVAILABLE`；未静默替换其上下文。该轮到期后按正常 API 明确取消本次测试 Run，失败历史保留，重新发起新会话验证修复后环境。首次真实监控分析记录：[修复前会话](https://argus.dev/?c=01a149f5-af16-745b-a1e1-72845c83c09c)。

## R02：业务流程通过，严格调用门禁未通过

修正登记后的 Workspace 实际 Pod 为 Running，用户容器引用只含一次摘要，且模型的 read／bash 均执行成功。R02 浏览器用例 **1/1 通过**：真实取证确认、计划切换、第一版预览、意见修订、第二版一次批准、刷新页面及结果展示完成。第二版 **7 个步骤均 succeeded**，第一版保持 superseded 且未派发。

Docker 独立核验得到 `workers=2`、`workers.bak=0`、systemd active/running、MainPID=669、health=healthy；不是仅凭模型总结认定修复成功。会话：[指标分析](https://argus.dev/?c=01a14a03-418d-7915-b2ab-d8e11042528d)、[排查与修复](https://argus.dev/?c=01a14a08-93d9-79f4-b89c-958973a11853)。

R02 的总验收仍为 **failed**：修复 Run 有一次响应头前的传输失败，约 10 秒后结束，未取得提供方用量。后续重试成功，Run 最终 succeeded，但严格的全部调用成功／用量门禁拒绝，因此尚未进入其余三个模型阶段。该调用和缺失用量保留，不改成成功或估计为已报告用量。

同域名 DNS 日志在相近时段记录 AliDNS DoT 上游超时／连接拒绝；现有日志缺少逐请求关联，不能据此断言该调用的唯一底层原因。随后在自有隔离 namespace 验证两组 DoT／DoH 公共端点的正常证书和解析响应，为 APIMart 域名单独切换到已验证的双 Cloudflare DoT 地址并缩短连接缓存，其他域名路由不变。策略只增加解析器的两个 `/32` TCP 853 目标；TLS 和 SSRF 规则不放宽。

通过正式 Argus 模型测试 API 连续 **5/5** 兼容性检查后，启动 R03 严格重跑。此前失败记录及当时的总门禁状态保持。[DNS 探测](../../build/plan7-dnsdiag-result.json)、[连续生产链路检查](../../build/plan7-kakj-dns-stability.json)、[实际 Workspace](../../build/plan7-evidence/plan7-kakj-20261017-r02/workspace-start-proof.json)、[物理服务核验](../../build/plan7-evidence/plan7-kakj-20261017-r02/portal-physical-health-proof.json)、[浏览器结果](../../build/plan7-evidence/plan7-kakj-20261017-r02/playwright-plan7-linux-chatbox/report.json)。

## R03 与 HTTPS DNS 修复

R03 再次完成现场取证和第二版计划的远端执行，Docker 独立 health 核验仍为 healthy；结束分析期间，第二次上下文压缩的提供方连接在响应头前失败。先前同一 Run 的压缩调用曾成功，不能将本次失败描述为 JSON 摘要或分组算法错误。

本次时间证据可直接关联：失败压缩调用于 **14:15:54.239 UTC** 结束；专用 DNS 于 **14:15:54.392 UTC** 记录 `api.apimart.ai` A 查询到 `1.0.0.1:853` 超时。此前还有另一条同目标超时。因此更换 DoT 上游、短期五次探测成功仍不足以说明本机 853 路径稳定；R03 总验收保留 failed。

本机专用模型 DNS 已仅为 APIMart 增加 DoH sidecar，外部访问限定两个公共 DNS 地址的 TCP 443，上游并行；不修改 API 服务 URL、不固定 API 服务器 IP、不放宽 TLS／SSRF 或业务权限。原 CoreDNS、其他域名和共享集群 DNS 保留。

组件来源为 [AdGuard dnsproxy](https://github.com/AdguardTeam/dnsproxy) v0.86.0，发布前核对厂商源码标签和版本，并以实际本地清单摘要固定镜像。厂商二进制携带低端口 file capability，派生镜像仅去除该属性，二进制 SHA256 与原版相同；运行保留非 root、drop ALL、禁止提权和只读根文件系统，不通过扩大权限解决启动问题。实际配置见[本机快照](./evidence/plan7-model-dns-doh-20261017.json)、[运行及镜像证据](../../build/plan7-model-doh-deployment.json)。

DoH 路径通过 **30/30** 不走缓存的查询验证。曾对旧终态 Run 提交手动压缩请求，但未产生新压缩调用，因此没有把它计入压缩验证。后续通过新运行中的真实长上下文及恢复用例验证，不修改旧失败记录。

## R04 与输出预算兼容性修复

R04 的实际浏览器流程再次 **1/1 通过**，第二版计划实际完成，修复 Run 最终 succeeded；严格模型调用门禁仍拒绝通过。其中一次推理已收到 HTTP 200，约 311 秒后读取返回 `unexpected_eof`，没有提供方用量；当时 DoH 日志无对应解析错误。该失败与 R03 的响应头前 DoT 超时分开记录，不能宣称切换 DNS 解决所有流错误。

随后独立的有界兼容性探测确认：向同一 `glm-5.3-flash` 请求 128 tokens 时，`max_completion_tokens` 两次分别返回 1422、2744 个输出 tokens，均以 stop 结束，限制被忽略；`max_tokens` 返回 128 tokens，结束原因 length，限制生效。探测不执行工具、不携带用户业务数据。它证明了预算协议问题，但不证明上游流终止的唯一原因。[首次探测](../../build/plan7-apimart-budget-probe.log)、[复核](../../build/plan7-apimart-budget-confirm.log)。

平台现于模型创建／更新时检测实际生效的输出参数，并将结果冻结在模型 revision；普通推理和压缩从同一 Run 绑定的 revision 读取，不在执行中切换。新探测检查提供方真实输出用量和 length 结束，仅在明确不支持参数或预算未生效时尝试另一参数；认证、限流及网络错误不触发该回退。运行时若提供方报告超出请求预算，则在后续工具派发前拒绝该响应。旧 revision 保持原事实，重新保存产生新版本。

同轮修复在全部地址均通过公共地址校验后尝试备用 TCP 连接；所有尝试共用原有 30 秒连接预算，仅发生于 HTTP 请求发送之前，不重发已发送的请求或失败的 SSE 流。混合私网答案仍立即拒绝。流错误分类保留可解包的底层原因，日志只输出固定分类。

该源码已通过全量 Go、真实 PostgreSQL／HTTP 的压缩恢复、Replay、契约生成检查，以及前端类型、单测、样式、i18n 和 real 镜像构建。2026-10-17 23:51 本地平台升级至 Helm revision **29**，只替换 Backend 和 Web，保留其他镜像、既有 values、数据及专用 DoH 路径。源码冻结为 `455f42daae1377f15b83ad29d3a330885557754c67483557e5a580cb52b43f3e`，构建前后相同。[工程检查](../../build/plan7-budget-engineering-result.json)、[部署](../../build/local-release-plan7-budget-20261017/deployment.json)。

通过正常账号 API 更新已有 APIMart 模型，产生 revision **2**，保持原模型 ID、端点、提供方模型及加密凭据，输出上限为 8192，验证并绑定 `max_tokens`。[模型版本回执](../../build/plan7-kakj-budget-model-update.json)。

## R05：原有存储容量耗尽；R06 接续

R05 的 13 次实际模型调用均成功，包括此前长响应场景；但首次取数文件交付和 Workspace 读取明确返回 `WORKSPACE_QUOTA_EXCEEDED`，结论检查以 `undelivered data file` 拒绝整轮通过。此处是本地容量前件，不能把该轮的模型调用成功当作业务闭环成功。

实际账号原有 3 个业务 Workspace 加上为审查保留的 7 个测试 Workspace，各预留 2 GiB，已达到企业 **20 GiB** 存储限制。并发计算配额尚有空位，因此已有的计算容量等待并不能证明存储可用。本地 D 盘可用约 60.6 GiB，Docker 文件系统可用约 537 GiB；为保留已有聊天、原始文件和失败证据，仅对该企业存储行核对原上限和预留值后将上限调整到 **40 GiB**，既有预留保持 20 GiB。没有提高并发计算配额、改变身份／资源权限或审批规则。[条件更新回执](../../build/plan7-kakj-storage-capacity.json)。

R05 历史保留，R06 在新会话中从真实取数重新开始。R06 尚未全部完成前不标记通过。本次部署健康 **20/20**、原 13 个 Deployment 的身份／副本以及 9 个 Backend／Web 实际容器镜像核验通过，独立回归用的临时 PostgreSQL 已核对所有权后删除。[运行核验](../../build/local-release-plan7-budget-20261017/runtime-proof.json)、[健康](../../build/plan7-kakj-budget-verify/verify.json)、[夹具清理](../../build/plan7-budget-regression-cleanup.json)。

## R06 实际账号验收

已通过真实遥测／Workspace 核验与浏览器排查修复：浏览器 **1/1**，第一版 superseded 且未派发，第二版两个步骤串行成功，一次批准；刷新和切换模式后已批准任务继续执行。独立 Docker 核验得到 workers=2、备份=0、systemd active/running，以及实际进程、最新 heartbeat 和 healthy。模型分析、命令、确认、反馈及回传文件均为现有账号正常创建的真实历史。

已通过连续两次取证：分别确认两个 connect，第一批回传后先在 Workspace 解析，再申请第二批，实际结论对应独立生成的唯一内容；源文件中的恶意指令未触发远程写入。R06 的总结果仍为 failed：恢复阶段一次 5225 ms 的响应头前传输失败没有提供方用量，随后虽重试成功，严格门禁仍拒绝；DoH 日志没有对应错误，现有固定分类不足以证明具体网络根因。

恢复阶段真实压缩成功（1840 输出 tokens，约 22 秒），真实解释器中断后原计划保持 result_unknown，后续步骤未执行。然而下一次推理用满 8192 tokens 并以 length 结束，正文为空，没有提出恢复请求；旧后端却将 Run 标为 succeeded/length。因本轮先被用量门禁拒绝，不能称其恢复业务通过，也不能只归因为网络。失败调用、空结果和原远端未知事实保留。

- [指标分析会话](https://argus.dev/?c=01a14a9d-ab8c-7096-87ab-98e2318f2439)
- [现场排查与修订计划会话](https://argus.dev/?c=01a14aa2-2131-7bf8-bc6f-95e0c0d13f7f)
- [连续确认会话](https://argus.dev/?c=01a14aab-1501-746f-b545-1809151f2ebb)
- [界面实际完成截图](../../build/plan7-evidence/plan7-kakj-20261018-r06/portal-visible-completed-plan.jpg)、[浏览器报告](../../build/plan7-evidence/plan7-kakj-20261018-r06/playwright-plan7-linux-chatbox/report.json)、[连续取证证据](../../build/plan7-evidence/plan7-kakj-20261018-r06/plan7-model-evidence-sequence.json)。

## 截断状态与模型推理配置修复

普通推理现在将 length、incomplete、content_filter 等非正常完成原因记录为 **MODEL_OUTPUT_INCOMPLETE**，保留部分正文、提供方用量及已完成操作，任务和步骤均为 failed。不会自动扩充预算、继续远程修改或将空正文视为恢复成功。真实 PostgreSQL＋HTTP 回归覆盖空截断、部分截断、其他不完整结束及正常完成；同时修正压缩额度预留与实际输出上限不一致的问题，二者共用同一上限值。

模型创建／更新 API 增加可选 `reasoning_effort`（provider_default／low／high／max），仅支持已实现的 Chat Completions 路径；省略时保留已有设置，显式 provider_default 在新版本清除覆盖。兼容性测试使用同一设置，成功后冻结到模型 revision，推理和压缩均读取该版本；未知值和不支持的协议拒绝。现有表单不新增设置入口，其更新也不会清除该覆盖。没有按供应商名称硬编码运行参数，也没有修改审批、工具或资源权限。

[NVIDIA 的 GLM-5.3-Flash 文档](https://docs.nvidia.com/nim/vision-language-models/latest/get-started/advanced/get-started-glm-5-3-flash.html#control-reasoning-effort)说明推理默认 max，可选择 low／high；这不能替代 APIMart 网关验证。本轮使用同一 APIMart 端点和模型进行独立、无真实主机操作的虚构恢复探测，明确发送 low 后取得有效 inspect 工具调用和真实用量；随后通过正常 Argus API 的完整兼容性测试，保存为模型 revision **3**、max_tokens、8192、low。早期探测曾因开发中的请求构造覆盖该字段而无效，已单列 invalidated，未作为 low 生效证据。[有效探测](../../build/plan7-apimart-reasoning-probe.log)、[模型版本](../../build/plan7-kakj-reasoning-model-update.json)。

修复源码冻结为 `222b11ab1e86211589963c5d703dd552735414e9c1614fb19485d5257ebbd30e`，构建前后相同。全量 Go、数据库／HTTP、Replay、契约、前端类型／单测／样式／i18n 和 real 构建通过。[工程门禁](../../build/plan7-completion-engineering-result.json)、[镜像构建](../../build/plan7-completion-image-builds.json)。

本地升级首次 Helm revision 30 因上一轮已完成的静态名称迁移 Job 模板不可变而失败；既有 `argusctl` 安装器已有本地镜像升级前重建迁移 Job 的处理。核对该 Job 的所属 release、完成状态和 UID 后按同一生命周期重建，保留其 UID 删除前件，最终 **revision 31 deployed**、新 Job succeeded、schema 16。其他 values 保留，没有因该错误清空数据库。[部署及迁移](../../build/local-release-plan7-completion-20261018/deployment.json)。

R07 在同一份新部署和模型 revision 3 上重新执行全部四阶段；没有复用旧阶段来标记新轮次通过。为保留原始聊天文件并留出后续工作空间，现有企业存储上限从 40 GiB 调整至 **48 GiB**，调整前 28 GiB 预留保持；计算并发与操作权限不变。[存储容量回执](../../build/plan7-kakj-storage-capacity-r07.json)。

R07 的 12 次模型调用全部成功，但指标结论仍被原始文件对照拒绝：CPU 文件实际 2 条序列、122 个采样点，模型只汇总了其中一条，填写 records=1、samples=61，最大值也不符。没有归一化错误答案或放宽统计口径。该轮保留 failed，说明 low 的简易工具探测成功不能证明其适用于完整 AIOps 推理。

随后通过正常 API 保存模型 revision **4**：仍为同一个 APIMart `glm-5.3-flash`，设置 high，输出上限恢复至最初登记的 16384，并继续使用已经验证的 max_tokens 参数。调整是为恢复推理提供合理预算，length 仍会明确失败；全部数据对照、调用用量、审批、恢复及 10 分钟任务门槛不变。新源码、镜像和部署保持同一冻结版本，R08 从真实取数重新执行四阶段。[模型版本回执](../../build/plan7-kakj-high-model-update.json)。

R08 尚未全部完成前不标记通过。[本次健康 20/20](../../build/plan7-kakj-completion-verify/verify.json)、[原资源与业务 ID 保留](../../build/local-release-plan7-completion-20261018/preservation.json)、[实际镜像与工作负载](../../build/local-release-plan7-completion-20261018/runtime-proof.json)、[临时 PostgreSQL 清理](../../build/plan7-completion-regression-cleanup.json)。

## R08：核心流程通过，确认 HTTP/2 流失败

R08 的 high 配置通过真实指标结论核验，核心浏览器 **1/1、4 分钟**通过，修订计划的三个步骤串行完成；独立 Docker 核验 workers=2、备份=0、MainPID=67125、active/running、healthy。[真实修复会话](https://argus.dev/?c=01a14acd-e490-7808-b47f-c552337f4c97)、[原始物理核验](../../build/plan7-evidence/plan7-kakj-20261018-r08/portal-physical-health-proof.json)。

总结果仍为 failed：连续取证阶段的一次模型调用已收到 HTTP 200，约 139 秒后失败。新增日志明确标记 **response_read / http2_stream_error**，不是事件持久化回调；没有提供方用量，虽然后续重试成功，门禁仍拒绝通过。该日志缩小了故障范围，但不能证明提供方 HTTP/2 实现或网络中间层的唯一根因。

模型网关现使用专用 `ModelClient()` 强制 HTTP/1.1 SSE，保留公共地址复验、DNS、CA、主机名校验和原有超时。通用 `Client()` 仍沿用原协议协商，客户 MCP 不采用这项策略。真实 TLS 测试在支持 HTTP/2 的测试服务端验证模型客户端实际协商 HTTP/1.1，并另行验证通用客户端没有被更改；普通推理、压缩及模型兼容性探测均调用同一模型客户端。

首个 HTTP/1.1 候选发现通用客户端也被影响，已在部署前标记 superseded，未发布。修订后的冻结源码为 `9f30ffd07df1dd7a0cfaa960f4b0aec4100d5ccacc1a98cc8f348a4a569592a7`。此前并行运行契约检查与编译／前端测试，契约生成暂时重写文件，导致缺失生成模块；受影响日志保留，改为 Go、数据库／HTTP、契约、前端验证顺序执行，完成后再捕获稳定 Docker 构建上下文。受影响的通过记录不替代修订版本的最终检查。

R09 将保持模型 revision 4（high、16384、max_tokens），在新部署从头执行四阶段。为保留旧会话文件并留出后续工作空间，当前企业存储上限从 48 GiB 调整至 **64 GiB**，原 36 GiB 预留不变；计算配额和权限不变。[容量回执](../../build/plan7-kakj-storage-capacity-r09.json)、[修订源码清单](../../build/local-release-plan7-http1-20261018-r02/source-before-build.json)。R09 仍待全部验收完成，未预先标记通过。

### HTTP/1.1 预检与 ALPN 修正

修订候选 r02 曾部署到 Helm revision 32，健康检查 20/20，但正式模型 API 预检失败，因此 **R09 未启动**。重复预检均在约 1 秒、响应头前失败；不能仅凭 transport_error 将它归因于 APIMart。随后不带密钥的真实公共端点诊断定位本地错误：模型客户端仍协商 h2，却用 HTTP/1.1 读取 HTTP/2 帧，产生 malformed HTTP response。Go 1.25.8 的 Transport.Clone 保留了原 TLS 配置中的 h2 ALPN，单设 Protocols 不足；原 TLS 测试重建了整份 TLS 配置，掩盖该问题。

已在模型专用客户端同时设置 HTTP/1.1 协议、关闭 ForceAttemptHTTP2、清空备用协议映射，并将 ALPN 限为 http/1.1；保留现有 CA、主机名及公共地址校验。TLS 测试只追加受信测试根，不替换生产 ALPN 配置。修正后同一公共端点的通用客户端协商 HTTP/2、模型客户端协商 HTTP/1.1，均收到预期的未认证 401；带用户密钥的虚构工具调用探测成功，真实用量完整，没有执行工具或传送业务数据。

- [修正前公共诊断](../../build/plan7-http1-public-diagnostic-before-alpn-fix.jsonl)、[修正后公共诊断](../../build/plan7-http1-public-diagnostic-after-alpn-fix.jsonl)、[受控认证探测](../../build/plan7-apimart-http1-auth-probe.log)。
- r02 保留 deployed_failed_model_preflight，不能计为已验收部署。最终修正源码冻结为 `7daac58ef2731e8e084bdaf6031ff92476e6e796ab8e9068eea47dfa981b2018`，门禁、构建、正式 API 预检与 R09 将依次在该版本执行；此前各失败状态保持。

## 最终同版本分阶段验收

R09 遥测、R10 核心修复仍出现独立于 HTTP/2 的响应头前传输失败；重试继续，但缺失用量使该轮严格门禁拒绝。没有再次修改模型、代码、超时、统计口径或断言，而是按原函数独立重跑失败阶段。测试入口的运行期参数仅选择阶段、关联已通过的真实会话；原领域断言、审批范围、真实效果、模型用量及超时门槛保持。[阶段入口回执](../../build/plan7-kakj-phase-overlay-receipt.json)、[全部阶段尝试](../../build/plan7-kakj-case-ledger.json)。

| 案例 | 运行标识 | 实际结果 |
| --- | --- | --- |
| 取数→Workspace→排查→修订→批准→串行修复→验证 | r13-core | 指标统计匹配原始文件；只读取证未改变配置；旧计划未派发；第二版两步骤执行成功；刷新与模式切换不取消已批准任务；实际 workers=2、备份=0、PID=111774、active/running、healthy |
| 两次独立取证与中间分析 | r11-sequence | 两个不同批准；两批之间在 Workspace 解析；结论对应实际唯一内容；夹带指令没有触发远程写入 |
| 压缩、解释器丢失与新确认恢复 | r12-recovery | 实际压缩调用；旧写入回执维持未知；新确认和新解释器代次恢复；实际 effects 严格为 first／second／third 各一行，没有重放 |
| 私有并发上下文与停止 | r14-activity | 不同聊天的实际命令重叠执行，变量不共享；模型调用读取经授权过滤的私有观察，聊天事件／DTO／界面无原始并发数据；长命令经明确 stop 后 cancelled |

四类案例使用同一源码 **7daac58…**、Backend／real Web 候选 **plan7-http1-20261018-r03**、平台 Helm revision **33**、模型 **glm-5.3-flash revision 4 / high / 16384 / max_tokens**。构建前后及验收后 **2652 份源码清单一致**；未把旧通过阶段替换为新版本的结果。最终核验覆盖 6 个实际会话、8 个 Run：正常任务 succeeded，明确停止的长命令对应 Run 为 cancelled/remote_execution_stopped，属于该场景的预期结果。

最终通过案例的 **47 次调用全部成功**，其中 46 次推理、1 次压缩，输入 **623865**、输出 **29679** tokens；每次均为提供方报告并完成内部结算，不包含注册探测、失败轮次或外部账单金额。浏览器 2/2，无跳过或 flaky。工程门禁为 **2169 项 Go 检查、73 个包、零失败**，另有真实数据库／HTTP 的 M4 回归、契约、前端类型／单测／样式／i18n 和 real 构建。健康 **20/20**，原 27 个工作负载、9 个 PVC、25 个业务记录 ID 保留。

恢复案例中记录了 TOOL_INPUT_INVALID 和两个 workflow 工具失败，并出现 quota_unavailable 能力快照；模型如实说明未在 Workspace 核验相应文件，未虚构本地读取。该案例的恢复结论以实际远端 effects、旧未知回执及压缩后调用证据为准；主流程和连续取证案例则实际完成了 Workspace 文件分析。这是本轮记录的能力限制，不将所有工具调用描述为零失败。HTTP/1.1 避开已定位的 HTTP/2 流路径，但不承诺消除第三方所有传输中断；历史未知用量保持未知。

### 直接查看实际会话

- [指标分析](https://argus.dev/?c=01a14b0b-89ff-7d36-99b6-a55d0f237184)
- [排查、修改意见和已完成计划](https://argus.dev/?c=01a14b0e-0d71-7760-88c9-796f4a5d35ed)
- [两次独立确认](https://argus.dev/?c=01a14b02-815a-762e-881c-d95863f0bca3)
- [未知结果后的恢复](https://argus.dev/?c=01a14b02-e71a-7b9d-b8c5-89701a932a91)
- [独立会话与私有上下文](https://argus.dev/?c=01a14b14-bbd7-7890-97ab-f38434a2ebba)

### 最终证据

- [47 次调用与四案例核验](../../build/plan7-evidence/plan7-kakj-20261018-r14-activity/acceptance-audit.json)、[实际调用记录](../../build/plan7-evidence/plan7-kakj-20261018-r14-activity/accepted-model-calls.json)。
- [核心浏览器](../../build/plan7-evidence/plan7-kakj-20261018-r13-core/playwright-plan7-linux-chatbox/report.json)、[历史与隐私浏览器](../../build/plan7-evidence/plan7-kakj-20261018-r14-activity/playwright-plan7-model/report.json)、[实际完成截图](../../build/plan7-evidence/plan7-kakj-20261018-r13-core/portal-visible-completed-plan.jpg)。
- [物理服务核验](../../build/plan7-evidence/plan7-kakj-20261018-r13-core/portal-physical-health-proof.json)、[恢复效果](../../build/plan7-evidence/plan7-kakj-20261018-r12-recovery/plan7-model-recovery.json)。
- [最终源码](../../build/local-release-plan7-http1-20261018-r03/source-after-acceptance.json)、[部署](../../build/local-release-plan7-http1-20261018-r03/deployment.json)、[运行镜像](../../build/local-release-plan7-http1-20261018-r03/runtime-proof.json)、[资源与原业务记录](../../build/local-release-plan7-http1-20261018-r03/preservation.json)、[健康检查](../../build/plan7-kakj-http1-r03-verify/verify.json)。
- [全量工程检查](../../build/plan7-http1-r03-engineering-result.json)、[具名计数](../../build/plan7-http1-r03-engineering-counts.json)、[源码行数／冻结核验](../../build/plan7-http1-r03-final-source-audit.json)、[临时数据库清理](../../build/plan7-http1-r03-regression-cleanup.json)。

实际账号历史、注册的加密模型凭据和专用 Docker 主机保留，供用户审查与继续使用。Windows Server、重新执行的 Windows／Kubernetes Flash 全矩阵及本地 Evaluation 加固边界仍按原未验证范围记录。

交付前对3717份源码、文档及本轮日志／证据进行提供方密钥与账号密码明文匹配扫描，零匹配。两份本次创建的DPAPI临时副本已逐个核对并以明确文件路径删除；未删除目录、应用内加密模型凭据或修改账号认证设置。[扫描](../../build/plan7-kakj-secret-scan.json)、[临时副本清理](../../build/plan7-kakj-credential-cleanup.json)。
