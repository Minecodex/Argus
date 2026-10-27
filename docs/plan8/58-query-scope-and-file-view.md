# 来源集合传输与 Linux 文件观测视图

2026-10-21，已实现并有实际组件证据；完整部署、完整 A62 与统一发布仍待验收。

## 来源集合按数据传输

既定上限允许一次查询包含 10,000 个来源选择器。锁定的 ClickHouse 驱动会把普通 `IN (?)` 数组展开到 SQL；实际探针在该规模返回 code 62（查询文本超限）。不调整 ClickHouse 查询大小、扫描、结果、时间或来源上限。

新增共享 `queryengine/chscope`：通过驱动的二进制 external table 传入 `selected_key`、`base_key` 和 Namespace facet 标志。PromQL、KQL、Trace、目录与概览保留原先各自的精确来源、基础来源或 Namespace 行过滤语义；过滤仍先于聚合、目录发现及跨信号计数。PromQL 的序列 ID 二进制选择表继续独立存在。

每次查询的表只属于该请求，保留既有 ClickHouse 设置、进度统计和上下文取消。无用户 SQL 拼接；列表达式由适配器固定。空表不匹配任何来源，重复选择不增加计数。统一范围解析、权限、关系版本、快照与缓存签名没有改变。

实际测试覆盖 10,000 个来源下的三信号、目录、概览及 Namespace 聚合；小量原场景继续执行。另以实际 ClickHouse 比较空集合、全来源、facet、错误 UID、混合、重复与字符串输入，结果与原范围规则一致。候选完整遥测回归及源码合入后的回归分别保存，不把候选通过称为正式部署通过。

证据：`artifacts/plan8-source-cardinality-probe.json`、`plan8-source-scope-transport-all.log`、`plan8-source-scope-candidate-final.log`。实现使用锁定 [clickhouse-go v2.40.1 的 external data API](https://github.com/ClickHouse/clickhouse-go/blob/v2.40.1/examples/clickhouse_api/external_data.go)。

## 文件日志与 systemd 视图一致

r24 正式接入的文件日志配置已经生效，但 `/tmp` 下真实日志无样本。生成服务始终使用 `PrivateTmp=true`，使采集器看到单独的临时目录。独立 systemd 容器已实际复现该可见性差异。

生成的 Linux 服务现在按生效 Bundle 判断：有启用的原生 filelog 来源时，使用 `PrivateTmp=false`，让声明的主机 `/tmp`、`/var/tmp` 路径保持原视图；其他来源保持私有临时目录。内部临时文件统一使用 `RuntimeDirectory=argus-otelcol-tmp`、0700 模式及对应 `TMPDIR`。`ProtectSystem=strict`、`ProtectHome` 和其余服务防护继续执行，Argus 身份/凭据目录仍由日志路径规则排除。文件日志停用后，服务重建恢复私有临时目录，checkpoint 与持久队列不位于临时目录。

这调整的是受管 Collector 的 mount namespace，不增加平台角色权限。实际生成单元测试验证：文件来源停用时看不到主机临时日志；启用后能够读取；主机临时目录写入仍被拒绝；内部 RuntimeDirectory 可写且为 0700。配置失败仍由既有事务恢复原服务单元。完整 native Collector → 平台文件轮转/checkpoint/查询链路继续由正式阶段验收。

最小系统镜像不必提供 system D-Bus。测试 Journald 的驱动改为写入本轮拥有的 oneshot 单元并经 `systemctl` 启动，产生实际 `_SYSTEMD_UNIT` 日志；不再额外要求 `systemd-run` 的系统总线。采集仍使用允许的系统 `journalctl`。

证据：`artifacts/plan8-systemd-filesystem-probe.json`、`plan8-systemd-generated-file-view.json`。行为依据 [systemd.exec](https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html#PrivateTmp=)；正式数据链路未通过前不把上述文件视图检查算作日志验收完成。
