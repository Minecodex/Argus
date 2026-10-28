"""Enroll actual databases through the normal public monitoring workflow."""
import argparse
import json
from pathlib import Path
import sys
from manage import Run,HOSTS,DATABASES,write_json
sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'common'))
from api import Api
from hosts import HostEnrollment
from monitoring import MonitoringBatch

ENGINES=['mysql','postgresql']

class B03(MonitoringBatch):
    def __init__(self,run,api):super().__init__(run,api,ENGINES)

    def targets(self):
        collector=self.map['hosts']['collector']['collector_id']
        credentials=json.loads((self.run.out/'private/database-credentials.json').read_text(encoding='utf-8'))
        for key in DATABASES:
            engine='mysql' if key.startswith('mysql') else 'postgresql'
            name='B03-'+('MySQL' if engine=='mysql' else 'PostgreSQL')+'-'+key[-1].upper()
            identity={'endpoint':key+(':3306' if engine=='mysql' else ':5432')}
            if engine=='postgresql':identity['database']='plan9'
            rows=self.api.call('GET','/monitoring/collections?search='+name)['items'];matching=[x for x in rows if x['name']==name]
            assert len(matching)<=1
            if not matching:
                preview=self.api.call('POST','/monitoring/commands/preview',{'kind':'collection','operation':'create',
                    'credentials':{'username':'monitor','password':credentials[key]['monitor']},'collection':{
                    'name':name,'integration':engine,'collector_id':collector,'target_name':name,'target_identity':identity,
                    'enabled':True,'credential_update':'replace','config':{'interval':'10s','timeout':'5s','tls':{'enabled':False}}}},key='target-'+key,expected=201)
                self.api.confirm(preview)
                matching=[x for x in self.api.call('GET','/monitoring/collections?search='+name)['items'] if x['name']==name]
            assert len(matching)==1
            row=matching[0]
            self.map['targets'][key]={'name':name,'engine':engine,'host':'collector','target_id':row['target']['id'],'collection_id':row['id'],'collector_id':collector,'identity':identity}
            self.save()
            row=self.api.wait('/monitoring/collections/'+row['id'],lambda x:x['apply_status']=='effective' and x['health']=='healthy' and x['freshness']=='fresh',timeout=180,label=name+' actual collection')
            write_json(self.run.out/(key+'-collection.json'),row)
            print(name+': native metrics fresh',flush=True)

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['onboard','collectors','builtins','targets','dashboards','alerts','exercise','lifecycle','verify','credentials']);p.add_argument('--session',required=True);p.add_argument('--ca',required=True);args=p.parse_args()
    run=Run();api=Api(run,args.session,args.ca);api.call('GET','/enterprise/auth/session')
    if args.action in ['onboard','collectors','builtins']:getattr(HostEnrollment(run,api,HOSTS,'b03'),args.action)()
    elif args.action in ['targets','dashboards']:getattr(B03(run,api),args.action)()
    else:
        from scenario import Scenario
        getattr(Scenario(run,api),args.action)()
    api.save()
