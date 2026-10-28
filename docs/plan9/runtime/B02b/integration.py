"""Normal platform enrollment of the real B02b network probes."""
import argparse
from pathlib import Path
import sys
from manage import Run,HOSTS,write_json
sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'common'))
from api import Api
from hosts import HostEnrollment
from monitoring import MonitoringBatch

ENGINES=['http','tcp','tls','dns','icmp']


class B02b(MonitoringBatch):
    def __init__(self,run,api):super().__init__(run,api,ENGINES)

    def targets(self):
        collector=self.map['hosts']['probe']['collector_id']
        for side in ['a','b']:
            peer='target-'+side
            definitions={
                'http':{'endpoint':f'http://{peer}:8080/health' if side=='a' else f'https://{peer}:8443/health'},
                'tcp':{'endpoint':peer+':9000'},'tls':{'endpoint':peer+':8443'},
                'dns':{'endpoint':peer+':5353','hostnames':['app.b02b.test']},'icmp':{'endpoint':peer}}
            for engine,identity in definitions.items():
                key=peer+'-'+engine;name='B02b-'+('HTTPS' if engine=='http' and side=='b' else engine.upper())+'-'+side.upper()
                rows=self.api.call('GET','/monitoring/collections?search='+name)['items'];matching=[x for x in rows if x['name']==name]
                assert len(matching)<=1
                config={'interval':'10s','timeout':'3s','tls':{'enabled':False}}
                if engine=='dns':config.update(dns_network='udp',record_type='A')
                if engine=='icmp':config['ping_count']=3
                if not matching:
                    preview=self.api.call('POST','/monitoring/commands/preview',{'kind':'collection','operation':'create','collection':{
                        'name':name,'integration':engine,'collector_id':collector,'target_name':name,'target_identity':identity,
                        'enabled':True,'credential_update':'remove','config':config}},key='target-'+key,expected=201)
                    self.api.confirm(preview)
                    matching=[x for x in self.api.call('GET','/monitoring/collections?search='+name)['items'] if x['name']==name]
                assert len(matching)==1
                row=self.api.wait('/monitoring/collections/'+matching[0]['id'],lambda x:x['apply_status']=='effective' and x['health']=='healthy' and x['freshness']=='fresh',timeout=180,label=name+' native data')
                self.map['targets'][key]={'name':name,'engine':engine,'peer':peer,'host':'probe','target_id':row['target']['id'],'collection_id':row['id'],'collector_id':collector,'identity':identity}
                self.save();write_json(self.run.out/(key+'-collection.json'),row)
                print(name+': native probe fresh',flush=True)


if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['onboard','collectors','builtins','targets','dashboards','alerts','exercise','retain','verify','lifecycle','review']);p.add_argument('--session',required=True);p.add_argument('--ca',required=True);args=p.parse_args()
    run=Run();api=Api(run,args.session,args.ca);api.call('GET','/enterprise/auth/session')
    if args.action in ['onboard','collectors','builtins']:
        enrollment=HostEnrollment(run,api,HOSTS,'b02b');getattr(enrollment,args.action)()
    elif args.action in ['targets','dashboards']:getattr(B02b(run,api),args.action)()
    else:
        from scenario import Scenario
        scenario=Scenario(run,api)
        for stage in (['exercise','lifecycle','retain','verify'] if args.action=='review' else [args.action]):
            print(stage+': started',flush=True);getattr(scenario,stage)();print(stage+': completed',flush=True)
    api.save()
