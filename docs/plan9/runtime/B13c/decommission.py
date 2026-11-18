"""Archive the used Reports application and remove its actual agent/host."""
import json
from manage import Run, now, write_json
from integration import Batch


def decommission(batch):
    run, api = batch.run, batch.api
    assert json.loads((run.out / 'archival-acceptance.json').read_text(encoding='utf-8'))['passed']
    file = run.out / 'reports-decommission.json'
    state = json.loads(file.read_text(encoding='utf-8')) if file.exists() else {'steps': [], 'started_at': now()}
    if state.get('passed'):
        return
    owner = batch.map['hosts']['reports']
    host_id = owner['host_id']
    host_path = '/enterprise/hosts/' + host_id
    orders = api.call('GET', '/enterprise/hosts/' + batch.map['hosts']['orders']['host_id'])
    assert orders['connection_status'] == 'online' and orders['labels']['plan9_run'] == run.id

    def done(step):
        if step not in state['steps']:
            state['steps'].append(step)
        write_json(file, state)
        print(step, flush=True)

    if 'related-access-retired' not in state['steps']:
        host = api.call('GET', host_path)
        assert host['labels']['plan9_run'] == run.id
        assert not any(r['host_id'] == host_id for r in api.call('GET', '/enterprise/remote-access-sessions?scope=active')['items'])
        policy = batch.map['alert_policies']['service']
        current = api.call('GET', '/alerts/configurations/' + policy['id'])
        assert current['name'] == policy['name']
        if current['spec']['enabled']:
            api.confirm(api.call('POST', '/alerts/configurations/preview', {'kind': 'policy', 'operation': 'disable', 'id': current['id'],
                            'expected_version': current['version']}, key='stop-owned-history-policy', expected=202))
        member = batch.map['member_terminal']
        grant_path = '/enterprise/remote-access-grants/' + member['grant_id']
        grant = api.call('GET', grant_path)
        assert grant['subject_id'] == batch.map['reader']['user_id'] and grant['host_ids'] == [host_id]
        if grant['status'] == 'enabled':
            grant = api.call('POST', grant_path + '/disable?expected_version=' + str(grant['version']))
            api.login()
        if grant['status'] == 'disabled':
            api.call('POST', grant_path + '/archive?expected_version=' + str(grant['version']))
            api.login()
        admin_path = '/enterprise/remote-access-grants/' + batch.map['remote']['grant_id']
        admin = api.call('GET', admin_path)
        assert admin['subject_id'] == api.user['id'] and set(admin['host_ids']) <= {orders['id'], host_id}
        if host_id in admin['host_ids']:
            if admin['status'] == 'enabled':
                admin = api.call('POST', admin_path + '/disable?expected_version=' + str(admin['version']))
                api.login()
            body = {key: admin[key] for key in ['subject_type', 'subject_id', 'protocols', 'actions', 'valid_from', 'valid_until']}
            body.update(host_ids=[orders['id']], managed_account_ids=[batch.map['remote']['accounts']['orders']], expected_version=admin['version'])
            admin = api.call('PUT', admin_path, body)
            api.login()
        if admin['status'] != 'enabled':
            api.call('POST', admin_path + '/enable?expected_version=' + str(admin['version']))
            api.login()
        for ident in [member['account_id'], batch.map['remote']['accounts']['reports']]:
            account = next(row for row in api.call('GET', '/enterprise/managed-accounts')['items'] if row['id'] == ident)
            assert account['host_id'] == host_id
            if account['status'] == 'active':
                api.call('PUT', '/enterprise/managed-accounts/' + ident, {'status': 'disabled', 'expected_version': account['version']})
                api.login()
        done('related-access-retired')

    app = batch.map['applications']['reports']
    app_path = '/applications/' + app['id']
    if 'application-archived' not in state['steps']:
        stream = api.call('GET', app_path + '/dashboard-bindings')
        for binding in stream['items']:
            assert binding['dashboard']['id'] == batch.map['batch_dashboard']['id']
            api.confirm(api.call('POST', app_path + '/dashboard-bindings/preview', {'operation': 'detach', 'dashboard_id': binding['dashboard']['id'],
                            'binding_id': binding['id'], 'expected_binding_version': binding['version'],
                            'expected_dashboard_version': binding['dashboard']['version'], 'expected_resource_version': stream['resource_version']},
                            key='end-reports-board-shortcut', expected=201))
        for key in ['reports', 'redis']:
            step = 'binding-ended-' + key
            if step not in state['steps']:
                api.confirm(api.call('POST', '/monitoring/commands/preview', {'kind': 'binding', 'operation': 'delete', 'id': app['bindings'][key],
                                'expected_version': 1}, key='end-reports-' + key, expected=201))
                done(step)
        row = api.call('GET', app_path)
        assert row['name'] == app['name']
        if row['status'] == 'active':
            api.confirm(api.call('POST', '/monitoring/commands/preview', {'kind': 'application', 'operation': 'archive', 'id': row['id'],
                            'expected_version': row['version']}, key='archive-reports-app', expected=201))
        assert api.call('GET', app_path)['status'] == 'archived'
        done('application-archived')

    if 'host-uninstalled' not in state['steps']:
        host = api.call('GET', host_path)
        assert host['labels']['plan9_run'] == run.id
        for entry in api.call('GET', '/monitoring/builtins?target_type=host&target_id=' + host_id):
            collection = entry.get('collection') or {}
            if collection.get('desired_enabled'):
                api.confirm(api.call('POST', '/monitoring/builtins/preview', {'target': {'kind': 'host', 'id': host_id}, 'key': entry['definition']['key'],
                                'enabled': False, 'expected_version': collection['version'], 'config': collection['config']}, key='disable-reports-' + collection['id'], expected=201))
                batch.converge('reports')
        if 'removal_id' not in state:
            if host.get('removal_operation_id'):
                state['removal_id'] = host['removal_operation_id']
            else:
                connection = api.call('POST', '/enterprise/hosts/connection-tests', {'address': host['address'], 'port': host['port'], 'platform': 'linux',
                                      'ssh_path': 'direct_executor', 'credential_id': owner['credential_id'], 'username': 'root'}, key='reports-final-ssh', expected=202)
                connection = api.wait('/enterprise/connection-tests/' + connection['id'], lambda x: x['status'] == 'succeeded', timeout=65, label='Final Reports uninstall connection')
                preview = api.call('POST', '/enterprise/host-removals/actions/preview', {'target_type': 'managed_host', 'target_id': host_id,
                                   'mode': 'uninstall', 'expected_version': host['resource_version'], 'credential_id': owner['credential_id'],
                                   'connection_test_id': connection['id']}, key='uninstall-owned-reports', expected=201)
                result = api.confirm(preview)
                state['removal_id'] = result['execution']['operation_ref']['id']
            write_json(file, state)
        removed = api.wait('/enterprise/host-removal-operations/' + state['removal_id'], lambda x: x['status'] == 'succeeded' and x['local_cleanup'] == 'verified',
                           timeout=120, label='Verified Reports local uninstall')
        native = run.shell('reports', 'systemctl show -p LoadState --value argus-connector.service argus-otelcol.service\n').split()
        assert native == ['not-found', 'not-found']
        run.save('reports-uninstall-native', {'operation': removed, 'local_units': native, 'verified_at': now()})
        done('host-uninstalled')
    if 'host-deleted' not in state['steps']:
        row = api.call('GET', host_path, expected=[200, 404])
        if not row.get('code'):
            assert row['status'] == 'uninstalled'
            api.confirm(api.call('POST', host_path + '/actions/preview-delete', {'expected_version': row['resource_version']}, key='delete-uninstalled-reports', expected=201))
        assert not any(row['id'] == host_id for row in api.call('GET', '/enterprise/hosts')['items'])
        done('host-deleted')
    for entry in [owner, batch.map['member_terminal']]:
        credential = next(x for x in api.call('GET', '/enterprise/credentials')['items'] if x['id'] == entry['credential_id'])
        assert credential['name'].startswith(run.id)
        if credential['status'] == 'active':
            api.call('PUT', '/enterprise/credentials/' + credential['id'], {'status': 'disabled', 'expected_version': credential['version']})
        secret = api.call('GET', '/enterprise/secrets/' + entry['secret_id'])
        assert secret['name'].startswith(run.id)
        if secret['status'] == 'active':
            api.call('DELETE', '/enterprise/secrets/' + secret['id'] + '?expected_version=' + str(secret['version']), expected=204)
    grant_path = '/enterprise/data-authorizations/role/' + batch.map['evaluator']['role_id']
    for kind, ident in [('host', host_id), ('application', app['id']), ('monitoring_target', batch.map['targets']['memcached']['target_id'])]:
        scope = api.call('GET', grant_path + '?resource_type=' + kind)
        if any(x['resource_id'] == ident and x['direct'] for x in scope['items']):
            api.call('POST', grant_path, {'resource_type': kind, 'resource_ids': [ident], 'remove': True, 'expected_version': scope['authorization_version']}, expected=204)
    for service in ['reports', 'memcached']:
        run.container(service)
        run.compose('stop', '-t', '5', service, timeout=30)
        assert not run.container(service)['State']['Running']
    batch.map['retired_hosts'] = ['reports']
    batch.map['targets']['memcached']['status'] = 'archived'
    app.update(status='archived', ended_bindings=list(app['bindings']))
    batch.save()
    state.update(passed=True, at=now(), host_id=host_id, archived_application=app['id'], stopped_native_services=['reports', 'memcached'],
                 retained_host=orders['id'], historical_alerts_and_telemetry_retained=True)
    write_json(file, state)
    print('Reports application archived, local agents removed and host deleted; Orders and Redis remain online', flush=True)


if __name__ == '__main__':
    run = Run()
    api = run.api()
    api.login()
    decommission(Batch(run, api))
