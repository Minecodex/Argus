# PostgreSQL 与 HTTP/TCP/证书期限探测

2026-10-19，补充 PostgreSQL 17、HTTP、TCP、TLS 证书期限的原生真实目标验证及四套默认模板；统一入口 resources 两组通过。仍未声明这些组合已完成正式接入、平台五类门禁或发布。

## 真实目标与范围

PostgreSQL 使用独立、带本轮 owner 的 postgres:17 容器和只读 pg_monitor 登录身份；该身份不能创建表或读取本轮业务样本表，仍能获取对应系统统计。密码包含字面 `${env:HOME}`/`$`，经过生产私有配置生成路径交付，没有放入公共配置或遥测数据。

HTTP/TCP 使用本机独立监听端点；验证 200、503、关闭连接及恢复，并核对原生状态/错误。证书探测使用实际本机 TLS 证书，对比 time_left 与证书 NotAfter。上游 tlscheckreceiver 读取证书信息，**不证明证书受信任或 HTTPS 请求成功**；门户目录已明确说明。没有为了通过测试修改 TLS 校验配置。

四条采集使用同一 Collector，保持独立来源/目标身份。验证错误数据库密码后无目标样本、恢复认证、关闭一条 HTTP 不影响 PostgreSQL/TCP/TLS，以及全部关闭后控制运行时仍存活。

## 实测发现与修复

上游 PostgreSQL 0.158.0 在认证/查询失败时仍记录 database.count/table.count 的零值。源码对应无条件记录发现列表长度和 numTables；这样的 0 不能作为数据库已读取的证明。

现通过上游已有 metrics 配置显式关闭 `postgresql.database.count`、`postgresql.table.count`，从本期支持指标中排除；保留真实查询得到的连接、大小、事务和 I/O。没有 fork 或改变 Receiver。错误密码无任何目标点、恢复后重新取数通过；未以过滤失败目标或放宽断言代替修复。

原生测试接收端也修正为空 Resource 块不计 DATA，通过实际数据点判断目标出现，原始消息仍保留供后端验证。HTTP/TCP 的 error 指标只在实际错误时出现，健康场景不要求不存在的错误样本，更不在图表补造 0。

来源：[PostgreSQL Receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.158.0/receiver/postgresqlreceiver)、[HTTP](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.158.0/receiver/httpcheckreceiver)、[TCP](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.158.0/receiver/tcpcheckreceiver)、[证书检查](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.158.0/receiver/tlscheckreceiver)。

## 默认模板

新增 postgresql.overview、http.overview、tcp.overview、tls.overview，当前模板实现共 19 套；受管版本、资源/应用过滤、个人副本和门户实例快捷入口沿用同一机制。

- PostgreSQL 六个面板：连接、上限、库大小、提交/回滚速率、块读取；参考 [9628](https://grafana.com/grafana/dashboards/9628-postgresql-database/)。
- HTTP 四个：耗时、成功类别、客户端/服务端错误类别；TCP 两个：连接状态和耗时；证书两个：最短剩余天数及期限趋势。参考 [7587](https://grafana.com/grafana/dashboards/7587-prometheus-blackbox-exporter/)，查询使用专用原生 Receiver 指标，不需要 blackbox_exporter。

同批真实数据经过正常化、Writer/ClickHouse、PromQL 的 14 个查询均有实际结果；没有使用人工构造遥测、合成时间点或缺指标占位。

## 证据

| 证据 | 结果 |
| --- | --- |
| `artifacts/plan8-postgres-probes-native-final.log` | 四目标、只读、实际协议故障/恢复、认证和生命周期通过 |
| `artifacts/plan8-postgres-probes-template-queries.log` | 14 个实际默认查询通过 |
| `artifacts/plan8-postgres-probes-unit-verified.log` | 适配器、模板、运行时、Workspace、CLI 回归通过 |
| `artifacts/plan8-resource-probes-20261019-716904692/manifest.json` | resources 与 resource-views 两组当前具名验证通过，前后源码/Collector 指纹一致 |

前几轮错误计数、健康时要求 error、测试覆盖控制专用 exporter 的失败日志保留。最后一项仅修复测试传输覆盖，不改变生产 nop 控制配置。自有数据库/Collector/网络随测试回收，测试不操作既有业务数据库。

仍需正式创建/接入、权限全组合、所有支持平台/拓扑、私有 CA/目标 TLS 范围及部署后验收。该记录不把其余服务或同协议不同组合标为支持，也不将组件结果写入正式五类门禁 passed。
