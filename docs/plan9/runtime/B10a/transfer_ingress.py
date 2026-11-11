"""Transfer the already-deployed immutable public image, without an external pull."""
import hashlib,json,subprocess
from run import Run,ROOT,write_json
from ingress import IMAGE
r=Run();out=r.out/'ingress-controller.tar';prefix=['kubectl','--context','docker-desktop','-n','kube-system']
pods=json.loads(subprocess.check_output(prefix+['get','pods','-l','app.kubernetes.io/name=argus-image-loader-argus','-o','json']))['items']
p=next(x for x in pods if not x['metadata'].get('deletionTimestamp') and x['status']['phase']=='Running')
assert p['metadata']['labels']['argus.io/release-id']=='argus' and any(x['kind']=='DaemonSet' and x['name']=='argus-image-loader-argus' for x in p['metadata']['ownerReferences'])
with out.open('wb') as stream:
 result=subprocess.run(prefix+['exec',p['metadata']['name'],'-c','loader','--','/host/ctr','--address','/run/containerd/containerd.sock','--namespace','k8s.io','images','export','--platform','linux/amd64','-',IMAGE.replace(':v1.15.1@','@')],stdout=stream,stderr=subprocess.PIPE,timeout=120)
assert result.returncode==0,result.stderr.decode(errors='replace')[-800:]
assert out.stat().st_size>10_000_000
write_json(r.out/'ingress-image-transfer.json',{'image':IMAGE,'archive_sha256':hashlib.sha256(out.read_bytes()).hexdigest(),'bytes':out.stat().st_size,'source':'verified_formal_node_image_cache_public_immutable_image'})
subprocess.run([str(r.kind),'load','image-archive',str(out),'--name',r.id],check=True,timeout=180)
subprocess.run(['docker','exec',r.id+'-control-plane','ctr','--namespace','k8s.io','images','tag','--force',IMAGE.replace(':v1.15.1@','@'),IMAGE],check=True)
print('Pinned public ingress image loaded from the verified node cache',flush=True)
