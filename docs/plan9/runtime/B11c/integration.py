"""Normal mode-C Bastion enrollment, member relay and two real telemetry tunnels."""
import argparse
import json
import uuid
from manage import Run,now
from common.hosts import HostEnrollment

VERSION='0.3.3-b11a-20261113'

class Batch(HostEnrollment):
    def __init__(self,run,api):
        super().__init__(run,api,json.loads((run.out/'host-specs.json').read_text(encoding='utf-8')),'b11c')

    @property
    def scope(self):return self.map['bastion_scope']['id']

    def credentials(self,key):
        state=self.map['hosts'].setdefault(key,{'name':self.host_specs[key]['name']})
        private=json.loads((self.run.private/'host-credentials.json').read_text(encoding='utf-8'))
        if not state.get('secret_id'):
            row=self.api.call('POST','/enterprise/secrets',{'name':self.run.id+'-'+key+'-ssh','type':'ssh_password','value':private[key]['password']},key='secret-'+key,expected=201)
            state['secret_id']=row['id'];self.save()
        if not state.get('credential_id'):
            row=self.api.call('POST','/enterprise/credentials',{'name':self.run.id+'-'+key+'-ssh','protocol':'ssh','username':'root','secret_id':state['secret_id']},key='credential-'+key,expected=201)
            state['credential_id']=row['id'];self.save()
        return state

    def connection(self,key,scope=None):
        state=self.credentials(key);spec=self.host_specs[key]
        body={'address':spec['address'],'port':22,'platform':'linux','ssh_path':'bastion_connector' if scope else 'direct_executor',
              'onboarding_control_path':'bastion_relay' if scope else 'executor_tunnel','username':'root','credential_id':state['credential_id']}
        if scope:body['bastion_scope_id']=scope
        row=self.api.call('POST','/enterprise/hosts/connection-tests',body,key=key+'-connection-'+uuid.uuid4().hex,expected=202)
        state['connection_test_id']=row['id'];self.save()
        def completed(value):
            if value['status'] in ['failed','expired','result_unknown']:
                self.run.save(key+'-connection-failed',value)
                raise RuntimeError(key+' actual SSH/callback probe failed: '+str(value.get('error_code')))
            return value['status']=='succeeded'
        row=self.api.wait('/enterprise/connection-tests/'+row['id'],completed,timeout=180,label=key+' real SSH and tunnel callback')
        self.run.save(key+'-connection',row);return row

    def bastion(self):
        state=self.credentials('bastion');self.api.login()
        if not self.map.get('bastion_scope'):
            existing=[row for row in self.api.call('GET','/enterprise/bastion-scopes')['items'] if row.get('labels',{}).get('plan9_run')==self.run.id]
            if not existing:
                test=self.connection('bastion')
                preview=self.api.call('POST','/enterprise/bastion-scopes/actions/preview-create',{
                    'name':state['name'],'environment':'development','labels':{'plan9_run':self.run.id,'plan9_batch':'b11c','node':'gateway'},
                    'install_mode':'direct_install_tunnel','address':self.host_specs['bastion']['address'],'port':22,'username':'root',
                    'credential_id':state['credential_id'],'connection_test_id':test['id']},key='bastion-create',expected=201)
                state['create_action_ref']=preview['action_ref'];self.save();self.api.confirm(preview);self.api.login()
                existing=[row for row in self.api.call('GET','/enterprise/bastion-scopes')['items'] if row.get('labels',{}).get('plan9_run')==self.run.id]
            assert len(existing)==1
            self.map['bastion_scope']={'id':existing[0]['id'],'name':existing[0]['name']};self.save()
        scope=self.api.wait('/enterprise/bastion-scopes/'+self.scope,
            lambda row:row['status']=='active' and row['relay_status']=='ready' and row.get('connector_host_id'),timeout=300,label='Mode-C Bastion and Relay')
        state['host_id']=scope['connector_host_id'];self.save()
        host=self.api.wait('/enterprise/hosts/'+state['host_id'],lambda row:row['connection_status']=='online',timeout=120)
        assert host['role']=='bastion' and host['control_path']=='executor_tunnel'
        self.run.save('bastion-online',{'scope':scope,'host':host,'actual_executor_control_tunnel':True})
        print('Real tunnel-only Bastion online through Direct Executor; no outbound platform access',flush=True)

    def member(self):
        state=self.credentials('member');self.api.login()
        if not state.get('host_id'):
            existing=[row for row in self.api.call('GET','/enterprise/hosts')['items'] if row.get('labels',{}).get('plan9_run')==self.run.id and row['role']=='managed_host']
            if not existing:
                test=self.connection('member',self.scope)
                preview=self.api.call('POST','/enterprise/hosts/actions/preview-create',{
                    'name':state['name'],'address':self.host_specs['member']['address'],'port':22,'platform':'linux','role':'managed_host',
                    'control_path':'bastion_relay','install_method':'ssh','ssh_path':'bastion_connector','bastion_scope_id':self.scope,
                    'credential_id':state['credential_id'],'username':'root','connection_test_id':test['id'],'environment':'development',
                    'labels':{'plan9_run':self.run.id,'plan9_batch':'b11c','node':'member'}},key='member-create',expected=201)
                state['create_action_ref']=preview['action_ref'];self.save();self.api.confirm(preview);self.api.login()
                existing=[row for row in self.api.call('GET','/enterprise/hosts')['items'] if row.get('labels',{}).get('plan9_run')==self.run.id and row['role']=='managed_host']
            assert len(existing)==1;state['host_id']=existing[0]['id'];self.save()
        host=self.api.wait('/enterprise/hosts/'+state['host_id'],lambda row:row['connection_status']=='online',timeout=240)
        assert host['control_path']=='bastion_relay' and host['bastion_scope_id']==self.scope
        self.run.save('member-online',host)
        print('Real private member installed by Bastion SSH and online through TLS Relay',flush=True)

    def converge(self,key):
        return self.api.wait('/enterprise/hosts/'+self.map['hosts'][key]['host_id']+'/collector',
             lambda row:row['status']=='converged' and row['desired_revision']==row['effective_revision'],timeout=300,label=key+' Collector convergence')

    def collectors(self):
        distribution=next(row for row in self.api.call('GET','/enterprise/telemetry/distributions') if row['version']==VERSION and row['name']=='argus-otelcol-linux-amd64')
        for key in ['bastion','member']:
            state=self.map['hosts'][key];base='/enterprise/hosts/'+state['host_id']+'/collector'
            current=self.api.call('GET',base,expected=[200,204,404])
            if not current or current.get('code'):
                body={'distribution_version_id':distribution['id'],'profile_ids':[],
                      'route_kind':'direct_argus' if key=='bastion' else 'bastion_gateway',
                      'transport':'executor_tunnel' if key=='bastion' else 'bastion_tunnel'}
                if key=='member':body['gateway_collector_id']=self.map['hosts']['bastion']['collector_id']
                preview=self.api.call('POST',base+'/actions/preview-install',body,key=key+'-collector-install',expected=201)
                state['collector_install_action_ref']=preview['action_ref'];self.save();self.api.confirm(preview)
            current=self.converge(key);state['collector_id']=current['id'];self.save()
            assert current['role']==('edge_gateway' if key=='bastion' else 'leaf')
            assert current['route']['transport']==('executor_tunnel' if key=='bastion' else 'bastion_tunnel')
            self.run.save(key+'-collector-online',current)
            print(key+' native Collector converged through '+current['route']['transport'],flush=True)

    def builtins(self):
        for key,state in self.map['hosts'].items():
            endpoint='/monitoring/builtins?target_type=host&target_id='+state['host_id']
            for metric in ['host_metrics','host_process_metrics','host_systemd_metrics','collector_metrics']:
                item=next(row for row in self.api.call('GET',endpoint) if row['definition']['key']==metric)
                assert item['available'],item.get('reason');collection=item.get('collection') or {}
                if not collection.get('desired_enabled'):
                    preview=self.api.call('POST','/monitoring/builtins/preview',{'target':{'kind':'host','id':state['host_id']},'key':metric,
                        'enabled':True,'expected_version':collection.get('version',0),'config':{'interval':'10s'}},key=key+'-'+metric,expected=201)
                    self.api.confirm(preview)
                self.converge(key)
                self.api.wait(endpoint,lambda rows:any(row['definition']['key']==metric and (row.get('collection') or {}).get('health')=='healthy'
                    and (row.get('collection') or {}).get('freshness')=='fresh' for row in rows),timeout=150,label=key+' '+metric+' native samples')
                print(key+' '+metric+' healthy/fresh before the next configuration',flush=True)
            state['builtins']=self.api.call('GET',endpoint);self.save()

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['bastion','member','collectors','builtins']);args=parser.parse_args()
    run=Run();api=run.api();api.login();getattr(Batch(run,api),args.action)()
