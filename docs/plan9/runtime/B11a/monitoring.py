"""Actual host dashboards, telemetry and bounded systemd alert exercise."""
from datetime import datetime,timedelta,timezone
import json
import time
from integration import Batch
from manage import Run,ROOT,now

SERVICE='plan9-b11a.service'
FAILED='max by (argus_resource_id) (systemd_unit_state{resource_systemd_unit_name="'+SERVICE+'",systemd_unit_active_state="failed"})'


def scope(batch):
    return {'resource_mode':'explicit','resource_types':['host'],'resource_ids':[batch.host]}


def metric(batch,expression):
    end=datetime.now(timezone.utc)
    return batch.api.call('POST','/enterprise/metrics/query',{'resource_ids':[batch.host],
        'time_range':{'from':(end-timedelta(minutes=15)).isoformat(),'to':end.isoformat()},
        'budget':{'max_scan_bytes':268435456,'max_rows':5000,'max_samples':1000000,
                  'max_series':10000,'timeout_ms':10000,'max_result_bytes':8388608},'query':expression})


def dashboards(batch):
    boards=batch.api.call('GET','/dashboards');report={}
    for key in ['host.overview','host.processes','host.systemd','collector.overview']:
        board=next(row for row in boards if row.get('managed_template_key')==key)
        path='/hosts/'+batch.host+'/dashboard-bindings'
        binding=batch.api.call('GET',path)
        if not any(item['dashboard']['id']==board['id'] for item in binding['items']):
            batch.api.confirm(batch.api.call('POST',path+'/preview',{'operation':'attach','dashboard_id':board['id'],
                'expected_dashboard_version':board['version'],'expected_resource_version':binding['resource_version']},key='bind-'+key,expected=201))
        batch.map['dashboards'][key]={'id':board['id'],'name':board['name']};batch.save()
        for attempt in range(12):
            end=datetime.now(timezone.utc)
            value=batch.api.call('POST','/dashboards/'+board['id']+'/execute',{
                **scope(batch),'from':(end-timedelta(hours=1)).isoformat(),'to':end.isoformat()})
            value.pop('context_token',None)
            batch.run.save(key+'-dashboard',value)
            if all(panel['status']=='success' for panel in value['panels']):break
            time.sleep(5)
        report[key]=[{'id':p['id'],'status':p['status']} for p in value['panels']]
        print(key+': '+str(report[key]),flush=True)
    batch.run.save('dashboard-checks',report)
    assert all(p['status']=='success' for panels in report.values() for p in panels),report


def configure_alert(batch):
    evaluator=json.loads((ROOT/'artifacts/plan9/common-evaluator.json').read_text(encoding='utf-8'))
    batch.map['evaluator']=evaluator;batch.save()
    path='/enterprise/data-authorizations/role/'+evaluator['role_id']
    grants=batch.api.call('GET',path+'?resource_type=host')
    if not any(row['resource_id']==batch.host and row['direct'] for row in grants['items']):
        batch.api.call('POST',path,{'resource_type':'host','resource_ids':[batch.host],'remove':False,
            'expected_version':grants['authorization_version']},expected=204)
    for _ in range(25):
        sample=metric(batch,FAILED)['data']['result']
        if len(sample)==1 and float(sample[0]['value'][1])==0:break
        time.sleep(3)
    else:raise RuntimeError('Real healthy systemd sample missing')
    name='B11a direct host service failed'
    row=next((p for p in batch.api.call('GET','/alerts/configurations?kind=policy') if p['name']==name),None)
    if row is None:
        spec={'description':'Actual controlled systemd service startup failure on the B11a direct SSH host',
            'enabled':True,'resources':{'mode':'explicit','types':['host'],'ids':[batch.host],'matchers':[]},
            'rules':[{'name':name,'expression':FAILED,'query_mode':'numeric','operator':'>','threshold':.5,
                      'recovery_threshold':0,'severity':'warning','unit':''}],
            'mode':'independent','match_by':['argus_resource_id'],'interval_seconds':10,'for_seconds':20,
            'recovery_seconds':20,'no_data_behavior':'report','error_behavior':'keep','channel_ids':[],'route_ids':[],
            'labels':{'plan9_batch':'b11a','plan9_run':batch.run.id}}
        trial=batch.api.call('POST','/alerts/trial',spec);assert trial['health']=='normal',trial
        batch.run.save('alert-baseline',{'trial':trial,'metric':sample,'spec':spec})
        batch.api.confirm(batch.api.call('POST','/alerts/configurations/preview',{
            'kind':'policy','operation':'create','name':name,'spec':spec},key='b11a-service-policy',expected=202))
        row=next(p for p in batch.api.call('GET','/alerts/configurations?kind=policy') if p['name']==name)
    batch.map['alert_policies']['service']={'id':row['id'],'name':name,'expression':FAILED};batch.save()
    print('Real systemd alert configured without outbound notifications',flush=True)


def exercise_alert(batch):
    if (batch.run.out/'alert-cycle.json').exists():return
    run=batch.run;policy=batch.map['alert_policies']['service']['id'];started=now();firing=None
    guard='plan9-b11a-auto-recover-'+str(int(time.time()))
    recover='rm -f /var/lib/plan9/b11a/fail-start; systemctl reset-failed '+SERVICE+' || true; systemctl start '+SERVICE
    try:
        run.shell('direct',"systemd-run --collect --unit="+guard+" --on-active=120s /bin/sh -ec '"+recover+"'\n"
            'systemctl stop '+SERVICE+'\ntouch /var/lib/plan9/b11a/fail-start\nsystemctl start '+SERVICE+' || true\n'
            'for i in 1 2 3 4 5; do systemctl is-failed --quiet '+SERVICE+' && exit 0; sleep 1; done\nexit 1\n')
        report=batch.api.wait('/alerts/instances?policy_id='+policy,lambda x:any(
            e['lifecycle']=='firing' and e['state']['kind']=='condition' and e['started_at']>=started[:19] for e in x['items']),
            timeout=100,label='Actual B11a service-failure alert')
        firing=next(e for e in report['items'] if e['lifecycle']=='firing' and e['state']['kind']=='condition')
        assert float(metric(batch,FAILED)['data']['result'][0]['value'][1])==1
        run.save('alert-firing',firing)
        print('Actual failed service produced a firing alert',flush=True)
    finally:
        run.shell('direct','systemctl stop '+guard+'.timer || true\n'+recover+'\n')
    report=batch.api.wait('/alerts/history?policy_id='+policy,lambda x:any(
        e['id']==firing['id'] and e['lifecycle']=='resolved' for e in x['items']),timeout=100,label='Real service recovery')
    event=next(e for e in report['items'] if e['id']==firing['id'])
    assert float(metric(batch,FAILED)['data']['result'][0]['value'][1])==0
    run.save('alert-cycle',{'id':event['id'],'policy_id':policy,'started_at':event['started_at'],
        'resolved_at':event['resolved_at'],'lifecycle':event['lifecycle'],'threshold':.5,'failed_value':1,'recovered_value':0})
    print('Actual service and alert recovered',flush=True)


if __name__=='__main__':
    import argparse
    p=argparse.ArgumentParser();p.add_argument('action',choices=['dashboards','configure_alert','exercise_alert']);args=p.parse_args()
    run=Run();api=run.api();api.login();globals()[args.action](Batch(run,api))
