"""Cache pressure, genuine evictions, recovery and independent collection."""
import json
from pathlib import Path
import time
import uuid
from manage import ENGINES,ROOT,write_json,now
from integration import B04

RULES={'redis':'max by (argus_resource_id) (redis_memory_used)',
       'memcached':'max by (argus_resource_id) (memcached_bytes)'}

class Scenario:
    def __init__(self,run,api):
        self.run,self.api=run,api;self.platform=B04(run,api);self.map=self.platform.map
    def expression(self,key,panel):
        definitions=json.loads((ROOT/'api/contracts/dashboard-templates.json').read_text(encoding='utf-8'))
        d=next(x for x in definitions if x['key']==key+'.overview')
        return next(x for x in d['spec']['panels'] if x['id']==panel)['targets'][0]['source_definition']['dsl']['expression']
    def query(self,key,expression):return self.platform.query(self.map['targets'][key],expression)
    def value(self,key,expression=None):
        rows=self.query(key,expression or RULES[key])['data']['result'];assert len(rows)==1
        return float(rows[0]['value'][1])
    def idle(self):
        reports=[]
        for key in ENGINES:
            hit=self.query(key,self.expression(key,'hits'));miss=self.query(key,self.expression(key,'misses'))
            assert hit['data']['result'] and miss['data']['result']
            assert all(float(x['value'][1])==0 for x in hit['data']['result']+miss['data']['result'])
            ratio=self.query(key,self.expression(key,'hit_ratio'));assert not ratio['data']['result']
            reports.append({'engine':key,'observed_at':now(),'hits':hit,'misses':miss,'ratio':ratio,'zero_denominator_not_zero_percent':True})
        write_json(self.run.out/'idle-ratio-checks.json',reports)
    def alerts(self):
        common=json.loads((self.run.out.parent/'common-evaluator.json').read_text(encoding='utf-8'));self.map['evaluator']=common
        for kind,ids in [('host',[x['host_id'] for x in self.map['hosts'].values()]),('monitoring_target',[x['target_id'] for x in self.map['targets'].values()])]:
            path='/enterprise/data-authorizations/role/'+common['role_id'];existing=self.api.call('GET',path+'?resource_type='+kind)
            add=[i for i in ids if not any(g['direct'] and g['resource_id']==i for g in existing['items'])]
            if add:self.api.call('POST',path,{'resource_type':kind,'resource_ids':add,'remove':False,'expected_version':existing['authorization_version']},expected=204)
        self.platform.save()
        for key,expression in RULES.items():
            name='B04 '+('Redis' if key=='redis' else 'Memcached')+' cache memory high'
            row=next((p for p in self.api.call('GET','/alerts/configurations?kind=policy') if p['name']==name),None)
            if row is None:
                baseline=self.value(key);assert baseline<=2*1048576
                spec={'description':'Plan9 B04: real bounded SET requests, 8 MiB cache and 120-second TTL; frozen thresholds','enabled':True,
                    'resources':{'mode':'explicit','types':['monitoring_target'],'ids':[self.map['targets'][key]['target_id']],'matchers':[]},
                    'rules':[{'name':name,'expression':expression,'query_mode':'numeric','operator':'>','threshold':5*1048576,'recovery_threshold':2*1048576,'severity':'warning','unit':'bytes'}],
                    'mode':'independent','match_by':['argus_resource_id'],'interval_seconds':10,'for_seconds':20,'recovery_seconds':20,
                    'no_data_behavior':'report','error_behavior':'keep','channel_ids':[],'route_ids':[],'labels':{'plan9_batch':'b04','plan9_run':self.run.id}}
                write_json(self.run.out/(key+'-alert-baseline.json'),{'observed_at':now(),'bytes':baseline,'frozen_spec':spec})
                trial=self.api.call('POST','/alerts/trial',spec);assert trial['health']=='normal';write_json(self.run.out/(key+'-trial.json'),trial)
                self.api.confirm(self.api.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'create','name':name,'spec':spec},key='policy-'+key,expected=202))
                row=next(p for p in self.api.call('GET','/alerts/configurations?kind=policy') if p['name']==name)
            self.map['alert_policies'][key]={'id':row['id'],'name':name,'expression':expression,'threshold':5*1048576,'recovery_threshold':2*1048576};self.platform.save()
            self.run.event('policy_ready_before_pressure',engine=key,policy_id=row['id']);print(key+': fixed memory alert configured',flush=True)
    def wait_alert(self,key,state,ids=None):
        path='/alerts/'+('history' if state=='resolved' else 'instances')+'?policy_id='+self.map['alert_policies'][key]['id']
        return self.api.wait(path,lambda result:any(x['lifecycle']==state and x['health']=='normal' and x['state']['kind']=='condition' and (not ids or x['id'] in ids) for x in result['items']),timeout=150,label=key+' '+state)
    def exercise(self):
        path=self.run.out/'scenario-checks.json';reports=json.loads(path.read_text(encoding='utf-8')) if path.exists() else {}
        for key in ENGINES:
            if reports.get(key,{}).get('recovered'):continue
            start=now()
            try:
                pressure=self.run.client('pressure',key);write_json(self.run.out/(key+'-pressure.json'),pressure)
                event=self.wait_alert(key,'firing');ids={x['id'] for x in event['items'] if x['state']['kind']=='condition' and x['lifecycle']=='firing'}
                write_json(self.run.out/(key+'-firing.json'),event)
                observed=self.value(key);assert observed>5*1048576
                native=self.run.client('native',key);raw=native['stats']['used_memory' if key=='redis' else 'bytes']
                assert abs(raw-observed)<262144
                other=next(x for x in ENGINES if x!=key);assert self.value(other)<=2*1048576
                evictions=self.query(key,'redis_keys_evicted' if key=='redis' else 'memcached_evictions');assert float(evictions['data']['result'][0]['value'][1])>0
                write_json(self.run.out/(key+'-native-comparison.json'),{'observed_at':now(),'native':native,'collected_bytes':observed,'difference_bytes':abs(raw-observed),'allowance_bytes':262144,'control':other,'control_healthy':True,'evictions_query':evictions})
                print(key+': actual eviction and alert verified',flush=True)
            finally:self.run.client('recover',key)
            write_json(self.run.out/(key+'-recovered.json'),self.wait_alert(key,'resolved',ids))
            reports[key]={'started_at':start,'finished_at':now(),'firing_ids':sorted(ids),'recovered':True,'actual_evictions':pressure['actual_evictions'],'healthy_control':other};write_json(path,reports)
            print(key+': memory and alert recovered',flush=True)
    def switch(self,key,enabled):
        path='/monitoring/collections/'+self.map['targets'][key]['collection_id']
        self.api.wait('/enterprise/hosts/'+self.map['hosts']['collector']['host_id']+'/collector',lambda c:c['status']=='converged',timeout=180,label='Collector ready')
        row=self.api.call('GET',path)
        if row['desired_enabled']!=enabled:
            self.api.confirm(self.api.call('POST','/monitoring/commands/preview',{'kind':'collection','operation':'set_enabled','id':row['id'],'expected_version':row['version'],'collection':{'enabled':enabled}},key='switch-'+uuid.uuid4().hex,expected=201))
        return self.api.wait(path,lambda c:c['desired_enabled']==enabled and c['apply_status']=='effective' and c['effective_revision']==c['desired_revision'],timeout=180,label='Collection intent applied')
    def lifecycle(self):
        start=now();policy='/alerts/configurations/'+self.map['alert_policies']['redis']['id']
        try:
            stopped=self.switch('redis',False)
            self.api.wait(policy,lambda x:x['health']=='not_evaluated',timeout=90,label='Explicit stop suspends evaluation')
            self.api.wait('/monitoring/collections/'+self.map['targets']['memcached']['collection_id'],lambda c:c['freshness']=='fresh' and c['last_sample_at']>start,timeout=90,label='Memcached still collects')
        finally:
            restored=self.switch('redis',True)
            self.api.wait('/monitoring/collections/'+self.map['targets']['redis']['collection_id'],lambda c:c['health']=='healthy' and c['freshness']=='fresh',timeout=120,label='Redis collection recovered')
            self.api.wait(policy,lambda x:x['health']=='normal',timeout=90,label='Redis alert evaluation recovered')
        write_json(self.run.out/'lifecycle-checks.json',{'started_at':start,'finished_at':now(),'redis_stopped_revision':stopped['desired_revision'],'restored_revision':restored['effective_revision'],'memcached_remained_fresh':True,'alert_suspended':True,'restored':True})
    def verify(self):
        report={'observed_at':now(),'targets':[]}
        for key,t in self.map['targets'].items():
            c=self.api.call('GET','/monitoring/collections/'+t['collection_id']);assert c['desired_enabled'] and c['health']=='healthy' and c['freshness']=='fresh' and c['desired_revision']==c['effective_revision']
            values=self.query(key,'redis_memory_used' if key=='redis' else 'memcached_bytes')['data']['result'];assert values
            assert all(x['metric']['argus_resource_id']==t['target_id'] and x['metric']['argus_collection_id']==t['collection_id'] for x in values)
            ratio=self.value(key,self.expression(key,'hit_ratio'));assert 0.35<ratio<0.65
            native=self.run.client('native',key);write_json(self.run.out/(key+'-final-native.json'),native)
            report['targets'].append({'name':t['name'],'fresh':True,'last_sample_at':c['last_sample_at'],'isolated':True,'hit_ratio':ratio,'effective_revision':c['effective_revision']})
        write_json(self.run.out/'monitoring-verification.json',report);self.platform.dashboards()
