"""Dedicated normal-login member and narrowly scoped background evaluator."""
import json
import secrets
from manage import Run, ROOT, write_json
from common.api import Api


def grant(api, role, kind, ids):
    path = '/enterprise/data-authorizations/role/' + role
    current = api.call('GET', path + '?resource_type=' + kind)
    missing = [ident for ident in ids if not any(x['resource_id'] == ident and x['direct'] for x in current['items'])]
    if missing:
        api.call('POST', path, {'resource_type': kind, 'resource_ids': missing, 'remove': False,
                               'expected_version': current['authorization_version']}, expected=204)


def reader(batch, extra_permissions=()):
    api, run = batch.api, batch.run
    name = batch.NAME
    state = batch.map.setdefault('reader', {})
    if 'role_id' not in state:
        role = api.call('POST', '/enterprise/roles', {'name': name + ' scoped reader',
                        'description': 'Only Orders application, Orders host, shared Redis and B13a dashboards',
                        'permissions': ['host.read', 'application.read', 'monitoring.read', 'telemetry.dashboard.read',
                                        'telemetry.collector.read', 'data_authorization.read', 'alert.read', *extra_permissions]}, key='reader-role', expected=201)
        state['role_id'] = role['id']
        batch.save()
    if 'user_id' not in state:
        departments = api.call('GET', '/enterprise/departments')
        departments = departments['items'] if isinstance(departments, dict) else departments
        department = next(x for x in departments if x['is_default'])
        username = 'p9-' + name.lower() + '-reader-' + run.id.split('-')[2]
        created = api.call('POST', '/enterprise/users', {'username': username, 'display_name': name + ' Orders 只读成员',
                           'department_id': department['id'], 'role_ids': [state['role_id']]}, key='reader-user', expected=201)
        write_json(run.private / 'reader-credentials.json', {'username': username, 'temporary_password': created['temporary_password'],
                                                           'password': 'P9!' + secrets.token_urlsafe(28) + 'aQ2'})
        state.update(user_id=created['user']['id'], username=username)
        batch.save()
    for kind, ids in [('host', [batch.map['hosts']['orders']['host_id']]),
                      ('application', [batch.map['applications']['orders']['id']]),
                      ('monitoring_target', [batch.map['targets']['redis']['target_id']]),
                      ('dashboard', [*[batch.map[k]['id'] for k in ['batch_dashboard', 'personal_dashboard'] if k in batch.map], *[x['id'] for x in batch.map['dashboards'].values()]])]:
        grant(api, state['role_id'], kind, list(set(ids)))
    api.login()
    credentials = json.loads((run.private / 'reader-credentials.json').read_text(encoding='utf-8'))
    session = run.private / 'reader-session.json'
    if not session.exists():
        write_json(session, {'origin': 'https://argus.dev', 'csrf': '', 'user': {}, 'cookies': []})
    member = Api(run, session, ROOT / 'deploy/.cache/argus-trust-bundle.pem',
                 {'enterprise_username': credentials['username'], 'password': credentials['password']})
    if not credentials.get('password_changed'):
        response = member.http.post(member.origin + '/api/v1/enterprise/auth/login', json={
            'username': credentials['username'], 'password': credentials['temporary_password']}, timeout=30)
        assert response.status_code == 200
        challenge = response.json()
        assert challenge['status'] == 'password_change_required', challenge.get('status')
        response = member.http.post(member.origin + '/api/v1/enterprise/auth/complete-password-change', json={
            'challenge_id': challenge['password_change_challenge']['challenge_id'], 'temporary_password': credentials['temporary_password'],
            'new_password': credentials['password']}, timeout=30)
        assert response.status_code == 200
        completed = response.json()
        assert completed.get('csrf_token') or completed.get('authenticated_session'), completed.get('status')
        credentials['password_changed'] = True
        write_json(run.private / 'reader-credentials.json', credentials)
    member.login()
    run.save('reader-scope', {'user_id': state['user_id'], 'role_id': state['role_id'], 'normal_login': True,
                            'allowed': {'host': batch.map['hosts']['orders']['host_id'], 'application': batch.map['applications']['orders']['id'],
                                        'monitoring_target': batch.map['targets']['redis']['target_id']},
                            'denied': {'host': batch.map['hosts']['reports']['host_id'], 'application': batch.map['applications']['reports']['id'],
                                       'monitoring_target': batch.map['targets']['memcached']['target_id']}})
    print('Dedicated member normal login and explicit Orders-only grants ready', flush=True)
    return member


def evaluator(batch):
    api = batch.api
    previous = json.loads((batch.run.out.parent / 'p9-b12-20261116-01/resource-map.json').read_text(encoding='utf-8'))['evaluator']
    roles = api.call('GET', '/enterprise/roles')['items']
    assert next(x for x in roles if x['id'] == previous['role_id'])['status'] == 'active'
    accounts = api.call('GET', '/enterprise/service-accounts')['items']
    assert next(x for x in accounts if x['id'] == previous['service_account_id'])['status'] == 'active'
    batch.map['evaluator'] = {**previous, 'borrowed_account_role': True}
    batch.save()
    grant(api, previous['role_id'], 'host', [x['host_id'] for x in batch.map['hosts'].values()])
    api.login()
    settings = api.call('GET', '/alerts/configurations?kind=settings')
    assert len(settings) == 1 and settings[0]['spec']['evaluator_id'] == previous['service_account_id']
    batch.run.save('evaluator-scope', {'service_account_id': previous['service_account_id'], 'role_id': previous['role_id'],
                                      'host_ids': [x['host_id'] for x in batch.map['hosts'].values()], 'separate_from_test_member': True,
                                      'no_external_notifications': True, 'existing_evaluator_settings_preserved': True})
    print('Separate background evaluator authorized only for current owned hosts', flush=True)


if __name__ == '__main__':
    import argparse
    from integration import Batch
    p = argparse.ArgumentParser()
    p.add_argument('action', choices=['reader', 'evaluator'])
    run = Run()
    api = run.api()
    api.login()
    globals()[p.parse_args().action](Batch(run, api))
