"""Create an owned reviewer's terminal and a real alert export baseline."""
from datetime import datetime, timedelta, timezone
import json
import secrets
from manage import Run, execute, now, write_json
from integration import Batch
from B13a.access import grant, reader
from B13a.dashboards import publish
from B13a.remote import configure as admin_terminals
from B13b.access import evaluator


def member_terminal(batch):
    api, run = batch.api, batch.run
    state = batch.map.setdefault('member_terminal', {})
    host = batch.map['hosts']['reports']['host_id']
    user = 'plan9review'
    if not state.get('secret_id'):
        credential_path = run.private / 'member-ssh.json'
        if not credential_path.exists():
            write_json(credential_path, {'username': user, 'password': secrets.token_urlsafe(30)})
        password = json.loads(credential_path.read_text(encoding='utf-8'))['password']
        run.shell('reports', f'id {user} >/dev/null 2>&1 || useradd --create-home --shell /bin/bash {user}\n')
        execute(['docker', 'exec', '-i', run.container('reports')['Id'], 'chpasswd'], data=user + ':' + password + '\n')
        row = api.call('POST', '/enterprise/secrets', {'name': run.id + '-review-ssh', 'type': 'ssh_password', 'value': password}, key='review-ssh-secret', expected=201)
        state['secret_id'] = row['id']
        batch.save()
    if not state.get('credential_id'):
        row = api.call('POST', '/enterprise/credentials', {'name': run.id + '-review-ssh', 'protocol': 'ssh', 'username': user,
                       'secret_id': state['secret_id']}, key='review-ssh-credential', expected=201)
        state['credential_id'] = row['id']
        batch.save()
    if not state.get('account_id'):
        row = api.call('POST', '/enterprise/managed-accounts', {'host_id': host, 'username': user, 'privilege_level': 'standard',
                       'credential_id': state['credential_id'], 'allowed_protocols': ['ssh']}, key='review-ssh-account', expected=201)
        state['account_id'] = row['id']
        batch.save()
    path = '/enterprise/data-authorizations/role/' + batch.map['reader']['role_id']
    grant(api, batch.map['reader']['role_id'], 'host', [host])
    api.login()
    if not state.get('grant_id'):
        clock = datetime.now(timezone.utc)
        row = api.call('POST', '/enterprise/remote-access-grants', {'subject_type': 'user', 'subject_id': batch.map['reader']['user_id'],
                       'host_ids': [host], 'managed_account_ids': [state['account_id']], 'protocols': ['ssh'], 'actions': ['terminal'],
                       'valid_from': (clock - timedelta(minutes=1)).isoformat(), 'valid_until': (clock + timedelta(days=1)).isoformat(), 'status': 'draft'},
                       key='review-ssh-grant', expected=201)
        state['grant_id'] = row['id']
        batch.save()
    row = api.call('GET', '/enterprise/remote-access-grants/' + state['grant_id'])
    if row['status'] != 'enabled':
        api.call('POST', '/enterprise/remote-access-grants/' + row['id'] + '/enable?expected_version=' + str(row['version']))
        api.login()
    entries = api.call('GET', path + '?resource_type=host')
    assert {x['resource_id'] for x in entries['items'] if x['direct']} == {h['host_id'] for h in batch.map['hosts'].values()}
    run.save('access-baseline', {'at': now(), 'member': batch.map['reader']['user_id'], 'retained_host': batch.map['hosts']['orders']['host_id'],
             'revoke_host': host, 'unprivileged_account': user, 'grant_id': state['grant_id']})


def alert_policy(batch):
    api = batch.api
    if batch.map['alert_policies'].get('service'):
        return
    evaluator(batch)
    assert not any(x['spec']['enabled'] for x in api.call('GET', '/alerts/configurations?kind=route'))
    name = 'B13c service failure history'
    spec = {'description': 'Actual service recovery history for scoped exports', 'enabled': True,
            'resources': {'mode': 'explicit', 'types': ['host'], 'ids': [x['host_id'] for x in batch.map['hosts'].values()], 'matchers': []},
            'rules': [{'name': 'Service process failed', 'expression': 'max by (argus_resource_id) (systemd_unit_state{resource_systemd_unit_name="plan9-b13c.service",systemd_unit_active_state="failed"})',
                       'query_mode': 'numeric', 'operator': '>', 'threshold': .5, 'recovery_threshold': 0, 'severity': 'warning', 'unit': ''}],
            'mode': 'independent', 'match_by': ['argus_resource_id'], 'interval_seconds': 10, 'for_seconds': 10, 'recovery_seconds': 10,
            'no_data_behavior': 'keep', 'error_behavior': 'keep', 'channel_ids': [], 'route_ids': [],
            'labels': {'plan9_run': batch.run.id}}
    trial = api.call('POST', '/alerts/trial', spec)
    assert trial['health'] == 'normal' and len(trial['observations']) == 2
    result = api.confirm(api.call('POST', '/alerts/configurations/preview', {'kind': 'policy', 'operation': 'create', 'name': name, 'spec': spec}, key='owned-history-policy', expected=202))
    batch.map['alert_policies']['service'] = {'name': name, 'id': result['resource_ref']['resource_id']}
    batch.save()


def prepare(batch):
    publish(batch)
    reader(batch, ['managed_account.read', 'remote_access.request', 'remote_access.session.create'])
    admin_terminals(batch)
    member_terminal(batch)
    alert_policy(batch)
    print('Owned dashboard copy, reader permissions, unprivileged terminal and alert policy ready', flush=True)


if __name__ == '__main__':
    run = Run()
    api = run.api()
    api.login()
    prepare(Batch(run, api))
