"""One real guarded service failure/recovery; complex alert cases are B13b."""
import time
from access import evaluator
from manage import Run, now

SERVICE = 'plan9-b13a.service'
EXPRESSION = 'max by (argus_resource_id) (systemd_unit_state{resource_systemd_unit_name="' + SERVICE + '",systemd_unit_active_state="failed"})'


def configure(batch):
    evaluator(batch)
    name = 'B13a Reports service failed'
    rows = batch.api.call('GET', '/alerts/configurations?kind=policy')
    policy = next((x for x in rows if x['name'] == name), None)
    if policy is None:
        spec = {'description': 'Real Reports HTTP process failure; shared Redis and Orders remain healthy; no external notifications',
                'enabled': True, 'resources': {'mode': 'explicit', 'types': ['host'], 'ids': [batch.map['hosts']['reports']['host_id']], 'matchers': []},
                'rules': [{'name': name, 'expression': EXPRESSION, 'query_mode': 'numeric', 'operator': '>', 'threshold': .5,
                           'recovery_threshold': 0, 'severity': 'warning', 'unit': ''}], 'mode': 'independent', 'match_by': ['argus_resource_id'],
                'interval_seconds': 10, 'for_seconds': 20, 'recovery_seconds': 20, 'no_data_behavior': 'report', 'error_behavior': 'keep',
                'channel_ids': [], 'route_ids': [], 'labels': {'plan9_batch': 'b13a', 'plan9_run': batch.run.id}}
        trial = batch.api.call('POST', '/alerts/trial', spec)
        batch.run.save('service-alert-trial', {'trial': trial, 'spec': spec})
        assert trial['health'] == 'normal', trial
        batch.api.confirm(batch.api.call('POST', '/alerts/configurations/preview', {'kind': 'policy', 'operation': 'create', 'name': name, 'spec': spec}, key='service-policy', expected=202))
        policy = next(x for x in batch.api.call('GET', '/alerts/configurations?kind=policy') if x['name'] == name)
    batch.map['alert_policies']['service'] = {'id': policy['id'], 'name': name, 'expression': EXPRESSION}
    batch.save()
    print('Reports service alert configured against actual healthy systemd samples', flush=True)


def exercise(batch):
    if (batch.run.out / 'alert-cycle.json').exists():
        return
    api, run = batch.api, batch.run
    ident = batch.map['alert_policies']['service']['id']
    guard = 'plan9-b13a-recover-' + str(int(time.time()))
    recover = 'systemctl reset-failed ' + SERVICE + ' || true; systemctl start ' + SERVICE
    firing = None
    started = now()
    try:
        run.shell('reports', 'systemd-run --collect --unit=' + guard + " --on-active=120s /bin/sh -ec '" + recover + "'\n"
                  'systemctl kill --kill-whom=main --signal=SIGKILL ' + SERVICE + '\n'
                  'for i in 1 2 3 4 5; do systemctl is-failed --quiet ' + SERVICE + ' && exit 0; sleep 1; done; exit 1\n')
        page = api.wait('/alerts/instances?policy_id=' + ident, lambda x: any(e['lifecycle'] == 'firing' and e['state']['kind'] == 'condition' and e['started_at'] >= started[:19] for e in x['items']),
                        timeout=95, label='Real Reports failure alert')
        firing = next(e for e in page['items'] if e['lifecycle'] == 'firing' and e['state']['kind'] == 'condition')
        run.save('alert-firing', firing)
        assert 'redis_endpoint' in run.shell('orders', 'curl -fsS http://127.0.0.1:18085/health\n')
        print('Real Reports failure fired; Orders and shared Redis stayed healthy', flush=True)
    finally:
        run.shell('reports', 'systemctl stop ' + guard + '.timer || true\n' + recover + '\n')
    page = api.wait('/alerts/history?policy_id=' + ident, lambda x: any(e['id'] == firing['id'] and e['lifecycle'] == 'resolved' for e in x['items']),
                    timeout=100, label='Actual Reports recovery')
    resolved = next(e for e in page['items'] if e['id'] == firing['id'])
    assert 'redis_endpoint' in run.shell('reports', 'curl -fsS http://127.0.0.1:18085/health\n')
    run.save('alert-cycle', {'id': resolved['id'], 'policy_id': ident, 'started_at': resolved['started_at'], 'resolved_at': resolved['resolved_at'],
                            'lifecycle': resolved['lifecycle'], 'guard_seconds': 120, 'independent_control_stayed_healthy': True})
    print('Reports service and the same alert recovered normally', flush=True)


if __name__ == '__main__':
    import argparse
    from integration import Batch
    p = argparse.ArgumentParser()
    p.add_argument('action', choices=['configure', 'exercise'])
    run = Run()
    api = run.api()
    api.login()
    globals()[p.parse_args().action](Batch(run, api))
