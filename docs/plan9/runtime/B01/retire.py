"""Normal API retirement of this owned batch; historical evidence is retained."""
import argparse
import json
from pathlib import Path
import sys

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent / 'common'))
from api import Api
from manage import Run, write_json, now, execute


def retire(run, api):
    mapping = json.loads(run.resources.read_text(encoding='utf-8'))
    checkpoint = run.out / 'cleanup.json'
    state = json.loads(checkpoint.read_text(encoding='utf-8')) if checkpoint.exists() else {
        'run_id': run.id, 'started_at': now(), 'authorization': 'User requested switching to B02a',
        'status': 'destroying', 'hosts': {}, 'steps': [], 'history_preserved': True}
    if state['status']=='destroyed':
        for kind in ['container','volume','network']:
            assert not execute(['docker',kind,'ls','-q','--filter','label=io.argus.plan9.run='+run.id]).strip()
        print('Already retired; owned Docker resources remain absent.',flush=True)
        return state

    def save(step):
        if step not in state['steps']:
            state['steps'].append(step)
        write_json(checkpoint, state)
        run.event('retirement_step', step=step)
        print(step, flush=True)

    if 'inventory' not in state['steps']:
        baseline = {}
        for key, entry in mapping['hosts'].items():
            host = api.call('GET', '/enterprise/hosts/' + entry['host_id'])
            assert host['labels']['plan9_run'] == run.id and host['name'] == entry['name']
            container = run.container(key)
            assert container['Id'] == next(x['id'] for x in mapping['containers'] if x['service'] == key)
            baseline[key] = host
        write_json(run.out / 'cleanup-platform-before.json', baseline)
        write_json(run.out / 'cleanup-volumes-before.json', json.loads(execute(['docker', 'volume', 'inspect',
            run.id + '_data-a', run.id + '_data-b'])))
        save('inventory')

    for key, policy in mapping['alert_policies'].items():
        step = 'policy-retired-' + key
        if step in state['steps']:
            continue
        row = api.call('GET', '/alerts/configurations/' + policy['id'], expected=[200, 404])
        if row and not row.get('code'):
            assert row['name'] == policy['name']
            preview = api.call('POST', '/alerts/configurations/preview', {
                'kind': 'policy', 'operation': 'delete', 'id': row['id'], 'expected_version': row['version']},
                key=step, expected=202)
            api.confirm(preview)
        save(step)

    for key, entry in mapping['hosts'].items():
        stage = state['hosts'].setdefault(key, {})
        hid = entry['host_id']
        if not stage.get('collections_disabled'):
            rows = api.call('GET', '/monitoring/builtins?target_type=host&target_id=' + hid)
            for item in rows:
                c = item.get('collection')
                if not c or c['status'] != 'active' or not c['desired_enabled']:
                    continue
                assert c['target']['id'] == hid
                preview = api.call('POST', '/monitoring/builtins/preview', {
                    'target': {'kind':'host','id':hid}, 'key':item['definition']['key'],
                    'enabled':False, 'expected_version':c['version'], 'config':c['config']},
                    key='disable-' + c['id'] + '-' + str(c['version']), expected=201)
                api.confirm(preview)
            api.wait('/enterprise/hosts/'+hid+'/collector', lambda x: x['status']=='converged', timeout=180, label='Stopped metrics configuration applied')
            stage['collections_disabled'] = True
            save('collections-retired-' + key)
        if not stage.get('removal_action'):
            host = api.call('GET', '/enterprise/hosts/' + hid)
            test = api.call('POST', '/enterprise/hosts/connection-tests', {
                'address': host['address'], 'port': host['port'], 'platform': 'linux',
                'ssh_path': 'direct_executor', 'credential_id': entry['credential_id'], 'username': 'root'},
                key='removal-connection-' + key, expected=202)
            test = api.wait('/enterprise/connection-tests/' + test['id'],
                lambda value: value['status'] == 'succeeded', timeout=90, label='Removal SSH preflight')
            preview = api.call('POST', '/enterprise/host-removals/actions/preview', {
                'target_type': 'managed_host', 'target_id': hid, 'expected_version': host['resource_version'],
                'mode': 'uninstall', 'credential_id': entry['credential_id'], 'connection_test_id': test['id']},
                key='remove-' + key, expected=201)
            stage['removal_action'] = preview['action_ref']
            write_json(checkpoint, state)
            result = api.confirm(preview)
            write_json(run.out / ('cleanup-' + key + '-removal-result.json'), result)
            stage['removal_result'] = result
            save('removal-confirmed-' + key)
        if not stage.get('local_cleanup_verified'):
            host = api.call('GET', '/enterprise/hosts/' + hid)
            operation_id = host['removal_operation_id']
            def removed(value):
                if value['status'] in ('failed', 'cleanup_unknown'):
                    raise RuntimeError('Host removal failed: ' + str(value.get('error_code')))
                return value['status'] == 'succeeded' and value['local_cleanup'] == 'verified'
            operation = api.wait('/enterprise/host-removal-operations/' + operation_id,
                removed, timeout=300, label='Verified host software removal')
            write_json(run.out / ('cleanup-' + key + '-verified.json'), operation)
            assert run.shell(key, 'systemctl show -p LoadState --value argus-connector.service argus-otelcol.service\n').split() == ['not-found', 'not-found']
            stage['local_cleanup_verified'] = True
            save('local-uninstall-verified-' + key)

    if 'evaluator-scope-revoked-and-replaced' not in state['steps']:
        evaluator = mapping['evaluator']
        grants_path = '/enterprise/data-authorizations/role/' + evaluator['role_id']
        grants = api.call('GET', grants_path + '?resource_type=host')
        owned = {x['host_id'] for x in mapping['hosts'].values()}
        revoke = [x['resource_id'] for x in grants['items'] if x['direct'] and x['resource_id'] in owned]
        if revoke:
            api.call('POST', grants_path, {'resource_type': 'host', 'resource_ids': revoke,
                'remove': True, 'expected_version': grants['authorization_version']}, expected=204)
        # Keep a shared Plan9 evaluator with no old batch grants; disable the old
        # immutable-named B01 identity after switching the enterprise singleton.
        common_path = run.out.parent / 'common-evaluator.json'
        common = json.loads(common_path.read_text(encoding='utf-8')) if common_path.exists() else {}
        for field, endpoint, payload in [
            ('service_account_id','/enterprise/service-accounts',{'name':'Plan9 scenario evaluator','description':'Current Plan9 batch only; no API keys or tools','allowed_tool_ids':[]}),
            ('role_id','/enterprise/roles',{'name':'Plan9 scenario metric reader','description':'Explicit current batch observations only','permissions':['host.read','monitoring.read']})]:
            if not common.get(field):
                row=api.call('POST',endpoint,payload,key='common-'+field,expected=201)
                common[field]=row['id'];write_json(common_path,common)
        if not common.get('role_binding_id'):
            row=api.call('POST','/enterprise/role-bindings',{'subject_type':'service_account','subject_id':common['service_account_id'],'role_id':common['role_id']},key='common-evaluator-binding',expected=201)
            common['role_binding_id']=row['id'];write_json(common_path,common)
        setting=api.call('GET','/alerts/configurations/'+evaluator['settings_id'])
        if setting['spec']['evaluator_id']!=common['service_account_id']:
            preview=api.call('POST','/alerts/configurations/preview',{'kind':'settings','operation':'update','id':setting['id'],'name':'Plan9 background evaluator','expected_version':setting['version'],'spec':{'evaluator_id':common['service_account_id']}},key='common-evaluator-settings',expected=202)
            api.confirm(preview)
        common['settings_id']=setting['id'];write_json(common_path,common)
        accounts=api.call('GET','/enterprise/service-accounts')['items']
        account=next(x for x in accounts if x['id']==evaluator['service_account_id'])
        if account['status']=='active':
            api.call('PUT','/enterprise/service-accounts/'+account['id'],{'status':'disabled','description':'Retired B01 evaluator; historical audit identity retained','allowed_tool_ids':[],'expected_version':account['version']})
        bindings=api.call('GET','/enterprise/role-bindings')['items']
        old_binding=next((x for x in bindings if x['id']==evaluator['role_binding_id']),None)
        if old_binding:
            api.call('DELETE','/enterprise/role-bindings/'+old_binding['id']+'?expected_version='+str(old_binding['version']),expected=204)
        roles=api.call('GET','/enterprise/roles')['items']
        old_role=next((x for x in roles if x['id']==evaluator['role_id']),None)
        if old_role:
            api.call('DELETE','/enterprise/roles/'+old_role['id']+'?expected_version='+str(old_role['version']),expected=204)
        state['retained_shared_evaluator']={**common,'old_host_grants_revoked':True,'legacy_evaluator_disabled':account['id']}
        save('evaluator-scope-revoked-and-replaced')


    for key, entry in mapping['hosts'].items():
        stage = state['hosts'][key]
        if not stage.get('record_deleted'):
            host = api.call('GET', '/enterprise/hosts/' + entry['host_id'], expected=[200, 404])
            if host and not host.get('code'):
                assert host['status'] == 'uninstalled'
                preview = api.call('POST', '/enterprise/hosts/' + entry['host_id'] + '/actions/preview-delete',
                    {'expected_version': host['resource_version']}, key='delete-' + key, expected=201)
                api.confirm(preview)
            stage['record_deleted'] = True
            save('host-record-deleted-' + key)
        credentials = api.call('GET', '/enterprise/credentials')['items']
        credential = next(x for x in credentials if x['id'] == entry['credential_id'])
        if credential['status'] == 'active':
            api.call('PUT', '/enterprise/credentials/' + credential['id'],
                {'status':'disabled','expected_version':credential['version']})
        secret = api.call('GET', '/enterprise/secrets/' + entry['secret_id'])
        if secret['status'] == 'active':
            api.call('DELETE', '/enterprise/secrets/' + secret['id'] + '?expected_version=' + str(secret['version']), expected=204)
        save('credentials-disabled-' + key)

    if 'compose-destroyed' not in state['steps']:
        for key in mapping['hosts']:
            run.container(key)  # Exact project/run ownership check before deletion.
        for volume in json.loads(execute(['docker', 'volume', 'inspect', run.id+'_data-a', run.id+'_data-b'])):
            assert volume['Labels']['io.argus.plan9.run'] == run.id
        run.compose('down', '--volumes', timeout=120)
        save('compose-destroyed')
    for kind in ['container', 'volume', 'network']:
        found = execute(['docker', kind, 'ls', '-q', '--filter', 'label=io.argus.plan9.run=' + run.id])
        assert not found.strip(), 'owned Docker resources remain: ' + kind
    hosts = api.call('GET', '/enterprise/hosts')['items']
    assert not any(x['id'] in owned for x in hosts)
    state.update(status='destroyed', finished_at=now(), containers_volumes_networks_remaining=0,
        active_owned_hosts_remaining=0, fixed_collection_history_retained=True,
        review_status='advanced_by_user_request_without_explicit_acceptance')
    save('cleanup-complete')
    return state


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--run', default='p9-b01-20261026-01')
    parser.add_argument('--session', required=True)
    parser.add_argument('--ca', required=True)
    args = parser.parse_args()
    run = Run(args.run)
    retire(run, Api(run, args.session, args.ca))
