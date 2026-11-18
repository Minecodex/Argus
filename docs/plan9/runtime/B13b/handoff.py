"""Seal actual B13b evidence; no user acceptance or next batch is inferred."""
import json
from pathlib import Path
from urllib.parse import urlencode
from manage import Run, ROOT, execute, now, write_json


def main():
    run = Run()
    def read(name):
        return json.loads((run.out / (name + '.json')).read_text(encoding='utf-8-sig'))
    required = ['core-acceptance', 'handling-acceptance', 'grouping-retry-acceptance', 'noise-acceptance', 'health-acceptance',
                'evaluator-permission-acceptance', 'explicit-stop-acceptance', 'postgres-tests', 'delivery-watch', 'current-state',
                'formal-health', 'native-collector-log-check', 'fault-cleanup']
    for name in required:
        assert read(name)['passed'], name
    assert len(read('formal-health')['checks']) == 20
    assert '7 passed' in (run.out / 'browser-chart-final.log').read_text(encoding='utf-8')
    assert 'generated contracts are current (isolated generation)' in (run.out / 'contracts-check.log').read_text(encoding='utf-8')
    assert 'error TS' not in (run.out / 'frontend-typecheck.log').read_text(encoding='utf-8')
    mapping = read('resource-map')
    core = read('core-acceptance')
    first = next(x for x in core['history'] if x['policy_id'] == mapping['alert_policies']['independent']['id'])
    root_url = 'https://argus.dev/alerts?'
    links = {
        'current': root_url + urlencode({'tab': 'active', 'policy_id': mapping['alert_policies']['no_data']['id']}),
        'history': root_url + urlencode({'tab': 'history', 'policy_id': mapping['alert_policies']['independent']['id'], 'instance': first['id']}),
        'deliveries': root_url + urlencode({'tab': 'deliveries', 'channel_id': mapping['notification']['channel_id']}),
        'policy': root_url + urlencode({'tab': 'policies', 'policy': mapping['alert_policies']['every']['id']}),
        'noise': root_url + 'tab=noise', 'channels': root_url + 'tab=channels'}
    release = read('template-release')
    helm = json.loads(execute(['helm', 'list', '--kube-context', 'docker-desktop', '-n', 'argus-system', '-f', '^argus-platform$', '-o', 'json']))[0]
    summary = {'run_id': run.id, 'batch': 'B13b', 'status': 'ready_for_review', 'human_acceptance': 'pending', 'verified_at': now(),
               'online_hosts': 2, 'native_cache_targets': 2, 'applications': 2, 'fresh_collections': 10,
               'native_default_dashboard_panels': 60, 'alert_policies': len(mapping['alert_policies']), 'browser_cases': 7, 'formal_health_checks': 20,
               'native_metric_trend_rendered_in_browser': True,
               'for_seconds': 30, 'recovery_seconds': 20, 'core_real_episodes': 6, 'short_pulse_did_not_fire': True,
               'handling_version_conflict_rejected': True, 'dedup_grouping_and_native_451_retry_verified': True,
               'no_data_error_and_explicit_stop_differentiated': True, 'silence_inhibition_and_release_verified': True,
               'background_scope_and_notification_principal_separate': True, 'current_intentional_no_data_probes': 1,
               'lifetime_terminal_delivery_failures': 0, 'receiver_refusals': 0, 'enqueue_failures': 0, 'counter_reset': False,
               'delivery_watch_seconds': 120, 'model_calls': 0, 'mock_or_replayed_telemetry': False,
               'formal_pvcs_and_deployment_uids_preserved': True, 'backend_image': release['backend_image'], 'helm_revision': int(helm['revision']),
               'schema_migrations_added': False, 'links': links, 'b13c_started': False,
               'resolved_product_issues': ['credential-scoped TLS CA', 'policy labels missing from routing', 'stored/public state separation', 'shared RawMessage buffer mutation'],
               'recorded_execution_failures': ['retired artifact path overlap rejected before runtime start; source checksum manifest partially overwritten',
                   'initial harness threshold/readiness/SMTP envelope assertions corrected with actual evidence preserved',
                   'Windows Node bundle process crash; missing prerequisites restored from frozen source; full generation/check passed']}
    run.save('handoff', summary)
    runtime = read('run')
    runtime.update(status='ready_for_review', human_acceptance='pending', handoff_at=summary['verified_at'])
    run.save('run', runtime)
    transient = {k:v['transient_process_exit_races'] for k,v in read('native-collector-log-check')['hosts'].items()}
    doc = f'''# B13b 查看与运行记录

2026-11-18，运行号 `{run.id}`。**已部署、自检通过、保留现场等待人工查看**。B13c 未启动，Plan9 未标记全部完成。用户“下一轮”授权封存和退役 B13a，不把其三个已知问题或人工验收写成已通过。

## 查看顺序

1. [当前 NoData 示例]({links['current']})：只有一条明确命名的 `B13b deliberate empty metric selection`。它刻意匹配不存在的指标标签，用来显示“无数据”，不是 Collector 故障、不是数值 0；`phase=health` 不匹配本轮通知路由，不向外发送消息。
2. [真实服务告警历史]({links['history']})：查看独立规则的首次触发、确认/指派/评论、恢复及时间线；当前服务均已恢复。独立、AND、OR 长故障共六个实际 Episode，短故障仅 Pending，没有触发。
3. [本地 TLS 通知与尝试记录]({links['deliveries']})：首次通知有一次真实 SMTP 451，再实际重试取得 250；同一告警正文保持一致，每次物理发送批次号允许不同。消息分组、成员和恢复投递均留存。接收人只有本批收件器 `b13b@plan9.invalid`，不代表公网机器人或真实邮箱联通。
4. [AND 多指标策略]({links['policy']})：失败状态与真实内存利用率组合；第二个内存条件使用 5% 作验证门槛，99% 条件作 AND 负控，不是生产阈值建议。
5. [屏蔽与抑制]({links['noise']})：查看 `B13b Reports maintenance window` 与主事件抑制子事件规则。两者均已禁用；历史中保留实际 muted/inhibited 记录。Reports 被静默时 Orders 三条控制事件仍投递，解除后新故障四条事件正常投递。
6. [通知配置]({links['channels']})：本批单独 SMTP 凭据、路由和分组。后台评估账号独立于通知凭据，曾实际撤销其 Reports 数据授权：评估被拒绝，管理员读取及显式本地协议测试仍可完成；随后仅恢复原本批授权。
7. [主机](https://argus.dev/hosts)及[应用](https://argus.dev/applications)：`B13b-Orders-Host`、`B13b-Reports-Host` 在线；Orders/Reports 应用分别为 production/staging，真实 HTTP 请求访问本批 Redis/Memcached。两应用共享一个 Redis 身份。默认仪表盘 60 面板均取得真实数据。管理员已配置两主机限定的七天 Shell/SSH 授权；终端全流程属于既有 B11/后续 B13c 验证，不以本次配置代替该验收。

两个逻辑主机共享 Docker Linux 内核，CPU/内存不能相加解释为独立物理容量。HTTP 流量和收件器最多八小时，故障都有自动恢复；当前所有故障计时器已清理，服务正常，只有上述空匹配 NoData 供查看。

## 实测与门禁

- 30 秒连续固定评估槽、20 秒恢复窗口；独立/AND/OR 与负控、短脉冲、重复评估去重、处理和旧处理版本拒绝均通过。
- 真实静默、同主机抑制、解除后的新周期通过；不删除指标或改成健康状态来冒充降噪。
- NoData 与合法 PromQL 的运行期多对多 Error 都没有伪造零值/成功时间；主动关闭 Memcached 采集得到 `not_evaluated / ALERT_COLLECTION_DISABLED`，恢复后同一目标得到新鲜实际样本。
- 独立只读用户正常登录，只能读取 Orders Episode；响应快照不含 Reports 身份，显式请求 Reports 详情被拒绝。后台评估权限、通知租约和路由各自验证。
- 7 项真实浏览器 E2E 覆盖 1280/1440/1920、中英、明暗、当前/历史/策略/通知/降噪与只读；严格 TLS，无 mock/重放、无模型调用。
- 独立临时 PostgreSQL 上告警领域及新回归通过，容器核对归属后清理。OpenAPI 隔离生成检查、企业门户类型检查、批次目录隔离测试通过。
- 当前两台主机 10 项采集健康新鲜，累计终态丢失/拒绝/入队失败均为 0；120 秒观察无增长、原实例 ID 和计数未重置。旧 B13a 的 2,210 指标点缺口仍保留，不能据此回写成当时零丢失。
- 凭据已迁出 `/root`，父目录权限 700，ProtectHome 保留；没有此前凭据挂载的 filesystem 警告。进程恰好退出时的原生采样竞争另行保留：`{transient}`，不宣称所有原生日志零错误。
- 正式健康 20/20，六个正式 PVC 和 Deployment UID 保留，Helm revision {summary['helm_revision']}。后台精确镜像：`{summary['backend_image']}`。未新增数据库迁移、未改变全局 CA 或 URL 安全策略。

## 本轮修复及失败记录

见 [B13b 实施记录](../37-b13b-alerts-scope-and-notification-20261118.md)。修复了凭据级 CA、策略标签未进入路由、展示裁剪状态误写回及 RawMessage 缓冲区复用覆盖策略。原先评估写回失败/长期 Pending 的日志、失败任务和超时记录保留，修复后在相同实际主机上重新触发/恢复/静默/抑制通过，没有清表制造通过。

另有接入脚本目录隔离错误：新容器启动被旧库存检查拦住，但 B13a 初始构建源码哈希表曾被准备步骤覆盖；运行镜像 ID 依据原 `resource-map` 和实际缓存镜像重核验，不伪造恢复该源码哈希表。B13a 的业务、告警、数据、截图和报告历史保留，失败副本位于 `preflight-path-overlap/`。目录归属与 retired 写保护已补回归。

测试脚本的初次阈值/恢复就绪/SMTP 批次断言错误及 Windows Node bundle 崩溃也保留；后续门禁实际通过。上述历史失败不标成“本轮全程零报错”。

## 证据与后续

证据根：`{run.out.as_posix()}`。主要文件：`handoff.json`、`core-acceptance.json`、`handling-acceptance.json`、`grouping-retry-acceptance.json`、`noise-acceptance.json`、`health-acceptance.json`、`explicit-stop-acceptance.json`、`evaluator-permission-acceptance.json`、`delivery-watch.json`、`current-state.json`、`native-collector-log-check.json`、`formal-health.json`、`template-release.json`、`postgres-tests.log`、`contracts-check.log`、`frontend-typecheck.log`、`browser-chart-final.log` 和 `browser-chart-final/` 截图（已等待真实趋势 SVG 渲染）。密码、会话、服务端私钥和原始诊断放 ACL 限定的 `private/`，不进仓库。

B13a 专属容器/卷/网络/遥测入口已清理，应用/副本/目标归档、测试用户禁用及授权移除；原账号、模型、Workspace 和历史继续保留。B13b 环境等待人工验收，B13c 的门户目标归档入口、完整撤权/轮换/生命周期继续未执行。
'''
    (ROOT / 'docs/plan9/batches/B13b-review.md').write_text(doc, encoding='utf-8')
    path = ROOT / 'docs/plan9/progress.json'
    progress = json.loads(path.read_text(encoding='utf-8'))
    previous = progress.get('current_runtime_validation')
    if previous and previous.get('run_id') != run.id:
        progress.setdefault('runtime_validation_history', []).append(previous)
    progress.update(status='ready_for_review', current_run_id=run.id, next_suggested_batch='B13c', current_runtime_validation=summary)
    progress['documentation'].update(current_review='batches/B13b-review.md', review_entry='batches/B13b-review.md',
                                     issues='37-b13b-alerts-scope-and-notification-20261118.md', b13b='37-b13b-alerts-scope-and-notification-20261118.md')
    progress['validation'].update(scope='B13b_documentation_and_real_runtime', status='passed_with_recorded_failures', runtime_passed=True,
                                  runtime_result='B13b_ready_for_review', report=str(run.out / 'handoff.json'), full_plan_delivery_complete=False)
    progress['delivery'].update(compose_packages='B01_through_B13a_retired_B13b_running', deployment='B13b_ready_for_review',
                                runtime_scripts='native_alert_windows_noise_health_scope_TLS_SMTP', human_acceptance='B13a_advanced_without_explicit_pass_B13b_pending',
                                real_model_calls='none_in_B13b')
    for batch in progress['batches']:
        if batch['id'] == 'B13':
            batch.update(status='in_progress', runtime_status='B13b_running', self_check='B13b_passed_with_recorded_failures', review_status='pending')
            for sub in batch['sub_batches']:
                if sub['id'] == 'B13b':
                    sub.update(status='ready_for_review', package_status='created', runtime_status='running', self_check='passed', review_status='pending', handoff='batches/B13b-review.md')
    for row in progress['runs']:
        if row['run_id'] == run.id:
            row.update(status='ready_for_review', human_acceptance='pending')
    write_json(path, progress)
    print('B13b evidence sealed and ready for review; known failures retained; B13c remains planned', flush=True)


if __name__ == '__main__':
    main()
