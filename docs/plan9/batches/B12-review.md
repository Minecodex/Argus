# B12 用户查看路线：真实模型、监控与操作闭环

最新[压缩恢复修复验证](../34-b12-compaction-recovery-fix-20261116.md)已部署v4，[新会话](https://argus.dev/?c=01a1e422-2c55-74de-9aa0-05d2e37987b6)的查询与报告Run都已完成，9次实际压缩、恢复失败0。末尾模型报告仍有速率误当命中率的语义错误，已标记未通过；请结合记录中的独立原件校核查看，B12整体人工验收不自动通过。

最新[会话任务重跑计量](../33-b12-session-rerun-metrics-20261116.md)已完成，查看[新会话](https://argus.dev/?c=01a1e3fd-c390-7673-ae10-9e7d5e5fd1fd)中的压缩失败、后续报告及原件。工具成功率有所提升，但报告CPU/内存范围和MiB换算错误，**该报告未通过语义验收**；不应作为正确观测结论使用。所有阻断、失败与计费均保留。

2026-11-16 补充：[通用上下文机制正式实现及实际复验](../32-b12-generic-context-runtime-20261116.md)已部署。请查看[压缩续读会话](https://argus.dev/?c=01a1e3ce-ed7f-72d9-96c9-b1876258daf0)中最后一份 `b12-context-verification.md`（7763字节），为模型按验收反馈重新读取原件后修订，六面板数值、身份、Hash和CSV整数均经独立代码校核；旧报告及原始291条历史不删除。下面“50次调用”“仅定位未修复”为早先现场记录；本次额外80次/$0.11052476，完整分项见新记录。旧工具错误未在新69条结果中复现，但模型仍有重复查询，部分取数 Run 已按观察预算停止，因此不宣称调用效率或自动结束全通过。B12 继续等待人工验收。

运行 `p9-b12-20261116-01`。**已部署并完成自动自检，保留环境，等待用户验收。** B11c 已按用户“没问题，开始下一轮”记为通过并正常卸载/销毁；历史审计、告警和录像保留。B13 尚未启动。

## 推荐查看顺序

1. 打开 [本批真实对话](https://argus.dev/?c=01a1e1f0-4809-78f5-bb8b-9b7b96e35a44)，沿用企业 `test` 的 `kakj` 正常登录。模型为 **B12 APIMart GLM-5.3-Flash**，当前配置 128K 应用上下文、4096 输出上限、低思考档位，同一个供应商/模型，没有 replay/mock 或自动换模型。
2. 对话中先可看到保留的容量/压缩失败与后续真实重试，再看 Workspace 分析报告、Host/Redis 面板取数结论，以及“已取消”和“执行完成”的普通命令卡片。展开卡片查看目标、root 账号、原始脚本、原生退出码和 HTTP 结果。
3. 预览或下载 **B12 Redis INFO 附件分析报告**。真实文件名 `b12-analysis.md`，2226 字节；[正常鉴权下载](https://argus.dev/api/v1/conversations/01a1e1f0-4809-78f5-bb8b-9b7b96e35a44/deliveries/e19d91f1-3feb-4b41-8a3b-09191f456868/content)。原始 CSV 是此 Redis 的8次实际 INFO，Workspace 实跑代码得到 `command_delta=37`、`peak_memory_bytes=1562696`，下载字节与结果已核对；没有将监控命令冒充业务吞吐。2026-11-16 已[统一修正文件预览](../29-b12-file-previews-20261116.md)：CSV 为表格，自定义报告标题也能按 Markdown 渲染，支持原文切换；真实登录的三种桌面宽度验收已通过。
4. 打开 [B12-Cache-Host](https://argus.dev/hosts/01a1e1dd-d568-724d-8d0b-40098d812b43)、[主机列表](https://argus.dev/hosts) 和 [中间件监控](https://argus.dev/monitoring)，查看新主机、B12-Redis、五项原生采集及实时小图。此批为新身份，未复用 B11c Host/Collector。主机控制与遥测均实际使用 Executor Tunnel；SSH 安装执行路径是 Direct Executor。
5. 查看四套 Host 默认仪表盘：[Host](https://argus.dev/dashboards/cee08ea9-0beb-5c7b-a006-e27d11e61d39?resource=01a1e1dd-d568-724d-8d0b-40098d812b43) / [Host processes](https://argus.dev/dashboards/c270e46c-5b78-5c92-8f5f-ea3b1009ff44?resource=01a1e1dd-d568-724d-8d0b-40098d812b43) / [Linux system services](https://argus.dev/dashboards/526b50e6-f324-5727-994f-cbdf0814881c?resource=01a1e1dd-d568-724d-8d0b-40098d812b43) / [Collector](https://argus.dev/dashboards/258c6833-6dce-5240-bddf-81bbf7c6f2c2?resource=01a1e1dd-d568-724d-8d0b-40098d812b43)。另有 [Redis 默认仪表盘](https://argus.dev/dashboards/6f0cd210-ab49-5d65-be71-f63eb4535beb?resource=3b24204d-9d9d-5c2a-b3fa-e37d621183a8)。共 **31 个面板**，模型实际读取 Host/Redis 的查询任务结果，不仅列目录。
6. 打开 [本批告警历史](https://argus.dev/alerts?tab=history&policy_id=f27f2413-e3e3-51a6-9f61-7a5552538c3a)，看 **B12 cache orders service failed** 的真实触发、恢复与时间线。当前 HTTP 服务和 Redis 均健康，本批业务告警已恢复，没有外部通知投递。

## 实际行为与时间（北京时间）

| 环节 | 已核实的实际结果 |
| --- | --- |
| 模型兼容性 | 基础响应、流式、工具调用、结构化输出四项均真实通过；模型和密钥通过正常平台 API 创建，密钥只写、加密保存 |
| 面板查询 | 模型已完成 Host/Redis 两个范围的真实取数任务，窗口及单位见对话；Host CPU/内存与 Redis 内存/命令速率/命中率均有真实数据证据 |
| Workspace | 实际挂载文件、执行代码、写入并发布报告；计算实例回收后，文件仍可正常鉴权下载 |
| 取消 | 第一张模型生成的恢复卡片经 UI 取消，原生状态 cancelled、结果仍 pending；没有派发执行，物理服务当时仍 failed |
| 受控故障 | 2026-11-16 10:10:09 对本批 HTTP 服务主进程注入 SIGKILL；Redis 与 Collector 保持运行，600 秒恢复定时器兜底 |
| 确认执行 | 新模型请求经真实卡片确认；2026-11-16 10:11:02 原生命令退出0，stdout 包含 active、health_http_status=200、Redis PONG，发生在恢复定时器期限之前 |
| 告警 | 实际告警 2026-11-16 10:10:40 触发，2026-11-16 10:11:30 恢复；本轮未手工制造告警行 |
| 最终状态 | 新 Run succeeded；HTTP 200、服务 active；五项采集 healthy/fresh；连续120秒无丢弃、拒绝或入队失败；正式健康20/20 |

## 用量、预算与保留失败

平台目前记录 **50 次调用**（其中压缩 12 次），实报输入 **819,839**、输出 **25,268** Token，按配置价格累计 **0.06780489 美元**。此数不包含兼容性探测，供应商实际账单以 APIMart 为准。参考价来源：[APIMart GLM 基础价格](https://apimart.ai/ru/model/llm/glm)。模型专属部门、用户月度额度均为1；本批按调用、Token 与十分钟 Run 窗口观察阈值停止，不自动增加额度。

初始 32K 配置曾被容量门禁正确拒绝，输入和草稿保留，尚未派发聊天模型调用。调整到官方支持范围内的128K后，首个只读 Run `01a1e1fb-1f42-762a-b9c0-550690e9438a` 在默认思考档位上发生 `CONTEXT_COMPACTION_FAILED`，失败压缩输出达到4096 Token上限。该失败和已消耗用量均保留；同模型低思考档位兼容性复测后，后续业务步骤完成。

**累计 `usage_complete` / `cached_usage_complete` 仍为 false**：聚合包含这条保留的失败调用，不将它改成成功。只读核对所有调用的输入、输出和缓存字段来源均为 provider，当前没有估算 Token；不是把未知缓存量补零。对话中的工具发现协议错误和模型自纠也保留，因此不声称所有中间调用一次成功。

2026-11-16 用户指出大量工具失败后，[逐项根因核对](../30-b12-tool-failure-analysis-20261116.md)确认：补齐指标 Run 的35条工具结果有18次失败（4次发现缺失、3次错误条件版本、5次参数/证据错误、6次文件读取失败）。六次读取失败关联的实际压缩快照缩短了完整 UUID 路径，属于平台上下文保真缺陷与模型错误使用缩写共同造成，不能只记为模型自纠。最终闭环成功仍成立，但不代表调用质量通过；本次仅定位，修复和强制压缩回归尚未实施，人工验收仍待确认。

随后按用户要求完成[通用引用原型的压缩断点对照](../31-b12-compaction-controlled-comparison-20261116.md)：两组共用原始文件、摘要、用户要求及模型，候选只增加精确引用。各重复2次，四次数值正确；模型调用12→7、工具失败4→1、输入Token123504→47880。基线一次 Bash 内出现6次缩写路径打开失败，候选为0。该结果是受控原型小样本，不是完整生产Harness新旧版A/B；正式执行器和本批原会话未修改。总22次新增直接供应商实验请求独立留账，其中1次预试用量未知保留；不冒充平台现有50次原生调用的一部分。临时Namespace已清理，生产接入与原12次契约失败的修复仍待后续。

几次浏览器失败为测试定位/等待问题：发布下载入口按报告标题命名；普通命令卡片不是旧完整计划布局；确认后状态投影短暂滞后时不能重复寻找确认按钮。测试已修正，使用原有已完成记录只读复查，未重放模型回复或再次执行修复命令。截图覆盖中英、明暗及1280/1440/1920桌面宽度。

## 环境与证据

新场景为一个 systemd/OpenSSH Host、一个受限 Redis 和实际 HTTP 缓存服务，合计2 CPU、1472 MiB；每5秒真实请求，最长8小时。主机共享 Docker VM 内核，硬件容量口径不代表独立物理机。Redis 应用账号仅允许本批键前缀，监控账号只允许 INFO/CONFIG GET/PING，不读取业务键。

Workspace 通过独立平台管理员正常配置本地 backend、固定摘要镜像及离线 Profile（500m CPU、512 MiB、600秒、配置网络none），企业最大并发1、月计算秒数3600。保留共享运行基座、账号和正式六个 PVC；本批未做业务 SQL 写入。清理 B11c 时修正的历史依赖投影已发布并通过真实正常卸载，未强制忘记未核实目标。

证据目录：`C:\Users\23574\.codex\artifacts\argus-plan9\p9-b12-20261116-01`。重点是 `checks.json`、`complete-workflow-ledger.json`、`model-native-query-proof.json`、`model-analysis.json`、`model-workspace-report.md`、`model-cancellation-proof.json`、`model-repair-proof.json`、`model-usage-final.json`、`delivery-watch.json`、`formal-health.json` 及各 `browser-*`。`private/` 不公开或提交，密钥不进入 Markdown/截图/源码。

环境继续保留，自动自检不代替用户确认。收到下一批指令后按 [运行包](../runtime/B12/runbook.md) 归档、停止本批调用/负载、退役 Host/监控并销毁专属容器卷；正常保留聊天、交付文件、计费和审计历史，以及用户提供的模型配置和共享 Workspace 基座。
