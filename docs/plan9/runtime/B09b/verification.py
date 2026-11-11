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


def final_review(run, api):
    """Recheck current samples, all panel values and the retained alert configuration."""
    import json

    samples=fresh(run,api)
    boards=dashboards(run,api)
    rows=run.builtins(api)
    assert len(rows)==8 and all(row['collection']['health']=='healthy' and
        row['collection']['freshness']=='fresh' for row in rows)
    policies=json.loads((run.out/'alert-policies.json').read_text(encoding='utf-8'))
    assert set(policies)==set(KEYS)
    for row in policies.values():
        current=api.call('GET','/alerts/configurations/'+row['id'])
        assert current['spec']['enabled'] and current['spec']['resources']['ids']==[run.cluster]
        assert current['health']=='normal',current
    cycle=json.loads((run.out/'alert-cycle.json').read_text(encoding='utf-8'))
    assert cycle['lifecycle']=='resolved'
    fault=json.loads((run.out/'stable-source-fault.json').read_text(encoding='utf-8'))
    assert fault['fault_rule_removed'] and fault['alert_resolved']['id']==cycle['id']
    assert fault['alert_firing']['id']==cycle['id']
    assert fault['failure']['version']==fault['recovered']['version']
    browser=json.loads((run.out/'browser-review.json').read_text(encoding='utf-8'))
    assert browser['passed'] and browser['cases']==5 and browser['strict_tls']
    instances=api.call('GET','/alerts/instances?limit=100')['items']
    active=[{k:row.get(k) for k in ('id','policy_id','lifecycle','health')}
        for row in instances if run.cluster in row.get('resource_ids',[])
        and row['lifecycle'] in ('firing','recovering')]
    diagnostics=[row for row in instances if run.cluster in row.get('resource_ids',[])
        and row['lifecycle']=='normal' and row['health']=='no_data' and row.get('started_at') is None]
    delivery=[row for row in instances if run.cluster in row.get('resource_ids',[])
        and row['lifecycle'] in ('firing','recovering') and row['state']['kind']=='collector_delivery_gap']
    component_active=[row for row in instances if run.cluster in row.get('resource_ids',[])
        and row['lifecycle'] in ('firing','recovering') and row['state']['kind']=='condition']
    assert not component_active
    observations=[]
    if delivery:observations.append('B09b-D02')
    if diagnostics:observations.append('B09b-A01')
    run.save('alert-observations',{'untriggered_no_data_rows':diagnostics,'delivery_alerts':delivery})
    before=json.loads((run.out/'platform-before.json').read_text(encoding='utf-8'))
    after=run.infra.platform()
    assert before==after,'Formal platform deployment or PVC baseline changed'
    previous=run.out.parent/'p9-b09a-20261108-01'
    cleanup=json.loads((previous/'cleanup-verification.json').read_text(encoding='utf-8'))
    assert cleanup['status']=='destroyed' and cleanup['owned_runtime_containers']==0
    from run import execute
    assert not execute(['docker','container','ls','-aq','--filter',
        'label=io.x-k8s.kind.cluster=p9-b09a-20261108-01'])
    run.save('platform-after',after)
    proof={'run_id':run.id,'batch':'B09b','status':'ready_for_review','human_acceptance':'pending',
        'recorded_at':datetime.now(timezone.utc).isoformat(),'cluster_id':run.cluster,
        'fresh_independent_cluster':True,'previous_B09a_destroyed':True,
        'component_samples':samples,'fresh_native_collections':8,'dashboards':len(boards),
        'nonempty_panels':30,'configured_component_alerts':5,'alert_cycle':cycle,
        'fault_same_source_recovered':True,'temporary_fault_rules_removed':True,
        'real_browser_cases':5,'runtime_unit_cases':4,'active_batch_alerts':len(active),
        'active_alert_items':active,'active_component_condition_alerts':len(component_active),
        'active_collector_delivery_alerts':len(delivery),'untriggered_no_data_state_rows':len(diagnostics),
        'recorded_observations':observations,'formal_platform_baseline_preserved':True,
        'preserved_pvcs':len(after['pvc']),
        'not_exercised':['host_node_claim_dedup_no_registered_node_host']}
    run.save('checks',proof)
    record=json.loads((run.out/'run.json').read_text(encoding='utf-8'))
    record.update(status='ready_for_review',ready_at=proof['recorded_at'],human_acceptance='pending')
    run.save('run',record)
    print(json.dumps(proof,ensure_ascii=False,indent=2),flush=True)
