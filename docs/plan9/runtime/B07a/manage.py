"""Owned HTTP services, SDK processes and Redis for Plan9 B07a."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import sys
import time
import subprocess
import shutil
HERE=Path(__file__).resolve().parent
ROOT=HERE.parents[3]
sys.path.insert(0,str(HERE.parent))
from B01.manage import execute,write_json,now
SERVICES=['collector','redis']
RUNS = {
    'B07a': ('p9-b07a-20261104-01', 'p9-b06d-20261029-01', 'http', 22341),
    'B07b': ('p9-b07b-20261104-01', 'p9-b07a-20261104-01', 'grpc', 22342),
    'B07c': ('p9-b07c-20261104-01', 'p9-b07b-20261104-01', 'jaeger', 22343),
}

class Run:
    def __init__(self, run_id=None, *, batch='B07a'):
        default_id, self.predecessor, self.transport, self.ssh_port = RUNS[batch]
        self.batch = batch
        self.protocol = {'http': 'HTTP', 'grpc': 'gRPC', 'jaeger': 'Jaeger gRPC'}[self.transport]
        self.listener_transport = 'http' if self.transport == 'http' else 'grpc'
        self.trace_source = 'jaeger' if self.transport == 'jaeger' else 'otlp'
        self.id = run_id or default_id
        if not re.fullmatch(r'p9-' + batch.lower() + r'-\d{8}-\d{2}', self.id):
            raise ValueError('invalid B07 run')
        self.hosts = {'collector': {'letter': 'C', 'ssh_port': self.ssh_port,
                                    'name': f'{batch}-{self.protocol}-Collector'}}
        self.image_tag = f'argus-plan9-http:{batch.lower()}-{self.id.split("-")[2]}'
        self.out=ROOT/'artifacts/plan9'/self.id
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
        assert c['Config']['Labels'].get('io.argus.plan9.batch')==self.batch
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
        for role,commands in [('monitor','+info +config|get +ping'),('app','~p9:* +set +get +blpop +ping +hello +select +client|setinfo +client|setname')]:
            acl+='user '+role+' on #'+hashlib.sha256(credentials[role].encode()).hexdigest()+' '+commands+'\n'
        (private/'users.acl').write_text(acl,encoding='utf-8',newline='\n')
        (private/'redis.conf').write_text('bind 0.0.0.0\nprotected-mode yes\nport 6379\naclfile /run/secrets/users.acl\nmaxmemory 8mb\nmaxmemory-policy allkeys-lru\nsave ""\nappendonly no\n',encoding='utf-8',newline='\n')
        if not self.resources.exists():write_json(self.resources,{'run_id':self.id,'hosts':{},'targets':{},'dashboards':{},'applications':{},'alert_policies':{},'containers':[]})
        if not (self.out/'run.json').exists():write_json(self.out/'run.json',{'run_id':self.id,'batch':self.batch,'sdk_transport':self.transport,'created_at':now(),'status':'preparing','authorization':'user_requested_batch_transition','budget':{'cpus':2,'memory_mib':1728,'redis_capacity_mib':8,'redis_keys_max':64,'fault_seconds_max':180,'background_requests_max':60000,'background_hours_max':8}})

    def build(self):
        context=self.out/'private/build-context';context.mkdir(parents=True,exist_ok=True)
        env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
        with (self.out/'private/build-sdk.log').open('wb') as log:
            subprocess.run(['go','build','-trimpath','-o',str(context/'app-binary'),'./docs/plan9/runtime/B07a/app'],cwd=ROOT,env=env,stdout=log,stderr=subprocess.STDOUT,check=True,timeout=300)
        shutil.copyfile(HERE/'load.py',context/'load.py');shutil.copytree(HERE/'systemd',context/'systemd',dirs_exist_ok=True)
        with (self.out/'private/build-host.log').open('wb') as log:
            subprocess.run(['docker','build','-t',self.image_tag,'-f',str(HERE/'Dockerfile'),str(context)],cwd=ROOT,stdout=log,stderr=subprocess.STDOUT,check=True,timeout=300)
        write_json(self.out/'build-inputs.json',{'head':execute(['git','rev-parse','HEAD']),'files':{p.relative_to(HERE).as_posix():hashlib.sha256(p.read_bytes()).hexdigest() for p in HERE.rglob('*') if p.is_file() and '__pycache__' not in p.parts},'binary_sha256':hashlib.sha256((context/'app-binary').read_bytes()).hexdigest()})

    def up(self):
        assert json.loads((self.out.parent/self.predecessor/'cleanup.json').read_text(encoding='utf-8'))['status']=='destroyed'
        mapping=json.loads(self.resources.read_text(encoding='utf-8'))
        if mapping['containers']:
            for item in mapping['containers']:assert self.container(item['service'])['Id']==item['id']
            return
        images={key:json.loads(execute(['docker','image','inspect',tag]))[0] for key,tag in {'HOST':self.image_tag,'REDIS':'redis:8.10.0-alpine'}.items()}
        env=f'PLAN9_RUN={self.id}\nPLAN9_BATCH={self.batch}\nPLAN9_SSH_PORT={self.ssh_port}\nPLAN9_PRIVATE={(self.out/"private").as_posix()}\n'
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
        write_json(self.out/'image-lock.json',lock)
        self.event('infrastructure_started',services=SERVICES)

    def request(self,label='manual'):
        if not re.fullmatch(r'[a-z0-9-]{1,24}', label):
            raise ValueError('invalid request label')
        return json.loads(self.shell('collector','python3 /usr/local/lib/plan9-load.py request --label '+label+'\n',timeout=15))
    def workload(self):
        self.shell('collector','systemctl start plan9-app@orders-worker plan9-app@orders-gateway\nsystemctl start plan9-load\nsystemctl is-active --quiet plan9-app@orders-worker && systemctl is-active --quiet plan9-app@orders-gateway && systemctl is-active --quiet plan9-load\n')
        self.event('bounded_http_workload_started',maximum_seconds=28800)
    def fault(self,kind,recover=False):
        assert kind in ['errors','delay','dependency'];body=json.dumps({'mode':'normal' if recover else kind,'until':0 if recover else int(time.time())+180})
        execute(['docker','exec','-i',self.container('collector')['Id'],'python3','-c',"import sys,pathlib;p=pathlib.Path('/var/lib/plan9/scenario.json');t=p.with_suffix('.tmp');t.write_bytes(sys.stdin.buffer.read());t.replace(p)"],data=body)
        self.event('recovery_requested' if recover else 'fault',scenario=kind,automatic_recovery_seconds=180)

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['prepare','build','up','workload']);p.add_argument('--batch',choices=RUNS,default='B07a');a=p.parse_args();getattr(Run(batch=a.batch),a.action)()
