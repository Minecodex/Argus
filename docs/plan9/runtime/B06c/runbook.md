# B06c SQL Server 运行包

本批运行两个独立 SQL Server 2022 CU18 Developer 实例（仅非生产验证）和一个 Debian systemd Collector 主机。A 为负载实例，B 为健康对照；不代表 Always On 或生产高可用验收。

## 运行边界

Compose 仅处理 `p9-b06c-20261029-01` 标签、资源清单中的对象。两数据库各限制 2.5 GiB/1.5 CPU，SQL 内存上限 1536 MiB；Collector 限制 1.25 GiB/1.5 CPU。三容器、三数据卷、一个私有网络，数据库不发布宿主机端口。仅 SSH 绑定 loopback 22321。

SA、monitor、app 密码由运行器随机产生并保存在 ACL 保护的 private 目录。SA 密码通过只读文件挂载，由启动封装传给原厂入口；Compose 环境和命令行不含密码。SQL 登录密码策略启用。业务账号只可操作 `plan9.dbo.samples`，监控账号只有 VIEW ANY DATABASE 和 VIEW SERVER PERFORMANCE STATE，不可查询业务表。

SQL/TDS 在本批私有网络使用无加密连接，本批不验收 SQL TLS。就绪检查使用原厂 sqlcmd 对容器本地连接，`-C` 仅信任该本地自签证书。Argus API/门户始终校验正式 CA。

## 部署

```powershell
python docs/plan9/runtime/B06c/manage.py prepare
docker pull mcr.microsoft.com/mssql/server:2022-CU18-ubuntu-22.04
docker build -t argus-plan9-sqlserver:b06c-20261029 -f docs/plan9/runtime/B06c/Dockerfile docs/plan9/runtime/B06c
docker build -t argus-plan9-sql-server:b06c-20261029 -f docs/plan9/runtime/B06c/Dockerfile.server docs/plan9/runtime/B06c
python docs/plan9/runtime/B06c/manage.py up
python docs/plan9/runtime/B06c/manage.py initialize
python docs/plan9/runtime/B06c/manage.py workload
$sessionPath = 'artifacts/plan9/reset-20261026/private/enterprise-session.json'
$caPath = 'artifacts/plan9/reset-20261026/trust-bundle.pem'
python docs/plan9/runtime/B06c/integration.py onboard --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06c/integration.py collectors --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06c/integration.py builtins --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06c/integration.py targets --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06c/integration.py alerts --session $sessionPath --ca $caPath
```

必须先确认 B06b cleanup 为 destroyed。`up` 使用已核验的实际镜像 ID；正常 API 安装 Connector/Collector、托管监控凭据、绑定目标及默认仪表盘。会话过期通过正式登录更新，不保存平台明文密码。

## 真实场景

A/B 固定 200/80 行；原生 TDS 执行建表、插入、读取、更新、删除、提交及回滚。A 每 2 秒、B 每 7 秒处理既有行，每进程最多 8 小时。到期仅停止低频客户端，服务与采集保留，等待用户反馈。

`client.py lock a` 使用两个真实事务争用同一行，持有锁 20 秒，通过监控账号查询 LCK_M_* 等待后释放、回滚；客户端锁等待上限 30 秒。

| 场景 | 实际动作 | 固定阈值 | 恢复 |
| --- | --- | --- | --- |
| 连接数 | A 新增 12 个真实 TDS 连接 | > 10，恢复 <= 5 | 关闭本批连接 |
| 批请求 | A 连续执行有界 SQL 请求 | > 15 请求/秒，恢复 <= 10 | 停止压力客户端 |

规则只绑定 A；每 10 秒评估，触发/恢复持续 20 秒。场景前记录正常值并冻结阈值。压力最多 180 秒/2000 请求，systemd 另设 190 秒上限；不耗尽磁盘或内存，不设置外部通知接收人。B 保持正常负载和监控作为对照。

```powershell
python docs/plan9/runtime/B06c/integration.py exercise --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06c/integration.py lifecycle --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06c/integration.py verify --session $sessionPath --ca $caPath
```

独立停采 A 时策略暂停评估、B 持续产生新样本，恢复后检查版本、Collector 收敛和评估健康。默认仪表盘六面板：用户连接数、批请求速率、缓冲缓存命中率、页面预期生存时间、锁等待速率、SQL 编译速率。锁定 receiver 的 SQL 查询路径把累计 DMV 计数输出为带 rate 名称的 gauge；默认模板 v2 对批请求、锁等待、SQL 编译统一使用 rate(...[1m])，保留原始指标。页面生存时间只取实例级 Buffer Manager，排除重复的 Buffer Node。

先完成 verify 的真实查询和关联，再运行 `artifacts/plan9/reset-20261026/run_b06c_browser.py`。E2E 覆盖中英、明暗、A/B 资源隔离、查询说明、真实告警趋势/时间线、1280/1920 桌面宽度。

## 清理

用户指定下一批后运行：

```powershell
python docs/plan9/runtime/B06c/retire.py --session $sessionPath --ca $caPath
```

停止本批客户端，经正常 API 收尾策略、采集、角色授权和主机，验证软件卸载后回收本批容器、卷和网络。保留企业、账号、共享定义和历史，不自动进入 B06d。
