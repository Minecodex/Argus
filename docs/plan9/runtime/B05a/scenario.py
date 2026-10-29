"""Real queue backlogs and held acknowledgements, with a separate vhost control."""
import json
import time
import uuid
from manage import write_json,now
from integration import B05a

def expression(state,side='work'):
    return 'sum by (argus_resource_id, resource_rabbitmq_vhost_name, resource_rabbitmq_queue_name) (rabbitmq_message_current{state="'+state+'",resource_rabbitmq_vhost_name="p9-'+side+'",resource_rabbitmq_queue_name="jobs"})'

RULES={'backlog':{'state':'ready','threshold':100,'recovery':5,'name':'B05a RabbitMQ ready backlog'},
       'unacked':{'state':'unacknowledged','threshold':5,'recovery':0,'name':'B05a RabbitMQ awaiting acknowledgement'}}

class Scenario:
    def __init__(self,run,api):self.run,self.api=run,api;self.platform=B05a(run,api);self.map=self.platform.map
    def query(self,query,seconds=0):return self.platform.query(self.map['targets']['rabbitmq'],query,seconds)
    def value(self,state,side='work'):
        rows=self.query(expression(state,side))['data']['result'];assert len(rows)==1
        return float(rows[0]['value'][1])
    def alerts(self):
        common=json.loads((self.run.out.parent/'common-evaluator.json').read_text(encoding='utf-8'));self.map['evaluator']=common
        for kind,ids in [('host',[x['host_id'] for x in self.map['hosts'].values()]),('monitoring_target',[self.map['targets']['rabbitmq']['target_id']])]:
            path='/enterprise/data-authorizations/role/'+common['role_id'];old=self.api.call('GET',path+'?resource_type='+kind)
            add=[i for i in ids if not any(x['direct'] and x['resource_id']==i for x in old['items'])]
            if add:self.api.call('POST',path,{'resource_type':kind,'resource_ids':add,'remove':False,'expected_version':old['authorization_version']},expected=204)
        self.platform.save()
        for key,rule in RULES.items():
            name=rule['name'];row=next((p for p in self.api.call('GET','/alerts/configurations?kind=policy') if p['name']==name),None)
            if row is None:
                baseline=self.value(rule['state']);assert baseline<=5
                spec={'description':'Plan9 B05a: real AMQP publisher confirms/manual acknowledgements; work vhost only; thresholds frozen before faults','enabled':True,
                    'resources':{'mode':'explicit','types':['monitoring_target'],'ids':[self.map['targets']['rabbitmq']['target_id']],'matchers':[]},
                    'rules':[{'name':name,'expression':expression(rule['state']),'query_mode':'numeric','operator':'>','threshold':rule['threshold'],'recovery_threshold':rule['recovery'],'severity':'warning','unit':''}],
                    'mode':'independent','match_by':['argus_resource_id','resource_rabbitmq_vhost_name','resource_rabbitmq_queue_name'],
                    'interval_seconds':10,'for_seconds':20,'recovery_seconds':20,'no_data_behavior':'report','error_behavior':'keep','channel_ids':[],'route_ids':[],
                    'labels':{'plan9_batch':'b05a','plan9_run':self.run.id}}
                write_json(self.run.out/(key+'-baseline.json'),{'observed_at':now(),'current':baseline,'frozen_spec':spec})
                trial=self.api.call('POST','/alerts/trial',spec);assert trial['health']=='normal';write_json(self.run.out/(key+'-trial.json'),trial)
                self.api.confirm(self.api.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'create','name':name,'spec':spec},key='policy-'+key,expected=202))
                row=next(p for p in self.api.call('GET','/alerts/configurations?kind=policy') if p['name']==name)
            self.map['alert_policies'][key]={'id':row['id'],'name':name,'expression':expression(rule['state']),'threshold':rule['threshold'],'recovery_threshold':rule['recovery']};self.platform.save()
            self.run.event('policy_frozen_before_fault',scenario=key,policy_id=row['id']);print(key+': alert configured',flush=True)
    def wait_alert(self,key,state,ids=None):
        path='/alerts/'+('history' if state=='resolved' else 'instances')+'?policy_id='+self.map['alert_policies'][key]['id']
        return self.api.wait(path,lambda r:any(x['lifecycle']==state and x['health']=='normal' and x['state']['kind']=='condition' and (not ids or x['id'] in ids) for x in r['items']),timeout=150,label=key+' '+state)
    def exercise(self):
        path=self.run.out/'scenario-checks.json';reports=json.loads(path.read_text(encoding='utf-8')) if path.exists() else {}
        for key,rule in RULES.items():
            if reports.get(key,{}).get('recovered'):continue
            start=now()
            try:
                self.run.fault(key)
                published=self.run.client('burst',count=250 if key=='backlog' else 20);write_json(self.run.out/(key+'-amqp-publish.json'),published)
                firing=self.wait_alert(key,'firing');ids={x['id'] for x in firing['items'] if x['lifecycle']=='firing' and x['state']['kind']=='condition'}
                write_json(self.run.out/(key+'-firing.json'),firing)
                value=self.value(rule['state']);assert value>rule['threshold']
                native=self.run.client('native');work=next(x for x in native['queues'] if x['vhost']=='p9-work');control=next(x for x in native['queues'] if x['vhost']=='p9-control')
                field='messages_ready' if key=='backlog' else 'messages_unacknowledged'
                assert work[field]>rule['threshold'] and abs(work[field]-value)<=10
                assert control['consumers']>=1 and control['messages_ready']<=5 and self.value(rule['state'],'control')<=5
                assert work['consumers']==0 if key=='backlog' else work['messages_unacknowledged']==10
                write_json(self.run.out/(key+'-native-comparison.json'),{'native':native,'collected_messages':value,'native_messages':work[field],'healthy_control':True})
                print(key+': actual queue state and alert verified',flush=True)
            finally:self.run.fault(key,True)
            write_json(self.run.out/(key+'-recovered.json'),self.wait_alert(key,'resolved',ids))
            native=self.run.client('native');assert all(x['messages_ready']<=5 and x['messages_unacknowledged']<=1 for x in native['queues'])
            reports[key]={'started_at':start,'finished_at':now(),'confirmed_burst':published['confirmed'],'observed_messages':value,'firing_ids':sorted(ids),'recovered':True,'control_vhost_healthy':True};write_json(path,reports)
            print(key+': actual acknowledgements drained queue and alert recovered',flush=True)
    def switch(self,enabled):
        target=self.map['targets']['rabbitmq'];path='/monitoring/collections/'+target['collection_id']
        self.api.wait('/enterprise/hosts/'+self.map['hosts']['collector']['host_id']+'/collector',lambda c:c['status']=='converged',timeout=180,label='Collector converged')
        c=self.api.call('GET',path)
        if c['desired_enabled']!=enabled:self.api.confirm(self.api.call('POST','/monitoring/commands/preview',{'kind':'collection','operation':'set_enabled','id':c['id'],'expected_version':c['version'],'collection':{'enabled':enabled}},key='switch-'+uuid.uuid4().hex,expected=201))
        return self.api.wait(path,lambda c:c['desired_enabled']==enabled and c['apply_status']=='effective' and c['desired_revision']==c['effective_revision'],timeout=180,label='Collection intent applied')
    def lifecycle(self):
        start=now()
        try:
            stopped=self.switch(False)
            for p in self.map['alert_policies'].values():self.api.wait('/alerts/configurations/'+p['id'],lambda x:x['health']=='not_evaluated',timeout=90,label='Stopped collection evaluation suspended')
            rows=self.api.call('GET','/monitoring/collections?limit=100')['items']
            own=[x for x in rows if x['target']['id']==self.map['hosts']['collector']['host_id']]
            assert own and all(x['desired_enabled'] and x['freshness']=='fresh' for x in own)
        finally:
            restored=self.switch(True)
            self.api.wait('/monitoring/collections/'+self.map['targets']['rabbitmq']['collection_id'],lambda c:c['health']=='healthy' and c['freshness']=='fresh',timeout=180,label='RabbitMQ samples restored')
            for p in self.map['alert_policies'].values():self.api.wait('/alerts/configurations/'+p['id'],lambda x:x['health']=='normal',timeout=90,label='Alert evaluation restored')
        write_json(self.run.out/'lifecycle-checks.json',{'started_at':start,'finished_at':now(),'stopped_revision':stopped['desired_revision'],'restored_revision':restored['effective_revision'],'host_collections_remained_fresh':True,'both_policies_suspended':True,'restored':True})
    def verify(self):
        t=self.map['targets']['rabbitmq'];c=self.api.call('GET','/monitoring/collections/'+t['collection_id']);assert c['desired_enabled'] and c['health']=='healthy' and c['freshness']=='fresh' and c['desired_revision']==c['effective_revision']
        query=self.query('rabbitmq_message_current');rows=query['data']['result'];assert len(rows)==4
        assert {x['metric']['resource_rabbitmq_vhost_name'] for x in rows}=={'p9-work','p9-control'}
        assert all(x['metric']['argus_resource_id']==t['target_id'] and x['metric']['argus_collection_id']==t['collection_id'] for x in rows)
        rates={}
        for metric in ['rabbitmq_message_published','rabbitmq_message_delivered','rabbitmq_message_acknowledged']:
            result=self.query('rate('+metric+'[5m])')['data']['result'];assert len(result)==2 and all(float(x['value'][1])>0 for x in result);rates[metric]=result
        native=self.run.client('native');assert len(native['queues'])==2 and all(q['consumers']==1 and q['messages']<=5 for q in native['queues'])
        write_json(self.run.out/'monitoring-verification.json',{'observed_at':now(),'health':c['health'],'freshness':c['freshness'],'last_sample_at':c['last_sample_at'],'same_queue_names_separate_vhosts':True,'target_and_collection_isolated':True,'rates':rates,'native':native})
        self.platform.dashboards()
