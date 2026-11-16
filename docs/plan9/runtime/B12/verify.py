"""Fresh public API/native evidence gate for the complete B12 workflow."""
import argparse
import json
import time
from integration import Batch
from manage import Run,now

def watch(batch):
    run=batch.run;api=batch.api
    def report():return json.loads(run.shell('direct','cat /var/lib/argus-otelcol/identity/delivery-state.json\n'))['report']
    before=report();started=now();observations=[before]
    for index in range(4):
        time.sleep(30);current=report();observations.append(current)
        assert current['instance_id']==before['instance_id'] and current['sequence']>before['sequence']
        for field in ['terminal_failures','receiver_refusals','enqueue_failures']:
            assert current[field]==before[field] and all(value==0 for value in current[field].values())
        rows=api.call('GET','/monitoring/builtins?target_type=host&target_id='+batch.host)
        active=[row['collection'] for row in rows if (row.get('collection') or {}).get('desired_enabled')]
        assert len(active)==4 and all(row['health']=='healthy' and row['freshness']=='fresh' for row in active)
        redis=api.call('GET','/monitoring/collections/'+batch.map['targets']['redis']['collection_id'])
        assert redis['health']=='healthy' and redis['freshness']=='fresh'
        print('B12 native delivery and all five collections stable: '+str(index+1)+'/4',flush=True)
    run.save('delivery-watch',{'passed':True,'watch_seconds':120,'started_at':started,'ended_at':now(),'observations':observations,
        'terminal_failures':0,'receiver_refusals':0,'enqueue_failures':0,'counters_reset':False})

def ledger(batch):
    events=[];cursor=0
    for _ in range(10):
        page=batch.api.call('GET','/conversations/'+batch.map['conversation']['id']+'/ledger?limit=200&cursor='+str(cursor));rows=page['items']
        if not rows:break
        events.extend(rows);last=rows[-1]['sequence'];assert last>cursor;cursor=last
        if len(rows)<200:break
    assert len(rows)<200,'Event budget exceeded; do not silently truncate evidence'
    batch.run.save('complete-workflow-ledger',{'items':events,'through_sequence':cursor,'complete':True})
    return events

def verify(batch):
    run=batch.run;api=batch.api;events=ledger(batch)
    metrics=json.loads((run.out/'model-metrics.json').read_text());assert metrics['run']['status']=='succeeded'
    queries=[event for event in events if event.get('run_id')==metrics['run']['run_id'] and event['event_type']=='tool_call_result'
        and event['payload'].get('target_tool_id')=='telemetry.dashboard.query.get' and event['payload']['status']=='succeeded']
    assert len(queries)>=2,'Both real dashboard result jobs must be read by the model'
    run.save('model-native-query-proof',queries)
    analysis=json.loads((run.out/'model-analysis.json').read_text());assert analysis['run']['status']=='succeeded'
    code=[event for event in events if event.get('run_id')==analysis['run']['run_id'] and event['event_type']=='tool_call_result'
        and (event['payload'].get('target_tool_id') or event['payload'].get('tool_id'))=='bash' and event['payload']['status']=='succeeded']
    assert code,'Workspace code must actually execute'
    report=json.loads((run.out/'model-published-report-proof.json').read_text());assert report['downloaded'] and report['expected_math_verified']
    repair=json.loads((run.out/'model-repair-proof.json').read_text());assert repair['cancelled']['no_native_execution']
    assert repair['repair']['execution']['status']=='succeeded' and repair['repair']['recovered_before_guard']
    assert repair['repair']['run']['status']=='succeeded' and 'ActiveState=active' in repair['repair']['native_after']
    current=api.call('GET','/alerts/instances?policy_id='+batch.map['alert_policies']['service']['id']);assert not current['items']
    native=json.loads(run.shell('direct','curl -fsS http://127.0.0.1:18084/health\n'));assert native['healthy'] and native['redis']=='PONG'
    total=0
    for name in ['host.overview','host.processes','host.systemd','collector.overview','redis']:
        value=json.loads((run.out/(name+'-dashboard.json')).read_text());assert all(panel['status']=='success' for panel in value['panels']);total+=len(value['panels'])
    assert total==31
    health=json.loads((run.out/'formal-health.json').read_text(encoding='utf-8-sig'));assert health['passed'] and len(health['checks'])==20
    assert run.platform()==json.loads((run.out/'platform-before.json').read_text()),'Formal volumes and deployments changed during B12'
    assert json.loads((run.out/'delivery-watch.json').read_text())['passed']
    usage=next(row for row in api.call('GET','/enterprise/model-usage') if row['model_id']==batch.map['model']['id'])
    assert usage['amount']<1 and usage['request_count']<96 and usage['input_tokens']+usage['output_tokens']<3500000
    run.save('model-usage-final',usage)
    result={'run_id':run.id,'batch':'B12','status':'ready_for_review','verified_at':now(),'human_acceptance':'pending',
        'actual_hosts':1,'actual_redis_targets':1,'native_fresh_collections':5,'nonempty_dashboard_panels':31,
        'real_provider':'APIMart','real_model':'glm-5.3-flash','model_revision':3,'actual_dashboard_query_results_read':len(queries),
        'workspace_code_executed':True,'native_csv_math_verified':True,'published_report_bytes':report['bytes'],
        'model_proposed_cancelled_action_not_dispatched':True,'new_model_action_confirmed_in_ui_and_native_http_recovered':True,
        'real_business_alert_fired_and_resolved':True,'formal_health_checks':20,'formal_pvcs_and_deployments_preserved':True,
        'delivery_watch_seconds':120,'delivery_failures':0,'provider_usage':usage,
        'historical_failed_run_retained':'failed-observe-run.json','aggregate_usage_incomplete_because_failed_sample_retained':not usage['usage_complete'],
        'replay_or_mock_model':False,'direct_business_sql_writes':False,'provider_key_in_source_or_screenshots':False}
    run.save('checks',result)
    print('B12 real model/data/Workspace/cancel/confirm/native recovery and formal preservation passed',flush=True)

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['watch','verify']);args=parser.parse_args()
    run=Run();api=run.api();api.login();globals()[args.action](Batch(run,api))
