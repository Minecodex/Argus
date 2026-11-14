"""Future user-authorized retirement; preserve sessions and published releases."""
import json
from integration import Batch
from manage import Run,execute,write_json,now
from common.retirement import retire as retire_hosts


def retire(batch):
    run=batch.run;api=batch.api;remote=batch.map.get('remote',{})
    before=run.platform();owned={row['host_id'] for row in batch.map['hosts'].values()}
    sessions=api.call('GET','/enterprise/remote-access-sessions?host_id='+batch.host)['items']
    run.save('retirement-sessions',sessions)
    for row in sessions:
        assert row['host_id'] in owned
        if row['status'] in ['authorized','connecting','active','terminating']:
            api.call('POST','/enterprise/remote-access-sessions/'+row['id']+'/terminate',{'reason':'Plan9 B11a accepted and retired'})
            api.wait('/enterprise/remote-access-sessions/'+row['id'],lambda x:x['status'] not in
                ['authorized','connecting','active','terminating'],timeout=90,label='Owned terminal closure')
    if remote.get('grant_id'):
        base='/enterprise/remote-access-grants/'+remote['grant_id'];grant=api.call('GET',base)
        assert set(grant['host_ids'])==owned
        if grant['status']=='enabled':grant=api.call('POST',base+'/disable?expected_version='+str(grant['version']))
        if grant['status']!='archived':api.call('POST',base+'/archive?expected_version='+str(grant['version']))
    if remote.get('account_id'):
        account=next(row for row in api.call('GET','/enterprise/managed-accounts')['items'] if row['id']==remote['account_id'])
        assert account['host_id'] in owned
        if account['status']=='active':api.call('PUT','/enterprise/managed-accounts/'+account['id'],{
            'expected_version':account['version'],'status':'disabled'})
    result=retire_hosts(run,api,execute,write_json,now)
    endpoint=batch.map.get('telemetry_network_endpoint')
    if endpoint:
        cli=['kubectl','--context','docker-desktop','-n',endpoint['namespace']]
        current=execute([*cli,'get','service',endpoint['name'],'--ignore-not-found','-o','json'])
        if current:
            row=json.loads(current)
            assert row['metadata']['uid']==endpoint['uid'] and row['metadata']['labels']['argus.plan9/run']==run.id
            execute([*cli,'delete','service',endpoint['name'],'--wait=true'])
    assert before==run.platform()
    run.save('cleanup-verification',{'status':'destroyed','run_id':run.id,'formal_resources_preserved':True,
        'sessions_recordings_audits_preserved':True,'published_collector_release_preserved':True,
        'borrowed_base_images_and_kind_network_preserved':True,'finished_at':now()})
    return result


if __name__=='__main__':
    run=Run();api=run.api();api.login();retire(Batch(run,api))
