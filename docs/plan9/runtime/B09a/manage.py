"""Owned Kubernetes infrastructure and normal workload processes for Plan9 B09a."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import sys
import time

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[3]
sys.path.insert(0, str(HERE.parent))
from B01.manage import execute, now, write_json

RUN = 'p9-b09a-20261108-01'
PREDECESSOR = 'p9-b08-20261105-01'
APP_NS = 'b09a-workloads'
API_SERVER = 'https://host.docker.internal:22443'
NODE_IMAGE = 'kindest/node:v1.36.1@sha256:3489c7674813ba5d8b1a9977baea8a6e553784dab7b84759d1014dbd78f7ebd5'
APP_IMAGE = 'argus-plan9-k8s:b09a-20261108'


class Run:
    def __init__(self):
        self.id = RUN
        self.out = ROOT/'artifacts/plan9'/RUN
        self.private = self.out/'private'
        self.resources = self.out/'resource-map.json'
        self.kubeconfig = self.private/'admin-kubeconfig.yaml'
        self.kind = self.private/'kind.exe'
        self.context = 'kind-'+RUN

    def event(self, kind, **values):
        with (self.out/'timeline.jsonl').open('a',encoding='utf-8') as stream:
            stream.write(json.dumps({'time':now(),'event':kind,**values})+'\n')

    def kubectl(self, *args, data=None, timeout=120):
        node=json.loads(execute(['docker','inspect',self.id+'-control-plane']))[0]
        assert node['Config']['Labels']['io.x-k8s.kind.cluster']==self.id
        saved=self.out/'cluster-infrastructure.json'
        if saved.exists():
            assert node['Id']==json.loads(saved.read_text(encoding='utf-8'))['container_id']
        return execute(['docker','exec','-i',node['Id'],'kubectl',
                       '--kubeconfig=/etc/kubernetes/admin.conf',*args],data=data,timeout=timeout)

    def get(self, kind, *args):
        return json.loads(self.kubectl('get',kind,*args,'-o','json'))

    def apply(self, objects):
        return self.kubectl('apply','-f','-',data=json.dumps({'apiVersion':'v1','kind':'List','items':objects}))

    def platform(self):
        result = {}
        for kind in ('deployments','pvc'):
            rows=json.loads(execute(['kubectl','--context','docker-desktop','get',kind,'-n','argus-system','-o','json']))['items']
            result[kind]={r['metadata']['name']:{
                'uid':r['metadata']['uid'],'generation':r['metadata'].get('generation'),
                'volume':r.get('spec',{}).get('volumeName'),
                'images':[c['image'] for c in r.get('spec',{}).get('template',{}).get('spec',{}).get('containers',[])]
            } for r in rows}
        return result

    def prepare(self):
        self.private.mkdir(parents=True,exist_ok=True)
        if os.name=='nt':
            execute(['icacls',str(self.private),'/inheritance:r','/grant:r',
                     execute(['whoami'])+':(OI)(CI)F','*S-1-5-18:(OI)(CI)F'])
        else:
            self.private.chmod(0o700)
        if not self.kind.exists():
            import requests
            url='https://github.com/kubernetes-sigs/kind/releases/download/v0.32.0/kind-windows-amd64'
            binary=requests.get(url,timeout=90)
            binary.raise_for_status()
            checksum=requests.get(url+'.sha256sum',timeout=30)
            checksum.raise_for_status()
            actual=hashlib.sha256(binary.content).hexdigest()
            assert actual==checksum.text.split()[0]
            self.kind.write_bytes(binary.content)
            write_json(self.out/'kind-tool.json',{'version':'v0.32.0','url':url,'sha256':actual})
        if not (self.out/'run.json').exists():
            write_json(self.out/'run.json',{'run_id':self.id,'batch':'B09a','created_at':now(),
                'authorization':'user_requested_batch_transition','status':'preparing',
                'limits':{'node_cpus':3,'node_memory_mib':3072,'http_request_interval_seconds':5,
                          'background_hours':8,'fault_seconds':120}})
            write_json(self.out/'platform-before.json',self.platform())
        if not self.resources.exists():
            write_json(self.resources,{'run_id':self.id,'cluster':{},'namespaces':{},
                'application':{},'log_rules':{},'dashboards':{},'alert_policies':{}})

    def cluster(self):
        previous=self.out.parent/PREDECESSOR
        assert json.loads((previous/'cleanup.json').read_text(encoding='utf-8'))['status']=='destroyed'
        assert (previous/'cleanup-verification.json').is_file()
        clusters=execute([str(self.kind),'get','clusters']).splitlines()
        if self.id not in clusters:
            with (self.out/'kind-create.log').open('wb') as output:
                import subprocess
                subprocess.run([str(self.kind),'create','cluster','--name',self.id,'--config',str(HERE/'kind.yaml'),
                    '--image',NODE_IMAGE,'--kubeconfig',str(self.kubeconfig),'--wait','180s','--retain'],
                    cwd=ROOT,stdout=output,stderr=subprocess.STDOUT,check=True,timeout=300)
        import yaml
        configuration=yaml.safe_load(self.kubeconfig.read_text(encoding='utf-8'))
        configuration['clusters'][0]['cluster'].update(server='https://127.0.0.1:22443',
            **{'tls-server-name':'host.docker.internal'})
        self.kubeconfig.write_text(yaml.safe_dump(configuration),encoding='utf-8')
        node=self.id+'-control-plane'
        details=json.loads(execute(['docker','inspect',node]))[0]
        assert details['Config']['Labels']['io.x-k8s.kind.cluster']==self.id
        execute(['docker','update','--cpus','3','--memory','3072m','--memory-swap','3072m',node])
        self.kubectl('wait','--for=condition=Ready','nodes','--all','--timeout=150s',timeout=160)
        write_json(self.out/'cluster-infrastructure.json',{'container_id':details['Id'],
            'node_image':NODE_IMAGE,'kind_version':execute([str(self.kind),'version']),
            'nodes':self.get('nodes'),'context':self.context,'api_server':API_SERVER})
        self.event('independent_cluster_ready',node=node)

    def build(self):
        execute(['docker','build','-t',APP_IMAGE,str(HERE)],timeout=180)
        image=json.loads(execute(['docker','image','inspect',APP_IMAGE]))[0]
        write_json(self.out/'workload-image.json',{'id':image['Id'],'tag':APP_IMAGE,
            'files':{p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in HERE.iterdir() if p.is_file()}})
        execute([str(self.kind),'load','docker-image',APP_IMAGE,'--name',self.id],timeout=180)

    def workloads(self):
        from workload_manifest import objects
        manifest=objects()
        write_json(self.out/'workloads.json',manifest)
        self.apply(manifest)
        for workload in ('deployment/orders','statefulset/ledger','daemonset/node-agent'):
            self.kubectl('rollout','status',workload,'-n',APP_NS,'--timeout=120s',timeout=130)
        self.kubectl('wait','--for=condition=complete','job/receipt','-n',APP_NS,'--timeout=90s',timeout=100)
        write_json(self.out/'workload-identities.json',self.get('deploy,sts,ds,job,svc,pvc,pod','-n',APP_NS))
        self.event('real_workloads_ready')

    def request(self, label, pod=None):
        import re
        assert re.fullmatch('[a-z][a-z0-9-]{0,55}',label)
        target='pod/'+pod if pod else 'deployment/driver'
        value=self.kubectl('exec','-n',APP_NS,target,'--','python3','/app/workload.py','request','--label',label)
        return json.loads(value)


if __name__=='__main__':
    parser=argparse.ArgumentParser()
    parser.add_argument('action',choices=['prepare','cluster','build','workloads'])
    getattr(Run(),parser.parse_args().action)()
