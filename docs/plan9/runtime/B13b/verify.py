"""Current B13b state and real data acceptance without any model calls."""
import argparse
import json
from manage import Run, now
from integration import Batch
from common.delivery import watch


def current(batch):
    run, api = batch.run, batch.api
    for name in ['core-acceptance', 'noise-acceptance', 'health-acceptance', 'evaluator-permission-acceptance', 'explicit-stop-acceptance']:
        assert json.loads((run.out / (name + '.json')).read_text(encoding='utf-8'))['passed'], name
    hosts = api.call('GET', '/enterprise/hosts')['items']
    assert len(hosts) == 2 and all(x['connection_status'] == 'online' and x['labels']['plan9_run'] == run.id for x in hosts)
    collections = api.call('GET', '/monitoring/collections?limit=100')['items']
    assert len(collections) == 10 and all(x['health'] == 'healthy' and x['freshness'] == 'fresh' and x['desired_enabled'] for x in collections)
    assert all(json.loads(run.shell(key, 'curl -fsS http://127.0.0.1:18085/health\n'))['run'] == run.id for key in batch.map['hosts'])
    counters = batch.delivery()
    assert all(not any(v for field in ['terminal_failures', 'receiver_refusals', 'enqueue_failures'] for v in x[field].values()) for x in counters.values())
    current_episodes = api.call('GET', '/alerts/instances?limit=200')['items']
    ids = {v['id'] for v in batch.map['alert_policies'].values()}
    owned = [x for x in current_episodes if x['policy_id'] in ids]
    assert len(owned) == 1 and owned[0]['policy_id'] == batch.map['alert_policies']['no_data']['id'] and owned[0]['health'] == 'no_data'
    assert owned[0]['state'].get('value') is None
    initial = json.loads((run.out / 'template-release-before.json').read_text())
    actual = run.platform()
    assert initial['pvc'] == actual['pvc']
    assert all(actual['deployments'][key]['uid'] == before['uid'] for key, before in initial['deployments'].items())
    run.save('current-state', {'passed': True, 'at': now(), 'hosts_online': 2, 'fresh_healthy_collections': 10, 'lifetime_delivery_counters': counters,
             'exactly_one_intentional_no_data_probe': True, 'intentional_probe_id': owned[0]['id'], 'business_services_healthy': True,
             'model_calls': 0, 'formal_pvcs_and_deployment_uids_preserved': True})
    print('Two hosts and ten collections healthy; lifetime delivery losses/refusals zero; one explicit empty-match probe retained', flush=True)


if __name__ == '__main__':
    p = argparse.ArgumentParser()
    p.add_argument('action', choices=['watch', 'current'])
    run = Run()
    api = run.api()
    api.login()
    globals()[p.parse_args().action](Batch(run, api))
