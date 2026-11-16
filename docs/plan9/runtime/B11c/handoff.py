"""Build the review route from actual runtime evidence and native timestamps."""
from datetime import datetime,timedelta,timezone
import json
from manage import Run,ROOT

run=Run();mapping=json.loads(run.resources.read_text());completed=(run.out/'checks.json').exists()
def load(name):return json.loads((run.out/(name+'.json')).read_text(encoding='utf-8-sig'))
def local(value):return datetime.fromisoformat(value.replace('Z','+00:00')).astimezone(timezone(timedelta(hours=8))).strftime('%Y-%m-%d %H:%M:%S')
service=load('alert-cycle');data=load('data-pause-cycle');rotation=load('credential-rotation');connection=load('connection-recovery')
root=mapping['hosts']['bastion'];member=mapping['hosts']['member'];policy=mapping['alert_policies']['service'];rows=[]
for key,title in [('bastion','堡垒机'),('member','私网成员')]:
    host=mapping['hosts'][key]
    rows.append('| '+title+' '+host['name']+' | '+' / '.join('['+board['name']+'](https://argus.dev/dashboards/'+board['id']+'?resource='+host['host_id']+')' for board in mapping['dashboards'].values())+' |')
text=f'''# B11c 用户查看路线

运行 `{run.id}`。状态：**{'已部署，自检通过，等待人工查看' if completed else '已部署，最终自检中，尚未标记交付'}**。B11c 是 B11 的最后一批，B12 真实模型聊天尚未启动。B11b 已按此前全量非账号重置授权退役，不追改人工结论。

## 从这里查看

1. 打开 [主机列表](https://argus.dev/hosts)，沿用企业 `test` 的原账号 `kakj` 登录。当前两台都是本轮新建并保留的真实 systemd/OpenSSH 主机；列表 CPU/内存/磁盘渐进加载。
2. 进入 [B11c-Tunnel-Bastion](https://argus.dev/hosts/{root['host_id']}) 和 [B11c-Private-Member](https://argus.dev/hosts/{member['host_id']})。查看详情中的实际控制路径与采集路由、采集配置和四套默认仪表盘。
3. 在主机“终端与会话”中选择 `root`、`Shell / PTY` 或 SSH，填写事由后建立会话。可运行 `hostname`、`id`、只读 `ps`，查看底/左/右停靠和调整尺寸。Shell 实际执行用户为 Connector 服务账号，SSH 为 root；不能将账号选择当成 Shell 的 sudo 授权。录像和命令审计必需，未开放文件上传或剪贴板。
4. 打开 [远程会话](https://argus.dev/remote-sessions) → 历史会话，搜索 `B11c`。四条中英/明暗自动验收会话已有真实命令输出与加密录像，可打开回放。撤权测试原会话为 `invalidated`，正常关闭会话为 `terminated`；用户授权已恢复。
5. 打开 [业务告警历史](https://argus.dev/alerts?tab=history&policy_id={policy['id']}) 与 [数据健康历史](https://argus.dev/alerts?tab=history&policy_id={mapping['alert_policies']['freshness']['id']})。业务和 no_data 测试异常均已恢复；当前列表保留 **两条 Collector delivery data gap 历史缺口诊断**，这是修复前真实丢弃的未处理证据，不能因恢复采样而伪装消失。查看当时趋势请使用下面的绝对时间。没有发送外部通知。

## 八个资源范围视图

| 主机 | 已真实执行的四套仪表盘 |
| --- | --- |
{chr(10).join(rows)}

共八项原生采集、八个资源范围视图、40 个非空面板。当前状态以实际采样和刷新时间为准。两台容器共享 Docker VM 内核，系统容量口径不等同于两台物理机；资源身份、进程、服务和隧道均分别验证。

## 北京时间故障与恢复

| 场景 | 实际时间 | 已验证结果 |
| --- | --- | --- |
| 修复后 OS 密码与 Secret/Credential v2→v3 | {local(rotation['started_at'])}～{local(rotation['verified_at'])} | 旧 v2 预览拒绝、新 v3 后台重连成功；原 Host/Connector/Collector/Host Key 保留；无新增丢弃 |
| Root 控制 SSH 中断与自动重连 | {local(connection['started_at'])}～{local(connection['verified_at'])} | Root 控制离线时数据仍推进；成员隧道受控制租约影响；恢复后八项来源均新鲜，无新增丢弃 |
| 成员服务真实退出失败 | 原生故障 {local(service['native_fault_started'])}；告警 {local(service['started_at'])}～{local(service['resolved_at'])} | `B11c private service failed` 已恢复；同一查询 Root=0、Member=1 |
| 主动停止 Root Collector | 原生暂停 {local(data['native_pause_started'])}；告警 {local(data['started_at'])}～{local(data['resolved_at'])} | 两台控制仍在线，真实 `Query health: no_data` 触发并恢复 |
| 自然过期预览 | 2026-11-15 12:32:50 创建，12:47:50 到期，12:47:59 拒绝 | 没有改时钟/TTL；无执行，成员采集仍启用、版本不变 |

## 已修复问题与保留的历史缺口

本轮发现并修复了 PostgreSQL jsonb 格式化导致重连证明被误拒绝的问题，server 与 worker 已同步部署。修复前的中断超过 300 秒重试上限，Root 历史丢弃 **2562** 指标点、Member **448** 指标点。计数未清零，缺口未伪造回补。修复后重新完成整轮轮换和故障验证，检查无新增丢弃；不能将它表述为全程零丢失。详见 [B11c-D01](../27-b11c-tunnel-reconnect-20261115.md)。

## 自动自检与保留环境

正常登录、严格 TLS 的真实浏览器已通过四条正向会话链路和一条撤权链路，覆盖中文/英文、浅色/深色及 1280/1440/1920 桌面宽度；录像事件包含真实命令输出。正式平台健康 **20/20**；六个 PVC 与 Deployment UID 保留。发布证据明确记录修复导致的镜像/代际变化，不声称正式部署完全未变。

原始证据位于 `C:/Users/23574/.codex/artifacts/argus-plan9/{run.id}`：`checks.json`、`server-fix-release.json`、`credential-rotation.json`、`expiry-refusal.json`、`connection-recovery.json`、`alert-cycle.json`、`data-pause-cycle.json`、`delivery-maintenance-gap.json`、`delivery-watch.json`、`browser/`、`revocation-browser-r3/`。私有目录不对外分发。

授权截至 `{mapping['remote']['valid_until']}`，仅限当前用户、两台 Host、两个托管账号及 Shell/SSH terminal。真实 HTTP 低负载最长八小时，结束后仅停负载，环境继续保留。自动自检不代表用户已接受。

后续由用户确认并要求下一批后按 [运行包](../runtime/B11c/runbook.md) 成员先行退役。借用的 evaluator 账号、角色及共享后台评估设置保留；只撤销本批 Host 数据授权。不会在本轮自动清理 B11c 或启动 B12。
'''
path=ROOT/'docs/plan9/batches/B11c-review.md';path.write_text(text,encoding='utf-8');(run.out/'handoff.md').write_text(text,encoding='utf-8')
print('Actual B11c review links and native timeline written; human acceptance remains pending',flush=True)
