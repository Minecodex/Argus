"""Owned B10b infrastructure; shared Kind/enrollment and normal API lifecycle."""
import json,os,re,shutil,subprocess,sys
from pathlib import Path
import yaml
HERE=Path(__file__).resolve().parent
ROOT=HERE.parents[3]
sys.path.insert(0,str(HERE.parent))
from B09a.manage import Run as KindRun,NODE_IMAGE,execute,write_json
RUN='p9-b10b-20261112-01'
PREDECESSOR='p9-b10a-20261111-01'
NAMESPACE='b10b-workloads'
IMAGE='argus-plan9-storage-jobs:b10b-20261112'

class Infrastructure(KindRun):
 batch='B10b'
 app_namespace=NAMESPACE
 cluster_name='B10b Kubernetes Storage and Jobs'
 def __init__(self,run_id=RUN):
  if not re.fullmatch(r'p9-b10b-\d{8}-\d{2}',run_id):raise ValueError('Invalid B10b run')
  self.id=run_id;self.out=ROOT/'artifacts/plan9'/run_id;self.private=self.out/'private'
  self.resources=self.out/'resource-map.json';self.kubeconfig=self.private/'admin-kubeconfig.yaml'
  self.kind=self.private/'kind.exe';self.context='kind-'+run_id
 def prepare(self):
  self.private.mkdir(parents=True,exist_ok=True)
  old=ROOT/'artifacts/plan9'/PREDECESSOR
  if not self.kind.exists() and (old/'private/kind.exe').exists():shutil.copy2(old/'private/kind.exe',self.kind)
  super().prepare()
  record=json.loads((self.out/'run.json').read_text(encoding='utf-8'))
  record.update(batch=self.batch,predecessor=PREDECESSOR,authorization='user_requested_next_wave_B10b',limits={'node_cpus':3,'node_memory_mib':3584,'background_hours':8,'max_workload_replicas':3,'fault_seconds':240})
  write_json(self.out/'run.json',record)
 @property
 def session(self):return self.private/'enterprise-session.json'
 def api(self):
  from common.api import Api
  return Api(self,self.session,ROOT/'artifacts/plan9/reset-20261026/trust-bundle.pem')
 @property
 def cluster_id(self):return json.loads(self.resources.read_text(encoding='utf-8'))['cluster']['id']
 def save(self,name,value):write_json(self.out/(name+'.json'),value)
 def cluster(self):
  old=ROOT/'artifacts/plan9'/PREDECESSOR
  assert json.loads((old/'cleanup-verification.json').read_text(encoding='utf-8'))['status']=='destroyed'
  if self.id not in execute([str(self.kind),'get','clusters']).splitlines():
   spec=yaml.safe_load((HERE.parent/'B09a/kind.yaml').read_text(encoding='utf-8'))
   spec['name']=self.id;spec['networking'].update(apiServerAddress='127.0.0.1',disableDefaultCNI=True,podSubnet='192.168.0.0/16')
   spec['nodes'][0]['labels']={'argus.plan9/run':self.id}
   spec['kubeadmConfigPatches'].append('kind: KubeletConfiguration\nserverTLSBootstrap: true\n')
   path=self.out/'kind.yaml';path.write_text(yaml.safe_dump(spec,sort_keys=False),encoding='utf-8')
   with (self.out/'kind-create.log').open('wb') as log:
    subprocess.run([str(self.kind),'create','cluster','--name',self.id,'--config',str(path),'--image',NODE_IMAGE,'--kubeconfig',str(self.kubeconfig),'--wait','0s','--retain'],stdout=log,stderr=subprocess.STDOUT,check=True,timeout=300)
  node=json.loads(execute(['docker','inspect',self.id+'-control-plane']))[0]
  assert node['Config']['Labels']['io.x-k8s.kind.cluster']==self.id
  execute(['docker','update','--cpus','3','--memory','3584m','--memory-swap','3584m',node['Id']])
  self.save('cluster-infrastructure',{'container_id':node['Id'],'node_image':NODE_IMAGE,'context':self.context,'default_cni_disabled':True})
  self.apply([{'apiVersion':'v1','kind':'Namespace','metadata':{'name':NAMESPACE,'labels':{'argus.plan9/run':self.id}}}])
  self.event('fresh_B10b_cluster_api_ready')
 def bootstrap(self):
  sys.path.insert(0,str(HERE.parent/'B09a'))
  from B09a.bootstrap import bootstrap
  bootstrap(self)
  from B10b.rbac import workbench_rbac
  workbench_rbac(self)
 def foundation(self):
  sys.path.insert(0,str(HERE.parent/'B09a'))
  from B09a.integration import Batch
  class Foundation(Batch):
   def inventory(b):
    path='/enterprise/kubernetes-clusters/'+b.cluster+'/inventory?type=namespace&limit=100'
    page=b.api.wait(path,lambda x:any(row['name']==NAMESPACE for row in x['items']),timeout=120,label='B10b namespace')
    row=next(x for x in page['items'] if x['name']==NAMESPACE)
    b.map['namespaces'][NAMESPACE]={'name':NAMESPACE,'uid':row['uid'],'id':page['namespace_references'][row['uid']]};b.save()
  b=Foundation(self,self.api());b.enroll()
  base='/enterprise/kubernetes-clusters/'+b.cluster+'/collector'
  current=b.api.call('GET',base,expected=[200,204,404])
  if current and current.get('status')=='degraded' and current.get('effective_revision',0)>0:
   b.api.confirm(b.api.call('POST',base+'/actions/preview-repair',{'distribution_version_id':current['distribution_version_id'],'profile_ids':[],'route_kind':'direct_argus','transport':'direct','expected_version':current['version']},key='repair-after-kubelet-tls-'+str(current['version']),expected=201))
  b.install()
  from B10b.collector import builtins
  builtins(self,b)
