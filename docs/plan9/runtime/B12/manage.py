"""Fresh B12 host/cache infrastructure; no model calls without a selected budget."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import sys

HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[3]
sys.path.insert(0,str(HERE.parent))
from B01.manage import Run as OwnedRun,execute,write_json,now
from B09a.manage import Run as PlatformRun
from common.api import Api

RUN='p9-b12-20261116-01'
BASE='argus-plan9-host:b11c-base-20261115'

class Run(OwnedRun):
    def __init__(self):
        self.id=RUN;self.out=Path(os.environ.get('ARGUS_PLAN9_ARTIFACT_ROOT','C:/Users/23574/.codex/artifacts/argus-plan9'))/RUN
        self.private=self.out/'private';self.resources=self.out/'resource-map.json';self.envfile=self.out/'compose.env'
    def save(self,name,value):write_json(self.out/(name+'.json'),value)
    def platform(self):return PlatformRun().platform()
    def api(self):return Api(self,self.private/'enterprise-session.json',ROOT/'deploy/.cache/argus-trust-bundle.pem',
        {'enterprise_username':os.environ.get('ARGUS_PLAN9_USERNAME','kakj'),'password':os.environ['ARGUS_PLAN9_PASSWORD']})
    def compose(self,*args,timeout=120):
        return execute(['C:/Program Files/Docker/Docker/resources/cli-plugins/docker-compose.exe','--env-file',str(self.envfile),
            '-f',str(HERE/'compose.yaml'),'--project-name',self.id,*args],timeout=timeout)
    def container(self,key='direct'):
        assert key in ['direct','redis'];ident=self.compose('ps','--all','--quiet',key);assert ident
        value=json.loads(execute(['docker','inspect',ident]))[0]
        assert value['Config']['Labels']['io.argus.plan9.run']==self.id
        previous=next((row for row in json.loads(self.resources.read_text())['containers'] if row['service']==key),None)
        if previous:assert value['Id']==previous['id']
        return value
    def prepare(self):
        self.private.mkdir(parents=True,exist_ok=True)
        execute(['icacls',str(self.private),'/inheritance:r','/grant:r',execute(['whoami'])+':(OI)(CI)F','*S-1-5-18:(OI)(CI)F'])
        if not self.resources.exists():
            self.save('resource-map',{'run_id':self.id,'hosts':{},'containers':[],'targets':{},'dashboards':{},'alert_policies':{},'remote':{}})
            write_json(self.private/'host-credentials.json',{'direct':{'username':'root','password':secrets.token_urlsafe(32)}})
            write_json(self.private/'redis-credentials.json',{role:secrets.token_urlsafe(32) for role in ['monitor','app']})
            self.save('run',{'run_id':self.id,'batch':'B12','status':'preparing','created_at':now(),'human_acceptance':'pending',
                'authorization':'没问题，开始下一轮','model_calls':0,'model_budget':None,'direct_business_database_writes':False,
                'limits':{'cpus':2,'memory_mib':1472,'traffic_seconds':28800,'redis_capacity_mib':16,'cached_order_keys_max':64}})
        if not (self.private/'enterprise-session.json').exists():write_json(self.private/'enterprise-session.json',{'origin':'https://argus.dev','csrf':'','user':{},'cookies':[]})
        values=json.loads((self.private/'redis-credentials.json').read_text());acl='user default off\n'
        for role,commands in [('monitor','+info +config|get +ping'),('app','~p9:b12:* +get +set +del +ping')]:
            acl+='user '+role+' on #'+hashlib.sha256(values[role].encode()).hexdigest()+' '+commands+'\n'
        (self.private/'users.acl').write_text(acl,encoding='utf-8')
        (self.private/'redis.conf').write_text('bind 0.0.0.0\nprotected-mode yes\nport 6379\naclfile /run/secrets/users.acl\nmaxmemory 16mb\nmaxmemory-policy allkeys-lru\nsave ""\nappendonly no\n',encoding='utf-8')
        api=self.api();api.login();models=api.call('GET','/enterprise/ai-models');models=models['items'] if isinstance(models,dict) else models
        self.save('model-preflight',{'at':now(),'models':[{k:row[k] for k in ['id','name','model_id','enabled','health_status'] if k in row} for row in models],
            'available_configuration_count':len(models),'real_model_calls':0,'budget':None,'required_user_input':'Select a tested model and explicit per-batch call/token/cost budget'})
        print('New B12 private credentials and real-scene package prepared; model calls remain disabled pending configuration/budget',flush=True)
    def build(self):
        base=json.loads(execute(['docker','image','inspect',BASE]))[0]
        assert base['Architecture']=='amd64'
        tag='argus-plan9-cache:b12-20261116'
        execute(['docker','build','--platform','linux/amd64','--build-arg','PLAN9_BASE='+BASE,'-t',tag,str(HERE)],timeout=240)
        assert json.loads(execute(['docker','image','inspect',BASE]))[0]['Id']==base['Id']
        redis='redis:8.10.0-alpine'
        execute(['docker','pull',redis],timeout=180)
        images={key:json.loads(execute(['docker','image','inspect',ref]))[0] for key,ref in [('host',tag),('redis',redis)]}
        env={'PLAN9_RUN':self.id,'PLAN9_HOST_IMAGE':images['host']['Id'],'PLAN9_REDIS_IMAGE':images['redis']['Id'],'PLAN9_PRIVATE':self.private.as_posix()}
        self.envfile.write_text(''.join(key+'='+value+'\n' for key,value in env.items()),encoding='utf-8')
        self.compose('config','--quiet')
        self.save('image-lock',{'base_id':base['Id'],'images':{key:{'id':value['Id'],'repo_digests':value['RepoDigests']} for key,value in images.items()},
            'source':{path.name:hashlib.sha256(path.read_bytes()).hexdigest() for path in HERE.iterdir() if path.is_file()}})
        print('Bounded real host/Redis images built and pinned; no formal image changed',flush=True)
    def up(self):
        assert json.loads((self.out.parent/'p9-b11c-20261115-01/cleanup-verification.json').read_text())['status']=='destroyed'
        mapping=json.loads(self.resources.read_text());assert not mapping['containers']
        self.save('platform-before',self.platform())
        self.compose('up','-d','--no-build','--wait','--wait-timeout','120',timeout=180)
        password=json.loads((self.private/'host-credentials.json').read_text())['direct']['password']
        execute(['docker','exec','-i',self.container()['Id'],'chpasswd'],data='root:'+password+'\n')
        for key in ['direct','redis']:
            value=self.container(key);mapping['containers'].append({'service':key,'id':value['Id'],'image_id':value['Image']});self.save('resource-map',mapping)
        host=self.container();self.save('host-specs',{'direct':{'name':'B12-Cache-Host','letter':'A','address':host['NetworkSettings']['Networks']['kind']['IPAddress'],'ssh_port':22}})
        assert self.platform()==json.loads((self.out/'platform-before.json').read_text())
        print('Fresh B12 Host and actual Redis/HTTP service healthy; prior scenario retired',flush=True)
    def workload(self):
        if self.shell('direct','systemctl is-active plan9-b12-traffic.service || true\n')!='active':
            self.shell('direct','systemd-run --collect --unit=plan9-b12-traffic --property=RuntimeMaxSec=28800 --property=MemoryMax=64M --property=CPUQuota=5% /usr/bin/python3 /usr/local/lib/plan9-b12/service.py traffic\n')
        self.save('native-redis',json.loads(self.shell('direct','python3 /usr/local/lib/plan9-b12/service.py native\n')))
        self.save('native-http',json.loads(self.shell('direct','curl -fsS http://127.0.0.1:18084/orders\n')))
        print('Actual bounded HTTP/Redis traffic running; native counters captured',flush=True)

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['prepare','build','up','workload']);args=parser.parse_args();getattr(Run(),args.action)()
