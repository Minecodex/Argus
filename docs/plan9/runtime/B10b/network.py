"""Calico only in this fresh cluster; prove readiness before workload enrollment."""
import hashlib,json
from pathlib import Path
import requests,yaml
from infrastructure import Infrastructure,write_json
from images import stage
def install(r):
 path=r.out/'calico-v3.33.0.yaml'
 if not path.exists():
  value=requests.get('https://raw.githubusercontent.com/projectcalico/calico/v3.33.0/manifests/calico.yaml',timeout=60);value.raise_for_status();path.write_bytes(value.content)
 assert hashlib.sha256(path.read_bytes()).hexdigest()=='2de8f47595fb9c41b3f47d7b767a1f8e72ecf84057af834738ff12689a234da5'
 docs=[x for x in yaml.safe_load_all(path.read_text(encoding='utf-8')) if x]
 references=sorted({c['image'] for o in docs if o['kind'] in ('Deployment','DaemonSet') for c in o['spec']['template']['spec'].get('containers',[])+o['spec']['template']['spec'].get('initContainers',[])})
 pinned={ref:stage(r,ref) for ref in references}
 for obj in docs:
  obj.setdefault('metadata',{}).setdefault('labels',{})['argus.plan9/run']=r.id
  if obj['kind'] in ('Deployment','DaemonSet'):
   spec=obj['spec']['template']['spec']
   for c in spec.get('containers',[])+spec.get('initContainers',[]):
    c['image']=pinned[c['image']];c['imagePullPolicy']='IfNotPresent'
    c.setdefault('resources',{}).setdefault('limits',{}).update(cpu='350m',memory='256Mi')
    if c['name']=='calico-node':
     for e in c.get('env',[]):
      if e['name']=='CALICO_IPV4POOL_IPIP':e['value']='Never'
      if e['name']=='CALICO_IPV4POOL_VXLAN':e['value']='Always'
     c['env'].append({'name':'CALICO_IPV4POOL_CIDR','value':'192.168.0.0/16'})
 r.kubectl('apply','--server-side','--field-manager=argus-plan9-b10b','-f','-',data=json.dumps({'apiVersion':'v1','kind':'List','items':docs}),timeout=180)
 r.kubectl('rollout','status','daemonset/calico-node','-n','kube-system','--timeout=180s',timeout=190)
 r.kubectl('rollout','status','deployment/calico-kube-controllers','-n','kube-system','--timeout=180s',timeout=190)
 r.kubectl('wait','--for=condition=Ready','nodes','--all','--timeout=120s',timeout=130)
 r.save('calico-install',{'version':'v3.33.0','source_sha256':hashlib.sha256(path.read_bytes()).hexdigest(),'images':pinned,'nodes':r.get('nodes'),'formal_context_changed':False})
 print('Fresh B10b Calico and node ready',flush=True)
if __name__=='__main__':install(Infrastructure())
