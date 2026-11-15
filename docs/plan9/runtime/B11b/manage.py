"""Owned Bastion + actually isolated member infrastructure; no business SQL."""
import argparse
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import secrets
import sys

HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[3]
sys.path.insert(0,str(HERE.parent))
from B01.manage import Run as OwnedRun,execute,write_json,now
from B09a.manage import Run as PlatformRun
from common.api import Api

RUN='p9-b11b-20261114-01';PREDECESSOR='p9-b11a-20261113-01';BASE='argus-plan9-host:b01-20261026'


class Run(OwnedRun):
    def __init__(self):
        self.id=RUN;self.out=Path(os.environ.get('ARGUS_PLAN9_ARTIFACT_ROOT',ROOT/'artifacts/plan9'))/RUN
        self.private=self.out/'private';self.resources=self.out/'resource-map.json';self.envfile=self.out/'compose.env'

    def save(self,name,value):write_json(self.out/(name+'.json'),value)
    def platform(self):return PlatformRun().platform()

    def api(self):
        return Api(self,self.private/'enterprise-session.json',ROOT/'artifacts/plan9/reset-20261026/trust-bundle.pem',
            {'enterprise_username':os.environ.get('ARGUS_PLAN9_USERNAME','kakj'),'password':os.environ['ARGUS_PLAN9_PASSWORD']})

    def compose(self,*args,timeout=120):
        cli=Path('C:/Program Files/Docker/Docker/resources/cli-plugins/docker-compose.exe')
        return execute([str(cli),'--env-file',str(self.envfile),'-f',str(HERE/'compose.yaml'),
            '--project-name',self.id,*args],timeout=timeout)

    def container(self,key):
        assert key in ['bastion','member'];ident=self.compose('ps','--all','--quiet',key);assert ident
        value=json.loads(execute(['docker','inspect',ident]))[0]
        labels=value['Config']['Labels'];assert labels['io.argus.plan9.run']==self.id and labels['io.argus.plan9.peer']==key
        saved=json.loads(self.resources.read_text(encoding='utf-8'))
        previous=next((row for row in saved['containers'] if row['service']==key),None)
        if previous:assert value['Id']==previous['id']
        return value

    def prepare(self):
        self.private.mkdir(parents=True,exist_ok=True)
        execute(['icacls',str(self.private),'/inheritance:r','/grant:r',execute(['whoami'])+':(OI)(CI)F','*S-1-5-18:(OI)(CI)F'])
        if not (self.out/'run.json').exists():
            self.save('run',{'run_id':self.id,'batch':'B11b','status':'preparing','created_at':now(),
                'authorization':'User requested the next Plan9 acceptance stage; B11a retired after advancement',
                'human_acceptance':'pending','direct_database_writes':False,'synthetic_telemetry':False,
                'limits':{'hosts':2,'total_cpus':3,'total_memory_mib':2560,'background_hours':8,'fault_seconds':120}})
            self.save('resource-map',{'run_id':self.id,'hosts':{},'containers':[],'targets':{},'dashboards':{},'alert_policies':{},'remote':{}})
            self.save('platform-before',self.platform())
            write_json(self.private/'host-credentials.json',{key:{'username':'root','password':secrets.token_urlsafe(32)} for key in ['bastion','member']})
        if not (self.private/'enterprise-session.json').exists():
            write_json(self.private/'enterprise-session.json',{'origin':'https://argus.dev','csrf':'','user':{},'cookies':[]})
        print('B11b private state and six formal PVC baseline prepared',flush=True)

    def build(self):
        base=json.loads(execute(['docker','image','inspect',BASE]))[0];assert base['Architecture']=='amd64'
        tag='argus-plan9-bastion:b11b-20261114'
        execute(['docker','build','--build-arg','PLAN9_BASE='+BASE,'-t',tag,str(HERE)],timeout=240)
        image=json.loads(execute(['docker','image','inspect',tag]))[0]
        self.save('image-lock',{'image_id':image['Id'],'base_id':base['Id'],'files':{p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in HERE.iterdir() if p.is_file()}})
        networks=json.loads(execute(['docker','network','inspect',*execute(['docker','network','ls','-q']).split()]))
        used=[ipaddress.ip_network(cfg['Subnet']) for network in networks for cfg in ((network.get('IPAM') or {}).get('Config') or []) if cfg.get('Subnet')]
        subnet=next(ipaddress.ip_network(value) for value in ['172.28.114.0/24','172.29.114.0/24','172.30.114.0/24'] if not any(ipaddress.ip_network(value).overlaps(old) for old in used if old.version==4))
        config={'PLAN9_RUN':self.id,'PLAN9_HOST_IMAGE':image['Id'],'PLAN9_PRIVATE_SUBNET':str(subnet),
                'PLAN9_BASTION_PRIVATE':str(subnet.network_address+2),'PLAN9_MEMBER_PRIVATE':str(subnet.network_address+3)}
        self.envfile.write_text(''.join(key+'='+value+'\n' for key,value in config.items()),encoding='utf-8')
        self.save('network-plan',config);self.compose('config','--quiet')
        print('Pinned image and non-overlapping internal network Compose validated',flush=True)

    def up(self):
        assert json.loads((self.out.parent/PREDECESSOR/'cleanup-verification.json').read_text())['status']=='destroyed'
        mapping=json.loads(self.resources.read_text());assert not mapping['containers']
        self.compose('up','-d','--no-build','--wait','--wait-timeout','120',timeout=180)
        credentials=json.loads((self.private/'host-credentials.json').read_text());plan=json.loads((self.out/'network-plan.json').read_text())
        for key in ['bastion','member']:
            value=self.container(key);execute(['docker','exec','-i',value['Id'],'chpasswd'],data='root:'+credentials[key]['password']+'\n')
            mapping['containers'].append({'service':key,'id':value['Id'],'image_id':value['Image'],'networks':{n:info['IPAddress'] for n,info in value['NetworkSettings']['Networks'].items()}})
            self.save('resource-map',mapping)
        inside=self.id+'_inside';bastion=self.container('bastion');member=self.container('member')
        assert set(member['NetworkSettings']['Networks'])=={inside}
        self.shell('bastion',"mkdir -p /etc/systemd/system/argus-connector.service.d\nprintf '%s\\n' '[Service]' 'Environment=ARGUS_BASTION_RELAY_ADVERTISE_ADDRESS="+plan['PLAN9_BASTION_PRIVATE']+"' 'Environment=TMPDIR=/var/lib/argus-connector/plan9-b11b-tmp' 'ExecStartPre=+/usr/bin/install -d -m 0700 -o argus-connector -g argus-connector /var/lib/argus-connector/plan9-b11b-tmp' > /etc/systemd/system/argus-connector.service.d/plan9-b11b.conf\nsystemctl daemon-reload\n")
        self.save('host-specs',{'bastion':{'name':'B11b-Bastion-Gateway','letter':'G','address':bastion['NetworkSettings']['Networks']['kind']['IPAddress'],'ssh_port':22},
            'member':{'name':'B11b-Private-Linux','letter':'M','address':plan['PLAN9_MEMBER_PRIVATE'],'ssh_port':22}})
        assert self.platform()==json.loads((self.out/'platform-before.json').read_text())
        print('Bastion and private-only member started; actual network attachment checked',flush=True)

    def workload(self):
        for key in ['bastion','member']:
            self.shell(key,'systemd-run --collect --unit=plan9-b11b-traffic --property=RuntimeMaxSec=28800 --property=MemoryMax=64M --property=CPUQuota=5% /usr/bin/python3 /usr/local/lib/plan9-b11b.py traffic\n')


if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['prepare','build','up','workload']);args=parser.parse_args();getattr(Run(),args.action)()
