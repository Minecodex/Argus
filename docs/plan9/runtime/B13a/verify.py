"""Real scope, data and delivery evidence; never substitutes mock telemetry."""
import argparse
import json
import time
from access import reader
from dashboards import execute
from integration import Batch
from manage import Run, now


def resource_set(value):
    return {(x['type'], x['id']) for x in value['resources']}


def panel(value, ident):
    return next(x for x in value['panels'] if x['id'] == ident)


def series(value, ident):
    return [x for t in panel(value, ident)['targets'] for x in (t.get('data') or [])]


def scope(batch):
    app = batch.map['applications']
    hosts = batch.map['hosts']
    targets = batch.map['targets']
    orders = {('host', hosts['orders']['host_id']), ('monitoring_target', targets['redis']['target_id'])}
    reports = {('host', hosts['reports']['host_id']), ('monitoring_target', targets['redis']['target_id']), ('monitoring_target', targets['memcached']['target_id'])}
    both = execute(batch, name='scope-both')
    assert resource_set(both) == orders | reports
    assert all(p['status'] == 'success' for p in both['panels'])
    assert len(series(both, 'redis-memory')) == 1
    assert len(series(both, 'redis-targets')) == 1 and float(series(both, 'redis-targets')[0]['value'][1]) == 1
    for key, expected in [('orders', orders), ('reports', reports)]:
        value = execute(batch, name='scope-' + key, application_ids=[app[key]['id']])
        assert resource_set(value) == expected, (key, value['resources'])
        assert float(series(value, 'redis-targets')[0]['value'][1]) == 1
    empty = execute(batch, name='scope-empty-resources', resource_mode='explicit', resource_ids=[])
    assert empty['scope_empty'] and not empty['resources']
    assert not any(series(empty, p['id']) for p in empty['panels'])
    empty_apps = execute(batch, name='scope-empty-applications', application_ids=[])
    assert empty_apps['scope_empty'] and not empty_apps['resources']
    local = execute(batch, name='scope-local-free', local_values={'memory': {'state': {'all': False, 'values': ['free']}}})
    assert resource_set(local) == resource_set(both)
    assert len(series(local, 'memory')) == 2 and all(x['metric']['state'] == 'free' for x in series(local, 'memory'))
    used = execute(batch, name='scope-local-used', local_values={'memory': {'state': {'all': False, 'values': ['used']}}})
    assert all(x['metric']['state'] == 'used' for x in series(used, 'memory'))
    no_match = execute(batch, name='scope-local-empty', local_values={'memory': {'state': {'all': False, 'values': ['b13a-does-not-exist']}}})
    assert not series(no_match, 'memory') and len(series(no_match, 'redis-memory')) == 1
    member = reader(batch)
    limited = execute(batch, api=member, name='scope-member-all', application_mode='all_authorized', application_ids=[])
    assert resource_set(limited) == orders
    assert not series(limited, 'memcached-memory')
    end = both['to']
    denials = []
    for name, body in [('application', {'application_mode': 'explicit', 'application_ids': [app['reports']['id']]}),
                       ('host', {'resource_mode': 'explicit', 'resource_types': ['host'], 'resource_ids': [hosts['reports']['host_id']]}),
                       ('target', {'resource_mode': 'explicit', 'resource_types': ['monitoring_target'], 'resource_ids': [targets['memcached']['target_id']]})]:
        result = member.call('POST', '/dashboards/' + batch.map['batch_dashboard']['id'] + '/execute',
                             {'from': both['from'], 'to': end, **body}, expected=[403, 404])
        denials.append({'kind': name, 'code': result['code'], 'request_id': result.get('request_id')})
    batch.run.save('scope-acceptance', {'passed': True, 'at': now(), 'both_resource_count': len(both['resources']),
                                      'shared_redis_series': 1, 'unique_redis_count': 1, 'member_resource_count': 2,
                                      'empty_resource_scope_preserved': True, 'empty_application_scope_preserved': True,
                                      'local_condition_only_narrows_its_panel': True, 'explicit_unauthorized_denials': denials})
    print('Scope matrix passed: 4 unique resources, one shared Redis, 2 reader resources, 3 explicit denials', flush=True)


def watch(batch):
    def reports():
        return {key: json.loads(batch.run.shell(key, 'cat /var/lib/argus-otelcol/identity/delivery-state.json\n'))['report'] for key in ['orders', 'reports']}
    before = reports()
    batch.run.save('delivery-watch-baseline', {'at': now(), 'reports': before})
    observations = [before]
    started = now()
    for index in range(4):
        time.sleep(30)
        current = reports()
        for key, value in current.items():
            assert value['instance_id'] == before[key]['instance_id'] and value['sequence'] > before[key]['sequence']
            for field in ['terminal_failures', 'receiver_refusals', 'enqueue_failures']:
                assert value[field] == before[key][field], (key, field, before[key][field], value[field])
            rows = batch.api.call('GET', '/monitoring/builtins?target_type=host&target_id=' + batch.map['hosts'][key]['host_id'])
            active = [x['collection'] for x in rows if (x.get('collection') or {}).get('desired_enabled')]
            assert len(active) == 4 and all(x['health'] == 'healthy' and x['freshness'] == 'fresh' for x in active)
        for target in batch.map['targets'].values():
            value = batch.api.call('GET', '/monitoring/collections/' + target['collection_id'])
            if value['status'] == 'active':
                assert value['health'] == 'healthy' and value['freshness'] == 'fresh'
        observations.append(current)
        print('Both Collector deliveries stable: ' + str(index + 1) + '/4', flush=True)
    batch.run.save('delivery-watch', {'passed': True, 'started_at': started, 'ended_at': now(), 'seconds': 120, 'observations': observations,
                                     'terminal_failure_increase': 0, 'receiver_refusal_increase': 0, 'enqueue_failure_increase': 0,
                                     'historical_baseline_retained': before, 'counter_reset': False})


if __name__ == '__main__':
    p = argparse.ArgumentParser()
    p.add_argument('action', choices=['scope', 'watch'])
    run = Run()
    api = run.api()
    api.login()
    globals()[p.parse_args().action](Batch(run, api))
