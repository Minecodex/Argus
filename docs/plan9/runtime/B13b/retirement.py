"""Retire B13b's isolated rules, local inbox, grants and native infrastructure."""
import hashlib
import json
from manage import Run, now
from integration import Batch
from B13a.retirement import cleanup
from scenario import receipts

NEXT = 'B13c'
AUTHORIZATION = '继续下一轮 B13c，把撤权、凭据轮换和资源生命周期按计划验收一遍。'


def seal(batch):
    run, api = batch.run, batch.api
    path = run.out / 'advance-authorization.json'
    if path.exists():
        assert json.loads(path.read_text(encoding='utf-8'))['next_batch'] == NEXT
        return
    required = ['handoff', 'core-acceptance', 'noise-acceptance', 'health-acceptance', 'current-state', 'delivery-watch', 'fault-cleanup']
    digests = {name: hashlib.sha256((run.out / (name + '.json')).read_bytes()).hexdigest() for name in required}
    for row in batch.map['containers']:
        run.container(row['service'])
    channel = batch.map['notification']['channel_id']
    rows = api.call('GET', '/alerts/deliveries?channel_id=' + channel + '&limit=200')
    assert not rows.get('next_cursor')
    run.save('last-notification-evidence', {'at': now(), 'delivery_records': rows['items'], 'native_receipts': receipts(batch)})
    run.save('advance-authorization', {'at': now(), 'statement': AUTHORIZATION, 'next_batch': NEXT, 'cleanup_authorized': True,
             'explicit_human_pass_not_claimed': True, 'formal_before': run.platform(), 'sealed_evidence_sha256': digests})


def retire_alert_configuration(batch):
    run, api = batch.run, batch.api
    state = batch.map['notification']
    for kind in ['inhibition', 'silence', 'route', 'grouping', 'channel']:
        ids = [state['channel_id']] if kind == 'channel' else list(batch.map['configurations'].get(kind, {}).values())
        for ident in ids:
            row = api.call('GET', '/alerts/configurations/' + ident, expected=[200, 404])
            if row.get('code'):
                continue
            prior = batch.map.get('prior_grouping') if kind == 'grouping' else None
            if prior and row['name'] == prior['name'] and row['spec'] == prior['spec']:
                continue
            assert row['kind'] == kind and row['name'].startswith(run.BATCH)
            body = {'kind': kind, 'operation': 'delete', 'id': ident, 'expected_version': row['version']}
            if prior:
                assert prior['id'] == ident
                body.update(operation='update', name=prior['name'], spec=prior['spec'])
            api.confirm(api.call('POST', '/alerts/configurations/preview', body, key='retire-b13b-' + ident, expected=202))
    evaluator = batch.map['evaluator']
    settings = api.call('GET', '/alerts/configurations?kind=settings')
    owned = next((x for x in settings if x['spec']['evaluator_id'] == evaluator['service_account_id']), None)
    if owned:
        body = {'kind': 'settings', 'operation': 'delete', 'id': owned['id'], 'expected_version': owned['version']}
        prior = batch.map.get('prior_evaluator_settings')
        if prior:
            assert prior['id'] == owned['id']
            body.update(operation='update', name=prior['name'], spec=prior['spec'])
        api.confirm(api.call('POST', '/alerts/configurations/preview', body, key='restore-prior-alert-settings', expected=202))
    run.shell('reports', 'if [ "$(systemctl show -p LoadState --value plan9-b13b-mail.service || true)" = loaded ]; then systemctl stop plan9-b13b-mail.service; fi\n')
    credential = next(x for x in api.call('GET', '/enterprise/credentials')['items'] if x['id'] == state['credential_id'])
    assert credential['name'] == run.id + '-local-smtp'
    if credential['status'] == 'active':
        api.call('PUT', '/enterprise/credentials/' + credential['id'], {'status': 'disabled', 'expected_version': credential['version']})
    secret = api.call('GET', '/enterprise/secrets/' + state['secret_id'])
    assert secret['name'] == run.id + '-local-smtp'
    if secret['status'] == 'active':
        api.call('DELETE', '/enterprise/secrets/' + secret['id'] + '?expected_version=' + str(secret['version']), expected=204)


def retire_evaluator(batch):
    api = batch.api
    state = batch.map['evaluator']
    name = batch.run.BATCH + ' owned background evaluator'
    account = next(x for x in api.call('GET', '/enterprise/service-accounts')['items'] if x['id'] == state['service_account_id'])
    assert account['name'] == name
    if account['status'] == 'active':
        api.call('PUT', '/enterprise/service-accounts/' + account['id'], {'description': account['description'],
                 'allowed_tool_ids': [], 'status': 'disabled', 'expected_version': account['version']})
    bindings = api.call('GET', '/enterprise/role-bindings')['items']
    match = next((x for x in bindings if x['id'] == state['role_binding_id']), None)
    if match:
        assert match['subject_id'] == account['id'] and match['role_id'] == state['role_id']
        api.call('DELETE', '/enterprise/role-bindings/' + match['id'] + '?expected_version=' + str(match['version']), expected=204)
    path = '/enterprise/data-authorizations/role/' + state['role_id']
    for kind in ['host', 'monitoring_target', 'application']:
        rows = api.call('GET', path + '?resource_type=' + kind)
        ids = [x['resource_id'] for x in rows['items'] if x['direct']]
        if ids:
            api.call('POST', path, {'resource_type': kind, 'resource_ids': ids, 'remove': True, 'expected_version': rows['authorization_version']}, expected=204)
    role = next(x for x in api.call('GET', '/enterprise/roles')['items'] if x['id'] == state['role_id'])
    assert role['name'] == batch.run.BATCH + ' metric evaluator scope'
    if role['status'] == 'active':
        api.call('DELETE', '/enterprise/roles/' + role['id'] + '?expected_version=' + str(role['version']), expected=204)
    prior = batch.map.get('prior_evaluator_settings')
    settings = api.call('GET', '/alerts/configurations?kind=settings')
    assert not any(x['spec']['evaluator_id'] == account['id'] for x in settings)
    if prior:
        assert any(x['id'] == prior['id'] and x['spec'] == prior['spec'] for x in settings)
    batch.run.save('evaluator-retirement', {'at': now(), 'owned_account_disabled': True, 'owned_grants_removed': True, 'prior_settings_restored': bool(prior)})


if __name__ == '__main__':
    run = Run()
    api = run.api()
    api.login()
    batch = Batch(run, api)
    seal(batch)
    cleanup(batch, NEXT, retire_alert_configuration, retire_evaluator)
