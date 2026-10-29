# B06a MongoDB 真实场景运行包

运行 `p9-b06a-20261029-01`。B05c 已按用户指令通过正常 API 停采、卸载、退役和回收，企业账号、共享评估身份和历史保留。

## 环境与权限

- 两个独立 MongoDB 7.0.16 standalone 实例 A/B，采用仓库已验证组合；实际镜像 ID/摘要见 `images.lock.json`。不声明副本集、分片、多文档事务或 TLS 验收。
- 一台正常 SSH 接入的 `B06a-MongoDB-Collector`，正式安装 Connector/Collector。业务客户端为 PyMongo 4.10.1。
- 每实例有独立随机 bootstrap、monitor、app 账号。monitor 仅 `clusterMonitor`；app 仅测试数据库 `plan9` 的 `readWrite`。
- 实测 monitor 不能读取或插入业务文档，app 不能读取管理员用户数据；Collector 接收 monitor 凭据，通过标准加密暂存、预览和确认流程配置。
- 三个容器、五个命名卷、一个专网；CPU 上限合计 3.5，内存上限 2816 MiB。每个 MongoDB 的 WiredTiger cache 为 0.25 GiB，连接数上限 128。只发布 SSH `127.0.0.1:22301`，MongoDB 端口不向宿主公开。

主机使用 systemd/private cgroup，无 Docker socket 和无关宿主目录挂载。凭据仅在受限 private 文件及主机 root-only 目录中，不进入镜像、命令行或报告。所有业务数据通过 MongoDB 协议写入本批数据库；不修改 Argus 业务库或直接写监控数据。

## 初始化和接入

```powershell
python docs/plan9/runtime/B06a/manage.py prepare
docker build --tag argus-plan9-mongodb:b06a-20261029 --file docs/plan9/runtime/B06a/Dockerfile docs/plan9/runtime/B06a
python docs/plan9/runtime/B06a/manage.py up
python docs/plan9/runtime/B06a/manage.py initialize
python docs/plan9/runtime/B06a/manage.py workload

$sessionPath = 'artifacts/plan9/reset-20261026/private/enterprise-session.json'
$caPath = 'artifacts/plan9/reset-20261026/trust-bundle.pem'
python docs/plan9/runtime/B06a/integration.py onboard --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06a/integration.py collectors --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06a/integration.py builtins --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06a/integration.py targets --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06a/integration.py alerts --session $sessionPath --ca $caPath
```

启动前必须有已核验的基础与 MongoDB 镜像，B05c 的 `cleanup.json` 必须为 `destroyed`。已有容器按准确 ID/归属核对，不重置注册身份。会话过期通过正常企业登录刷新。

初始化真正执行 insert、find、update、delete，并保留 A 200 条、B 80 条固定文档及索引。正常负载对原有文档执行更新和读取：A 每 2 秒、B 每 7 秒，每进程最多 8 小时。文档 payload 有界，不不断追加文档。

## 告警与恢复

```powershell
python docs/plan9/runtime/B06a/integration.py exercise --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06a/integration.py lifecycle --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06a/integration.py verify --session $sessionPath --ca $caPath
```

阈值在故障前按真实基线冻结，只作用 A；B 是健康对照。

| 场景 | 真实操作 | 告警 | 恢复 |
| --- | --- | --- | --- |
| 连接增加 | 打开 12 个独立 MongoClient，执行认证、ping、读取并保持连接 | current connections > 20；恢复 <= 12 | 关闭这些客户端及其驱动连接池 |
| 更新操作增多 | 使用 app 对既有文档执行 updateOne，最多 2000 次，每次间隔至少 50 ms | 一分钟 update rate > 5 ops/s；恢复 <= 1 ops/s | 停止压力，等待真实速率窗口自然回落 |

每 10 秒评估，触发/恢复持续 20 秒；压力最长 180 秒，并由 systemd 190 秒上限兜底。没有降低已经冻结的阈值以取得通过结果。客户端对象不等于单条 TCP 连接，PyMongo 还有拓扑监测连接，所以实际连接数以 serverStatus/Collector 观测为准。

操作告警使用一分钟窗口；默认操作和缓存速率图使用五分钟窗口，告警详情趋势与规则同窗口。恢复等待计数速率自然衰减，不清累计计数或写零值。压力结束再次核验固定文档数和权限边界。

独立停采 A 时，两条策略暂停评估，B 继续取得新样本；恢复后核验配置版本、样本和评估。业务客户端继续真实运行。`no_data_behavior=report` 的短暂查询健康事件如有发生，按实际结果保留。

## 共享默认仪表盘

MongoDB 模板 v2 有七个面板：连接数、进程内存、数据量、存储量、操作速率、网络接收速率、缓存操作速率。缓存 hit/miss 计数来自原生 Receiver。

本次观察到 `mongodb_connection_count` 随数据库维度重复返回同一实例的 serverStatus 连接值。默认图用 `max by (argus_resource_id, type)` 按实例和连接类型去重，保留原始指标，不把各数据库副本相加。数据库数据量/存储量仍保留数据库维度；类型和单位不混用。

真实门户 E2E：`web/apps/enterprise/e2e/plan9-b06a-real.spec.ts`。先完成 `verify` 的默认图关联，再运行浏览器检查；覆盖 A/B 范围、七类面板、中英/明暗、历史告警及时间线、1280/1920 桌面宽度。最终入口见 [B06a 查看记录](../../batches/B06a-review.md)。

## 清理

用户指定下一批/清理后执行：

```powershell
python docs/plan9/runtime/B06a/retire.py --session $sessionPath --ca $caPath
```

停止压力和业务进程，通过正常 API 收尾策略、采集、凭据、授权及主机卸载，再按准确 ID/run 标签回收本批三个容器、五个卷和网络。企业账号、默认模板、共享评估身份和历史保留；不自动进入 B06b。
