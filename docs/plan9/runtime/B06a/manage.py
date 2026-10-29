"""Owned MongoDB instances, bounded wire-protocol workloads and recovery."""
import argparse
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
HOSTS={'collector':{'letter':'C','ssh_port':22301,'name':'B06a-MongoDB-Collector'}}
SERVICES=['collector','mongodb-a','mongodb-b']

class Run:
    def __init__(self,run_id='p9-b06a-20261029-01'):
        if not re.fullmatch(r'p9-b06a-\d{8}-\d{2}',run_id):raise ValueError('invalid B06a run')
        self.id=run_id;self.out=ROOT/'artifacts/plan9'/run_id;self.resources=self.out/'resource-map.json';self.envfile=self.out/'compose.env'
    def event(self,kind,**fields):
        with (self.out/'timeline.jsonl').open('a',encoding='utf-8') as stream:stream.write(json.dumps({'time':now(),'event':kind,**fields})+'\n')
    def compose(self,*args,timeout=120):
        plugin=Path('C:/Program Files/Docker/Docker/resources/cli-plugins/docker-compose.exe');command=[str(plugin)] if os.name=='nt' and plugin.is_file() else ['docker','compose']
        return execute([*command,'--env-file',str(self.envfile),'--file',str(HERE/'compose.yaml'),'--project-name',self.id,*args],timeout=timeout)
    def container(self,key):
        assert key in SERVICES
        cid=self.compose('ps','--all','--quiet',key)
        if not cid:raise RuntimeError('Owned container missing: '+key)
        c=json.loads(execute(['docker','inspect',cid]))[0];assert c['Config']['Labels'].get('io.argus.plan9.run')==self.id and c['Config']['Labels'].get('io.argus.plan9.batch')=='B06a';return c
    def shell(self,key,script,timeout=120):return execute(['docker','exec','-i',self.container(key)['Id'],'/bin/sh','-se'],data=script,timeout=timeout)
    def prepare(self):
        self.out.mkdir(parents=True,exist_ok=True);private=self.out/'private';private.mkdir(exist_ok=True)
        if os.name=='nt':execute(['icacls',str(private),'/inheritance:r','/grant:r',execute(['whoami'])+':(OI)(CI)F','*S-1-5-18:(OI)(CI)F'])
        else:private.chmod(0o700)
        if not (private/'host-credentials.json').exists():write_json(private/'host-credentials.json',{'collector':{'username':'root','password':secrets.token_urlsafe(30)}})
        if not (private/'mongodb-credentials.json').exists():write_json(private/'mongodb-credentials.json',{side:{role:secrets.token_urlsafe(30) for role in ['bootstrap','monitor','app']} for side in ['a','b']})
        creds=json.loads((private/'mongodb-credentials.json').read_text(encoding='utf-8'))
        for side in ['a','b']:(private/(side+'-root')).write_text(creds[side]['bootstrap'],encoding='utf-8')
        if not self.resources.exists():write_json(self.resources,{'run_id':self.id,'hosts':{},'targets':{},'dashboards':{},'alert_policies':{},'containers':[]})
        if not (self.out/'run.json').exists():write_json(self.out/'run.json',{'run_id':self.id,'batch':'B06a','created_at':now(),'status':'preparing','authorization':'下一阶段，清理 B05c 后部署 B06a，把 MongoDB 的真实场景、监控、仪表盘和告警都接好，我再去平台看。','budget':{'cpus':3.5,'memory_mib':2816,'standalone_instances':2,'documents_a':200,'documents_b':80,'payload_bytes_max':1024,'additional_clients_max':12,'update_operations_max':2000,'pressure_seconds_max':180,'background_hours_max':8}})
    def up(self):
        assert json.loads((ROOT/'artifacts/plan9/p9-b05c-20261029-01/cleanup.json').read_text(encoding='utf-8'))['status']=='destroyed';mapping=json.loads(self.resources.read_text(encoding='utf-8'))
        if mapping['containers']:
            for x in mapping['containers']:assert self.container(x['service'])['Id']==x['id']
            return
        images={key:json.loads(execute(['docker','image','inspect',tag]))[0] for key,tag in {'HOST':'argus-plan9-mongodb:b06a-20261029','MONGODB':'mongo:7.0.16'}.items()}
        self.envfile.write_text('PLAN9_RUN='+self.id+'\nPLAN9_PRIVATE='+(self.out/'private').as_posix()+'\n'+''.join('PLAN9_'+key+'_IMAGE='+v['Id']+'\n' for key,v in images.items()),encoding='utf-8',newline='\n')
        assert set(json.loads(self.compose('config','--format','json'))['services'])==set(SERVICES);self.compose('up','--detach','--no-build','--wait','--wait-timeout','180',timeout=240)
        password=json.loads((self.out/'private/host-credentials.json').read_text(encoding='utf-8'))['collector']['password'];execute(['docker','exec','-i',self.container('collector')['Id'],'chpasswd'],data='root:'+password+'\n')
        self.shell('collector','install -d -m 700 /var/lib/plan9/private\n')
        execute(['docker','exec','-i',self.container('collector')['Id'],'python3','-c',"import sys,pathlib;p=pathlib.Path('/var/lib/plan9/private/mongodb.json');p.write_bytes(sys.stdin.buffer.read());p.chmod(0o600)"],data=(self.out/'private/mongodb-credentials.json').read_text(encoding='utf-8'))
        for key in SERVICES:
            c=self.container(key);mapping['containers'].append({'service':key,'id':c['Id'],'image_id':c['Image']})
        write_json(self.resources,mapping);lock={'images':{key:{'id':v['Id'],'digests':v.get('RepoDigests',[]),'architecture':v['Architecture']} for key,v in images.items()}};write_json(self.out/'image-lock.json',lock);write_json(HERE/'images.lock.json',lock);self.event('infrastructure_started')
    def client(self,action,side='a'):
        assert action in ['initialize','native','verify'] and side in ['a','b'];return json.loads(self.shell('collector',f'python3 /usr/local/lib/plan9-mongo-client.py {action} {side}\n',timeout=90))
    def initialize(self):
        for side in ['a','b']:write_json(self.out/(side+'-protocol-preflight.json'),self.client('initialize',side));print(side+': real CRUD and monitoring privileges verified',flush=True)
    def workload(self):self.shell('collector','systemctl start plan9-mongo-workload@a plan9-mongo-workload@b\n');self.event('bounded_workload_started',maximum_seconds=28800)
    def fault(self,kind,recover=False):
        assert kind in ['connections','operations'];marker='/var/lib/plan9/stop-mongo-'+kind
        if recover:self.shell('collector','touch '+marker+'\n')
        else:self.shell('collector','rm -f '+marker+'\nif ! systemctl is-active --quiet plan9-mongo-pressure-'+kind+'; then systemd-run --collect --unit=plan9-mongo-pressure-'+kind+' --property=RuntimeMaxSec=190 --property=MemoryMax=160M python3 /usr/local/lib/plan9-mongo-client.py pressure a --kind '+kind+'; fi\n')
        self.event('recovery_requested' if recover else 'fault',scenario=kind,automatic_recovery_seconds=180)

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['prepare','up','initialize','workload']);a=p.parse_args();getattr(Run(),a.action)()
