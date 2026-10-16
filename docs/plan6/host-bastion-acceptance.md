# Docker Linux Host 与 Bastion 真实闭环追加验收

本次为用户新增的独立验收范围，既有r02/revision9核心门禁保留为历史基线。2026-10-12 h14整轮追加验收通过、原生进程退出0、清理与正式恢复完成；最终报告为`artifacts/plan6-host-loop/20261012-14/final-host-loop-acceptance.json`。历史失败保留于下文。

最终五业务阶段、GLM当前/恢复/仪表盘离线分析三场景、真实浏览器3/3、四协议首次/恢复8批16投递、20环境检查通过。源码前后指纹d16badd6e90b3083cd45f8dc77650847c9150aba4b650199a2d3d9e4857f6fca/2210文件一致，19个原工作负载及Namespace UID、原副本与就绪保持；owned Docker/K8s资源全部清理。h14结束时正式保留revision9；后续同一源码l01完整核心all退出0，并于2026-10-12保留values发布至revision10，正式镜像、数据与门户全部核验通过，见[最新发布证据](../../artifacts/plan6-local-release-20261012/final-latest-release.json)。

链路：Docker普通Linux主机 → Docker Linux堡垒机 → 隔离Argus正常构建 → Kafka/ClickHouse/Query → 仪表盘与告警 → 实际HTTP/SMTP接收器 → 人工处理/降噪/恢复/历史 → GLM-5.3。

- 两个容器运行systemd/OpenSSH，私有cgroup与CPU/内存限额；普通主机只在无出站网络，堡垒机连接两张owned网络。检查普通主机不能直接访问平台。
- Bastion命令注册，经其SSH安装Host Connector，实际固定TLS中继保留Host/SNI/CA。Collector通过同一Preview/Commit顺序启用Edge Gateway和Leaf；不得用SQL把其他Collector重新绑定成该拓扑。
- 使用host-basic真实CPU/内存scraper、真实Host/Source ID与仪表盘查询。CPU负载仅在owned普通主机内创建，记录负载前/中/后指标；禁止注入argus-telemetry-e2e或把合成指标冒充系统监控。
- 核对触发/通知分组、四种协议实际受理与SMTP收信、确认/认领/评论、屏蔽及解除、抑制及解除、恢复防抖、历史和授权导出。
- GLM普通中文查询当前/恢复告警，结构化选中对应主机仪表盘，实际取数并读取Workspace文件。核对实际查询范围、文件哈希/记录数、最终结论、真实用量与只读配置不变。
- 真实浏览器检查Host/Collector、两图数据、历史/通知和GLM会话结果；必要前件缺失判失败。
- 成功/失败均恢复Argus原副本，保留Judex及其他数据。Docker容器/网络按记录ID和标签核对，临时K8s按Namespace/UID清理。

Docker容器共享Docker Desktop的WSL Linux内核；本轮证明真实Linux软件、SSH/Connector/Collector/指标/告警/模型链路，不作为独立VM或物理机指标隔离验收。通知使用真实协议接收器，外部群与真实邮箱仍单列。

可单独运行：`go run ./cmd/argus-dev e2e run --suite plan6 --plan6-phase host --plan6-host-fixtures <owned-Docker-manifest> --real-model-config docs/plan6/e2e/glm-model.json --shared-controller-config <trusted-controller-config> --kube-context docker-desktop`。该阶段明确记录追加覆盖，不代替Plan6全部完整suite。

当前接点：两个owned Docker Linux容器的systemd/SSH/private-cgroup前件通过；追加runner与断言实现中，完整真实闭环尚待运行。证据目录：`artifacts/plan6-host-loop/20261011-01`。

## 当前执行接点

2026-10-11：追加Go runner测试、Enterprise类型和完整前端lint通过。`plan6-host-20261011-h01 --suite plan6 --plan6-phase host`已启动，原生Go PID39692，源码冻结。Docker所有权manifest：`artifacts/plan6-host-loop/20261011-01/docker-ownership.json`；两个容器均systemd/SSH/private-cgroup正常，Host只有internal网络，Bastion双网络。主日志`host-loop-h01.log`，进程结果`host-loop-h01-process.json`，完整输出`artifacts/plan6-e2e/plan6-host-20261011-h01`。尚未取得追加完整闭环成功结果；下一步等同轮阶段结果并修复实际暴露的问题。

## h01失败与h02接续

h01在Docker Bastion首次HTTPS引导返回TLS连接错误（exit35），尚未进入真实系统指标和业务闭环。Docker主机/堡垒机及owned网络、临时Namespace已清理，进程退出1。Docker Desktop Kubernetes节点本身在kind网络，直接使用Service IP未建立实际容器入口。追加仅透传TLS字节的owned socat桥接，通过临时NodePort暴露443/9443/4317/4318；保持URL/Host/SNI/CA和Collector身份，普通Host仅internal网络，共享kind网络只连接本次桥接容器、不删除或修改已有节点。

当前h02：`plan6-host-20261012-h02`，原生Go PID26372，追加源码冻结；Dockermanifest `artifacts/plan6-host-loop/20261011-02/docker-ownership.json`，日志`host-loop-h02.log`、进程结果`host-loop-h02-process.json`，输出`artifacts/plan6-e2e/plan6-host-20261012-h02`。桥接改动runner测试通过，完整真实闭环尚待本轮结果。

## h02地址可达性与h03接续

h02 TLS桥接成功，Bastion注册并上报ready；检测到双网卡Bastion广告出站地址，internal-only Host缺到该地址的客户路由，追加场景因此失败而未计通过。修复仅对已核对的Bastion容器实际IP增加/32路由，下一跳是同一Bastion的internal地址，不增加默认路由、不开放整个出站网络。源代码没有修改Bastion发现/协议/鉴权逻辑。h02临时资源清理及原副本恢复结束，退出1。

### h03失败与h04接续

h03注册成功，但目标回连预检报 `HOST_ONBOARDING_CALLBACK_TIMEOUT`。独立网络复现确认：同一监听端口通过Bastion内网IP可连接，经/32路由访问出站IP超时，Docker internal隔离在内核路由外仍有效。因此不扩大网络连通范围，为多网卡Bastion新增首次启动的 `ARGUS_BASTION_RELAY_ADVERTISE_ADDRESS` 配置；冻结地址与端口代数持久化，后续重启不能环境变量漂移，非法地址fail closed。相关设计同步到主机接入文档。生产组件与runner完整包测试通过；原h03临时资源清理、正式副本恢复完成，退出1。

当前h04：`plan6-host-20261012-h04`，manifest `artifacts/plan6-host-loop/20261012-04/docker-ownership.json`，日志`host-loop-h04.log`，进程结果`host-loop-h04-process.json`。Bastion使用真实内网IP的服务配置，不改数据库广告记录或放宽TLS。清理前保存Docker服务journal。所有h01/h02/h03失败保留，追加完整闭环仍待同轮结果。网络复现：`artifacts/plan6-host-loop/20261012-04/docker-routing-proof.json`。

### h04回调通过与h05接续

h04已通过真实Bastion SSH及 `callback_verified=true/callback_control_path=bastion_relay`，Connector journal确认实际广告internal IP。随后新runner的Host Preview将环境错误设为 `e2e`，正式输入校验返回400；改为契约中的 `development`。未放宽API校验。h04清理及原副本恢复结束，退出1。

其他接续断言：SMTP仍使用既有真实协议接收器的 `ops@example.test` envelope；两实例分别人工确认/认领/评论；完整检查屏蔽/解除、单一warning抑制/解除和恢复事件，处理人及资源/来源范围必须保留。GLM最终JSON和授权工具观测逐项核对，伪造数值/身份/状态的回归必须拒绝。

当前h05：`plan6-host-20261012-h05`，manifest `artifacts/plan6-host-loop/20261012-05/docker-ownership.json`；日志`host-loop-h05.log`、进程结果`host-loop-h05-process.json`，完整输出`artifacts/plan6-e2e/plan6-host-20261012-h05`。完整追加闭环仍未通过，下一步检查同轮实际采样/通知/模型/浏览器和最终清理结果。

### h05真实采集通过与h06接续

h05实际Host/Collector及Edge Gateway均converged，真实bastion_gateway/bastion_tunnel路由通过；hostmetrics实际CPU4.9991382467%、内存45.5257912964%，partial=false且Host/Source标签准确。两个容器在注册前各自初始化独立Linux machine-id并重启验证。仪表盘新构造器遗漏空数组，正式契约拒绝 `detail_query_targets=null`；修正LocalFilters/DetailQueryTargets/Drilldowns/Thresholds/ParameterBindings为显式数组，保持HTTP契约严格。h05清理恢复结束，退出1。

追加host阶段改为其独立必要前件M2/M4及正常production构建切换，再执行真实Docker/Collector/模型/浏览器链路。原all阶段M3/M4/P5-native/M7/自监控及全部门禁完全保留；host-prerequisite-coverage.json明确列出未执行的核心基线，不累计为核心all通过。每轮使用新资源/目录并冻结源码。

当前h06：`plan6-host-20261012-h06`，manifest `artifacts/plan6-host-loop/20261012-06/docker-ownership.json`；日志`host-loop-h06.log`，进程结果`host-loop-h06-process.json`，完整输出`artifacts/plan6-e2e/plan6-host-20261012-h06`。仍要求五项业务阶段、GLM精确结论和真实浏览器/清理恢复全部通过。

### h06矩阵断言与h07接续

h06实际接入再次通过，仪表盘草稿、Preview/Confirm和发布均成功；新增断言停在矩阵检查。源码确认Runtime直接返回Prometheus matrix数组，旧断言只认result/series包装而不认metric/values。由于当时成功HTTP响应未保存，不能把h06矩阵数据宣称通过。修正为对真实Prometheus类型的JSON检查，要求两图完整success、Host/Source标签与每个有限百分比点正确，拒绝partial/空矩阵/外部Host；失败前也保存执行响应。使用真实promql.Matrix序列化的回归和正式OpenAPI契约回归通过。触发和恢复中间状态分别保存独立证据。

当前h07：`plan6-host-20261012-h07`，manifest `artifacts/plan6-host-loop/20261012-07/docker-ownership.json`；日志`host-loop-h07.log`，进程结果`host-loop-h07-process.json`，输出`artifacts/plan6-e2e/plan6-host-20261012-h07`。h06清理恢复结束，退出1；追加完整闭环仍未通过。

### h07三项业务阶段通过与h08模型重验

h07同轮通过真实Docker接入、hostmetrics/双图矩阵、告警持续pending→firing、四协议4批8投递对账、两实例人工确认/认领/评论、屏蔽/解除及warning抑制/解除。只读观察保存95条中间状态，未注入指标。真实GLM已查询告警，精确结论断言失败；h07未保存回复/完整投影，不能认定模型正确或历史根因已证明。检查发现断言只接受列表page，未覆盖真实alert.event.get的instance详情及其更近观测。补详情投影并维持Host/Policy/状态及数值严格核对，补实际详情值与伪造值回归，所有回复/工具/样本在断言前保存。h07清理恢复完成，退出1；恢复历史、仪表盘真实模型和浏览器仍待同轮通过。

当前h08：`plan6-host-20261012-h08`，manifest `artifacts/plan6-host-loop/20261012-08/docker-ownership.json`；日志`host-loop-h08.log`，进程结果`host-loop-h08-process.json`，输出`artifacts/plan6-e2e/plan6-host-20261012-h08`。不累计旧局部结果为完整追加成功。

### h08真实GLM正确响应与h09完整接续

h08完整证据证明GLM仅对指定Host调用event.query/history.query、metric.query/overview及两个event.get，正确返回最新详情数值、两条firing条件、处理/降噪和实际已恢复NoData健康事件。新断言写死两项而误拒第三条真实健康历史，属于验收器错误。改为逐项核对实际查询到的完整实例集合（包含健康事件），两条目标状态的CPU条件仍必须存在；不允许遗漏、伪造、其他Host或数值/状态错误。真实h08提取回归保存原回复/工具哈希，修正验证通过。h08本身依然退出1，尚未执行仪表盘模型、恢复和浏览器，不能视为整轮完成；资源清理/正式恢复已结束。

当前h09：`plan6-host-20261012-h09`，原生Go PID39916，manifest `artifacts/plan6-host-loop/20261012-09/docker-ownership.json`；日志`host-loop-h09.log`，进程结果`host-loop-h09-process.json`，产物`artifacts/plan6-e2e/plan6-host-20261012-h09`。参与源码冻结，继续完整追加验收。

### h09仪表盘取数与h10离线分析前件

h09真实GLM当前告警查询已通过精确结论、范围、实际参数、用量和只读配置验证。GLM仪表盘get/context.resolve/query/query.get均成功，仅查指定Host，两图success、文件封存并交付。模型准确声明缺少离线工具，没有把清单成功冒充数值分析。根因是追加runner只配置M4 smoke profile，遗漏已有configureP5Workspace公开API前件；补真实OpenSandbox backend/image/agent_workspace profile及额度，不放宽可用性或网络边界。原matrix验收器误按instant vector求和而失败，补矩阵点验证与独立samples/min/max/last计算，保留metric_sum=null语义、immutable hash和实际read/bash要求。h09资源清理/正式恢复结束，退出1；尚无恢复/浏览器通过。

当前h10：`plan6-host-20261012-h10`，manifest `artifacts/plan6-host-loop/20261012-10/docker-ownership.json`，日志`host-loop-h10.log`，进程结果`host-loop-h10-process.json`，产物`artifacts/plan6-e2e/plan6-host-20261012-h10`。前件覆盖包含真实offline Workspace profile/quota。相关完整runner包测试通过，参与源码冻结。

### h10真实文件分析通过与h11恢复通知时序

h10同轮真实GLM主机告警及仪表盘离线文件分析全部通过：两次成功bash，实际文件Hash/记录数与matrix samples/min/max/last吻合，精确Host范围、provider用量及配置不变。停止真实CPU负载后，两条件recovering→resolved，完整处理/屏蔽解除/抑制解除/恢复历史及原资源/来源保留通过。恢复通知对账在group wait发送完成前冻结，误报missing recovery；清理前持久事实显示16条投递已全部accepted，属于新增runner时序缺陷。补逐渠道×两固定条件×event类型等待accepted，然后冻结批次/读接收记录；同时核对发送owner UUID及正Fence，不用总数、健康事件或重跑结果替代本轮对账。h10退出1、清理及正式恢复完成，恢复模型与浏览器尚未执行。

当前h11：`plan6-host-20261012-h11`，manifest `artifacts/plan6-host-loop/20261012-11/docker-ownership.json`；日志`host-loop-h11.log`，进程结果`host-loop-h11-process.json`，输出`artifacts/plan6-e2e/plan6-host-20261012-h11`。源码冻结，相关完整runner包测试通过；整轮追加验收仍待最终通过。

### h11恢复协议通过与h12历史工具语义

h11首次/恢复四协议8批次对账及owner/Fence检查通过，完整处理/降噪/恢复历史和GLM当前告警/真实仪表盘文件分析通过。恢复后的GLM误用event.query传resolved，多次取空页后错误推断无历史；真实history仍有两条件resolved。工具共享Schema容许不能返回的状态，发现描述未明确当前空页不证明历史不存在。修正event/history工具独立生命周期enum及中英文Description/Result/Examples；不改变服务端SQL/HTTP、权限、元工具、搜索语义或Commit。补Schema错状态拒绝及正确历史状态通过回归，相关alerting/toolgateway/runner测试通过。h11退出1并清理恢复，真实浏览器尚未执行；保留原工具/回复证明模型错误。

当前h12：`plan6-host-20261012-h12`，manifest `artifacts/plan6-host-loop/20261012-12/docker-ownership.json`；日志`host-loop-h12.log`，进程结果`host-loop-h12-process.json`，产物`artifacts/plan6-e2e/plan6-host-20261012-h12`。普通中文请求及严格业务断言保持，模型自行发现history.query；源码冻结，完整追加门禁待通过。

### h12历史查询正确与h13模型投影减重

h12恢复模型已自行Describe/Invoke history.query，正确读取两条resolved及事件详情，并复用策略CPU表达式。最终总结前以CONTEXT_COMPACTION_FAILED终止：没有compaction模型调用，单个首用户回合的多次完整详情包含重复政策/来源快照，保守wire-byte预算触发hard compaction而没有可压缩的旧用户回合。未放宽预算/上下文窗口或改普通请求。落实原始Data/模型投影/展示分离：领域Metadata.Project提供ModelSummary，Agent仍按完整原文生成hash/ref并过滤/限幅，event.get保留一次实例触发快照和各事件关键事实/快照引用，完整事件原文与UI保持。敏感字段、原文不变、事实/引用、减重及相关告警/Agent/Gateway测试通过。h12退出1并清理恢复，浏览器尚未执行。

当前h13：`plan6-host-20261012-h13`，manifest `artifacts/plan6-host-loop/20261012-13/docker-ownership.json`；日志`host-loop-h13.log`，进程结果`host-loop-h13-process.json`，输出`artifacts/plan6-e2e/plan6-host-20261012-h13`。当前/历史Schema、实际离线分析与完整协议门禁保持，源码冻结。

### h13五业务与三浏览器通过，h14镜像别名核验

h13五个业务阶段、GLM当前/恢复/仪表盘精确结论、四协议首次/恢复8批、三真实浏览器（37.9秒、零失败）全部通过。最终镜像核验失败：egress Pod请求本轮tag，CRI上报正式r02仓库旧root。两根镜像platform manifest均148dd4…8940、config均f21beda…8207，实际平台内容相同而OCI root因构建记录不同。不是业务或新代码运行错误，但h13仍退出1并保留失败；清理恢复完成。

生产切换显式pin本轮egress root。别名仅在请求root严格等于验收root时可由该Pod所在节点的本轮owned image-loader读取content，通过逐级SHA-256验证上报root/platform/config；只有平台manifest及config与验收链完全相同才通过。未读取/修改共享控制器或正式镜像，未接受任意不同digest。相关镜像身份/完整runner回归通过。

当前h14：`plan6-host-20261012-h14`，manifest `artifacts/plan6-host-loop/20261012-14/docker-ownership.json`；日志`host-loop-h14.log`，进程结果`host-loop-h14-process.json`，输出`artifacts/plan6-e2e/plan6-host-20261012-h14`。仍要求同轮源码指纹、完整业务/模型/浏览器、最终核验/清理/恢复及退出0。
