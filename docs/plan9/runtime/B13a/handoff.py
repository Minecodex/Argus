"""Seal already verified B13a evidence and synchronize the manual-review record."""
import json
from pathlib import Path
from urllib.parse import urlencode
from manage import Run, ROOT, now, write_json


def main():
    run = Run()
    mapping = json.loads(run.resources.read_text(encoding='utf-8'))
    def read(name):
        return json.loads((run.out / (name + '.json')).read_text(encoding='utf-8-sig'))
    for name in ['postgres-tests', 'scope-acceptance', 'archival-acceptance', 'delivery-watch', 'formal-health', 'current-state', 'combined-unit-correction']:
        assert read(name)['passed'], name
    assert len(read('formal-health')['checks']) == 20
    assert '8 passed' in (run.out / 'browser-delivery.log').read_text(encoding='utf-8')
    assert 'generated contracts are current (isolated generation)' in (run.out / 'contracts-check.log').read_text(encoding='utf-8')
    assert 'error TS' not in (run.out / 'frontend-typecheck.log').read_text(encoding='utf-8')
    panels = sum(len(p) for key in ['orders', 'reports'] for p in read(key + '-dashboard-checks').values())
    panels += sum(len(x['panels']) for x in read('dashboard-checks'))
    assert panels == 60
    archived = mapping['archive_probe']['input']
    history = 'https://argus.dev/dashboards/' + mapping['dashboards']['memcached']['id'] + '?' + urlencode({
        'resource': mapping['targets']['memcached']['target_id'], 'resource_type': 'monitoring_target',
        'time_from': archived['from'], 'time_to': archived['to']})
    overview = 'https://argus.dev/dashboards/' + mapping['batch_dashboard']['id']
    personal = 'https://argus.dev/dashboards/' + mapping['personal_dashboard']['id']
    finding = {'id': 'B13a-D01', 'status': 'open', 'stage': 'initial_configuration',
               'observed_reason': 'SOURCE_REJECTED', 'permanent_metric_point_losses': {'orders': 1642, 'reports': 568},
               'counter_reset': False, 'current_watch_seconds': 120, 'new_failures_during_watch': 0,
               'root_cause_claim': 'Initial consecutive configuration changes coincide with rejection; controlled cutover reproduction remains required',
               'next_gate': 'B13c configuration cutover/drain investigation; fresh-start verification in the next accepted batch'}
    run.save('known-findings', {'items': [finding, {'id': 'B13a-D02', 'status': 'open', 'scope': 'lab_host_filesystem',
        'reason': 'The credential bind mount under /root is hidden in the Collector service namespace; native filesystem scraper logs no-such-file.',
        'boundary': 'Do not weaken ProtectHome or reveal credentials for a metric; future lab packaging must avoid this protected bind-mount path.'},
        {'id': 'B13a-D03', 'status': 'not_claimed', 'scope': 'target_archive_portal_entry',
         'reason': 'Target archive was verified through the normal preview/confirmation API; full portal target-lifecycle interactions belong to B13c.'}]})
    snapshot = {'run_id': run.id, 'batch': 'B13a', 'status': 'ready_for_review_with_known_findings', 'verified_at': now(),
                'human_acceptance': 'pending', 'online_hosts': 2, 'applications': 2, 'application_environments': ['production', 'staging'],
                'monitoring_targets_created': 2, 'active_monitoring_targets': 1, 'archived_monitoring_targets': 1,
                'initial_nonempty_default_dashboard_panels': 60, 'historical_archived_panels': 9, 'current_fresh_collections': 9,
                'browser_cases': 8, 'formal_health_checks': 20, 'scope_and_permissions_passed': True, 'shared_redis_count': 1,
                'personal_copy_edited_and_published': True, 'historical_result_unchanged_after_target_archive': True,
                'real_service_alert_fired_and_resolved': True, 'delivery_watch_seconds': 120, 'new_delivery_failures_during_watch': 0,
                'historical_delivery_loss': finding['permanent_metric_point_losses'], 'known_findings': ['B13a-D01', 'B13a-D02', 'B13a-D03'],
                'model_calls': 0, 'mock_or_replayed_telemetry': False, 'direct_business_sql_writes': False,
                'formal_pvcs_and_deployment_uids_preserved': True, 'backend_image': read('template-release')['backend_image'],
                'helm_revision': 109, 'schema_migrations_added': False, 'overview': overview, 'personal_copy': personal, 'archive_history': history}
    run.save('handoff', snapshot)
    record = read('run')
    record.update(status=snapshot['status'], human_acceptance='pending', handoff_at=snapshot['verified_at'], known_findings=snapshot['known_findings'])
    run.save('run', record)
    mapping['targets']['memcached'].update(status='archived', archive_record='archival-acceptance.json')
    mapping['applications']['reports']['ended_bindings'] = ['memcached']
    run.save('resource-map', mapping)
    doc = f'''# B13a 查看与运行记录

2026-11-17，运行号 `{run.id}`。状态为 **已部署、核心自检通过并保留已知问题、等待人工验收**。B13b/B13c 未启动，Plan9 未标记全部完成。

## 先看哪里

1. [B13a 范围与共享依赖大盘]({overview})：默认最近 15 分钟、30 秒刷新。应用筛选中选择 `B13a Orders`（production）或 `B13a Reports`（staging）以检查关联范围；两者一起选择时，Redis 只计一次。
2. [Redis 个人副本]({personal})：由受管 Redis v3 复制，已修改标题并通过正式预览/确认发布；原模板保持独立。命令速率单位为 `ops/s`。
3. [Memcached 归档历史]({history})：固定为归档前实际时间段。该目标和采集已归档、当前清单不显示，9 个历史面板仍可读，归档前后原始结果完全相同。
4. [主机列表](https://argus.dev/hosts)：`B13a-Orders-Host`、`B13a-Reports-Host` 均在线。管理员已有两台主机限定的 Shell/SSH 账号及七天终端授权，保留命令审计和录像。
5. [应用列表](https://argus.dev/applications)：两应用处于不同环境，当前都关联各自主机与同一个 Redis；Reports 的 Memcached 关联在归档测试中已结束。
6. [告警中心](https://argus.dev/alerts)：`B13a Reports service failed` 已真实触发并恢复。初始化投递缺口另行保留，不自动伪装成已恢复。

“不限制应用 / 全部授权资源”允许查询当前主体可读的保留期历史，包括已归档来源；这不代表来源仍在采集。检查 B13a 当前关联时请使用应用筛选，查看归档数据时使用上面的绝对时间链接。

## 实际环境

| 对象 | 真实行为 | 当前状态 |
| --- | --- | --- |
| Orders 主机 / 应用 | 每五秒执行真实 HTTP→共享 Redis 请求 | 在线、production 应用有效 |
| Reports 主机 / 应用 | 真实 HTTP→Redis/Memcached 请求 | 在线、staging 应用有效 |
| B13a-Shared-Redis | 原生 Redis receiver、独立监控凭据 | 采集中，两个应用共享同一个目标身份 |
| B13a-Reports-Memcached | 原生 Memcached receiver | 已完成目标归档验证，保留历史 |
| B13a Orders 只读成员 | 独立普通登录、角色和资源授权 | 仅 Orders 应用/主机、共享 Redis 和明确授权仪表盘；无终端授权 |
| 后台评估主体 | 与只读成员分离 | 仅新增本轮主机范围，无外部通知 |

两台逻辑主机共享 Docker Linux 内核；其 CPU/内存不能相加解释为两台独立物理机容量。本轮真实 HTTP 流量最多八小时，缓存键带 TTL；容器和数据在用户验收前保留。

## 自检证据

- 默认仪表盘 60 面板曾全部取得实际数据；当前 8 项主机内置采集 + Redis 共 9 项健康且新鲜。Memcached 9 面板通过历史链接查看。
- 单/双应用、类型和目标范围、显式空范围、面板局部条件均通过。两应用合并前 4 个唯一资源，归档后当前关联为 3 个；共享 Redis 数量为 1。
- 只读成员的“全部授权”保留当前授权范围；显式请求 Reports 应用、主机、Memcached 三类目标均被拒绝。真实界面资源候选不出现 Reports，清空选择后保持空范围。
- 真实 Reports 服务退出 → 告警触发 → 自动恢复保护/主动恢复 → 同一告警恢复；Orders 与 Redis 在故障窗口保持可用。
- 8 项真实浏览器 E2E，覆盖 1280/1440/1920、中英、明暗、键盘刷新、可见自动刷新、离开页面停止请求、只读和空范围；未跳过 TLS。
- 真实临时 PostgreSQL 上 `internal/monitoring`、`internal/dashboard` 测试通过，临时数据库容器已清理；隔离契约生成检查及前端类型检查通过。
- 正式健康检查 20/20；六个正式 PVC 和所有 Deployment UID 保留。后台精确镜像为 `{snapshot['backend_image']}`，Helm revision 109，没有新增数据库迁移。
- 本轮未调用模型，不消耗 APIMart Token 或 Workspace 额度。B12 模型、会话、报告、账单和审计继续保留。

## 已知问题，不能写成全程零失败

- **B13a-D01**：初始化时两台 Collector 曾报告 `SOURCE_REJECTED`，Orders 永久丢弃 1,642、Reports 568 个指标点，总计 2,210。日志证实拒绝，与连续配置切换同窗；尚未以受控复现证明完整根因。最新 120 秒观察没有新增丢失、receiver 拒绝或入队失败，原实例 ID 和计数保留。公共接入脚本已改为等待每个内置配置生效后再提交下一项；首次接入和切换排空效果仍需后续实际验证，不放宽服务端来源栅栏、不清零历史告警。
- **B13a-D02**：本轮容器把凭据文件绑定在 `/root` 下，Collector 服务隔离后 filesystem scraper 对此挂载路径记录读取警告。CPU/内存、缓存和已有磁盘序列可读，但不宣称整机所有挂载点采集无警告；后续测试包装需避免此路径，不能为此放松 Secret/ProtectHome 隔离。
- **B13a-D03**：本次目标归档经过正式 API 的预览、确认和执行验证。独立目标归档的门户入口、完整撤权/生命周期交互留在 B13c，不以 API 通过冒充门户全流程通过。

## 修复与交接

新增修复和契约见 [B13a 实施记录](../36-b13a-dashboards-scope-and-archive-20261117.md)，运行方式见 [runbook](../runtime/B13a/runbook.md)。

证据根目录：`{run.out.as_posix()}`。主要文件：`handoff.json`、`scope-acceptance.json`、`personal-copy-check.json`、`archival-acceptance.json`、`alert-cycle.json`、`delivery-watch.json`、`known-findings.json`、`current-state.json`、`postgres-tests.json`、`contracts-check.log`、`frontend-typecheck.log`、`browser-delivery.log`、`formal-health.json`、`template-release.json`。截图位于 `browser-delivery/`；密码/会话及初始原始日志位于 ACL 限定的 `private/`，不进入仓库。

用户本次“继续下一轮 B13a”已作为 B12 接受/退役授权记录。B12 专属主机正常卸载，Redis 采集与目标已归档，Compose 容器/卷/网络及专属遥测入口已清理。B13a 现在等待用户查看，后续须在接受本轮并选择下一轮后才退役；不自动推进 B13b。
'''
    (ROOT / 'docs/plan9/batches/B13a-review.md').write_text(doc, encoding='utf-8')
    path = ROOT / 'docs/plan9/progress.json'
    progress = json.loads(path.read_text(encoding='utf-8'))
    prior = progress.get('current_runtime_validation')
    if prior and prior.get('run_id') != run.id:
        history_rows = progress.setdefault('runtime_validation_history', [])
        if not any(x.get('run_id') == prior.get('run_id') and x.get('verified_at') == prior.get('verified_at') for x in history_rows):
            history_rows.append(prior)
    progress.update(status='ready_for_review_with_known_findings', current_run_id=run.id, next_suggested_batch='B13b', current_runtime_validation=snapshot)
    progress['documentation'].update(review_entry='batches/B13a-review.md', current_review='batches/B13a-review.md',
                                     issues='36-b13a-dashboards-scope-and-archive-20261117.md', b13a='36-b13a-dashboards-scope-and-archive-20261117.md')
    progress['delivery'].update(compose_packages='B01_through_B12_retired_B13a_running', runtime_scripts='two_hosts_two_caches_two_apps_scoped_reader_dashboard_archive',
                                deployment='B13a_ready_for_review_with_known_findings', human_acceptance='B12_accepted_B13a_pending',
                                real_model_calls='none_in_B13a_B12_model_and_usage_history_retained')
    progress['validation'].update(scope='B13a_documentation_and_real_runtime', status='passed_with_known_findings',
                                  report=str(run.out / 'handoff.json'), runtime_passed=True, runtime_result=snapshot['status'],
                                  known_findings=snapshot['known_findings'], full_plan_delivery_complete=False)
    if (run.out / 'documentation-checks.json').exists():
        progress['validation']['local_links_checked'] = read('documentation-checks')['valid_local_links']
    for batch in progress['batches']:
        if batch['id'] == 'B12':
            batch.update(status='retired', review_status='accepted', runtime_status='destroyed', human_acceptance='accepted')
        if batch['id'] == 'B13':
            batch.update(status='in_progress', package_status='B13a_created', self_check='B13a_passed_with_known_findings', review_status='pending', runtime_status='B13a_running')
            if run.id not in batch['runs']:
                batch['runs'].append(run.id)
            for sub in batch['sub_batches']:
                if sub['id'] == 'B13a':
                    sub.update(status=snapshot['status'], package_status='created', self_check='passed_with_findings', review_status='pending', runtime_status='running',
                               handoff='batches/B13a-review.md', known_findings=snapshot['known_findings'])
    for item in progress['runs']:
        if item['run_id'] == 'p9-b12-20261116-01':
            item.update(status='retired', human_acceptance='accepted')
    if not any(x['run_id'] == run.id for x in progress['runs']):
        progress['runs'].append({'run_id': run.id, 'batch': 'B13a', 'status': snapshot['status'], 'record': str(run.out / 'run.json'), 'human_acceptance': 'pending'})
    previous_out = run.out.parent / 'p9-b12-20261116-01'
    accepted = json.loads((previous_out / 'human-acceptance.json').read_text(encoding='utf-8'))
    if not any(x['run_id'] == 'p9-b12-20261116-01' for x in progress['human_acceptances']):
        progress['human_acceptances'].append({'batch': 'B12', 'run_id': 'p9-b12-20261116-01', 'at': accepted['at'], 'statement': accepted['authorization']})
    if not any(x['run_id'] == 'p9-b12-20261116-01' for x in progress['cleanup_records']):
        progress['cleanup_records'].append({'run_id': 'p9-b12-20261116-01', 'status': 'destroyed', 'record': str(previous_out / 'cleanup-verification.json'), 'at': now()})
    progress['b12_closeout'].update(status='accepted_and_retired', human_acceptance='accepted', runtime_status='retired', preserved=True)
    write_json(path, progress)
    print('B13a handoff sealed with all evidence and known findings; B12 retired; B13b/c remain unstarted', flush=True)


if __name__ == '__main__':
    main()
