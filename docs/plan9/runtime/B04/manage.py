"""Owned cache services and normal protocol clients for Plan9 B04."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import sys
HERE=Path(__file__).resolve().parent
ROOT=HERE.parents[3]
sys.path.insert(0,str(HERE.parent))
from B01.manage import execute,write_json,now
HOSTS={'collector':{'letter':'C','ssh_port':22261,'name':'B04-Cache-Collector'}}
ENGINES=['redis','memcached']
SERVICES=['collector',*ENGINES]

class Run:
    def __init__(self,run_id='p9-b04-20261027-01'):
        if not re.fullmatch(r'p9-b04-\d{8}-\d{2}',run_id):raise ValueError('invalid B04 run')
        self.id=run_id;self.out=ROOT/'artifacts/plan9'/run_id
        self.resources=self.out/'resource-map.json';self.envfile=self.out/'compose.env'

    def event(self,kind,**fields):
        with (self.out/'timeline.jsonl').open('a',encoding='utf-8') as stream:
            stream.write(json.dumps({'time':now(),'event':kind,**fields})+'\n')

    def compose(self,*args,timeout=120):
        plugin=Path('C:/Program Files/Docker/Docker/resources/cli-plugins/docker-compose.exe')
        command=[str(plugin)] if os.name=='nt' and plugin.is_file() else ['docker','compose']
        return execute([*command,'--env-file',str(self.envfile),'--file',str(HERE/'compose.yaml'),'--project-name',self.id,*args],timeout=timeout)

    def container(self,key):
        assert key in SERVICES
        cid=self.compose('ps','--all','--quiet',key)
        if not cid:raise RuntimeError('Owned container missing: '+key)
        c=json.loads(execute(['docker','inspect',cid]))[0]
        assert c['Config']['Labels'].get('io.argus.plan9.run')==self.id
        assert c['Config']['Labels'].get('io.argus.plan9.batch')=='B04'
        return c

    def shell(self,key,script,timeout=120):
        return execute(['docker','exec','-i',self.container(key)['Id'],'/bin/sh','-se'],data=script,timeout=timeout)

    def prepare(self):
        self.out.mkdir(parents=True,exist_ok=True);private=self.out/'private';private.mkdir(exist_ok=True)
        if os.name=='nt':execute(['icacls',str(private),'/inheritance:r','/grant:r',execute(['whoami'])+':(OI)(CI)F','*S-1-5-18:(OI)(CI)F'])
        else:private.chmod(0o700)
        if not (private/'host-credentials.json').exists():write_json(private/'host-credentials.json',{'collector':{'username':'root','password':secrets.token_urlsafe(30)}})
        if not (private/'redis-credentials.json').exists():write_json(private/'redis-credentials.json',{role:secrets.token_urlsafe(30) for role in ['monitor','app']})
        credentials=json.loads((private/'redis-credentials.json').read_text(encoding='utf-8'))
        acl='user default off\n'
        for role,commands in [('monitor','+info +config|get +ping'),('app','~p9:* +set +get +del +exists +expire +ttl +scan +ping')]:
            acl+='user '+role+' on #'+hashlib.sha256(credentials[role].encode()).hexdigest()+' '+commands+'\n'
        (private/'users.acl').write_text(acl,encoding='utf-8',newline='\n')
        (private/'redis.conf').write_text('bind 0.0.0.0\nprotected-mode yes\nport 6379\naclfile /run/secrets/users.acl\nmaxmemory 8mb\nmaxmemory-policy allkeys-lru\nsave ""\nappendonly no\n',encoding='utf-8',newline='\n')
        if not self.resources.exists():write_json(self.resources,{'run_id':self.id,'hosts':{},'targets':{},'dashboards':{},'alert_policies':{},'containers':[]})
        if not (self.out/'run.json').exists():write_json(self.out/'run.json',{'run_id':self.id,'batch':'B04','created_at':now(),'status':'preparing','authorization':'清理 B03 后部署 B04，把 Redis 和 Memcached 的真实场景、监控、仪表盘和告警都接好，我再去平台看。','budget':{'cpus':2.5,'memory_mib':1568,'cache_capacity_mib_each':8,'pressure_keys_max':1024,'pressure_value_bytes':32768,'pressure_ttl_seconds':120,'background_hours_max':8}})

    def up(self):
        assert json.loads((ROOT/'artifacts/plan9/p9-b03-20261027-01/cleanup.json').read_text(encoding='utf-8'))['status']=='destroyed'
        mapping=json.loads(self.resources.read_text(encoding='utf-8'))
        if mapping['containers']:
            for item in mapping['containers']:assert self.container(item['service'])['Id']==item['id']
            return
        images={key:json.loads(execute(['docker','image','inspect',tag]))[0] for key,tag in {'HOST':'argus-plan9-cache:b04-20261027','REDIS':'redis:8.10.0-alpine','MEMCACHED':'memcached:1.6-alpine'}.items()}
        env='PLAN9_RUN='+self.id+'\nPLAN9_PRIVATE='+(self.out/'private').as_posix()+'\n'
        env+=''.join('PLAN9_'+key+'_IMAGE='+value['Id']+'\n' for key,value in images.items())
        self.envfile.write_text(env,encoding='utf-8',newline='\n')
        assert set(json.loads(self.compose('config','--format','json'))['services'])==set(SERVICES)
        self.compose('up','--detach','--no-build','--wait','--wait-timeout','120',timeout=180)
        password=json.loads((self.out/'private/host-credentials.json').read_text(encoding='utf-8'))['collector']['password']
        execute(['docker','exec','-i',self.container('collector')['Id'],'chpasswd'],data='root:'+password+'\n')
        for key in SERVICES:
            c=self.container(key);mapping['containers'].append({'service':key,'id':c['Id'],'image_id':c['Image']})
        write_json(self.resources,mapping)
        lock={'images':{key:{'id':v['Id'],'digests':v.get('RepoDigests',[]),'architecture':v['Architecture']} for key,v in images.items()}}
        write_json(self.out/'image-lock.json',lock);write_json(HERE/'images.lock.json',lock)
        self.event('infrastructure_started',services=SERVICES)

    def client(self,action,key='redis'):
        assert action in ['verify','native','pressure','recover'] and key in ENGINES
        return json.loads(self.shell('collector',f'python3 /usr/local/lib/plan9-cache-client.py {action} {key}\n',timeout=60))

    def verify(self):
        for key in ENGINES:
            write_json(self.out/(key+'-protocol-checks.json'),self.client('verify',key));print(key+': actual SET/GET, miss and TTL verified',flush=True)

    def workload(self):
        for key in ENGINES:
            self.shell('collector',f'systemctl reset-failed plan9-cache-{key}.service 2>/dev/null || true\nif ! systemctl is-active --quiet plan9-cache-{key}.service; then systemd-run --unit=plan9-cache-{key} --property=RuntimeMaxSec=28800 --property=MemoryMax=64M --property=CPUQuota=10% python3 /usr/local/lib/plan9-cache-client.py background {key}; fi\n')
        self.event('bounded_background_started',maximum_seconds=28800)

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['prepare','up','verify','workload']);a=p.parse_args();getattr(Run(),a.action)()
