"""Five component scrape alerts with explicit B09b resource grants."""
import json

from run import KEYS, write_json
from verification import query

NAMES = ('API Server', 'CoreDNS', 'Scheduler', 'Controller Manager', 'etcd')


def configure(run, api):
    shared=json.loads((run.out.parent/'common-evaluator.json').read_text(encoding='utf-8'))
    path='/enterprise/data-authorizations/role/'+shared['role_id']
    current=api.call('GET',path+'?resource_type=kubernetes_cluster')
    if not any(row['direct'] and row['resource_id']==run.cluster for row in current['items']):
        api.call('POST',path,{'resource_type':'kubernetes_cluster','resource_ids':[run.cluster],
            'remove':False,'expected_version':current['authorization_version']},expected=204)
    policies=api.call('GET','/alerts/configurations?kind=policy')
    records={}
    mapping=json.loads(run.infra.resources.read_text(encoding='utf-8'))
    mapping['evaluator']=shared
    write_json(run.infra.resources,mapping)
    for key,label in zip(KEYS,NAMES):
        collection=next(row['collection'] for row in run.builtins(api) if row['definition']['key']==key)
        expression='min by (argus_resource_id) (up{argus_collection_id="'+collection['id']+'"})'
        observed=query(run,api,expression)['data']['result']
        assert len(observed)==1 and float(observed[0]['value'][1])==1,key
        name='B09b '+label+' metrics scrape unavailable'
        row=next((row for row in policies if row['name']==name),None)
        if row is None:
            spec={'description':'Real B09b '+label+' native scrape availability','enabled':True,
                'resources':{'mode':'explicit','types':['kubernetes_cluster'],'ids':[run.cluster],'matchers':[]},
                'rules':[{'name':name,'expression':expression,'query_mode':'numeric','operator':'<',
                    'threshold':.5,'recovery_threshold':1,'severity':'warning','unit':''}],
                'mode':'independent','match_by':['argus_resource_id'],'interval_seconds':10,'for_seconds':20,
                'recovery_seconds':20,'no_data_behavior':'report','error_behavior':'keep',
                'channel_ids':[],'route_ids':[],'labels':{'plan9_run':run.id,'plan9_batch':'b09b'}}
            trial=api.call('POST','/alerts/trial',spec)
            assert trial['health']=='normal',trial
            run.save(key+'-alert-baseline',{'query':observed,'spec':spec,'trial':trial})
            api.confirm(api.call('POST','/alerts/configurations/preview',{
                'kind':'policy','operation':'create','name':name,'spec':spec},
                key='component-alert-'+key,expected=202))
            row=next(row for row in api.call('GET','/alerts/configurations?kind=policy') if row['name']==name)
        assert row['spec']['resources']['ids']==[run.cluster]
        if row['spec']['rules'][0]['expression']!=expression:
            import copy
            spec=copy.deepcopy(row['spec'])
            spec['rules'][0]['expression']=expression
            trial=api.call('POST','/alerts/trial',spec)
            assert trial['health']=='normal'
            api.confirm(api.call('POST','/alerts/configurations/preview',{
                'kind':'policy','operation':'update','id':row['id'],'name':name,
                'expected_version':row['version'],'spec':spec},
                key='component-resource-label-'+row['id']+'-'+str(row['version']),expected=202))
            row=api.call('GET','/alerts/configurations/'+row['id'])
        records[key]={'id':row['id'],'name':name,'expression':expression}
        mapping['alert_policies'][key]=records[key]
        write_json(run.infra.resources,mapping)
        run.save('alert-policies',records)
    run.save('alert-policies',records)
    run.save('alert-policy',records['kubernetes_scheduler_metrics'])
    mapping=json.loads(run.infra.resources.read_text(encoding='utf-8'))
    mapping['alert_policies']=records
    mapping['evaluator']=shared
    write_json(run.infra.resources,mapping)
    run.mapping=mapping
    print('Five native component scrape alerts configured',flush=True)
