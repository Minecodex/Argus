"""Actual workbench calls, native observations and retained B10a objects."""
import copy,json,secrets,time,uuid
from datetime import datetime,timezone
from urllib.parse import urlencode
from infrastructure import NAMESPACE,IMAGE,write_json
from manifests import objects,template,meta
def ready(r,kind,name,timeout=120):
 r.kubectl('rollout','status',kind+'/'+name,'-n',NAMESPACE,'--timeout='+str(timeout)+'s',timeout=timeout+10)
def native_receipt(r,label='verification'):
 code="import json,urllib.request;print(urllib.request.urlopen('http://b10a-api:8080/?label="+label+"',timeout=5).read().decode())"
 return json.loads(r.kubectl('exec','-n',NAMESPACE,'deployment/b10a-api','--','python3','-c',code))
def create_objects(r,a):
 token=secrets.token_urlsafe(24);r.secret_canaries=[token]
 for kind,obj in objects(r):
  existing=r.kubectl('get',kind,obj['metadata']['name'],'-n',NAMESPACE,'--ignore-not-found','-o','name')
  if existing:
   assert r.get(kind,obj['metadata']['name'],'-n',NAMESPACE)['metadata']['labels']['argus.plan9/run']==r.id
   continue
  body={'operation':'create','type':kind,'name':obj['metadata']['name'],'object':obj}
  if kind=='secret':body['secret_values']={'demo-token':token}
  r.mutate(a,body,'create-'+kind+'-'+obj['metadata']['name'])
 for kind,name in [('deployment','b10a-api'),('statefulset','b10a-ledger'),('daemonset','b10a-node-observer')]:ready(r,kind,name)
 receipt=native_receipt(r);assert receipt['mode']=='baseline' and receipt['secret_present']
 mapping=json.loads(r.resources.read_text(encoding="utf-8"));mapping['objects']={}
 for kind,obj in objects(r):
  current=r.get(kind,obj['metadata']['name'],'-n',NAMESPACE)
  mapping['objects'][kind+'/'+obj['metadata']['name']]={'uid':current['metadata']['uid'],'resource_version':current['metadata']['resourceVersion'],'namespace':NAMESPACE}
 write_json(r.resources,mapping);r.save('baseline-receipt',receipt)
 print('Eight B10a objects created by Argus; native rollout, HTTP, config and Secret presence verified',flush=True)
def exercise(r,a):
 report={}
 # Namespace lifecycle and UID observation use the same public mutation API.
 name='b10a-transient'
 r.mutate(a,{'operation':'create','type':'namespace','name':name,'object':{'apiVersion':'v1','kind':'Namespace','metadata':{'name':name,'labels':{'argus.plan9/run':r.id}}}},'create-transient-namespace')
 ns=r.get('namespace',name)
 r.mutate(a,{'operation':'delete','type':'namespace','name':name,'uid':ns['metadata']['uid'],'expected_resource_version':ns['metadata']['resourceVersion']},'delete-transient-namespace')
 r.kubectl('wait','--for=delete','namespace/'+name,'--timeout=90s')
 report['namespace_created_and_deleted']=ns['metadata']['uid']
 # Native ready counts and Pod identities must change after platform writes.
 for kind,name in [('deployment','b10a-api'),('statefulset','b10a-ledger')]:
  r.change(a,kind,name,'scale',replicas=2);ready(r,kind,name)
  current=r.get(kind,name,'-n',NAMESPACE);assert current['status']['readyReplicas']==2
  report[kind+'_scale_two']={'uid':current['metadata']['uid'],'ready':2}
  r.change(a,kind,name,'scale',replicas=1);ready(r,kind,name)
 before=r.get('pods','-n',NAMESPACE,'-l','app=b10a-api')['items'][0]
 r.change(a,'deployment','b10a-api','restart');ready(r,'deployment','b10a-api')
 after=r.get('pods','-n',NAMESPACE,'-l','app=b10a-api')['items']
 assert any(x['metadata']['uid']!=before['metadata']['uid'] and not x['metadata'].get('deletionTimestamp') for x in after)
 report['restart_old_pod_uid']=before['metadata']['uid']
 # Store the actual approved ReplicaSet reference, then patch and roll back.
 sets=r.get('replicasets','-n',NAMESPACE,'-l','app=b10a-api')['items']
 prior=max(sets,key=lambda x:int(x['metadata']['annotations'].get('deployment.kubernetes.io/revision','0')))
 ref={'name':prior['metadata']['name'],'uid':prior['metadata']['uid'],'resource_version':prior['metadata']['resourceVersion'],'revision':int(prior['metadata']['annotations']['deployment.kubernetes.io/revision'])}
 spec=next(obj for kind,obj in objects(r) if kind=='deployment')
 next(v for v in spec['spec']['template']['spec']['containers'][0]['env'] if v['name']=='VERSION')['value']='v2'
 r.change(a,'deployment','b10a-api','patch',object=spec);ready(r,'deployment','b10a-api')
 assert native_receipt(r,'version-two')['version']=='v2'
 # Updating replicas can update ReplicaSet resourceVersion. Re-read the exact
 # prior UID/revision immediately before the rollback preview.
 latest=r.get('replicaset',ref['name'],'-n',NAMESPACE);assert latest['metadata']['uid']==ref['uid']
 ref['resource_version']=latest['metadata']['resourceVersion']
 r.change(a,'deployment','b10a-api','rollback',rollback=ref);ready(r,'deployment','b10a-api')
 assert native_receipt(r,'rolled-back')['version']=='v1'
 report['rollback']=ref
 config=r.get('configmap','b10a-settings','-n',NAMESPACE)
 r.change(a,'configmap','b10a-settings','patch',object={'data':{'mode':'updated','message':'Actual ConfigMap update'}})
 # The mounted ConfigMap projection is eventually refreshed by kubelet.
 end=time.monotonic()+120
 while time.monotonic()<end:
  receipt=native_receipt(r,'config-updated')
  if receipt['mode']=='updated':break
  time.sleep(3)
 else:raise RuntimeError('Mounted configuration did not update')
 report['config_update']=receipt
 conflict={'operation':'patch','type':'configmap','name':'b10a-settings','namespace_id':r.namespace(a),'uid':config['metadata']['uid'],'expected_resource_version':config['metadata']['resourceVersion'],'object':{'data':{'mode':'stale-write-must-not-apply'}}}
 response=a.call('POST','/enterprise/kubernetes-clusters/'+r.cluster_id+'/inventory/actions/preview-mutation',conflict,key='stale-version-negative',expected=[400,409,412])
 assert response.get('code');report['version_conflict_code']=response['code']
 assert r.get('configmap','b10a-settings','-n',NAMESPACE)['data']['mode']=='updated'
 pod=next(x for x in r.get('pods','-n',NAMESPACE,'-l','app=b10a-api')['items'] if not x['metadata'].get('deletionTimestamp'))
 r.change(a,'pod',pod['metadata']['name'],'delete')
 ready(r,'deployment','b10a-api')
 deadline=time.monotonic()+90
 while time.monotonic()<deadline:
  current=r.get('pods','-n',NAMESPACE,'-l','app=b10a-api')['items']
  if any(x['metadata']['uid']!=pod['metadata']['uid'] and x['status'].get('phase')=='Running' for x in current):break
  time.sleep(2)
 else:raise RuntimeError('Deployment did not replace the deleted Pod')
 report['deleted_pod_uid']=pod['metadata']['uid']
 r.save('object-operation-exercise',report)
 print('Real namespace, scale, restart, rollback, config projection, stale-version rejection and Pod replacement verified',flush=True)
def monitoring(r,a):
 from monitoring import configure
 configure(r,a)
def sessions(r,a):
 from sessions import verify
 verify(r,a)
def verify(r,a):
 from verification import verify
 verify(r,a)
actions={'objects':create_objects,'exercise':exercise,'monitoring':monitoring,'sessions':sessions,'verify':verify}
