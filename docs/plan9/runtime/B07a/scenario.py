"""Actual SDK request, error, dependency and three-signal verification."""
import json
import uuid
import time
from datetime import datetime
from manage import write_json,now
from integration import ApplicationBatch

from queries import apm,scope

REQUESTS='sum by (argus_resource_id) (rate(plan9_http_requests_total{resource_service_name="orders-gateway"}[1m]))'
RULES={'errors':{'query':'100 * sum by (argus_resource_id) (rate(plan9_http_errors_total{resource_service_name="orders-gateway"}[1m])) / '+REQUESTS,'threshold':25,'recovery':5,'name':'HTTP error ratio high','unit':'%'},
       'dependency':{'query':'1000 * sum by (argus_resource_id) (rate(plan9_http_duration_seconds_sum{resource_service_name="orders-gateway"}[1m])) / sum by (argus_resource_id) (rate(plan9_http_duration_seconds_count{resource_service_name="orders-gateway"}[1m]))','threshold':300,'recovery':150,'name':'HTTP dependency latency high','unit':'ms'}}

class Scenario:
    def __init__(self,run,api):self.run,self.api=run,api;self.platform=ApplicationBatch(run,api);self.map=self.platform.map;self.app=self.map['applications']['orders']
    def query(self,query,side='a'):return self.api.call('POST','/enterprise/metrics/query',{**scope(self.app['id']),'query':query})
    def value(self,kind,side='a'):
        rows=self.query(RULES[kind]['query'],side)['data']['result'];assert len(rows)==1;return float(rows[0]['value'][1])
    def alerts(self):
        common=json.loads((self.run.out.parent/'common-evaluator.json').read_text(encoding='utf-8'));self.map['evaluator']=common
        for kind,ids in [('host',[x['host_id'] for x in self.map['hosts'].values()]),('monitoring_target',[x['target_id'] for x in self.map['targets'].values()]),('application',[self.app['id']])]:
            path='/enterprise/data-authorizations/role/'+common['role_id'];old=self.api.call('GET',path+'?resource_type='+kind);add=[i for i in ids if not any(x['direct'] and x['resource_id']==i for x in old['items'])]
            if add:self.api.call('POST',path,{'resource_type':kind,'resource_ids':add,'remove':False,'expected_version':old['authorization_version']},expected=204)
        self.platform.save()
        for key,rule in RULES.items():
            name=f'{self.run.batch} {rule["name"]}';row=next((p for p in self.api.call('GET','/alerts/configurations?kind=policy') if p['name']==name),None)
            if row is None:
                baseline=self.value(key);assert baseline<=rule['recovery']
                spec={'description':f'Plan9 {self.run.batch} actual HTTP errors and Redis waits for this application','enabled':True,
                    'resources':{'mode':'explicit','types':['application'],'ids':[self.app['id']],'matchers':[]},
                    'rules':[{'name':name,'expression':rule['query'],'query_mode':'numeric','operator':'>','threshold':rule['threshold'],'recovery_threshold':rule['recovery'],'severity':'warning','unit':rule['unit']}],
                    'mode':'independent','match_by':['argus_resource_id'],'interval_seconds':10,'for_seconds':20,'recovery_seconds':20,'no_data_behavior':'report','error_behavior':'keep','channel_ids':[],'route_ids':[],'labels':{'plan9_batch':self.run.batch.lower(),'plan9_run':self.run.id}}
                write_json(self.run.out/(key+'-baseline.json'),{'observed_at':now(),'current':baseline,'frozen_spec':spec});trial=self.api.call('POST','/alerts/trial',spec);assert trial['health']=='normal';write_json(self.run.out/(key+'-trial.json'),trial)
                self.api.confirm(self.api.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'create','name':name,'spec':spec},key='policy-'+key,expected=202));row=next(p for p in self.api.call('GET','/alerts/configurations?kind=policy') if p['name']==name)
            self.map['alert_policies'][key]={'id':row['id'],'name':name,'expression':rule['query'],'threshold':rule['threshold'],'recovery_threshold':rule['recovery']};self.platform.save();self.run.event('policy_frozen_before_scenario',scenario=key,policy_id=row['id']);print(key+': alert configured',flush=True)
    def wait_alert(self,key,state,ids=None,started_at=None):
        path='/alerts/'+('history' if state=='resolved' else 'instances')+'?policy_id='+self.map['alert_policies'][key]['id']
        return self.api.wait(path,lambda r:any(x['lifecycle']==state and x['health']=='normal' and x['state']['kind']=='condition' and (not ids or x['id'] in ids) and (not started_at or datetime.fromisoformat(x['started_at'].replace('Z','+00:00'))>=datetime.fromisoformat(started_at)) for x in r['items']),timeout=150,label=key+' '+state)
    def exercise(self):
        path=self.run.out/'scenario-checks.json';reports=json.loads(path.read_text(encoding='utf-8')) if path.exists() else {}
        for key,rule in RULES.items():
            if reports.get(key,{}).get('recovered'):continue
            start=now();ids=set()
            try:
                self.run.fault(key);receipt=self.run.request(key);write_json(self.run.out/(key+'-http-receipt.json'),receipt)
                assert receipt['status']==(503 if key=='errors' else 200)
                if key=='dependency':assert receipt['duration_seconds']>=.55
                firing=self.wait_alert(key,'firing',started_at=start);ids={x['id'] for x in firing['items'] if x['lifecycle']=='firing' and x['state']['kind']=='condition' and datetime.fromisoformat(x['started_at'].replace('Z','+00:00'))>=datetime.fromisoformat(start)};write_json(self.run.out/(key+'-firing.json'),firing);observed=self.value(key);assert observed>rule['threshold']
                c=self.api.call('GET','/monitoring/collections/'+self.map['targets']['redis']['collection_id']);assert c['health']=='healthy' and c['freshness']=='fresh'
                trace=self.verify_receipt(receipt,key);write_json(self.run.out/(key+'-trace-proof.json'),trace);print(key+': real HTTP outcome, trace and alert verified',flush=True)
            finally:self.run.fault(key,True)
            write_json(self.run.out/(key+'-recovered.json'),self.wait_alert(key,'resolved',ids));receipt=self.run.request('recovered-'+key);assert receipt['status']==200 and receipt['duration_seconds']<.3
            reports[key]={'started_at':start,'finished_at':now(),'observed_value':observed,'firing_ids':sorted(ids),'recovered':True,'redis_remained_healthy':True,'recovery_request':receipt};write_json(path,reports);print(key+': actual recovery and alert resolution verified',flush=True)
        if not (self.run.out/'delay-checks.json').exists():
            try:
                self.run.fault('delay');items=[self.run.request('delay') for _ in range(3)];assert all(x['status']==200 and x['duration_seconds']>=.49 for x in items);write_json(self.run.out/'delay-checks.json',items)
            finally:self.run.fault('delay',True)

    def verify_receipt(self,receipt,label):
        app=self.app['id'];tid=receipt['response']['trace_id'];rid=receipt['request_id'];deadline=time.monotonic()+90
        while True:
            traces=apm(self.api,app,'traces',{'traceId':tid});items=traces['data']['queryTraces']['traces']
            if items and items[0]['spanCount']>=4:break
            if time.monotonic()>deadline:raise RuntimeError('Actual trace did not reach the scoped view')
            time.sleep(2)
        row=items[0];assert row['resourceId']==app and row['rootPresent'] and row['missingParentCount']==0
        detail=apm(self.api,app,'trace_detail',{'traceId':tid,'sourceId':row['sourceId'],'resourceId':app})['data']['queryTrace'];spans=detail['spans'];assert {s['serviceName'] for s in spans}=={'orders-gateway','orders-worker'};assert all(s['traceId']==tid and s['resourceId']==app for s in spans)
        assert any(s['operationName']=='Redis SET/GET' for s in spans)
        if label=='dependency':assert any(s['operationName']=='Redis BLPOP wait' for s in spans)
        while True:
            logs=self.api.call('POST','/enterprise/logs/query',{**scope(app),'query':'body : '+json.dumps(rid)})
            if len(logs['data'])>=2:break
            if time.monotonic()>deadline:raise RuntimeError('Actual correlated logs did not arrive')
            time.sleep(2)
        assert all(x['resource_id']==app and x['trace_id']==tid for x in logs['data']);write_json(self.run.out/(label+'-correlated-logs.json'),logs)
        return {'request_id':rid,'trace_id':tid,'trace':detail,'correlated_log_count':len(logs['data'])}

    def switch(self,enabled):
        self.platform.converge();rows=self.api.call('GET','/applications/'+self.app['id']+'/ingest-endpoints')['items'];current=next(e for e in rows if e['id']==self.app['endpoint_id'])
        if current['enabled']!=enabled:self.platform.commit({'kind':'endpoint','operation':'set_enabled','id':current['id'],'expected_version':current['version'],'endpoint':{'enabled':enabled}},'endpoint-switch-'+uuid.uuid4().hex)
        self.platform.converge()
        return self.api.wait('/monitoring/ingest-endpoints/'+current['id']+'/connection',lambda c:c['desired_enabled']==enabled and c['configuration_status']=='effective',timeout=180,label='SDK endpoint intent applied')
    def lifecycle(self):
        start=now()
        # Gracefully flush SDKs before disabling their endpoint; do not manufacture export loss.
        self.run.shell('collector','systemctl stop plan9-load\nsystemctl stop plan9-app@orders-gateway plan9-app@orders-worker\n')
        try:
            stopped=self.switch(False)
            write_json(self.run.out/'sdk-disabled-connection.json',stopped)
            # SDK applications currently follow their configured no-data policy;
            # native collection not_evaluated semantics must not be assumed here.
            port=next(x['port'] for x in stopped['listeners'] if x['transport']==self.run.listener_transport)
            probe=self.run.shell('collector',f"python3 -c \"import socket;s=socket.socket();s.settimeout(3);print(s.connect_ex(('127.0.0.1',{port})));s.close()\"\n")
            assert int(probe.strip())!=0
            auxiliary_ready = None
            if 'aux_ingest' in self.app:
                auxiliary = self.api.call('GET','/monitoring/ingest-endpoints/'+self.app['aux_ingest']['endpoint_id']+'/connection')
                auxiliary_ready = auxiliary['desired_enabled'] and auxiliary['configuration_status']=='effective'
                assert auxiliary_ready
            self.api.wait('/monitoring/collections/'+self.map['targets']['redis']['collection_id'],lambda c:c['health']=='healthy' and c['freshness']=='fresh' and c['last_sample_at']>start,timeout=90,label='Redis remains fresh')
            health={key:self.api.call('GET','/alerts/configurations/'+p['id'])['health'] for key,p in self.map['alert_policies'].items()}
        finally:
            self.switch(True);self.run.workload()
            self.api.wait('/monitoring/ingest-endpoints/'+self.app['endpoint_id']+'/connection',lambda c:bool(c.get('last_received_at')) and c['last_received_at']>start,timeout=120,label='SDK observations resumed')
            for p in self.map['alert_policies'].values():self.api.wait('/alerts/configurations/'+p['id'],lambda x:x['health']=='normal',timeout=120,label='Application evaluation resumed')
        write_json(self.run.out/'lifecycle-checks.json',{'started_at':start,'finished_at':now(),'endpoint_stop_restore':True,'sdk_transport':self.run.transport,'sdk_listener_closed_when_disabled':True,'auxiliary_endpoint_remained_effective':auxiliary_ready,'policy_health_when_disabled':health,'automatic_policy_suspension_verified':False,'redis_remained_fresh':True,'real_sdk_resumed':True})
    def verify(self):
        self.platform.services();reports={}
        for view in ['services','instances','endpoints','red','topology']:
            value=apm(self.api,self.app['id'],view);data=next(iter(value['data'].values()));assert data['status']=='available';write_json(self.run.out/('apm-'+view+'.json'),value);reports[view]={'status':data['status'],'rows':len(data.get('rows',[])),'nodes':len(data.get('nodes',[])),'edges':len(data.get('edges',[]))}
            if view in ['services','instances']:assert len(data['rows'])==2
            if view=='topology':assert len(data['nodes'])==2 and len(data['edges'])==1
        receipt=self.run.request('final');assert receipt['status']==200;worker=json.loads(receipt['response']['worker_response']);assert worker['redis_value']==receipt['request_id'] and worker['trace_id']==receipt['response']['trace_id'];write_json(self.run.out/'final-http-receipt.json',receipt);proof=self.verify_receipt(receipt,'final');write_json(self.run.out/'final-trace-proof.json',proof)
        metrics=self.query('plan9_http_requests_total');assert len(metrics['data']['result'])==2 and all(x['metric']['argus_resource_id']==self.app['id'] for x in metrics['data']['result']);write_json(self.run.out/'sdk-metrics-verification.json',metrics)
        write_json(self.run.out/'application-verification.json',{'observed_at':now(),'views':reports,'services_discovered':2,'request_trace_and_logs_correlated':True,'actual_redis_roundtrip':True,'mock_data':False})
        from dashboard import dashboards
        dashboards(self.platform)
