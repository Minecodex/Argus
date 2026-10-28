"""Real protocol failures, certificate changes and normal alert evaluation."""
import json
import time
import uuid
from manage import write_json,now
from integration import B02b

HTTP_STATUS='sum by (argus_resource_id) (httpcheck_status{http_status_class=~"2xx|3xx"} and topk by (argus_resource_id, http_status_class) (1, timestamp(httpcheck_status{http_status_class=~"2xx|3xx"})))'
DNS_STATUS='min by (argus_resource_id) (dnscheck_status and topk by (argus_resource_id, resource_dns_domain) (1, timestamp(dnscheck_status)))'
RULES={
    'http':(HTTP_STATUS,'<',0.5,1,''),
    'tcp':('min by (argus_resource_id) (tcpcheck_status)','<',0.5,1,''),
    'tls':('min by (argus_resource_id) (tlscheck_time_left) / 86400','<',7,14,'days'),
    'dns':(DNS_STATUS,'<',0.5,1,''),
    'icmp':('max by (argus_resource_id) (ping_loss_ratio)','>',50,0,'%'),
}


class Scenario:
    def __init__(self,run,api):
        self.run,self.api=run,api;self.platform=B02b(run,api);self.map=self.platform.map

    def query(self,engine,expression=None,side='a'):
        return self.platform.query(self.map['targets']['target-'+side+'-'+engine],expression or RULES[engine][0])

    def value(self,engine,expression=None,side='a'):
        rows=self.query(engine,expression,side)['data']['result']
        if len(rows)!=1:raise RuntimeError(engine+': expected one current value, got '+str(len(rows)))
        return float(rows[0]['value'][1])

    def wait_value(self,engine,predicate,expression=None,side='a',timeout=90):
        deadline=time.monotonic()+timeout
        while time.monotonic()<deadline:
            value=self.value(engine,expression,side)
            if predicate(value):return value
            time.sleep(2)
        raise RuntimeError(engine+': value did not converge')

    def alerts(self):
        common=json.loads((self.run.out.parent/'common-evaluator.json').read_text(encoding='utf-8'))
        self.map['evaluator']=common
        for kind,ids in [('host',[x['host_id'] for x in self.map['hosts'].values()]),('monitoring_target',[x['target_id'] for x in self.map['targets'].values()])]:
            path='/enterprise/data-authorizations/role/'+common['role_id'];existing=self.api.call('GET',path+'?resource_type='+kind)
            add=[i for i in ids if not any(g['direct'] and g['resource_id']==i for g in existing['items'])]
            if add:self.api.call('POST',path,{'resource_type':kind,'resource_ids':add,'remove':False,'expected_version':existing['authorization_version']},expected=204)
        policies=self.api.call('GET','/alerts/configurations?kind=policy')
        for engine,(expression,operator,threshold,recovery,unit) in RULES.items():
            name='B02b '+{'http':'HTTP unhealthy response','tcp':'TCP port unavailable','tls':'TLS certificate expires soon','dns':'DNS lookup failed','icmp':'ICMP packet loss'}[engine]
            row=next((p for p in policies if p['name']==name),None)
            if row is None:
                baseline=self.value(engine)
                assert baseline>=recovery if operator=='<' else baseline<=recovery
                write_json(self.run.out/(engine+'-baseline.json'),self.query(engine))
                spec={'description':'Plan9 B02b native probes; thresholds frozen before endpoint faults','enabled':True,
                    'resources':{'mode':'explicit','types':['monitoring_target'],'ids':[self.map['targets']['target-a-'+engine]['target_id']],'matchers':[]},
                    'rules':[{'name':name,'expression':expression,'query_mode':'numeric','operator':operator,'threshold':threshold,'recovery_threshold':recovery,'severity':'warning','unit':unit}],
                    'mode':'independent','match_by':['argus_resource_id'],'interval_seconds':10,'for_seconds':20,'recovery_seconds':20,
                    'no_data_behavior':'report','error_behavior':'keep','channel_ids':[],'route_ids':[],
                    'labels':{'plan9_batch':'b02b','plan9_run':self.run.id}}
                trial=self.api.call('POST','/alerts/trial',spec);assert trial['health']=='normal'
                write_json(self.run.out/(engine+'-trial.json'),trial)
                preview=self.api.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'create','name':name,'spec':spec},key='policy-'+engine,expected=202)
                self.api.confirm(preview)
                row=next(p for p in self.api.call('GET','/alerts/configurations?kind=policy') if p['name']==name)
            self.map['alert_policies'][engine]={'id':row['id'],'name':name,'expression':expression,'operator':operator,'threshold':threshold,'recovery_threshold':recovery}
            self.platform.save();self.run.event('policy_ready_before_fault',probe=engine,policy_id=row['id'],threshold=threshold)
            print(engine+': alert ready',flush=True)

    def wait_alert(self,engine,state,ids=None):
        path='/alerts/'+('history' if state=='resolved' else 'instances')+'?policy_id='+self.map['alert_policies'][engine]['id']
        return self.api.wait(path,lambda result:any(x['lifecycle']==state and x['health']=='normal' and x['state']['kind']=='condition' and (not ids or x['id'] in ids) for x in result['items']),timeout=150,label=engine+' '+state)

    def native(self,engine,side='a'):
        peer='target-'+side
        if engine=='http':command="curl -sS -o /dev/null -w '%{http_code}\\n' http://"+peer+':8080/health'
        elif engine=='dns':command='dig @'+peer+' -p 5353 app.b02b.test A +noall +comments +answer'
        elif engine=='icmp':command='ping -c 3 -W 1 '+peer
        elif engine=='tls':command='openssl s_client -brief -connect '+peer+':8443 -servername '+peer+' -verify_hostname '+peer+' -verify_return_error -CAfile /pki/ca.crt </dev/null 2>&1'
        else:command="python3 - <<'PY'\nimport socket\nwith socket.create_connection(('"+peer+"',9000),timeout=3) as connection:\n connection.sendall(b'actual TCP request')\n print(connection.recv(128).decode())\nPY"
        return {'observed_at':now(),'probe':engine,'peer':peer,'output':self.run.shell('probe','set +e\n'+command+'\nresult=$?\nprintf "NATIVE_EXIT=%s\\n" "$result"\nexit 0\n',timeout=20)}

    def exercise(self):
        path=self.run.out/'scenario-checks.json';reports=json.loads(path.read_text(encoding='utf-8')) if path.exists() else {}
        for engine in RULES:
            if reports.get(engine,{}).get('recovered'):continue
            start=now()
            try:
                self.run.fault(engine)
                event=self.wait_alert(engine,'firing');ids={x['id'] for x in event['items'] if x['state']['kind']=='condition' and x['lifecycle']=='firing'}
                write_json(self.run.out/(engine+'-firing.json'),event)
                write_json(self.run.out/(engine+'-native-during.json'),self.native(engine))
                control=self.value(engine,side='b')
                assert control>=RULES[engine][3] if RULES[engine][1]=='<' else control<=RULES[engine][3]
                if engine=='icmp':
                    assert self.value('icmp')==100
                    assert not self.query('icmp','ping_rtt_avg and (ping_loss_ratio < 100)')['data']['result']
                if engine=='tls':
                    self.run.fault('tls_expired')
                    expired=self.wait_value('tls',lambda n:n<0)
                    check=self.native('tls');assert 'NATIVE_EXIT=0' not in check['output']
                    write_json(self.run.out/'tls-expired-verified.json',{'remaining_days':expired,'native_strict_verification':check,'clock_modified':False})
                print(engine+': actual failure and healthy peer verified',flush=True)
            finally:self.run.fault(engine,True)
            recovered=self.wait_alert(engine,'resolved',ids)
            write_json(self.run.out/(engine+'-recovered.json'),recovered)
            reports[engine]={'started_at':start,'finished_at':now(),'firing_ids':sorted(ids),'recovered':True,'healthy_peer':True}
            write_json(path,reports);print(engine+': recovery recorded',flush=True)
        self.run.preflight()

    def lifecycle(self):
        target=self.map['targets']['target-a-tcp'];path='/monitoring/collections/'+target['collection_id']
        policy='/alerts/configurations/'+self.map['alert_policies']['tcp']['id']
        def change(enabled):
            for attempt in range(3):
                row=self.api.call('GET',path)
                if row['desired_enabled']!=enabled:
                    self.api.wait('/enterprise/hosts/'+self.map['hosts']['probe']['host_id']+'/collector',lambda c:c['status']=='converged',timeout=180,label='Collector configuration converged')
                    row=self.api.call('GET',path)
                    preview=self.api.call('POST','/monitoring/commands/preview',{'kind':'collection','operation':'set_enabled','id':row['id'],'expected_version':row['version'],'collection':{'enabled':enabled}},key='tcp-switch-'+str(row['version'])+'-'+str(enabled)+'-'+uuid.uuid4().hex,expected=201)
                    try:self.api.confirm(preview)
                    except RuntimeError as error:
                        if 'ACTION_INVALIDATED' not in str(error) or attempt==2:raise
                        self.run.event('retry_invalidated_preview',collection_id=row['id'],enabled=enabled)
                        continue
                self.api.wait(path,lambda c:c['desired_enabled']==enabled and c['apply_status']=='effective' and c['effective_revision']==c['desired_revision'],timeout=180,label='Collection intent applied')
                return
        report={'started_at':now()}
        try:
            change(False)
            self.api.wait(policy,lambda x:x['health']=='not_evaluated',timeout=90,label='TCP disabled evaluation suspended')
            for key,value in self.map['targets'].items():
                if key=='target-a-tcp':continue
                row=self.api.call('GET','/monitoring/collections/'+value['collection_id']);assert row['freshness']=='fresh' and row['desired_enabled']
            report.update(policy_health='not_evaluated',other_nine_targets_fresh=True)
        finally:
            change(True);self.api.wait(path,lambda x:x['apply_status']=='effective' and x['freshness']=='fresh' and x['health']=='healthy',timeout=180,label='TCP restored')
            self.api.wait(policy,lambda x:x['health']=='normal',timeout=90,label='TCP evaluation restored')
            report.update(restored=True,finished_at=now());write_json(self.run.out/'lifecycle-checks.json',report)

    def retain(self):
        for engine in ['http','tls']:
            self.run.fault(engine,retain=True)
            write_json(self.run.out/(engine+'-retained-firing.json'),self.wait_alert(engine,'firing'))
        self.run.event('review_faults_retained',http_status=503,tls_validity='two-day certificate',control='target-b healthy')

    def verify(self):
        report={'observed_at':now(),'collections':[],'values':{},'isolation':[]}
        for key,target in self.map['targets'].items():
            c=self.api.call('GET','/monitoring/collections/'+target['collection_id']);assert c['desired_enabled'] and c['apply_status']=='effective' and c['freshness']=='fresh'
            report['collections'].append({'target':key,'health':c['health'],'freshness':c['freshness'],'last_sample_at':c['last_sample_at']})
            wrong='ping_loss_ratio' if target['engine']!='icmp' else 'tcpcheck_status'
            assert not self.platform.query(target,wrong)['data']['result'];report['isolation'].append(key)
        for engine in RULES:
            report['values'][engine]={'A':self.value(engine),'B':self.value(engine,side='b')}
        assert report['values']['http']['A']==0 and report['values']['http']['B']==1
        assert 0<report['values']['tls']['A']<7 and report['values']['tls']['B']>14
        assert report['values']['dns']=={'A':1,'B':1} and report['values']['tcp']=={'A':1,'B':1}
        assert report['values']['icmp']=={'A':0,'B':0}
        write_json(self.run.out/'monitoring-verification.json',report)
        self.platform.dashboards()
