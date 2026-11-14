"""Preserve the setup loss and verify subsequent direct delivery without resets."""
import json
import time
from integration import Batch
from manage import Run,now


def snapshot(run):
    return json.loads(run.shell('direct','cat /var/lib/argus-otelcol/identity/delivery-state.json\n'))['report']


run=Run();api=run.api();api.login();batch=Batch(run,api)
start=snapshot(run);observations=[start];began=now()
for index in range(4):
    time.sleep(30)
    row=snapshot(run);observations.append(row)
    assert row['instance_id']==start['instance_id'] and row['sequence']>start['sequence']
    for field in ['terminal_failures','receiver_refusals','enqueue_failures']:
        assert row[field]==start[field],(field,start[field],row[field])
    definitions=api.call('GET','/monitoring/builtins?target_type=host&target_id='+batch.host)
    assert all((x.get('collection') or {}).get('freshness')=='fresh' for x in definitions
        if (x.get('collection') or {}).get('desired_enabled'))
    print('Direct delivery observation '+str(index+1)+'/4: no new losses',flush=True)
run.save('delivery-watch',{'passed':True,'started_at':began,'ended_at':now(),'watch_seconds':120,
    'observations':observations,'new_terminal_failures':0,'new_enqueue_failures':0,'new_receiver_refusals':0,
    'historical_metric_points_preserved':start['terminal_failures']['metric_points'],
    'historical_gap_handled':False,'last_failure_at':start.get('last_failure_at')})
