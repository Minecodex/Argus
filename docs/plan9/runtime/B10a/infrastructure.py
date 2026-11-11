"""Owned B10a cluster; shares the established Kind and enrollment primitives."""
import json,os,re,shutil,subprocess,sys
from pathlib import Path
import yaml
HERE=Path(__file__).resolve().parent
ROOT=HERE.parents[3]
sys.path.insert(0,str(HERE.parent))
from B09a.manage import Run as KindRun,NODE_IMAGE,execute,write_json
RUN='p9-b10a-20261111-01'
PREDECESSOR='p9-b09b-20261110-01'
NAMESPACE='b10a-workloads'
IMAGE='argus-plan9-workbench:b10a-20261111'
class Infrastructure(KindRun):
 batch='B10a'
 app_namespace=NAMESPACE
 cluster_name='B10a Kubernetes Workbench'
 def __init__(self,run_id=RUN):
  if not re.fullmatch(r'p9-b10a-\d{8}-\d{2}',run_id):raise ValueError('Invalid B10a run')
  self.id=run_id;self.out=ROOT/'artifacts/plan9'/run_id;self.private=self.out/'private'
  self.resources=self.out/'resource-map.json';self.kubeconfig=self.private/'admin-kubeconfig.yaml'
  self.kind=self.private/'kind.exe';self.context='kind-'+run_id
 def prepare(self):
  self.private.mkdir(parents=True,exist_ok=True)
  old=ROOT/'artifacts/plan9'/PREDECESSOR
  if not self.kind.exists():shutil.copy2(old/'private/kind.exe',self.kind)
  super().prepare()
  record=json.loads((self.out/'run.json').read_text(encoding="utf-8"));record.update(batch=self.batch,predecessor=PREDECESSOR,authorization='user_requested_next_batch_B10a')
  write_json(self.out/'run.json',record)
 def cluster(self):
  old=ROOT/'artifacts/plan9'/PREDECESSOR
  assert json.loads((old/'cleanup.json').read_text(encoding="utf-8"))['status']=='destroyed'
  assert (old/'cleanup-verification.json').exists()
  if self.id not in execute([str(self.kind),'get','clusters']).splitlines():
   spec=yaml.safe_load((HERE.parent/'B09a/kind.yaml').read_text(encoding="utf-8"));spec['name']=self.id
   spec['networking']['apiServerAddress']='127.0.0.1';spec['nodes'][0]['labels']={'argus.plan9/run':self.id}
   path=self.out/'kind.yaml';path.write_text(yaml.safe_dump(spec,sort_keys=False))
   with (self.out/'kind-create.log').open('wb') as log:
    subprocess.run([str(self.kind),'create','cluster','--name',self.id,'--config',str(path),'--image',NODE_IMAGE,'--kubeconfig',str(self.kubeconfig),'--wait','180s','--retain'],stdout=log,stderr=subprocess.STDOUT,check=True,timeout=300)
  node=json.loads(execute(['docker','inspect',self.id+'-control-plane']))[0]
  assert node['Config']['Labels']['io.x-k8s.kind.cluster']==self.id
  execute(['docker','update','--cpus','3','--memory','3072m','--memory-swap','3072m',node['Id']])
  self.kubectl('wait','--for=condition=Ready','nodes','--all','--timeout=150s',timeout=160)
  write_json(self.out/'cluster-infrastructure.json',{'container_id':node['Id'],'node_image':NODE_IMAGE,'nodes':self.get('nodes'),'context':self.context})
  self.apply([{'apiVersion':'v1','kind':'Namespace','metadata':{'name':NAMESPACE,'labels':{'argus.plan9/run':self.id}}}])
  self.event('fresh_B10a_cluster_ready')
 def images(self):
  execute(['docker','build','-t',IMAGE,str(HERE)],timeout=180)
  write_json(self.out/'workload-image.json',{'image':IMAGE,'id':json.loads(execute(['docker','image','inspect',IMAGE]))[0]['Id']})
  execute([str(self.kind),'load','docker-image',IMAGE,'--name',self.id],timeout=180)
 def bootstrap(self):
  sys.path.insert(0,str(HERE.parent/'B09a'))
  from B09a.bootstrap import bootstrap
  bootstrap(self)
  from B10a.bootstrap import workbench_rbac
  workbench_rbac(self)
 def foundation(self,session):
  sys.path.insert(0,str(HERE.parent/'B09a'))
  from B09a.integration import Batch
  from common.api import Api
  class Foundation(Batch):
   def inventory(self):
    value=self.api.wait('/enterprise/kubernetes-clusters/'+self.cluster+'/inventory?type=namespace&limit=100',lambda x:any(y['name']==NAMESPACE for y in x['items']),timeout=120,label='B10a namespace')
    row=next(y for y in value['items'] if y['name']==NAMESPACE);native=self.run.get('namespace',NAMESPACE)
    assert row['uid']==native['metadata']['uid']
    self.map['namespaces'][NAMESPACE]={'name':NAMESPACE,'uid':row['uid'],'id':value['namespace_references'][row['uid']]};self.save()
  api=Api(self,session,ROOT/'artifacts/plan9/reset-20261026/trust-bundle.pem')
  batch=Foundation(self,api);batch.enroll();batch.install();batch.builtins()
  return batch
