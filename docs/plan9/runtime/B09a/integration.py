"""Enroll the dedicated B09a cluster and native collectors using normal APIs."""
import argparse
from datetime import datetime,timedelta,timezone
import json
import uuid
import yaml
from urllib.parse import urlencode

from manage import Run,API_SERVER,APP_NS,ROOT,RUN,now,write_json
from common.api import Api

BUDGET={'max_scan_bytes':268435456,'max_rows':20000,'max_samples':2000000,
        'max_series':20000,'timeout_ms':10000,'max_result_bytes':8388608}


def scope(resource,seconds=900):
    end=datetime.now(timezone.utc)
    return {'resource_ids':[resource],'time_range':{'from':(end-timedelta(seconds=seconds)).isoformat(),
            'to':end.isoformat()},'budget':BUDGET}


class Batch:
    def __init__(self,run,api):
        self.run,self.api=run,api
        self.map=json.loads(run.resources.read_text(encoding='utf-8'))

    def save(self):
        write_json(self.run.resources,self.map)

    @property
    def cluster(self):
        return self.map['cluster']['id']

    @property
    def collector(self):
        return self.map['cluster']['collector_id']

    @property
    def namespace(self):
        return self.map['namespaces'][APP_NS]['id']

    def command(self,body,key):
        response=self.api.confirm(self.api.call('POST','/monitoring/commands/preview',body,key=key,expected=201))
        return response['resource_ref']['resource_id']

    def enroll(self):
        state=self.map['cluster']
        configuration=yaml.safe_load((self.run.private/'platform-kubeconfig.yaml').read_text(encoding='utf-8'))
        address=state.get('api_server') or configuration['clusters'][0]['cluster']['server']
        if not state.get('secret_id'):
            secret=self.api.call('POST','/enterprise/secrets',{'name':self.run.id+'-kubeconfig','type':'kubeconfig',
                'value':(self.run.private/'platform-kubeconfig.yaml').read_text(encoding='utf-8')},
                key='cluster-secret',expected=201)
            state['secret_id']=secret['id']
            self.save()
        if not state.get('credential_id'):
            credential=self.api.call('POST','/enterprise/credentials',{'name':self.run.id+'-kubernetes',
                'protocol':'kubernetes','secret_id':state['secret_id']},key='cluster-credential',expected=201)
            state['credential_id']=credential['id']
            self.save()
        if not state.get('id'):
            test=self.api.call('POST','/enterprise/kubernetes-clusters/connection-tests',{
                'api_server':address,'connection_mode':'direct','credential_id':state['credential_id']},
                key='cluster-connection',expected=202)
            def success(value):
                if value['status'] in ('failed','expired','result_unknown'):
                    write_json(self.run.out/'cluster-connection-failed.json',value)
                    raise RuntimeError('Cluster preflight '+str(value.get('error_code')))
                return value['status']=='succeeded'
            tested=self.api.wait('/enterprise/connection-tests/'+test['id'],success,timeout=90)
            write_json(self.run.out/'cluster-connection.json',tested)
            preview=self.api.call('POST','/enterprise/kubernetes-clusters/actions/preview-create',{
                'name':getattr(self.run,'cluster_name','B09a Kubernetes Workloads'),'api_server':address,'connection_mode':'direct',
                'credential_id':state['credential_id'],'connection_test_id':test['id'],
                'default_namespace':getattr(self.run,'app_namespace',APP_NS),'environment':'development',
                'labels':{'plan9_run':self.run.id,'plan9_batch':getattr(self.run,'batch','B09a').lower()}},key='cluster-create',expected=201)
            response=self.api.confirm(preview)
            state.update(id=response['resource_ref']['resource_id'],name=getattr(self.run,'cluster_name','B09a Kubernetes Workloads'),api_server=address)
            self.save()
        print('Cluster enrolled through the normal API',flush=True)
        self.inventory()

    def inventory(self):
        base='/enterprise/kubernetes-clusters/'+self.cluster+'/inventory'
        value=self.api.wait(base+'?type=namespace&limit=100',
            lambda value:any(item['name']==APP_NS for item in value['items']),timeout=120,label='Namespace inventory')
        namespace=next(item for item in value['items'] if item['name']==APP_NS)
        actual=self.run.get('namespace',APP_NS)
        assert namespace['uid']==actual['metadata']['uid']
        self.map['namespaces'][APP_NS]={'id':value['namespace_references'][namespace['uid']],
            'uid':namespace['uid'],'name':APP_NS}
        self.save()
        reports={}
        for kind in ('deployment','statefulset','daemonset','job','service','pod','pvc'):
            path=base+'?'+urlencode({'type':kind,'namespace_mode':'explicit','namespace_ids':self.namespace,'limit':100})
            reports[kind]=self.api.call('GET',path)
            assert reports[kind]['complete'] and reports[kind]['items'],kind
            assert all(x['namespace']==APP_NS for x in reports[kind]['items'])
        assert {x['name'] for x in reports['deployment']['items']}=={'orders','driver'}
        write_json(self.run.out/'inventory-verification.json',reports)
        print('Actual Namespace UID and seven workload/object views verified',flush=True)

    def install(self):
        state=self.map['cluster']
        base='/enterprise/kubernetes-clusters/'+self.cluster+'/collector'
        current=self.api.call('GET',base,expected=[200,204,404])
        if (not current or current.get('code') or
                current['status']=='degraded' and current['effective_revision']==0):
            distributions=self.api.call('GET','/enterprise/telemetry/distributions')
            distribution=next(row for row in distributions if row['support_status']=='supported' and
                any(artifact['platform']=='linux_amd64' for artifact in row['artifacts']))
            preview=self.api.call('POST',base+'/actions/preview-install',{
                'distribution_version_id':distribution['id'],'profile_ids':[],'route_kind':'direct_argus',
                'transport':'direct'},key='native-collector-install-'+str((current or {}).get('version',0)),expected=201)
            self.api.confirm(preview)
        current=self.converge()
        state['collector_id']=current.get('id') or current.get('collector_id')
        self.save()
        write_json(self.run.out/'collector-installed.json',current)
        print('Native Kubernetes Collector installed and converged',flush=True)

    def converge(self):
        base='/enterprise/kubernetes-clusters/'+self.cluster+'/collector'
        def complete(value):
            if value['status'] in ('degraded','result_unknown'):
                write_json(self.run.out/'collector-failed.json',value)
                raise RuntimeError('Collector rollout '+value['status']+' '+str(value.get('last_error_code')))
            return value['status']=='converged' and value['desired_revision']==value['effective_revision']
        return self.api.wait(base,complete,timeout=240,label='Kubernetes Collector')

    def builtins(self):
        collector_path='/enterprise/kubernetes-clusters/'+self.cluster+'/collector'
        current=self.api.call('GET',collector_path)
        if current['status']=='degraded' and current['effective_revision']>0:
            preview=self.api.call('POST',collector_path+'/actions/preview-repair',{
                'distribution_version_id':current['distribution_version_id'],'profile_ids':[],
                'route_kind':'direct_argus','transport':'direct','expected_version':current['version']},
                key='repair-candidate-'+str(current['version']),expected=201)
            self.api.confirm(preview)
        base='/monitoring/builtins?target_type=kubernetes_cluster&target_id='+self.cluster
        for key in ('kubernetes_cluster_metrics','kubernetes_node_metrics','collector_metrics'):
            self.converge()
            row=next(row for row in self.api.call('GET',base) if row['definition']['key']==key)
            assert row['available'],(key,row.get('reason'))
            current=row.get('collection',{})
            if not current.get('desired_enabled'):
                preview=self.api.call('POST','/monitoring/builtins/preview',{
                    'target':{'kind':'kubernetes_cluster','id':self.cluster},'key':key,'enabled':True,
                    'expected_version':current.get('version',0),'config':{'interval':'10s'}},
                    key='enable-'+key,expected=201)
                self.api.confirm(preview)
            self.converge()
            row=next(row for row in self.api.call('GET',base) if row['definition']['key']==key)
            self.map.setdefault('builtins',{})[key]={'id':row['collection']['id']}
            self.save()
        for key,state in self.map['builtins'].items():
            sample=self.api.wait('/monitoring/collections/'+state['id'],lambda row:
                row['apply_status']=='effective' and row['freshness']=='fresh' and row['health']=='healthy',
                timeout=120,label=key+' real sample')
            write_json(self.run.out/(key+'-fresh.json'),sample)
        print('Cluster, node/container and Collector metrics have fresh samples',flush=True)

    def logs(self):
        state=self.map['application']
        if not state.get('id'):
            state.update(id=self.command({'kind':'application','operation':'create','application':{
                'name':'B09a Orders Container Logs','environment':'testing','description':'Actual CRI stdout and stderr'}},'log-application'),
                name='B09a Orders Container Logs')
            self.save()
        if not state.get('service_id'):
            state['service_id']=self.command({'kind':'service','operation':'create','service':{
                'application_id':state['id'],'service_name':'orders-api','service_namespace':'plan9-b09a'}},'log-service')
            self.save()
        if not self.map['log_rules'].get('orders'):
            deployment=self.run.get('deployment','orders','-n',APP_NS)
            config={'scope':'business','start_at':'beginning','format':'text','max_log_bytes':262144,
                'workload':{'namespace_id':self.namespace,'type':'deployment','name':'orders',
                            'uid':deployment['metadata']['uid'],'containers':['http']}}
            ident=self.command({'kind':'collection','operation':'create','collection':{
                'name':'B09a Orders stdout and stderr','integration':'k8slog','collector_id':self.collector,
                'application_id':state['id'],'service_id':state['service_id'],'enabled':True,
                'credential_update':'keep','config':config}},'orders-log-rule')
            self.map['log_rules']['orders']={'id':ident,'deployment_uid':deployment['metadata']['uid']}
            self.save()
        self.converge()
        receipt=self.run.request('initial-error-'+uuid.uuid4().hex[:8])
        proof=self.wait_log(receipt[0]['message'],2)
        write_json(self.run.out/'container-log-verification.json',{'http_receipts':receipt,'logs':proof})
        print('Real container stdout/stderr reached the scoped application',flush=True)

    def query_log(self,marker):
        return self.api.call('POST','/enterprise/logs/query',{
            **scope(self.map['application']['id']),'query':'body : '+json.dumps(marker)})

    def wait_log(self,marker,count=1):
        import time
        deadline=time.monotonic()+90
        while True:
            data=self.query_log(marker)
            rows=data['data']
            if len(rows)==count:
                assert all(row['resource_id']==self.map['application']['id'] and row['source_type']=='filelog' for row in rows)
                return data
            if len(rows)>count:
                raise RuntimeError('Unexpected duplicate container records')
            if time.monotonic()>deadline:
                raise RuntimeError(f'{marker}: only {len(rows)} of {count} logs arrived')
            time.sleep(2)

    def query(self,expression,resource=None,seconds=0):
        request={**scope(resource or self.cluster,seconds or 900),'query':expression}
        if seconds:
            request['step_seconds']=10
        return self.api.call('POST','/enterprise/metrics/query_range' if seconds else '/enterprise/metrics/query',request)


if __name__=='__main__':
    parser=argparse.ArgumentParser()
    parser.add_argument('action',choices=['enroll','install','builtins','logs','inventory','dashboards','alerts','exercise','pod_lifecycle','retire'])
    parser.add_argument('--session',required=True)
    parser.add_argument('--ca',required=True)
    args=parser.parse_args()
    run=Run()
    api=Api(run,args.session,args.ca)
    api.call('GET','/enterprise/auth/session')
    batch=Batch(run,api)
    from dashboard import dashboards
    from scenario import configure_alert,exercise_alert,restart_and_replace
    from retire import retire
    actions={'dashboards':dashboards,'alerts':configure_alert,'exercise':exercise_alert,
             'pod_lifecycle':restart_and_replace,'retire':retire}
    if args.action in actions:
        actions[args.action](batch)
    else:
        getattr(batch,args.action)()
