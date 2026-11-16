"""Future authorized cleanup of B12 scene; retain user model and shared Workspace."""
import json
from integration import Batch
from manage import Run,execute,write_json,now
from common.retirement import retire
from scenario import recover

def cleanup(batch):
    run=batch.run;api=batch.api;before=run.platform()
    host=api.call('GET','/enterprise/hosts/'+batch.host);assert host['labels']['plan9_run']==run.id
    run.container('direct');run.container('redis')
    events=[];cursor=0
    for _ in range(10):
        rows=api.call('GET','/conversations/'+batch.map['conversation']['id']+'/ledger?limit=200&cursor='+str(cursor))['items']
        if not rows:break
        events.extend(rows);cursor=rows[-1]['sequence']
        if len(rows)<200:break
    runs={event['run_id'] for event in events if event.get('run_id')}
    for rid in runs:
        current=api.call('GET','/runs/'+rid)
        if current['status'] not in ['succeeded','failed','cancelled','timed_out']:
            api.call('POST','/runs/'+rid+'/cancel',key='retire-run-'+rid)
    run.save('retirement-conversation-archive',{'events':events,'at':now(),'model_and_workspace_retained':True})
    recover(batch);run.shell('direct','systemctl stop plan9-b12-traffic.service || true\n')
    retire(run,api,execute,write_json,now)
    endpoint=batch.map['telemetry_network_endpoint'];cli=['kubectl','--context','docker-desktop','-n',endpoint['namespace']]
    raw=execute([*cli,'get','service',endpoint['name'],'--ignore-not-found','-o','json'])
    if raw:
        row=json.loads(raw);assert row['metadata']['uid']==endpoint['uid'] and row['metadata']['labels']['argus.plan9/run']==run.id
        execute([*cli,'delete','service',endpoint['name'],'--wait=true'])
    assert before==run.platform()
    run.save('cleanup-verification',{'status':'destroyed','at':now(),'formal_pvcs_and_deployments_preserved':True,
        'conversation_report_billing_and_audit_retained':True,'user_model_and_shared_workspace_retained':True})

if __name__=='__main__':
    run=Run();api=run.api();api.login();cleanup(Batch(run,api))
