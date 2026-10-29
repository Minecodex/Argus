# B06b Elasticsearch 真实场景运行包

运行 `p9-b06b-20261029-01`。B06a 已通过正常 API 停采、卸载、退役和回收；历史、企业账号和共享评估身份保留。

## 组成与启动适配

- 两个独立 Elasticsearch 8.17.0 单节点集群 A/B，使用 Basic 自生成许可证；每节点 JVM 堆 512 MiB。
- 一台正常 SSH 接入的 `B06b-Elasticsearch-Collector`，安装正式 Connector/Collector。A 为场景对象，B 为健康对照。
- 三个容器、三个命名卷、专属网络。CPU 上限合计 3.5，内存上限 4352 MiB；节点关闭 mmap，未修改宿主全局 sysctl。
- 只发布 SSH `127.0.0.1:22311`，Elasticsearch HTTP/transport 端口不向宿主发布。本批使用专网明文 HTTP，不包含 TLS、多节点容灾或性能容量认证。

原官方仓库下载较慢，本次改用 Docker Hub 官方 `elasticsearch:8.17.0`，实际服务版本已核验；并不声称两处镜像摘要相同。版本、上游摘要与本批衍生镜像均记录在 `images.lock.json`。

Windows 文件绑定在容器内显示 777 权限，标准入口拒绝读取该密码文件。本批启动封装以原来的 UID 1000 将输入复制到私有 tmpfs，设为 600 后调用原厂入口；没有放宽密码文件检查，也未将密码放进参数/镜像。运行时权限和 UID 已实际检查。

账号分别为 elastic（仅初始化）、monitor（集群/指定索引统计）、app（仅 `p9-*` 读写）、operator（仅 `p9-*` 索引管理）。实测 monitor 不能搜索业务文档或创建索引，app 不能读取安全管理用户接口。平台仅托管 monitor 凭据。

## 准备、协议初始化及接入

```powershell
python docs/plan9/runtime/B06b/manage.py prepare
docker pull elasticsearch:8.17.0
docker build --tag argus-plan9-elastic-server:b06b-20261029 --file docs/plan9/runtime/B06b/Dockerfile.server docs/plan9/runtime/B06b
docker build --tag argus-plan9-elasticsearch:b06b-20261029 --file docs/plan9/runtime/B06b/Dockerfile docs/plan9/runtime/B06b
python docs/plan9/runtime/B06b/manage.py up
python docs/plan9/runtime/B06b/manage.py initialize
python docs/plan9/runtime/B06b/manage.py workload

$sessionPath = 'artifacts/plan9/reset-20261026/private/enterprise-session.json'
$caPath = 'artifacts/plan9/reset-20261026/trust-bundle.pem'
python docs/plan9/runtime/B06b/integration.py onboard --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06b/integration.py collectors --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06b/integration.py builtins --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06b/integration.py targets --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06b/integration.py alerts --session $sessionPath --ca $caPath
```

`up` 要求 B06a 的 `cleanup.json` 为 `destroyed`，采用实际 image ID，重跑检查既有身份/容器 ID。正常企业会话过期时重新登录，不把平台密码写入脚本。B06a 首次归档中出现过 MONITORING_UNAVAILABLE，核对状态后按检查点继续正常流程完成；未改数据库或绕过版本保护。

两集群各有 `p9-business` 索引，1 主分片、初始 0 副本，A 200 条、B 80 条固定文档。初始化通过真实 Bulk API 写入（最多四并发），并实际执行 get、update、delete、全文/关键词检索。正常负载 A 每 2 秒、B 每 7 秒更新既有文档并检索，每进程最多 8 小时。

## 场景、告警与恢复

```powershell
python docs/plan9/runtime/B06b/integration.py exercise --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06b/integration.py lifecycle --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06b/integration.py verify --session $sessionPath --ca $caPath
```

阈值在场景前冻结，只针对 A，每 10 秒评估，触发/恢复持续 20 秒：

| 场景 | 实际动作 | 告警 | 恢复 |
| --- | --- | --- | --- |
| 单节点 yellow | operator 将索引副本数从 0 改为 1，实际出现 1 个未分配副本 | yellow/red 指示值 > 0.5，恢复为 0 | 改回 0 副本，实际恢复 green |
| 文档增多 | 四并发 Bulk 写入额外 200 条本批文档，A 从 200 到 400 | 主分片活动文档数 > 300，恢复 <= 210 | 仅删除 `pressure-*` 测试 ID 并刷新，恢复 200 |

yellow 是单节点无法分配副本的真实结果，不表示本批主分片数据丢失；期间仍验证文档可检索。B 的 80 条文档和 green 状态保持正常。压力最长 180 秒，systemd 另设 190 秒上限；finally 恢复设置或删除压力文档。没有配置外部通知接收人，也不通过耗尽内存/磁盘来强行触发告警。

独立停采 A 时策略暂停评估、B 继续有新样本；恢复后检查生效版本、样本和评估。短暂查询健康事件如有发生，按实际结果保留。

## 默认图的口径

Elasticsearch 模板 v2 共七面板：三色健康指示、节点数、活动文档数、JVM 堆、可用磁盘、索引操作速率、未分配分片数。原生健康值为 green/yellow/red 三个 0/1 序列，值 1 表示当前状态。

索引文档/操作指标原生同时提供 primary_shards/total 和 `_all` 汇总。默认图选择主分片和具体索引，避免把副本、汇总与逐索引数据重复计入；原始指标保留。活动文档数不包含 Lucene 尚未合并清除的 deleted 文档。

每实例七面板，两实例共 14 次验证。真实门户 E2E 为 `web/apps/enterprise/e2e/plan9-b06b-real.spec.ts`，先完成 `verify` 的图表关联，再验证中英/明暗、A/B 范围、实际状态、历史告警/时间线及 1280/1920 桌面宽度。

## 清理

用户指定下一批后执行：

```powershell
python docs/plan9/runtime/B06b/retire.py --session $sessionPath --ca $caPath
```

停止压力和低频客户端，恢复 0 副本并删除本批压力文档；经正常 API 收尾策略、采集、凭据、授权和主机，再核对准确 ID/run 标签，回收本批容器、卷和网络。账号、共享定义、历史保留，不自动进入 B06c。
