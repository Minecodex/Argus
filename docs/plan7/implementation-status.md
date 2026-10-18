# 实施状态

> 2026-10-18：当前 Markdown 计划、一次性命令授权与普通会话展示已完成，见 [第 18 篇](./18-markdown-plan-command-consent.md)。本地 revision 40／schema 19，健康 20/20；4 个最终真实场景、38 次干净模型调用完整通过，原 39 个聊天和5881条事件保留。下文旧版本号、固定计划规则和通过数保留历史适用范围。

更新日期：2026-10-18。**原 Plan7 范围的 P7-T01～T09 已完成历史验证；最新补丁也已完成本地部署和实际账号的 Docker Linux 四类业务验收。** 使用 APIMart `glm-5.3-flash`，同版本通过案例47/47次模型调用用量完整、浏览器2/2；早期失败保持。详见[实际账号实测](./14-apimart-linux-chatbox.md)。

历史完整发布为 plan7-release-20261017-r04、源码1bfaa848…，57项矩阵、104项具名检查、19项浏览器、GLM-5.3的82次调用通过。当前 Backend／Web 补丁为 plan7-http1-20261018-r03、源码7daac58…；2652份清单构建及验收前后一致，2169项Go检查、数据库／HTTP、契约、前端检查及real构建通过。最新Flash结果来自当前版本的实际调用，与旧跨平台矩阵分开记录。

| 任务 | 状态 | 当前交付 |
| --- | --- | --- |
| P7-T01 领域与契约 | completed | 领域、模式快照、SQL、Schema、OpenAPI、protobuf、客户端与权限一致 |
| P7-T02 Agent 场景与恢复 | completed | connect、连续确认、Workspace 分析、压缩／重启恢复；原始要求与修订保留、统计口径明确 |
| P7-T03 资源适配与共享会话 | completed | Linux、Windows 11、Kubernetes 正式路径、身份、会话、控制／输出／文件通道及停止 |
| P7-T04 connect 闭环 | completed | 冻结预览、确认及派发复验、幂等、结果与新批次再确认 |
| P7-T05 固定计划 | completed | 一次批准、串行、修订、后台执行、停止与新版本恢复 |
| P7-T06 文件与证据 | completed | 自动交换、Hash／来源／完整性、独立重传、正确媒体类型及二进制下载 |
| P7-T07 Chatbox | completed | 两模式、计划／命令卡、进度／停止／恢复、双语主题、三个桌面视口；正式 Web 无测试出口覆盖 |
| P7-T08 权限与审计 | completed | 企业／资源／会话隔离、撤权与失效、凭据保护、私有模型并发上下文 |
| P7-T09 验收与交付 | completed | 历史完整矩阵57/57；当前同版本Linux四类案例47/47调用完整、2/2浏览器；20/20健康及原数据保留，跨平台Flash未重跑 |

本地部署为 argus-data revision12、argus-sandbox revision11、argus-platform revision33，schema16；原27个工作负载、9个PVC和25个业务记录ID保留，健康20/20。9个Backend／Web运行容器匹配候选的Linux清单；Web缓存使用旧索引别名，但实际平台清单相同，已复核描述符链。独立临时数据库已清理；实际账号历史、专用Docker主机及正常加密的模型凭据留供用户审查。

早期压缩、统计、预算参数、截断状态、HTTP/2及ALPN问题和重跑记录单列。当前四类案例分别在同一冻结代码／模型配置通过，原断言与门槛不变；旧失败和未知用量没有改成成功。恢复案例如实记录了Workspace能力受限及工具错误，远端实际恢复与不重放验证通过；主流程和连续取证实际完成了Workspace分析。

Windows 实机范围仍为 Windows 11，Windows Server 兼容性未验证；本地 Evaluation 的网络与共享 Sandbox 限制保留。客户 MCP 独立边界不变。
