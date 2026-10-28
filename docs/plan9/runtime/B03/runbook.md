# B03 MySQL / PostgreSQL 真实运行包

本批运行 `p9-b03-20261027-01`。B02b 经用户确认后已通过正常停采、卸载和 Compose 回收流程清理。账号、共享后台评估身份、正式平台及历史数据保留。

## 组成与边界

- `collector`：独立 systemd/SSH 主机，通过正常 API 安装 Connector 和 Collector；不是预装身份镜像。继承 B01 的干净主机基础，禁用旧业务单元。
- `mysql-a/mysql-b`：MySQL 8.4 系列，本次实际 8.4.13；各有独立数据卷、随机 root/monitor/app 账号密码。
- `postgres-a/postgres-b`：本次 PostgreSQL 18.6；各有独立数据卷、随机 postgres/monitor/app 密码。
- 全部镜像按本次验证的 image ID 启动，摘要见 `images.lock.json`。只有主机 SSH 映射 `127.0.0.1:22251`；数据库仅对本批网络开放，测试连接显式不启用 TLS，不声称已验收数据库 TLS。
- 共 5 个容器，CPU 上限合计 5、内存上限合计 3584 MiB。主机因 systemd 安装流程使用 privileged/private cgroup；无宿主 Docker socket 或无关磁盘挂载。

业务客户端在采集主机上运行，通过 MySQL/PostgreSQL 协议操作本批 `plan9` 数据库。A/B 分别固定 200/50 行，每 2/7 秒一次有限更新、读取及提交/回滚，最多运行 8 小时。到期只停负载，保留数据库与采集供人工检查；可显式重启负载。正式 Argus 数据库从未作为场景写入目标。

监控账号与业务账号分离：MySQL 为 PROCESS、REPLICATION CLIENT、performance_schema SELECT；PostgreSQL 为 pg_monitor 与 CONNECT。客户端验证监控账号读业务表和建表均被拒绝。密码仅在本机受限 private 目录、容器只读挂载和正常 API 请求中使用，不进入仓库/命令行/报告。

## 启动和正常接入

前提是 `artifacts/plan9/p9-b02b-20261027-01/cleanup.json` 为 `destroyed`。重跑检查容器身份，不清空已部署的数据或重建账号。

```powershell
python docs/plan9/runtime/B03/manage.py prepare
docker build --tag argus-plan9-databases:b03-20261027 --file docs/plan9/runtime/B03/Dockerfile docs/plan9/runtime/B03
python docs/plan9/runtime/B03/manage.py up
python docs/plan9/runtime/B03/manage.py initialize
python docs/plan9/runtime/B03/manage.py workload

$sessionPath = 'artifacts/plan9/reset-20261026/private/enterprise-session.json'
$caPath = 'artifacts/plan9/reset-20261026/trust-bundle.pem'
python docs/plan9/runtime/B03/integration.py onboard --session $sessionPath --ca $caPath
python docs/plan9/runtime/B03/integration.py collectors --session $sessionPath --ca $caPath
python docs/plan9/runtime/B03/integration.py builtins --session $sessionPath --ca $caPath
python docs/plan9/runtime/B03/integration.py targets --session $sessionPath --ca $caPath
python docs/plan9/runtime/B03/integration.py dashboards --session $sessionPath --ca $caPath
python docs/plan9/runtime/B03/integration.py alerts --session $sessionPath --ca $caPath
```

会话过期须通过正常企业登录刷新。`up` 之前镜像需在本机存在；使用已验证锁定镜像，禁止将未核验的新标签称为同次运行。

## 故障与恢复验证

```powershell
python docs/plan9/runtime/B03/integration.py exercise --session $sessionPath --ca $caPath
python docs/plan9/runtime/B03/integration.py credentials --session $sessionPath --ca $caPath
python docs/plan9/runtime/B03/integration.py lifecycle --session $sessionPath --ca $caPath
python docs/plan9/runtime/B03/integration.py verify --session $sessionPath --ca $caPath
```

- 先冻结两条测试告警：连接数 `> 8` 持续 20 秒触发，`<= 4` 持续 20 秒恢复，评估周期 10 秒。
- 每种数据库只对 A 增加 10 个实际业务连接，最长 180 秒，另有 systemd 190 秒上限；B 为健康对照。告警触发后制造 20 秒可释放事务锁，原生查询核对等待，再关闭连接并等待同一事件恢复。
- 对 MySQL-A、PostgreSQL-A 分别通过采集配置预览/确认设置错误监控密码，验证真实失败及其余目标新鲜采集；finally 恢复原始凭据。业务/数据库管理员密码不改变。当前原生数据库 Receiver 在认证失败时不输出目标数据，门户表现为“未知”，随后样本过期；具体认证原因仅出现在 Collector 日志。这一诊断缺口登记为 B03-D01，不能将原先要求“平台明确失败代码”的未通过检查改记为通过。
- 独立停用 MySQL-A，等待配置生效与后台评估暂停，验证其余三实例持续采集，再恢复。
- 不保留压力连接或锁。告警在历史中查看，轻量业务负载继续运行至上限；不手工写告警/指标，不对正式数据库执行 SQL。

默认模板为每类 6 个面板，四实例共 24 次面板查询；MySQL 展示运行时长、线程、缓冲池和操作/行锁，PostgreSQL 展示连接、连接上限、空间、提交/回滚与块读取。数据库的可用指标由原生 Receiver 决定，未采集字段不补值。

真实浏览器测试：`web/apps/enterprise/e2e/plan9-b03-real.spec.ts`。开启 `ARGUS_PLAN9_B03_E2E=1` 并配置正常会话、resource map、B02b retired map 和正式 HTTPS origin，验证中英/明暗、四目标跳转和独立来源、历史告警详情与时间线、1280/1920 桌面宽度。交付状态以 [B03 查看记录](../../batches/B03-review.md) 为准。

## 下一批时清理

仅在用户指定清理/下一批之后执行：

```powershell
python docs/plan9/runtime/B03/retire.py --session $sessionPath --ca $caPath
```

先停止负载并解除连接，再停用本批策略、归档采集及其凭据、正常卸载主机、撤销本批数据权限；确认精确容器 ID/run 标签后清理 5 个容器、5 个卷和网络。保留默认模板、共享身份和产品历史，禁止全局 prune 或清正式数据库。
