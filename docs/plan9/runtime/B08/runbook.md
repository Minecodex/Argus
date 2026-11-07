# B08 Linux 文件日志与 Journald 运行包

本批在同一个独立 Debian systemd 主机中运行真实 HTTP 服务。服务处理请求时向专用目录写结构化 JSON，遇到受控 HTTP 503 时写一条带堆栈的多行异常，并将请求记录写入 systemd Journal。Collector 从操作系统实际采集。两条文件规则归属应用和服务，Journald 规则限定 `plan9-b08.service` 并归属执行主机。

## 启动与接入

从仓库根目录执行。复用 B01 的本地 Debian systemd 基础镜像。构建记录保留镜像身份和源文件摘要，Compose 用本批镜像 ID。启动前要求 B07c 完成归档、撤销接入和清理核对。

```powershell
$session = 'artifacts/plan9/reset-20261026/private/enterprise-session.json'
$ca = 'artifacts/plan9/reset-20261026/trust-bundle.pem'
$manage = 'docs/plan9/runtime/B08/manage.py'
$integration = 'docs/plan9/runtime/B08/integration.py'
python $manage prepare
python $manage build
python $manage up
python $integration enroll --session $session --ca $ca
python $integration alerts --session $session --ca $ca
```

测试主机仅在本机回环绑定 SSH 22344。容器上限为 1.5 vCPU、1280 MiB、512 个进程，Journal 持久日志上限为 32 MiB。SSH 凭据随机生成并存放在私有目录。主机指标、systemd 状态和 Collector 自身指标通过原生采集接入。告警评估角色仅追加本批主机范围。

## 场景与真实验证

```powershell
python $integration rotation --session $session --ca $ca
python $integration lifecycle --session $session --ca $ca
python $integration exercise --session $session --ca $ca
python $integration redaction --session $session --ca $ca
python $manage workload
python $integration dashboards --session $session --ca $ca
python docs/plan9/runtime/B08/browser.py
```

- rename/create 和 copytruncate 使用两份独立日志文件。逐条核对序号、内容与资源归属，每种轮转用 inode 变化验证执行方式。旋转过程中保持测试后台负载停止。
- Collector 重启使用真实 systemd invocation 变化核验。单独停用 JSON 规则，验证多行和 Journald 持续，重新启用后检索停采期间写入的记录。
- 使用受控启动失败验证 `systemd_unit_state` 告警。阈值 0.5，恢复阈值 0，每 10 秒评估，触发和恢复分别持续 20 秒。100 秒后定时恢复，驱动也在故障结束时主动恢复。错误日志用于交叉核对实际故障。
- 真实文件中写入非凭据的随机测试字符串验证 `authorization` 字段脱敏。证据记录结果，原值只在本批容器文件中短暂存在。
- 低速负载间隔 15 秒，每 8 个请求有一次真实 HTTP 503，以便在页面对照正常记录与多行异常。后台最多运行 8 小时或 1,920 次。
- 查询页面验证搜索、记录字段与来源。当前版本的日志记录界面提供查看和筛选，下载是本批验收缺口。

## 清理

```powershell
python $integration retire --session $session --ca $ca
```

停止负载，归档本批规则、仪表盘与应用，撤销本批授权与告警策略，执行 Collector 和 Connector 的正常卸载。根据运行清单与所有权标签清理 Compose 的容器、两个卷和网络，保留历史数据和共享平台。清理结果和实际入口见 [B08 验收记录](../../batches/B08-review.md)。
