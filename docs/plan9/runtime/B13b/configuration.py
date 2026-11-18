"""Real-rule fixtures through existing preview/confirm, not telemetry fabrication."""
import argparse
import copy
import json
from datetime import datetime, timedelta, timezone
from integration import Batch
from manage import Run

FAILED = 'max by (argus_resource_id) (systemd_unit_state{resource_systemd_unit_name="plan9-b13b.service",systemd_unit_active_state="failed"})'
MEMORY = 'max by (argus_resource_id) (system_memory_utilization{state="used"})'
CPU = '1 - avg by (argus_resource_id) (system_cpu_utilization{state="idle"})'


def create(batch, kind, key, name, spec):
    state = batch.map.setdefault('configurations', {}).setdefault(kind, {})
    if key in state:
        return batch.api.call('GET', '/alerts/configurations/' + state[key])
    matching = [x for x in batch.api.call('GET', '/alerts/configurations?kind=' + kind) if x['name'] == name]
    assert len(matching) <= 1
    if not matching:
        batch.api.confirm(batch.api.call('POST', '/alerts/configurations/preview', {'kind': kind, 'operation': 'create', 'name': name, 'spec': spec},
                          key=kind + '-' + key, expected=202))
        matching = [x for x in batch.api.call('GET', '/alerts/configurations?kind=' + kind) if x['name'] == name]
    assert len(matching) == 1
    row = matching[0]
    state[key] = row['id']
    if kind == 'policy':
        batch.map['alert_policies'][key] = {'id': row['id'], 'name': name}
    batch.save()
    return row


def enabled(batch, kind, key, value):
    ident = batch.map['configurations'][kind][key]
    row = batch.api.call('GET', '/alerts/configurations/' + ident)
    if row['spec']['enabled'] != value:
        batch.api.confirm(batch.api.call('POST', '/alerts/configurations/preview', {'kind': kind, 'operation': 'enable' if value else 'disable',
                          'id': ident, 'expected_version': row['version']}, key='enabled-' + ident + '-' + str(row['version']) + '-' + str(value), expected=202))
    return batch.api.call('GET', '/alerts/configurations/' + ident)


def rule(name, expression, threshold, recovery=0, severity='warning'):
    return {'name': name, 'expression': expression, 'query_mode': 'numeric', 'operator': '>', 'threshold': threshold,
            'recovery_threshold': recovery, 'severity': severity, 'unit': ''}


def policy(batch, rules, mode='independent', phase='core', enabled=True, hosts=None, sources=None, hold=30, routed=True):
    return {'description': 'Controlled real B13b metrics. Native data and authorization remain authoritative.', 'enabled': enabled,
            'resources': {'mode': 'explicit', 'types': ['host'], 'ids': hosts or [x['host_id'] for x in batch.map['hosts'].values()],
                          'source_ids': sources or [], 'matchers': []},
            'rules': rules, 'mode': mode, 'match_by': ['argus_resource_id'], 'interval_seconds': 10, 'for_seconds': hold,
            'recovery_seconds': 20, 'no_data_behavior': 'report', 'error_behavior': 'report', 'channel_ids': [],
            # An empty route list means all global routes, not no route. Keep
            # every fixture pinned to our one route; health/negative phases do
            # not match that route and therefore never notify another channel.
            'route_ids': [batch.map['configurations']['route']['owned']],
            'labels': {'plan9_run': batch.run.id, 'plan9_batch': 'b13b', 'phase': phase}}


def configure(batch):
    api = batch.api
    cid = batch.map['notification']['channel_id']
    route_spec = {'enabled': True, 'matchers': [{'key': 'plan9_run', 'operator': '=', 'value': batch.run.id},
                  {'key': 'phase', 'operator': '=~', 'value': '^(core|child)$'}], 'channel_ids': [cid], 'continue': False, 'priority': 10}
    route = create(batch, 'route', 'owned', 'B13b owned notification route', route_spec)
    if route['spec'] != route_spec:
        api.confirm(api.call('POST', '/alerts/configurations/preview', {'kind': 'route', 'operation': 'update', 'id': route['id'],
                        'expected_version': route['version'], 'name': route['name'], 'spec': route_spec}, key='scope-owned-route-phases', expected=202))
    rows = api.call('GET', '/alerts/configurations?kind=grouping')
    grouping = {'group_by': ['plan9_run', 'phase', 'severity'], 'wait_seconds': 5, 'interval_seconds': 5, 'repeat_seconds': 3600,
                'timing': ['first', 'recovery', 'severity']}
    assert len(rows) <= 1
    if not rows:
        create(batch, 'grouping', 'owned', 'B13b grouping', grouping)
    elif rows[0]['spec'] != grouping:
        assert all(x['name'].startswith('B13b') for x in api.call('GET', '/alerts/configurations?kind=policy'))
        batch.map.setdefault('prior_grouping', rows[0])
        batch.save()
        api.confirm(api.call('POST', '/alerts/configurations/preview', {'kind': 'grouping', 'operation': 'update', 'id': rows[0]['id'],
                        'expected_version': rows[0]['version'], 'name': 'B13b grouping', 'spec': grouping}, key='owned-grouping', expected=202))
        batch.map['configurations'].setdefault('grouping', {})['owned'] = rows[0]['id']
        batch.save()
    source = json.loads((batch.run.out / 'reports-host.overview-dashboard.json').read_text(encoding='utf-8'))['panels'][0]['sources']
    ids = {x['id'] for x in source if x['resource_id'] == batch.map['hosts']['reports']['host_id'] and x['type'] == 'hostmetrics'}
    assert len(ids) == 1
    batch.map.setdefault('host_sources', {})['reports_metrics'] = next(iter(ids))
    batch.save()
    cases = [
        ('independent', 'B13b independent service and CPU', policy(batch, [rule('Service failed', FAILED, .5), rule('CPU critical guard', CPU, .99, .95)])),
        ('every', 'B13b AND service and measured memory', policy(batch, [rule('Service failed', FAILED, .5), rule('Memory observed above five percent', MEMORY, .05, .04)], mode='every')),
        ('any', 'B13b OR service or CPU', policy(batch, [rule('Service failed', FAILED, .5), rule('CPU critical guard', CPU, .99, .95)], mode='any')),
        ('negative', 'B13b AND negative control', policy(batch, [rule('Service failed', FAILED, .5), rule('Memory above 99 percent negative control', MEMORY, .99, .98)], mode='every', phase='negative', routed=False)),
        ('child', 'B13b dependent service child', policy(batch, [rule('Dependent service failed', FAILED, .5)], phase='child', enabled=False)),
        ('cpu_health', 'B13b CPU data health', policy(batch, [rule('Measured CPU guard', CPU, .99, .95)], phase='health', enabled=False,
             hosts=[batch.map['hosts']['reports']['host_id']], sources=list(ids), hold=0, routed=False)),
        ('no_data', 'B13b deliberate empty metric selection', policy(batch, [rule('Intentional unmatched state', 'system_cpu_utilization{state="b13b_intentionally_absent"}', .5)],
             phase='health', enabled=False, hosts=[batch.map['hosts']['reports']['host_id']], sources=list(ids), hold=0, routed=False)),
        ('query_error', 'B13b bounded many-to-many query error', policy(batch, [rule('Intentional runtime join error', 'system_cpu_utilization * on (argus_resource_id) system_cpu_utilization', .5)],
             phase='health', enabled=False, hosts=[batch.map['hosts']['reports']['host_id']], sources=list(ids), hold=0, routed=False)),
    ]
    trials = {}
    for key, name, spec in cases:
        trials[key] = api.call('POST', '/alerts/trial', spec)
        if key not in ['no_data', 'query_error']:
            assert trials[key]['health'] == 'normal', (key, trials[key]['health'], trials[key].get('error_code'))
        create(batch, 'policy', key, name, spec)
    batch.run.save('policy-native-trials', trials)
    assert trials['no_data']['health'] == 'no_data'
    assert trials['query_error']['health'] == 'error'
    clock = datetime.now(timezone.utc)
    create(batch, 'silence', 'owned', 'B13b Reports maintenance window', {'enabled': False,
        'resource_ids': [batch.map['hosts']['reports']['host_id']], 'policy_ids': [batch.map['alert_policies'][k]['id'] for k in ['independent', 'every', 'any']],
        'matchers': [], 'starts_at': (clock-timedelta(minutes=1)).isoformat(), 'ends_at': (clock+timedelta(hours=4)).isoformat(), 'reason': 'Owned B13b silence verification'})
    create(batch, 'inhibition', 'owned', 'B13b primary service inhibits child', {'enabled': False,
        'resource_ids': [x['host_id'] for x in batch.map['hosts'].values()],
        'source': [{'key': 'policy_id', 'operator': '=', 'value': batch.map['alert_policies']['independent']['id']}],
        'target': [{'key': 'policy_id', 'operator': '=', 'value': batch.map['alert_policies']['child']['id']}],
        'equal': ['resource_id'], 'reason': 'Only the same owned host primary failure inhibits its child'})
    print('Eight actual-metric policies, compound controls, grouping, owned route, silence and inhibition configured', flush=True)


if __name__ == '__main__':
    run = Run()
    api = run.api()
    api.login()
    configure(Batch(run, api))
