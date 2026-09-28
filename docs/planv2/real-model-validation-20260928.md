# GLM 真实模型验收（2026-09-28）

用户已要求使用实测可连通的真实 API 启动剩余验收，取代此前“本轮暂缓”的执行安排。本页记录本轮实际结果；没有结果的项目不提前关闭。

## 最新结论

PlanV2 首期主清单 **37/37 关闭**。最后的 p 轮真实 GLM 异常组 **4/4 通过**、部署 verify **20/20 通过**、退出码 **0**；临时资源已按归属和身份核对清理。完整模型范围采用下表的分轮证据，**不是某一轮最新镜像一次性 13/13，也不是任意问题的模型准确性保证**。此前失败轮次和真实发现的问题保留，未改写为通过。

| 首期模型场景 | 通过证据 |
| --- | --- |
| 目标询问、builder 创建、DSL 创建 | o 轮 3 项；服务端时间窗口、默认主机/集群类型、严格校验、宿主确认和发布内容一致 |
| 三信号文件分析 | i 轮 1 项；真实文件哈希/记录数、读取工具及完整 Provider 用量通过 |
| 查询变量及 Host/Cluster 关联建议 | h 轮 1 项；变量引用范围和最终绑定与预览一致 |
| 多仪表盘独立默认、按需选图、追问、最新发布版 | h 轮 4 项；参数/版本/范围及 Chat 指标、错误日志数与文件一致 |
| 无数据、查询中断、预算耗尽、真实满空间 | p 轮 4 项；真实交付、观测值、未知结论、查询/交付状态和最终预算逐项核对 |

这 13 个选定通过样本共 142 次模型调用、2,250,441 个 Provider Tokens；这是选定证据的统计，不是所有失败/预检轮次的总账单。p 轮自身 54 次调用、569,400 Tokens，全部用量及缓存统计完整。来源和逐项断言见 [分轮汇总](../../artifacts/planv2-real-20260928/model-acceptance-rollup.json)、[p 轮结果](../../artifacts/planv2-e2e/p2-glm-20260928-p/planv2-real-model.json)、[部署检查](../../artifacts/planv2-e2e/p2-glm-20260928-p/verify/verify.json)。

覆盖判定不采信模型自报“已分析”：无遥测文件时只能零观测、空数值和 unknown，失败原因、查询/交付状态及预算必须与服务端事实一致。预算优先核对 JSON，遗漏辅助字段时仅接受五维带明确标签的准确正文，错误 JSON 不能被正文覆盖。早期额外要求发布证明文件造成 o 轮三信号任务达到步骤上限；该失败保留，三信号通过证据使用 i 轮。后续已将该测试的额外证明简化为 Chat 摘要，但没有把这份新提示标成已单独实测通过。

最终清理见 [结果](../../artifacts/planv2-real-20260928/final-cleanup.json) 与 [私有集群回收](../../artifacts/planv2-real-20260928/isolated-cluster-cleanup.json)：临时 kind 节点和本轮 LB 辅助容器残留 0；默认上下文仍为 docker-desktop，readyz=ok，9 个原有 Agentx PVC UID/Bound 未变。原 Judex 命名空间/PVC 在并行环境变化中消失，未重建；本轮临时恢复的控制器已按 UID 回收，未把新的共享环境说成原样恢复。

本轮是本地 evaluation 功能验收：NetworkPolicy 强制拒绝、外部 Egress Gateway 和强隔离 Sandbox 的生产门禁仍未证明。本轮未重跑先前的 32 个正式页面场景，未将分轮结果描述为单次全量回归。以下各节保留过程与失败事实。

### m 轮独立集群复验进行中

l 轮在 doctor 阶段退出 2，未部署、未调用模型：并行 Judex 部署已将 OpenSandbox CRD 归属迁到新的命名空间，其 v0.2.1 控制器处于 ImagePullBackOff。未修改该项目配置。随后共享集群里的这组 CRD 被其他操作移除；本轮只回收自己创建的临时恢复命名空间，核对了 9 个归属对象，并使用记录的 Namespace UID/resourceVersion 删除，见 [临时恢复回收记录](../../artifacts/planv2-real-20260928/recovery-cleanup.json)。

为避免继续争用共享控制器，按 [kind 官方流程](https://kind.sigs.k8s.io/docs/user/quick-start/) 创建独立临时集群 `argus-planv2-final-20260928`（Kubernetes v1.36.1），仅使用私有 kubeconfig；系统默认上下文保持 docker-desktop。节点内存上限 8 GiB。m 轮执行 core、failures 共 8 项，结果尚未产生。完成后需按记录的 Docker 容器身份回收该集群；其他项目的集群资源不在清理范围。

m 轮在模型测试前因 kind 没有 LoadBalancer 地址退出 1，测试命名空间已清理。E2E 显式转发模式现使用真实 Service ClusterIP 供集群内访问、端口转发供宿主访问，不再错误等待云负载均衡；普通非转发路径仍要求已分配的 LB 地址，两个路径均有回归测试。最终部署 verify 的 connector-lb 检查保留，未跳过。

n 轮前置查询与文件交付成功，但首个离线 read 在沙箱冷启动时超时，后续 bash 及证明文件发布成功；严格 read 断言未通过，真实模型阶段未启动，退出码 1，测试命名空间已清理。上游日志显示 Create 等待工作负载就绪最多 60 秒，客户端普通 HTTP 超时却是 30 秒。现仅 Create 使用 90 秒请求窗口，普通请求仍为 30 秒，调用方取消仍优先且不自动重放创建。相关 OpenSandbox、Workspace、Sandbox、Agent 和 Harness 回归通过，o 轮重跑验证。

按 [官方 Cloud Provider KIND 说明](https://kind.sigs.k8s.io/docs/user/loadbalancer/) 补齐真实 LB 实现。最初本机缓存的 Docker Desktop 定制镜像连接了 desktop 集群，发现后立即停止并按记录身份移除，相关记录保留；改用官方 v0.10.0，并在集群发现入口限定 `argus-planv2-final-20260928`。测试 Service 获得真实地址 `172.31.0.8`，节点到其 TCP 53 已连通。n 轮将在此隔离环境继续，不修改共享集群的 CRD 或应用配置。

### k 轮复验进行中

用户继续要求完善后，本轮按已说明的最小临时范围恢复共享 OpenSandbox 控制器：仅创建此前清单的 5 个命名空间级对象，没有恢复 Judex 应用/PVC，也没有修改现有 CRD 或集群 RBAC。控制器已 Ready，k 轮独立测试部署完成，执行 core、failures 共 8 项。创建前记录集群资源，创建后记录每个资源 UID；结束时需先检查不存在其他沙箱依赖、命名空间内没有无关资源，再用 Namespace UID/resourceVersion 前置条件回收临时恢复。k 轮结果尚未产生，35/37 状态不提前改变。

k 轮最终 8 项执行、6 项通过，104 次模型调用、1,313,341 Tokens，Provider 用量完整。DSL 创建使用了虚构的 2025 年日期检索目录，反复缩小窗口后达到步骤上限；创建目录此前没有提供服务端时钟。现 catalog.resources 输出 v2 增加 server_time、timezone 和默认一小时的 suggested_catalog_range，创建 Skill Revision 4 要求使用这些事实，已通过真实 PostgreSQL 验证。

满空间已正确触发 WORKSPACE_QUOTA_EXCEEDED，模型正确区分查询 success / 交付 failed / 未分析，并在正文准确列出预算；但遗漏了额外 JSON budget_after，旧断言将“缺字段”与“余额错误”合并报错。验收提示现改为一个完整摘要模板；预算优先核对结构化字段，缺字段时仅接受五个维度均有明确标签且数值完全一致的自然语言表达，错误的结构化值不能被正文覆盖。回归拒绝维度互换、旧值和泛称“预算充足”，并保存每次期望预算用于追溯。k 轮失败记录不改写；l 轮继续复验 core、failures 8 项。

### i 轮进行中的创建默认值修正

i 轮三信号文件分析已正常完成，暂未发生 ModelCall 失败。DSL 创建生成了正确表达式，却仅设置 `applicable_resource_types=[host]`，漏掉实际有样本的集群，导致样本为 no_data。该配置的硬校验有效，不能靠硬门禁禁止提前配置；问题是 AI 擅自缩小了用户未限制的范围。创建 Skill Revision 3 现与正式编辑器 `components/dashboards/model.ts` 一致，默认包含 host 与 kubernetes_cluster，仅在明确要求或来源能力不支持时缩小。真实模型创建及确定性工具验收同时检查该默认值，待新镜像复验。

i 轮最终 8 项执行、6 项通过，111 次模型调用全部有完整 Provider 用量，合计 1,314,367 Tokens，没有再次中断。满空间任务已准确返回 WORKSPACE_QUOTA_EXCEEDED，模型也说明未读到日志、不能判断；但验收 JSON 的通用 status 写成任务 failed，而面板查询实际 success，严格层级断言未通过。现把测试摘要明确拆为 query_status、delivery_status、analysis_status，各自核对权威字段，避免含糊的 status 约定。i 轮退出码 1，本轮测试命名空间已清理；最终共享环境核对发生下述变化，不能标为全部原有资源保留通过。

### j 轮环境前置阻塞

计划以 core、failures 共 8 项复验。`p2-glm-20260928-j` 在 doctor 阶段退出 2，尚未部署、未发起模型调用：原共享 OpenSandbox 控制器所在 `judex-poc-d15` 命名空间消失，3 个 CRD 仍保留 `osb/judex-poc-d15` 的 Helm 归属。当前无法确认删除者；测试日志没有删除该命名空间的记录，清理代码仍校验 release 标签与 UID/resourceVersion。

此前 `cleanup-i.json` 是卸载尚未完全结束的中间快照（仍有一个测试命名空间），不是最终清理通过证明。最终只读核对见 [环境变化记录](../../artifacts/planv2-real-20260928/environment-change-after-i.json)：测试命名空间残留 0，9 个 Agentx PVC UID 不变，原 Judex PVC 随其命名空间缺失；不能宣称原有全部 10 个 PVC 仍在。

已准备 [临时控制器恢复清单](../../artifacts/planv2-real-20260928/opensandbox-temporary-recovery.json)，仅命名空间、ServiceAccount、leader-election Role/RoleBinding 和原版本控制器 Deployment，共 5 个对象；不恢复应用/PVC，不改变现有 CRD/ClusterRole/ClusterRoleBinding。该清单尚未执行，等待用户确认外部命名空间的临时恢复范围。主清单保持 35/37，AI-03/AI-04 不能提前关闭。

### g 轮扩展与修正

`p2-glm-20260928-g`：基础四项、多仪表盘默认值、明确条件与按需选图、追问继承、混合无数据、扫描预算耗尽及真实 Workspace 满空间场景通过。128 次真实模型调用，Provider 合计 1,823,946 Tokens；该数字属于含失败验收项的整轮，不算全部通过的成绩。清理及原有资源核对通过。

- 变量草稿已正确生成变量与 Host/Cluster 建议，但断言把 `value` 操作中不生效的 `group_by` 当成聚合，未进入确认。已修正语义断言，回归仍拒绝真正的 `sum` 聚合及变量错误影响第二张图。
- 第二张仪表盘重发布的测试请求复用了第一张的确认幂等键，被正确返回 `IDEMPOTENCY_CONFLICT`。已改为按对象生成键；最新版本模型场景当轮未执行。
- 查询中断真实发生，任务/Panel 是 `partial`、Target 是 `error/QUERY_UNAVAILABLE`；断言遗漏了 Target 状态，已修正。
- 同一中断场景发现模型引用查询前余额，声称“预算充足”，但失败取数已经耗尽预留额度。查询工具输出 v2 增加实时预算快照；分析 Skill Revision 4 要求失败后重新读取各维余额并区分失败原因。新增严格 Chat 预算数值核对，不能仅凭剩余调用次数声称预算充足。真实 PostgreSQL 回归已验证任务身份不变时 `query.get` 能反映变动后的预算。

### h 轮结果与并发修正

`p2-glm-20260928-h` 通过变量/Host/Cluster 关联生成与确认。三信号分析中一次 ModelCall 被标记为 `MODEL_COMPATIBILITY_FAILED`，自动重试后业务完成，但失败调用缺少 Provider 用量，不能算严格通过。PostgreSQL 在 `05:16:31.124 UTC` 对 `NextConversationSequence` 报序列化冲突，与该调用结束时间 `05:16:31.125677 UTC` 一致。

h 轮最终执行 13 项、通过 11 项，退出码 1，清理及原有资源核对通过。变量/关联、四轮条件场景、无数据、查询失败及预算耗尽均通过；查询失败后的最新预算数字与数据库一致。160 次模型调用，已报告的 Provider 用量合计 2,581,480 Tokens；一次中断缺少用量，因此这不是完整账单总数。

已用真实 PostgreSQL 并发锁测试复现：模型流式增量及最终回复同时遇到后台查询事件时都会失败。修正仅将这两种受任务租约保护的事件事务改为 READ COMMITTED，依赖原子序号更新串行化；没有放宽任务 fencing，也不重放应用回调或模型请求。修正前两分支三轮均复现，修正后三轮通过，原租约心跳/过期接管/回调不重放回归继续通过。h 轮运行中的镜像不包含该后续修正，修正后的实际模型复验另行记录。

满空间场景观察到前台 bash 与后台交付的租约竞争，后台先以 WORKSPACE_BUSY 耗尽重试，没有到达严格要求的容量错误。后台导入现仅在取得访问租约前最多等待两分钟，每次重验来源，取消仍生效；不重放字节写入或重试容量/权限错误。真实 PostgreSQL/RPC 的导入来源、哈希、恢复、容量及撤权回归和等待/取消测试通过。测试结论新增独立 analysis_status，保留清单里的查询 status，避免把“查询 success、文件未交付、尚未分析”误判成虚构结论。修正后的 core、failures 共 8 项将用新镜像专项复验；h 轮其余五项结果单独保留，不把 h 整轮改成通过。

### f 轮基础通过（历史）

`p2-glm-20260928-f` 四项真实模型基础场景 **4/4 通过**，最终 Harness 退出码 **0**。部署使用真实 Collector、Kafka、ClickHouse、PostgreSQL、Worker、对象存储和 Workspace PVC；模型为真实 `glm-5.3`，协议为 OpenAI-compatible `chat_completions`。

| 场景 | 已核验事实 |
| --- | --- |
| 未选择仪表盘 | 模型列出授权清单，引导通过选择器/@ 引用选择；具体仪表盘读取、条件解析和查询调用为 0 |
| builder 创建 | 真实 Catalog、单一 builder 配置、草稿校验、发布预览、宿主确认及已发布版本一致；模型没有直接提交 |
| DSL 创建 | 真实 Catalog、原始 PromQL 语义、草稿校验及确认发布一致；对象生命周期版本与发布版本分开解释 |
| 三信号分析 | 已发布查询物化为真实文件；模型实际执行 sha256sum 和 Python，解析并发布证明文件；服务端逐文件校验 Hash 与记录数，metrics=1 条序列、logs=3 行、traces=100 项 |

这四项业务任务共 **46 次模型调用**，Provider 实报输入 **498,510**、输出 **18,986**、合计 **517,496 Tokens**，其中缓存输入 **288,960**；每项用量及缓存统计均完整。模型兼容探针另计，历史失败轮次不混入这组成功统计；配置单价不作为供应商实际账单。

模型分析说明了冻结版本、时间范围、全部三张适用图的检查结果，以及 Trace 定义只返回 100 项、无法据此推断其余数据的限制。自动断言覆盖文件事实和协议，本轮不对自由文本中的每一句时间/数值描述作通用语义认证。

最终环境核对：临时 Namespace 残留 **0**，Kubernetes readyz 为 ok，原有 **10 个 PVC** UID/Bound 未变，**4 个外部 OpenSandbox 对象**未变，**40 个运行 Pod 全部 Ready**；Server 同一 Pod 零重启，保留 256 MiB/192 MiB 内存限制。没有暂停需要恢复的正式服务。

证据：[模型结果](../../artifacts/planv2-e2e/p2-glm-20260928-f/planv2-real-model.json)、[专项范围结果](../../artifacts/planv2-e2e/p2-glm-20260928-f/result.json)、[清理及保留核对](../../artifacts/planv2-real-20260928/cleanup-f.json)。`scope=real_model_with_telemetry_and_files`、`browser_executed=false`，本轮没有重新运行此前全部 32 个页面场景；前端类型、153 项企业门户测试、API Client 回归、i18n/样式/受影响 ESLint、相关 Go、真实 PostgreSQL 和完整契约生成检查通过。

部署仍是本地 evaluation profile：NetworkPolicy 强制隔离未验证、没有外部 Egress Gateway、使用共享普通容器 Sandbox；功能通过不等于生产安全验收。

## f 轮当时未关闭的范围（历史）

- 查询变量的真实模型生成，以及 Host/Cluster 关联建议的专项质量验证。
- 连续追问、多仪表盘独立默认条件、最新版本切换和具体问题选图策略的真实模型组合验证。
- 无数据、查询失败、预算/容量不足等分支的模型覆盖说明，以及更广泛的结论准确性。

f 轮关闭 P2V-AI-02 与 P2V-E2E-02；当时主清单为 **33 项关闭、4 项仍有部分验收待补**，不能将四个基础场景通过写成原计划全部模型质量验收完成。当前状态以本页顶部为准。

## 预检与范围

- 模型为 `glm-5.3`，协议为 `chat_completions`。本轮用户提供的是 Coding Plan Key，使用官方 Coding Plan 端点做开发验收；标准 API 端点返回余额不足。该配置不作为 Argus 正式产品部署或订阅额度适用性的证明。
- Argus `ProviderTester` 的基础、流式、工具调用、结构化输出四项预检通过，尚不能替代自然语言业务任务验收。
- 本机及临时 Pod 的 DNS 将模型域名解析为 `198.18.0.80`，现有公网地址校验按预期拒绝。通过公共 DNS over HTTPS 核验真实公网地址后，仅在本轮拥有的 Server/Worker Pod 中增加临时 `hostAliases`；TLS 主机名验证及原有公网/SSRF 策略保持启用。地址是当次环境调整，不能当作长期固定配置。
- 凭据仅由进程环境变量提供。配置和报告不存放 API Key。订阅测试配置的 Token 单价为 0，不把它解释成标准 API 免费或实际账单费用。

## 可重复入口

`argus-dev e2e run --suite planv2 --planv2-real-model-only --real-model-config <配置路径>` 运行真实模型任务及其临时部署、真实遥测和文件前置验证；不重跑浏览器、容量及故障套件。结果显式记录 `scope=real_model_with_telemetry_and_files` 和 `browser_executed=false`，不能据此宣称当轮全套页面通过。省略该范围开关仍执行完整 PlanV2。

扩展 Harness 默认运行 core（4）、authoring（1）、conditions（4）、failures（4），共 13 个场景。专项复验可显式加 `--planv2-model-groups authoring,conditions`；只允许在 real-model-only 范围中使用，未知或重复组拒绝。专项报告保留 `groups` 和实际 planned_tasks，部署结果标为 `selected_real_model_groups_with_telemetry_and_files`，不得把子集通过当作全部通过。模型异常组与此前非模型故障套件是不同验收范围。

模型配置可选 `endpoint_ip` 是 E2E 部署提示，只接受满足现有模型公网地址策略的 IP，不进入产品模型配置；只修改已核验归属的临时 Namespace，清理时随其删除。

首轮自然语言场景为 builder 创建、DSL 创建及三信号文件分析。前两项核对真实 Catalog、草稿、宿主确认及已发布查询；分析核对实际查询任务、离线工具、文件 Hash 和记录数。泛问目标选择、追问、多仪表盘和结论质量等剩余验收不因这三个场景通过而自动关闭。

## 执行状态

`p2-glm-20260928-a` 完成镜像构建与临时部署，在 M2 的 MFA 注册请求等待响应时超时，模型业务任务尚未开始，退出码 1。本轮 Server 零重启，无证据表明该失败来自 GLM。临时 Namespace 清理完成，原有 PVC 与共享 OpenSandbox 对象核对通过，40 个运行 Pod 全部 Ready。现使用同一配置进行 b 轮复验，并由启动器直接实时落盘 stdout/stderr，避免 PowerShell 重定向缓冲影响排障。

b 轮在 `setup/status` 返回 502，网页代理日志确认连接后端 Service 被拒绝，而更新后的 Server Pod 已 Ready。测试准备阶段在入口就绪检查之后再次滚动 Server，且旧入口检查仅覆盖静态页面，不能证明 API 路由已收敛。现将临时 DNS 配置提前至测试夹具安装前，并在入口准备中加入真实 `setup/status` 路由等待；相关回归拒绝 502 及仅返回静态 `ready` 的响应，待 c 轮实际复验。

c 轮已通过初始化、MFA、资源接入、Agent/Gateway、Collector 生命周期与重启、真实三信号查询、自监控及 Workspace 文件前置验证，并启动真实 GLM。首个 builder 任务进行了 9 次实际推理，Provider 实报输入 50,680、输出 1,920、合计 52,600 Tokens，用量完整；随后以 `CONTEXT_COMPACTION_FAILED` 结束，没有生成可发布草稿，余下两项未执行。本轮不计业务通过。

该轮测试配置将上下文限制为 65,536。Agent 的保守线级字节预算使硬门限为 45,260 字节；读取工具契约后触发硬门限，而单轮内尚无可压缩的完整旧轮次。实测 draft.create 输入 Schema 为 14,759 字节，未发现契约无限展开。[GLM-5.3 官方说明](https://docs.bigmodel.cn/cn/guide/models/text/glm-5.3.md) 标明 1M 上下文及 128K 最大输出；d 轮改用 1,000,000 上下文并继续限制单次输出 8,192，保持现有安全预算与完整轮次压缩规则。c 轮临时环境已清理，原有资源核对通过。d 轮结果见下段。

d 轮已越过上下文限制，实际进行了 24 次推理（输入 357,192、输出 12,184、合计 369,376 Tokens；用量完整）。模型保存了草稿，但使用了未在原工具 Schema 中说明合法值的版本/时间类型，并在 range step 的必填组合上反复尝试；最终以 `RUN_STEP_LIMIT_REACHED` 失败，未发布。发现的产品缺口是契约提示不足、草稿操作未返回校验问题，以及样本 API 将 `valid=false` 的详细报告丢为普通错误。

修复在公共 OpenAPI 中声明版本/时间/模式/语言的合法值，补足步长和来源继承说明；UI 类型同步更新。草稿工具返回同一领域的结构/编译报告，新增仅创建模式可见的 `draft.validate`，直接复用人工 Runtime.SampleDraft。样本 API 对配置错误返回完整报告及 `not_executed`，发布硬门禁继续拒绝。真实 PostgreSQL 回归验证错误草稿可保存、校验有具体反馈、旧版本拒绝及不能发布；创建 Skill 更新至 Revision 2。e 轮还新增未选仪表盘先列清单并询问的自然语言场景，总计四项；后续实际复验见最新结论，不以代码修复本身代替模型验收。

e 轮的目标询问与 builder 创建通过；DSL 也完成了正确发布且 Run succeeded，但存在一条 `MODEL_RESPONSE_INTERRUPTED`，用量不完整，严格验收仍标失败，原 Harness 因此未执行文件分析。该轮合计实报 31 次模型调用、316,896 Tokens；这不是包含中断调用的完整账单统计。模型证据保留在 `artifacts/planv2-real-20260928/model-e-observed.json`。

对应 PostgreSQL 日志在 `02:27:48.398 UTC` 记录 `LockRuntimeTaskLease` 的序列化冲突，Worker 并未重启。租约心跳更新被错误当成失去所有权并取消模型流。修复仅对尚未进入应用回调的锁获取冲突做最多三次事务重试；实际 owner/fence/expiry 不匹配仍取消，不重放应用回调，不填造缺失 Token。真实数据库三轮回归同时覆盖心跳竞争、过期接管拒绝及应用回调不重放。

另补齐确认后上下文的 `result_resource_*` 字段，区分原输入目标、新建对象及独立版本计数，避免将对象版本 2 与发布版本/上下文版本 1 当作矛盾；创建模式读取已选定义不再错误要求分析参数。询问提示明确通过 Chat 选择器或 @ 引用选择，编号/名称文本不能替代结构化选择。f 轮增加该交互断言，并让独立场景的验收失败汇总后统一报错，不再阻止后续文件分析；每项仍要求真实行为与完整 Provider 用量。
