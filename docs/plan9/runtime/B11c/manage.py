"""Owned B11c tunnel-only Bastion and restricted member; fresh reset baseline."""
import argparse
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys

HERE=Path(__file__).resolve().parent
ROOT=HERE.parents[3]
sys.path.insert(0,str(HERE.parent))
from B01.manage import Run as OwnedRun,execute,write_json,now
from B09a.manage import Run as PlatformRun
from common.api import Api

RUN='p9-b11c-20261115-01'
BASE='argus-plan9-host:b11c-base-20261115'

class Run(OwnedRun):
    def __init__(self):
        self.id=RUN
        self.out=Path(os.environ.get('ARGUS_PLAN9_ARTIFACT_ROOT','C:/Users/23574/.codex/artifacts/argus-plan9'))/RUN
        self.private=self.out/'private';self.resources=self.out/'resource-map.json';self.envfile=self.out/'compose.env'

    def save(self,name,value):write_json(self.out/(name+'.json'),value)
    def platform(self):return PlatformRun().platform()
    def api(self):
        return Api(self,self.private/'enterprise-session.json',ROOT/'deploy/.cache/argus-trust-bundle.pem',
                   {'enterprise_username':os.environ.get('ARGUS_PLAN9_USERNAME','kakj'),'password':os.environ['ARGUS_PLAN9_PASSWORD']})

    def compose(self,*args,timeout=120):
        cli=Path('C:/Program Files/Docker/Docker/resources/cli-plugins/docker-compose.exe')
        return execute([str(cli),'--env-file',str(self.envfile),'-f',str(HERE/'compose.yaml'),'--project-name',self.id,*args],timeout=timeout)

    def container(self,key):
        assert key in ['bastion','member']
        ident=self.compose('ps','--all','--quiet',key);assert ident
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
            reset=Path('C:/Users/23574/.codex/artifacts/argus-account-reset-20261115/completion.json')
            assert json.loads(reset.read_text(encoding='utf-8'))['status']=='complete'
            assert json.loads((self.out.parent/'p9-b11b-20261114-01/cleanup-verification.json').read_text())['status']=='destroyed'
            self.save('run',{'run_id':self.id,'batch':'B11c','status':'preparing','created_at':now(),'human_acceptance':'pending',
                'authorization':'User requested the next B11c acceptance stage when available, otherwise B12; B11c remains planned',
                'business_objects_via_product_api':True,'direct_business_sql_writes':False,'synthetic_telemetry':False,
                'limits':{'hosts':2,'cpus':3,'memory_mib':2560,'fault_guard_seconds':120,'traffic_hours':8}})
            self.save('resource-map',{'run_id':self.id,'hosts':{},'containers':[],'dashboards':{},'alert_policies':{},'remote':{}})
            self.save('platform-before',self.platform())
            write_json(self.private/'host-credentials.json',{key:{'username':'root','password':secrets.token_urlsafe(32)} for key in ['bastion','member']})
        if not (self.private/'enterprise-session.json').exists():
            write_json(self.private/'enterprise-session.json',{'origin':'https://argus.dev','csrf':'','user':{},'cookies':[]})
        api=self.api();api.login()
        hosts=api.call('GET','/enterprise/hosts')['items']
        assert all(row.get('labels',{}).get('plan9_run')==self.id for row in hosts)
        self.save('baseline-hosts',hosts)
        # Remove only the predecessor's leftover, UID-matched temporary ingress.
        previous=json.loads((self.out.parent/'p9-b11b-20261114-01/resource-map.json').read_text(encoding='utf-8'))
        endpoint=previous.get('telemetry_network_endpoint')
        if endpoint:
            cli=['kubectl','--context','docker-desktop','-n',endpoint['namespace']]
            raw=execute([*cli,'get','service',endpoint['name'],'--ignore-not-found','-o','json'])
            if raw:
                current=json.loads(raw)
                assert current['metadata']['uid']==endpoint['uid'] and current['metadata']['labels']['argus.plan9/run']==previous['run_id']
                execute([*cli,'delete','service',endpoint['name'],'--wait=true'])
                self.save('predecessor-endpoint-cleanup',{'name':endpoint['name'],'uid':endpoint['uid'],'deleted':True})
        print('Fresh B11c baseline and private credentials prepared; formal data retained',flush=True)

    def build(self):
        releases=json.loads(Path('C:/Users/23574/.codex/artifacts/argus-account-reset-20261115/release-images.json').read_text())
        trust=next(row for row in releases if row['name']=='argus-remote-runtime')
        trust_ref='localhost:5001/argus/argus-remote-runtime:'+trust['target']+'@'+trust['digest']
        for name,args in [('base',['--build-arg','TRUST_IMAGE='+trust_ref,'-t',BASE,str(HERE.parent/'B01')]),
                          ('hosts',['--build-arg','PLAN9_BASE='+BASE,'-t','argus-plan9-bastion:b11c-20261115',str(HERE)])]:
            with (self.out/('build-'+name+'.log')).open('wb') as log:
                subprocess.run(['docker','build','--platform','linux/amd64',*args],cwd=ROOT,stdout=log,stderr=subprocess.STDOUT,check=True,timeout=600)
        image=json.loads(execute(['docker','image','inspect','argus-plan9-bastion:b11c-20261115']))[0]
        assert image['Architecture']=='amd64'
        self.save('image-lock',{'image_id':image['Id'],'trust_source':trust_ref,'files':{p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in HERE.iterdir() if p.is_file()}})
        networks=json.loads(execute(['docker','network','inspect',*execute(['docker','network','ls','-q']).split()]))
        used=[ipaddress.ip_network(item['Subnet']) for net in networks for item in ((net.get('IPAM') or {}).get('Config') or []) if item.get('Subnet')]
        subnet=next(ipaddress.ip_network(value) for value in ['172.28.115.0/24','172.29.115.0/24','172.30.115.0/24'] if not any(ipaddress.ip_network(value).overlaps(old) for old in used if old.version==4))
        env={'PLAN9_RUN':self.id,'PLAN9_HOST_IMAGE':image['Id'],'PLAN9_PRIVATE_SUBNET':str(subnet),
             'PLAN9_BASTION_PRIVATE':str(subnet.network_address+2),'PLAN9_MEMBER_PRIVATE':str(subnet.network_address+3)}
        self.envfile.write_text(''.join(key+'='+value+'\n' for key,value in env.items()),encoding='utf-8')
        self.save('network-plan',env);self.compose('config','--quiet')
        print('Fresh pinned B11c systemd/SSH/firewall image and bounded Compose validated',flush=True)

    def up(self):
        mapping=json.loads(self.resources.read_text());assert not mapping['containers']
        self.compose('up','-d','--no-build','--wait','--wait-timeout','120',timeout=180)
        private=json.loads((self.private/'host-credentials.json').read_text());plan=json.loads((self.out/'network-plan.json').read_text())
        for key in ['bastion','member']:
            value=self.container(key);execute(['docker','exec','-i',value['Id'],'chpasswd'],data='root:'+private[key]['password']+'\n')
            mapping['containers'].append({'service':key,'id':value['Id'],'image_id':value['Image']});self.save('resource-map',mapping)
        root=self.container('bastion');member=self.container('member')
        assert set(member['NetworkSettings']['Networks'])=={self.id+'_inside'}
        assert root['HostConfig']['NetworkMode']!='host'
        self.shell('bastion',"mkdir -p /etc/systemd/system/argus-connector.service.d\nprintf '%s\\n' '[Service]' 'Environment=ARGUS_BASTION_RELAY_ADVERTISE_ADDRESS="+plan['PLAN9_BASTION_PRIVATE']+"' 'Environment=TMPDIR=/var/lib/argus-connector/plan9-b11c-tmp' 'ExecStartPre=+/usr/bin/install -d -m 0700 -o argus-connector -g argus-connector /var/lib/argus-connector/plan9-b11c-tmp' > /etc/systemd/system/argus-connector.service.d/plan9-b11c.conf\nsystemctl daemon-reload\n")
        self.save('host-specs',{'bastion':{'name':'B11c-Tunnel-Bastion','letter':'G','address':root['NetworkSettings']['Networks']['kind']['IPAddress'],'ssh_port':22},
             'member':{'name':'B11c-Private-Member','letter':'M','address':plan['PLAN9_MEMBER_PRIVATE'],'ssh_port':22}})
        assert self.platform()==json.loads((self.out/'platform-before.json').read_text())
        print('Two real B11c hosts running; no prior host or Collector identity reused',flush=True)

    def workload(self):
        for key in ['bastion','member']:
            current=self.shell(key,'systemctl is-active plan9-b11c-traffic.service || true\n')
            if current!='active':
                self.shell(key,'systemd-run --collect --unit=plan9-b11c-traffic --property=RuntimeMaxSec=28800 --property=MemoryMax=64M --property=CPUQuota=5% /usr/bin/python3 /usr/local/lib/plan9-b11c.py traffic\n')
        self.save('workload',{'started_at':now(),'requests_every_seconds':15,'maximum_seconds':28800,'maximum_memory_mib_each':64,'maximum_cpu_percent_each':5})
        print('Real bounded HTTP traffic running on both hosts',flush=True)

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['prepare','build','up','workload']);args=parser.parse_args();getattr(Run(),args.action)()
