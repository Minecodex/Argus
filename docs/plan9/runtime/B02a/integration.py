"""B02a normal API enrollment, monitoring targets and dashboard verification."""
import argparse
from datetime import datetime, timedelta, timezone
import json
from pathlib import Path
import sys

from manage import Run, HOSTS, write_json, now
sys.path.insert(0, str(Path(__file__).resolve().parents[1]/'common'))
from api import Api
from hosts import HostEnrollment

ENGINES = {'nginx':(8081,'/nginx_status'), 'apache':(8082,'/server-status?auto'), 'haproxy':(8404,'/stats')}


class B02a:
    def __init__(self, run, api):
        self.run, self.api = run, api
        self.map=json.loads(run.resources.read_text(encoding='utf-8'))

    def save(self):
        write_json(self.run.resources,self.map)

    def targets(self):
        available=self.api.call('GET','/monitoring/integrations')
        write_json(self.run.out/'integration-capabilities.json',available)
        for host, state in self.map['hosts'].items():
            for engine,(port,path) in ENGINES.items():
                key=host+'-'+engine
                name='B02a-'+engine.title()+'-'+HOSTS[host]['letter']
                existing=self.api.call('GET','/monitoring/collections?search='+name)['items']
                matches=[x for x in existing if x['name']==name]
                assert len(matches)<=1
                if not matches:
                    preview=self.api.call('POST','/monitoring/commands/preview',{
                        'kind':'collection','operation':'create','collection':{
                            'name':name,'integration':engine,'collector_id':state['collector_id'],
                            'target_name':name,'target_identity':{'endpoint':f'http://{host}:{port}{path}'},
                            'enabled':True,'credential_update':'remove',
                            'config':{'interval':'10s','timeout':'5s','tls':{'enabled':False}}}},key='target-'+key,expected=201)
                    self.api.confirm(preview)
                    matches=[x for x in self.api.call('GET','/monitoring/collections?search='+name)['items'] if x['name']==name]
                assert len(matches)==1
                row=matches[0]
                current=self.api.wait('/monitoring/collections/'+row['id'],lambda x:x['apply_status']=='effective' and x['freshness']=='fresh' and x['health']=='healthy',timeout=180,label=name+' collection')
                self.map['targets'][key]={'name':name,'host':host,'engine':engine,'target_id':current['target']['id'],'collection_id':current['id'],'collector_id':state['collector_id'],'endpoint':f'http://{host}:{port}{path}'}
                self.save()
                write_json(self.run.out/(key+'-collection.json'),current)
                print(name+': native metrics fresh',flush=True)

    def query(self, target, expression, range_seconds=0):
        end=datetime.now(timezone.utc)
        path='/enterprise/metrics/query-range' if range_seconds else '/enterprise/metrics/query'
        body={'query':expression,'resource_ids':[target['target_id']],
              'time_range':{'from':(end-timedelta(seconds=max(range_seconds,300))).isoformat(),'to':end.isoformat()},
              'budget':{'max_scan_bytes':268435456,'max_rows':10000,'max_samples':1000000,'max_series':10000,'timeout_ms':10000,'max_result_bytes':8388608}}
        if range_seconds:body['step_seconds']=10
        return self.api.call('POST',path,body)

    def dashboards(self):
        boards=self.api.call('GET','/dashboards')
        for engine in ENGINES:
            matches=[x for x in boards if x.get('managed_template_key')==engine+'.overview']
            assert len(matches)==1,engine+' managed dashboard unavailable'
            self.map['dashboards'][engine]={'id':matches[0]['id'],'name':matches[0]['name']}
        self.save()
        reports=[]
        for key,target in self.map['targets'].items():
            board=next(x for x in boards if x['id']==self.map['dashboards'][target['engine']]['id'])
            path='/monitoring/targets/'+target['target_id']+'/dashboard-bindings'
            bindings=self.api.call('GET',path)
            if not any(x['dashboard']['id']==board['id'] for x in bindings['items']):
                preview=self.api.call('POST',path+'/preview',{'operation':'attach','dashboard_id':board['id'],
                    'expected_dashboard_version':board['version'],'expected_resource_version':bindings['resource_version']},key='bind-'+key,expected=201)
                self.api.confirm(preview)
            end=datetime.now(timezone.utc)
            result=self.api.call('POST','/dashboards/'+board['id']+'/execute',{
                'from':(end-timedelta(minutes=15)).isoformat(),'to':end.isoformat(),
                'resource_mode':'explicit','resource_types':['monitoring_target'],'resource_ids':[target['target_id']]})
            result.pop('context_token',None)
            write_json(self.run.out/(key+'-dashboard.json'),result)
            rows=[{'id':p['id'],'status':p['status']} for p in result['panels']]
            reports.append({'target':key,'dashboard':board['id'],'panels':rows})
            print(key+': '+str(rows),flush=True)
        write_json(self.run.out/'dashboard-checks.json',reports)
        assert all(p['status']=='success' for report in reports for p in report['panels'])


if __name__=='__main__':
    parser=argparse.ArgumentParser()
    parser.add_argument('action',choices=['onboard','collectors','builtins','targets','dashboards','alerts','exercise','retain','verify','lifecycle','review','refresh'])
    parser.add_argument('--run',default='p9-b02a-20261027-01')
    parser.add_argument('--session',required=True)
    parser.add_argument('--ca',required=True)
    args=parser.parse_args();run=Run(args.run);api=Api(run,args.session,args.ca)
    api.call('GET','/enterprise/auth/session')
    if args.action in ('onboard','collectors','builtins'):
        getattr(HostEnrollment(run,api,HOSTS,'b02a'),args.action)()
    elif args.action in ('targets','dashboards'):
        getattr(B02a(run,api),args.action)()
    else:
        from scenario import Scenario
        scenario=Scenario(run,api)
        for stage in (['exercise','lifecycle','retain','verify'] if args.action=='review' else [args.action]):
            print(stage+': started',flush=True)
            getattr(scenario,stage)()
            print(stage+': completed',flush=True)
    api.save()
