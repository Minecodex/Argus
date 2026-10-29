# B05c ZooKeeper 真实场景运行包

运行 `p9-b05c-20261029-01`。用户要求“下一阶段”后，B05b 经正常 API 停采、卸载和退役，Docker 专属资源清理；账号和历史保留。

## 环境和权限

- 两个独立 ZooKeeper 3.9.6 实例 A/B，均为真实 standalone，不能称为一个高可用 ensemble。
- 一台正常 SSH 接入的 `B05c-ZooKeeper-Collector`，安装正式 Connector/Collector。A 做压力，B 为持续正常读写对照。
- 使用 Kazoo 2.9.0 建立真实会话，创建、读取、更新节点和注册 Watch。`/plan9` 子树使用各实例独立的随机 digest ACL；匿名读取实际被拒绝。
- Collector 通过原生 `mntr` 采集。仅允许 `mntr,ruok,stat,srvr` 四字命令；该统计协议本身不使用业务节点的 digest 身份，不能把节点 ACL 说成对监控 TCP 端点的认证。
- 三个容器、七个命名卷、一个专网。CPU 上限合计 3、内存上限 2048 MiB；每个 ZooKeeper JVM 堆 64–128 MiB。只发布主机 SSH `127.0.0.1:22291`，2181 不向宿主发布。

数据、事务日志、服务日志分别在本批各实例的独立卷内。主机使用独立 systemd/private cgroup，无 Docker socket 和无关宿主目录。镜像使用本次核验的 image ID/摘要，见 `images.lock.json`。密码只放在受限 private 文件和客户端 root-only 目录，不写入镜像、命令行或报告。

本批不包含多节点选主/容灾或 TLS 验收，不与正式平台的 Keeper/存储混用，不向 Argus 数据库或遥测入口写预制指标。

## 初始化和接入

```powershell
python docs/plan9/runtime/B05c/manage.py prepare
docker build --tag argus-plan9-zookeeper:b05c-20261029 --file docs/plan9/runtime/B05c/Dockerfile docs/plan9/runtime/B05c
python docs/plan9/runtime/B05c/manage.py up
python docs/plan9/runtime/B05c/manage.py initialize
python docs/plan9/runtime/B05c/manage.py workload

$sessionPath = 'artifacts/plan9/reset-20261026/private/enterprise-session.json'
$caPath = 'artifacts/plan9/reset-20261026/trust-bundle.pem'
python docs/plan9/runtime/B05c/integration.py onboard --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05c/integration.py collectors --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05c/integration.py builtins --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05c/integration.py targets --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05c/integration.py alerts --session $sessionPath --ca $caPath
```

初始化实测节点写读、更新触发 `CHANGED` Watch 通知、匿名 `NoAuth`、ephemeralOwner 与真实会话匹配；关闭所属会话后，由服务器自动删除临时节点。这里没有调用 delete 来伪装会话回收。

正常负载在每个实例保持一个真实会话、一个临时节点和一个 Watch，每两秒更新/读取有界数据（不超过 1 KiB），每进程最长 8 小时；不会不断累积节点。到期关闭会话，服务和监控继续保留。

## 压力、告警和恢复

```powershell
python docs/plan9/runtime/B05c/integration.py exercise --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05c/integration.py lifecycle --session $sessionPath --ca $caPath
python docs/plan9/runtime/B05c/integration.py verify --session $sessionPath --ca $caPath
```

在故障前记录正常基线并冻结阈值，只选择 A：

| 场景 | 实际动作 | 告警阈值 | 恢复 |
| --- | --- | --- | --- |
| 连接数增多 | 打开 12 个真实认证会话，各持有临时节点和 Watch | connections > 8；恢复 <= 4 | 关闭会话、服务器删除临时节点 |
| ZNode 增多 | 一个真实会话创建 80 个临时节点并注册 Watch | znodes > 60；恢复 <= 20 | 关闭该会话，服务器回收 80 个节点 |

评估间隔 10 秒，触发/恢复持续 20 秒；客户端压力最长 180 秒，另有 systemd 190 秒上限。显式恢复先关闭会话，再检查 `/plan9/connections` 和 `/plan9/bulk` 已空。B 在整个过程中保持正常。`mntr` 与 Collector 的采样时刻可能不同，连接数核验考虑短暂统计连接的影响，不凭空补值。

单独停采 A 时，两条策略暂停评估，B 仍持续取得新样本；重新启用后核验配置版本、样本和评估恢复。`no_data_behavior=report` 的短暂查询健康事件如有发生，按实际结果保留记录。

默认 ZooKeeper 仪表盘有六个面板：活动连接、ZNode、Watch、当前请求、平均延迟、数据包速率。两目标分别验证共 12 次面板查询；中英标题补齐，继续使用共享图表、统计卡片和主题组件，不改数据表达式。

真实浏览器测试为 `web/apps/enterprise/e2e/plan9-b05c-real.spec.ts`。先完成 `verify` 的仪表盘关联，再运行测试；覆盖中英/明暗、A/B 目标范围、全部面板、历史告警、时间线和 1280/1920 桌面宽度。最终入口见 [B05c 查看记录](../../batches/B05c-review.md)。

## 用户要求下一批后清理

```powershell
python docs/plan9/runtime/B05c/retire.py --session $sessionPath --ca $caPath
```

先停止压力与正常客户端，等待服务器回收临时节点，再通过正常 API 收尾策略、采集、凭据和授权，卸载并退役主机，最后按准确 ID/run 标签清理本批三个容器、七个卷及网络。企业账号、共享模板、评估身份和历史保留，不自动进入 B06a。
