"""Load exact digest, update only the Kubernetes readers/operation executors, preserve other services."""
import json
from run import Run,execute,write_json
r=Run()
before=r.platform();r.save('native-ca-platform-before-backend',before)
digest=json.loads((r.out/'native-ca-file-fix/backend-image.json').read_text(encoding="utf-8"))['containerimage.digest']
image='host.docker.internal:5001/argus/argus-backend:local-20261112-b10b-native-ca-v3@'+digest
base=['kubectl','--context','docker-desktop'];pods=json.loads(execute(base+['get','pods','-n','kube-system','-l','app.kubernetes.io/name=argus-image-loader-argus','-o','json']))['items']
for p in pods:
 assert p['metadata']['labels']['argus.io/release-id']=='argus' and not p['metadata'].get('deletionTimestamp')
 cmd=base+['exec','-n','kube-system',p['metadata']['name'],'-c','loader','--','/host/ctr','--address','/run/containerd/containerd.sock','--namespace','k8s.io','images']
 execute(cmd+['pull','--plain-http',image],timeout=180)
 execute(cmd+['tag','--force',image,'host.docker.internal:5001/argus/argus-backend@'+digest])
report={}
for name in ['argus-server','argus-worker','argus-direct-executor','argus-connector-gateway']:
 cmd=base+['-n','argus-system'];current=json.loads(execute(cmd+['get','deployment',name,'-o','json']))
 old=current['spec']['template']['spec']['containers'][0]['image']
 assert current['metadata']['uid']==before['deployments'][name]['uid'] and old==before['deployments'][name]['images'][0]
 patch=[{'op':'test','path':'/metadata/uid','value':current['metadata']['uid']},{'op':'test','path':'/metadata/resourceVersion','value':current['metadata']['resourceVersion']},{'op':'test','path':'/spec/template/spec/containers/0/image','value':old},{'op':'replace','path':'/spec/template/spec/containers/0/image','value':image}]
 file=r.out/(name+'-fix-patch.json');write_json(file,patch)
 execute(cmd+['patch','deployment',name,'--type=json','--patch-file',str(file)])
 execute(cmd+['rollout','status','deployment/'+name,'--timeout=180s'],timeout=190)
 active=[x for x in json.loads(execute(cmd+['get','pods','-l','app.kubernetes.io/name='+name,'-o','json']))['items'] if not x['metadata'].get('deletionTimestamp')]
 assert active and all(x['status']['containerStatuses'][0]['ready'] and x['status']['containerStatuses'][0]['imageID'].endswith(digest) for x in active)
 report[name]={'image':image,'previous':old,'pod_uids':[x['metadata']['uid'] for x in active]}
 r.save('native-ca-backend-release',report);print(name+': exact image ready',flush=True)
after=r.platform();assert after['pvc']==before['pvc']
for name,row in before['deployments'].items():
 if name not in report:assert after['deployments'][name]==row,name
r.save('native-ca-platform-after-backend',after)
print('Other deployments and all six formal persistent bindings preserved',flush=True)
