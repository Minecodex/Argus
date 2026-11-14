"""Owned B11a systemd host, SSH install and direct telemetry infrastructure."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import sys

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[3]
sys.path.insert(0,str(HERE.parent))
from B01.manage import Run as OwnedRun,execute,now,write_json
from B09a.manage import Run as PlatformRun
from common.api import Api

RUN='p9-b11a-20261113-01'
PREDECESSOR='p9-b10c-20261113-01'
BASE='argus-plan9-host:b01-20261026'


class Run(OwnedRun):
    def __init__(self):
        self.id=RUN
        self.out=Path(os.environ.get('ARGUS_PLAN9_ARTIFACT_ROOT',ROOT/'artifacts/plan9'))/RUN
        self.private=self.out/'private'
        self.resources=self.out/'resource-map.json'
        self.envfile=self.out/'compose.env'

    def save(self,name,value): write_json(self.out/(name+'.json'),value)

    def platform(self): return PlatformRun().platform()

    def api(self):
        return Api(self,self.private/'enterprise-session.json',ROOT/'artifacts/plan9/reset-20261026/trust-bundle.pem',
                   {'enterprise_username':os.environ.get('ARGUS_PLAN9_USERNAME','kakj'),'password':os.environ['ARGUS_PLAN9_PASSWORD']})

    def compose(self,*args,timeout=120):
        plugin=Path('C:/Program Files/Docker/Docker/resources/cli-plugins/docker-compose.exe')
        command=[str(plugin)] if plugin.exists() else ['docker','compose']
        return execute([*command,'--env-file',str(self.envfile),'-f',str(HERE/'compose.yaml'),
                        '--project-name',self.id,*args],timeout=timeout)

    def container(self,key='direct'):
        assert key=='direct'
        ident=self.compose('ps','--all','--quiet',key)
        assert ident
        value=json.loads(execute(['docker','inspect',ident]))[0]
        assert value['Config']['Labels']['io.argus.plan9.run']==self.id
        assert value['Config']['Labels']['io.argus.plan9.batch']=='B11a'
        saved=json.loads(self.resources.read_text(encoding='utf-8'))
        if saved['containers']: assert value['Id']==saved['containers'][0]['id']
        return value

    def prepare(self):
        self.private.mkdir(parents=True,exist_ok=True)
        execute(['icacls',str(self.private),'/inheritance:r','/grant:r',execute(['whoami'])+':(OI)(CI)F','*S-1-5-18:(OI)(CI)F'])
        if not (self.out/'run.json').exists():
            self.save('run',{'run_id':self.id,'batch':'B11a','status':'preparing','created_at':now(),
                'authorization':'User explicitly accepted B10c and requested cleanup then B11a deployment',
                'human_acceptance':'pending','shared_kernel_scope':True,
                'limits':{'cpus':1.5,'memory_mib':1280,'background_hours':8,'fault_seconds':120}})
            self.save('platform-before',self.platform())
            self.save('resource-map',{'run_id':self.id,'hosts':{},'containers':[],'targets':{},'dashboards':{},
                                     'alert_policies':{},'sessions':{},'profiles':{},'borrowed':['formal Argus','kind network','cached base image']})
            write_json(self.private/'host-credentials.json',{'direct':{'username':'root','password':secrets.token_urlsafe(32)}})
        if not (self.private/'enterprise-session.json').exists():
            write_json(self.private/'enterprise-session.json',{'origin':'https://argus.dev','csrf':'','user':{},'cookies':[]})
        print('B11a private state and formal deployment/PVC baseline prepared',flush=True)

    def build(self):
        base=json.loads(execute(['docker','image','inspect',BASE]))[0]
        assert base['Architecture']=='amd64'
        tag='argus-plan9-direct:b11a-20261113'
        execute(['docker','build','--build-arg','PLAN9_BASE='+BASE,'-t',tag,str(HERE)],timeout=240)
        assert json.loads(execute(['docker','image','inspect',BASE]))[0]['Id']==base['Id']
        image=json.loads(execute(['docker','image','inspect',tag]))[0]
        self.save('image-lock',{'image_id':image['Id'],'base_id':base['Id'],'architecture':image['Architecture'],
                               'files':{p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in HERE.iterdir() if p.is_file()}})
        self.envfile.write_text('PLAN9_RUN='+self.id+'\nPLAN9_HOST_IMAGE='+image['Id']+'\n',encoding='utf-8')
        self.compose('config','--quiet')
        print('Pinned systemd/SSH host image built and Compose validated',flush=True)

    def up(self):
        old=self.out.parent/PREDECESSOR
        assert json.loads((old/'cleanup-verification.json').read_text())['status']=='destroyed'
        mapping=json.loads(self.resources.read_text(encoding='utf-8'))
        assert not mapping['containers'],'Inspect any existing infrastructure before restarting'
        self.compose('up','-d','--no-build','--wait','--wait-timeout','120',timeout=180)
        value=self.container();password=json.loads((self.private/'host-credentials.json').read_text())['direct']['password']
        execute(['docker','exec','-i',value['Id'],'chpasswd'],data='root:'+password+'\n')
        address=value['NetworkSettings']['Networks']['kind']['IPAddress']
        assert address
        mapping['containers']=[{'service':'direct','id':value['Id'],'image_id':value['Image'],'address':address}]
        self.save('resource-map',mapping)
        self.save('host-specs',{'direct':{'letter':'D','ssh_port':22,'name':'B11a-Linux-Direct',
            'address':address,'control_path':'direct','collector_transport':'direct'}})
        native=json.loads(self.shell('direct',"python3 -c \"import os,json,socket;print(json.dumps({'hostname':socket.gethostname(),'pid':os.getpid(),'machine_id':open('/etc/machine-id').read().strip()}))\"\n"))
        self.save('native-host-identity',native)
        assert self.platform()==json.loads((self.out/'platform-before.json').read_text())
        print('New direct SSH host running at '+address+':22',flush=True)

    def workload(self):
        self.shell('direct','systemd-run --collect --unit=plan9-b11a-traffic --property=RuntimeMaxSec=28800 --property=MemoryMax=64M --property=CPUQuota=5% /usr/bin/python3 /usr/local/lib/plan9-b11a.py traffic\n')
        print('Eight-hour bounded low-frequency real HTTP requests started',flush=True)


if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['prepare','build','up','workload']);args=p.parse_args()
    getattr(Run(),args.action)()
