"""B10b normal API creates and actual runtime observations."""
import json,time,uuid
from infrastructure import NAMESPACE
from manifests import deployment,pvc,job,cronjob,hpa,policy,meta
def create(r,a,kind,obj,wait=True):
 name=obj['metadata']['name']
 if r.kubectl('get',kind,name,'-n',NAMESPACE,'--ignore-not-found','-o','name'):
  assert r.get(kind,name,'-n',NAMESPACE)['metadata']['labels']['argus.plan9/run']==r.id
  return None
 return r.mutate(a,{'operation':'create','type':kind,'name':name,'object':obj},'create-'+kind+'-'+name+'-'+uuid.uuid4().hex[:8],wait=wait)
def ready(r,name):
 r.kubectl('rollout','status','deployment/'+name,'-n',NAMESPACE,'--timeout=150s',timeout=160)
def request(r,target='b10b-storage',path='/',client='b10b-allowed',timeout=5):
 code="import urllib.request;print(urllib.request.urlopen("+repr('http://'+target+':8080'+path)+",timeout="+str(timeout)+").read().decode())"
 return json.loads(r.kubectl('exec','-n',NAMESPACE,'deployment/'+client,'--','python3','-c',code,timeout=15))
def wait_native(r,fn,label,timeout=180):
 end=time.monotonic()+timeout
 while time.monotonic()<end:
  try:value=fn()
  except RuntimeError as error:
   if '(NotFound)' not in str(error):raise
   value=None
  if value:return value
  time.sleep(3)
 raise RuntimeError(label+' did not converge')
def objects(r,a):
 claim=create(r,a,'pvc',pvc(r,'b10b-data'),False)
 for name in ('b10b-storage','b10b-autoscale'):
  create(r,a,'service',{'apiVersion':'v1','kind':'Service','metadata':meta(r,name),'spec':{'selector':{'app':name},'ports':[{'port':8080,'targetPort':8080,'name':'http'}]}})
 create(r,a,'deployment',deployment(r,'b10b-storage','b10b-data'));ready(r,'b10b-storage')
 if claim:r.await_operation(a,claim,'create-pvc-b10b-data')
 for name,allowed in [('b10b-allowed',True),('b10b-denied',False)]:
  create(r,a,'deployment',deployment(r,name,mode='client',allowed=allowed));ready(r,name)
 create(r,a,'deployment',deployment(r,'b10b-autoscale'));ready(r,'b10b-autoscale')
 create(r,a,'hpa',hpa(r))
 create(r,a,'cronjob',cronjob(r))
 create(r,a,'job',job(r,'b10b-job'))
 r.save('baseline-receipt',request(r,path='/write?marker=initial-'+uuid.uuid4().hex[:8]))
 mapping=json.loads(r.resources.read_text(encoding='utf-8'));mapping['objects']={}
 for kind in ('pvc','deployment','service','job','cronjob','hpa'):
  for obj in r.get(kind,'-n',NAMESPACE)['items']:
   if obj['metadata'].get('labels',{}).get('argus.plan9/run')==r.id:mapping['objects'][kind+'/'+obj['metadata']['name']]={'uid':obj['metadata']['uid'],'resource_version':obj['metadata']['resourceVersion'],'namespace':NAMESPACE}
 r.save('resource-map',mapping)
 print('PVC, real file server, clients, autoscaling target, HPA, Job and CronJob created through Argus',flush=True)
def storage(r,a):
 for client in ('b10b-allowed','b10b-denied'):r.change(a,'deployment',client,'scale',replicas=0)
 marker='persistent-'+uuid.uuid4().hex[:8];before=request(r,path='/write?marker='+marker,client='b10b-storage')
 r.change(a,'deployment','b10b-storage','restart');ready(r,'b10b-storage')
 after=request(r,client='b10b-storage');assert after['receipt']['marker']==marker
 capacity=after['capacity_bytes']
 claim=r.get('pvc','b10b-data','-n',NAMESPACE)
 if claim['spec']['resources']['requests']['storage']!='128Mi':
  r.change(a,'pvc','b10b-data','patch',object={'spec':{'resources':{'requests':{'storage':'128Mi'}}}})
 expanded=request(r,client='b10b-storage');assert expanded['receipt']['marker']==marker and expanded['capacity_bytes']>capacity
 current=r.get('pvc','b10b-data','-n',NAMESPACE);assert current['status']['capacity']['storage']=='128Mi'
 response=a.call('POST','/enterprise/kubernetes-clusters/'+r.cluster_id+'/inventory/actions/preview-mutation',{'type':'pvc','name':'b10b-data','operation':'patch','namespace_id':r.namespace(a),'uid':current['metadata']['uid'],'expected_resource_version':current['metadata']['resourceVersion'],'object':{'spec':{'resources':{'requests':{'storage':'32Mi'}}}}},key='shrink-must-be-rejected',expected=[400,409,422])
 assert response.get('code')
 scratch=create(r,a,'pvc',pvc(r,'b10b-scratch','32Mi'),False)
 create(r,a,'deployment',deployment(r,'b10b-scratch','b10b-scratch'));ready(r,'b10b-scratch')
 if scratch:r.await_operation(a,scratch,'scratch-pvc-bound')
 volume=r.get('pvc','b10b-scratch','-n',NAMESPACE)['spec']['volumeName']
 r.change(a,'deployment','b10b-scratch','delete')
 wait_native(r,lambda:not r.get('pods','-n',NAMESPACE,'-l','app=b10b-scratch')['items'],'scratch consumer deleted')
 r.change(a,'pvc','b10b-scratch','delete')
 r.kubectl('wait','--for=delete','pv/'+volume,'--timeout=120s',timeout=130)
 r.save('storage-exercise',{'before':before,'after_restart':after,'after_online_expansion':expanded,'pvc':current,'shrink_rejected':response['code'],'scratch_volume_deleted':volume})
 for kind in ('pvc','pv','storageclass'):r.save('inventory-'+kind,r.inventory(a,kind,namespace=kind=='pvc'))
 for client in ('b10b-allowed','b10b-denied'):r.change(a,'deployment',client,'scale',replicas=1);ready(r,client)
 print('Actual file persisted through restart, mounted filesystem expanded 64Mi->128Mi, shrink rejected and scratch volume reclaimed',flush=True)
def jobs(r,a):
 submit=create(r,a,'job',job(r,'b10b-pausable-job',30),False)
 wait_native(r,lambda:r.get('job','b10b-pausable-job','-n',NAMESPACE).get('status',{}).get('active')==1,'Job active')
 r.change(a,'job','b10b-pausable-job','patch',object={'spec':{'suspend':True}})
 suspended=r.get('job','b10b-pausable-job','-n',NAMESPACE);assert suspended['spec']['suspend'] and not suspended['status'].get('active')
 r.change(a,'job','b10b-pausable-job','patch',object={'spec':{'suspend':False}})
 completed=wait_native(r,lambda:r.get('job','b10b-pausable-job','-n',NAMESPACE).get('status',{}).get('succeeded'),'Resumed Job completed',90)
 r.change(a,'cronjob','b10b-cron','patch',object={'spec':{'suspend':True}})
 paused=r.get('cronjob','b10b-cron','-n',NAMESPACE);assert paused['spec']['suspend']
 name='b10b-manual-'+uuid.uuid4().hex[:6]
 r.mutate(a,{'operation':'create','type':'job','name':name,'cronjob_source':{'name':'b10b-cron','uid':paused['metadata']['uid'],'resource_version':paused['metadata']['resourceVersion']}},'manual-'+name)
 r.change(a,'cronjob','b10b-cron','patch',object={'spec':{'suspend':False}})
 scheduled=wait_native(r,lambda:next((x for x in r.get('jobs','-n',NAMESPACE)['items'] if any(y['kind']=='CronJob' and y['uid']==paused['metadata']['uid'] for y in x['metadata'].get('ownerReferences',[])) and x.get('status',{}).get('succeeded')),None),'Actual scheduled CronJob',timeout=170)
 logs=r.kubectl('logs','-n',NAMESPACE,'job/'+name)
 assert 'job-completed' in logs
 r.save('jobs-exercise',{'suspended':suspended,'manual_job':name,'scheduled_job':scheduled,'manual_stdout':logs,'cron_resumed':not r.get('cronjob','b10b-cron','-n',NAMESPACE)['spec']['suspend']})
 from deadline import install
 install(r)
 for kind in ('job','cronjob'):r.save('inventory-'+kind,r.inventory(a,kind))
 print('Job suspend/resume, CronJob pause/resume, normal manual run and real scheduled execution completed',flush=True)
def network(r,a):
 allowed=request(r);denied_baseline=request(r,client='b10b-denied')
 create(r,a,'networkpolicy',policy(r))
 def blocked():
  try:request(r,client='b10b-denied',timeout=2);return False
  except RuntimeError:return True
 wait_native(r,blocked,'Real Calico traffic denial',30)
 assert request(r)['run']==r.id
 r.change(a,'networkpolicy','b10b-storage-allow-client','delete')
 recovered=wait_native(r,lambda:request(r,client='b10b-denied'),'Traffic restored',30)
 create(r,a,'networkpolicy',policy(r));wait_native(r,blocked,'Retained isolation enforced',30)
 r.save('network-policy-exercise',{'baseline_allowed':allowed,'baseline_denied':denied_baseline,'denied_after_policy':True,'allowed_after_policy':request(r),'restored_after_delete':recovered,'final_policy_enforced':True})
 r.save('inventory-networkpolicy',r.inventory(a,'networkpolicy'))
 print('Calico policy verified by successful allowed traffic, blocked denied traffic and restoration',flush=True)
def monitoring(r,a):
 from monitoring import configure
 configure(r,a)
def verify(r,a):
 from verification import verify
 verify(r,a)
def retire(r,a):
 from retirement import retire
 retire(r,a)
actions={'objects':objects,'storage-test':storage,'jobs-test':jobs,'network-test':network,'monitoring':monitoring,'verify':verify,'retire':retire}
