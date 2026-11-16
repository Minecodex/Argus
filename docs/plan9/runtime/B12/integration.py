"""New, normally enrolled B12 Host and authenticated Redis monitoring."""
import argparse
import json
from manage import Run,now
from common.hosts import HostEnrollment
from common.monitoring import MonitoringBatch

VERSION='0.3.3-b11a-20261113'

class Batch(HostEnrollment):
    def __init__(self,run,api):super().__init__(run,api,json.loads((run.out/'host-specs.json').read_text()),'b12')
    @property
    def host(self):return self.map['hosts']['direct']['host_id']
    @property
    def collector(self):return self.map['hosts']['direct']['collector_id']
    def converge(self):return self.api.wait('/enterprise/hosts/'+self.host+'/collector',lambda row:row['status']=='converged' and row['desired_revision']==row['effective_revision'],timeout=240,label='B12 Collector convergence')
    def enroll(self):
        self.onboard();self.collectors(distribution_version=VERSION);self.converge()
        current=self.api.call('GET','/enterprise/hosts/'+self.host)
        assert current['control_path']=='executor_tunnel' and not current.get('bastion_scope_id')
        collector=self.api.call('GET','/enterprise/hosts/'+self.host+'/collector')
        assert collector['route']['transport']=='executor_tunnel'
        self.run.save('direct-route',{'host_id':self.host,'collector_id':self.collector,'actual_ssh_installation':True,
            'ssh_path':'direct_executor','control_path':current['control_path'],'telemetry_transport':collector['route']['transport']})
    def builtins(self):
        path='/monitoring/builtins?target_type=host&target_id='+self.host
        for key in ['host_metrics','host_process_metrics','host_systemd_metrics','collector_metrics']:
            row=next(row for row in self.api.call('GET',path) if row['definition']['key']==key);assert row['available']
            collection=row.get('collection') or {}
            if not collection.get('desired_enabled'):
                preview=self.api.call('POST','/monitoring/builtins/preview',{'target':{'kind':'host','id':self.host},'key':key,
                    'enabled':True,'expected_version':collection.get('version',0),'config':{'interval':'10s'}},key='builtin-'+key,expected=201)
                self.api.confirm(preview)
            self.converge()
            self.api.wait(path,lambda rows:any(row['definition']['key']==key and (row.get('collection') or {}).get('health')=='healthy'
                and (row.get('collection') or {}).get('freshness')=='fresh' for row in rows),timeout=150,label='B12 '+key+' native samples')
            print('B12 '+key+' healthy/fresh',flush=True)
        self.run.save('builtin-collections',self.api.call('GET',path))
    def target(self):
        name='B12-Redis';rows=[row for row in self.api.call('GET','/monitoring/collections?search='+name)['items'] if row['name']==name]
        if not rows:
            credentials=json.loads((self.run.private/'redis-credentials.json').read_text())
            preview=self.api.call('POST','/monitoring/commands/preview',{'kind':'collection','operation':'create',
                'collection':{'name':name,'integration':'redis','collector_id':self.collector,'target_name':name,
                    'target_identity':{'endpoint':'redis:6379'},'enabled':True,'credential_update':'replace',
                    'config':{'interval':'10s','timeout':'5s','tls':{'enabled':False}}},
                'credentials':{'username':'monitor','password':credentials['monitor']}},key='redis-collection',expected=201)
            self.api.confirm(preview);rows=[row for row in self.api.call('GET','/monitoring/collections?search='+name)['items'] if row['name']==name]
        assert len(rows)==1;row=rows[0]
        self.map['targets']['redis']={'name':name,'engine':'redis','host':'direct','target_id':row['target']['id'],'collection_id':row['id'],'collector_id':self.collector,'identity':{'endpoint':'redis:6379'}};self.save()
        current=self.api.wait('/monitoring/collections/'+row['id'],lambda value:value['apply_status']=='effective' and value['health']=='healthy' and value['freshness']=='fresh',timeout=180,label='Actual B12 Redis samples')
        self.run.save('redis-collection',current);print('New Redis monitor authenticated with its restricted account; native samples fresh',flush=True)
    def dashboards(self):
        from B11a.monitoring import dashboards as host_dashboards
        host_dashboards(self)
        MonitoringBatch(self.run,self.api,['redis']).dashboards()
        self.map=json.loads(self.run.resources.read_text());print('Actual Host and Redis managed dashboards bound and executed',flush=True)

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['enroll','builtins','target','dashboards']);args=parser.parse_args()
    run=Run();api=run.api();api.login();getattr(Batch(run,api),args.action)()
