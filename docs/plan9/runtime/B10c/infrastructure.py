"""Owned B10c infrastructure and normal Argus API access."""
import base64
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time

import yaml

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[3]
sys.path.insert(0, str(HERE.parent))
from B09a.manage import Run as KindRun, NODE_IMAGE, execute, write_json
from common.api import Api

RUN = 'p9-b10c-20261113-01'
PREDECESSOR = 'p9-b10b-20261112-01'
NAMESPACE = 'b10c-workloads'
BASE_IMAGE = 'argus-plan9-workbench:b10a-20261111'


class Run(KindRun):
    batch = 'B10c'
    app_namespace = NAMESPACE
    cluster_name = 'B10c Helm Extensions and Node Maintenance'

    def __init__(self):
        self.id = RUN
        self.out = Path(os.environ.get('ARGUS_PLAN9_ARTIFACT_ROOT', ROOT/'artifacts/plan9'))/RUN
        self.private = self.out/'private'
        self.resources = self.out/'resource-map.json'
        self.kubeconfig = self.private/'admin-kubeconfig.yaml'
        self.kind = self.private/'kind.exe'
        self.context = 'kind-'+RUN

    @property
    def cluster_id(self):
        return json.loads(self.resources.read_text(encoding='utf-8'))['cluster']['id']

    @property
    def session(self):
        return self.private/'enterprise-session.json'

    @property
    def image(self):
        return json.loads((self.out/'source-state.json').read_text(encoding='utf-8'))['workload_image']

    def source_addresses(self):
        return json.loads((self.out/'source-state.json').read_text(encoding='utf-8'))['addresses']

    def save(self, name, value):
        write_json(self.out/(name+'.json'), value)

    def prepare(self):
        self.private.mkdir(parents=True, exist_ok=True)
        cached = ROOT/'artifacts/plan9'/PREDECESSOR/'private/kind.exe'
        if not self.kind.exists() and cached.exists():
            shutil.copy2(cached, self.kind)
        super().prepare()
        state = json.loads((self.out/'run.json').read_text(encoding='utf-8'))
        state.update(batch=self.batch, predecessor=PREDECESSOR,
                     authorization='user_requested_next_plan9_batch_20261113',
                     limits={'nodes': 3, 'control_plane_memory_mib': 3072,
                             'worker_memory_mib': 2048, 'background_hours': 8,
                             'fault_seconds': 240, 'chart_max_bytes': 393216})
        self.save('run', state)
        if not self.session.exists():
            self.session.write_text(json.dumps({'origin':'https://argus.dev','csrf':'','user':{},'cookies':[]}), encoding='utf-8')

    def api(self):
        credentials = None
        if os.environ.get('ARGUS_PLAN9_PASSWORD'):
            credentials = {'enterprise_username':os.environ.get('ARGUS_PLAN9_USERNAME','kakj'),
                           'password':os.environ['ARGUS_PLAN9_PASSWORD']}
        return Api(self, self.session, ROOT/'artifacts/plan9/reset-20261026/trust-bundle.pem', credentials)

    def preflight(self):
        a = self.api()
        a.call('GET', '/enterprise/auth/session')
        native = json.loads(execute(['docker','inspect',PREDECESSOR+'-control-plane']))[0]
        old = ROOT/'artifacts/plan9'/PREDECESSOR
        mapping = json.loads((old/'resource-map.json').read_text(encoding='utf-8'))
        original = json.loads((old/'cluster-infrastructure.json').read_text(encoding='utf-8'))
        assert native['Id'] == original['container_id']
        assert native['Config']['Labels']['io.x-k8s.kind.cluster'] == PREDECESSOR
        cluster = a.call('GET', '/enterprise/kubernetes-clusters/'+mapping['cluster']['id'])
        assert cluster['labels']['plan9_run'] == PREDECESSOR
        formal = self.platform()
        assert len(formal['pvc']) == 6
        self.save('predecessor-before', {'resource_map':mapping,'container_id':native['Id'], 'formal':formal})
        self.save('preflight', {'run_id':self.id,'status':'verified','predecessor':PREDECESSOR,
                               'formal_pvc_count':6,'node_image':NODE_IMAGE,
                               'artifact_directory':str(self.out),'helm':execute(['helm','version','--short']),
                               'compose':execute(['docker','compose','version','--short'])})
        print('Normal login, owned B10b identity and six formal PVCs verified', flush=True)

    def cluster(self):
        old = ROOT/'artifacts/plan9'/PREDECESSOR
        assert json.loads((old/'cleanup-verification.json').read_text(encoding='utf-8'))['status'] == 'destroyed'
        if self.id not in execute([str(self.kind),'get','clusters']).splitlines():
            labels = {'argus.plan9/run':self.id}
            config = {'kind':'Cluster','apiVersion':'kind.x-k8s.io/v1alpha4','name':self.id,
                      'networking':{'apiServerAddress':'127.0.0.1'},
                      'nodes':[{'role':'control-plane','labels':labels},
                               {'role':'worker','labels':{**labels,'argus.plan9/role':'worker'}},
                               {'role':'worker','labels':{**labels,'argus.plan9/role':'worker'}}],
                      'kubeadmConfigPatches':[
                          'kind: KubeletConfiguration\nserverTLSBootstrap: true\n',
                          'kind: ClusterConfiguration\napiServer:\n  certSANs: [host.docker.internal, localhost]\n  extraArgs:\n  - name: service-account-max-token-expiration\n    value: 168h\n']}
            path = self.out/'kind.yaml'
            path.write_text(yaml.safe_dump(config, sort_keys=False), encoding='utf-8')
            with (self.out/'kind-create.log').open('wb') as log:
                subprocess.run([str(self.kind),'create','cluster','--name',self.id,'--config',str(path),
                                '--image',NODE_IMAGE,'--kubeconfig',str(self.kubeconfig),'--wait','180s','--retain'],
                               stdout=log, stderr=subprocess.STDOUT, check=True, timeout=360)
        nodes = []
        for suffix in ('control-plane','worker','worker2'):
            node = json.loads(execute(['docker','inspect',self.id+'-'+suffix]))[0]
            assert node['Config']['Labels']['io.x-k8s.kind.cluster'] == self.id
            memory = '3072m' if suffix == 'control-plane' else '2048m'
            execute(['docker','update','--cpus','2','--memory',memory,'--memory-swap',memory,node['Id']])
            nodes.append({'name':self.id+'-'+suffix,'container_id':node['Id']})
        self.save('cluster-infrastructure', {'container_id':nodes[0]['container_id'], 'nodes':nodes,
                                            'node_image':NODE_IMAGE,'context':self.context})
        self.apply([{'apiVersion':'v1','kind':'Namespace','metadata':{'name':NAMESPACE,'labels':{'argus.plan9/run':self.id}}}])
        self.kubectl('wait','--for=condition=Ready','nodes','--all','--timeout=120s')
        self.event('fresh_three_node_cluster_ready')

    def bootstrap(self):
        sys.path.insert(0, str(HERE.parent/'B09a'))
        from B09a.bootstrap import bootstrap
        bootstrap(self)
        hosts = 'server = "http://host.docker.internal:5001"\n[host."http://host.docker.internal:5001"]\n  capabilities = ["pull", "resolve"]\n'
        for node in self.get('nodes')['items']:
            name = node['metadata']['name']
            assert name.startswith(self.id+'-')
            execute(['docker','exec','-i',name,'sh','-ec',
                     'mkdir -p /etc/containerd/certs.d/host.docker.internal:5001; cat > /etc/containerd/certs.d/host.docker.internal:5001/hosts.toml'],data=hosts)
        config_path = self.private/'platform-kubeconfig.yaml'
        config = yaml.safe_load(config_path.read_text(encoding='utf-8'))
        token = self.kubectl('create','token','argus-kubernetes-connector','-n','plan9-access','--duration=168h')
        config['users'][0]['user']['token'] = token
        config_path.write_text(yaml.safe_dump(config), encoding='utf-8')
        claims = json.loads(base64.urlsafe_b64decode(token.split('.')[1]+'==='))
        assert claims['exp']-claims['iat'] >= 6*86400
        self.save('credential-expiry', {'issued_at':claims['iat'],'expires_at':claims['exp'], 'native_permissions_unchanged':True})
        expected_nodes = {node['metadata']['name'] for node in self.get('nodes')['items']}
        for _ in range(45):
            pending = self.get('csr')['items']
            for csr in pending:
                if csr['spec'].get('signerName') == 'kubernetes.io/kubelet-serving' and not csr.get('status',{}).get('certificate'):
                    assert csr['spec']['username'].startswith('system:node:'+self.id+'-')
                    self.kubectl('certificate','approve',csr['metadata']['name'])
            certified = {csr['spec']['username'].removeprefix('system:node:') for csr in self.get('csr')['items']
                         if csr['spec'].get('signerName')=='kubernetes.io/kubelet-serving' and csr.get('status',{}).get('certificate')}
            if expected_nodes.issubset(certified):
                self.save('kubelet-serving-certificates', {'verified_nodes':sorted(expected_nodes),'trusted_cluster_ca':True})
                break
            time.sleep(2)
        else:
            raise RuntimeError('Owned kubelet serving certificates did not converge')
        dns = self.get('deployment','coredns','-n','kube-system')
        dns['spec']['template']['spec']['nodeSelector'] = {'kubernetes.io/hostname':self.id+'-control-plane'}
        self.apply([dns])
        self.kubectl('rollout','status','deployment/coredns','-n','kube-system','--timeout=90s')
        # Keep the durable Collector gateway and its local state volume away
        # from the maintenance worker. This is owned-cluster bootstrap only.
        if any(t['key']=='node-role.kubernetes.io/control-plane' for t in self.get('node',self.id+'-control-plane')['spec'].get('taints',[])):
            self.kubectl('taint','nodes',self.id+'-control-plane','node-role.kubernetes.io/control-plane:NoSchedule-')
        self.kubectl('cordon',self.id+'-worker',self.id+'-worker2')
        from permissions import install
        install(self)

    def foundation(self):
        sys.path.insert(0, str(HERE.parent/'B09a'))
        from B09a.integration import Batch
        from B10b.collector import builtins
        class Foundation(Batch):
            @property
            def namespace(b):
                return b.map['namespaces'][NAMESPACE]['id']
            def inventory(b):
                page = b.api.wait('/enterprise/kubernetes-clusters/'+b.cluster+'/inventory?type=namespace&limit=100',
                                  lambda x:any(v['name']==NAMESPACE for v in x['items']), timeout=90, label='B10c namespace')
                row = next(v for v in page['items'] if v['name']==NAMESPACE)
                b.map['namespaces'][NAMESPACE] = {'name':NAMESPACE,'uid':row['uid'],'id':page['namespace_references'][row['uid']]}
                b.save()
        b = Foundation(self,self.api())
        b.enroll()
        b.install()
        builtins(self,b)
        gateway = self.get('pods','-n','argus-telemetry','-l','app.kubernetes.io/component=gateway')['items']
        if not gateway:
            gateway = [p for p in self.get('pods','-n','argus-telemetry')['items'] if p['metadata']['name'].startswith('argus-otelcol-gateway-')]
        assert gateway and all(p['spec']['nodeName']==self.id+'-control-plane' for p in gateway)
        self.kubectl('uncordon',self.id+'-worker',self.id+'-worker2')
        from sources import load_nodes
        load_nodes(self)

    def namespace(self, api):
        return json.loads(self.resources.read_text(encoding='utf-8'))['namespaces'][NAMESPACE]['id']

    def mutate(self, api, body, label, wait=True):
        definitions = api.call('GET','/enterprise/kubernetes-clusters/'+self.cluster_id+'/inventory/types')['items']
        descriptor = next(v for v in definitions if v['key']==body['type'])
        if descriptor['namespaced']:
            body = {'namespace_id':self.namespace(api), **body}
        preview = api.call('POST','/enterprise/kubernetes-clusters/'+self.cluster_id+'/inventory/actions/preview-mutation',
                           body, key=label, expected=201)
        submitted = api.confirm(preview,wait=False)
        self.save(label+'-submitted', submitted)
        operation = self.operation_ref(api,submitted)
        return self.wait_operation(api, operation, label) if wait else operation

    def operation_ref(self, api, submitted):
        direct = submitted.get('resource_ref') or {}
        if direct.get('resource_type')=='kubernetes_operation':
            return direct['resource_id']
        execution = submitted.get('execution') or {}
        ident = execution.get('execution_id')
        assert ident, 'Confirmation must identify its actual execution'
        def attached(value):
            if (value.get('operation_ref') or {}).get('kind')=='kubernetes_operation':
                return True
            if value['status'] in ('failed','cancelled','expired','result_unknown'):
                raise RuntimeError('Dispatch '+value['status']+' '+str(value.get('error_code')))
            return False
        state = api.wait('/enterprise/executions/'+ident,attached,timeout=90,label='Operation reference attached')
        return state['operation_ref']['id']

    def wait_operation(self, api, operation, label, timeout=240, expected=('succeeded','applied')):
        def done(value):
            if value['status'] in ('failed','conflict','cancelled','result_unknown','expired') and value['status'] not in expected:
                self.save(label+'-failed', value)
                raise RuntimeError(label+': '+value['status']+' '+str(value.get('error_code')))
            return value['status'] in expected
        result = api.wait('/enterprise/kubernetes-clusters/'+self.cluster_id+'/operations/'+operation,
                          done, timeout=timeout, label=label)
        self.save(label+'-operation', result)
        return result

    def change(self, api, kind, name, operation, **values):
        native = self.get(kind,name,'-n',NAMESPACE)
        return self.mutate(api, {'type':kind,'name':name,'operation':operation,
                                'uid':native['metadata']['uid'],'expected_resource_version':native['metadata']['resourceVersion'], **values},
                           operation+'-'+name+'-'+native['metadata']['resourceVersion'])
