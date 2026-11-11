"""Explicit later retirement; shares typed lifecycle cleanup and proves ownership."""
import json,sys
from run import ROOT,execute
def retire(r,a):
 before=r.platform()
 mapping=json.loads(r.resources.read_text(encoding='utf-8'));assert mapping['run_id']==r.id
 sys.path.insert(0,str(ROOT/'docs/plan9/runtime/B09a'))
 from B09a.integration import Batch
 from B09a.retire import retire as retire_owned
 retire_owned(Batch(r,a))
 assert not execute(['docker','container','ls','-aq','--filter','label=io.x-k8s.kind.cluster='+r.id])
 assert a.call('GET','/enterprise/kubernetes-clusters/'+mapping['cluster']['id'],expected=404).get('code')
 after=r.platform();assert before==after,'Formal services or storage changed during batch cleanup'
 r.save('cleanup-verification',{'status':'destroyed','run_id':r.id,'owned_runtime_containers':0,'formal_platform_and_pvcs_preserved':True,'history_preserved':True})
 record=json.loads((r.out/'run.json').read_text(encoding='utf-8'));record['status']='destroyed';r.save('run',record)
if __name__=='__main__':
 from run import Run
 r=Run();retire(r,r.api())
