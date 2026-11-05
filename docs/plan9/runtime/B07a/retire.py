"""Retire batch-owned application/key/dashboard before the shared host cleanup."""
import argparse
import json
from pathlib import Path
import sys
from manage import Run,RUNS,execute,write_json,now
from integration import ApplicationBatch,Api
sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'common'))
from retirement import retire

def retire_application(run,api):
    p=ApplicationBatch(run,api);app=p.map['applications']['orders'];file=run.out/'application-cleanup.json'
    endpoint_states = [app]
    if 'aux_ingest' in app:
        endpoint_states.append(app['aux_ingest'])
    state=json.loads(file.read_text(encoding='utf-8')) if file.exists() else {'started_at':now(),'steps':[]}
    def saved(step):
        state['steps'].append(step);write_json(file,state);run.event('application_retirement_step',step=step)
    if 'clients-stopped' not in state['steps']:
        run.fault('errors',True);run.shell('collector','systemctl stop plan9-load\nsystemctl stop plan9-app@orders-gateway plan9-app@orders-worker\n');saved('clients-stopped')
    if 'policies-retired' not in state['steps']:
        for policy in p.map['alert_policies'].values():
            row=api.call('GET','/alerts/configurations/'+policy['id'],expected=[200,404])
            if not row.get('code'):api.confirm(api.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'delete','id':row['id'],'expected_version':row['version']},key='retire-app-policy-'+row['id'],expected=202))
        saved('policies-retired')
    if 'endpoint-stopped' not in state['steps']:
        for endpoint in endpoint_states:
            p.converge()
            row=next(e for e in api.call('GET','/applications/'+app['id']+'/ingest-endpoints')['items'] if e['id']==endpoint['endpoint_id'])
            if row['enabled']:p.commit({'kind':'endpoint','operation':'set_enabled','id':row['id'],'expected_version':row['version'],'endpoint':{'enabled':False}},'stop-sdk-endpoint')
        p.converge()
        saved('endpoint-stopped')
    if 'key-revoked' not in state['steps']:
        for endpoint in endpoint_states:
            row=next(k for k in api.call('GET','/monitoring/ingest-endpoints/'+endpoint['endpoint_id']+'/keys')['items'] if k['id']==endpoint['key_id'])
            if not row.get('revoked_at'):p.commit({'kind':'ingest_key','operation':'revoke','id':row['id'],'expected_version':row['version'],'ingest_key':{'endpoint_id':endpoint['endpoint_id']}},'revoke-sdk-key')
        saved('key-revoked')
    if 'redis-unbound' not in state['steps']:
        # This batch creates one immutable binding and never updates it. The
        # original creation receipt supplies its optimistic-concurrency version.
        receipts=[json.loads(line) for line in (run.out/'timeline.jsonl').read_text(encoding='utf-8').splitlines()]
        receipt=next(x['resource_ref'] for x in receipts if (x.get('resource_ref') or {}).get('resource_id')==app['redis_binding_id'])
        binding={'application_id':app['id'],'target':{'kind':'monitoring_target','id':p.map['targets']['redis']['target_id']}}
        p.commit({'kind':'binding','operation':'delete','id':app['redis_binding_id'],'expected_version':receipt['version'],'binding':binding},'unbind-redis');saved('redis-unbound')
    if 'dashboard-archived' not in state['steps']:
        board=api.call('GET','/dashboards/'+p.map['batch_dashboard']['id'])['dashboard']
        if board['lifecycle']=='active':api.confirm(api.call('POST','/dashboards/'+board['id']+'/actions/preview',{'id':board['id'],'operation':'archive','expected_version':board['version'],'name':board['name'],'description':board.get('description',''),'sort_order':board.get('sort_order',0)},key='archive-batch-dashboard',expected=201))
        saved('dashboard-archived')
    if 'application-archived' not in state['steps']:
        row=api.call('GET','/applications/'+app['id'])
        if row['status']=='active':
            application={field:row[field] for field in ('name','environment','description','owner_name') if field in row}
            p.commit({'kind':'application','operation':'archive','id':row['id'],'expected_version':row['version'],'application':application},'archive-batch-app')
        saved('application-archived')
    if 'application-grant-revoked' not in state['steps']:
        path='/enterprise/data-authorizations/role/'+p.map['evaluator']['role_id'];grants=api.call('GET',path+'?resource_type=application')
        if any(g['direct'] and g['resource_id']==app['id'] for g in grants['items']):api.call('POST',path,{'resource_type':'application','resource_ids':[app['id']],'remove':True,'expected_version':grants['authorization_version']},expected=204)
        saved('application-grant-revoked')
    state.update(finished_at=now(),status='retired',history_preserved=True);write_json(file,state)

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('--session',required=True);parser.add_argument('--ca',required=True);parser.add_argument('--batch',choices=RUNS,default='B07a');args=parser.parse_args();run=Run(batch=args.batch);api=Api(run,args.session,args.ca)
    retired=run.out/'cleanup.json'
    if not retired.exists() or json.loads(retired.read_text(encoding='utf-8'))['status']!='destroyed':retire_application(run,api)
    retire(run,api,execute,write_json,now)
