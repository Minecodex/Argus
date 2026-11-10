"""Require current target samples and all managed component dashboard panels."""
from datetime import datetime, timedelta, timezone
import math
import subprocess
import time

from run import KEYS, TEMPLATES

BUDGET = {'max_scan_bytes': 268435456, 'max_rows': 20000, 'max_samples': 2000000,
          'max_series': 20000, 'timeout_ms': 10000, 'max_result_bytes': 8388608}
METRICS = dict(zip(KEYS, ('apiserver_request_total', 'coredns_dns_requests_total',
    'scheduler_schedule_attempts_total', 'workqueue_depth', 'etcd_server_has_leader')))


def query(run, api, expression):
    end = datetime.now(timezone.utc)
    return api.call('POST', '/enterprise/metrics/query', {
        'resource_ids': [run.cluster], 'time_range': {'from': (end - timedelta(minutes=15)).isoformat(),
        'to': end.isoformat()}, 'budget': BUDGET, 'query': expression})


def fresh(run, api):
    def healthy(rows):
        wanted = [r for r in rows if r['definition']['key'] in KEYS]
        return len(wanted) == len(KEYS) and all((r.get('collection') or {}).get('health') == 'healthy'
            and r['collection']['apply_status'] == 'effective' and r['collection']['freshness'] == 'fresh'
            and r['collection']['desired_enabled'] for r in wanted)
    path = '/monitoring/builtins?target_type=kubernetes_cluster&target_id=' + run.cluster
    rows = api.wait(path, healthy, timeout=240, label='Five real component samples')
    first = {r['definition']['key']: r['collection']['last_sample_at'] for r in rows if r['definition']['key'] in KEYS}
    def advanced(rows):
        return healthy(rows) and all(r['collection']['last_sample_at'] > first[r['definition']['key']]
            for r in rows if r['definition']['key'] in KEYS)
    # Require another actual scrape, rather than merely accepting the first cached sample.
    rows = api.wait(path, advanced, timeout=90, label='Continued component collection')
    run.save('fresh-components', rows)
    results = {}
    for row in rows:
        key = row['definition']['key']
        if key not in KEYS:
            continue
        ident = row['collection']['id']
        selector = '{argus_collection_id="' + ident + '"}'
        up = query(run, api, 'up' + selector)['data']['result']
        metric = query(run, api, METRICS[key] + selector)['data']['result']
        assert up and all(float(r['value'][1]) == 1 for r in up), (key, 'scrape failed')
        assert metric and any(math.isfinite(float(r['value'][1])) for r in metric), (key, 'no target metrics')
        results[key] = {'collection_id': ident, 'first_sample_at': first[key],
            'latest_sample_at': row['collection']['last_sample_at'], 'target_metric': METRICS[key],
            'target_series': len(metric), 'healthy_targets': len(up)}
    run.save('continuous-samples', results)
    print('Five components have advancing target samples and successful native scrapes', flush=True)
    return results


def dashboards(run, api):
    boards = api.call('GET', '/dashboards')
    reports = {}
    for key, template in TEMPLATES.items():
        board = next(b for b in boards if b.get('managed_template_key') == template)
        path = '/kubernetes/clusters/' + run.cluster + '/dashboard-bindings'
        binding = api.call('GET', path)
        if not any(r['dashboard']['id'] == board['id'] for r in binding['items']):
            api.confirm(api.call('POST', path + '/preview', {'operation': 'attach',
                'dashboard_id': board['id'], 'expected_dashboard_version': board['version'],
                'expected_resource_version': binding['resource_version']}, key='bind-' + template, expected=201))
        end = datetime.now(timezone.utc)
        value = api.call('POST', '/dashboards/' + board['id'] + '/execute', {
            'from': (end - timedelta(minutes=15)).isoformat(), 'to': end.isoformat(),
            'resource_mode': 'explicit', 'resource_types': ['kubernetes_cluster'], 'resource_ids': [run.cluster]})
        value.pop('context_token', None)
        run.save(template + '-dashboard', value)
        panels = []
        for panel in value['panels']:
            points = []
            for target in panel['targets']:
                for series in target.get('data') or []:
                    points.extend(series.get('values', []))
                    if series.get('value'):
                        points.append(series['value'])
            finite = sum(math.isfinite(float(p[1])) for p in points)
            panels.append({'id': panel['id'], 'status': panel['status'], 'finite_points': finite})
        reports[template] = {'id': board['id'], 'name': board['name'], 'collection': key, 'panels': panels}
        print(template + ': ' + str(sum(p['status'] == 'success' and p['finite_points'] > 0 for p in panels)) + '/' + str(len(panels)) + ' nonempty panels', flush=True)
    run.save('dashboard-verification', reports)
    assert len(reports) == 5 and all(len(b['panels']) == 6 and all(
        p['status'] == 'success' and p['finite_points'] > 0 for p in b['panels']) for b in reports.values()), 'Component dashboard data is incomplete'
    run.event('component_dashboards_verified', dashboards=5, nonempty_panels=30)
    return reports


def permissions(run):
    from environment import assert_owner
    node = assert_owner(run)
    gateway = run.infra.get('deployment', 'argus-otelcol-gateway', '-n', 'argus-telemetry')
    account = gateway['spec']['template']['spec']['serviceAccountName']
    actor = 'system:serviceaccount:argus-telemetry:' + account
    checks = [
        ('component_metrics', True, ['get', '/metrics']),
        ('system_pod_proxy', True, ['get', 'pods', '--subresource=proxy', '-n', 'kube-system']),
        ('node_statistics', True, ['get', 'nodes', '--subresource=stats']),
        ('business_pod_proxy', False, ['get', 'pods', '--subresource=proxy', '-n', 'b09a-workloads']),
        ('secrets', False, ['list', 'secrets', '-n', 'argus-telemetry']),
        ('pod_exec', False, ['create', 'pods', '--subresource=exec', '-n', 'kube-system']),
        ('rbac_write', False, ['create', 'clusterroles']),
    ]
    results = {}
    for label, expected, args in checks:
        value = subprocess.run(['docker', 'exec', node, 'kubectl', '--kubeconfig=/etc/kubernetes/admin.conf',
            'auth', 'can-i', *args, '--as=' + actor], capture_output=True, text=True, timeout=15)
        assert value.returncode in (0, 1) and value.stdout.strip() in ('yes', 'no'), label
        allowed = value.stdout.strip() == 'yes'
        assert allowed == expected, label
        results[label] = allowed
    run.save('runtime-authorization', {'service_account': actor, 'checks': results})
    print('Installed Collector RBAC: metrics/system proxy allowed; business proxy, Secret, exec and RBAC writes denied', flush=True)
