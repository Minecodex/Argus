"""Final B09a observations through native Kubernetes and normal product APIs."""
import argparse
from datetime import datetime,timezone
import json
import re

from integration import Api,Batch
from manage import Run,APP_NS,RUN,execute,now,write_json
from scenario import AVAILABLE


def verify(batch):
    run,api=batch.run,batch.api
    collector=batch.converge()
    assert collector['desired_revision']==collector['effective_revision']
    reports={}
    observed=datetime.now(timezone.utc)
    for key,state in batch.map['builtins'].items():
        row=api.call('GET','/monitoring/collections/'+state['id'])
        assert row['health']=='healthy' and row['freshness']=='fresh' and row['apply_status']=='effective',row
        age=(observed-datetime.fromisoformat(row['last_sample_at'].replace('Z','+00:00'))).total_seconds()
        assert 0<=age<90,(key,age)
        reports[key]={k:row[k] for k in ('id','name','last_sample_at','health','freshness')}
    assert len(reports)==3
    available=batch.query(AVAILABLE)['data']['result']
    assert len(available)==1 and float(available[0]['value'][1])==2
    pods=run.get('pods','-n',APP_NS,'-l','app=orders')['items']
    assert len(pods)==2 and all(p['status']['containerStatuses'][0]['ready'] for p in pods)
    namespace=run.get('namespace',APP_NS)
    assert namespace['metadata']['uid']==batch.map['namespaces'][APP_NS]['uid']
    claim=run.get('pvc','data-ledger-0','-n',APP_NS)
    assert claim['status']['phase']=='Bound'
    receipt=run.request('final-error-'+now()[11:19].replace(':',''))
    logs=batch.wait_log(receipt[0]['message'],2)
    write_json(run.out/'final-http-and-logs.json',{'receipts':receipt,'logs':logs})
    board=json.loads((run.out/'dashboard-checks.json').read_text(encoding='utf-8'))
    assert all(panel['status']=='success' for rows in board.values() for panel in rows)
    policy=batch.map['alert_policies']['replicas']['id']
    condition=json.loads((run.out/'alert-cycle.json').read_text(encoding='utf-8'))
    assert api.call('GET','/alerts/configurations/'+policy)['health']=='normal'
    history=api.call('GET','/alerts/history?policy_id='+policy)
    assert any(row['id']==condition['id'] and row['lifecycle']=='resolved' for row in history['items'])
    active=[{key:item.get(key) for key in ('id','name','policy_id','health','lifecycle','state')}
        for item in api.call('GET','/alerts/instances?limit=100')['items']
        if set(item.get('resource_ids',[]))&{batch.cluster,batch.map['application']['id']}]
    lifecycle=json.loads((run.out/'pod-lifecycle.json').read_text(encoding='utf-8'))
    assert lifecycle['previous_pod']['uid']!=lifecycle['replacement']['uid']
    assert lifecycle['previous_pod']['restart_count_after']>lifecycle['previous_pod']['restart_count_before']
    assert lifecycle['old_persistent_logs_retained']
    browser=json.loads((run.out/'browser-review.json').read_text(encoding='utf-8'))
    assert browser['exit_code']==0
    text=(run.out/'browser-review.log').read_text(encoding='utf-8')
    assert re.search(r'^\s*4 passed\b',text,re.M),text[-1000:]
    initial=json.loads((run.out/'platform-before.json').read_text(encoding='utf-8'))
    final=run.platform()
    assert final['pvc']==initial['pvc']
    changed=[name for name,row in initial['deployments'].items() if row!=final['deployments'][name]]
    assert set(changed)=={'argus-direct-executor','argus-worker'},changed
    assert initial['deployments']['argus-worker']['images']==final['deployments']['argus-worker']['images']
    endpoint=batch.map['telemetry_network_endpoint']
    current=json.loads(execute(['kubectl','--context','docker-desktop','get','svc',endpoint['name'],
        '-n',endpoint['namespace'],'-o','json']))
    assert current['metadata']['uid']==endpoint['uid'] and current['metadata']['labels']['argus.plan9/run']==RUN
    write_json(run.out/'platform-after.json',final)
    proof={'run_id':RUN,'status':'ready_for_review','observed_at':now(),
        'predecessor_retired':True,'real_cluster_nodes':1,'actual_namespace_uid':namespace['metadata']['uid'],
        'fresh_native_collections':reports,'available_orders_replicas':2,'bound_application_pvc':True,
        'replica_alert':condition,'active_batch_alerts':len(active),'active_alert_items':active,
        'container_restart_and_pod_replacement_verified':True,'previous_and_persisted_logs_verified':True,
        'dashboard_panels_passed':sum(len(rows) for key,rows in board.items() if key!='namespace'),
        'namespace_panel_executions_passed':len(board['namespace']),'browser_cases_passed':4,
        'platform_components_changed':changed,'original_pvcs_preserved':len(initial['pvc']),
        'human_acceptance':'pending'}
    write_json(run.out/'checks.json',proof)
    state=json.loads((run.out/'run.json').read_text(encoding='utf-8'))
    state.update(status='ready_for_review',ready_at=now(),human_acceptance='pending',
        review_entry='docs/plan9/batches/B09a-review.md')
    write_json(run.out/'run.json',state)
    print(json.dumps(proof,ensure_ascii=False,indent=2))


if __name__=='__main__':
    p=argparse.ArgumentParser()
    p.add_argument('--session',required=True)
    p.add_argument('--ca',required=True)
    args=p.parse_args()
    run=Run()
    verify(Batch(run,Api(run,args.session,args.ca)))
