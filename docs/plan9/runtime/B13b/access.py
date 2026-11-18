"""Independent scoped test member and owned background evaluation principal."""
from manage import Run
from B13a.access import reader, grant
from integration import Batch


def evaluator(batch):
    api = batch.api
    name = batch.NAME
    state = batch.map.setdefault('evaluator', {})
    if 'service_account_id' not in state:
        row = api.call('POST', '/enterprise/service-accounts', {'name': name + ' owned background evaluator', 'description': 'Only this batch resources; no tool credentials',
                       'allowed_tool_ids': []}, key='owned-evaluator', expected=201)
        state['service_account_id'] = row['id']
        batch.save()
    if 'role_id' not in state:
        row = api.call('POST', '/enterprise/roles', {'name': name + ' metric evaluator scope', 'description': 'Explicit owned Host/native-cache/application data only',
                       'permissions': ['host.read', 'monitoring.read', 'application.read']}, key='owned-evaluator-role', expected=201)
        state['role_id'] = row['id']
        batch.save()
    if 'role_binding_id' not in state:
        row = api.call('POST', '/enterprise/role-bindings', {'subject_type': 'service_account', 'subject_id': state['service_account_id'], 'role_id': state['role_id']},
                       key='owned-evaluator-role-binding', expected=201)
        state['role_binding_id'] = row['id']
        batch.save()
    for kind, ids in [('host', [x['host_id'] for x in batch.map['hosts'].values()]),
                      ('monitoring_target', [x['target_id'] for x in batch.map['targets'].values()]),
                      ('application', [x['id'] for x in batch.map['applications'].values()])]:
        grant(api, state['role_id'], kind, ids)
    api.login()
    settings = api.call('GET', '/alerts/configurations?kind=settings')
    assert len(settings) <= 1
    if not settings or settings[0]['spec']['evaluator_id'] != state['service_account_id']:
        assert all(x['name'].startswith(name) for x in api.call('GET', '/alerts/configurations?kind=policy')), 'Do not replace evaluator for unrelated policies'
        body = {'kind': 'settings', 'operation': 'create', 'name': name + ' background evaluator', 'spec': {'evaluator_id': state['service_account_id']}}
        if settings:
            batch.map.setdefault('prior_evaluator_settings', settings[0])
            batch.save()
            body.update(operation='update', id=settings[0]['id'], expected_version=settings[0]['version'])
        api.confirm(api.call('POST', '/alerts/configurations/preview', body, key='owned-background-settings', expected=202))
    batch.run.save('evaluator-scope', {'service_account_id': state['service_account_id'], 'role_id': state['role_id'],
                    'host_ids': [x['host_id'] for x in batch.map['hosts'].values()], 'target_ids': [x['target_id'] for x in batch.map['targets'].values()],
                    'separate_from_reader_and_notification_credentials': True, 'no_api_key_created': True})
    print('Owned background evaluator and explicit batch-only data grants configured', flush=True)


if __name__ == '__main__':
    import argparse
    p = argparse.ArgumentParser()
    p.add_argument('action', choices=['reader', 'evaluator'])
    run = Run()
    api = run.api()
    api.login()
    globals()[p.parse_args().action](Batch(run, api))
