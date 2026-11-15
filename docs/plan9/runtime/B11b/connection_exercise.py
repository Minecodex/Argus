"""Bounded member-control interruption with independent Bastion/data evidence."""
from datetime import datetime,timezone
import json
import time
from integration import Batch
from manage import Run,now


def native_delivery(run,key):
    return json.loads(run.shell(key,'cat /var/lib/argus-otelcol/identity/delivery-state.json\n'))['report']


def exercise(batch):
    run=batch.run;api=batch.api
    if (run.out/'connection-recovery.json').exists():return
    root=batch.map['hosts']['bastion']['host_id'];member=batch.map['hosts']['member']['host_id']
    endpoint='/monitoring/builtins?target_type=host&target_id='
    before={key:native_delivery(run,key) for key in ['bastion','member']}
    started=now();guard='plan9-b11b-connector-recover-'+str(int(time.time()));interruption=None
    try:
        run.shell('member','systemd-run --collect --unit='+guard+' --on-active=110s /bin/systemctl start argus-connector.service\nsystemctl stop argus-connector.service\n')
        offline=api.wait('/enterprise/hosts/'+member,lambda row:row['connection_status'] in ['offline','degraded'],timeout=100,label='Member control interruption')
        bastion=api.call('GET','/enterprise/hosts/'+root);assert bastion['connection_status']=='online'
        root_sources=api.wait(endpoint+root,lambda rows:any((row.get('collection') or {}).get('last_sample_at','')>started
            for row in rows),timeout=35,label='Independent Bastion samples during member control interruption')
        member_sources=api.call('GET',endpoint+member)
        root_stamps=[row['collection']['last_sample_at'] for row in root_sources if (row.get('collection') or {}).get('desired_enabled')]
        assert root_stamps and max(root_stamps)>started
        interruption={'observed_at':now(),'bastion':bastion,'member':offline,
            'bastion_collections':root_sources,'member_collections':member_sources,
            'member_native_metrics':run.shell('member',"curl -fsS http://127.0.0.1:8888/metrics | grep -E '^otelcol_exporter_queue_size'\n"),
            'bastion_online_does_not_imply_member_control_online':True,
            'member_samples_continued':any((row.get('collection') or {}).get('last_sample_at','')>started for row in member_sources)}
        run.save('connection-interruption',interruption)
        print('Actual member control offline while Bastion remains online; source timestamps and queue captured',flush=True)
    finally:run.shell('member','systemctl stop '+guard+'.timer || true\nsystemctl start argus-connector.service\n')
    online=api.wait('/enterprise/hosts/'+member,lambda row:row['connection_status']=='online',timeout=180,label='Member relay reconnection')
    resumed=now()
    sources=api.wait(endpoint+member,lambda rows:all(row['collection']['health']=='healthy' and row['collection']['freshness']=='fresh'
        and row['collection']['last_sample_at']>resumed for row in rows if (row.get('collection') or {}).get('desired_enabled')),
        timeout=150,label='Actual member telemetry resumed')
    after={key:native_delivery(run,key) for key in ['bastion','member']}
    for key in before:
        for metric in ['terminal_failures','receiver_refusals','enqueue_failures']:assert after[key][metric]==before[key][metric]
    run.save('connection-recovery',{'started_at':started,'offline_observed_at':interruption['observed_at'],
        'reconnected_at':online.get('updated_at'),'data_verified_at':now(),'sources':sources,
        'before_delivery':before,'after_delivery':after,'no_new_delivery_loss':True,
        'guard_seconds':110,'bastion_stayed_online':True,'control_path':'bastion_relay','telemetry_transport':'bastion_tunnel'})
    print('Actual member relay and telemetry resumed with unchanged permanent loss counters',flush=True)


if __name__=='__main__':
    run=Run();api=run.api();api.login();exercise(Batch(run,api))
