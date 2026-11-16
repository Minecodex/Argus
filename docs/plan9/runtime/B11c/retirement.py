"""Future user-authorized member-first/Bastion-last product retirement."""
import json
import uuid
from integration import Batch
from manage import Run,execute,now


def retire(batch):
    run=batch.run;api=batch.api;file=run.out/'cleanup.json'
    state=json.loads(file.read_text()) if file.exists() else {'run_id':run.id,'status':'destroying','started_at':now(),'steps':[],'hosts':{}}
    if state['status']=='destroyed':return
    before=run.platform();host_ids={row['host_id'] for row in batch.map['hosts'].values()}
    def save(step):
        if step not in state['steps']:state['steps'].append(step)
        run.save('cleanup',state);print(step,flush=True)
    if 'ownership-archived' not in state['steps']:
        hosts={key:api.call('GET','/enterprise/hosts/'+item['host_id']) for key,item in batch.map['hosts'].items()}
        assert all(row['labels']['plan9_run']==run.id for row in hosts.values())
        for key in hosts:run.container(key)
        scope=api.call('GET','/enterprise/bastion-scopes/'+batch.scope);assert scope['labels']['plan9_run']==run.id and scope['member_count']==1
        run.save('retirement-archive',{'hosts':hosts,'scope':scope,'alerts':api.call('GET','/alerts/instances'),
            'recordings':api.call('GET','/enterprise/remote-access-recordings'),'formal':before,'at':now()});save('ownership-archived')
    if 'remote-access-retired' not in state['steps']:
        for session in api.call('GET','/enterprise/remote-access-sessions?scope=active')['items']:
            if session['host_id'] in host_ids:
                api.call('POST','/enterprise/remote-access-sessions/'+session['id']+'/terminate',{'reason':'User advanced Plan9 beyond B11c'})
                api.wait('/enterprise/remote-access-sessions/'+session['id'],lambda row:row['status'] not in ['authorized','connecting','active','terminating'],timeout=90)
        remote=batch.map['remote'];base='/enterprise/remote-access-grants/'+remote['grant_id'];grant=api.call('GET',base)
        assert set(grant['host_ids'])==host_ids
        if grant['status']=='enabled':grant=api.call('POST',base+'/disable?expected_version='+str(grant['version']));api.login()
        if grant['status']!='archived':api.call('POST',base+'/archive?expected_version='+str(grant['version']));api.login()
        for ident in remote['accounts'].values():
            account=next(row for row in api.call('GET','/enterprise/managed-accounts')['items'] if row['id']==ident);assert account['host_id'] in host_ids
            if account['status']=='active':api.call('PUT','/enterprise/managed-accounts/'+ident,{'expected_version':account['version'],'status':'disabled'});api.login()
        save('remote-access-retired')
    for key,policy in batch.map['alert_policies'].items():
        step='policy-retired-'+key
        if step in state['steps']:continue
        row=api.call('GET','/alerts/configurations/'+policy['id'],expected=[200,404])
        if not row.get('code'):
            assert row['name']==policy['name'];api.confirm(api.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'delete','id':row['id'],'expected_version':row['version']},key=step,expected=202))
        save(step)
    for key in ['member','bastion']:
        info=batch.map['hosts'][key];hid=info['host_id'];stage=state['hosts'].setdefault(key,{})
        if not stage.get('collections_stopped'):
            run.shell(key,'systemctl stop plan9-b11c-traffic.service || true\n')
            path='/monitoring/builtins?target_type=host&target_id='+hid
            for item in api.call('GET',path):
                row=item.get('collection') or {}
                if not row.get('desired_enabled'):continue
                api.confirm(api.call('POST','/monitoring/builtins/preview',{'target':{'kind':'host','id':hid},'key':item['definition']['key'],
                    'enabled':False,'expected_version':row['version'],'config':row['config']},key='stop-'+row['id'],expected=201))
                batch.converge(key)
            stage['collections_stopped']=True;save(key+'-collections-stopped')
        if not stage.get('collector_uninstalled'):
            # An edge gateway must release its Collector/route/tunnel dependencies
            # before the Bastion removal preview is eligible for confirmation.
            if not stage.get('deleted'):
                base='/enterprise/hosts/'+hid+'/collector';collector=api.call('GET',base)
                if collector['status']!='uninstalled':
                    result=api.confirm(api.call('POST',base+'/actions/preview-uninstall',{
                        'expected_version':collector['version'],'distribution_version_id':collector['distribution_version_id'],
                        'profile_ids':[],'route_kind':collector['route']['kind'],'transport':collector['route']['transport']},
                        key=key+'-collector-uninstall-'+str(collector['version']),expected=201))
                    collector=api.wait(base,lambda row:row['status']=='uninstalled',timeout=180,label=key+' Collector uninstall')
                run.save(key+'-collector-uninstall-verified',collector)
            else:
                assert stage['uninstalled']
                evidence=json.loads((run.out/(key+'-uninstall-verified.json')).read_text())
                assert evidence['status']=='succeeded' and evidence['local_cleanup']=='verified'
            stage['collector_uninstalled']=True;save(key+'-collector-uninstalled')
        if not stage.get('removal_id'):
            host=api.call('GET','/enterprise/hosts/'+hid)
            test_body={'address':host['address'],'port':host['port'],'platform':'linux','ssh_path':'bastion_connector' if key=='member' else 'direct_executor',
                'credential_id':info['credential_id'],'username':'root'}
            if key=='member':test_body['bastion_scope_id']=batch.scope
            test=api.call('POST','/enterprise/hosts/connection-tests',test_body,key='uninstall-connection-'+uuid.uuid4().hex,expected=202)
            test=api.wait('/enterprise/connection-tests/'+test['id'],lambda row:row['status']=='succeeded',timeout=150)
            target=api.call('GET','/enterprise/bastion-scopes/'+batch.scope) if key=='bastion' else host
            # A rejected confirmation is not replayed: obtain a fresh proof and
            # preview after rereading current identity/version/authorization.
            attempt=uuid.uuid4().hex
            preview=api.call('POST','/enterprise/host-removals/actions/preview',{'target_type':'bastion_scope' if key=='bastion' else 'managed_host',
                'target_id':batch.scope if key=='bastion' else hid,'mode':'uninstall','expected_version':target['resource_version'],
                'credential_id':info['credential_id'],'connection_test_id':test['id']},key=key+'-uninstall-'+attempt,expected=201)
            run.save(key+'-uninstall-preview-'+attempt,{'preview':preview,'target':target,'connection_test':test,'at':now()})
            result=api.confirm(preview);stage['removal_id']=result['execution']['operation_ref']['id'];save(key+'-uninstall-submitted')
        if not stage.get('uninstalled'):
            def completed(row):
                if row['status'] in ['failed','cleanup_unknown']:raise RuntimeError(key+' uninstall failed: '+str(row.get('error_code')))
                return row['status']=='succeeded' and row['local_cleanup']=='verified'
            operation=api.wait('/enterprise/host-removal-operations/'+stage['removal_id'],completed,timeout=300,label=key+' verified uninstall')
            assert run.shell(key,'systemctl show argus-connector.service argus-otelcol.service -p LoadState --value\n').split()==['not-found','not-found']
            run.save(key+'-uninstall-verified',operation);stage['uninstalled']=True;save(key+'-uninstalled')
        if not stage.get('deleted'):
            if key=='member':
                host=api.call('GET','/enterprise/hosts/'+hid,expected=[200,404])
                if not host.get('code'):api.confirm(api.call('POST','/enterprise/hosts/'+hid+'/actions/preview-delete',{'expected_version':host['resource_version']},key='delete-member',expected=201))
            else:
                scope=api.call('GET','/enterprise/bastion-scopes/'+batch.scope,expected=[200,404])
                if not scope.get('code'):
                    assert scope['member_count']==0;api.confirm(api.call('POST','/enterprise/bastion-scopes/'+batch.scope+'/actions/preview-delete',{'expected_version':scope['resource_version']},key='delete-bastion-scope',expected=201))
            stage['deleted']=True;save(key+'-retired')
        credential=next(row for row in api.call('GET','/enterprise/credentials')['items'] if row['id']==info['credential_id'])
        if credential['status']=='active':api.call('PUT','/enterprise/credentials/'+credential['id'],{'status':'disabled','expected_version':credential['version']})
        secret=api.call('GET','/enterprise/secrets/'+info['secret_id'])
        if secret['status']=='active':api.call('DELETE','/enterprise/secrets/'+secret['id']+'?expected_version='+str(secret['version']),expected=204)
    if 'evaluator-revoked' not in state['steps']:
        path='/enterprise/data-authorizations/role/'+batch.map['evaluator']['role_id'];grants=api.call('GET',path+'?resource_type=host')
        remove=[row['resource_id'] for row in grants['items'] if row['direct'] and row['resource_id'] in host_ids]
        if remove:api.call('POST',path,{'resource_type':'host','resource_ids':remove,'remove':True,'expected_version':grants['authorization_version']},expected=204)
        api.login();save('evaluator-revoked')
    if 'compose-destroyed' not in state['steps']:
        for key in ['bastion','member']:run.container(key)
        run.compose('down','--volumes',timeout=150);save('compose-destroyed')
    endpoint=batch.map['telemetry_network_endpoint'];cli=['kubectl','--context','docker-desktop','-n',endpoint['namespace']]
    current=execute([*cli,'get','service',endpoint['name'],'--ignore-not-found','-o','json'])
    if current:
        row=json.loads(current);assert row['metadata']['uid']==endpoint['uid'] and row['metadata']['labels']['argus.plan9/run']==run.id
        execute([*cli,'delete','service',endpoint['name'],'--wait=true'])
    job=batch.map.get('catalog_job')
    if job:
        raw=execute([*cli,'get','job',job['name'],'--ignore-not-found','-o','json'])
        if raw:
            row=json.loads(raw);assert row['metadata']['uid']==job['uid'] and row['metadata']['labels']['argus.plan9/run']==run.id
            execute([*cli,'delete','job',job['name'],'--wait=true'])
    for kind in ['container','volume','network']:assert not execute(['docker',kind,'ls','-q','--filter','label=io.argus.plan9.run='+run.id])
    assert before==run.platform();state.update(status='destroyed',finished_at=now(),history_preserved=True);save('cleanup-complete')
    run.save('cleanup-verification',{'run_id':run.id,'status':'destroyed','formal_resources_preserved':True,
        'sessions_recordings_alerts_audit_preserved':True,'shared_images_network_and_catalog_preserved':True,'at':now()})


if __name__=='__main__':
    run=Run();api=run.api();api.login();retire(Batch(run,api))
