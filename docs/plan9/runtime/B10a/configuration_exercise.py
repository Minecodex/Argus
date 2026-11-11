"""Additional real configuration/network updates; retained objects stay usable."""
import base64,hashlib,json,secrets,time
from infrastructure import NAMESPACE
def exercise(r,a):
 result={}
 token=secrets.token_urlsafe(24);expected=hashlib.sha256(token.encode()).hexdigest();r.secret_canaries=[token]
 r.change(a,'secret','b10a-demo-secret','patch',object={'data':{'demo-token':'[REDACTED]'}},secret_values={'demo-token':token})
 command="import hashlib,pathlib;print(hashlib.sha256(pathlib.Path('/secret/demo-token').read_bytes()).hexdigest())"
 end=time.monotonic()+120
 while time.monotonic()<end:
  observed=r.kubectl('exec','-n',NAMESPACE,'deployment/b10a-api','--','python3','-c',command).strip()
  if observed==expected:break
  time.sleep(3)
 else:raise RuntimeError('Mounted protected Secret did not refresh')
 result['secret_projection_updated']=True
 r.change(a,'service','b10a-api','patch',object={'spec':{'ports':[{'name':'http','port':8080,'targetPort':8080},{'name':'review','port':8081,'targetPort':8080}]}})
 code="import urllib.request;print(urllib.request.urlopen('http://b10a-api:8081/?label=service-port-updated',timeout=5).read().decode())"
 receipt=json.loads(r.kubectl('exec','-n',NAMESPACE,'deployment/b10a-api','--','python3','-c',code));assert receipt['run']==r.id
 result['service_updated_http']=receipt
 r.change(a,'ingress','b10a-http','patch',object={'metadata':{'annotations':{'plan9.argus/review-version':'2'}}})
 from ingress import verify
 result['ingress_update_http']=verify(r)
 before={x['metadata']['uid'] for x in r.get('pods','-n',NAMESPACE,'-l','app=b10a-node-observer')['items']}
 r.change(a,'daemonset','b10a-node-observer','restart')
 r.kubectl('rollout','status','daemonset/b10a-node-observer','-n',NAMESPACE,'--timeout=90s',timeout=100)
 after={x['metadata']['uid'] for x in r.get('pods','-n',NAMESPACE,'-l','app=b10a-node-observer')['items'] if not x['metadata'].get('deletionTimestamp')}
 assert before.isdisjoint(after);result['daemonset_replaced_pod_uids']=list(after)
 r.save('configuration-network-exercise',result)
 print('Protected Secret mounted update, Service port, Ingress route and DaemonSet restart verified',flush=True)
if __name__=='__main__':
 from run import Run
 r=Run();exercise(r,r.api())
