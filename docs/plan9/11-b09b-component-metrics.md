# B09b 组件指标与直方图查询

2026-11-09，继续完成当前 Kubernetes 集群中 API Server、CoreDNS、Scheduler、Controller Manager 和 etcd 的正式安装采集与默认仪表盘验证。底层组件配置和模板复用 Plan8 的既有实现，本轮验证实际已登记 Collector 到部署后门户的完整链路。

## 独立集群准备

当前集群是 B09a 创建并记录 Docker ID 的独立 Kind 节点。本轮保留其资源身份、工作负载、日志和已有查看入口，新增 B09b 所有权的授权及有界活动 Namespace，不销毁后重装。正式 Docker Desktop 控制面不变。

按照[组件认证边界](../plan8/44-control-metrics-authentication.md)，测试节点显式准备受集群 CA 签发的 Scheduler/Controller Manager 服务端证书及监听，运行时仍使用 ServiceAccount 的 `/metrics` 读取授权。CoreDNS/etcd 的 API Pod Proxy 授权限于 kube-system，不使用 etcd 客户端私钥或匿名许可。原始清单和 LF 规范化 SHA256 保存，外部变化会拒绝覆盖。准备后的控制面重启收敛可能超过两分钟，读取使用短超时并等待实际新参数及 Ready，而不是仅等待旧 Pod Ready。

## 真实仪表盘发现的查询缺口

五项组件目标样本均成功进入正式链路，但 API Server 延迟面板报 `QUERY_BUDGET_EXCEEDED`，后续面板按整板预算跳过。第一层原因是超过 128 条序列后仅使用外部集合子查询，丢失常量 UUID 集合的主键裁剪；实际 API 直方图的序列 UUID 随机分布，进一步暴露现有数值投影不包含 Histogram/Summary 完整载荷的问题。

修正保持原有资源授权、FINAL 元数据选择、来源版本围栏、冲突顺序、毫秒时间边界和原生 PromQL：

- 选中 UUID 使用 ClickHouse 原生 `Array(UUID)` 参数传输，SQL 文本保持有界；二进制外部表继续限定最终成员及优先级。少量来源键同样使用类型化参数，大来源集合仍使用既有外部表。[官方驱动示例](https://github.com/ClickHouse/clickhouse-go/blob/main/examples/clickhouse_api/query_parameters.go)说明参数传输格式。
- 数值投影保留，新增完整的 `argus_histogram_source_time` 投影，按来源、采样时间、序列及摄入键排序，包含经典/指数直方图及 Summary 字段。ClickHouse 选择合适的物理读取路径，原始样本仍是唯一事实来源。
- 租户管理流程补建并同步物化已有样本，两个投影均存在才通过 VerifyTenant。该变更是现有字段契约上的物理访问路径，不增加 PostgreSQL 迁移，不修改原始样本、全局预算或查询授权。
- 物化使用短查询观察实际活动 Part 的投影覆盖率，完成前不标记租户就绪。已有任务断线后接续，已完成投影不重建，避免大表同步读取超过连接的 30 秒超时；等待受调用上下文和十分钟上限约束。
- 日常采集表保留四列，只在失败时展示已有的简短诊断；停止采集后不再显示上次抓取失败，历史样本仍可查询。
- 配置待应用期间每两秒核对状态，稳定后恢复十五秒采样状态刷新；避免服务端已完成启停而页面继续显示上一轮“待应用”。

## 验证要求与证据

隔离回归使用随机分布的 256 条 Histogram 与无关历史记录，验证升级前后结果相同、已有样本物化、严格小预算读取和一万条序列的有界传输。此类受控数据库行只用于查询回归，不算组件采集证明。

正式证明来自 `artifacts/plan9/p9-b09b-20261109-01/`：五类实际目标指标的连续样本、五套默认仪表盘逐面板执行、实际失败/恢复及停采流程、正常登录的桌面中英明暗页面，以及不可变镜像、Rollout 和 PVC 保护核对。具体运行方式见 [B09b 运行包](./runtime/B09b/runbook.md)，最终结果见 [B09b 查看记录](./batches/B09b-review.md)。

最终验收：五类指标持续更新，五套共 30 个面板实际取数；5 项部署后真实浏览器流程与 8 项 Mock 交互通过。Go 全量、隔离 ClickHouse 的完整 Telemetry/QueryEngine、类型、ESLint 与运行包单测通过，健康 20/20，6 个 PVC 绑定一致。先前预算失败、初始化超时和测试定位/输出问题的记录保留；最终成功记录见 [B09b 查看记录](./batches/B09b-review.md)。
