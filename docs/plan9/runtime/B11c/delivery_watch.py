"""Observe both persistent delivery reports without clearing failure counters."""
import json
import time
from integration import Batch
from manage import Run,now


def watch(batch):
    run=batch.run;api=batch.api
    def snapshot():
        return {key:json.loads(run.shell(key,'cat /var/lib/argus-otelcol/identity/delivery-state.json\n'))['report'] for key in ['bastion','member']}
    started=now();baseline=snapshot();observations=[baseline]
    for index in range(4):
        time.sleep(30);current=snapshot();observations.append(current)
        for key in baseline:
            assert current[key]['instance_id']==baseline[key]['instance_id'] and current[key]['sequence']>baseline[key]['sequence']
            for field in ['terminal_failures','enqueue_failures','receiver_refusals']:
                assert current[key][field]==baseline[key][field]
            rows=api.call('GET','/monitoring/builtins?target_type=host&target_id='+batch.map['hosts'][key]['host_id'])
            enabled=[row['collection'] for row in rows if (row.get('collection') or {}).get('desired_enabled')]
            assert len(enabled)==4 and all(row['health']=='healthy' and row['freshness']=='fresh' for row in enabled)
        print('Both real paths healthy; no new loss/refusal/enqueue failures, historical counters retained: '+str(index+1)+'/4',flush=True)
    result={'passed':True,'started_at':started,'ended_at':now(),'watch_seconds':120,
        'observations':observations,'new_terminal_failures':0,'new_enqueue_failures':0,'new_receiver_refusals':0,
        'absolute_delivery':observations[-1],'historical_gap_record':'delivery-maintenance-gap.json','counters_reset':False}
    run.save('delivery-watch',result)
    return result

if __name__=='__main__':
    run=Run();api=run.api();api.login();watch(Batch(run,api))
