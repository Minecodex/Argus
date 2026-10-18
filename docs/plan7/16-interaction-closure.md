# Chatbox 交互收尾修复与复测

日期：2026-10-18。版本：chat-interactions-20261018-r02。状态：CB-R01／CB-R02 已关闭，工程、真实模型、浏览器、部署与数据保留验收通过。

## 修复内容

CB-R01 的根因是组件装配顺序。Server 和 Worker 在补充远程文件来源校验之前调用 Workspace.RegisterTools；该方法按值捕获 Service，导致注册的 publish_file 工具持有不认识 remote: 来源的旧配置。顶层 Workspace 工具和远程结果导入使用后续完整配置，因此文件能写入，而报告发布在来源校验阶段失败，未生成交付记录。

现在由 component.Workspace 构造完整的远程／Dashboard 来源校验，再交给 HTTP、顶层工具、原生工具注册和文件交换。两个应用入口共用构造路径。来源拒绝回归通过真实 PostgreSQL 和注册后的 publish_file 调用，验证拒绝不会进入文件存储或创建 Workspace 工作负载。

CB-R02 增加了新指导性计划实施 Run 的停止验收。mock 适配器原先不返回活动实施 Run，取消接口也未实现，现已补齐持久状态、归属检查和取消投影。12 个语言／主题／视口组合均验证确认计划后刷新、恢复停止按钮、点击停止及再次刷新保持停止状态；真实模型测试进一步验证远端长命令实际终止，后续命令未派发。

## 冻结与工程门禁

源码指纹：`65d81c929b0a7efe469c01823cf8640bc4e1b40116cf506b330f2f33c0a68ead`，2696 份源文件。手写文件均不超过 2000 行。

| 门禁 | 结果 |
| --- | --- |
| Go | 2231 项通过，98 项按独立环境开关跳过，74 个包通过 |
| 前端单测 | 1319 项通过 |
| 浏览器界面矩阵 | 17 项通过，包含三个桌面视口、中英文、明暗主题和新实施 Run 停止 |
| 类型、ESLint、样式、i18n、契约 | 通过 |
| real 构建 | Backend／Web 镜像构建与发布通过 |
| Windows 11 | 实际 PowerShell／runtimewire 回归通过 |
| Kubernetes | 隔离测试 namespace 的 Pod 状态、文件及实例替换通过；namespace 已清理 |

## 真实模型证据

沿用 kakj、APIMart glm-5.3-flash、正式 Connector 链路和本地 Docker Linux。最终四个实际浏览器场景通过：两种权限下的询问、计划修订、确认、ReAct 修复、报告下载，以及两种权限下的新实施 Run 停止。选用场景共 45/45 次模型调用成功，提供方输入／输出用量完整且配额已结算；6 个 Run 成功、2 个实施 Run 按用户操作取消。6 次普通操作确认的人工／策略来源符合对应权限模式。两个修复目标均实际恢复为 HTTP 200，两份下载报告的大小和 SHA-256 与服务端交付记录一致。

最终选用会话：

- [请求审批修复复测](https://argus.dev/?c=01a14cfb-a4fa-7b93-9c0b-475f51c1bc33)。
- [完全访问修复复测](https://argus.dev/?c=01a14cec-648d-751c-b2d1-2718f9495f2c)：独立复测 21 次模型调用均成功。
- [请求审批停止](https://argus.dev/?c=01a14cea-3371-7f35-80b7-96d4b7a0ce8e)。
- [完全访问停止](https://argus.dev/?c=01a14cea-f84d-7a0f-853e-16eac6fa3782)。

停止测试先创建指导性计划，人工确认后生成不同于规划 Run 的实施 Run；命令创建 started 后等待 90 秒，再尝试创建 finished，下一批再尝试创建 follow-up。浏览器刷新后点击停止，实际远程回执为 cancelled／confirmed；两类后续标记均未出现，随后复查仍保持此结果。

首轮四个浏览器流程通过，但用量审计发现 3 次模型链路调用失败并缺失提供方用量，不能用 Run 成功替代调用用量完整。两种修复模式都在新的专用目录独立重跑，最终验收仅选用完整通过的四个场景，明确列于 accepted-reports.json；原失败调用、自动恢复后的会话及其报告保留，不改写为成功。首次浏览器启动另因未启用本地域名证书测试配置而在 session API 失败，未进入模型流程；修正测试启动配置后重跑。

请求审批的第一次独立复测遇到企业 Workspace 配额已满：32 个保留会话各预留 2 GiB，占满 64 GiB，1925 字节报告也无法申请新 Workspace，因此下载断言如实失败。核对宿主仍有约 478 GiB 可用后，仅将本地测试企业额度由 64 GiB 调至 80 GiB，未删除会话或文件；随后在新目录重跑通过。最终预留量为 66 GiB。

最终调用仍保留模型自行纠正的参数校验错误，以及一次 REMOTE_CONTEXT_LOST。该上下文错误没有创建冻结执行请求（frozen_requests=0），后续通过新请求及正常确认恢复；不把这些工具错误改写为成功。报告发布与下载无失败。本轮未触发模型压缩，不声明新增真实压缩验收。

## 部署与保留

argus-platform revision 36、schema 18，20/20 健康检查通过，54 项已有工作负载／PVC 身份、副本与卷绑定保留检查通过。原会话和新实测会话均保留。升级前备份大小为 310727016 字节，仅存于本机私有目录。

第一次升级因已完成迁移 Job 的镜像字段不可变而失败。核验 Helm 归属和完成状态后，使用 UID／resourceVersion 前件仅删除该 Job，再升级成功；未删除数据库或 PVC。revision 35 的失败记录保留。

本轮工程、浏览器、真实调用、下载 Hash、审批来源及停止回执证据位于 `build/chat-interactions-20261018-r02/`。构建前、构建后及全部验收后源码指纹一致。独立测试 PostgreSQL 与匿名卷、helper 提取容器、临时 namespace 已清理；专用 Linux 主机、测试目标与 7 个实际会话保留供查看。旧版本证据继续对应各自源码；不混用历史 57 项矩阵通过数。本机受控 kubectl 因网络隔离能力不足而按设计拒绝，Windows Server 兼容性仍不在本轮已验证范围。

主要证据：[工程](../../build/chat-interactions-20261018-r02/engineering.json)、[最终调用审计](../../build/chat-interactions-20261018-r02/acceptance-audit.json)、[首轮含失败调用的审计](../../build/chat-interactions-20261018-r02/attempt-audit-r02.json)、[健康](../../build/chat-interactions-20261018-r02/health-after/verify.json)、[数据与资源保留](../../build/chat-interactions-20261018-r02/preservation.json)。
