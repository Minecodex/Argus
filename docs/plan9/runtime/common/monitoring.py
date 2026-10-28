"""Normal shared monitoring queries and managed dashboard attachment."""
from datetime import datetime,timedelta,timezone
import json

def write_json(path,value):
    path.write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')

class MonitoringBatch:
    def __init__(self,run,api,engines):
        self.run,self.api,self.engines=run,api,engines
        self.map=json.loads(run.resources.read_text(encoding='utf-8'))
    def save(self):
        write_json(self.run.resources,self.map)
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
        for engine in self.engines:
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
