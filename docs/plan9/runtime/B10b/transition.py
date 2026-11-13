"""Retire only the verified B10a predecessor; preserve the formal platform."""
import base64,json,os,shutil,subprocess,sys,time
import yaml
from infrastructure import Infrastructure,ROOT,PREDECESSOR,execute
r=Infrastructure();r.prepare();old=ROOT/'artifacts/plan9'/PREDECESSOR
shutil.copy2(old/'private/enterprise-session.json',r.session)
a=r.api();a.call('GET','/enterprise/auth/session')
before=r.platform()
record=json.loads((old/'cluster-infrastructure.json').read_text(encoding='utf-8'))
node=json.loads(execute(['docker','inspect',PREDECESSOR+'-control-plane']))[0]
assert node['Id']==record['container_id'] and node['Config']['Labels']['io.x-k8s.kind.cluster']==PREDECESSOR
mapping=json.loads((old/'resource-map.json').read_text(encoding='utf-8'))
assert a.call('GET','/enterprise/kubernetes-clusters/'+mapping['cluster']['id'])['labels']['plan9_run']==PREDECESSOR
r.save('predecessor-before',{'resource_map':mapping,'container_id':node['Id'],'formal_platform':before})
with (r.out/'predecessor-retire.log').open('wb') as log:
 subprocess.run([sys.executable,str(ROOT/'docs/plan9/runtime/B10a/run.py'),'retire'],cwd=ROOT,stdout=log,stderr=subprocess.STDOUT,check=True,timeout=1200)
after=r.platform();assert before==after
assert not execute(['docker','ps','-aq','--filter','label=io.x-k8s.kind.cluster='+PREDECESSOR])
proof=json.loads((old/'cleanup-verification.json').read_text(encoding='utf-8'))
assert proof['status']=='destroyed'
r.save('predecessor-cleanup-verification',proof)
print('B10a retired through normal APIs; formal deployments and six PVC bindings preserved',flush=True)
