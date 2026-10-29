"""Bounded real MongoDB connections and updates, with a healthy peer."""
import json
import time
import uuid
from manage import write_json,now
from integration import B06a

RULES={'connections':{'query':'max by (argus_resource_id) (mongodb_connection_count{type="current"})','threshold':20,'recovery':12,'name':'B06a MongoDB connections high','unit':''},
       'operations':{'query':'sum by (argus_resource_id) (rate(mongodb_operation_count{operation="update"}[1m]))','threshold':5,'recovery':1,'name':'B06a MongoDB update rate high','unit':'ops/s'}}

class Scenario:
    def __init__(self,run,api):self.run,self.api=run,api;self.platform=B06a(run,api);self.map=self.platform.map
    def query(self,query,side='a'):return self.platform.query(self.map['targets'][side],query)
    def value(self,kind,side='a'):
        rows=self.query(RULES[kind]['query'],side)['data']['result'];assert len(rows)==1
        return float(rows[0]['value'][1])
    def alerts(self):
        common=json.loads((self.run.out.parent/'common-evaluator.json').read_text(encoding='utf-8'));self.map['evaluator']=common
        for kind,ids in [('host',[x['host_id'] for x in self.map['hosts'].values()]),('monitoring_target',[x['target_id'] for x in self.map['targets'].values()])]:
            path='/enterprise/data-authorizations/role/'+common['role_id'];old=self.api.call('GET',path+'?resource_type='+kind);add=[i for i in ids if not any(x['direct'] and x['resource_id']==i for x in old['items'])]
            if add:self.api.call('POST',path,{'resource_type':kind,'resource_ids':add,'remove':False,'expected_version':old['authorization_version']},expected=204)
        self.platform.save()
        for key,rule in RULES.items():
            name=rule['name'];row=next((p for p in self.api.call('GET','/alerts/configurations?kind=policy') if p['name']==name),None)
            if row is None:
                baseline=self.value(key);assert baseline<=rule['recovery']
                spec={'description':'Plan9 B06a: actual MongoDB client connections and updateOne requests; only A; thresholds frozen before pressure','enabled':True,
                    'resources':{'mode':'explicit','types':['monitoring_target'],'ids':[self.map['targets']['a']['target_id']],'matchers':[]},
                    'rules':[{'name':name,'expression':rule['query'],'query_mode':'numeric','operator':'>','threshold':rule['threshold'],'recovery_threshold':rule['recovery'],'severity':'warning','unit':rule['unit']}],
                    'mode':'independent','match_by':['argus_resource_id'],'interval_seconds':10,'for_seconds':20,'recovery_seconds':20,'no_data_behavior':'report','error_behavior':'keep','channel_ids':[],'route_ids':[],'labels':{'plan9_batch':'b06a','plan9_run':self.run.id}}
                write_json(self.run.out/(key+'-baseline.json'),{'observed_at':now(),'current':baseline,'frozen_spec':spec});trial=self.api.call('POST','/alerts/trial',spec);assert trial['health']=='normal';write_json(self.run.out/(key+'-trial.json'),trial)
                self.api.confirm(self.api.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'create','name':name,'spec':spec},key='policy-'+key,expected=202));row=next(p for p in self.api.call('GET','/alerts/configurations?kind=policy') if p['name']==name)
            self.map['alert_policies'][key]={'id':row['id'],'name':name,'expression':rule['query'],'threshold':rule['threshold'],'recovery_threshold':rule['recovery']};self.platform.save();self.run.event('policy_frozen_before_load',scenario=key,policy_id=row['id']);print(key+': alert configured',flush=True)
    def wait_alert(self,key,state,ids=None):
        path='/alerts/'+('history' if state=='resolved' else 'instances')+'?policy_id='+self.map['alert_policies'][key]['id']
        return self.api.wait(path,lambda r:any(x['lifecycle']==state and x['health']=='normal' and x['state']['kind']=='condition' and (not ids or x['id'] in ids) for x in r['items']),timeout=150,label=key+' '+state)
    def exercise(self):
        path=self.run.out/'scenario-checks.json';reports=json.loads(path.read_text(encoding='utf-8')) if path.exists() else {}
        for key,rule in RULES.items():
            if reports.get(key,{}).get('recovered'):continue
            start=now();before=self.run.client('native')
            try:
                self.run.fault(key);firing=self.wait_alert(key,'firing');ids={x['id'] for x in firing['items'] if x['lifecycle']=='firing' and x['state']['kind']=='condition'};write_json(self.run.out/(key+'-firing.json'),firing)
                process=json.loads(self.run.shell('collector','cat /var/lib/plan9/pressure-'+key+'.json\n'));assert process['ready']
                observed=self.value(key);assert observed>rule['threshold'];native=self.run.client('native');control=self.run.client('native','b');assert self.value(key,'b')<=rule['recovery']
                if key=='connections':assert process['opened_clients']==12 and abs(native['connections']['current']-observed)<=6
                else:assert process['operations']>100 and native['operations']['update']-before['operations']['update']>100
                write_json(self.run.out/(key+'-native-comparison.json'),{'observed_at':now(),'collected_value':observed,'before':before,'native':native,'healthy_control':control,'actual_client_process':process});print(key+': actual pressure, native counters and alert verified',flush=True)
            finally:self.run.fault(key,True)
            write_json(self.run.out/(key+'-recovered.json'),self.wait_alert(key,'resolved',ids));after=self.run.client('verify');assert after['documents']==200;write_json(self.run.out/(key+'-after-recovery.json'),after)
            stopped=json.loads(self.run.shell('collector','cat /var/lib/plan9/pressure-'+key+'.json\n'));assert not stopped['ready'];write_json(self.run.out/(key+'-client-completed.json'),stopped)
            reports[key]={'started_at':start,'finished_at':now(),'observed_value':observed,'firing_ids':sorted(ids),'recovered':True,'healthy_control':True,'documents_preserved':200,'actual_pressure_updates':stopped['completed_operations']};write_json(path,reports);print(key+': clients stopped and alert recovered',flush=True)
    def switch(self,enabled):
        t=self.map['targets']['a'];path='/monitoring/collections/'+t['collection_id'];self.api.wait('/enterprise/hosts/'+self.map['hosts']['collector']['host_id']+'/collector',lambda c:c['status']=='converged',timeout=180,label='Collector converged');c=self.api.call('GET',path)
        if c['desired_enabled']!=enabled:self.api.confirm(self.api.call('POST','/monitoring/commands/preview',{'kind':'collection','operation':'set_enabled','id':c['id'],'expected_version':c['version'],'collection':{'enabled':enabled}},key='switch-'+uuid.uuid4().hex,expected=201))
        return self.api.wait(path,lambda c:c['desired_enabled']==enabled and c['apply_status']=='effective' and c['desired_revision']==c['effective_revision'],timeout=180,label='Collection intent applied')
    def lifecycle(self):
        start=now()
        try:
            stopped=self.switch(False)
            for p in self.map['alert_policies'].values():self.api.wait('/alerts/configurations/'+p['id'],lambda x:x['health']=='not_evaluated',timeout=90,label='Evaluation suspended')
            self.api.wait('/monitoring/collections/'+self.map['targets']['b']['collection_id'],lambda x:x['health']=='healthy' and x['freshness']=='fresh' and x['last_sample_at']>start,timeout=90,label='MongoDB B remains fresh')
        finally:
            restored=self.switch(True);self.api.wait('/monitoring/collections/'+self.map['targets']['a']['collection_id'],lambda c:c['health']=='healthy' and c['freshness']=='fresh',timeout=180,label='MongoDB A resumed')
            for p in self.map['alert_policies'].values():self.api.wait('/alerts/configurations/'+p['id'],lambda x:x['health']=='normal',timeout=90,label='Evaluation resumed')
        write_json(self.run.out/'lifecycle-checks.json',{'started_at':start,'finished_at':now(),'stopped_revision':stopped['desired_revision'],'restored_revision':restored['effective_revision'],'b_remained_fresh':True,'both_policies_suspended':True,'restored':True})
    def verify(self):
        reports=[]
        for side,t in self.map['targets'].items():
            c=self.api.call('GET','/monitoring/collections/'+t['collection_id']);assert c['desired_enabled'] and c['health']=='healthy' and c['freshness']=='fresh' and c['desired_revision']==c['effective_revision']
            rows=self.query('mongodb_connection_count{type="current"}',side)['data']['result'];assert rows and all(x['metric']['argus_resource_id']==t['target_id'] and x['metric']['argus_collection_id']==t['collection_id'] for x in rows)
            native=self.run.client('verify',side);assert native['documents']==(200 if side=='a' else 80) and native['actual_counter_updates']>0
            reports.append({'side':side,'name':t['name'],'health':c['health'],'last_sample_at':c['last_sample_at'],'isolated':True,'native':native})
        write_json(self.run.out/'monitoring-verification.json',{'observed_at':now(),'targets':reports});self.platform.dashboards()
