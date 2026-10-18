# Plan7 复查修复与再次验收

日期：2026-10-17。**P7-R01／R02 已关闭，P7-T01～T09 全部 completed。** 完整冻结版本 `plan7-20261017-f40` 的 all 运行成功退出 0，全部门禁及环境恢复通过。

## 修复内容

1. **P7-R01 文件类型及预览。** 远程输出和普通上传共用[媒体类型识别](../../internal/workspace/import_media.go)，在正在校验的字节流中最多采样 512 字节；保持文件内容、Hash 和传输方式。查询文件沿用 JSON 契约，远程文本／CSV／二进制按内容识别，文本扩展名不能覆盖二进制特征。前端按明确类型决定是否读取文本预览，二进制仍可下载。回归覆盖实际 RPC／数据库记录、写入后恢复、误导性 `.txt` 后缀、CSV 预览与下载字节。
2. **P7-R02 原环境前后基线。** [环境快照](../../internal/app/argusdev/e2e_environment_snapshot.go)在任何缩容前保存工作负载健康、namespace／PVC／PV 身份及绑定。采集或存证失败不开始暂停；恢复前后核对存储身份，等待 Deployment／StatefulSet 就绪并比对健康，保存明确成功／失败记录。不会把仅恢复副本数当作健康恢复，也不会覆盖被替换的存储对象。
3. **模型流结束处理。** [Provider](../../internal/integration/modelprovider/provider.go)收到 Chat Completions `[DONE]` 或 Responses 终止事件后结束读取，不再等待连接 EOF。Chat 的独立 usage 帧仍被收集；提前结束、无效用量及未完成流仍被拒绝。保持既有超时上限。[回归测试](../../internal/integration/modelprovider/stream_completion_test.go)使用保持 HTTP 连接打开的真实测试服务复现修复前失败与修复后通过。
4. **证据范围和安全诊断。** 模型系统指令明确不适用／未观察不等于 0，指标不能证明日志错误数；实际请求回归覆盖两种模式。流失败日志增加有界类别，隐私测试保证不包含原始响应、地址或 API Key。
5. **工具参数错误恢复。** 按[协议定义](https://developers.openai.com/api/reference/resources/chat)，模型参数字符串可能不是合法 JSON。Provider 收齐完整响应和真实用量；Agent 在派发前验证 JSON 对象，工具入口继续执行原 Schema／权限校验。非法参数不会执行或默认补成 `{}`，会持久记录安全错误并让模型纠正。实际数据库回归验证重启仍不执行无效参数，已派发未知结果仍不重放。

## 验证与证据

| 项目 | F40 结果 |
| --- | --- |
| 完整运行 | `--suite plan7 --plan7-phase all`，退出 0 |
| 源码指纹 | `616ca4c3cf27e03a9d198d274c2324276ac3161e6d9a8df0f8651211c7c6c439` |
| 源码一致性 | 构建前后相同；2627 个清单内源码文件重新核对无差异，包含工作区未提交代码 |
| required 矩阵 | **57/57**；43 个不同引用证据文件的 SHA256 复核一致 |
| Go 证据 | **99 个具名检查**均执行通过；全量 Go 测试通过 |
| 真实浏览器 | **19/19**，无失败／跳过／flaky；双语、明暗和 1280／1440／1920 桌面宽度 |
| 真实模型 | GLM-5.3：**76 次调用全部成功**，含 74 次推理、2 次压缩；提供方用量及额度结算完整 |
| 完整工程检查 | Replay 协议、生成契约、前端类型／单测／样式／i18n、real 构建通过 |
| 实机补充 | 9 卷存储、Kubernetes 适配器、Linux runner／runtimewire 通过 |
| 环境恢复 | **20 个原工作负载、9 个原 PVC** 的身份／绑定及健康核对通过 |

本轮 Agent 输入 906749、输出 87262 tokens；只统计 F40，不含模型注册探测和此前失败轮次。结论同时核对不可变文件、远端实际效果、批准记录和模型请求 Hash，不以模型自述替代执行事实。

F34～F39 的中断、超时、脚本不匹配、用量不完整及数据口径失败保留在[实施记录](./08-implementation-notes.md)，没有混入本轮通过项。参数校验层级修复由独立 HTTP／数据库回归证明；不从历史的宽泛错误码倒推未保存的原始响应。

## 证据入口

- [验收摘要](./evidence/plan7-20261017-f40-summary.json)、[57 项矩阵副本](./evidence/plan7-20261017-f40-matrix.json)。矩阵 evidence.path 相对于原始 F40 目录。
- [完整日志](../../build/plan7-all-f40.log)、[原始结果](../../build/plan7-evidence/plan7-20261017-f40/result.json)、[工程检查](../../build/plan7-evidence/plan7-20261017-f40/plan7-engineering.json)、[部署自检](../../build/plan7-evidence/plan7-20261017-f40/verify/verify.json)。
- [构建前源码](../../build/plan7-evidence/plan7-20261017-f40/source-before-build.json)、[验收后源码](../../build/plan7-evidence/plan7-20261017-f40/source-after-acceptance.json)、[镜像来源](../../build/plan7-evidence/plan7-20261017-f40/image-provenance.json)。
- [主浏览器](../../build/plan7-evidence/plan7-20261017-f40/playwright-plan7/report.json)、[真实模型历史浏览器](../../build/plan7-evidence/plan7-20261017-f40/playwright-plan7-model/report.json)、[模型用量](../../build/plan7-evidence/plan7-20261017-f40/plan7-model-usage-summary.json)。
- [服务恢复事实](../../build/plan7-evidence/plan7-20261017-f40/plan7-service-recovery.json)、[上下文丢失恢复](../../build/plan7-evidence/plan7-20261017-f40/plan7-model-recovery.json)、[模型私有并发事实](../../build/plan7-evidence/plan7-20261017-f40/plan7-model-activity.json)。
- [暂停前环境基线](../../build/plan7-evidence/plan7-20261017-f40/original-environment.json)、[恢复后核验](../../build/plan7-evidence/plan7-20261017-f40/original-environment-restored.json)、[清理核验](../../build/plan7-evidence/plan7-20261017-f40/plan7-cleanup-verification.json)、[独立数据库夹具清理](../../build/plan7-evidence/plan7-20261017-f40/plan7-database-fixture-cleanup.json)。

恢复和私有并发截图已检查，文件类型显示为正确的 `text/plain`，旧未知执行事实保留。测试 namespace、Docker 目标、Windows 测试进程和临时目录均已清理，共享控制器身份及就绪状态保持；额外的独立 PostgreSQL 夹具经容器 ID 和归属核对后删除。

## 完成门禁与范围

本次冻结版本已独立通过全部既定门禁；当前任务状态及两项问题关闭记录见 [progress.json](./progress.json)，历史验收见 [F33 报告](./09-acceptance-report.md)。

Windows 实机范围仍为用户确认的 Windows 11；Windows Server 兼容性另记未验证。代码仅部署到隔离验收环境，既有业务部署已恢复，未发布本轮新版本。
