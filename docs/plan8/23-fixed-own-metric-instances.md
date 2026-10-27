# 自身指标的固定采集实例

状态：四项基础能力已接入领域、配置编译、API、门户和默认模板框架；T09 尚未完成，不代表所有内置能力通过或已发布。

| 实例 | 权威资源 | 执行位置 | 来源 |
| --- | --- | --- | --- |
| host_metrics | Host | 本机 Collector | hostmetrics |
| collector_metrics | Host / Cluster | 本机或 Agent/Gateway 各自 | collector_self |
| kubernetes_cluster_metrics | Cluster | 单个 Gateway | k8s_cluster |
| kubernetes_node_metrics | Cluster | 每个 Agent | kubeletstats |

固定实例使用“资源类型＋权威资源 UUID＋能力 key”的稳定身份，不创建平行监控目标，不选择其他主机执行，不接收应用/环境或凭据信息。每项保留独立期望状态、配置/生效版本、健康及样本新鲜度。关闭全部指标仍保留 Collector 控制运行。

主机和 K8s 已移除原有大开关配置区，改用共享实例表格及逐项 PendingAction 预览确认。业务日志和应用接入保留独立规则/Endpoint，不再放在 Host 自身能力开关里。空 Profile 不再阻止路由或生命周期预览。

确认安装/配置时，旧固定指标 Profile 在同一事务内物化为实际采集记录并移出 Profile ID/渲染列表，避免双采；已有独立实例的显式关闭不会被物化逻辑重新开启，配置应用成功也不伪造健康或样本。已转换实例不能通过旧 Profile API 重新启用。未触发新配置的历史安装仍需 T16 完整迁移并验证，不能在正式发布前遗漏。

Host 文件系统采集纳入实际 overlay/虚拟文件系统，同时排除 proc/sysfs 等伪文件系统，修复容器目标缺少文件系统用量的问题。采集范围仍是该运行环境，不将容器结果冒充物理主机/Windows 的实测。

## 验证与默认仪表盘

- [领域 PostgreSQL](../../artifacts/plan8-builtin-own-instance-postgres-current.log)：稳定身份、独立启停、版本冲突、资源授权、不能选择其他执行者、不创建监控目标。
- [旧 Profile 物化](../../artifacts/plan8-own-metric-materialization-postgres.log)：重复执行不产生多实例，保留显式 off，不生成虚假生效/样本。
- [实际 Collector 配置](../../artifacts/plan8-fixed-metrics-native-config.log)：Host、K8s Agent/Gateway 放置和原生组件加载；属于配置门禁。
- [实际 CPU/内存/文件系统/网络及自监控](../../artifacts/plan8-own-metric-native-capture-final.log)：独立 source/collection，Host 关闭后自监控继续，全关闭无隐式指标且进程仍运行。
- [真实 Writer / ClickHouse 的 12 组模板查询](../../artifacts/plan8-own-default-native-query.log)：Host、Collector 默认内容全部取得原生样本，无关来源为空。
- [中英明暗四场景](../../artifacts/plan8-own-default-portal-e2e.log)：Host/K8s 自身开关、相互独立、默认仪表盘预选该资源、个人副本入口和可访问性；使用 mock API。

Host 默认模板参考 Grafana 1860，Collector 参考 12553，查询已按当前 Receiver 真实指标改写。默认入口仅在当前用户有相应权威资源权限且配置过该能力时提供，停采后仍可查保留期历史；用户副本继续独立。K8s 组件默认内容、进程/系统服务、控制面组件、Node/Host Claim、所有平台的正式安装/迁移/运维链路仍待完成。
