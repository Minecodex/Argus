"""Actual replica loss, container restart, Pod replacement and native log continuity."""
from datetime import datetime
import json
import time
from urllib.parse import urlencode
import uuid

from integration import scope
from manage import APP_NS,RUN,execute,now,write_json

AVAILABLE='max by (argus_resource_id) (k8s_deployment_available{resource_k8s_namespace_name="'+APP_NS+'",resource_k8s_deployment_name="orders"})'


def configure_alert(batch):
    role=json.loads((batch.run.out.parent/'common-evaluator.json').read_text(encoding='utf-8'))
    batch.map['evaluator']=role
    batch.save()
    reader=next(x for x in batch.api.call('GET','/enterprise/roles')['items'] if x['id']==role['role_id'])
    if 'kubernetes.read' not in reader['permissions']:
        batch.api.call('PUT','/enterprise/roles/'+reader['id'],{
            'name':reader['name'],'description':reader.get('description',''),
            'permissions':reader['permissions']+['kubernetes.read'],'expected_version':reader['version']})
    path='/enterprise/data-authorizations/role/'+role['role_id']
    grants=batch.api.call('GET',path+'?resource_type=kubernetes_cluster')
    if not any(x['direct'] and x['resource_id']==batch.cluster for x in grants['items']):
        batch.api.call('POST',path,{'resource_type':'kubernetes_cluster','resource_ids':[batch.cluster],
            'remove':False,'expected_version':grants['authorization_version']},expected=204)
    name='B09a Orders available replicas below 2'
    rows=batch.api.call('GET','/alerts/configurations?kind=policy')
    row=next((r for r in rows if r['name']==name),None)
    if row is None:
        value=batch.query(AVAILABLE)['data']['result']
        assert len(value)==1 and float(value[0]['value'][1])==2,value
        spec={'description':'Controlled B09a deployment replica reduction','enabled':True,
            'resources':{'mode':'explicit','types':['kubernetes_cluster'],'ids':[batch.cluster],'matchers':[]},
            'rules':[{'name':name,'expression':AVAILABLE,'query_mode':'numeric','operator':'<',
                'threshold':1.5,'recovery_threshold':2,'severity':'warning','unit':''}],
            'mode':'independent','match_by':['argus_resource_id'],'interval_seconds':10,
            'for_seconds':20,'recovery_seconds':20,'no_data_behavior':'report','error_behavior':'keep',
            'channel_ids':[],'route_ids':[],'labels':{'plan9_run':RUN,'plan9_batch':'b09a'}}
        trial=batch.api.call('POST','/alerts/trial',spec)
        write_json(batch.run.out/'alert-baseline.json',{'value':value,'spec':spec,'trial':trial})
        assert trial['health']=='normal',trial
        batch.api.confirm(batch.api.call('POST','/alerts/configurations/preview',{
            'kind':'policy','operation':'create','name':name,'spec':spec},key='replicas-policy',expected=202))
        row=next(r for r in batch.api.call('GET','/alerts/configurations?kind=policy') if r['name']==name)
    batch.map['alert_policies']['replicas']={'id':row['id'],'name':name,'expression':AVAILABLE}
    batch.save()


def exercise_alert(batch):
    run=batch.run
    path=run.out/'alert-cycle.json'
    if path.exists():
        return
    policy=batch.map['alert_policies']['replicas']['id']
    before=now()
    recovery=['kubectl','--kubeconfig=/etc/kubernetes/admin.conf','-n',APP_NS,'scale','deployment/orders','--replicas=2']
    node=RUN+'-control-plane'
    try:
        execute(['docker','exec',node,'systemd-run','--unit=plan9-b09a-recover','--on-active=120s',*recovery])
        run.kubectl('scale','deployment/orders','-n',APP_NS,'--replicas=1')
        run.kubectl('rollout','status','deployment/orders','-n',APP_NS,'--timeout=45s')
        fired=batch.api.wait('/alerts/instances?policy_id='+policy,lambda r:any(
            row['lifecycle']=='firing' and row['state']['kind']=='condition' for row in r['items']),
            timeout=100,label='Replica loss alert')
        event=next(row for row in fired['items'] if row['lifecycle']=='firing' and row['state']['kind']=='condition')
        assert datetime.fromisoformat(event['started_at'].replace('Z','+00:00'))>=datetime.fromisoformat(before)
        assert float(batch.query(AVAILABLE)['data']['result'][0]['value'][1])==1
        write_json(run.out/'alert-firing.json',event)
        receipt=run.request('one-replica-error-'+uuid.uuid4().hex[:8])
        batch.wait_log(receipt[0]['message'],2)
        write_json(run.out/'one-replica-http.json',receipt)
    finally:
        run.kubectl('scale','deployment/orders','-n',APP_NS,'--replicas=2')
        run.kubectl('rollout','status','deployment/orders','-n',APP_NS,'--timeout=90s')
        execute(['docker','exec',node,'systemctl','stop','plan9-b09a-recover.timer'])
    resolved=batch.api.wait('/alerts/history?policy_id='+policy,lambda r:any(
        row['id']==event['id'] and row['lifecycle']=='resolved' for row in r['items']),timeout=100,
        label='Replica alert recovery')
    recovered=next(row for row in resolved['items'] if row['id']==event['id'])
    assert float(batch.query(AVAILABLE)['data']['result'][0]['value'][1])==2
    write_json(path,{key:recovered[key] for key in ('id','started_at','resolved_at','lifecycle')})
    print('Replica alert fired at one ready replica and recovered at two',flush=True)


def pod_receipt(run,pod,label):
    script='import json;from urllib.request import urlopen;print(urlopen("http://127.0.0.1:8080/orders?label='+label+'").read().decode())'
    return json.loads(run.kubectl('exec','-n',APP_NS,pod,'--','python3','-c',script))


def live_logs(batch,pod,previous=False):
    base='/enterprise/kubernetes-clusters/'+batch.cluster+'/log-sessions'
    created=batch.api.call('POST',base,{'namespace_id':batch.namespace,'pod':pod['metadata']['name'],
        'pod_uid':pod['metadata']['uid'],'container':'http','previous':previous,'tail_lines':100,'since_seconds':600},
        key='live-logs-'+uuid.uuid4().hex,expected=201)
    session=created['session']
    try:
        active=batch.api.call('POST',base+'/'+session['id']+'/attach',{'ticket':created['ticket']},
                              key='attach-'+session['id'])
        assert active['status']=='active'
        result=batch.api.call('POST',base+'/'+session['id']+'/read',{})
        assert result['content'] and not result['truncated'],result
        result.pop('next_cursor',None)
        return {'session':session,'batch':result}
    finally:
        batch.api.call('POST',base+'/'+session['id']+'/close',key='close-'+session['id'])


def restart_and_replace(batch):
    run=batch.run
    done=run.out/'pod-lifecycle.json'
    if done.exists():
        return
    pods=run.get('pods','-n',APP_NS,'-l','app=orders')['items']
    assert len(pods)==2
    pod=pods[0]
    name=pod['metadata']['name']
    before=pod_receipt(run,name,'before-restart-'+uuid.uuid4().hex[:8])
    batch.wait_log(before['message'])
    current=live_logs(batch,pod)
    assert before['message'] in current['batch']['content']
    count=pod['status']['containerStatuses'][0]['restartCount']
    container=pod['status']['containerStatuses'][0]['containerID'].split('://',1)[1]
    runtime=json.loads(execute(['docker','exec',RUN+'-control-plane','crictl','inspect',container]))
    assert runtime['status']['labels']['io.kubernetes.pod.uid']==pod['metadata']['uid']
    assert runtime['status']['metadata']['name']=='http'
    process=int(runtime['info']['pid'])
    assert process>1
    execute(['docker','exec',RUN+'-control-plane','kill','-KILL',str(process)])
    deadline=time.monotonic()+90
    while True:
        changed=run.get('pod',name,'-n',APP_NS)
        states=changed.get('status',{}).get('containerStatuses',[])
        if states and states[0]['restartCount']>count and states[0]['ready']:
            break
        if time.monotonic()>deadline:
            raise RuntimeError('Actual container restart failed to recover')
        time.sleep(2)
    assert changed['metadata']['uid']==pod['metadata']['uid']
    after=pod_receipt(run,name,'after-restart-'+uuid.uuid4().hex[:8])
    batch.wait_log(after['message'])
    previous=live_logs(batch,changed,True)
    assert before['message'] in previous['batch']['content']
    run.kubectl('delete','pod',name,'-n',APP_NS,'--wait=true')
    run.kubectl('rollout','status','deployment/orders','-n',APP_NS,'--timeout=90s')
    newer=run.get('pods','-n',APP_NS,'-l','app=orders')['items']
    replacement=next(row for row in newer if row['metadata']['uid'] not in {x['metadata']['uid'] for x in pods})
    final=pod_receipt(run,replacement['metadata']['name'],'after-replacement-'+uuid.uuid4().hex[:8])
    batch.wait_log(final['message'])
    assert batch.query_log(before['message'])['data']
    batch.inventory()
    proof={'previous_pod':{'name':name,'uid':pod['metadata']['uid'],'restart_count_before':count,
                          'restart_count_after':changed['status']['containerStatuses'][0]['restartCount']},
        'replacement':{'name':replacement['metadata']['name'],'uid':replacement['metadata']['uid']},
        'http_receipts':[before,after,final],'current_log_session':current,'previous_log_session':previous,
        'persistent_rule_followed_deployment_uid':batch.map['log_rules']['orders']['deployment_uid'],
        'old_persistent_logs_retained':True}
    write_json(done,proof)
    print('Real container restart, previous logs, Pod replacement and persistent logs verified',flush=True)
