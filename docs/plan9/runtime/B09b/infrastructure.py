"""Fresh B09b infrastructure built with the shared, ownership-checked Kind runtime."""
import json
import re
import shutil
import subprocess
import sys

from B09a.manage import Run as KindRun, ROOT, NODE_IMAGE, APP_IMAGE, execute, write_json

RUN = 'p9-b09b-20261110-01'
PREDECESSOR = 'p9-b09a-20261108-01'
NAMESPACE = 'b09b-components'


class Infrastructure(KindRun):
    batch = 'B09b'
    app_namespace = NAMESPACE
    cluster_name = 'B09b Kubernetes Control Plane'

    def __init__(self, run_id=RUN):
        if not re.fullmatch(r'p9-b09b-\d{8}-\d{2}', run_id):
            raise ValueError('Invalid B09b run ID')
        self.id = run_id
        self.out = ROOT/'artifacts/plan9'/run_id
        self.private = self.out/'private'
        self.resources = self.out/'resource-map.json'
        self.kubeconfig = self.private/'admin-kubeconfig.yaml'
        self.kind = self.private/'kind.exe'
        self.context = 'kind-'+run_id

    def prepare(self):
        old = ROOT/'artifacts/plan9'/PREDECESSOR
        self.private.mkdir(parents=True,exist_ok=True)
        if not self.kind.exists() and (old/'private/kind.exe').exists():
            shutil.copy2(old/'private/kind.exe',self.kind)
        super().prepare()
        record=json.loads((self.out/'run.json').read_text(encoding='utf-8'))
        record.update(batch=self.batch,predecessor=PREDECESSOR,
            authorization='user_requested_cleanup_B09a_and_fresh_B09b')
        write_json(self.out/'run.json',record)

    def cluster(self):
        import yaml
        old=ROOT/'artifacts/plan9'/PREDECESSOR
        assert json.loads((old/'cleanup.json').read_text(encoding='utf-8'))['status']=='destroyed'
        assert (old/'cleanup-verification.json').is_file()
        clusters=execute([str(self.kind),'get','clusters']).splitlines()
        if self.id not in clusters:
            template=yaml.safe_load((ROOT/'docs/plan9/runtime/B09a/kind.yaml').read_text(encoding='utf-8'))
            template['name']=self.id
            template['nodes'][0]['labels']={'argus.plan9/run':self.id}
            template['networking']['apiServerAddress']='127.0.0.1'
            spec=self.out/'kind.yaml'
            spec.write_text(yaml.safe_dump(template,sort_keys=False),encoding='utf-8')
            with (self.out/'kind-create.log').open('wb') as output:
                subprocess.run([str(self.kind),'create','cluster','--name',self.id,'--config',str(spec),
                    '--image',NODE_IMAGE,'--kubeconfig',str(self.kubeconfig),'--wait','180s','--retain'],
                    cwd=ROOT,stdout=output,stderr=subprocess.STDOUT,check=True,timeout=300)
        node=self.id+'-control-plane'
        current=json.loads(execute(['docker','inspect',node]))[0]
        assert current['Config']['Labels']['io.x-k8s.kind.cluster']==self.id
        execute(['docker','update','--cpus','3','--memory','3072m','--memory-swap','3072m',node])
        self.kubectl('wait','--for=condition=Ready','nodes','--all','--timeout=150s',timeout=160)
        write_json(self.out/'cluster-infrastructure.json',{'container_id':current['Id'],
            'node_image':NODE_IMAGE,'kind_version':execute([str(self.kind),'version']),
            'nodes':self.get('nodes'),'context':self.context})
        self.apply([{'apiVersion':'v1','kind':'Namespace','metadata':{
            'name':NAMESPACE,'labels':{'argus.plan9/run':self.id}}}])
        image=json.loads(execute(['docker','image','inspect',APP_IMAGE]))[0]
        write_json(self.out/'activity-image.json',{'image':APP_IMAGE,'id':image['Id']})
        execute([str(self.kind),'load','docker-image',APP_IMAGE,'--name',self.id],timeout=180)
        self.event('fresh_B09b_cluster_ready')

    def bootstrap(self):
        sys.path.insert(0,str(ROOT/'docs/plan9/runtime/B09a'))
        from B09a.bootstrap import bootstrap
        bootstrap(self)

    def foundation(self, session):
        sys.path.insert(0,str(ROOT/'docs/plan9/runtime/B09a'))
        from B09a.integration import Batch
        from common.api import Api

        class Foundation(Batch):
            def inventory(self):
                base='/enterprise/kubernetes-clusters/'+self.cluster+'/inventory?type=namespace&limit=100'
                value=self.api.wait(base,lambda value:any(row['name']==NAMESPACE for row in value['items']),
                                    timeout=120,label='New B09b namespace')
                row=next(row for row in value['items'] if row['name']==NAMESPACE)
                actual=self.run.get('namespace',NAMESPACE)
                assert row['uid']==actual['metadata']['uid']
                self.map['namespaces'][NAMESPACE]={'id':value['namespace_references'][row['uid']],
                    'uid':row['uid'],'name':NAMESPACE}
                self.save()

        api=Api(self,session,ROOT/'artifacts/plan9/reset-20261026/trust-bundle.pem')
        batch=Foundation(self,api)
        batch.enroll()
        batch.install()
        batch.builtins()
        return batch
