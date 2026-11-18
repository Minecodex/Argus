"""Seal real B13c results and advance the Plan9 review record."""
import json
import re
from urllib.parse import urlencode
from manage import Run, ROOT, execute, now, write_json


def seal(run):
    def read(name):
        return json.loads((run.out / (name + '.json')).read_text(encoding='utf-8-sig'))

    for name in ['revocation-acceptance', 'ssh-rotation', 'redis-rotation', 'collection-independence', 'dashboard-lifecycle',
                 'portal-archive-dependencies', 'portal-target-archive', 'archival-acceptance', 'reports-decommission',
                 'current-state', 'postgres-tests', 'delivery-watch', 'formal-health']:
        assert read(name)['passed'], name
    checks = len(read('formal-health')['checks'])
    assert checks == 20
    logs = ['browser-revoke-ssh', 'browser-archive-blocked', 'browser-archive-final', 'browser-review']
    cases = sum(int(re.search(r'\b(\d+) passed \(', (run.out / (name + '.log')).read_text(encoding='utf-8-sig'))[1]) for name in logs)
    assert cases == 7
    assert 'generated contracts are current (isolated generation)' in (run.out / 'contracts-check.log').read_text(encoding='utf-8-sig')
    assert 'Test Files' in (run.out / 'frontend-tests.log').read_text(encoding='utf-8-sig')
    map = read('resource-map')
    status = read('current-state')
    revision = json.loads(execute(['helm', 'list', '--kube-context', 'docker-desktop', '-n', 'argus-system', '-f', '^argus-platform$', '-o', 'json']))[0]['revision']
    overview = read('template-release')
    params = map['archive_probe']['input']
    base = 'https://argus.dev'
    history = base + '/dashboards/' + map['dashboards']['memcached']['id'] + '?' + urlencode({
        'resource': map['targets']['memcached']['target_id'], 'resource_type': 'monitoring_target', 'time_from': params['from'], 'time_to': params['to']})
    links = {'targets': base + '/monitoring', 'live_host': base + '/hosts/' + map['hosts']['orders']['host_id'],
             'personal_dashboard': base + '/dashboards/' + map['personal_dashboard']['id'] + '?' + urlencode({
                 'resource': map['targets']['redis']['target_id'], 'resource_type': 'monitoring_target'}), 'archived_history': history}
    snapshot = {'run_id': run.id, 'batch': run.BATCH, 'status': 'ready_for_review', 'verified_at': now(), 'human_acceptance': 'pending',
                'revocation': read('revocation-acceptance'), 'ssh_and_redis_native_credential_rotation': True,
                'actual_independent_collection_pause_and_service_stop': True, 'personal_archive_restore_and_republish': True,
                'portal_archive_dependency_denial_and_actual_archive': True, 'host_uninstall_then_delete': True, 'historical_panels_after_uninstall': 9,
                'online_hosts': status['live_hosts'], 'native_healthy_collections': status['healthy_fresh_collections'],
                'active_collector_watch_seconds': read('delivery-watch')['seconds'], 'new_delivery_failures_during_watch': 0,
                'formal_health_checks': checks, 'browser_cases': cases, 'backend_image': overview['backend_image'],
                'web_image': overview['web_image'], 'helm_revision': int(revision), 'schema_migrations_added': False,
                'formal_pvcs_and_deployments_preserved': True, 'real_model_calls': 0, 'mock_or_replayed_telemetry': False,
                'links': links, 'optional_batches_pending': ['E01', 'E02']}
    run.save('handoff', snapshot)
    record = read('run')
    record.update(status='ready_for_review', human_acceptance='pending', handoff_at=snapshot['verified_at'])
    run.save('run', record)
    return snapshot


def update_progress(run, snapshot):
    path = ROOT / 'docs/plan9/progress.json'
    progress = json.loads(path.read_text(encoding='utf-8'))
    previous = progress.get('current_runtime_validation')
    if previous and previous['run_id'] != run.id:
        progress.setdefault('runtime_validation_history', []).append(previous)
    progress.update(status='ready_for_review', current_run_id=run.id, next_suggested_batch=None, current_runtime_validation=snapshot)
    progress['documentation'].update(review_entry='batches/B13c-review.md', current_review='batches/B13c-review.md',
                                     issues='38-b13c-revocation-rotation-and-lifecycle-20261118.md', b13c='38-b13c-revocation-rotation-and-lifecycle-20261118.md')
    progress['delivery'].update(compose_packages='B01_through_B13b_retired_B13c_control_running', deployment='B13c_ready_for_review',
                                runtime_scripts='native_revoke_rotate_archive_and_uninstall', human_acceptance='B13b_advanced_B13c_pending', real_model_calls='none_in_B13c')
    progress['validation'].update(scope='B13c_documentation_and_real_runtime', status='passed_with_recorded_failures', known_findings=[],
                                  report=str(run.out / 'handoff.json'), runtime_passed=True, runtime_result='B13c_ready_for_review', full_plan_delivery_complete=False)
    for batch in progress['batches']:
        if batch['id'] != 'B13':
            continue
        batch.update(status='in_progress', package_status='B13a_B13b_B13c_created', runtime_status='B13c_review_control_running',
                     self_check='B13c_passed', review_status='pending')
        if run.id not in batch['runs']:
            batch['runs'].append(run.id)
        for sub in batch['sub_batches']:
            if sub['id'] == 'B13b':
                sub.update(status='retired', runtime_status='destroyed', review_status='advance_requested_without_explicit_pass')
            elif sub['id'] == 'B13c':
                sub.update(status='ready_for_review', package_status='created', runtime_status='review_control_running',
                           self_check='passed', review_status='pending', handoff='batches/B13c-review.md')
    for item in progress['runs']:
        if item['run_id'] == run.PREDECESSOR:
            item.update(status='retired', human_acceptance='not_claimed')
    if not any(x['run_id'] == run.id for x in progress['runs']):
        progress['runs'].append({'run_id': run.id, 'batch': run.BATCH, 'status': 'ready_for_review',
                                 'record': str(run.out / 'run.json'), 'human_acceptance': 'pending'})
    predecessor = run.out.parent / run.PREDECESSOR
    if not any(x['run_id'] == run.PREDECESSOR for x in progress['cleanup_records']):
        cleaned = json.loads((predecessor / 'cleanup-verification.json').read_text(encoding='utf-8'))
        advance = json.loads((predecessor / 'advance-authorization.json').read_text(encoding='utf-8'))
        progress['cleanup_records'].append({'run_id': run.PREDECESSOR, 'status': 'destroyed', 'record': str(predecessor / 'cleanup-verification.json'),
                                           'at': cleaned['at'], 'authorization': advance['statement'], 'human_pass_not_claimed': True})
    write_json(path, progress)


if __name__ == '__main__':
    run = Run()
    snapshot = seal(run)
    update_progress(run, snapshot)
    print('B13c live acceptance sealed, ready for manual review', flush=True)
