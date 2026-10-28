"""Retire owned Plan9 services through normal product APIs, then Compose."""
import json


def retire(run, api, execute, write_json, now):
    mapping=json.loads(run.resources.read_text(encoding='utf-8'))
    output=run.out/'cleanup.json'
    state=json.loads(output.read_text(encoding='utf-8')) if output.exists() else {
        'run_id':run.id,'status':'destroying','started_at':now(),'steps':[], 'hosts':{},
        'history_preserved':True,'shared_evaluator_preserved':True}
    def save(step):
        if step not in state['steps']:state['steps'].append(step)
        write_json(output,state);run.event('retirement_step',step=step);print(step,flush=True)
    if state['status']=='destroyed':return state
    if 'inventory' not in state['steps']:
        baseline={}
        for key,h in mapping['hosts'].items():
            value=api.call('GET','/enterprise/hosts/'+h['host_id'])
            assert value['labels']['plan9_run']==run.id
            baseline[key]=value
        for c in mapping['containers']:
            assert run.container(c['service'])['Id']==c['id']
        write_json(run.out/'cleanup-platform-before.json',baseline)
        write_json(run.out/'cleanup-alerts-before.json',api.call('GET','/alerts/instances'))
        save('inventory')
    # Acknowledge the accepted batch's historical delivery evidence without
    # declaring a recovery or erasing the permanent loss record.
    owned_hosts={h['host_id'] for h in mapping['hosts'].values()}
    if 'accepted-delivery-evidence-acknowledged' not in state['steps']:
        for event in api.call('GET','/alerts/instances')['items']:
            if event['state']['kind']=='collector_delivery_gap' and set(event['resource_ids'])<=owned_hosts and not event['acknowledged']:
                api.call('POST','/alerts/instances/'+event['id']+'/handle',{'operation':'acknowledge','expected_handling_version':event['handling_version']},key='accept-delivery-'+event['id'])
        save('accepted-delivery-evidence-acknowledged')
    for key, p in mapping.get('alert_policies',{}).items():
        step='policy-retired-'+key
        if step in state['steps']:continue
        row=api.call('GET','/alerts/configurations/'+p['id'],expected=[200,404])
        if not row.get('code'):
            assert row['name']==p['name']
            preview=api.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'delete','id':row['id'],'expected_version':row['version']},key=step,expected=202)
            api.confirm(preview)
        save(step)
    for key,t in mapping.get('targets',{}).items():
        step='collection-archived-'+key
        if step in state['steps']:continue
        row=api.call('GET','/monitoring/collections/'+t['collection_id'])
        assert row['target']['id']==t['target_id']
        if row['status']=='active':
            preview=api.call('POST','/monitoring/commands/preview',{'kind':'collection','operation':'archive','id':row['id'],'expected_version':row['version'],'collection':{}},key=step,expected=201)
            api.confirm(preview)
        save(step)
    for key,h in mapping['hosts'].items():
        hid=h['host_id'];stage=state['hosts'].setdefault(key,{})
        if not stage.get('builtins_stopped'):
            for item in api.call('GET','/monitoring/builtins?target_type=host&target_id='+hid):
                row=item.get('collection')
                if not row or not row['desired_enabled']:continue
                preview=api.call('POST','/monitoring/builtins/preview',{'target':{'kind':'host','id':hid},'key':item['definition']['key'],'enabled':False,'expected_version':row['version'],'config':row['config']},key='stop-'+row['id'],expected=201)
                api.confirm(preview)
            api.wait('/enterprise/hosts/'+hid+'/collector',lambda x:x['status']=='converged',timeout=180,label='Final stopped collector configuration')
            stage['builtins_stopped']=True;save('builtins-stopped-'+key)
        if not stage.get('removal_id'):
            host=api.call('GET','/enterprise/hosts/'+hid)
            if host.get('removal_operation_id'):
                stage['removal_id']=host['removal_operation_id']
            else:
                connection=api.call('POST','/enterprise/hosts/connection-tests',{'address':host['address'],'port':host['port'],'platform':'linux','ssh_path':'direct_executor','credential_id':h['credential_id'],'username':'root'},key='removal-connection-'+key,expected=202)
                connection=api.wait('/enterprise/connection-tests/'+connection['id'],lambda x:x['status']=='succeeded',timeout=90,label='Removal SSH preflight')
                preview=api.call('POST','/enterprise/host-removals/actions/preview',{'target_type':'managed_host','target_id':hid,'mode':'uninstall','expected_version':host['resource_version'],'credential_id':h['credential_id'],'connection_test_id':connection['id']},key='remove-'+key,expected=201)
                result=api.confirm(preview);stage['removal_id']=result['execution']['operation_ref']['id']
            write_json(output,state)
        if not stage.get('uninstalled'):
            def removed(value):
                if value['status'] in ('failed','cleanup_unknown'):raise RuntimeError('Removal failed: '+str(value.get('error_code')))
                return value['status']=='succeeded' and value['local_cleanup']=='verified'
            operation=api.wait('/enterprise/host-removal-operations/'+stage['removal_id'],removed,timeout=300,label='Verified software removal')
            write_json(run.out/('cleanup-'+key+'-verified.json'),operation)
            assert run.shell(key,'systemctl show -p LoadState --value argus-connector.service argus-otelcol.service\n').split()==['not-found','not-found']
            stage['uninstalled']=True;save('software-uninstalled-'+key)
        if not stage.get('deleted'):
            host=api.call('GET','/enterprise/hosts/'+hid,expected=[200,404])
            if not host.get('code'):
                assert host['status']=='uninstalled'
                preview=api.call('POST','/enterprise/hosts/'+hid+'/actions/preview-delete',{'expected_version':host['resource_version']},key='delete-'+key,expected=201)
                api.confirm(preview)
            stage['deleted']=True;save('host-retired-'+key)
        credentials=api.call('GET','/enterprise/credentials')['items'];credential=next(x for x in credentials if x['id']==h['credential_id'])
        if credential['status']=='active':api.call('PUT','/enterprise/credentials/'+credential['id'],{'status':'disabled','expected_version':credential['version']})
        secret=api.call('GET','/enterprise/secrets/'+h['secret_id'])
        if secret['status']=='active':api.call('DELETE','/enterprise/secrets/'+secret['id']+'?expected_version='+str(secret['version']),expected=204)
    role=mapping['evaluator']['role_id'];path='/enterprise/data-authorizations/role/'+role
    for kind,ids in [('host',owned_hosts),('monitoring_target',{t['target_id'] for t in mapping.get('targets',{}).values()})]:
        grants=api.call('GET',path+'?resource_type='+kind)
        revoke=[g['resource_id'] for g in grants['items'] if g['direct'] and g['resource_id'] in ids]
        if revoke:api.call('POST',path,{'resource_type':kind,'resource_ids':revoke,'remove':True,'expected_version':grants['authorization_version']},expected=204)
    save('batch-evaluator-grants-revoked')
    if 'compose-destroyed' not in state['steps']:
        for c in mapping['containers']:assert run.container(c['service'])['Id']==c['id']
        volume_names=execute(['docker','volume','ls','-q','--filter','label=io.argus.plan9.run='+run.id]).split()
        for name in volume_names:
            volume=json.loads(execute(['docker','volume','inspect',name]))[0];assert volume['Labels']['io.argus.plan9.run']==run.id
        run.compose('down','--volumes',timeout=120);save('compose-destroyed')
    for kind in ['container','volume','network']:assert not execute(['docker',kind,'ls','-q','--filter','label=io.argus.plan9.run='+run.id]).strip()
    rows=api.call('GET','/monitoring/collections?limit=100')['items']
    old_ids={t['collection_id'] for t in mapping.get('targets',{}).values()}
    assert not any(c['id'] in old_ids or c['target']['id'] in owned_hosts for c in rows)
    state.update(status='destroyed',finished_at=now(),owned_runtime_residuals=0,current_collections_remaining=0,historical_target_identities_retained=True)
    save('cleanup-complete');return state
