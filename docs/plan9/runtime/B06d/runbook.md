# B06d Oracle 运行包

本批为两个独立 Oracle Database Free 23.5 实例和一台 Debian systemd Collector，运行 ID `p9-b06d-20261029-01`。A 执行压力，B 作健康对照；每实例服务为 FREEPDB1，独立 PLAN9_APP schema 固定 200/80 行。该组合不等于 RAC、Data Guard、生产容量或 TLS 验收。

## 已知范围与预算

沿用 Plan8 已验证的 23.5/Collector 0.159 组合；31/41/42 字节密码组合仍未支持。本批随机产生 29 字节 ASCII 密码，完整传给客户端和原生 Receiver，不截断、不替换用户密码。[镜像维护者](https://github.com/gvenzl/oci-oracle-free)已将 23.5 列为不再更新版本，本批只用于隔离环境复验已有支持组合，不作为生产版本建议。

每数据库 1.5 CPU/2560 MiB，Collector 1.5 CPU/1280 MiB，总上限 4.5 CPU/6400 MiB。三容器、三卷、一个私有网络；Oracle 1521 不发布到宿主机，SSH 仅绑定 127.0.0.1:22331。

root/monitor/app 密码由 prepare 随机生成，存入 ACL 保护的 private 目录。原厂 ORACLE_PASSWORD_FILE 读取只读文件，Compose 和命令行不含密码。SQLPlus 管理命令通过 stdin；业务及监控凭据经正常平台 API 托管。Oracle 使用本批私有网络的普通连接，Argus API/门户校验正式 CA。

## 部署

```powershell
python docs/plan9/runtime/B06d/manage.py prepare
docker pull gvenzl/oracle-free:23.5-slim-faststart
docker build -t argus-plan9-oracle:b06d-20261029 -f docs/plan9/runtime/B06d/Dockerfile docs/plan9/runtime/B06d
python docs/plan9/runtime/B06d/manage.py up
python docs/plan9/runtime/B06d/manage.py initialize
python docs/plan9/runtime/B06d/manage.py workload
$sessionPath = 'artifacts/plan9/reset-20261026/private/enterprise-session.json'
$caPath = 'artifacts/plan9/reset-20261026/trust-bundle.pem'
python docs/plan9/runtime/B06d/integration.py onboard --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06d/integration.py collectors --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06d/integration.py builtins --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06d/integration.py targets --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06d/integration.py alerts --session $sessionPath --ca $caPath
```

up 先核验 B06c 已清理，使用已锁定实际镜像 ID。initialize 使用容器内 SQLPlus 正常创建 schema/表及最小授权；随后 python-oracledb thin 客户端执行真实插入、查询、更新、删除、提交和回滚，提交结果由第二个会话读取验证。

PLAN9_APP 只有 CREATE SESSION 和自己表的所有者权限，表由初始化管理员创建，schema 限额 16 MiB；没有 CREATE TABLE 权限。MONITOR 只有 CREATE SESSION 和原生 Receiver 必需的 13 个 SYS 视图 SELECT，不能读取业务表或创建表。验证实际返回 ORA-00942/ORA-01031，不用 DBA 或 SELECT ANY DICTIONARY 代替最小授权。

初次配置曾遇到审计 SQLSTATE 40001，Collector 停留旧版本；经正常 preview-repair/confirm 恢复，并重新核验新鲜样本。原始诊断与发送缺口记录保留，未写 Argus 数据库、未声称修复底层审计竞争。重试先查当前实际状态，不重复创建已存在对象。

## 有界场景

正常客户端 A 每 2 秒、B 每 7 秒更新既有行；每五轮执行一次回滚，其他轮提交，最多 8 小时。到期只停止负载，服务和采集继续保留。

| 场景 | 真实操作 | 固定告警与恢复 |
| --- | --- | --- |
| 用户会话 | A 新增 12 个原生连接，每 5 秒查询 DUAL，结束时关闭 | USER 会话总数 > 10，恢复 <= 6 |
| 提交速率 | A 对专用行执行 UPDATE+COMMIT，最多 2000 次 | 原生累计 user_commits 的 rate[1m] > 5 次/秒，恢复 <= 2 |

两策略只绑定 A，每 10 秒评估、触发/恢复持续 20 秒，无外部通知收件人。压力最长 180 秒，另有 systemd 190 秒上限；finally 关闭连接。B 保持正常事务和新样本作对照。告警阈值在执行前冻结，不通过直接改指标或降低阈值冒充触发。

```powershell
python docs/plan9/runtime/B06d/integration.py exercise --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06d/integration.py lifecycle --session $sessionPath --ca $caPath
python docs/plan9/runtime/B06d/integration.py verify --session $sessionPath --ca $caPath
python artifacts/plan9/reset-20261026/run_b06d_browser.py
```

verify 完成仪表盘绑定后才启动浏览器。默认每实例六面板：会话、CPU、PGA、表空间用量、表空间上限、提交速率。会话区分 USER/BACKGROUND 与 ACTIVE/INACTIVE。表空间上限来自原生配置/自动扩展上限，不等于宿主机可用磁盘或 Free 版容量限制。

独立停采 A 时策略暂停，B 保持新样本；恢复后等配置生效、Collector 收敛、查询与评估正常。原生计数差分使用客户端单调时钟，核对实际提交率。真实门户 E2E 覆盖中英/明暗、目标隔离、六面板、告警历史、1280/1920 桌面宽度，并用保留的 B06c 历史事件验证 no_data 展示修正。

## 清理

用户指定下一批后执行：

```powershell
python docs/plan9/runtime/B06d/retire.py --session $sessionPath --ca $caPath
```

先停止压力及正常客户端，经正常 API 收尾策略、采集、授权、凭据、主机卸载和退役，再核对 ID/run 标签删除本批容器、卷和网络。企业、账号、共享定义、业务存储和历史保留，不自动进入 B07a。
