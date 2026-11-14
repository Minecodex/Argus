"""Observe the owned native gateway Collector for a full retry horizon; never reset counters."""
from datetime import datetime,timezone
import json,time
from run import Run
r=Run();a=r.api();started=time.monotonic()
d=r.get('deployment','argus-otelcol-gateway','-n','argus-telemetry')
selector=','.join(k+'='+v for k,v in d['spec']['selector']['matchLabels'].items())
pod=next(p for p in r.get('pods','-n','argus-telemetry','-l',selector)['items'] if not p['metadata'].get('deletionTimestamp'))
volume=next(v for v in pod['spec']['volumes'] if v['name']=='identity-state')
pvc=r.get('pvc',volume['persistentVolumeClaim']['claimName'],'-n','argus-telemetry');pv=r.get('pv',pvc['spec']['volumeName'])
path=pv['spec']['hostPath']['path']+'/delivery-state.json'
def ledger():
 from infrastructure import execute
 # kubectl ownership guard is checked on every sample before the native file read.
 r.kubectl('get','pod',pod['metadata']['name'],'-n','argus-telemetry','-o','name')
 value=json.loads(execute(['docker','exec',r.id+'-control-plane','cat',path]))
 return {k:value.get(k) for k in ('report','preserved_shutdown_failures','process_id')}
baseline=ledger();r.save('delivery-baseline',baseline);observations=[]
first=a.call('GET','/monitoring/builtins?target_type=kubernetes_cluster&target_id='+r.cluster_id)
first_samples={x['definition']['key']:x['collection']['last_sample_at'] for x in first if x.get('collection',{}).get('desired_enabled')}
while True:
 state=ledger()
 for key in ('terminal_failures','enqueue_failures','receiver_refusals'):
  assert state['report'][key]==baseline['report'][key],(key,state)
 current=a.call('GET','/alerts/instances?limit=100')
 assert not any(x['lifecycle']=='normal' and x['health']=='no_data' and not x.get('started_at') for x in current['items'])
 own=[x for x in current['items'] if r.cluster_id in x.get('resource_ids',[])]
 assert all(x['lifecycle']!='normal' for x in own),'Untriggered normal row appeared in current alerts'
 observations.append({'at':datetime.now(timezone.utc).isoformat(),'ledger':state,'owned_current_alerts':len(own)})
 r.save('delivery-observations',observations)
 if time.monotonic()-started>=305:break
 time.sleep(10)
assert state['report']['sequence']>baseline['report']['sequence']
rows=a.call('GET','/monitoring/builtins?target_type=kubernetes_cluster&target_id='+r.cluster_id)
enabled=[x for x in rows if x.get('collection',{}).get('desired_enabled')]
assert len(enabled)==3 and all(x['collection']['health']=='healthy' and x['collection']['freshness']=='fresh' and x['collection']['last_sample_at']>first_samples[x['definition']['key']] for x in enabled)
logs=r.kubectl('logs','-n','argus-telemetry','deployment/argus-otelcol-gateway','--since=6m',timeout=20)
(r.out/'collector-final-logs.txt').write_text(logs,encoding='utf-8')
terminal=[line for line in logs.splitlines() if 'Dropping data' in line or 'Rejecting data' in line]
assert not terminal,'Actual terminal exporter rejection logged'
r.save('delivery-watch',{'passed':True,'watch_seconds':time.monotonic()-started,'baseline':baseline,'final':state,'terminal_failure_delta':0,'enqueue_failure_delta':0,'receiver_refusal_delta':0,'healthy_advancing_collections':3,'owned_current_alerts':len(own),'historical_delivery_event_ids':[x['id'] for x in own if x['state']['kind']=='collector_delivery_gap'],'historical_failure_counters_preserved':True,'untriggered_no_data_current_rows':0,'native_terminal_log_check_passed':True,'recorded_at':datetime.now(timezone.utc).isoformat()})
print('Native delivery and advancing real samples passed the complete retry horizon',flush=True)
