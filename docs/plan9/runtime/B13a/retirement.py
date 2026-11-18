"""Retire the explicitly advanced B13a scene; preserve evidence and formal data."""
import json
from integration import Batch
from manage import Run, execute, write_json, now
from common.retirement import retire


def cleanup(batch, next_batch='B13b', before_resources=None, after_resources=None):
    run, api = batch.run, batch.api
    final = run.out / 'cleanup-verification.json'
    if final.exists() and json.loads(final.read_text(encoding='utf-8'))['status'] == 'destroyed':
        return
    authorization = json.loads((run.out / 'advance-authorization.json').read_text(encoding='utf-8'))
    assert authorization['cleanup_authorized'] and authorization['next_batch'] == next_batch
    before = authorization['formal_before']
    state_file = run.out / 'platform-retirement.json'
    state = json.loads(state_file.read_text(encoding='utf-8')) if state_file.exists() else {'steps': [], 'at': now()}
    def done(step):
        if step not in state['steps']:
            state['steps'].append(step)
        write_json(state_file, state)
        print(step, flush=True)
    host_ids = {x['host_id'] for x in batch.map['hosts'].values()}
    if 'evidence-sealed' not in state['steps']:
        assert all(api.call('GET', '/enterprise/hosts/' + hid)['labels']['plan9_run'] == run.id for hid in host_ids)
        for container in batch.map['containers']:
            run.container(container['service'])
        run.save('retirement-evidence', {'at': now(), 'authorization': authorization['statement'], 'human_pass_not_claimed': True,
                 'hosts': [api.call('GET', '/enterprise/hosts/' + hid) for hid in host_ids],
                 'alerts': api.call('GET', '/alerts/instances'), 'formal': before,
                 'delivery': {key: json.loads(run.shell(key, 'cat /var/lib/argus-otelcol/identity/delivery-state.json\n')) for key in batch.map['hosts']},
                 'known_findings_retained': authorization.get('known_findings_retained', [])})
        done('evidence-sealed')
    if 'policy-and-workload-stopped' not in state['steps']:
        for policy in batch.map['alert_policies'].values():
            current = api.call('GET', '/alerts/configurations/' + policy['id'], expected=[200, 404])
            if not current.get('code'):
                assert current['name'] == policy['name']
                api.confirm(api.call('POST', '/alerts/configurations/preview', {'kind': 'policy', 'operation': 'delete', 'id': current['id'],
                                'expected_version': current['version']}, key='retire-policy-' + current['id'], expected=202))
        prefix = 'plan9-' + run.BATCH.lower()
        for key in batch.map['hosts']:
            run.shell(key, f'for unit in {prefix}-traffic.service {prefix}.service; do\n'
                      'state=$(systemctl show -p LoadState --value "$unit" || true)\n'
                      'case "$state" in loaded) systemctl stop "$unit" ;; not-found|"") ;; *) exit 1 ;; esac\ndone\n')
        done('policy-and-workload-stopped')
    if before_resources and 'batch-specific-configurations-retired' not in state['steps']:
        before_resources(batch)
        done('batch-specific-configurations-retired')
    if 'remote-access-retired' not in state['steps']:
        for session in api.call('GET', '/enterprise/remote-access-sessions?scope=active')['items']:
            if session['host_id'] in host_ids:
                api.call('POST', '/enterprise/remote-access-sessions/' + session['id'] + '/terminate', {'reason': 'User advanced Plan9 to ' + next_batch})
                api.wait('/enterprise/remote-access-sessions/' + session['id'], lambda x: x['status'] not in ['authorized', 'connecting', 'active', 'terminating'], timeout=90)
        remote = batch.map['remote']
        path = '/enterprise/remote-access-grants/' + remote['grant_id']
        grant = api.call('GET', path)
        assert set(grant['host_ids']) == host_ids
        if grant['status'] == 'enabled':
            grant = api.call('POST', path + '/disable?expected_version=' + str(grant['version']))
            api.login()
        if grant['status'] != 'archived':
            api.call('POST', path + '/archive?expected_version=' + str(grant['version']))
            api.login()
        for ident in remote['accounts'].values():
            account = next(x for x in api.call('GET', '/enterprise/managed-accounts')['items'] if x['id'] == ident)
            assert account['host_id'] in host_ids
            if account['status'] == 'active':
                api.call('PUT', '/enterprise/managed-accounts/' + ident, {'status': 'disabled', 'expected_version': account['version']})
                api.login()
        done('remote-access-retired')
    owned_resources = host_ids | {x['id'] for x in batch.map['applications'].values()} | {x['target_id'] for x in batch.map['targets'].values()}
    for key in ['batch_dashboard', 'personal_dashboard']:
        step = 'dashboard-archived-' + key
        if key not in batch.map or step in state['steps']:
            continue
        ident = batch.map[key]['id']
        detail = api.call('GET', '/dashboards/' + ident)['dashboard']
        assert detail['name'].startswith(run.BATCH)
        if detail['lifecycle'] == 'active':
            bindings = api.call('GET', '/dashboards/' + ident + '/bindings')
            for binding in bindings:
                assert binding['resource_id'] in owned_resources
                kind = binding['resource_type']
                resource = binding['resource_id']
                path = ('/hosts/' if kind == 'host' else '/applications/' if kind == 'application' else '/monitoring/targets/') + resource + '/dashboard-bindings'
                current = api.call('GET', path)
                row = next(x for x in current['items'] if x['id'] == binding['id'])
                api.confirm(api.call('POST', path + '/preview', {'operation': 'detach', 'dashboard_id': ident, 'binding_id': row['id'],
                                'expected_binding_version': row['version'], 'expected_dashboard_version': row['dashboard']['version'],
                                'expected_resource_version': current['resource_version']}, key='retire-board-binding-' + row['id'], expected=201))
            detail = api.call('GET', '/dashboards/' + ident)['dashboard']
            api.confirm(api.call('POST', '/dashboards/' + ident + '/actions/preview', {'id': ident, 'operation': 'archive',
                            'expected_version': detail['version'], 'name': detail['name'], 'description': detail['description'], 'sort_order': 0}, key=step, expected=201))
        done(step)
    for key, app in batch.map['applications'].items():
        for name, ident in app['bindings'].items():
            step = 'binding-ended-' + ident
            if step in state['steps'] or name in app.get('ended_bindings', []):
                continue
            api.confirm(api.call('POST', '/monitoring/commands/preview', {'kind': 'binding', 'operation': 'delete', 'id': ident,
                            'expected_version': 1}, key=step, expected=201))
            done(step)
        step = 'application-archived-' + key
        if step not in state['steps']:
            row = api.call('GET', '/applications/' + app['id'])
            assert row['name'] == app['name']
            if row['status'] == 'active':
                api.confirm(api.call('POST', '/monitoring/commands/preview', {'kind': 'application', 'operation': 'archive', 'id': row['id'],
                                'expected_version': row['version']}, key=step, expected=201))
            done(step)
    if 'test-reader-retired' not in state['steps']:
        member = batch.map['reader']
        user = api.call('GET', '/enterprise/users/' + member['user_id'])
        assert user['username'] == member['username']
        if user['status'] != 'disabled':
            api.call('PUT', '/enterprise/users/' + user['id'], {'status': 'disabled', 'expected_version': user['version']})
        assignments = api.call('GET', '/enterprise/users/' + user['id'] + '/role-assignments')
        user = api.call('GET', '/enterprise/users/' + user['id'])
        if member['role_id'] in assignments['direct_role_ids']:
            assert assignments['direct_role_ids'] == [member['role_id']]
            api.call('PUT', '/enterprise/users/' + user['id'] + '/role-assignments', {'role_ids': [], 'department_id': user['department_id'],
                     'expected_user_version': user['version'], 'expected_authorization_version': assignments['authorization_version']})
        path = '/enterprise/data-authorizations/role/' + member['role_id']
        for kind in ['host', 'application', 'monitoring_target', 'dashboard']:
            grants = api.call('GET', path + '?resource_type=' + kind)
            ids = [x['resource_id'] for x in grants['items'] if x['direct']]
            if ids:
                api.call('POST', path, {'resource_type': kind, 'resource_ids': ids, 'remove': True, 'expected_version': grants['authorization_version']}, expected=204)
        role = next(x for x in api.call('GET', '/enterprise/roles')['items'] if x['id'] == member['role_id'])
        assert role['name'] == run.BATCH + ' scoped reader'
        if role['status'] == 'active':
            api.call('DELETE', '/enterprise/roles/' + role['id'] + '?expected_version=' + str(role['version']), expected=204)
        done('test-reader-retired')
    retire(run, api, execute, write_json, now)
    if after_resources and 'batch-specific-identities-retired' not in state['steps']:
        after_resources(batch)
        done('batch-specific-identities-retired')
    endpoint = batch.map['telemetry_network_endpoint']
    cli = ['kubectl', '--context', 'docker-desktop', '-n', endpoint['namespace']]
    raw = execute([*cli, 'get', 'service', endpoint['name'], '--ignore-not-found', '-o', 'json'])
    if raw:
        service = json.loads(raw)
        assert service['metadata']['uid'] == endpoint['uid'] and service['metadata']['labels']['argus.plan9/run'] == run.id
        execute([*cli, 'delete', 'service', endpoint['name'], '--wait=true'])
    assert before == run.platform()
    run.save('cleanup-verification', {'status': 'destroyed', 'at': now(), 'human_pass_not_claimed': True, 'known_findings_preserved': True,
             'formal_pvcs_and_deployments_preserved': True, 'model_workspace_accounts_and_history_retained': True,
             'owned_apps_boards_targets_archived': True, 'test_member_disabled_and_grants_removed': True})
    print(run.BATCH + ' retired through normal APIs; formal data, history and findings retained', flush=True)


if __name__ == '__main__':
    run = Run()
    api = run.api()
    api.login()
    cleanup(Batch(run, api))
