"""Host-bound administrator terminal access through the normal product grant."""
from datetime import datetime, timedelta, timezone
from integration import Batch
from manage import Run


def configure(batch):
    api = batch.api
    state = batch.map.setdefault('remote', {})
    accounts = state.setdefault('accounts', {})
    for key, host in batch.map['hosts'].items():
        if key not in accounts:
            row = api.call('POST', '/enterprise/managed-accounts', {'host_id': host['host_id'], 'username': 'root',
                           'privilege_level': 'administrator', 'credential_id': host['credential_id'], 'allowed_protocols': ['shell', 'ssh']},
                           key=key + '-terminal-account', expected=201)
            accounts[key] = row['id']
            batch.save()
    if not state.get('grant_id'):
        now = datetime.now(timezone.utc)
        row = api.call('POST', '/enterprise/remote-access-grants', {'subject_type': 'user', 'subject_id': api.user['id'],
                       'host_ids': [x['host_id'] for x in batch.map['hosts'].values()], 'managed_account_ids': list(accounts.values()),
                       'protocols': ['shell', 'ssh'], 'actions': ['terminal'], 'valid_from': (now - timedelta(minutes=1)).isoformat(),
                       'valid_until': (now + timedelta(days=7)).isoformat(), 'status': 'draft'}, key='owned-review-terminals', expected=201)
        state.update(grant_id=row['id'], user_id=api.user['id'], valid_until=row['valid_until'])
        batch.save()
    row = api.call('GET', '/enterprise/remote-access-grants/' + state['grant_id'])
    if row['status'] == 'draft':
        row = api.call('POST', '/enterprise/remote-access-grants/' + row['id'] + '/enable?expected_version=' + str(row['version']))
        api.login()
    assert row['status'] == 'enabled'
    batch.run.save('remote-authorization', {'accounts': accounts, 'grant': row, 'recording': 'required', 'command_audit': 'required',
                                           'reader_terminal_grants': 0, 'external_recipients': 0})
    print('Two host-bound administrator terminal accounts and the seven-day grant ready', flush=True)


if __name__ == '__main__':
    run = Run()
    api = run.api()
    api.login()
    configure(Batch(run, api))
