"""Owned B03 databases. No connections or SQL to the Argus storage services."""
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

HOSTS={'collector':{'letter':'C','ssh_port':22251,'name':'B03-DB-Collector'}}
DATABASES=['mysql-a','mysql-b','postgres-a','postgres-b']
SERVICES=['collector',*DATABASES]


class Run:
    def __init__(self,run_id='p9-b03-20261027-01'):
        if not re.fullmatch(r'p9-b03-\d{8}-\d{2}',run_id):raise ValueError('invalid B03 run')
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
        assert c['Config']['Labels'].get('io.argus.plan9.batch')=='B03'
        return c

    def shell(self,key,script,timeout=120):
        return execute(['docker','exec','-i',self.container(key)['Id'],'/bin/sh','-se'],data=script,timeout=timeout)

    def prepare(self):
        self.out.mkdir(parents=True,exist_ok=True);private=self.out/'private';private.mkdir(exist_ok=True)
        if os.name=='nt':execute(['icacls',str(private),'/inheritance:r','/grant:r',execute(['whoami'])+':(OI)(CI)F','*S-1-5-18:(OI)(CI)F'])
        else:private.chmod(0o700)
        if not (private/'host-credentials.json').exists():write_json(private/'host-credentials.json',{'collector':{'username':'root','password':secrets.token_urlsafe(30)}})
        if not (private/'database-credentials.json').exists():
            credentials={key:{role:secrets.token_urlsafe(30) for role in ['root','monitor','app']} for key in DATABASES}
            write_json(private/'database-credentials.json',credentials)
            for key in DATABASES:(private/(key+'-root')).write_text(credentials[key]['root'],encoding='utf-8')
        if not self.resources.exists():write_json(self.resources,{'run_id':self.id,'hosts':{},'targets':{},'dashboards':{},'alert_policies':{},'containers':[]})
        if not (self.out/'run.json').exists():write_json(self.out/'run.json',{'run_id':self.id,'batch':'B03','created_at':now(),'status':'preparing','authorization':'B02b 没啥问题，清理后切换部署 B03 吧','budget':{'cpus':5,'memory_mib':3584,'rows_per_database_max':200,'background_hours_max':8,'connection_load_max_seconds':180,'lock_max_seconds':30}})

    def up(self):
        assert json.loads((ROOT/'artifacts/plan9/p9-b02b-20261027-01/cleanup.json').read_text(encoding='utf-8'))['status']=='destroyed'
        mapping=json.loads(self.resources.read_text(encoding='utf-8'))
        if mapping['containers']:
            for item in mapping['containers']:assert self.container(item['service'])['Id']==item['id']
            return
        images={key:json.loads(execute(['docker','image','inspect',tag]))[0] for key,tag in {
            'HOST':'argus-plan9-databases:b03-20261027','MYSQL':'mysql:8.4','POSTGRES':'postgres:18.6-alpine'}.items()}
        env='PLAN9_RUN='+self.id+'\nPLAN9_PRIVATE='+(self.out/'private').as_posix()+'\n'
        env+=''.join('PLAN9_'+key+'_IMAGE='+value['Id']+'\n' for key,value in images.items())
        self.envfile.write_text(env,encoding='utf-8',newline='\n')
        rendered=json.loads(self.compose('config','--format','json'));assert set(rendered['services'])==set(SERVICES)
        self.compose('up','--detach','--no-build','--wait','--wait-timeout','180',timeout=240)
        password=json.loads((self.out/'private/host-credentials.json').read_text(encoding='utf-8'))['collector']['password']
        execute(['docker','exec','-i',self.container('collector')['Id'],'chpasswd'],data='root:'+password+'\n')
        for key in SERVICES:
            c=self.container(key);mapping['containers'].append({'service':key,'id':c['Id'],'image_id':c['Image']})
        write_json(self.resources,mapping)
        lock={'images':{key:{'id':v['Id'],'digests':v.get('RepoDigests',[]),'architecture':v['Architecture']} for key,v in images.items()},'source_hashes':{p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in HERE.iterdir() if p.is_file() and p.name!='images.lock.json'}}
        write_json(self.out/'image-lock.json',lock);write_json(HERE/'images.lock.json',lock)
        self.event('infrastructure_started',services=SERVICES)

    def client(self,action,key='mysql-a',seconds=180):
        assert action in ['init','verify','connections','lock','background','recover'] and key in DATABASES and 1<=seconds<=28800
        return self.shell('collector',f'python3 /usr/local/lib/plan9-db-client.py {action} {key} --seconds {seconds}\n',timeout=seconds+30)

    def initialize(self):
        for key in DATABASES:
            result=json.loads(self.client('init',key));write_json(self.out/(key+'-native-baseline.json'),result);print(key+': actual protocol initialized',flush=True)

    def workload(self):
        for key in DATABASES:
            self.shell('collector',f'systemctl reset-failed plan9-db-{key}.service 2>/dev/null || true\nif ! systemctl is-active --quiet plan9-db-{key}.service; then systemd-run --unit=plan9-db-{key} --property=RuntimeMaxSec=28800 --property=MemoryMax=96M --property=CPUQuota=10% python3 /usr/local/lib/plan9-db-client.py background {key} --seconds 28800; fi\n')
        self.event('bounded_background_started',maximum_seconds=28800,maximum_rows=200)


if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['prepare','up','initialize','workload']);args=p.parse_args();getattr(Run(),args.action)()
