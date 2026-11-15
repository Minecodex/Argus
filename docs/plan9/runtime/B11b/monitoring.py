"""Real Bastion/member dashboards, leaf identity and service fault/recovery."""
import argparse
from datetime import datetime,timedelta,timezone
import json
import time
from integration import Batch
from manage import Run,ROOT,now

SERVICE='plan9-b11b.service'
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
    print('Four managed dashboards bound to both actual hosts; eight resource-scoped views executed',flush=True)


def metric(batch,ids=None):
    ids=ids or [batch.map['hosts']['member']['host_id']];end=datetime.now(timezone.utc)
    return batch.api.call('POST','/enterprise/metrics/query',{'scope':{'mode':'explicit','resources':[{'type':'host','id':ident} for ident in ids]},
        'time_range':{'from':(end-timedelta(minutes=15)).isoformat(),'to':end.isoformat()},'budget':BUDGET,'query':FAILED})


def configure_alert(batch):
    evaluator=json.loads((ROOT/'artifacts/plan9/common-evaluator.json').read_text(encoding='utf-8'));batch.map['evaluator']=evaluator;batch.save()
    path='/enterprise/data-authorizations/role/'+evaluator['role_id'];grants=batch.api.call('GET',path+'?resource_type=host')
    ids=[state['host_id'] for state in batch.map['hosts'].values()]
    missing=[ident for ident in ids if not any(row['resource_id']==ident and row['direct'] for row in grants['items'])]
    if missing:
        batch.api.call('POST',path,{'resource_type':'host','resource_ids':missing,'remove':False,'expected_version':grants['authorization_version']},expected=204)
        batch.api.login()
    host=batch.map['hosts']['member']['host_id'];name='B11b private member service failed'
    row=next((r for r in batch.api.call('GET','/alerts/configurations?kind=policy') if r['name']==name),None)
    if row is None:
        sample=metric(batch)['data']['result'];assert len(sample)==1 and float(sample[0]['value'][1])==0
        spec={'description':'Actual systemd failure on an isolated member reached through its Bastion',
            'enabled':True,'resources':{'mode':'explicit','types':['host'],'ids':[host],'matchers':[]},
            'rules':[{'name':name,'expression':FAILED,'query_mode':'numeric','operator':'>','threshold':.5,'recovery_threshold':0,'severity':'warning','unit':''}],
            'mode':'independent','match_by':['argus_resource_id'],'interval_seconds':10,'for_seconds':20,'recovery_seconds':20,
            'no_data_behavior':'report','error_behavior':'keep','channel_ids':[],'route_ids':[],
            'labels':{'plan9_batch':'b11b','plan9_run':batch.run.id}}
        trial=batch.api.call('POST','/alerts/trial',spec);assert trial['health']=='normal';batch.run.save('alert-baseline',{'spec':spec,'trial':trial,'sample':sample})
        batch.api.confirm(batch.api.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'create','name':name,'spec':spec},key='member-service-policy',expected=202))
        row=next(r for r in batch.api.call('GET','/alerts/configurations?kind=policy') if r['name']==name)
    batch.map['alert_policies']['service']={'id':row['id'],'name':name,'expression':FAILED};batch.save()
    print('Private-member real service alert configured without outbound notifications',flush=True)


def exercise_alert(batch):
    run=batch.run
    if (run.out/'alert-cycle.json').exists():return
    member=batch.map['hosts']['member']['host_id'];root=batch.map['hosts']['bastion']['host_id'];policy=batch.map['alert_policies']['service']['id']
    started=now();guard='plan9-b11b-auto-recover-'+str(int(time.time()));firing=None
    recover='rm -f /var/lib/plan9/b11b/fail-start; systemctl reset-failed '+SERVICE+' || true; systemctl start '+SERVICE
    try:
        run.shell('member',"systemd-run --collect --unit="+guard+" --on-active=120s /bin/sh -ec '"+recover+"'\n"
            'systemctl stop '+SERVICE+'\ntouch /var/lib/plan9/b11b/fail-start\nsystemctl start '+SERVICE+' || true\n'
            'for i in 1 2 3 4 5; do systemctl is-failed --quiet '+SERVICE+' && exit 0; sleep 1; done\nexit 1\n')
        report=batch.api.wait('/alerts/instances?policy_id='+policy,lambda value:any(row['lifecycle']=='firing' and row['state']['kind']=='condition'
            and row['started_at']>=started[:19] for row in value['items']),timeout=100,label='Private member real fault alert')
        firing=next(row for row in report['items'] if row['lifecycle']=='firing' and row['state']['kind']=='condition');run.save('alert-firing',firing)
        values=metric(batch,[root,member])['data']['result'];actual={row['metric']['argus_resource_id']:float(row['value'][1]) for row in values}
        assert actual=={root:0,member:1},actual
        run.save('leaf-identity-during-fault',{'values':values,'gateway_did_not_overwrite_leaf_id':True,'member_id':member,'bastion_id':root})
        print('Member fault triggered its own alert; Bastion remains healthy with a distinct native resource identity',flush=True)
    finally:run.shell('member','systemctl stop '+guard+'.timer || true\n'+recover+'\n')
    history=batch.api.wait('/alerts/history?policy_id='+policy,lambda value:any(row['id']==firing['id'] and row['lifecycle']=='resolved' for row in value['items']),timeout=100,label='Private member service recovery')
    event=next(row for row in history['items'] if row['id']==firing['id']);assert float(metric(batch)['data']['result'][0]['value'][1])==0
    run.save('alert-cycle',{'id':event['id'],'policy_id':policy,'started_at':event['started_at'],'resolved_at':event['resolved_at'],'lifecycle':'resolved'})
    print('Actual private service and alert recovered',flush=True)


def configure_data_alert(batch):
    name='B11b private member telemetry freshness'
    expression=FAILED.replace('systemd_unit_state{','last_over_time(systemd_unit_state{').replace('"failed"})','"failed"}[30s]))')
    row=next((r for r in batch.api.call('GET','/alerts/configurations?kind=policy') if r['name']==name),None)
    if row is None:
        spec=json.loads((batch.run.out/'alert-baseline.json').read_text())['spec']
        spec['description']='Thirty-second real sample window; report missing telemetry separately from host connection state'
        spec['rules'][0].update(name=name,expression=expression)
        trial=batch.api.call('POST','/alerts/trial',spec);assert trial['health']=='normal',trial
        batch.run.save('telemetry-alert-baseline',{'spec':spec,'trial':trial})
        batch.api.confirm(batch.api.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'create','name':name,'spec':spec},key='member-data-policy',expected=202))
        row=next(r for r in batch.api.call('GET','/alerts/configurations?kind=policy') if r['name']==name)
    batch.map['alert_policies']['telemetry']={'id':row['id'],'name':name,'expression':expression};batch.save()


def exercise_data_pause(batch):
    run=batch.run;api=batch.api
    if (run.out/'data-pause-cycle.json').exists():return
    member=batch.map['hosts']['member']['host_id'];root=batch.map['hosts']['bastion']['host_id'];policy=batch.map['alert_policies']['telemetry']['id']
    started=now();guard='plan9-b11b-data-recover-'+str(int(time.time()));event=None
    before=json.loads(run.shell('member','cat /var/lib/argus-otelcol/identity/delivery-state.json\n'))['report']
    try:
        run.shell('member','systemd-run --collect --unit='+guard+' --on-active=110s /bin/systemctl start argus-otelcol.service\nsystemctl stop argus-otelcol.service\n')
        rows=api.wait('/alerts/instances?policy_id='+policy,lambda page:any(row['state']['kind']=='health' and row['health']=='no_data' and row['lifecycle']=='firing' for row in page['items']),timeout=95,label='Actual missing-member telemetry diagnosis')
        event=next(row for row in rows['items'] if row['state']['kind']=='health' and row['health']=='no_data' and row['lifecycle']=='firing')
        assert api.call('GET','/enterprise/hosts/'+member)['connection_status']=='online'
        assert api.call('GET','/enterprise/hosts/'+root)['connection_status']=='online'
        run.save('data-pause-firing',event)
        run.save('data-vs-control-diagnosis',{'member_control_online':True,'bastion_control_online':True,
            'native_member_collector':run.shell('member','systemctl show argus-otelcol.service -p ActiveState -p SubState\n'),
            'member_collections':api.call('GET','/monitoring/builtins?target_type=host&target_id='+member),
            'bastion_collections':api.call('GET','/monitoring/builtins?target_type=host&target_id='+root),
            'health':'no_data','observed_at':now()})
        print('Both host controls online while actual member collection pause fired no_data diagnosis',flush=True)
    finally:run.shell('member','systemctl stop '+guard+'.timer || true\nsystemctl start argus-otelcol.service\n')
    rows=api.wait('/alerts/history?policy_id='+policy,lambda page:any(row['id']==event['id'] and row['lifecycle']=='resolved' for row in page['items']),timeout=110,label='Member data-health recovery')
    recovered=next(row for row in rows['items'] if row['id']==event['id'])
    after=json.loads(run.shell('member','cat /var/lib/argus-otelcol/identity/delivery-state.json\n'))['report']
    for field in ['terminal_failures','enqueue_failures','receiver_refusals']:assert before[field]==after[field]
    run.save('data-pause-cycle',{'id':event['id'],'name':event['state']['name'],'policy_id':policy,'started_at':event['started_at'],
        'resolved_at':recovered['resolved_at'],'lifecycle':'resolved','native_pause_started':started,'no_new_delivery_loss':True})
    print('Actual samples resumed and no_data diagnosis recovered without new delivery loss',flush=True)


if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['dashboards','configure_alert','exercise_alert','configure_data_alert','exercise_data_pause']);args=parser.parse_args()
    run=Run();api=run.api();api.login();globals()[args.action](Batch(run,api))
