"""Observe both persistent delivery reports without clearing failure counters."""
import json
import time
from integration import Batch
from manage import Run,now


run=Run();api=run.api();api.login();batch=Batch(run,api)
def snapshot():
    return {key:json.loads(run.shell(key,'cat /var/lib/argus-otelcol/identity/delivery-state.json\n'))['report'] for key in ['bastion','member']}
started=now();baseline=snapshot();observations=[baseline]
for index in range(4):
    time.sleep(30);current=snapshot();observations.append(current)
    for key in baseline:
        assert current[key]['instance_id']==baseline[key]['instance_id'] and current[key]['sequence']>baseline[key]['sequence']
        for field in ['terminal_failures','enqueue_failures','receiver_refusals']:
            assert current[key][field]==baseline[key][field]
            assert all(value==0 for value in current[key][field].values())
        rows=api.call('GET','/monitoring/builtins?target_type=host&target_id='+batch.map['hosts'][key]['host_id'])
        enabled=[row['collection'] for row in rows if (row.get('collection') or {}).get('desired_enabled')]
        assert len(enabled)==4 and all(row['health']=='healthy' and row['freshness']=='fresh' for row in enabled)
    print('Both real delivery paths healthy, zero loss/refusal/enqueue failures: '+str(index+1)+'/4',flush=True)
run.save('delivery-watch',{'passed':True,'started_at':started,'ended_at':now(),'watch_seconds':120,
    'observations':observations,'terminal_failures':0,'enqueue_failures':0,'receiver_refusals':0,'counters_reset':False})
