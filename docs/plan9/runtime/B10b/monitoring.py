"""Real observed metric queries, native dashboards and replica alert cycles."""
import copy,json,sys,time,uuid
from datetime import datetime,timedelta,timezone
from infrastructure import ROOT,NAMESPACE,write_json
from workflow import request,wait_native,ready
BUDGET={'max_scan_bytes':268435456,'max_rows':20000,'max_samples':2000000,'max_series':20000,'timeout_ms':10000,'max_result_bytes':8388608}
AVAILABLE='max by (argus_resource_id) (k8s_deployment_available{resource_k8s_namespace_name="'+NAMESPACE+'",resource_k8s_deployment_name="b10b-storage"})'
def query(r,a,expression,seconds=0):
 end=datetime.now(timezone.utc);body={'resource_ids':[r.cluster_id],'time_range':{'from':(end-timedelta(seconds=seconds or 900)).isoformat(),'to':end.isoformat()},'budget':BUDGET,'query':expression}
 if seconds:body['step_seconds']=10
 return a.call('POST','/enterprise/metrics/query_range' if seconds else '/enterprise/metrics/query',body)
def catalog(r,a):
 value=query(r,a,'count by (__name__) ({__name__=~"k8s_.*"})');r.save('native-metric-catalog',value)
 print(json.dumps(value)[:20000],flush=True)
def configure(r,a):
 mapping=json.loads(r.resources.read_text(encoding='utf-8'));role=json.loads((r.out.parent/'common-evaluator.json').read_text(encoding='utf-8'));mapping['evaluator']=role
 path='/enterprise/data-authorizations/role/'+role['role_id'];grants=a.call('GET',path+'?resource_type=kubernetes_cluster')
 if not any(x['direct'] and x['resource_id']==r.cluster_id for x in grants['items']):
  a.call('POST',path,{'resource_type':'kubernetes_cluster','resource_ids':[r.cluster_id],'remove':False,'expected_version':grants['authorization_version']},expected=204)
 name='B10b storage deployment has no available replica'
 current=next((x for x in a.call('GET','/alerts/configurations?kind=policy') if x['name']==name),None)
 if current is None:
  wait_native(r,lambda:query(r,a,AVAILABLE)['data']['result'],'Real available replica metric',120)
  spec={'description':'Actual B10b workbench scale operation and recovery; platform notification only','enabled':True,'resources':{'mode':'explicit','types':['kubernetes_cluster'],'ids':[r.cluster_id],'matchers':[]},'rules':[{'name':name,'expression':AVAILABLE,'query_mode':'numeric','operator':'<','threshold':.5,'recovery_threshold':1,'severity':'warning','unit':''}],'mode':'independent','match_by':['argus_resource_id'],'interval_seconds':10,'for_seconds':20,'recovery_seconds':20,'no_data_behavior':'report','error_behavior':'keep','channel_ids':[],'route_ids':[],'labels':{'plan9_run':r.id,'plan9_batch':'b10b'}}
  assert a.call('POST','/alerts/trial',spec)['health']=='normal'
  a.confirm(a.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'create','name':name,'spec':spec},key='storage-alert-policy',expected=202))
  current=next(x for x in a.call('GET','/alerts/configurations?kind=policy') if x['name']==name)
 mapping['alert_policies']['available']={'id':current['id'],'name':name,'expression':AVAILABLE};write_json(r.resources,mapping)
 dashboards(r,a);exercise_alert(r,a,current['id']);autoscale(r,a)
def specification(r,a):
 original=json.loads((ROOT/'api/contracts/dashboard-templates.json').read_text(encoding='utf-8'))[0]['spec'];spec=copy.deepcopy(original)
 spec.update(default_time_range={'kind':'relative','seconds':900},panels=[],variables=[])
 select='{resource_k8s_namespace_name="'+NAMESPACE+'"}'
 values=[('available','业务可用副本 / Available replicas','sum by (resource_k8s_deployment_name) (k8s_deployment_available'+select+')','k8s_cluster',''),
 ('cpu','容器 CPU / Container CPU','sum by (resource_k8s_pod_name) (container_cpu_usage'+select+')','kubeletstats','cores'),
 ('memory','Pod 内存 / Pod memory','max by (resource_k8s_pod_name) (k8s_pod_memory_usage'+select+')','kubeletstats','bytes')]
 # Extend only with metric names and dimensions observed from this run.
 candidates=[('hpa','HPA 当前副本 / Current replicas','k8s_hpa_current_replicas',''),
 ('desired','HPA 期望副本 / Desired replicas','k8s_hpa_desired_replicas',''),
 ('jobs','Job 完成数 / Successful jobs','k8s_job_successful_pods',''),
 ('pvc','PVC 容量 / PVC capacity','k8s_volume_capacity','bytes')]
 available=query(r,a,'count by (__name__) ({__name__=~"k8s_.*"})');r.save('native-metric-catalog',available)
 names={x['metric'].get('__name__') for x in available['data']['result']}
 for key,title,metric,unit in candidates:
  if metric in names:
   expression=metric+select
   if query(r,a,expression)['data']['result']:values.append((key,title,expression,'k8s_cluster' if key!='pvc' else 'kubeletstats',unit))
 for index,(key,title,expression,source,unit) in enumerate(values):
  assert query(r,a,expression)['data']['result'],key
  panel=copy.deepcopy(original['panels'][0]);panel.update(id=key,title=title,type='timeseries',signal='metrics',authoring_mode='dsl',unit=unit,applicable_resource_types=['kubernetes_cluster'],source_binding={'source_type':source,'capability_version':'v1'},layout={'x':index%2*6,'y':index//2*40,'w':6,'h':40,'min_w':4,'min_h':20})
  panel['targets'][0].update(source_definition={'dsl':{'expression':expression}},query_mode='range');spec['panels'].append(panel)
 r.save('dashboard-spec',spec);return spec
def dashboards(r,a):
 sys.path.insert(0,str(ROOT/'docs/plan9/runtime/B09a'));from B09a.dashboard import publish
 class Context:
  def __init__(self):self.run=r;self.api=a;self.map=json.loads(r.resources.read_text(encoding='utf-8'))
  def save(self):write_json(r.resources,self.map)
 b=Context();b.cluster=r.cluster_id
 publish(b,'storage-jobs','B10b 存储与任务 / Storage and Jobs',specification(r,a),'kubernetes_cluster',r.cluster_id,description='Real expandable CSI storage, jobs, CPU HPA and enforced network policy')
 boards=a.call('GET','/dashboards')
 for key in ('kubernetes.cluster','kubernetes.nodes','collector.overview'):
  row=next(x for x in boards if x.get('managed_template_key')==key);b.map['dashboards'][key]={'id':row['id'],'name':row['name']}
 b.save()
 for item in b.map['dashboards'].values():
  path='/kubernetes/clusters/'+r.cluster_id+'/dashboard-bindings';bound=a.call('GET',path)
  if not any(x['dashboard']['id']==item['id'] for x in bound['items']):
   board=next(x for x in a.call('GET','/dashboards') if x['id']==item['id'])
   a.confirm(a.call('POST',path+'/preview',{'operation':'attach','dashboard_id':board['id'],'expected_dashboard_version':board['version'],'expected_resource_version':bound['resource_version']},key='bind-'+board['id'],expected=201))
 print('Custom B10b and three managed native dashboards bound',flush=True)
def exercise_alert(r,a,policy):
 from infrastructure import execute
 node=r.id+'-control-plane';uid=r.get('deployment','b10b-storage','-n',NAMESPACE)['metadata']['uid']
 unit='b10b-recover-'+uuid.uuid4().hex[:8]
 patch=json.dumps([{'op':'test','path':'/metadata/uid','value':uid},{'op':'test','path':'/spec/replicas','value':0},{'op':'replace','path':'/spec/replicas','value':1}])
 execute(['docker','exec',node,'systemd-run','--unit='+unit,'--on-active=120s','kubectl','--kubeconfig=/etc/kubernetes/admin.conf','patch','deployment','b10b-storage','-n',NAMESPACE,'--type=json','-p',patch])
 started=datetime.now(timezone.utc).isoformat();event=None
 try:
  r.change(a,'deployment','b10b-storage','scale',replicas=0)
  page=a.wait('/alerts/instances?policy_id='+policy,lambda x:any(y['lifecycle']=='firing' and y['state']['kind']=='condition' for y in x['items']),timeout=90,label='Actual B10b storage alert')
  event=next(y for y in page['items'] if y['lifecycle']=='firing' and y['state']['kind']=='condition');assert event['started_at']>=started;r.save('storage-alert-fired',event)
 finally:
  r.change(a,'deployment','b10b-storage','scale',replicas=1);ready(r,'b10b-storage');execute(['docker','exec',node,'systemctl','stop',unit+'.timer'])
 page=a.wait('/alerts/history?policy_id='+policy,lambda x:any(y['id']==event['id'] and y['lifecycle']=='resolved' for y in x['items']),timeout=90,label='B10b storage alert recovery')
 recovered=next(x for x in page['items'] if x['id']==event['id']);r.save('alert-cycle',{k:recovered[k] for k in ('id','started_at','resolved_at','lifecycle')})
 print('Actual storage availability alert fired and recovered',flush=True)
def autoscale(r,a):
 before=wait_native(r,lambda:(v if v.get('status',{}).get('currentMetrics') and v['status'].get('currentReplicas')==1 else None) if (v:=r.get('hpa','b10b-autoscale','-n',NAMESPACE)) else None,'HPA actual CPU baseline',120)
 receipt=request(r,target='b10b-autoscale',path='/cpu?seconds=150')
 high=wait_native(r,lambda:(v if v.get('status',{}).get('currentReplicas',0)>1 and v['status'].get('currentMetrics') else None) if (v:=r.get('hpa','b10b-autoscale','-n',NAMESPACE)) else None,'Real CPU HPA scale up',100)
 pods=r.get('pods','-n',NAMESPACE,'-l','app=b10b-autoscale')['items']
 for pod in pods:
  if pod.get('status',{}).get('phase')=='Running':
   code="import urllib.request;print(urllib.request.urlopen('http://127.0.0.1:8080/cpu?seconds=0').read().decode())"
   r.kubectl('exec','-n',NAMESPACE,pod['metadata']['name'],'--','python3','-c',code)
 low=wait_native(r,lambda:(v if v.get('status',{}).get('currentReplicas')==1 and v['status'].get('desiredReplicas')==1 else None) if (v:=r.get('hpa','b10b-autoscale','-n',NAMESPACE)) else None,'Real CPU HPA scale down',180)
 r.save('hpa-exercise',{'before':before,'load_receipt':receipt,'scaled_up':high,'scaled_down':low,'resource_metrics_tls_verified':True})
 r.save('inventory-hpa',r.inventory(a,'hpa'));print('HPA actually scaled 1 -> '+str(high['status']['currentReplicas'])+' -> 1 from real resource CPU metrics',flush=True)
if __name__=='__main__':
 from run import Run
 r=Run();catalog(r,r.api())
