"""Real TLS scrape failure, independent stop/recovery and an actual alert cycle."""
import copy
from datetime import datetime, timezone
import ipaddress
import json
import subprocess
import time
import uuid

from run import KEYS
from verification import query

KEY = 'kubernetes_scheduler_metrics'


def exercise(run, api):
    existing = run.out / 'failure-and-stop.json'
    if existing.exists():
        report = json.loads(existing.read_text())
        if report.get('recovered', {}).get('health') == 'healthy' and report.get('alert_firing'):
            record_configuration_closure(run, api, report)
            return
    row = next(r for r in run.builtins(api) if r['definition']['key'] == KEY)
    original = copy.deepcopy(row['collection']['config'])
    ident = row['collection']['id']
    expression = 'min by (argus_resource_id) (up{argus_collection_id="' + ident + '"})'
    assert float(query(run, api, expression)['data']['result'][0]['value'][1]) == 1
    name = 'B09b Scheduler metrics scrape unavailable'
    policies = api.call('GET', '/alerts/configurations?kind=policy')
    policy = next((r for r in policies if r['name'] == name), None)
    if policy is None:
        spec = {'description': 'Real component certificate-name failure and recovery', 'enabled': True,
            'resources': {'mode': 'explicit', 'types': ['kubernetes_cluster'], 'ids': [run.cluster], 'matchers': []},
            'rules': [{'name': name, 'expression': expression, 'query_mode': 'numeric', 'operator': '<',
                       'threshold': .5, 'recovery_threshold': 1, 'severity': 'warning', 'unit': ''}],
            'mode': 'independent', 'match_by': ['argus_resource_id'], 'interval_seconds': 10,
            'for_seconds': 20, 'recovery_seconds': 20, 'no_data_behavior': 'report', 'error_behavior': 'keep',
            'channel_ids': [], 'route_ids': [], 'labels': {'plan9_run': run.id, 'plan9_batch': 'b09b'}}
        trial = api.call('POST', '/alerts/trial', spec)
        assert trial['health'] == 'normal'
        api.confirm(api.call('POST', '/alerts/configurations/preview', {
            'kind': 'policy', 'operation': 'create', 'name': name, 'spec': spec}, key='component-alert', expected=202))
        policy = next(r for r in api.call('GET', '/alerts/configurations?kind=policy') if r['name'] == name)
    run.save('alert-policy', {'id': policy['id'], 'name': name, 'expression': expression})
    start = datetime.now(timezone.utc).isoformat()
    before = {r['definition']['key']: r['collection']['last_sample_at'] for r in run.builtins(api)
              if r['definition']['key'] in KEYS}
    report = {'started_at': start, 'policy_id': policy['id']}
    event = None
    try:
        failed_config = {**original, 'component_tls': {'server_name': 'b09b-invalid.invalid'}}
        run.change(api, KEY, True, failed_config)
        failed = api.wait('/monitoring/collections/' + ident,
                          lambda r: r['health'] == 'failed' and r['error_code'] == 'COLLECTION_SCRAPE_FAILED',
                          timeout=90, label='Actual component TLS failure')
        last_good = failed['last_sample_at']
        assert float(query(run, api, expression)['data']['result'][0]['value'][1]) == 0
        print('Actual Scheduler TLS verification failed; native up=0, target sample time preserved', flush=True)
        time.sleep(25)
        still = api.call('GET', '/monitoring/collections/' + ident)
        assert still['last_sample_at'] == last_good and still['health'] == 'failed'
        others = [r for r in run.builtins(api) if r['definition']['key'] in KEYS and r['definition']['key'] != KEY]
        assert all(r['collection']['health'] == 'healthy' and r['collection']['freshness'] == 'fresh'
                   and r['collection']['last_sample_at'] > before[r['definition']['key']] for r in others)
        fired = api.wait('/alerts/instances?policy_id=' + policy['id'], lambda r: any(
            x['lifecycle'] == 'firing' and x['state']['kind'] == 'condition' for x in r['items']),
            timeout=100, label='Actual component scrape alert')
        event = next(x for x in fired['items'] if x['lifecycle'] == 'firing' and x['state']['kind'] == 'condition')
        assert event['started_at'] >= start
        report.update(failure=failed, repeated_failure=still, other_components=others, alert_firing=event)
        run.save('failure-and-stop', report)
        stopped = run.change(api, KEY, False)
        assert not stopped['desired_enabled'] and stopped['apply_status'] == 'effective'
        report['stopped'] = stopped
        run.save('failure-and-stop', report)
        print('Real alert fired; Scheduler independently stopped with the other components still collecting', flush=True)
    finally:
        run.change(api, KEY, True, original)
        recovered = api.wait('/monitoring/collections/' + ident, lambda r:
            r['health'] == 'healthy' and r['freshness'] == 'fresh' and not r['error_code'],
            timeout=120, label='Restored component certificate trust')
        report['recovered'] = recovered
        run.save('failure-and-stop', report)
    assert event is not None
    assert recovered['last_sample_at'] > report['failure']['last_sample_at']
    record_configuration_closure(run, api, report)


def record_configuration_closure(run, api, report):
    history = api.call('GET', '/alerts/history?policy_id=' + report['policy_id'])
    event = next(x for x in history['items'] if x['id'] == report['alert_firing']['id'])
    assert event['lifecycle'] == 'closed' and event['state']['closure_reason'] == 'source_changed'
    run.save('configuration-alert-closure', {k: event[k] for k in ('id', 'started_at', 'resolved_at', 'lifecycle')})
    run.event('component_configuration_failure_stop_recovery_verified', policy_id=report['policy_id'])
    print('TLS failure, independent stop and actual sample recovery verified; config change correctly closed the old alert source', flush=True)


def availability(run, api):
    """A bounded lab-node network fault leaves the Collector/source version intact."""
    from environment import assert_owner
    from run import execute
    node = assert_owner(run)
    pods = run.infra.get('pods', '-n', 'argus-telemetry', '-l', 'app=argus-otelcol-gateway')['items']
    if not pods:
        gateway = run.infra.get('deployment', 'argus-otelcol-gateway', '-n', 'argus-telemetry')
        selector = ','.join(k + '=' + v for k, v in gateway['spec']['selector']['matchLabels'].items())
        pods = run.infra.get('pods', '-n', 'argus-telemetry', '-l', selector)['items']
    pod = next(p for p in pods if not p['metadata'].get('deletionTimestamp'))
    source = ipaddress.ip_address(pod['status']['podIP'])
    tool = 'iptables' if source.version == 4 else 'ip6tables'
    marker = 'p9-b09b-' + uuid.uuid4().hex[:10]
    rule = ['-s', str(source), '-p', 'tcp', '--dport', '10259', '-m', 'comment', '--comment', marker, '-j', 'REJECT']
    remove = [tool, '-w', '5', '-D', 'INPUT', *rule]
    policy = json.loads((run.out / 'alert-policy.json').read_text())
    row = next(r for r in run.builtins(api) if r['definition']['key'] == KEY)['collection']
    assert row['health'] == 'healthy' and row['freshness'] == 'fresh'
    original_version = row['version']
    started = datetime.now(timezone.utc).isoformat()
    report = {'started_at': started, 'node_id': node, 'gateway_uid': pod['metadata']['uid'],
              'gateway_ip': str(source), 'collection_version': original_version,
              'fault_rule_comment': marker, 'automatic_restore_seconds': 120}
    event = None
    unit = 'plan9-b09b-recover-' + uuid.uuid4().hex[:8]
    execute(['docker', 'exec', node, 'systemd-run', '--unit=' + unit, '--on-active=120s', *remove])
    try:
        execute(['docker', 'exec', node, tool, '-w', '5', '-I', 'INPUT', '1', *rule])
        failed = api.wait('/monitoring/collections/' + row['id'], lambda x:
                          x['health'] == 'failed' and x['error_code'] == 'COLLECTION_SCRAPE_FAILED',
                          timeout=60, label='Actual network scrape failure')
        assert failed['version'] == original_version
        assert float(query(run, api, policy['expression'])['data']['result'][0]['value'][1]) == 0
        fired = api.wait('/alerts/instances?policy_id=' + policy['id'], lambda r: any(
            x['lifecycle'] == 'firing' and x['state']['kind'] == 'condition' for x in r['items']),
            timeout=60, label='Stable-source component alert')
        event = next(x for x in fired['items'] if x['lifecycle'] == 'firing' and x['state']['kind'] == 'condition')
        assert event['started_at'] >= started
        report.update(failure=failed, alert_firing=event)
        others = [r for r in run.builtins(api) if r['definition']['key'] in KEYS and r['definition']['key'] != KEY]
        assert all(r['collection']['health'] == 'healthy' and r['collection']['freshness'] == 'fresh' for r in others)
        run.save('stable-source-fault', report)
        print('Stable-source Scheduler scrape failed and a real condition alert fired; four other components stayed healthy', flush=True)
    finally:
        result = subprocess.run(['docker', 'exec', assert_owner(run), *remove], capture_output=True, timeout=15)
        assert result.returncode in (0, 1)
        execute(['docker', 'exec', node, 'systemctl', 'stop', unit + '.timer'])
        check = subprocess.run(['docker', 'exec', node, tool, '-w', '5', '-C', 'INPUT', *rule], capture_output=True, timeout=15)
        assert check.returncode == 1, 'Owned fault rule was not removed'
    recovered = api.wait('/monitoring/collections/' + row['id'], lambda x:
        x['health'] == 'healthy' and x['freshness'] == 'fresh' and not x['error_code'],
        timeout=90, label='Stable-source actual sample recovery')
    assert recovered['version'] == original_version and recovered['last_sample_at'] > failed['last_sample_at']
    assert event is not None
    history = api.wait('/alerts/history?policy_id=' + policy['id'], lambda r: any(
        x['id'] == event['id'] and x['lifecycle'] == 'resolved' for x in r['items']),
        timeout=90, label='Same component condition alert recovery')
    resolved = next(x for x in history['items'] if x['id'] == event['id'])
    assert resolved['state'].get('closure_reason') != 'source_changed'
    report.update(recovered=recovered, alert_resolved=resolved, fault_rule_removed=True)
    run.save('stable-source-fault', report)
    run.save('alert-cycle', {k: resolved[k] for k in ('id', 'started_at', 'resolved_at', 'lifecycle')})
    run.event('stable_source_component_alert_recovered', alert_id=event['id'])
    print('Same-source component samples resumed and the same condition alert resolved; fault rule removed', flush=True)
