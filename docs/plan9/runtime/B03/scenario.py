"""Bounded real connections/locks, credential failure and collection isolation."""
import json
import secrets
import time
import uuid
from manage import DATABASES,write_json,now
from integration import B03

RULES={'mysql':'max by (argus_resource_id) (mysql_threads{kind="connected"})',
       'postgresql':'sum by (argus_resource_id) (postgresql_backends)'}
KEYS={'mysql':'mysql-a','postgresql':'postgres-a'}

class Scenario:
    def __init__(self,run,api):
        self.run,self.api=run,api;self.platform=B03(run,api);self.map=self.platform.map

    def value(self,key):
        target=self.map['targets'][key]
        result=self.platform.query(target,RULES[target['engine']]);rows=result['data']['result']
        assert len(rows)==1
        return float(rows[0]['value'][1])

    def alerts(self):
        common=json.loads((self.run.out.parent/'common-evaluator.json').read_text(encoding='utf-8'));self.map['evaluator']=common
        for kind,ids in [('host',[x['host_id'] for x in self.map['hosts'].values()]),('monitoring_target',[x['target_id'] for x in self.map['targets'].values()])]:
            path='/enterprise/data-authorizations/role/'+common['role_id'];existing=self.api.call('GET',path+'?resource_type='+kind)
            add=[i for i in ids if not any(g['direct'] and g['resource_id']==i for g in existing['items'])]
            if add:self.api.call('POST',path,{'resource_type':kind,'resource_ids':add,'remove':False,'expected_version':existing['authorization_version']},expected=204)
        self.platform.save()
        for engine,expression in RULES.items():
            key=KEYS[engine];name='B03 '+('MySQL' if engine=='mysql' else 'PostgreSQL')+' connections high'
            row=next((p for p in self.api.call('GET','/alerts/configurations?kind=policy') if p['name']==name),None)
            if row is None:
                baseline=self.value(key);assert baseline<=4
                spec={'description':'Plan9 B03: 10 actual additional client connections; thresholds frozen before load','enabled':True,
                    'resources':{'mode':'explicit','types':['monitoring_target'],'ids':[self.map['targets'][key]['target_id']],'matchers':[]},
                    'rules':[{'name':name,'expression':expression,'query_mode':'numeric','operator':'>','threshold':8,'recovery_threshold':4,'severity':'warning','unit':''}],
                    'mode':'independent','match_by':['argus_resource_id'],'interval_seconds':10,'for_seconds':20,'recovery_seconds':20,
                    'no_data_behavior':'report','error_behavior':'keep','channel_ids':[],'route_ids':[],'labels':{'plan9_batch':'b03','plan9_run':self.run.id}}
                write_json(self.run.out/(engine+'-alert-baseline.json'),{'observed_at':now(),'connections':baseline,'frozen_spec':spec})
                trial=self.api.call('POST','/alerts/trial',spec);assert trial['health']=='normal'
                write_json(self.run.out/(engine+'-trial.json'),trial)
                self.api.confirm(self.api.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'create','name':name,'spec':spec},key='policy-'+engine,expected=202))
                row=next(p for p in self.api.call('GET','/alerts/configurations?kind=policy') if p['name']==name)
            self.map['alert_policies'][engine]={'id':row['id'],'name':name,'expression':expression,'threshold':8,'recovery_threshold':4}
            self.platform.save();self.run.event('policy_ready_before_load',engine=engine,policy_id=row['id'],threshold=8)
            print(engine+': alert configured from actual baseline',flush=True)

    def wait_alert(self,engine,state,ids=None):
        path='/alerts/'+('history' if state=='resolved' else 'instances')+'?policy_id='+self.map['alert_policies'][engine]['id']
        return self.api.wait(path,lambda result:any(x['lifecycle']==state and x['health']=='normal' and x['state']['kind']=='condition' and (not ids or x['id'] in ids) for x in result['items']),timeout=150,label=engine+' '+state)

    def exercise(self):
        path=self.run.out/'scenario-checks.json';reports=json.loads(path.read_text(encoding='utf-8')) if path.exists() else {}
        for engine,key in KEYS.items():
            if reports.get(engine,{}).get('recovered'):continue
            started=now()
            try:
                self.run.shell('collector',f'systemd-run --unit=plan9-connections-{key}-{uuid.uuid4().hex[:8]} --property=RuntimeMaxSec=190 --property=MemoryMax=96M python3 /usr/local/lib/plan9-db-client.py connections {key} --seconds 180\n')
                event=self.wait_alert(engine,'firing');ids={x['id'] for x in event['items'] if x['lifecycle']=='firing' and x['state']['kind']=='condition'}
                write_json(self.run.out/(engine+'-firing.json'),event)
                current=self.value(key);control=self.value(key[:-1]+'b');assert current>8 and control<=4
                native=json.loads(self.run.client('verify',key));write_json(self.run.out/(engine+'-native-during-load.json'),native)
                lock=json.loads(self.run.client('lock',key,20));assert lock['wait_seconds']>=19 and int(lock['blocked_native'][0][-1])>=1
                write_json(self.run.out/(engine+'-lock.json'),lock)
                print(engine+': real connections, lock wait and healthy B verified',flush=True)
            finally:self.run.client('recover',key)
            recovered=self.wait_alert(engine,'resolved',ids);write_json(self.run.out/(engine+'-recovered.json'),recovered)
            reports[engine]={'started_at':started,'finished_at':now(),'firing_ids':sorted(ids),'recovered':True,'healthy_b_connections':control,'a_connections_during_load':current,'lock_released':True}
            write_json(path,reports);print(engine+': recovery recorded',flush=True)

    def converged(self):
        return self.api.wait('/enterprise/hosts/'+self.map['hosts']['collector']['host_id']+'/collector',lambda c:c['status']=='converged',timeout=180,label='Collector configuration converged')

    def change(self,key,enabled=None,password=None):
        target=self.map['targets'][key];path='/monitoring/collections/'+target['collection_id']
        self.converged();row=self.api.call('GET',path)
        if password is None:
            if row['desired_enabled']==enabled:return row
            body={'kind':'collection','operation':'set_enabled','id':row['id'],'expected_version':row['version'],'collection':{'enabled':enabled}}
        else:
            body={'kind':'collection','operation':'update','id':row['id'],'expected_version':row['version'],'credentials':{'username':'monitor','password':password},'collection':{
                'name':row['name'],'integration':row['integration'],'target':row['target'],'collector_id':row['collector_id'],'enabled':row['desired_enabled'],'credential_update':'replace','config':row['config']}}
        self.api.confirm(self.api.call('POST','/monitoring/commands/preview',body,key='update-'+key+'-'+uuid.uuid4().hex,expected=201))
        result=self.api.wait(path,lambda c:c['apply_status']=='effective' and c['effective_revision']==c['desired_revision'],timeout=180,label='Collection configuration applied')
        self.converged();return result

    def credentials(self):
        credentials=json.loads((self.run.out/'private/database-credentials.json').read_text(encoding='utf-8'));reports=[]
        for key in ['mysql-a','postgres-a']:
            start=now();path='/monitoring/collections/'+self.map['targets'][key]['collection_id'];before=self.api.call('GET',path)
            try:
                self.change(key,password='wrong-'+secrets.token_urlsafe(25))
                # Native database receivers emit no target points on auth failure.
                # Current product observation is unknown/stale, unlike Prometheus
                # receivers which emit an explicit up=0 diagnostic. Preserve this
                # limitation instead of inferring an authentication error from silence.
                failure=self.api.wait(path,lambda c:c['freshness']=='stale' and c['health']!='healthy',timeout=160,label='No fresh target samples after credential failure')
                assert failure['last_attempt_at']>start
                needle='Access denied for user' if key.startswith('mysql') else 'password authentication failed'
                native_count=self.run.shell('collector',"journalctl -u argus-otelcol --since '"+start+"' --no-pager -o cat | python3 -c \"import sys; print(sum('"+needle+"' in line and '"+('mysql/' if key.startswith('mysql') else 'postgresql/')+"' in line for line in sys.stdin))\"\n")
                assert int(native_count)>0,'native authentication rejection not observed'
                controls=[]
                for other in DATABASES:
                    if other==key:continue
                    c=self.api.call('GET','/monitoring/collections/'+self.map['targets'][other]['collection_id'])
                    assert c['health']=='healthy' and c['freshness']=='fresh' and c['last_sample_at']>start;controls.append(other)
                write_json(self.run.out/(key+'-wrong-credential.json'),failure)
                print(key+': real authentication failure, other targets fresh',flush=True)
            finally:
                self.change(key,password=credentials[key]['monitor'])
                restored=self.api.wait(path,lambda c:c['health']=='healthy' and c['freshness']=='fresh' and c['last_sample_at']>start,timeout=180,label='Original credential restored')
                write_json(self.run.out/(key+'-credential-restored.json'),restored)
            reports.append({'target':key,'started_at':start,'finished_at':now(),'error_code':failure['error_code'],'platform_health_during_failure':failure['health'],'platform_freshness_during_failure':failure['freshness'],'native_authentication_rejections':int(native_count),'recovered':True,'healthy_controls':controls,'previous_revision':before['desired_revision'],'restored_revision':restored['effective_revision'],'diagnostic_limitation':'B03-D01: native receiver auth cause remains in Collector log; platform only shows unknown/stale'})
            write_json(self.run.out/'credential-checks.json',reports)

    def lifecycle(self):
        key='mysql-a';start=now();target=self.map['targets'][key];policy='/alerts/configurations/'+self.map['alert_policies']['mysql']['id']
        try:
            stopped=self.change(key,enabled=False)
            self.api.wait(policy,lambda x:x['health']=='not_evaluated',timeout=90,label='Explicit stop suspends evaluation')
            for other in DATABASES[1:]:
                self.api.wait('/monitoring/collections/'+self.map['targets'][other]['collection_id'],lambda c:c['freshness']=='fresh' and c['last_sample_at']>start,timeout=60,label=other+' remains collecting')
        finally:
            restored=self.change(key,enabled=True)
            self.api.wait('/monitoring/collections/'+target['collection_id'],lambda c:c['health']=='healthy' and c['freshness']=='fresh',timeout=120,label='Stopped collection restored')
            self.api.wait(policy,lambda x:x['health']=='normal',timeout=90,label='Alert evaluation resumed')
        write_json(self.run.out/'lifecycle-checks.json',{'started_at':start,'finished_at':now(),'independently_stopped':key,'other_three_fresh':True,'policy_health_during_stop':'not_evaluated','restored':True,'stopped_revision':stopped['desired_revision'],'restored_revision':restored['effective_revision']})

    def verify(self):
        report={'observed_at':now(),'targets':[]}
        for key,target in self.map['targets'].items():
            row=self.api.call('GET','/monitoring/collections/'+target['collection_id'])
            assert row['health']=='healthy' and row['freshness']=='fresh' and row['desired_enabled'] and row['effective_revision']==row['desired_revision']
            query=self.platform.query(target,'mysql_threads' if target['engine']=='mysql' else 'postgresql_backends')
            assert query['data']['result']
            assert all(x['metric']['argus_resource_id']==target['target_id'] and x['metric']['argus_collection_id']==target['collection_id'] for x in query['data']['result'])
            native=json.loads(self.run.client('verify',key));assert native['row_count']==(200 if key.endswith('a') else 50) and native['updates']>0
            write_json(self.run.out/(key+'-final-native.json'),native)
            report['targets'].append({'name':target['name'],'target_id':target['target_id'],'last_sample_at':row['last_sample_at'],'healthy':True,'isolated':True,'credential_configured':row['credential_configured'],'configuration_revision':row['effective_revision'],'row_count':native['row_count']})
        write_json(self.run.out/'monitoring-verification.json',report)
        self.platform.dashboards()
