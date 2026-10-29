"""Enroll the owned RabbitMQ through normal Argus APIs and confirmations."""
import argparse
import json
from pathlib import Path
import sys
from manage import Run,HOSTS,write_json
sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'common'))
from api import Api
from hosts import HostEnrollment
from monitoring import MonitoringBatch

class B05a(MonitoringBatch):
    def __init__(self,run,api):super().__init__(run,api,['rabbitmq'])
    def targets(self):
        name='B05a-RabbitMQ';collector=self.map['hosts']['collector']['collector_id'];identity={'endpoint':'http://rabbitmq:15672'}
        rows=self.api.call('GET','/monitoring/collections?search='+name)['items'];matching=[x for x in rows if x['name']==name];assert len(matching)<=1
        if not matching:
            private=json.loads((self.run.out/'private/rabbitmq-credentials.json').read_text(encoding='utf-8'))
            body={'kind':'collection','operation':'create','credentials':{'username':'monitor','password':private['monitor']},'collection':{'name':name,'integration':'rabbitmq','collector_id':collector,'target_name':name,'target_identity':identity,'enabled':True,'credential_update':'replace','config':{'interval':'10s','timeout':'5s','tls':{'enabled':False}}}}
            self.api.confirm(self.api.call('POST','/monitoring/commands/preview',body,key='rabbitmq-target',expected=201))
            matching=[x for x in self.api.call('GET','/monitoring/collections?search='+name)['items'] if x['name']==name]
        assert len(matching)==1;row=matching[0]
        self.map['targets']['rabbitmq']={'name':name,'engine':'rabbitmq','host':'collector','target_id':row['target']['id'],'collection_id':row['id'],'collector_id':collector,'identity':identity};self.save()
        row=self.api.wait('/monitoring/collections/'+row['id'],lambda x:x['apply_status']=='effective' and x['health']=='healthy' and x['freshness']=='fresh',timeout=180,label='RabbitMQ real native metrics')
        write_json(self.run.out/'rabbitmq-collection.json',row);print('RabbitMQ native data fresh',flush=True)

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['onboard','collectors','builtins','targets','dashboards','alerts','exercise','lifecycle','verify']);p.add_argument('--session',required=True);p.add_argument('--ca',required=True);a=p.parse_args()
    run=Run();api=Api(run,a.session,a.ca);api.call('GET','/enterprise/auth/session')
    if a.action in ['onboard','collectors','builtins']:getattr(HostEnrollment(run,api,HOSTS,'b05a'),a.action)()
    elif a.action in ['targets','dashboards']:getattr(B05a(run,api),a.action)()
    else:
        from scenario import Scenario
        getattr(Scenario(run,api),a.action)()
    api.save()
