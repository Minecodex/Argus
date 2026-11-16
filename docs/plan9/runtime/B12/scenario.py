"""Real alert evidence, native Redis CSV, and bounded model-repair fault window."""
import argparse
import csv
from datetime import datetime,timedelta,timezone
import io
import json
import time
from integration import Batch
from manage import Run,now

SERVICE='plan9-b12.service'
FAILED='max by (argus_resource_id) (systemd_unit_state{resource_systemd_unit_name="'+SERVICE+'",systemd_unit_active_state="failed"})'

def configure(batch):
    from B11c.monitoring import evaluator
    evaluator(batch)
    name='B12 cache orders service failed';api=batch.api
    policy=next((row for row in api.call('GET','/alerts/configurations?kind=policy') if row['name']==name),None)
    if policy is None:
        spec={'description':'Actual cache HTTP systemd failure; Redis stays healthy; no outbound notification',
            'enabled':True,'resources':{'mode':'explicit','types':['host'],'ids':[batch.host],'matchers':[]},
            'rules':[{'name':name,'expression':FAILED,'query_mode':'numeric','operator':'>','threshold':0.5,'recovery_threshold':0,'severity':'warning','unit':''}],
            'mode':'independent','match_by':['argus_resource_id'],'interval_seconds':10,'for_seconds':20,'recovery_seconds':20,
            'no_data_behavior':'report','error_behavior':'keep','channel_ids':[],'route_ids':[],
            'labels':{'plan9_run':batch.run.id,'plan9_batch':'b12'}}
        trial=api.call('POST','/alerts/trial',spec);assert trial['health']=='normal'
        batch.run.save('alert-baseline',{'spec':spec,'trial':trial})
        api.confirm(api.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'create','name':name,'spec':spec},key='service-policy',expected=202))
        policy=next(row for row in api.call('GET','/alerts/configurations?kind=policy') if row['name']==name)
    batch.map['alert_policies']['service']={'id':policy['id'],'name':name,'expression':FAILED};batch.save()
    if not batch.map.get('conversation'):
        model=batch.map['model']
        chat=api.call('POST','/conversations',{'title':'B12 真实主机、缓存与 Workspace 闭环验收','selected_model_id':model['id']},key='actual-model-conversation',expected=201)
        if chat.get('mode')!='execute' or chat.get('access_mode')!='request_approval':
            chat=api.call('PUT','/conversations/'+chat['id'],{'mode':'execute','access_mode':'request_approval','expected_version':chat['version']})
        batch.map['conversation']={'id':chat['id'],'model_id':model['id'],'title':chat['title']};batch.save()
    print('Owned real service alert and normal selected-model conversation configured',flush=True)

def dataset(batch):
    path=batch.run.out/'native-redis-observations.csv'
    if path.exists():return
    rows=[]
    for _ in range(8):
        native=json.loads(batch.run.shell('direct','python3 /usr/local/lib/plan9-b12/service.py native\n'));stats=native['stats']
        rows.append({'observed_at':native['at'],'used_memory_bytes':int(stats['used_memory']),'total_commands_processed':int(stats['total_commands_processed']),
            'keyspace_hits':int(stats['keyspace_hits']),'keyspace_misses':int(stats['keyspace_misses'])});time.sleep(2)
    out=io.StringIO();writer=csv.DictWriter(out,fieldnames=list(rows[0]));writer.writeheader();writer.writerows(rows);path.write_text(out.getvalue(),encoding='utf-8')
    batch.run.save('native-dataset-evidence',{'rows':rows,'source':'Actual authenticated Redis INFO snapshots; includes monitoring commands',
        'expected':{'command_delta':rows[-1]['total_commands_processed']-rows[0]['total_commands_processed'],
            'peak_memory_bytes':max(row['used_memory_bytes'] for row in rows)},'fabricated_telemetry':False})
    print('Eight actual Redis snapshots exported to the upload CSV; no Argus table writes',flush=True)

def fault(batch):
    run=batch.run;guard='plan9-b12-model-recover-'+str(int(time.time()))
    recover='systemctl reset-failed '+SERVICE+' || true; systemctl start '+SERVICE
    original_pid=run.shell('direct','systemctl show '+SERVICE+' -p MainPID --value\n');assert int(original_pid)>1
    run.shell('direct',"systemd-run --collect --unit="+guard+" --on-active=600s /bin/sh -ec '"+recover+"'\n"
        'systemctl kill --kill-whom=main --signal=SIGKILL '+SERVICE+'\n'
        'for i in 1 2 3 4 5; do systemctl is-failed --quiet '+SERVICE+' && exit 0; sleep 1; done; exit 1\n')
    event={'started_at':now(),'guard':guard,'guard_seconds':600,'owned_host_id':batch.host,'service':SERVICE,'recover_script':recover,
        'injection':'SIGKILL of the owned HTTP service main process; Redis/Collector unchanged','original_pid':int(original_pid),
        'guard_deadline':(datetime.now(timezone.utc)+timedelta(seconds=600)).isoformat()}
    run.save('model-repair-fault',event)
    rows=batch.api.wait('/alerts/instances?policy_id='+batch.map['alert_policies']['service']['id'],
        lambda page:any(row['lifecycle']=='firing' and row['state']['kind']=='condition' for row in page['items']),timeout=100,label='Actual B12 service alert')
    event['firing_alert']=next(row for row in rows['items'] if row['lifecycle']=='firing' and row['state']['kind']=='condition');run.save('model-repair-fault',event)
    print('Actual B12 process exit produced a firing alert; 600-second model-repair recovery guard armed',flush=True)

def recover(batch):
    run=batch.run;file=run.out/'model-repair-fault.json'
    if not file.exists():return
    fault=json.loads(file.read_text())
    run.shell('direct','systemctl stop '+fault['guard']+'.timer || true\n'+fault['recover_script']+'\n')
    print('Owned service and fault guard restored safely',flush=True)

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['configure','dataset','fault','recover']);args=parser.parse_args()
    run=Run();api=run.api();api.login();globals()[args.action](Batch(run,api))
