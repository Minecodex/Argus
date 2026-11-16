# B11c 隧道、轮换和恢复运行包

运行：`p9-b11c-20261115-01`。B11b 已在账号保留重部署时退役；本批从空业务基线创建两台新主机。B12 真实模型聊天留待下一批，未使用模拟 Provider。

## 实际路由与预算

```text
Argus Direct Executor ──SSH──> B11c-Tunnel-Bastion
       控制回环 18443/19443       │ 私网 SSH + TLS Relay 8445/9445
       数据回环 14317/14318       ▼
                         B11c-Private-Member
成员 Leaf ──bastion_tunnel──> Root Edge Gateway ──executor_tunnel──> Argus Ingest
```

- Root：`direct_install_tunnel`，控制 `executor_tunnel`；采集 `edge_gateway/direct_argus/executor_tunnel`。
- Member：控制 `bastion_relay`；采集 `leaf/bastion_gateway/bastion_tunnel`。成员只有 `internal=true` 私网，未映射宿主端口。
- Root 在自己的容器网络命名空间拒绝直达平台的新出站连接；保留回环、已建立流及成员 SSH。Member 另拒绝直达 Root 的 OTLP 4317/4318。先用同一平台 IP/端口正向验证，再执行真实负向套接字检查，不靠错误 DNS 演示隔离。
- 两台真实 systemd/OpenSSH 容器，共享 Docker VM 内核；CPU/内存硬件口径不代表两台独立物理机。合计 3 CPU、2560 MiB、每台 512 PID，日志有界。
- `plan9-b11c.service` 监听 18083；每 15 秒真实 HTTP 请求，最长 8 小时，64 MiB/5% CPU。到期只停负载，主机保留。
- Linux amd64 Collector 使用已验证签名制品 `0.3.3-b11a-20261113`，仅重新发布这一架构。运行二进制内的旧 `service.version=0.3.2-plan8` 为原制品版本元信息，签名和 SHA256 以发布记录为准。

## 分阶段命令

工作目录 `D:/workspace/go/Argus`。设置 `ARGUS_PLAN9_PASSWORD`、`PYTHONUTF8=1`、`PYTHONIOENCODING=utf-8`、`PYTHONDONTWRITEBYTECODE=1`；CA 使用 `ARGUS_PLAN9_CA_BUNDLE=D:/workspace/go/Argus/deploy/.cache/argus-trust-bundle.pem`。证据默认写 C 盘；私钥、密码、Cookie 只放受 ACL 保护的 `private/`。

```text
python docs/plan9/runtime/B11c/run.py prepare
python docs/plan9/runtime/B11c/run.py build
python docs/plan9/runtime/B11c/run.py up
python docs/plan9/runtime/B11c/run.py network
python docs/plan9/runtime/B11c/run.py isolation
python docs/plan9/runtime/B11c/run.py publish
python docs/plan9/runtime/B11c/run.py bastion
python docs/plan9/runtime/B11c/run.py member
python docs/plan9/runtime/B11c/run.py collectors
python docs/plan9/runtime/B11c/run.py builtins
python docs/plan9/runtime/B11c/run.py workload
python docs/plan9/runtime/B11c/run.py remote
python docs/plan9/runtime/B11c/run.py configure_alerts
python docs/plan9/runtime/B11c/run.py revocation
python docs/plan9/runtime/B11c/run.py expiry_prepare
python docs/plan9/runtime/B11c/run.py rotate
python docs/plan9/runtime/B11c/run.py connection
python docs/plan9/runtime/B11c/run.py exercise_alert
python docs/plan9/runtime/B11c/run.py exercise_data_pause
python docs/plan9/runtime/B11c/run.py expiry_check
python docs/plan9/runtime/B11c/run.py dashboards
python docs/plan9/runtime/B11c/run.py browser
python docs/plan9/runtime/B11c/run.py delivery_watch
python docs/plan9/runtime/B11c/run.py verify
```

这些是显式阶段，不得盲目重跑已部分完成的写操作。轮换使用私有 journal；失败时核对 OS 密码、Secret/Credential 版本、预览及执行状态再继续，不回退版本。本次修复前失败已保留；修复后的 `rotate_after_fix` 在同一 Host 上实际执行 v2→v3。

已有场景仍在运行，用户再次要求“执行 B11c”时，使用 `python docs/plan9/runtime/B11c/recheck.py --review-id recheck-20261116-01` 复用原身份、确认网络/权限/采集并恢复到期的八小时负载，再执行实时仪表盘、四条真实会话/录像、正式健康及 120 秒投递观察。每次使用新的日期/序号，证据单独写入原 run 的 `rechecks/`；不覆盖首次故障、轮换或过期测试记录，不声称本次重放了历史写操作。脚本不会重新部署正式平台或推进 B12。

过期测试先创建真实停采预览但不确认，采集持续开启；等待原生 15 分钟并核对服务端 Date，再确认并验证拒绝、无执行和采集版本不变。禁止修改时钟、TTL 或数据库。

撤权测试先执行，恢复授权后再发过期预览，避免把授权版本变化冒充时间过期。真实 PTY 进入 `invalidated`，旧票据被拒；只撤销本批 host/account/user 精确授权，恢复供人工查看。录像与命令审计必须开启，不放开文件或剪贴板。

故障各自配 110/120 秒 systemd 恢复定时器和 finally：控制 SSH 只杀持有 18443/19443 的已核实子进程，数据 SSH 和 SSH 服务保留；业务服务失败与 Root Collector 暂停分别执行。每次实际恢复后再进入下一类故障。

## 修复与证据边界

发现的真实重连问题、修复版本、原失败和历史数据缺口见 [B11c-D01](../../27-b11c-tunnel-reconnect-20261115.md)。投递验证比较同一持久实例在验证窗口内的增量，不清零历史计数，也不将历史丢弃声称为零。

正式健康使用正常 `argusctl verify --config deploy/.cache/argus-local-formal.yaml --output json --artifacts <本批>/private/formal-health`；JSON 保存为 `formal-health.json`。完成后按归属删除其配置副本。核对实际镜像、rollout、全部 Deployment/PVC UID 和正常登录。

本次自动审批拒绝配置副本删除（仅返回 `blocked by policy`），因此保留在受 ACL 保护的私有目录，不转移到公开证据或源码。Go 模块和编译缓存已使用 Go 自带 `clean` 清理，保留安装的 SDK、已发布镜像和必要验收证据。

全部业务对象通过正式 API、Preview/Confirm 和真实安装/注册/会话创建。诊断允许只读数据库查询；没有业务 SQL 写入、遥测伪造或手工告警事件。受管仪表盘定义、既有账号与正式数据保留。

## 用户要求下一批后的退役

`python docs/plan9/runtime/B11c/run.py retire`，本轮保留环境时不执行。先封存证据和关闭会话，停用本批授权、告警与采集；成员先经 Bastion 新 SSH 检测、卸载并核实，再删除成员；Root 经 Direct Executor 新 SSH 检测后卸载和删除 Scope。卸载预检是 SSH-only，不申请新的注册回调。

撤销本批 evaluator Host 数据授权，保留借用的服务账号和角色。最后核对 UID/标签后删除专属 Compose 卷、私网和遥测 Service/目录发布 Job；保留共享 kind 网络、签名制品、产品模板、正式六个 PVC 及历史会话、录像、审计和告警。
