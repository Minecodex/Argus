# 本地发布与模型实测结果

本页记录 2026-10-17 的 R04 历史发布：该次本地重建完成，APIMart `glm-5.3-flash` 前三轮实测均未整体通过。2026-10-18 的新补丁部署、实际 `kakj` 账号验收与界面历史以 [14-apimart-linux-chatbox.md](./14-apimart-linux-chatbox.md) 为准。独立 GLM-5.3 的 57 项回归不作为 APIMart 通过证明；旧失败保留。

## 2026-10-17 历史发布

- 环境为 `docker-desktop`，保留既有 `argus-system`、`argus-sandbox`、`argus-ingress`、用户配置和业务数据。
- 企业门户：[https://argus.dev](https://argus.dev)；平台门户：[https://platform.argus.dev](https://platform.argus.dev)。两个实际入口均通过默认 TLS 校验并正常渲染登录页，无页面异常。
- 七份候选镜像按验收摘要固定：Backend、Web、MinIO、Workspace、Workspace Egress、Collector、remote runtime。8 个 Backend、Web 和 MinIO 共 10 个实际运行工作负载的 Image ID 与对应描述符链匹配，6 项运行配置镜像绑定正确。
- Helm：`argus-data` revision **12**、`argus-sandbox` revision **11**、`argus-platform` revision **28**；均先服务端 Dry Run，再保留 values 升级。数据库 schema **16**，三个远程执行权限及既有内置管理员授权完整。
- 当前 Connector 与三平台 Collector 制品发布到唯一版本路径，沿用原签名身份。实际 HTTPS 下载核对清单、8 份制品的大小和 Hash，全部二进制签名正确；既有远程主机的已安装程序未自动替换。
- 已有离线 Workspace 镜像登记通过原 ID、版本、引用和摘要的条件更新；其 ID、Profile、现有会话及文件保持。

升级核对保留 **27 个原工作负载、9 个 PVC、39 个业务 Secret**，其他工作负载和共享控制器 Spec 保持；12 张业务表的原 25 个 ID 全部存在。部署健康自检 **20/20** 通过。发布前数据库备份和敏感配置在当前用户受保护目录保留，未卸载重装或清空数据。

## 独立发布回归

此前已授权的 GLM-5.3 用于 `plan7-release-20261017-r04` 的独立 `--suite plan7 --plan7-phase all` 回归，**进程退出 0**，原环境恢复通过。它检验当前代码的发布条件，未修改 APIMart 三轮失败状态。

| 项目 | 结果 |
| --- | --- |
| 冻结源码 | `1bfaa8486b974c54fc674920826677e2c855cebf1c25c294fef7c5ed3678a7e3`，构建前后相同，2633 个清单文件 |
| 完整矩阵 | **57/57**；45 份引用证据 Hash 复核一致 |
| 具名 Go 检查 | **104 项**，包含 PostgreSQL＋HTTP 的活动长任务压缩恢复 |
| 真实浏览器 | **19/19**；无失败、跳过或 flaky，双语、明暗及三个桌面视口 |
| 实际模型 | **82/82** 调用成功：80 推理、2 压缩，提供方用量和内部额度结算完整 |
| 模型用量 | 输入 962545、输出 85378 tokens；不含注册探测和此前失败轮次，不代表外部账单金额 |
| 工程检查 | 全量 Go、Replay、契约、前端类型／单测／样式／i18n、real 构建、9 卷存储、Kubernetes 适配器、Linux runner／runtimewire 均通过 |

实际模型覆盖取数→Workspace 分析→现场确认→计划反馈修订→一次批准→串行执行→服务恢复验证，以及连续确认、上下文丢失恢复、Windows／Kubernetes 文件交换、证据局限和仅模型可见的并发事实。模型自述未代替原始文件、批准记录或实际远端效果。

Plan7 构建入口直接使用正式 Web 配置并生成回执；矩阵核对实际镜像摘要，拒绝测试出口地址、Mock、来源错配或摘要替换。正式发布使用同一份通过验收的 Web 镜像。

## APIMart 测试

地址按[官方兼容接口](https://docs.apimart.ai/en/api-reference/texts/general/chat-completions)采用 `https://api.apimart.ai/v1`，请求始终使用用户指定的 `glm-5.3-flash`。实际目录和流式工具调用探测通过，响应标识为 `ZHIPU/GLM-5.3-Flash`；未据此推断底层权重或完整兼容性。

| 轮次 | 已取得的证据 | 未通过原因 |
| --- | --- | --- |
| R01 | 控制链路、18 项主浏览器、遥测分析、现场诊断、原计划／反馈修订及第二版远端步骤执行 | 一次响应头前传输错误；修复后分析最终 `CONTEXT_COMPACTION_FAILED`。诊断保存 29 次成功、1 次失败调用 |
| R02 | 16 次模型调用均成功，CPU 最小／最大／最后值正确 | 将 13 个采样点填入 `records`，原始文件实际是 1 条序列；结论核对拒绝 |
| R03 | 正式构建、控制链路、18 项主浏览器、实际模型调用 | 一次 HTTP 200 后流读取或事件处理错误（约 132 秒），初始遥测任务最终超过既定 10 分钟窗口；诊断时 17 次成功、1 次失败、1 次未完成，用量不完整 |

**APIMart 的完整修复／恢复／跨平台模型流程尚未通过验收。** 最后一类流错误的底层原因未定位，当前有界日志不足以区分提供方、传输或事件写入层；不能笼统归因为模型或平台。没有通过提高超时、改写答案、放宽用量条件或替换该测试模型取得通过。

三个失败轮次的日志及诊断保留；各轮原 20 个工作负载及 9 个 PVC 均恢复核验通过。临时 APIMart 密钥只经受保护的加密文件和进程环境传入，460 份本轮证据及日志无明文匹配，加密副本已核验归属后删除。原业务模型配置未持久替换为测试密钥。

## 修复与范围

**P7-R03 活动长任务压缩。** 硬阈值可在完整工具组之后压缩，活动审批仍受保护；本 Run 原始要求和修改意见从不可变事件记录按顺序重建，附件按当前状态复验。摘要不能产生批准或重放权限。实际 PostgreSQL／HTTP 回归验证预算恢复、续任务、原事件保留和原文约束；独立发布回归的长上下文与恢复场景通过。

**P7-R04 统计口径。** 实际系统指令及报告契约明确 matrix 的 `records` 是序列条数，`samples` 是 `values` 中采样点数量。单／多序列回归通过；原始文件 Hash、统计计算和结论比较不变，未对错误答案归一化。这些代码及契约修复已验证，APIMart 整体可用性仍保留未通过。

本地仍为 Evaluation：原有 NetworkPolicy deny 未确认、默认出口路由及共享容器 Sandbox 的限制明确保留；20/20 健康检查不等于 Production 加固通过。Windows 实机为已确认的 Windows 11，Windows Server 兼容性未验证。测试 namespace、Docker 目标、Windows 进程／临时目录和独立 PostgreSQL 夹具已清理，原三个 Docker 容器及共享控制器保留。

## 证据

- [发布摘要](./evidence/plan7-release-20261017-r04-summary.json)、[57 项矩阵副本](./evidence/plan7-release-20261017-r04-matrix.json)；矩阵路径相对原始运行目录。
- [发布回归日志](../../build/plan7-release-r04.log)、[退出记录](../../build/plan7-release-r04-exit.json)、[原始结果](../../build/plan7-evidence/plan7-release-20261017-r04/result.json)、[模型用量](../../build/plan7-evidence/plan7-release-20261017-r04/plan7-model-usage-summary.json)、[证据复核](../../build/plan7-release-r04-evidence-audit.json)。
- [构建前源码](../../build/plan7-evidence/plan7-release-20261017-r04/source-before-build.json)、[验收后源码](../../build/plan7-evidence/plan7-release-20261017-r04/source-after-acceptance.json)、[镜像](../../build/local-release-plan7-20261017/images.json)、[Web 构建回执](../../build/plan7-evidence/plan7-release-20261017-r04/web-build-config.json)。
- [部署](../../build/local-release-plan7-20261017/deployment.json)、[运行镜像](../../build/local-release-plan7-20261017/runtime-proof.json)、[健康检查](../../build/local-release-plan7-20261017/verify/verify.json)、[门户浏览器](../../build/local-release-plan7-20261017/portal-browser-smoke.json)、[实际下载验签](../../build/local-release-plan7-20261017/artifact-download-verification.json)。
- [业务记录保留](../../build/local-release-plan7-20261017/database-preservation.json)、[资源／凭据保留](../../build/local-release-plan7-20261017/preservation.json)、[schema／权限](../../build/local-release-plan7-20261017/catalog-and-schema.json)、[清理](../../build/local-release-plan7-20261017/cleanup-verification.json)。
- [R01 日志](../../build/plan7-apimart-r01.log)、[R02 日志](../../build/plan7-apimart-r02.log)、[R03 日志](../../build/plan7-apimart-r03.log)、[密钥扫描](../../build/plan7-apimart-secret-scan.json)、[密钥清理](../../build/plan7-apimart-credential-cleanup.json)。
