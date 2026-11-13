"""Retire consumers and CSI claims before deleting the owned Kind node."""
import json,sys
from run import ROOT,execute
from infrastructure import NAMESPACE
def retire(r,a):
 before=r.platform();mapping=json.loads(r.resources.read_text(encoding='utf-8'));assert mapping['run_id']==r.id
 ns=r.get('namespace',NAMESPACE);assert ns['metadata']['uid']==mapping['namespaces'][NAMESPACE]['uid'] and ns['metadata']['labels']['argus.plan9/run']==r.id
 volumes=[x['spec'].get('volumeName') for x in r.get('pvc','-n',NAMESPACE)['items']]
 proof=[]
 for kind in ('cronjob','hpa','deployment','job','networkpolicy','service'):
  for obj in r.get(kind,'-n',NAMESPACE)['items']:
   own=obj['metadata'].get('labels',{}).get('argus.plan9/run')==r.id or obj.get('spec',{}).get('template',{}).get('metadata',{}).get('labels',{}).get('argus.plan9/run')==r.id
   if not own:raise RuntimeError('Unowned '+kind+' blocks automatic retirement')
   r.change(a,kind,obj['metadata']['name'],'delete');proof.append({'type':kind,'uid':obj['metadata']['uid']})
 r.kubectl('wait','--for=delete','pods','--all','-n',NAMESPACE,'--timeout=120s',timeout=130)
 for obj in r.get('pvc','-n',NAMESPACE)['items']:
  assert obj['metadata']['labels']['argus.plan9/run']==r.id
  r.change(a,'pvc',obj['metadata']['name'],'delete');proof.append({'type':'pvc','uid':obj['metadata']['uid']})
 for volume in volumes:
  if volume:r.kubectl('wait','--for=delete','pv/'+volume,'--timeout=120s',timeout=130)
 r.save('csi-retirement',{'consumers_deleted':proof,'owned_pvs_reclaimed':volumes})
 sys.path.insert(0,str(ROOT/'docs/plan9/runtime/B09a'));from B09a.integration import Batch;from B09a.retire import retire as retire_owned
 retire_owned(Batch(r,a))
 assert not execute(['docker','container','ls','-aq','--filter','label=io.x-k8s.kind.cluster='+r.id])
 assert a.call('GET','/enterprise/kubernetes-clusters/'+mapping['cluster']['id'],expected=404).get('code')
 assert r.platform()==before
 r.save('cleanup-verification',{'status':'destroyed','run_id':r.id,'owned_runtime_containers':0,'formal_platform_and_pvcs_preserved':True,'history_preserved':True,'csi_volumes_reclaimed_before_node_deletion':True})
 record=json.loads((r.out/'run.json').read_text(encoding='utf-8'));record['status']='destroyed';r.save('run',record)
