"""Differentiate missing data, query Error, explicit stop and evaluator grants."""
import json
from configuration import create, enabled, policy, rule
from integration import Batch
from manage import Run, now
from scenario import instances, wait_instances, deliveries


def health_case(batch, key, expected):
    row = enabled(batch, 'policy', key, True)
    trial = batch.api.call('POST', '/alerts/trial', row['spec'])
    assert trial['health'] == expected, trial
    batch.api.wait('/alerts/configurations/' + row['id'], lambda x: x['health'] == expected, timeout=60, label=key + ' evaluated health')
    rows = wait_instances(batch, lambda r: any(x['state']['kind'] == 'health' and x['health'] == expected for x in r), [key], timeout=60, name=key + '-health')
    episode = next(x for x in rows if x['state']['kind'] == 'health' and x['health'] == expected)
    assert episode['state'].get('value') is None and not episode['state'].get('last_successful_at')
    batch.run.save(key + '-acceptance', {'passed': True, 'at': now(), 'trial': trial, 'instance': episode,
                                       'no_fabricated_zero_or_healthy_recovery': True})
    return episode


def stop_collection(batch):
    run, api = batch.run, batch.api
    if (run.out / 'explicit-stop-acceptance.json').exists():
        return
    target = batch.map['targets']['memcached']
    spec = policy(batch, [rule('Actual Memcached memory guard', 'memcached_bytes', 1048576, 524288)], phase='health', enabled=False, hold=0, routed=False)
    spec['resources'] = {'mode': 'explicit', 'types': ['monitoring_target'], 'ids': [target['target_id']], 'source_ids': [], 'matchers': []}
    spec['rules'][0]['unit'] = 'bytes'
    create(batch, 'policy', 'cache_health', 'B13b explicit cache collection stop', spec)
    probe = enabled(batch, 'policy', 'cache_health', True)
    baseline = api.call('POST', '/alerts/trial', probe['spec'])
    assert baseline['health'] == 'normal' and baseline['observations']
    before = api.call('GET', '/monitoring/collections/' + target['collection_id'])
    stopped = False
    try:
        api.confirm(api.call('POST', '/monitoring/commands/preview', {'kind': 'collection', 'operation': 'set_enabled', 'id': before['id'],
                        'expected_version': before['version'], 'collection': {'enabled': False}}, key='controlled-cache-stop', expected=201))
        stopped = True
        batch.converge('reports')
        trial = api.call('POST', '/alerts/trial', probe['spec'])
        assert trial['health'] == 'not_evaluated' and trial['error_code'] == 'ALERT_COLLECTION_DISABLED', trial
        api.wait('/alerts/configurations/' + probe['id'], lambda x: x['health'] == 'not_evaluated', timeout=60, label='Explicit collection stop is suspended')
        run.save('explicit-stop-under-test', {'at': now(), 'baseline': baseline, 'stopped': trial, 'service_was_not_failed': True})
    finally:
        if stopped:
            current = api.call('GET', '/monitoring/collections/' + target['collection_id'])
            if not current['desired_enabled']:
                api.confirm(api.call('POST', '/monitoring/commands/preview', {'kind': 'collection', 'operation': 'set_enabled', 'id': current['id'],
                                'expected_version': current['version'], 'collection': {'enabled': True}}, key='controlled-cache-resume', expected=201))
            batch.converge('reports')
    fresh = api.wait('/monitoring/collections/' + target['collection_id'], lambda x: x['health'] == 'healthy' and x['freshness'] == 'fresh' and
                     x['last_sample_at'] > before['last_sample_at'], timeout=90, label='Native Memcached samples resume')
    recovered = api.call('POST', '/alerts/trial', probe['spec'])
    assert recovered['health'] == 'normal' and recovered['observations']
    enabled(batch, 'policy', 'cache_health', False)
    run.save('explicit-stop-acceptance', {'passed': True, 'at': now(), 'collection_id': target['collection_id'], 'stopped_health': 'not_evaluated',
                                        'stopped_reason': 'ALERT_COLLECTION_DISABLED', 'fresh_after_resume': fresh['last_sample_at'], 'native_trial_after_resume': recovered})
    print('Explicit native cache stop suspended evaluation; fresh data resumed through the same source identity', flush=True)


def evaluation_scope(batch):
    run, api = batch.run, batch.api
    if (run.out / 'evaluator-permission-acceptance.json').exists():
        return
    states = {key: api.call('GET', '/alerts/configurations/' + p['id'])['spec']['enabled'] for key, p in batch.map['alert_policies'].items()}
    for key, active in states.items():
        if active:
            enabled(batch, 'policy', key, False)
    path = '/enterprise/data-authorizations/role/' + batch.map['evaluator']['role_id']
    hid = batch.map['hosts']['reports']['host_id']
    probe = api.call('GET', '/alerts/configurations/' + batch.map['alert_policies']['cpu_health']['id'])
    baseline = api.call('POST', '/alerts/trial', probe['spec'])
    assert baseline['health'] == 'normal'
    removed = False
    try:
        grant = api.call('GET', path + '?resource_type=host')
        assert any(x['direct'] and x['resource_id'] == hid for x in grant['items'])
        api.call('POST', path, {'resource_type': 'host', 'resource_ids': [hid], 'remove': True, 'expected_version': grant['authorization_version']}, expected=204)
        removed = True
        api.login()
        denied = api.call('POST', '/alerts/trial', probe['spec'], expected=403)
        assert api.call('GET', '/enterprise/hosts/' + hid)['id'] == hid
        cid = batch.map['notification']['channel_id']
        channel = api.call('GET', '/alerts/configurations/' + cid)
        before = {x['id'] for x in deliveries(batch)}
        api.confirm(api.call('POST', '/alerts/commands/preview', {'operation': 'notification.test', 'id': cid, 'expected_version': channel['version']}, key='notification-independent-of-evaluator-grant', expected=202))
        mail = api.wait('/alerts/deliveries?channel_id=' + cid, lambda p: any(x['id'] not in before and x['status'] == 'accepted' and x['message']['event_type'] == 'test' for x in p['items']),
                        timeout=90, label='Generic owned notification does not borrow evaluator data grants')
        run.save('evaluator-denied-under-test', {'at': now(), 'denial': denied, 'admin_host_read_still_allowed': True,
                 'notification_test': [x for x in mail['items'] if x['id'] not in before]})
    finally:
        if removed:
            current = api.call('GET', path + '?resource_type=host')
            api.call('POST', path, {'resource_type': 'host', 'resource_ids': [hid], 'remove': False, 'expected_version': current['authorization_version']}, expected=204)
            api.login()
        for key, active in states.items():
            if active:
                enabled(batch, 'policy', key, True)
    assert api.call('POST', '/alerts/trial', probe['spec'])['health'] == 'normal'
    run.save('evaluator-permission-acceptance', {'passed': True, 'at': now(), 'revoked_background_scope_rejected': True, 'exact_owned_grant_restored': True,
                                              'admin_permissions_unchanged': True, 'notification_principal_and_routing_independent': True})
    print('Background grant revocation blocked evaluation while admin read and generic local notification remained independent', flush=True)


def exercise(batch):
    run = batch.run
    assert (run.out / 'noise-acceptance.json').exists()
    for key in ['negative', 'cpu_health', 'no_data', 'query_error']:
        row = batch.api.call('GET', '/alerts/configurations/' + batch.map['alert_policies'][key]['id'])
        wanted = [batch.map['configurations']['route']['owned']]
        if row['spec']['route_ids'] != wanted:
            row['spec']['route_ids'] = wanted
            batch.api.confirm(batch.api.call('POST', '/alerts/configurations/preview', {'kind': 'policy', 'operation': 'update', 'id': row['id'],
                             'name': row['name'], 'expected_version': row['version'], 'spec': row['spec']}, key='pin-probe-route-' + key, expected=202))
    health_case(batch, 'no_data', 'no_data')
    try:
        health_case(batch, 'query_error', 'error')
    finally:
        enabled(batch, 'policy', 'query_error', False)
    stop_collection(batch)
    evaluation_scope(batch)
    # Keep the explicitly named empty-match probe for manual inspection; the
    # owned route does not match phase=health. It is not a Collector outage.
    enabled(batch, 'policy', 'no_data', True)
    run.save('health-acceptance', {'passed': True, 'at': now(), 'no_data_is_not_zero': True, 'error_is_not_healthy_recovery': True,
                                 'explicit_stop_is_not_no_data': True, 'one_intentional_empty_match_probe_kept': True, 'no_health_probe_notification_route': True})


if __name__ == '__main__':
    run = Run()
    api = run.api()
    api.login()
    exercise(Batch(run, api))
