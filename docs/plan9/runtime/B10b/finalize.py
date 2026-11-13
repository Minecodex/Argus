"""Update current review pointers only after final native and browser checks pass."""
import json,subprocess
from datetime import datetime,timezone,timedelta
from pathlib import Path
from run import Run,ROOT
r=Run();checks=json.loads((r.out/'checks.json').read_text(encoding='utf-8'));assert checks['status']=='ready_for_review'
base=ROOT/'docs/plan9';cluster=r.cluster_id;mapping=json.loads(r.resources.read_text(encoding='utf-8'))
cycle=checks['actual_alert_cycle'];deadline=json.loads((r.out/'activity-deadline.json').read_text(encoding='utf-8'))['epoch']
local=lambda value:datetime.fromisoformat(value.replace('Z','+00:00')).astimezone(timezone(timedelta(hours=8))).strftime('%Y-%m-%d %H:%M:%S')
stop=datetime.fromtimestamp(deadline,timezone.utc).astimezone(timezone(timedelta(hours=8))).strftime('%Y-%m-%d %H:%M:%S')
board=mapping['dashboards']['storage-jobs']['id']
text=f"""# B10b Kubernetes 存储与任务查看记录

当前运行 `{r.id}`，集群 **B10b Kubernetes Storage and Jobs**，命名空间 **b10b-workloads**。B10a 已按本次下一轮指令清理，历史保留；本批环境运行中，人工验收待进行。正式环境六个 PVC 绑定保留。

## 查看路线

1. 打开 [B10b 资源工作台](https://argus.dev/kubernetes/{cluster}?tab=resources)，选择 `b10b-workloads`。
2. 「存储」查看 `b10b-data` PVC，实际容量为 128 MiB，StorageClass 为 `b10b-expandable`；查看绑定 PV 和只读 StorageClass。已经完成 64 → 128 MiB 的在线扩容、重启保留文件、缩容拒绝，以及临时卷删除回收。
3. 「工作负载」查看 `b10b-job`、`b10b-pausable-job` 和 `b10b-cron`。已完成暂停/恢复、真实定时执行与手动运行；`b10b-ui-*` 是浏览器通过正常操作创建的真实任务，可能随后按 CronJob 的最近两条历史策略回收，执行记录保留。对应 Pod 的日志应显示 `job-completed`；业务 `b10b-storage-*` Pod 支持 Shell，可查看 `/data/receipt.json`、`/data/jobs.jsonl` 和 `df -k /data`。
4. 「弹性伸缩」查看 `b10b-autoscale` HPA，目前恢复 1 副本。实际 CPU 压力已完成 **1 → 3 → 1**，可以在仪表盘选择最近一小时查看历史波动。
5. 「网络」查看 `b10b-storage-allow-client` NetworkPolicy，当前保持生效。`b10b-allowed` 可以访问存储服务，`b10b-denied` 被真实 Calico 拒绝；已验证删除策略后恢复、重建后再次阻断。
6. 打开 [本批仪表盘](https://argus.dev/dashboards/{board}?resource={cluster}&resource_type=kubernetes_cluster) 或[四个绑定视图](https://argus.dev/kubernetes/{cluster}?tab=dashboards)：本批可用副本、HPA 当前/期望副本、Job 完成数、CPU/内存，以及集群、节点和 Collector 默认视图。
7. 在[集群详情](https://argus.dev/kubernetes/{cluster}?tab=resources)切换「采集配置」，三项原生采集应为健康、新鲜。节点行「证书信任」展示已配置公开 CA；节点名称匹配始终来自固定节点端点。
8. [告警中心](https://argus.dev/alerts) 的告警历史搜索 `B10b storage deployment has no available replica`。实际触发于 **{local(cycle['started_at'])}**，恢复于 **{local(cycle['resolved_at'])}**（北京时间）。当前本批活跃告警为 0；没有配置外部通知收件人。

## 本轮结果与范围

- 三项采集健康，四个仪表盘共 **{checks['total_dashboard_panels']}** 个面板取得真实数据。
- 六项部署页面 E2E 通过，覆盖中英明暗、桌面宽度、七类对象、真实 CronJob 手动执行、Pod 日志/终端、节点公开 CA 表单及仪表盘。
- 完整重试周期持续观察 **{checks['delivery_watch_seconds']:.1f} 秒**，没有新增终止投递、入队失败或接收拒绝；本轮累计终止丢失指标点为 **{checks['terminal_failed_points']}**。台账没有清零。
- 正式健康 **{checks['formal_health_checks']}/20** 通过，六个原有 PVC 保留。正式环境原有 NetworkPolicy 探测和无外部 Egress Gateway 降级提示仍记录；本批独立 Calico 的阻断测试已实际通过。
- 节点 CA 文件已由实际 Agent 使用；独立错误 CA 测试被原生 TLS 拒绝，测试 Job/ConfigMap 已清理。长 Job 原始创建观察被后续编辑替代的旧记录，已明确结算为 `KUBERNETES_OPERATION_SUPERSEDED`；后续暂停、恢复和完成分别有成功记录。

Rawfile 在本环境没有提供该 PVC 的 kubelet volume stats。PVC 对象、实际挂载 statvfs/df 和保留文件均已验证；没有生成虚假的 PVC 使用率曲线。详细原因、源码及发布范围见[修复记录](../19-b10b-storage-jobs-and-node-ca-20261112.md)。

两个低速客户端每 30 秒运行一次，后台活动截止 **{stop}（北京时间）**。CronJob 在同一时刻由精确 UID 保护暂停；环境和监控保留。查看通过并要求下一批后，先回收消费者/PVC/PV，再退役 Collector、集群与专属入口，进入 B10c。

[运行与清理流程](../runtime/B10b/runbook.md)。证据目录：`artifacts/plan9/{r.id}/`，最终状态见 `checks.json`。
"""
(base/'batches/B10b-review.md').write_text(text,encoding='utf-8',newline=chr(10))
p=base/'batches/B10a-review.md';text=p.read_text(encoding='utf-8');start=text.index('\n\n')+2;end=text.index('\n\n',start)
text=text[:start]+'历史运行 `p9-b10a-20261111-01` 已于 2026-11-12 按用户下一轮指令完成退役。独立 Kind 节点、Collector、凭据授权和专属入口已回收，正式六个 PVC 保留，历史指标、告警、操作和证据保留。该指令不视为对每项反馈的显式验收通过。当前查看 [B10b](./B10b-review.md)。'+text[end:];p.write_text(text,encoding='utf-8',newline=chr(10))
p=base/'README.md';text=p.read_text(encoding='utf-8');start=text.index('创建日期：');end=text.index('\n\n',start)
text=text[:start]+f'创建日期：2026-10-26。当前阶段为 **B10a 已退役，B10b 存储、任务、真实 CPU HPA 和 NetworkPolicy 已部署，等待用户查看**。三项原生采集、四个仪表盘共 {checks["total_dashboard_panels"]} 个面板、实际告警触发/恢复、六项部署页面 E2E 和正式健康 20/20 通过。'+text[end:]
old='当前 [B10a 查看入口](./batches/B10a-review.md) 提供独立集群的常用对象、差异预览/确认、真实变更、Pod 日志/终端、四个仪表盘和副本告警。端口转发遵循现有 CLI/API 边界。'
text=text.replace(old,'B10a 已清理，原始反馈与修复证据保留。[当前 B10b 查看入口](./batches/B10b-review.md) 提供真实 PVC 在线扩容、任务暂停/恢复/手动执行、CPU HPA、网络隔离、四个仪表盘和已恢复告警。')
anchor='| [B10a 运行包]';pos=text.index('\n',text.index(anchor))
if '| [B10b 查看记录]' not in text:text=text[:pos+1]+'| [B10b 查看记录](./batches/B10b-review.md) | 存储扩容、Job/CronJob、CPU HPA、真实 NetworkPolicy 与告警历史 |\n| [B10b 运行包](./runtime/B10b/runbook.md) | 固定版本 CNI/CSI/metrics-server、正常平台操作、有界活动及先回收卷再销毁 |\n| [B10b 节点证书和操作结算](./19-b10b-storage-jobs-and-node-ca-20261112.md) | 受管公开 CA 文件、错误 CA 拒绝、后续编辑替代旧观察与正式发布 |\n'+text[pos+1:];p.write_text(text,encoding='utf-8',newline=chr(10))
p=base/'progress.json';value=json.loads(p.read_text(encoding='utf-8'));value.update(status='B10b_deployed_waiting_user_review',current_run_id=r.id,next_suggested_batch='B10c')
value['documentation'].update(review_entry='batches/B10b-review.md',current_review='batches/B10b-review.md',issues='19-b10b-storage-jobs-and-node-ca-20261112.md',storage_jobs_and_node_ca='19-b10b-storage-jobs-and-node-ca-20261112.md')
value['delivery'].update(compose_packages='B01_through_B10a_retired_B10b_running_others_planned',runtime_scripts='B10b_native_Kubernetes_CSI_jobs_CPU_HPA_Calico_normal_API',deployment='B10b_fresh_deployed',human_acceptance='B10b_pending_user_review')
value['baseline'].update(formal_version='B10b_native_ca_v3_and_node_ca_web_v1_frozen_Schema52',source_head=subprocess.check_output(['git','rev-parse','--short','HEAD'],cwd=ROOT,text=True).strip())
batch=next(x for x in value['batches'] if x['id']=='B10');batch.update(status='B10b_pending_user_review',package_status='B10a_B10b_implemented_and_executed',self_check='B10b_real_storage_jobs_CPU_HPA_network_policy_metrics_alerts_and_6_UI_cases_passed',runtime_status='B10b_running')
prior=next(x for x in batch['sub_batches'] if x['id']=='B10a');prior.update(status='destroyed',runtime_status='destroyed',review_status='advanced_by_user_request_without_explicit_acceptance',cleanup_record='../../artifacts/plan9/p9-b10a-20261111-01/cleanup-verification.json')
current=next(x for x in batch['sub_batches'] if x['id']=='B10b');current.update(status='ready_for_review',package_status='implemented_and_executed',self_check='actual_CSI_online_resize_reclaim_jobs_suspend_resume_manual_scheduled_CPU_1_3_1_Calico_deny_restore_6_UI_cases_and_health20_passed',review_status='pending_user_review',runtime_status='deployed',handoff='batches/B10b-review.md',open_issues=[],limitations=['Rawfile per-PVC kubelet volume stats unavailable; real object capacity and mounted statvfs/df verified'],fixes=['explicit_node_CA_file_materialization','superseded_accepted_operation_settlement'])
if r.id not in current.setdefault('runs',[]):current['runs'].append(r.id)
if r.id not in batch['runs']:batch['runs'].append(r.id)
p.write_text(json.dumps(value,ensure_ascii=True,indent=2)+'\n',encoding='utf-8',newline=chr(10))
p=base/'01-batch-catalog.md';text=p.read_text(encoding='utf-8');text=text.replace('所有条目都是待执行方案。','以下条目定义各批验证范围，当前部署和退役状态见 [状态清单](./progress.json)。')
anchor='## B10：Kubernetes 工作台实际操作';text=text if '当前 B10b 的实际结果' in text else text.replace(anchor,anchor+'\n\n当前 B10b 的实际结果、能力限制和平台入口见 [B10b 查看记录](./batches/B10b-review.md)，B10a 已清理。');p.write_text(text,encoding='utf-8',newline=chr(10))
p=ROOT/'docs/README.md';text=p.read_text(encoding='utf-8');text=text if '56. [B10b' in text else text+'\n56. [B10b 存储、任务、节点证书和操作结算实测](./plan9/19-b10b-storage-jobs-and-node-ca-20261112.md)\n';p.write_text(text,encoding='utf-8',newline=chr(10))
p=ROOT/'docs/plan8/23-fixed-own-metric-instances.md';text=p.read_text(encoding='utf-8');text=text if '## 节点公开 CA 的实际文件信任' in text else text+'\n\n## 节点公开 CA 的实际文件信任\n\n2026-11-12，节点指标固定实例复用公开 CA 配置与证书信任表单。原生 kubeletstats ServiceAccount 客户端按文件读取 CA，因此配置编译使用固定的 `/etc/argus-otelcol/kubelet-ca.pem`，部署器把规范化的公开 PEM 写入候选配置卷并纳入既有大小预算、校验和回滚。未配置时保持原 kubelet 证书路径。节点始终按固定节点名称校验，不支持服务名覆盖；API 拒绝该字段，门户隐藏对应输入。没有新增任意路径、端点、私钥或跳过校验选项。\n\n受管 CA 文件取样与错误 CA 的原生拒绝、三项实际采集及中英明暗表单验收见 [B10b 记录](../plan9/19-b10b-storage-jobs-and-node-ca-20261112.md)。\n';p.write_text(text,encoding='utf-8',newline=chr(10))
print('Review pointers updated only after final checks passed',flush=True)
