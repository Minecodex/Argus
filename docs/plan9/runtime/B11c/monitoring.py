"""Current-batch resource dashboards, evaluator scope and real fault diagnostics."""
from datetime import datetime,timedelta,timezone
import json
import time
from integration import Batch
from manage import Run,ROOT,now

SERVICE='plan9-b11c.service'
FAILED='max by (argus_resource_id) (systemd_unit_state{resource_systemd_unit_name="'+SERVICE+'",systemd_unit_active_state="failed"})'
BUDGET={'max_scan_bytes':268435456,'max_rows':5000,'max_samples':1000000,'max_series':10000,'timeout_ms':10000,'max_result_bytes':8388608}

class HostView:
    def __init__(self,batch,key):self.batch=batch;self.key=key;self.api=batch.api;self.map=batch.map;self.run=self
    @property
    def host(self):return self.map['hosts'][self.key]['host_id']
    def save(self,*args):
        if args:self.batch.run.save(self.key+'-'+args[0],args[1])
        else:self.batch.save()

def dashboards(batch):
    from B11a.monitoring import dashboards as managed
    for key in ['bastion','member']:managed(HostView(batch,key))
    print('Eight current resource-scoped dashboard views executed',flush=True)

def evaluator(batch):
    previous=json.loads((batch.run.out.parent/'p9-b11b-20261114-01/resource-map.json').read_text(encoding='utf-8'))['evaluator']
    roles=batch.api.call('GET','/enterprise/roles')
    roles=roles['items'] if isinstance(roles,dict) else roles
    role=next(row for row in roles if row['id']==previous['role_id'])
    assert role['status']=='active'
    accounts=batch.api.call('GET','/enterprise/service-accounts')['items']
    account=next(row for row in accounts if row['id']==previous['service_account_id']);assert account['status']=='active'
    batch.map['evaluator']={'role_id':role['id'],'service_account_id':account['id'],'name':role['name'],'borrowed_account_role':True};batch.save()
    path='/enterprise/data-authorizations/role/'+role['id'];current=batch.api.call('GET',path+'?resource_type=host')
    ids=[row['host_id'] for row in batch.map['hosts'].values()]
    missing=[hid for hid in ids if not any(row['resource_id']==hid and row['direct'] for row in current['items'])]
    if missing:batch.api.call('POST',path,{'resource_type':'host','resource_ids':missing,'remove':False,'expected_version':current['authorization_version']},expected=204);batch.api.login()
    settings=batch.api.call('GET','/alerts/configurations?kind=settings')
    setting=next((row for row in settings if row['spec'].get('evaluator_id')==account['id']),None)
    if setting is None:
        assert not settings,'An unrelated evaluator configuration must not be overwritten'
        batch.api.confirm(batch.api.call('POST','/alerts/configurations/preview',{'kind':'settings','operation':'create',
            'name':'B11c background evaluator','spec':{'evaluator_id':account['id']}},key='evaluator-settings',expected=202))
        setting=next(row for row in batch.api.call('GET','/alerts/configurations?kind=settings') if row['spec']['evaluator_id']==account['id'])
    batch.map['evaluator']['settings_id']=setting['id'];batch.save()
    batch.run.save('evaluator-scope',{'role_id':role['id'],'added_host_ids':missing,'only_owned_current_hosts':True,'role_permissions_unchanged':True})
    print('Existing evaluator account role authorized only for the two current owned hosts',flush=True)

def query(batch,expression,ids=None):
    end=datetime.now(timezone.utc);ids=ids or [row['host_id'] for row in batch.map['hosts'].values()]
    return batch.api.call('POST','/enterprise/metrics/query',{'scope':{'mode':'explicit','resources':[{'type':'host','id':hid} for hid in ids]},
        'time_range':{'from':(end-timedelta(minutes=15)).isoformat(),'to':end.isoformat()},'budget':BUDGET,'query':expression})

def configure_alerts(batch):
    evaluator(batch)
    for key,name,expression,host_key in [
        ('service','B11c private service failed',FAILED,'member'),
        ('freshness','B11c tunnel telemetry freshness','max by (argus_resource_id) (last_over_time(otelcol_process_uptime[30s]))','bastion')]:
        rows=batch.api.call('GET','/alerts/configurations?kind=policy')
        policy=next((row for row in rows if row['name']==name),None)
        if policy is None:
            spec={'description':'Real B11c service/tunnel diagnosis; no outbound notification targets','enabled':True,
                  'resources':{'mode':'explicit','types':['host'],'ids':[batch.map['hosts'][host_key]['host_id']],'matchers':[]},
                  'rules':[{'name':name,'expression':expression,'query_mode':'numeric','operator':'>','threshold':0.5 if key=='service' else 1e20,
                            'recovery_threshold':0,'severity':'warning','unit':''}],
                  'mode':'independent','match_by':['argus_resource_id'],'interval_seconds':10,'for_seconds':20,'recovery_seconds':20,
                  'no_data_behavior':'report','error_behavior':'keep','channel_ids':[],'route_ids':[],
                  'labels':{'plan9_run':batch.run.id,'plan9_batch':'b11c'}}
            trial=batch.api.call('POST','/alerts/trial',spec);assert trial['health']=='normal'
            batch.run.save(key+'-alert-trial',{'spec':spec,'trial':trial})
            batch.api.confirm(batch.api.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'create','name':name,'spec':spec},key=key+'-policy',expected=202))
            policy=next(row for row in batch.api.call('GET','/alerts/configurations?kind=policy') if row['name']==name)
        batch.map['alert_policies'][key]={'id':policy['id'],'name':name,'expression':expression};batch.save()
    print('Real service and short-window data-health policies configured; no external notifications',flush=True)

def exercise_alert(batch):
    from lifecycle import delivery,no_loss
    run=batch.run;api=batch.api
    if (run.out/'alert-cycle.json').exists():return
    member=batch.map['hosts']['member']['host_id'];root=batch.map['hosts']['bastion']['host_id'];policy=batch.map['alert_policies']['service']['id']
    started=now();guard='plan9-b11c-service-recover-'+str(int(time.time()));firing=None;before=delivery(run)
    recover='rm -f /var/lib/plan9/b11c/fail-start; systemctl reset-failed '+SERVICE+' || true; systemctl start '+SERVICE
    try:
        run.shell('member',"systemd-run --collect --unit="+guard+" --on-active=120s /bin/sh -ec '"+recover+"'\n"
            'systemctl stop '+SERVICE+'\ntouch /var/lib/plan9/b11c/fail-start\nsystemctl start '+SERVICE+' || true\n'
            'for i in 1 2 3 4 5; do systemctl is-failed --quiet '+SERVICE+' && exit 0; sleep 1; done\nexit 1\n')
        report=api.wait('/alerts/instances?policy_id='+policy,lambda page:any(row['lifecycle']=='firing' and row['state']['kind']=='condition'
            and row['started_at']>=started[:19] for row in page['items']),timeout=100,label='Real member systemd failure alert')
        firing=next(row for row in report['items'] if row['lifecycle']=='firing' and row['state']['kind']=='condition');run.save('alert-firing',firing)
        values=query(batch,FAILED,[root,member])['data']['result'];actual={row['metric']['argus_resource_id']:float(row['value'][1]) for row in values}
        assert actual=={root:0,member:1},actual
        run.save('leaf-identity-during-fault',{'values':values,'gateway_did_not_overwrite_leaf_id':True,'member_id':member,'bastion_id':root})
        print('Real member service failure fired; separate Bastion identity remains healthy',flush=True)
    finally:run.shell('member','systemctl stop '+guard+'.timer || true\n'+recover+'\n')
    history=api.wait('/alerts/history?policy_id='+policy,lambda page:any(row['id']==firing['id'] and row['lifecycle']=='resolved' for row in page['items']),timeout=110,label='Member service recovery')
    event=next(row for row in history['items'] if row['id']==firing['id']);assert float(query(batch,FAILED,[member])['data']['result'][0]['value'][1])==0
    after=delivery(run);no_loss(before,after)
    run.save('alert-cycle',{'id':event['id'],'policy_id':policy,'started_at':event['started_at'],'resolved_at':event['resolved_at'],
        'lifecycle':'resolved','native_fault_started':started,'guard_seconds':120,'no_new_delivery_loss':True})
    print('Actual member service recovered and native alert resolved',flush=True)

def exercise_data_pause(batch):
    from lifecycle import delivery,no_loss,fresh
    run=batch.run;api=batch.api
    if (run.out/'data-pause-cycle.json').exists():return
    root=batch.map['hosts']['bastion']['host_id'];policy=batch.map['alert_policies']['freshness']['id']
    started=now();guard='plan9-b11c-data-recover-'+str(int(time.time()));event=None;before=delivery(run)
    try:
        run.shell('bastion','systemd-run --collect --unit='+guard+' --on-active=120s /bin/systemctl start argus-otelcol.service\nsystemctl stop argus-otelcol.service\n')
        rows=api.wait('/alerts/instances?policy_id='+policy,lambda page:any(row['state']['kind']=='health' and row['health']=='no_data' and row['lifecycle']=='firing' for row in page['items']),timeout=105,label='Actual root gateway pause no_data diagnosis')
        event=next(row for row in rows['items'] if row['state']['kind']=='health' and row['health']=='no_data' and row['lifecycle']=='firing')
        hosts={key:api.call('GET','/enterprise/hosts/'+value['host_id']) for key,value in batch.map['hosts'].items()}
        assert all(row['connection_status']=='online' for row in hosts.values())
        run.save('data-pause-firing',event)
        run.save('data-vs-control-diagnosis',{'hosts':hosts,'health':'no_data','observed_at':now(),
            'native_root_collector':run.shell('bastion','systemctl show argus-otelcol.service -p ActiveState -p SubState\n'),
            'both_controls_online':True})
        print('Actual telemetry gateway paused and no_data fired while both controls stay online',flush=True)
    finally:run.shell('bastion','systemctl stop '+guard+'.timer || true\nsystemctl start argus-otelcol.service\n')
    rows=api.wait('/alerts/history?policy_id='+policy,lambda page:any(row['id']==event['id'] and row['lifecycle']=='resolved' for row in page['items']),timeout=120,label='Telemetry data-health recovery')
    event=next(row for row in rows['items'] if row['id']==event['id']);sources=fresh(batch,now());after=delivery(run);no_loss(before,after)
    run.save('data-pause-cycle',{'id':event['id'],'name':event['state']['name'],'policy_id':policy,'started_at':event['started_at'],
        'resolved_at':event['resolved_at'],'lifecycle':'resolved','native_pause_started':started,'no_new_delivery_loss':True,
        'guard_seconds':120,'fresh_sources':sources,'delivery_before':before,'delivery_after':after})
    print('Both data paths fresh again; no_data resolved without new permanent loss',flush=True)

if __name__=='__main__':
    import argparse
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['dashboards','evaluator','configure_alerts','exercise_alert','exercise_data_pause']);args=parser.parse_args()
    run=Run();api=run.api();api.login();globals()[args.action](Batch(run,api))
