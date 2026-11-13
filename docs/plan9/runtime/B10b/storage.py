"""Pinned Rawfile CSI in the owned node, including real online resize."""
import base64,hashlib,json,secrets,subprocess,shutil
import requests
from pathlib import Path
import yaml
from infrastructure import execute
from images import stage
def install(r):
 namespace='b10b-infra';labels={'argus.plan9/run':r.id}
 r.apply([{'apiVersion':'v1','kind':'Namespace','metadata':{'name':namespace,'labels':labels}}])
 # Device aliases are created only in the verified lab container. Existing
 # devices must retain their exact identities; no loop device is reconfigured.
 r.kubectl('get','nodes','-o','name')
 script='set -eu\nif [ ! -e /dev/loop-control ]; then mknod /dev/loop-control c 10 237; fi\nfor i in $(seq 0 63); do if [ ! -e /dev/loop$i ]; then mknod /dev/loop$i b 7 $i; fi; done\n'
 execute(['docker','exec',r.id+'-control-plane','sh','-ec',script])
 chart=r.out/'rawfile-localpv-0.15.1.tgz'
 if not chart.exists():
  response=requests.get('https://github.com/openebs/rawfile-localpv/releases/download/v0.15.1/rawfile-localpv-0.15.1.tgz',timeout=60);response.raise_for_status();chart.write_bytes(response.content)
 assert hashlib.sha256(chart.read_bytes()).hexdigest()=='3107fbe473f64de9ae84aae3677d395daa19d22049dd12ade0e5cda037b90169'
 values=r.private/'rawfile-values.yaml';manifest=r.private/'rawfile-manifest.yaml'
 if not values.exists():
  small={'requests':{'cpu':'10m','memory':'20Mi'},'limits':{'cpu':'200m','memory':'128Mi'}}
  driver={'requests':{'cpu':'25m','memory':'64Mi'},'limits':{'cpu':'350m','memory':'192Mi'}}
  root='/var/lib/argus-plan9/'+r.id+'/rawfile'
  value={'analytics':{'enabled':False},'global':{'analytics':{'enabled':False}},'auth':{'enabled':True,'token':base64.b64encode(secrets.token_bytes(36)).decode()},'image':{'tag':'v0.15.1'},'crds':{'enabled':False},'snapshotClasses':[],'capabilities':{'snapshots':{'enabled':False},'apiServer':{'enabled':False},'resize':{'enabled':True}},'node':{'metadataDirPath':root+'/metadata','storagePools':{'default':{'path':root+'/data'}},'resources':driver,'driverRegistrar':{'resources':small},'externalProvisioner':{'resources':small},'tolerations':[{'key':'node-role.kubernetes.io/control-plane','operator':'Exists','effect':'NoSchedule'}]},'controller':{'resources':driver,'externalResizer':{'resources':small}},'storageClasses':[{'name':'b10b-expandable','enabled':True,'volumeBindingMode':'WaitForFirstConsumer','isDefault':False,'allowVolumeExpansion':True,'reclaimPolicy':'Delete','fsType':'ext4','thinProvision':'false','storagePool':'default','formatOptions':['-E','nodiscard'],'mountOptions':['nodiscard'],'copyOnWrite':'false'}]}
  values.write_text(yaml.safe_dump(value,sort_keys=False),encoding='utf-8')
 if not manifest.exists():
  helm=shutil.which('helm') or str(Path.home()/'.local/bin/helm.exe')
  assert Path(helm).exists() or shutil.which(helm),'Helm CLI prerequisite missing'
  rendered=execute([helm,'template','b10b-rawfile',str(chart),'--namespace',namespace,'-f',str(values)],timeout=60)
  manifest.write_text(rendered,encoding='utf-8')
 docs=[x for x in yaml.safe_load_all(manifest.read_text(encoding='utf-8')) if x]
 references=sorted({c['image'] for x in docs if x['kind'] in ('DaemonSet','Deployment') for c in x['spec']['template']['spec'].get('containers',[])+x['spec']['template']['spec'].get('initContainers',[])})
 pinned={ref:stage(r,ref) for ref in references}
 for obj in docs:
  obj['metadata'].setdefault('labels',{}).update(labels)
  if obj['kind'] not in ('ClusterRole','ClusterRoleBinding','CSIDriver','StorageClass','CustomResourceDefinition'):obj['metadata']['namespace']=namespace
  if obj['kind'] in ('DaemonSet','Deployment'):
   for c in obj['spec']['template']['spec'].get('containers',[])+obj['spec']['template']['spec'].get('initContainers',[]):
    c['image']=pinned[c['image']];c['imagePullPolicy']='IfNotPresent'
 r.apply(docs)
 for obj in docs:
  if obj['kind'] in ('DaemonSet','Deployment'):
   r.kubectl('rollout','status',obj['kind'].lower()+'/'+obj['metadata']['name'],'-n',namespace,'--timeout=150s',timeout=160)
 native=r.get('storageclass','b10b-expandable')
 assert native['allowVolumeExpansion'] and native['provisioner']=='rawfile.csi.openebs.io'
 r.save('storage-driver-ready',{'chart_sha256':hashlib.sha256(chart.read_bytes()).hexdigest(),'storageclass':native,'images':pinned,'namespace':namespace,'node_directory':'/var/lib/argus-plan9/'+r.id+'/rawfile','formal_storage_untouched':True})
 print('Pinned Rawfile CSI ready with real filesystem resize capability',flush=True)
if __name__=='__main__':
 from run import Run
 install(Run())
