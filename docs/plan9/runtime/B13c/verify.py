"""Assert the live control, final inventory, and historical data after uninstall."""
import argparse
import json
from manage import Run, now
from integration import Batch
from B13a.archive import projection
from common.delivery import watch


def current(batch):
    run, api = batch.run, batch.api
    def read(name):
        return json.loads((run.out / (name + '.json')).read_text(encoding='utf-8-sig'))
    required = ['revocation-acceptance', 'ssh-rotation', 'redis-rotation', 'collection-independence', 'dashboard-lifecycle',
                'portal-archive-dependencies', 'portal-target-archive', 'archival-acceptance', 'reports-decommission']
    for name in required:
        assert read(name)['passed'], name
    orders, reports = [batch.map['hosts'][key]['host_id'] for key in ['orders', 'reports']]
    active = api.call('GET', '/enterprise/hosts')['items']
    assert len(active) == 1 and active[0]['id'] == orders and active[0]['connection_status'] == 'online'
    removed = api.call('GET', '/enterprise/hosts/' + reports, expected=404)
    assert removed['code']
    collections = api.call('GET', '/monitoring/collections?limit=100')['items']
    assert len(collections) == 5 and all(x['desired_enabled'] and x['health'] == 'healthy' and x['freshness'] == 'fresh' for x in collections)
    apps = api.call('GET', '/applications?search=B13c')['items']
    assert [x['id'] for x in apps] == [batch.map['applications']['orders']['id']]
    targets = api.call('GET', '/monitoring/targets?search=B13c&limit=100')['items']
    assert [x['id'] for x in targets] == [batch.map['targets']['redis']['target_id']]
    assert all(not run.container(key)['State']['Running'] for key in ['reports', 'memcached'])
    assert json.loads(run.shell('orders', 'curl -fsS http://127.0.0.1:18085/health\n'))['run'] == run.id
    range_input = batch.map['archive_probe']['input']
    board = batch.map['dashboards']['memcached']['id']
    value = api.call('POST', '/dashboards/' + board + '/execute', range_input)
    value.pop('context_token', None)
    assert projection(read('archival-history-before')) == projection(value)
    run.save('archival-history-after-uninstall', value)
    grant = api.call('GET', '/enterprise/remote-access-grants/' + batch.map['remote']['grant_id'])
    assert grant['status'] == 'enabled' and grant['host_ids'] == [orders]
    member_scope = api.call('GET', '/enterprise/data-authorizations/role/' + batch.map['reader']['role_id'] + '?resource_type=host')
    assert {x['resource_id'] for x in member_scope['items'] if x['direct']} == {orders}
    native = json.loads(run.shell('orders', 'cat /var/lib/argus-otelcol/identity/delivery-state.json\n'))['report']
    fields = ['terminal_failures', 'receiver_refusals', 'enqueue_failures']
    assert all(native[field] == read('delivery-before-builtins')['orders'][field] for field in fields)
    before, after = read('template-release-before'), run.platform()
    assert before['pvc'] == after['pvc'] and all(after['deployments'][key]['uid'] == row['uid'] for key, row in before['deployments'].items())
    run.save('current-state', {'passed': True, 'at': now(), 'live_hosts': 1, 'live_targets': 1, 'live_applications': 1,
             'healthy_fresh_collections': 5, 'archived_history_after_uninstall_same': True, 'frozen_historical_panels': len(value['panels']),
             'retired_containers_stopped_for_review': ['reports', 'memcached'], 'native_delivery': native,
             'formal_pvcs_and_deployment_ids_preserved': True, 'model_calls': 0})
    print('One online control host and Redis with five fresh collections; archived data still identical after actual uninstall', flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('step', choices=['current', 'watch'])
    run = Run()
    api = run.api()
    api.login()
    globals()[parser.parse_args().step](Batch(run, api))
