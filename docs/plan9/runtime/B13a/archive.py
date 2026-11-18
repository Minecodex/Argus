"""Archive the owned Memcached target only after dependencies end; retain history."""
from datetime import datetime, timedelta, timezone
import json
from integration import Batch
from manage import Run, now


def projection(value):
    return [{'id': p['id'], 'status': p['status'], 'targets': [{'id': t['id'], 'status': t['status'], 'result_type': t.get('result_type'), 'data': t.get('data')} for t in p['targets']]} for p in value['panels']]


def archive(batch, phase='all'):
    assert phase in ['all', 'prepare', 'verify']
    run, api = batch.run, batch.api
    target = batch.map['targets']['memcached']
    ident = target['target_id']
    board = batch.map['dashboards']['memcached']['id']
    state = batch.map.setdefault('archive_probe', {})
    if 'input' not in state:
        end = datetime.now(timezone.utc) - timedelta(seconds=60)
        state['input'] = {'from': (end - timedelta(minutes=12)).isoformat(), 'to': end.isoformat(), 'resource_mode': 'explicit',
                          'resource_types': ['monitoring_target'], 'resource_ids': [ident]}
        batch.save()
    path = '/dashboards/' + board + '/execute'
    if 'before' not in state:
        result = api.call('POST', path, state['input'])
        result.pop('context_token', None)
        assert all(x['status'] == 'success' for x in result['panels'])
        run.save('archival-history-before', result)
        state['before'] = True
        batch.save()
    collection = api.call('GET', '/monitoring/collections/' + target['collection_id'])
    current_target = next((x for x in api.call('GET', '/monitoring/targets?integration=memcached&limit=200')['items'] if x['id'] == ident), None)
    if phase == 'verify':
        assert not current_target, 'Portal archive must complete before checking history'
    if current_target:
        preview_body = {'kind': 'target', 'operation': 'archive', 'id': ident, 'expected_version': current_target['version']}
        if not state.get('dependency_rejection'):
            rejected = api.call('POST', '/monitoring/commands/preview', preview_body, key='archive-blocked-dependencies', expected=409)
            state['dependency_rejection'] = rejected['code']
            batch.save()
        if collection['status'] == 'active':
            api.confirm(api.call('POST', '/monitoring/commands/preview', {'kind': 'collection', 'operation': 'archive', 'id': collection['id'],
                            'expected_version': collection['version'], 'collection': {}}, key='archive-memcached-collection', expected=201))
            batch.converge('reports')
        binding_id = batch.map['applications']['reports']['bindings']['memcached']
        if not state.get('binding_ended'):
            api.confirm(api.call('POST', '/monitoring/commands/preview', {'kind': 'binding', 'operation': 'delete', 'id': binding_id,
                            'expected_version': 1}, key='end-memcached-application-binding', expected=201))
            state['binding_ended'] = True
            batch.save()
        bindpath = '/monitoring/targets/' + ident + '/dashboard-bindings'
        bindings = api.call('GET', bindpath)
        for binding in bindings['items']:
            api.confirm(api.call('POST', bindpath + '/preview', {'operation': 'detach', 'dashboard_id': binding['dashboard']['id'],
                            'binding_id': binding['id'], 'expected_binding_version': binding['version'],
                            'expected_dashboard_version': binding['dashboard']['version'], 'expected_resource_version': bindings['resource_version']},
                            key='end-memcached-board-' + binding['dashboard']['id'], expected=201))
            bindings = api.call('GET', bindpath)
        if phase == 'prepare':
            state['dependencies_retired'] = True
            batch.save()
            return
        current = next(x for x in api.call('GET', '/monitoring/targets?integration=memcached&limit=200')['items'] if x['id'] == ident)
        state['archive_execution'] = api.confirm(api.call('POST', '/monitoring/commands/preview', {'kind': 'target', 'operation': 'archive', 'id': ident,
                        'expected_version': current['version']}, key='archive-memcached-target', expected=201))
        batch.save()
    after = api.call('POST', path, state['input'])
    after.pop('context_token', None)
    run.save('archival-history-after', after)
    before = json.loads((run.out / 'archival-history-before.json').read_text(encoding='utf-8'))
    assert projection(before) == projection(after), 'Archival changed the frozen historical result'
    collection = api.call('GET', '/monitoring/collections/' + target['collection_id'])
    assert collection['status'] == 'archived' and collection['target']['id'] == ident
    assert not any(x['id'] == ident for x in api.call('GET', '/monitoring/targets?integration=memcached&limit=200')['items'])
    state.update(passed=True, target_id=ident, ended_at=now(), historical_panels=len(after['panels']), frozen_result_identical=True,
                 archived_collection_id=target['collection_id'], removed_from_current_inventory=True, raw_data_and_source_identity_retained=True)
    batch.save()
    run.save('archival-acceptance', state)
    print('Actual target archived; current inventory excludes it and all historical panel data remains identical', flush=True)


if __name__ == '__main__':
    run = Run()
    api = run.api()
    api.login()
    archive(Batch(run, api))
