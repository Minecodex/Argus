"""Real cluster/node metrics, scoped dashboards and replica availability alert."""
import copy,json,time,uuid,sys
from datetime import datetime,timedelta,timezone
from infrastructure import NAMESPACE,ROOT,write_json,execute
from manifests import template,meta
AVAILABLE='max by (argus_resource_id) (k8s_deployment_available{resource_k8s_namespace_name="'+NAMESPACE+'",resource_k8s_deployment_name="b10a-api"})'
BUDGET={'max_scan_bytes':268435456,'max_rows':20000,'max_samples':2000000,'max_series':20000,'timeout_ms':10000,'max_result_bytes':8388608}
def query(r,a,expression):
 end=datetime.now(timezone.utc)
 return a.call('POST','/enterprise/metrics/query',{'resource_ids':[r.cluster_id],'time_range':{'from':(end-timedelta(minutes=15)).isoformat(),'to':end.isoformat()},'budget':BUDGET,'query':expression})
def configure(r,a):
 mapping=json.loads(r.resources.read_text(encoding="utf-8"));role=json.loads((r.out.parent/'common-evaluator.json').read_text(encoding="utf-8"));mapping['evaluator']=role
 path='/enterprise/data-authorizations/role/'+role['role_id']
 grants=a.call('GET',path+'?resource_type=kubernetes_cluster')
 if not any(x['direct'] and x['resource_id']==r.cluster_id for x in grants['items']):
  a.call('POST',path,{'resource_type':'kubernetes_cluster','resource_ids':[r.cluster_id],'remove':False,'expected_version':grants['authorization_version']},expected=204)
 name='B10a API deployment has no available replica'
 current=next((x for x in a.call('GET','/alerts/configurations?kind=policy') if x['name']==name),None)
 if current is None:
  deadline=time.monotonic()+120
  while time.monotonic()<deadline:
   value=query(r,a,AVAILABLE)['data']['result']
   if value and float(value[0]['value'][1])==1:break
   time.sleep(3)
  else:raise RuntimeError('Real available replica metric did not reach Argus')
  spec={'description':'Real B10a platform scale operation and recovery','enabled':True,'resources':{'mode':'explicit','types':['kubernetes_cluster'],'ids':[r.cluster_id],'matchers':[]},
   'rules':[{'name':name,'expression':AVAILABLE,'query_mode':'numeric','operator':'<','threshold':.5,'recovery_threshold':1,'severity':'warning','unit':''}],
   'mode':'independent','match_by':['argus_resource_id'],'interval_seconds':10,'for_seconds':20,'recovery_seconds':20,'no_data_behavior':'report','error_behavior':'keep','channel_ids':[],'route_ids':[],'labels':{'plan9_run':r.id,'plan9_batch':'b10a'}}
  assert a.call('POST','/alerts/trial',spec)['health']=='normal'
  a.confirm(a.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'create','name':name,'spec':spec},key='replica-alert-policy',expected=202))
  current=next(x for x in a.call('GET','/alerts/configurations?kind=policy') if x['name']==name)
 mapping['alert_policies']['available']={'id':current['id'],'name':name,'expression':AVAILABLE};write_json(r.resources,mapping)
 # Only the user batch's low-rate HTTP generator runs until a fixed deadline.
 file=r.out/'activity-deadline.json'
 deadline=json.loads(file.read_text(encoding="utf-8"))['epoch'] if file.exists() else time.time()+8*3600
 if not file.exists():write_json(file,{'epoch':deadline,'hours':8})
 if not r.kubectl('get','deployment','b10a-driver','-n',NAMESPACE,'--ignore-not-found','-o','name'):
  pod=template(r,'b10a-driver','load');pod['spec']['containers'][0]['env'].append({'name':'PLAN9_DEADLINE','value':str(deadline)})
  obj={'apiVersion':'apps/v1','kind':'Deployment','metadata':meta(r,'b10a-driver'),'spec':{'replicas':1,'selector':{'matchLabels':{'app':'b10a-driver'}},'template':pod}}
  r.mutate(a,{'operation':'create','type':'deployment','name':'b10a-driver','object':obj},'create-real-driver')
 dashboards(r,a)
 exercise_alert(r,a,current['id'])
def dashboards(r,a):
 sys.path.insert(0,str(ROOT/'docs/plan9/runtime/B09a'))
 from B09a.dashboard import publish,specification
 class Context:
  def __init__(self):self.run=r;self.api=a;self.map=json.loads(r.resources.read_text(encoding="utf-8"))
  def save(self):write_json(r.resources,self.map)
 b=Context();b.cluster=r.cluster_id
 # Preserve the established dashboard specification and source adapters.
 spec=specification()
 for panel in spec['panels']:
  panel['targets'][0]['source_definition']['dsl']['expression']=panel['targets'][0]['source_definition']['dsl']['expression'].replace('b09a-workloads',NAMESPACE)
 spec['default_time_range']={'kind':'relative','seconds':900}
 ident=publish(b,'workloads','B10a 工作台 / Workbench',spec,'kubernetes_cluster',r.cluster_id,description='B10a Kubernetes 工作台真实观测 / Real Kubernetes workbench observations')
 mapping=b.map;available=a.call('GET','/dashboards')
 for key in ('kubernetes.cluster','kubernetes.nodes','collector.overview'):
  row=next(x for x in available if x.get('managed_template_key')==key);mapping['dashboards'][key]={'id':row['id'],'name':row['name']}
 write_json(r.resources,mapping)
 for item in mapping['dashboards'].values():
  path='/kubernetes/clusters/'+r.cluster_id+'/dashboard-bindings';bound=a.call('GET',path)
  if not any(x['dashboard']['id']==item['id'] for x in bound['items']):
   board=next(x for x in a.call('GET','/dashboards') if x['id']==item['id'])
   a.confirm(a.call('POST',path+'/preview',{'operation':'attach','dashboard_id':board['id'],'expected_dashboard_version':board['version'],'expected_resource_version':bound['resource_version']},key='bind-'+board['id'],expected=201))
 print('Three managed dashboards and one B10a workload dashboard bound',flush=True)
def exercise_alert(r,a,policy):
 deployment=r.get('deployment','b10a-api','-n',NAMESPACE);uid=deployment['metadata']['uid'];node=r.id+'-control-plane'
 marker='b10a-recover-'+uuid.uuid4().hex[:8]
 patch=json.dumps([{'op':'test','path':'/metadata/uid','value':uid},{'op':'test','path':'/spec/replicas','value':0},{'op':'replace','path':'/spec/replicas','value':1}])
 execute(['docker','exec',node,'systemd-run','--unit='+marker,'--on-active=120s','kubectl','--kubeconfig=/etc/kubernetes/admin.conf','patch','deployment','b10a-api','-n',NAMESPACE,'--type=json','-p',patch])
 started=datetime.now(timezone.utc).isoformat();event=None
 try:
  r.change(a,'deployment','b10a-api','scale',replicas=0)
  page=a.wait('/alerts/instances?policy_id='+policy,lambda x:any(y['lifecycle']=='firing' and y['state']['kind']=='condition' for y in x['items']),timeout=90,label='Real B10a replica alert')
  event=next(y for y in page['items'] if y['lifecycle']=='firing' and y['state']['kind']=='condition')
  assert event['started_at']>=started
  r.save('replica-alert-fired',event)
 finally:
  r.change(a,'deployment','b10a-api','scale',replicas=1)
  r.kubectl('rollout','status','deployment/b10a-api','-n',NAMESPACE,'--timeout=90s',timeout=100)
  execute(['docker','exec',node,'systemctl','stop',marker+'.timer'])
 assert event is not None
 page=a.wait('/alerts/history?policy_id='+policy,lambda x:any(y['id']==event['id'] and y['lifecycle']=='resolved' for y in x['items']),timeout=90,label='Real B10a replica alert recovery')
 recovered=next(x for x in page['items'] if x['id']==event['id']);r.save('alert-cycle',{k:recovered[k] for k in ('id','started_at','resolved_at','lifecycle')})
 print('Real workbench scale-to-zero alert fired and recovered after platform restoration',flush=True)
