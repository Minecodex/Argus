"""Separate collection intent, native service state, dashboard and archive lifecycle."""
import argparse
from datetime import datetime, timedelta, timezone
import json
from manage import Run, now
from integration import Batch
from B13a.archive import archive


def snapshots(batch):
    run, api = batch.run, batch.api
    if (run.out / 'collection-independence.json').exists():
        return
    target = batch.map['targets']['memcached']
    path = '/monitoring/collections/' + target['collection_id']
    control = '/monitoring/collections/' + batch.map['targets']['redis']['collection_id']

    def enable(value):
        row = api.call('GET', path)
        if row['desired_enabled'] != value:
            api.confirm(api.call('POST', '/monitoring/commands/preview', {'kind': 'collection', 'operation': 'set_enabled', 'id': row['id'],
                            'expected_version': row['version'], 'collection': {'enabled': value}}, key='collection-toggle-' + str(row['version']), expected=201))
        batch.converge('reports')
        return api.wait(path, lambda x: x['desired_enabled'] == value and x['apply_status'] == 'effective' and
                        x['effective_revision'] == x['desired_revision'], timeout=90, label='Independent collection intent converges')

    def applied_healthy(after):
        return api.wait(path, lambda x: x['desired_enabled'] and x['health'] == 'healthy' and x['freshness'] == 'fresh'
                        and x.get('last_sample_at', '') > after, timeout=100, label='Actual cache observations resumed')

    start = api.call('GET', path)
    assert start['desired_enabled'] and start['health'] == 'healthy'
    restored = start
    if not (run.out / 'collection-pause-cycle.json').exists():
        try:
            paused = enable(False)
            native = json.loads(run.shell('reports', 'curl -fsS http://127.0.0.1:18085/health\n'))
            assert native['memcached_endpoint'] == 'memcached:11211'
            assert run.container('memcached')['State']['Running']
            assert paused['health'] == 'stopped'
            assert api.call('GET', control)['freshness'] == 'fresh'
        finally:
            enable(True)
        restored = applied_healthy(start['last_sample_at'])
        run.save('collection-pause-cycle', {'at': now(), 'collection_id': start['id'], 'paused': paused, 'after': restored,
                 'native_http_and_memcached_healthy_while_disabled': True, 'other_source_remained_fresh': True})
        print('Pausing collection left the Memcached process and Reports HTTP requests working', flush=True)

    current_target = run.container('memcached')
    try:
        run.compose('stop', '-t', '5', 'memcached', timeout=25)
        stopped_at = now()
        assert not run.container('memcached')['State']['Running']
        status = run.shell('reports', 'curl -sS -o /dev/null -w "%{http_code}" --max-time 8 http://127.0.0.1:18085/health\n')
        assert status == '503'
        report = api.wait(path, lambda x: x['freshness'] == 'stale' and x['desired_enabled'], timeout=150,
                          label='Freshness window elapsed while the real target was down and collection was enabled')
        assert report.get('last_sample_at', '') <= stopped_at
        assert report['version'] == restored['version'] and report['desired_revision'] == restored['desired_revision']
        assert api.call('GET', control)['freshness'] == 'fresh'
        assert json.loads(run.shell('orders', 'curl -fsS http://127.0.0.1:18085/health\n'))['app'] == 'orders'
    finally:
        run.compose('start', 'memcached', timeout=25)
    assert run.container('memcached')['Id'] == current_target['Id']
    recovered = applied_healthy(restored['last_sample_at'])
    replay = json.loads(run.shell('reports', 'curl -fsS --retry 5 --retry-delay 1 http://127.0.0.1:18085/health\n'))
    assert replay['memcached_endpoint'] == 'memcached:11211'
    run.save('collection-independence', {'passed': True, 'at': now(), 'native_stopped_at': stopped_at,
             'native_stop_observed_when_collection_enabled': report, 'cache_recovered': recovered, 'same_native_container': True,
             'collection_id': start['id'], 'target_id': target['target_id'], 'explicit_pause_was_not_resource_stop': True,
             'resource_stop_did_not_change_collection_intent': True})
    print('Stopping actual Memcached left its collection enabled and the independent Orders/Redis control healthy', flush=True)


def copies(batch):
    run, api = batch.run, batch.api
    if (run.out / 'dashboard-lifecycle.json').exists():
        return
    owned = batch.map['personal_dashboard']
    managed_id, copy_id = owned['source_id'], owned['id']
    managed_path, copy_path = '/dashboards/' + managed_id, '/dashboards/' + copy_id
    source = api.call('GET', managed_path)
    personal = api.call('GET', copy_path)
    source_published = source['dashboard']
    baseline = personal['dashboard']
    assert baseline['id'] != source_published['id'] and source_published['managed_template_key'] == 'redis.overview'
    assert not baseline.get('managed_template_key') and baseline['lifecycle'] == 'active'
    end = datetime.now(timezone.utc)
    scope = {'from': (end - timedelta(minutes=10)).isoformat(), 'to': end.isoformat(), 'resource_mode': 'explicit',
             'resource_types': ['monitoring_target'], 'resource_ids': [batch.map['targets']['redis']['target_id']]}
    def preview(path, row, operation, result=201):
        return api.call('POST', path + '/actions/preview', {'id': row['id'], 'operation': operation, 'expected_version': row['version'],
                        'name': row['name'], 'description': row['description'], 'sort_order': 0}, key=operation + '-private-board-' + row['id'], expected=result)

    protected = preview(managed_path, source_published, 'archive', 403)
    if not owned.get('lifecycle_edit_draft'):
        draft = api.call('POST', '/dashboard-drafts', {'dashboard_id': copy_id, 'name': baseline['name'], 'description': baseline['description'],
                         'spec': personal['revision']['spec'], 'proposed_bindings': [{'resource_type': 'monitoring_target', 'resource_id': scope['resource_ids'][0]}]},
                         key='personal-lifecycle-draft', expected=201)
        draft['spec']['panels'][0]['title'] = 'B13c Redis 运行时长 · 轮换后 / Uptime after rotation'
        draft = api.call('PATCH', '/dashboard-drafts/' + draft['id'], {'name': draft['name'], 'description': draft['description'],
                         'spec': draft['spec'], 'proposed_bindings': draft['proposed_bindings'], 'expected_version': draft['draft_version']})
        owned['lifecycle_edit_draft'] = draft['id']
        batch.save()
    draft_path = '/dashboard-drafts/' + owned['lifecycle_edit_draft']
    draft = api.call('GET', draft_path)
    if draft['base_object_version'] != baseline['version']:
        draft = api.call('POST', draft_path + '/rebase', {'expected_version': draft['draft_version'],
                         'object_version': baseline['version'], 'revision_id': baseline['active_revision_id']})
    publication = api.call('POST', draft_path + '/preview', {'expected_version': draft['draft_version'], 'parameters': scope}, key='publication-before-archive', expected=201)
    api.confirm(preview(copy_path, baseline, 'archive'))
    archived = api.call('GET', copy_path)['dashboard']
    assert archived['lifecycle'] == 'archived' and archived['active_revision_id'] == baseline['active_revision_id']
    try:
        unable = api.call('POST', '/enterprise/pending-actions/' + publication['action_ref'] + '/confirm', key='publication-fenced-by-archive', expected=[409, 410])
        assert unable['code'] == 'PENDING_ACTION_INVALIDATED'
        directory = next(x for x in api.call('GET', '/dashboards') if x['id'] == copy_id)
        assert directory['lifecycle'] == 'archived'
        assert api.call('GET', managed_path)['dashboard']['active_revision_id'] == source_published['active_revision_id']
        working = api.call('POST', managed_path + '/execute', scope)
        assert all(x['status'] == 'success' for x in working['panels'])
    finally:
        api.confirm(preview(copy_path, archived, 'restore'))
    restored = api.call('GET', copy_path)['dashboard']
    assert restored['lifecycle'] == 'active' and restored['active_revision_id'] == baseline['active_revision_id']
    rebased = api.call('POST', draft_path + '/rebase', {'expected_version': draft['draft_version'],
                       'object_version': restored['version'], 'revision_id': restored['active_revision_id']})
    assert rebased['spec']['panels'][0]['title'] == draft['spec']['panels'][0]['title']
    new = api.call('POST', draft_path + '/preview', {'expected_version': rebased['draft_version'], 'parameters': scope}, key='publication-after-rebase', expected=201)
    api.confirm(new)
    delivered = api.call('GET', copy_path)
    unchanged = api.call('GET', managed_path)
    assert delivered['revision']['id'] != personal['revision']['id'] and delivered['revision']['spec']['panels'][0]['title'] == draft['spec']['panels'][0]['title']
    assert unchanged['revision']['id'] == source['revision']['id'] and unchanged['revision']['spec_hash'] == source['revision']['spec_hash']
    run.save('dashboard-lifecycle', {'passed': True, 'at': now(), 'managed_id': managed_id, 'personal_id': copy_id, 'managed_archive_denial': protected['code'],
             'managed_spec_hash_unchanged': unchanged['revision']['spec_hash'], 'personal_archive_fenced_old_publication': unable['code'],
             'restored_personal_revision': delivered['revision']['id'], 'managed_panel_count_during_personal_archive': len(working['panels'])})
    print('Managed dashboard protected; personal copy archived independently, restored and republished using a fresh baseline', flush=True)


if __name__ == '__main__':
    p = argparse.ArgumentParser()
    p.add_argument('step', choices=['independence', 'copies', 'prepare-target', 'verify-target'])
    run = Run()
    api = run.api()
    api.login()
    batch = Batch(run, api)
    step = p.parse_args().step
    if step == 'independence':
        snapshots(batch)
    elif step == 'copies':
        copies(batch)
    else:
        archive(batch, 'prepare' if step == 'prepare-target' else 'verify')
