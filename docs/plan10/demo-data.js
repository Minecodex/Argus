/* Deliberate fixtures for this local prototype. Nothing here is live telemetry. */
window.ARGUS_DEMO = {
  resources: [
    {id:'demo-host-b04',kind:'host',name:'B04-Cache-Collector',environment:'test',parent:'—',series:{host:['B04-Cache-Collector']}},
    {id:'demo-host-b05',kind:'host',name:'B05-Web-Collector',environment:'test',parent:'—',series:{host:['B05-Web-Collector']}},
    {id:'demo-cluster',kind:'kubernetes_cluster',name:'dev-cluster',environment:'test',parent:'—',series:{k8s:['checkout-api-6f8c','inventory-api-7b2d']},services:['checkout-api']},
    {id:'demo-namespace',kind:'kubernetes_namespace',name:'checkout',environment:'test',parent:'dev-cluster',series:{k8s:['checkout-api-6f8c']},services:['checkout-api']},
    {id:'demo-application',kind:'application',name:'checkout-app',environment:'test',parent:'—',series:{k8s:['checkout-api-6f8c']},services:['checkout-api']},
    {id:'demo-redis',kind:'monitoring_target',name:'redis-cache',environment:'test',parent:'B04-Cache-Collector',series:{redis:['redis-cache']}}
  ],
  metrics: [
    { name: 'system_cpu_utilization', title: ['CPU 使用率', 'CPU utilization'], group: 'host', source: 'hostmetrics', type: 'Gauge', unit: 'ratio', observed: true, labels: ['state', 'cpu', 'argus_resource_id'], desc: ['各 CPU 状态的时间占比，原始值为 0–1。使用 idle 的补集计算忙碌率，再按资源聚合；不要将所有 state 相加当作使用率。', 'Time fraction per CPU state, from 0 to 1. Compute busy time as the complement of idle, then aggregate by resource. Do not sum all states as utilization.'], query: '1 - avg by (argus_resource_id) (system_cpu_utilization{state="idle"})', recipe: ['按主机查看 CPU 忙碌率', 'CPU busy ratio by host'], caution: ['展示为百分比时乘 100；原始结果仍为比例。', 'Multiply by 100 for percentage display; the query returns a ratio.'] },
    { name: 'system_memory_utilization', title: ['内存使用率', 'Memory utilization'], group: 'host', source: 'hostmetrics', type: 'Gauge', unit: 'ratio', observed: true, labels: ['state', 'argus_resource_id'], desc: ['主机各内存状态占比。筛选 used，保留资源维度，不把不同主机的比例直接相加。', 'Memory fraction by state. Select used and retain resource identity; do not sum utilization ratios across hosts.'], query: 'system_memory_utilization{state="used"}', recipe: ['查看已用内存占比', 'Used memory ratio'], caution: ['不同操作系统对 cached / buffered 的统计口径可能不同。', 'Cached and buffered memory semantics can differ by operating system.'] },
    { name: 'system_network_io', title: ['网络收发字节', 'Network I/O bytes'], group: 'host', source: 'hostmetrics', type: 'Counter', unit: 'bytes', observed: true, labels: ['device', 'direction', 'argus_resource_id'], desc: ['网卡累计接收或发送的字节数。用 rate 观察每秒吞吐，保留方向和网卡标签。', 'Cumulative bytes received or transmitted. Use rate for throughput; retain direction and device labels.'], query: 'sum by (argus_resource_id, direction) (rate(system_network_io[5m]))', recipe: ['最近 5 分钟平均吞吐', 'Average throughput over 5 minutes'], caution: ['Counter 会重置；先 rate 再聚合，窗口至少覆盖多个采样点。', 'Counters can reset. Apply rate before aggregation and use a window covering multiple samples.'] },
    { name: 'k8s_pod_memory_usage', title: ['Pod 内存用量', 'Pod memory usage'], group: 'k8s', source: 'kubeletstats', type: 'Gauge', unit: 'bytes', observed: true, labels: ['k8s_namespace_name', 'k8s_pod_name', 'argus_resource_id'], desc: ['Pod 的已观测内存用量。维度以实际字段目录为准，Namespace 数据范围由后端授权裁剪。', 'Observed memory use per Pod. Dimensions come from the actual field catalog; namespace scope is enforced by the backend.'], query: 'k8s_pod_memory_usage', recipe: ['查看各 Pod 内存用量', 'Memory use per Pod'], caution: ['目录和补全中的标签仅表示可筛选维度，不授予 Namespace 权限。', 'Catalog labels are filtering dimensions and never grant namespace access.'] },
    { name: 'container_cpu_usage', title: ['容器 CPU 用量', 'Container CPU usage'], group: 'k8s', source: 'kubeletstats', type: 'Gauge', unit: 'cores', observed: true, labels: ['k8s_namespace_name', 'container_name', 'argus_resource_id'], desc: ['容器正在使用的 CPU 核数。该指标为 Gauge，不应套用 Counter 的 rate 模板。', 'CPU cores in use by a container. This is a gauge; do not apply a counter rate recipe.'], query: 'container_cpu_usage', recipe: ['查看容器 CPU 核数', 'CPU cores per container'], caution: ['1 core 表示一个逻辑核；不能不带配额分母就标成百分比。', '1 core means one logical core. A percentage requires a quota denominator.'] },
    { name: 'redis_memory_used', title: ['Redis 已用内存', 'Redis used memory'], group: 'redis', source: 'redis', type: 'Gauge', unit: 'bytes', observed: true, labels: ['argus_resource_id', 'argus_source_id'], desc: ['Redis 进程使用的内存字节数。资源和采集来源需要同时可追溯。', 'Bytes of memory used by Redis. Both resource and collection source remain traceable.'], query: 'redis_memory_used', recipe: ['查看 Redis 内存用量', 'Redis memory usage'], caution: ['字节值不等于内存使用率；没有有效 maxmemory 时不生成百分比。', 'Bytes do not imply utilization. Do not calculate a ratio without a valid maxmemory.'] },
    { name: 'redis_memory_rss', title: ['Redis 常驻内存', 'Redis resident memory'], group: 'redis', source: 'redis', type: 'Gauge', unit: 'bytes', observed: false, labels: ['argus_resource_id'], desc: ['集成定义中的常驻内存指标。本例当前时间范围未观测到样本。', 'Resident memory defined by the integration. No sample is observed in this example window.'], query: 'redis_memory_rss', recipe: ['查看常驻内存', 'Resident memory'], caution: ['支持采集不等于已采到；无样本显示为空，不能补成 0。', 'Supported does not mean collected. Missing samples stay empty, never zero.'] }
  ],
  functions: [
    { name: 'rate', insert: 'rate(', desc: ['Counter 每秒平均增长率', 'Average per-second counter rate'] },
    { name: 'sum', insert: 'sum(', desc: ['对序列求和，可添加 by 分组', 'Sum series; optionally group with by'] },
    { name: 'avg', insert: 'avg(', desc: ['对序列求平均值', 'Average across series'] },
    { name: 'histogram_quantile', insert: 'histogram_quantile(0.95, ', desc: ['计算直方图分位数', 'Calculate a histogram quantile'] },
    { name: 'increase', insert: 'increase(', desc: ['Counter 在窗口中的增长量', 'Counter increase over a window'] },
    { name: 'max', insert: 'max(', desc: ['按维度取最大值', 'Maximum across series'] }
  ],
  logs: [
    { time: '10:58:42.136', level: 'ERROR', service: 'checkout-api', body: 'Redis request timed out after 3000ms', trace: 'a8e5c090d2b41f76' },
    { time: '10:57:18.502', level: 'WARN', service: 'checkout-api', body: 'Retrying inventory request (attempt 2)', trace: 'b702d15a00ec192f' },
    { time: '10:56:09.774', level: 'INFO', service: 'checkout-api', body: 'Order created successfully', trace: 'c491af06bd023e52' }
  ],
  traceQuery: '{ queryTraces(serviceName: "checkout-api", pageNum: 1, pageSize: 20) { total traces { traceId rootService duration status } } }',
  logQuery: 'service_name:"checkout-api"',
  logFields: ['service_name', 'severity_text', 'body', 'trace_id'],
  traceFields: ['queryTraces', 'queryTrace', 'serviceName', 'durationMin', 'traceId', 'rootService', 'duration', 'status']
};
