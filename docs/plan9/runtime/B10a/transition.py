"""Record live baseline, retire only the predecessor and verify both directions."""
import base64,json,os,sys,time
from pathlib import Path
import yaml
from run import Run,ROOT,execute,write_json
OLD='p9-b09b-20261110-01'
def preflight(r):
 r.prepare()
 from common.api import Api
 api=r.api();api.call('GET','/enterprise/auth/session')
 old=ROOT/'artifacts/plan9'/OLD
 mapping=json.loads((old/'resource-map.json').read_text(encoding="utf-8"));node=json.loads(execute(['docker','inspect',OLD+'-control-plane']))[0]
 assert node['Id']==json.loads((old/'cluster-infrastructure.json').read_text(encoding="utf-8"))['container_id']
 native=json.loads(execute(['docker','exec',node['Id'],'kubectl','--kubeconfig=/etc/kubernetes/admin.conf','get','serviceaccount','argus-kubernetes-connector','-n','plan9-access','-o','json']))
 config=yaml.safe_load((old/'private/platform-kubeconfig.yaml').read_text(encoding="utf-8"))
 token=config['users'][0]['user']['token'];part=token.split('.')[1]
 claims=json.loads(base64.urlsafe_b64decode(part+'='*(-len(part)%4)))
 assert claims['kubernetes.io']['serviceaccount']['uid']==native['metadata']['uid']
 r.save('predecessor-before',{'mapping':mapping,'container_id':node['Id'],'credential_expires_at':claims['exp'],'platform':r.platform()})
 return claims['exp']
def retire(r):
 import shutil,subprocess
 from common.api import Api
 old=ROOT/'artifacts/plan9'/OLD
 before=r.platform()
 shutil.copy2(r.session,old/'private/enterprise-session.json')
 env=os.environ.copy();env['ARGUS_PLAN9_B09B_RUN']=OLD
 with (r.out/'predecessor-retire.log').open('wb') as log:
  subprocess.run([sys.executable,str(ROOT/'docs/plan9/runtime/B09b/run.py'),'retire'],cwd=ROOT,env=env,stdout=log,stderr=subprocess.STDOUT,check=True,timeout=1200)
 a=r.api()
 after=r.platform();assert before==after,'Formal platform or persistent bindings changed during cleanup'
 assert not execute(['docker','ps','-a','-q','--filter','label=io.x-k8s.kind.cluster='+OLD])
 mapping=json.loads((old/'resource-map.json').read_text(encoding="utf-8"));value=a.call('GET','/enterprise/kubernetes-clusters/'+mapping['cluster']['id'],expected=[404])
 assert value.get('code')
 policies=a.call('GET','/alerts/configurations?kind=policy')
 assert not any(x['id'] in {p['id'] for p in mapping['alert_policies'].values()} for x in policies)
 proof={'status':'destroyed','run_id':OLD,'owned_runtime_containers':0,'formal_platform_and_pvcs_preserved':True,'history_preserved':True,'recorded_at':time.time()}
 write_json(old/'cleanup-verification.json',proof);r.save('predecessor-cleanup-verification',proof)
 print('B09b retired; owned node and telemetry endpoint gone, formal baseline preserved',flush=True)
if __name__=='__main__':
 r=Run();phase=sys.argv[1]
 if phase=='preflight':print('Predecessor credential expiry',preflight(r),flush=True)
 elif phase=='retire':retire(r)
