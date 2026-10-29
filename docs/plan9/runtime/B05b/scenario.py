"""Kafka lag derived from actual committed offsets, with an independent control."""
import json
import time
import uuid
from manage import write_json,now
from integration import B05b

def expression(kind,side='work'):
    group='p9-'+side+'-consumer'
    if kind=='lag':return 'sum by (argus_resource_id, group, topic) (kafka_consumer_group_lag{group="'+group+'",topic="p9-'+side+'"})'
    return 'max by (argus_resource_id, group) (kafka_consumer_group_members{group="'+group+'"})'
RULES={'lag':{'name':'B05b Kafka consumer lag','operator':'>','threshold':100,'recovery':5},
       'members':{'name':'B05b Kafka consumer group empty','operator':'<','threshold':1,'recovery':1}}

class Scenario:
    def __init__(self,run,api):self.run,self.api=run,api;self.platform=B05b(run,api);self.map=self.platform.map
    def query(self,query):return self.platform.query(self.map['targets']['kafka'],query)
    def value(self,kind,side='work'):
        rows=self.query(expression(kind,side))['data']['result'];assert len(rows)==1
        return float(rows[0]['value'][1])
    def alerts(self):
        common=json.loads((self.run.out.parent/'common-evaluator.json').read_text(encoding='utf-8'));self.map['evaluator']=common
        for kind,ids in [('host',[x['host_id'] for x in self.map['hosts'].values()]),('monitoring_target',[self.map['targets']['kafka']['target_id']])]:
            path='/enterprise/data-authorizations/role/'+common['role_id'];old=self.api.call('GET',path+'?resource_type='+kind)
            add=[i for i in ids if not any(x['direct'] and x['resource_id']==i for x in old['items'])]
            if add:self.api.call('POST',path,{'resource_type':kind,'resource_ids':add,'remove':False,'expected_version':old['authorization_version']},expected=204)
        self.platform.save()
        for key,rule in RULES.items():
            name=rule['name'];row=next((p for p in self.api.call('GET','/alerts/configurations?kind=policy') if p['name']==name),None)
            if row is None:
                baseline=self.value(key);assert baseline<=rule['recovery'] if rule['operator']=='>' else baseline>=rule['recovery']
                spec={'description':'Plan9 B05b: actual work group commits and Kafka log-end offsets; fixed thresholds before faults','enabled':True,
                    'resources':{'mode':'explicit','types':['monitoring_target'],'ids':[self.map['targets']['kafka']['target_id']],'matchers':[]},
                    'rules':[{'name':name,'expression':expression(key),'query_mode':'numeric','operator':rule['operator'],'threshold':rule['threshold'],'recovery_threshold':rule['recovery'],'severity':'warning','unit':''}],
                    'mode':'independent','match_by':['argus_resource_id','group'],'interval_seconds':10,'for_seconds':20,'recovery_seconds':20,'no_data_behavior':'report','error_behavior':'keep','channel_ids':[],'route_ids':[],
                    'labels':{'plan9_batch':'b05b','plan9_run':self.run.id}}
                write_json(self.run.out/(key+'-baseline.json'),{'observed_at':now(),'current':baseline,'frozen_spec':spec})
                trial=self.api.call('POST','/alerts/trial',spec);assert trial['health']=='normal';write_json(self.run.out/(key+'-trial.json'),trial)
                self.api.confirm(self.api.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'create','name':name,'spec':spec},key='policy-'+key,expected=202))
                row=next(p for p in self.api.call('GET','/alerts/configurations?kind=policy') if p['name']==name)
            self.map['alert_policies'][key]={'id':row['id'],'name':name,'expression':expression(key),'threshold':rule['threshold'],'recovery_threshold':rule['recovery']};self.platform.save()
            self.run.event('policy_frozen_before_fault',scenario=key,policy_id=row['id']);print(key+': alert configured',flush=True)
    def wait_alert(self,key,state,ids=None):
        path='/alerts/'+('history' if state=='resolved' else 'instances')+'?policy_id='+self.map['alert_policies'][key]['id']
        return self.api.wait(path,lambda r:any(x['lifecycle']==state and x['health']=='normal' and x['state']['kind']=='condition' and (not ids or x['id'] in ids) for x in r['items']),timeout=150,label=key+' '+state)
    def exercise(self):
        path=self.run.out/'scenario-checks.json';reports=json.loads(path.read_text(encoding='utf-8')) if path.exists() else {}
        for key in RULES:
            if reports.get(key,{}).get('recovered'):continue
            start=now();published=None;paused_offsets=None
            try:
                self.run.fault(key)
                if key=='lag':
                    deadline=time.monotonic()+20
                    while True:
                        state=json.loads(self.run.shell('collector','cat /var/lib/plan9/consumer-work.json\n'))
                        if state['paused'] and state['assigned_partitions']==3:break
                        if time.monotonic()>deadline:raise RuntimeError('Consumer did not pause')
                        time.sleep(1)
                    paused=self.run.client('native');paused_offsets=next(g for g in paused['groups'] if g['group']=='p9-work-consumer')['partitions'];write_json(self.run.out/'lag-before-offsets.json',paused)
                    published=self.run.client('burst',count=240);write_json(self.run.out/'lag-confirmed-publish.json',published)
                firing=self.wait_alert(key,'firing');ids={x['id'] for x in firing['items'] if x['lifecycle']=='firing' and x['state']['kind']=='condition'};write_json(self.run.out/(key+'-firing.json'),firing)
                observed=self.value(key);native=self.run.client('native');work=next(g for g in native['groups'] if g['group']=='p9-work-consumer');control=next(g for g in native['groups'] if g['group']=='p9-control-consumer')
                assert control['members']==1 and sum(x['lag'] for x in control['partitions'])<=5 and self.value('lag','control')<=5
                if key=='lag':
                    assert observed>100 and work['members']==1
                    assert [x['committed'] for x in work['partitions']]==[x['committed'] for x in paused_offsets]
                    raw=sum(x['log_end']-x['committed'] for x in work['partitions']);assert abs(raw-observed)<=10
                else:assert observed==0 and work['members']==0
                write_json(self.run.out/(key+'-native-comparison.json'),{'native':native,'observed_value':observed,'healthy_control':True,'lag_is_log_end_minus_real_commit':True})
                print(key+': native state, real offsets and alert verified',flush=True)
            finally:self.run.fault(key,True)
            write_json(self.run.out/(key+'-recovered.json'),self.wait_alert(key,'resolved',ids))
            final=self.run.client('native');assert all(g['members']==1 and sum(p['lag'] for p in g['partitions'])<=5 for g in final['groups'])
            write_json(self.run.out/(key+'-after-offsets.json'),final)
            reports[key]={'started_at':start,'finished_at':now(),'firing_ids':sorted(ids),'observed_value':observed,'published_burst':published['delivered'] if published else 0,'recovered':True,'control_group_healthy':True,'offset_reset_used':False};write_json(path,reports)
            print(key+': real consumers resumed and alert recovered',flush=True)
    def switch(self,enabled):
        t=self.map['targets']['kafka'];path='/monitoring/collections/'+t['collection_id']
        self.api.wait('/enterprise/hosts/'+self.map['hosts']['collector']['host_id']+'/collector',lambda c:c['status']=='converged',timeout=180,label='Collector converged')
        c=self.api.call('GET',path)
        if c['desired_enabled']!=enabled:self.api.confirm(self.api.call('POST','/monitoring/commands/preview',{'kind':'collection','operation':'set_enabled','id':c['id'],'expected_version':c['version'],'collection':{'enabled':enabled}},key='switch-'+uuid.uuid4().hex,expected=201))
        return self.api.wait(path,lambda c:c['desired_enabled']==enabled and c['apply_status']=='effective' and c['desired_revision']==c['effective_revision'],timeout=180,label='Collection intent applied')
    def lifecycle(self):
        start=now()
        try:
            stopped=self.switch(False)
            for p in self.map['alert_policies'].values():self.api.wait('/alerts/configurations/'+p['id'],lambda x:x['health']=='not_evaluated',timeout=90,label='Explicit stop suspends evaluation')
            rows=self.api.call('GET','/monitoring/collections?limit=100')['items'];own=[x for x in rows if x['target']['id']==self.map['hosts']['collector']['host_id']];assert own and all(x['desired_enabled'] and x['freshness']=='fresh' for x in own)
        finally:
            restored=self.switch(True)
            self.api.wait('/monitoring/collections/'+self.map['targets']['kafka']['collection_id'],lambda c:c['health']=='healthy' and c['freshness']=='fresh',timeout=180,label='Kafka samples recovered')
            for p in self.map['alert_policies'].values():self.api.wait('/alerts/configurations/'+p['id'],lambda x:x['health']=='normal',timeout=90,label='Evaluation resumed')
        write_json(self.run.out/'lifecycle-checks.json',{'started_at':start,'finished_at':now(),'stopped_revision':stopped['desired_revision'],'restored_revision':restored['effective_revision'],'host_metrics_fresh':True,'both_policies_suspended':True,'restored':True})
    def verify(self):
        t=self.map['targets']['kafka'];c=self.api.call('GET','/monitoring/collections/'+t['collection_id']);assert c['desired_enabled'] and c['health']=='healthy' and c['freshness']=='fresh' and c['effective_revision']==c['desired_revision']
        rows=self.query('kafka_consumer_group_lag')['data']['result'];assert len(rows)==6
        assert {x['metric']['group'] for x in rows}=={'p9-work-consumer','p9-control-consumer'}
        assert all(x['metric']['argus_resource_id']==t['target_id'] and x['metric']['argus_collection_id']==t['collection_id'] for x in rows)
        native=self.run.client('native');assert native['topics']=={'p9-work':3,'p9-control':3} and len(native['broker_ids'])==1
        assert all(g['members']==1 and all(p['committed']>0 and p['log_end']>=p['committed'] for p in g['partitions']) and sum(p['lag'] for p in g['partitions'])<=5 for g in native['groups'])
        cli={side:self.run.cli('kafka-consumer-groups.sh','--describe','--group','p9-'+side+'-consumer') for side in ['work','control']}
        write_json(self.run.out/'monitoring-verification.json',{'observed_at':now(),'last_sample_at':c['last_sample_at'],'health':c['health'],'freshness':c['freshness'],'groups_and_partitions_isolated':True,'native':native,'broker_cli_offset_observation':cli})
        self.platform.dashboards()
