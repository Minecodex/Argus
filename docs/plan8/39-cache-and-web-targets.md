# Memcached、Apache 与 HAProxy 原生目标

2026-10-19，新增三个真实目标及默认面板的组件验收。统一 cache-web 入口同时执行实际目标和同批 Writer/ClickHouse 查询，源码/Collector 指纹一致。仍不表示正式注册、部署后平台五类门禁或全计划完成。

| 目标 | 实际范围 | 默认模板 |
| --- | --- | --- |
| Memcached | 官方 memcached:1.6-alpine，本轮独立缓存，内置 memcachedreceiver 0.158.0 | 内存、连接、条目、淘汰、命令和网络；参考 [11527](https://grafana.com/grafana/dashboards/11527-memcached-overview/) |
| Apache | 官方 httpd:2.4-alpine，既有 mod_status + ExtendedStatus，独立测试配置 | uptime、连接、Worker、请求、流量和 scoreboard；参考 [3894](https://grafana.com/grafana/dashboards/3894-apache/) |
| HAProxy | 官方 haproxy:3.2-alpine，独立 CSV stats Basic Auth，只读 stats，实际转发到本轮 Apache | 输入/输出、会话、请求、错误和排队，保留原 Frontend/Backend/Server 属性；参考 [12693](https://grafana.com/grafana/dashboards/12693-haproxy/) |

配置和采样使用已经内置的专用 Receiver，没有独立 exporter。测试期间只在自有服务配置启用只读统计端点，不修改现有业务服务。

验证同一 Collector 的三个独立目标、实际 HTTP 流量、HAProxy 错误密码无目标样本及恢复、关闭 Memcached 不影响其余目标、全部关闭后控制运行时存活。公共配置和原始遥测不承担认证输入；认证仍走现有私有配置生成路径。

新增 memcached.overview、apache.overview、haproxy.overview，当前默认模板实现共 22 套。18 个面板使用同批真实数据，通过正常化、Writer/ClickHouse 及正式 PromQL 执行；没有生成缺失值或伪造时间点。

证据：

- `artifacts/plan8-cache-web-native.log`：三个原生目标、认证和独立生命周期通过。
- `artifacts/plan8-cache-web-template-queries.log`：18 个实际查询通过。
- `artifacts/plan8-cache-web-20261019-3092518491/manifest.json`：两组具名验证通过，源码/Collector 指纹相同。

Docker 镜像在独立配置/owner 下运行，容器、匿名卷和网络由测试按归属回收。Memcached 当前组合没有 SASL/TLS 验收，Apache 当前只验证本轮统计端点，HAProxy 仅本轮 standalone CSV/Basic Auth；其他认证、平台、版本和拓扑不能由此扩大声明。正式五类门禁保持待完成。
