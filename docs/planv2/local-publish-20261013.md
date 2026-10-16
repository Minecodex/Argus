# 本地 Kubernetes 镜像发布：2026-10-13

按用户“本地 k8s 全部重新构建新的镜像发布下”的要求，将当前工作区的 Argus 实现发布到 `docker-desktop`。节点架构为 `linux/amd64`，控制面、遥测和数据服务位于 `argus-system`，沙箱位于 `argus-sandbox`。沿用现有 Helm 配置、Secret 引用、PVC 和业务数据。

## 镜像与运行配置

本次六个仓库内 Dockerfile 均重新执行构建，以 `chat-analysis-20261013-r1` 推送本地 Registry，并载入节点 containerd 缓存。允许正常构建缓存复用；正式运行引用使用本次 Manifest SHA-256 摘要固定。完整引用见[镜像清单](../../artifacts/local-publish-20261013/images.json)。

| 镜像 | 发布对象 |
| --- | --- |
| argus-backend | Server、Worker、Direct Executor、Connector Gateway、PKI Controller、Telemetry Ingest/Writer/Query，以及 Migration/目录同步 Job |
| argus-web | Enterprise、Platform、Template Runtime 三个真实 API 构建 |
| minio | 当前 Artifact Store |
| argus-workspace | 已登记离线分析镜像，保留 Image ID、Backend 和 Profile，原镜像 Version 1 更新到 2 |
| argus-workspace-egress | 新建 Workspace 的 Egress 运行配置 |
| argus-otelcol | 从当前源代码构建 linux-amd64 Collector，更新 Kubernetes 安装镜像配置 |

Connector 三平台分发制品随 Backend 构建。Collector 镜像另行构建并导入，未把未包含 Collector 的五镜像 `images build/load` 输出称为六镜像发布。现有远程主机的已安装程序不在本次 Kubernetes 镜像发布范围内。

`argus-data` 升级到 Revision 11，`argus-platform` 升级到 Revision 15。两次 Upgrade 均先经过 Server Dry Run，再使用 `--reuse-values` 和明确的镜像覆盖文件发布。数据库 Migration 和 Telemetry Catalog Sync 成功，8 个 Backend Deployment、Web 和 MinIO 完成更新。Workspace 镜像登记经维护事务更新，使用原 ID、Version、引用和摘要作为并发条件，只修改镜像引用、摘要及对应 Version/更新时间。

第三方依赖沿用当前已部署版本，包括 PostgreSQL 18.6、Redis 8.10.0、ClickHouse 26.3.17.110、OpenSandbox Server v0.2.2 和外部共享 Controller；不以旧安装配置中的默认版本覆盖它们。

## 验证与保留

- 全量 `go test ./...` 通过。
- 全工作区前端 Typecheck、Lint、i18n、共享样式检查通过；95 个测试文件、414 项前端测试通过；三门户真实 API 镜像构建通过。
- 当前部署 `argusctl verify` 20/20 通过，包括工作负载、服务健康、HTTPS/PKI、数据库、Redis、对象存储、Kafka、ClickHouse 及 OpenSandbox 生命周期。见[正式健康检查](../../artifacts/local-publish-20261013/verify/verify.json)。
- 六个 Registry 标签与记录的摘要一致，均包含 linux/amd64 Manifest；8 个 Backend、1 个 Web 和 1 个 MinIO 正在运行的 Image ID 与本次摘要匹配，未仅凭 Deployment 的镜像文字判定发布成功。
- 18 个控制面工作负载及 2 个 Sandbox/Ingress 工作负载 UID/副本保持且 Ready；控制面的 6 个 PVC UID、Volume 名称保持且 Bound。Sandbox 的 3 个既有 Workspace PVC 当前均 Bound；本轮未保存其发布前独立快照，不将此项表述为前后 UID 比较。
- 原有 1 台主机、1 位企业用户、3 张仪表盘、3 个会话和 1 个模型的原始 ID 全部保留。Helm Values 对比确认仅声明的镜像覆盖发生变化；19 个其他项目/集群工作负载的 UID 和 Spec 保持。见[发布核对](../../artifacts/local-publish-20261013/release-verification.json)。
- 正式 Enterprise、Platform 浏览器登录页正常加载，无控制台错误。此项只证明入口加载，不替代登录后的自动分析浏览器验收。自动分析的独立真实模型、存储和双语主题回归仍按[专项报告](./automatic-analysis-20261013.md)记录。

本地 Evaluation 环境原有的 NetworkPolicy deny 未确认、默认出口路由及共享容器 Sandbox 状态保留在健康检查报告中；20/20 不代表 Production 隔离加固验收或当前 PlanV2 整版联合回归完成。

发布前数据库备份、Helm Values、Dry Run 与渲染结果保存在 `artifacts/local-publish-20261013/private/`，避免将凭证放入公开证据。临时健康探针已清理，既有业务 Namespace 和持久卷未删除；本地 Registry 和受管理的 Image Loader 保留供后续发布使用。
