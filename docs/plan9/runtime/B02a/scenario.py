"""Real connection pressure, backend faults, native checks and alert evidence."""
from datetime import datetime, timezone
import csv
import io
import json
import time

from manage import write_json, now


class Scenario:
    def __init__(self, run, api):
        from integration import B02a
        self.run, self.api = run, api
        self.platform = B02a(run, api)
        self.map = self.platform.map

    def query(self, key, expression):
        return self.platform.query(self.map['targets'][key], expression)

    def alerts(self):
        evaluator=json.loads((self.run.out.parent/'common-evaluator.json').read_text(encoding='utf-8'))
        self.map['evaluator']=evaluator
        for kind, ids in [('host',[x['host_id'] for x in self.map['hosts'].values()]),
                          ('monitoring_target',[x['target_id'] for x in self.map['targets'].values()])]:
            path='/enterprise/data-authorizations/role/'+evaluator['role_id']
            current=self.api.call('GET',path+'?resource_type='+kind)
            missing=[x for x in ids if not any(v['resource_id']==x and v['direct'] for v in current['items'])]
            if missing:
                self.api.call('POST',path,{'resource_type':kind,'resource_ids':missing,'remove':False,
                    'expected_version':current['authorization_version']},expected=204)
        expressions={
            'nginx':('sum by (argus_resource_id) (nginx_connections_current{state="active"})',8,4,'connections'),
            'apache':('sum by (argus_resource_id) (apache_workers{state="busy"})',8,4,'workers'),
            'haproxy':('sum by (argus_resource_id) (rate(haproxy_requests_total{resource_haproxy_proxy_name="web",resource_haproxy_service_name="FRONTEND",status_code="5xx"}[30s]))',0.1,0,'req/s'),
        }
        policies=self.api.call('GET','/alerts/configurations?kind=policy')
        for engine,(expression,threshold,recovery,unit) in expressions.items():
            key='web-a-'+engine
            name='B02a '+{'nginx':'Nginx active connections','apache':'Apache busy workers','haproxy':'HAProxy backend HTTP 5xx'}[engine]
            matches=[p for p in policies if p['name']==name]
            if matches:
                row=matches[0]
            else:
                baseline=self.query(key,expression)
                rows=baseline['data']['result']
                assert len(rows)==1 and float(rows[0]['value'][1])<=recovery, (engine,'baseline not healthy')
                write_json(self.run.out/(engine+'-baseline.json'),baseline)
                spec={'description':'Plan9 B02a test policy; thresholds frozen before real load/fault',
                    'enabled':True,'resources':{'mode':'explicit','types':['monitoring_target'],
                    'ids':[self.map['targets'][key]['target_id']],'matchers':[]},
                    'rules':[{'name':name,'expression':expression,'query_mode':'numeric','operator':'>',
                              'threshold':threshold,'recovery_threshold':recovery,'severity':'warning','unit':unit}],
                    'mode':'independent','match_by':['argus_resource_id'],'interval_seconds':10,
                    'for_seconds':20,'recovery_seconds':20,'no_data_behavior':'report','error_behavior':'keep',
                    'channel_ids':[],'route_ids':[],'labels':{'plan9_run':self.run.id,'plan9_batch':'b02a'}}
                trial=self.api.call('POST','/alerts/trial',spec)
                write_json(self.run.out/(engine+'-trial.json'),trial)
                assert trial['health']=='normal',engine+' trial failed'
                preview=self.api.call('POST','/alerts/configurations/preview',{
                    'kind':'policy','operation':'create','name':name,'spec':spec},key='policy-'+engine,expected=202)
                self.api.confirm(preview)
                row=next(p for p in self.api.call('GET','/alerts/configurations?kind=policy') if p['name']==name)
            self.map['alert_policies'][engine]={'id':row['id'],'name':name,'target':key,
                'expression':expression,'threshold':threshold,'recovery_threshold':recovery}
            self.platform.save()
            self.run.event('policy_ready_before_fault',engine=engine,policy_id=row['id'],threshold=threshold)
            print(engine+': scoped alert configured',flush=True)

    def wait_alert(self, engine, lifecycle, instance_ids=None):
        path='/alerts/'+('history' if lifecycle=='resolved' else 'instances')+'?policy_id='+self.map['alert_policies'][engine]['id']
        return self.api.wait(path,lambda result:any(x['lifecycle']==lifecycle and x['health']=='normal'
            and x['state']['kind']=='condition' and (not instance_ids or x['id'] in instance_ids)
            for x in result['items']),timeout=180,label=engine+' '+lifecycle)

    def native(self, host):
        data={}
        data['nginx']=self.run.shell(host,'curl -fsS http://127.0.0.1:8081/nginx_status\n')
        data['apache']=self.run.shell(host,'curl -fsS "http://127.0.0.1:8082/server-status?auto"\n')
        raw=self.run.shell(host,'curl -fsS "http://127.0.0.1:8404/stats;csv"\n')
        data['haproxy']=[row for row in csv.DictReader(io.StringIO(raw.removeprefix('# '))) if row.get('pxname') in ('web','application')]
        data['observed_at']=now()
        return data

    def exercise(self, refresh=False):
        prefix='refresh-' if refresh else ''
        record=self.run.out/(prefix+'scenario-checks.json')
        results=json.loads(record.read_text(encoding='utf-8')) if record.exists() else {}
        for engine in (['nginx','apache'] if refresh else ['nginx','apache','haproxy']):
            if results.get(engine,{}).get('recovered'):
                continue
            start=now()
            try:
                if engine=='haproxy':self.run.fault('web-a')
                else:self.run.load('web-a',engine)
                firing=self.wait_alert(engine,'firing')
                ids={x['id'] for x in firing['items'] if x['lifecycle']=='firing' and x['state']['kind']=='condition'}
                write_json(self.run.out/(prefix+engine+'-firing.json'),firing)
                write_json(self.run.out/(prefix+engine+'-native-during.json'),self.native('web-a'))
                print(engine+': actual alert firing',flush=True)
            finally:
                if engine=='haproxy':self.run.fault('web-a',True)
                else:self.run.shell('web-a','systemctl stop plan9-load-'+engine+'.service || true\n')
            restored=self.wait_alert(engine,'resolved',ids)
            write_json(self.run.out/(prefix+engine+'-recovered.json'),restored)
            results[engine]={'started_at':start,'finished_at':now(),'firing_ids':sorted(ids),'recovered':True}
            write_json(record,results)
            print(engine+': actual recovery recorded',flush=True)
        for host in ['web-a','web-b']:
            write_json(self.run.out/(prefix+host+'-native-snapshot.json'),self.native(host))

    def refresh(self):
        self.exercise(refresh=True)

    def retain(self):
        self.run.fault('web-a')
        current=self.wait_alert('haproxy','firing')
        write_json(self.run.out/'review-current-haproxy-firing.json',current)
        write_json(self.run.out/'review-current-native-a.json',self.native('web-a'))
        write_json(self.run.out/'review-current-native-b.json',self.native('web-b'))
        self.run.event('review_fault_retained',host='web-a',service='plan9-proxy-backend',
            policy_id=self.map['alert_policies']['haproxy']['id'])

    def lifecycle(self):
        target=self.map['targets']['web-a-nginx']
        path='/monitoring/collections/'+target['collection_id']
        policy_path='/alerts/configurations/'+self.map['alert_policies']['nginx']['id']
        original=self.api.call('GET',path)
        assert original['desired_enabled']
        def change(enabled):
            current=self.api.call('GET',path)
            if current['desired_enabled']==enabled:return
            preview=self.api.call('POST','/monitoring/commands/preview',{
                'kind':'collection','operation':'set_enabled','id':current['id'],
                'expected_version':current['version'],'collection':{'enabled':enabled}},
                key='lifecycle-'+str(current['version'])+'-'+str(enabled),expected=201)
            self.api.confirm(preview)
        report={'started_at':now()}
        try:
            change(False)
            stopped=self.api.wait(path,lambda x:not x['desired_enabled'] and x['apply_status']=='effective',timeout=120,label='Nginx target stop applied')
            health=self.api.wait(policy_path,lambda x:x['health']=='not_evaluated',timeout=90,label='Stopped target evaluation suspended')
            alerts=self.api.call('GET','/alerts/instances?policy_id='+self.map['alert_policies']['nginx']['id'])
            assert not any(x['state']['kind']=='health' for x in alerts['items'])
            for key,item in self.map['targets'].items():
                if key=='web-a-nginx':continue
                peer=self.api.call('GET','/monitoring/collections/'+item['collection_id'])
                assert peer['desired_enabled'] and peer['freshness']=='fresh' and peer['health']=='healthy'
            report.update(stopped_collection=stopped['id'],policy_health=health['health'],new_no_data_episodes=0,other_five_targets_fresh=True)
            print('Nginx independent stop: suspended evaluation; other five targets remain fresh',flush=True)
        finally:
            change(True)
            self.api.wait(path,lambda x:x['desired_enabled'] and x['apply_status']=='effective' and x['freshness']=='fresh' and x['health']=='healthy',timeout=180,label='Nginx target restored')
            self.api.wait(policy_path,lambda x:x['health']=='normal',timeout=90,label='Restored target evaluation normal')
            report.update(restored=True,finished_at=now())
            write_json(self.run.out/'lifecycle-checks.json',report)

    def verify(self):
        results={'checked_at':now(),'collections':[],'isolation':[]}
        for key,target in self.map['targets'].items():
            c=self.api.call('GET','/monitoring/collections/'+target['collection_id'])
            assert c['health']=='healthy' and c['freshness']=='fresh' and c['apply_status']=='effective'
            results['collections'].append({'target':key,'healthy':True,'fresh':True,'last_sample_at':c['last_sample_at']})
            wrong='apache_requests' if target['engine']=='nginx' else 'nginx_requests'
            assert not self.query(key,wrong)['data']['result'], 'cross-target data leaked'
            results['isolation'].append({'target':key,'unrelated_metric_empty':True})
        for host in ['web-a','web-b']:
            sampled=self.query(host+'-nginx','nginx_requests')['data']['result']
            native=self.native(host)
            # The status request itself increments Nginx's native request counter.
            native_count=int(native['nginx'].splitlines()[2].split()[2])
            assert len(sampled)==1 and 0<=native_count-float(sampled[0]['value'][1])<=30
            results[host]={'native_nginx_requests':native_count,'sampled_nginx_requests':float(sampled[0]['value'][1])}
        results['external_notification_channels']=0
        write_json(self.run.out/'monitoring-verification.json',results)
        self.platform.dashboards()
