# B10a 工作台与采集生命周期修复（2026-11-11）

B10a 以真实对象操作发现四项共享问题，修复同时服务所有 Kubernetes 集群和工作台。没有对测试集群做专用查询或数据绕行。当前用户查看入口见 [B10a 记录](./batches/B10a-review.md)。

## Secret 引用与敏感值

原通用敏感字段策略将内置 Pod 模板的 `volumes[].secret` 引用结构误判为凭据，导致使用 Secret 挂载的正常 Deployment 创建被拒绝，对象查看也丢失引用。

`kubernetesinventory.ProjectManifest` 在共享清单投影中识别内置 Kubernetes GVK 和准确的 SecretVolumeSource/SecretProjection 路径，仅保护有效引用结构，继续对内部内容应用敏感字段规则。清单校验和公开对象读取使用同一入口；Secret 内容和明文敏感环境变量仍被保护，未知 GVK/字段不会取得例外。JSON 深拷贝保留整数精度，不修改调用者输入。

验证覆盖七种内置工作负载、直接/投影卷、未知结构、环境变量、敏感字段、已脱敏值和大整数；真实平台创建的 Deployment/StatefulSet/DaemonSet 均挂载本批 Secret。Secret 正常更新后的容器挂载也已核对。

## 操作历史的 ID 游标

工作台以 ID 排序分页，但共享签名游标强制时间字段，超过默认 20 条后接口返回错误。只在明确 `Sort=id` 时允许 ID 位置，不改变既有排序；其余时间排序仍要求时间字段。签名、主体、企业、过滤器、授权版本和过期校验保持。

真实接口 limit=1 的下一页和门户多页操作记录已验证。浏览器测试按实际操作类型、真实分页和查询完成状态定位记录，未假定 UUID 顺序等于执行时间。

## 已交付凭据的交互会话

Secret FulfillLease 正常交付会把一次性租约置为 consumed，后续会话持续授权却只接受 active，终端/转发连接立即被判为授权变化。

持续授权仅在会话为 connecting/active、已经由准确执行端认领、Direct 的 execution gateway 与 owner 一致时接受 consumed。既有租约期限、资源绑定、接收方、凭据/密钥版本、路由、当前用户权限、HTTP Session、Cluster/Namespace/Pod 身份仍逐项检查。第二次领取凭据仍原子拒绝，未认领和错误执行端不能借 consumed 启动新后端。

真实 PostgreSQL 门禁验证首次交付后激活和心跳、重复领取拒绝、错误执行端拒绝及凭据轮换撤销；已部署 CLI 转发获得真实 HTTP 和双向字节，门户实际执行命令返回容器挂载的 `updated`。

## 短命 Namespace 的指标批次

创建后快速删除的 Namespace 可能从未进入分钟级库存。直接带原生 Namespace UID 的指标将无活动库存持续解释为元数据待就绪，阻塞整个混合批次，最终超出 300 秒重试上限。本批修复前实际丢弃 133 个点，原台账和日志保留。

沿用 [未验证 Namespace 归属](../plan8/79-unverified-namespace-attribution.md) 的既定 Cluster 范围语义，将直接 UID 路径的「无活动库存」纳入同一行为：可信 Collector/source/配置/Cluster 仍必须验证；保留 Cluster 数据，移除伪造的保留字段，不写可信 Namespace UID，标记归属 unverified。明确的活动库存 UID 矛盾仍拒绝，数据库故障仍重试。Namespace 授权查询不会因此看到这些数据，不能按名称映射到其他代次。

真实 PostgreSQL、混合批次不部分改写与 Namespace 范围隔离回归通过。发布后重新通过 Argus 创建/删除真实 Namespace，并观察 311.8 秒：终止失败、入队失败、接收拒绝增量均为 0，三项采集持续推进，原生日志没有新的终止丢弃。历史缺口不自动变成恢复，不能宣称修复补回了旧数据。

## 发布与验证边界

正式库仍为 Schema52。从前一轮冻结源码构建修复；没有运行正式迁移。当前发布：

| 组件 | 镜像版本与摘要 |
| --- | --- |
| Server / Worker / Direct Executor / Connector Gateway | `local-20261111-b10a-lifecycle-v3@sha256:d2afa58071575e0666e88dc73a46d0fbbbaeced7be2b184ab05d6c2920042ea2` |
| Telemetry Ingest | `local-20261111-b10a-lifecycle-v4@sha256:54c44dd6fe0725f0eec363af961ed19ef3bd2e708b2cf756d560829bc9cb7ca8` |

精确 digest 已加载正式节点 containerd 并验证运行镜像。原生 Collector 沿用已修复的 B09b v2 镜像。其他部署和六个原有 PVC 绑定保留。

- [工作台 PostgreSQL 门禁](../../artifacts/plan9/p9-b10a-20261111-01/postgres-regression.json) 与 [Namespace 门禁](../../artifacts/plan9/p9-b10a-20261111-01/postgres-namespace-regression.json) 使用自有临时 PostgreSQL，结束后删除。
- [真实操作](../../artifacts/plan9/p9-b10a-20261111-01/object-operation-exercise.json)、[配置/网络](../../artifacts/plan9/p9-b10a-20261111-01/configuration-network-exercise.json)、[真实页面](../../artifacts/plan9/p9-b10a-20261111-01/browser-review.json) 与 [仪表盘渲染](../../artifacts/plan9/p9-b10a-20261111-01/dashboard-browser-review.json) 分别保存。
- [投递观察](../../artifacts/plan9/p9-b10a-20261111-01/delivery-watch.json)、[历史缺口](../../artifacts/plan9/p9-b10a-20261111-01/current-delivery-inspection.json) 和 [最终核验](../../artifacts/plan9/p9-b10a-20261111-01/checks.json) 区分已发生失败与修复后增量。
- Go vet、生成契约检查、前端类型和两份 E2E lint 作为代码检查。契约工具原有 OpenAPI warnings、正式 evaluation 环境原有 NetworkPolicy/Egress Gateway 警告不计为本次新增修复项。

自动化成功只代表可交付查看，人工验收仍 pending。B10b 的存储/任务、B10c 的 Helm/RBAC 委派与节点治理尚未运行。
