"""Owned B02a real web processes; no writes to Argus business databases."""
import argparse
from datetime import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import sys

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[3]
sys.path.insert(0, str(HERE.parent))
from B01.manage import execute, write_json, now

HOSTS = {
    'web-a': {'letter':'A','ssh_port':22231,'name':'B02a-Web-A'},
    'web-b': {'letter':'B','ssh_port':22232,'name':'B02a-Web-B'},
}


class Run:
    def __init__(self, run_id='p9-b02a-20261027-01'):
        if not re.fullmatch(r'p9-b02a-\d{8}-\d{2}', run_id):
            raise ValueError('invalid B02a run ID')
        self.id = run_id
        self.out = ROOT/'artifacts/plan9'/run_id
        self.envfile = self.out/'compose.env'
        self.resources = self.out/'resource-map.json'

    def event(self, kind, **fields):
        with (self.out/'timeline.jsonl').open('a', encoding='utf-8') as stream:
            stream.write(json.dumps({'time':now(),'event':kind,**fields})+'\n')

    def compose(self, *args, timeout=120):
        plugin = Path('C:/Program Files/Docker/Docker/resources/cli-plugins/docker-compose.exe')
        command = [str(plugin)] if os.name == 'nt' and plugin.is_file() else ['docker','compose']
        return execute([*command,'--env-file',str(self.envfile),'--file',str(HERE/'compose.yaml'),
                        '--project-name',self.id,*args],timeout=timeout)

    def container(self, key):
        if key not in HOSTS:
            raise ValueError('unknown owned web host')
        cid=self.compose('ps','--all','--quiet',key)
        if not cid:
            raise RuntimeError('owned container missing')
        value=json.loads(execute(['docker','inspect',cid]))[0]
        assert value['Config']['Labels'].get('io.argus.plan9.run')==self.id
        assert value['Config']['Labels'].get('io.argus.plan9.batch')=='B02a'
        return value

    def shell(self, key, script, timeout=120):
        return execute(['docker','exec','-i',self.container(key)['Id'],'/bin/bash','-se'],data=script,timeout=timeout)

    def prepare(self):
        self.out.mkdir(parents=True,exist_ok=True)
        private=self.out/'private';private.mkdir(exist_ok=True)
        if os.name=='nt':
            execute(['icacls',str(private),'/inheritance:r','/grant:r',execute(['whoami'])+':(OI)(CI)F','*S-1-5-18:(OI)(CI)F'])
        else:private.chmod(0o700)
        if not self.resources.exists():
            write_json(self.resources,{'run_id':self.id,'hosts':{},'targets':{},'dashboards':{},'alert_policies':{},'containers':[]})
        credentials=private/'host-credentials.json'
        if not credentials.exists():
            write_json(credentials,{key:{'username':'root','password':secrets.token_urlsafe(30)} for key in HOSTS})
        if not (self.out/'run.json').exists():
            write_json(self.out/'run.json',{'run_id':self.id,'batch':'B02a','status':'preparing','created_at':now(),
                'authorization':'User requested B02 and selected B02a first','budget':{'cpus':3,'memory_mib':2560,'load_max_seconds':120,'background_max_seconds':86400}})
        self.event('prepared')

    def up(self):
        previous=json.loads((ROOT/'artifacts/plan9/p9-b01-20261026-01/cleanup.json').read_text(encoding='utf-8'))
        assert previous['status']=='destroyed'
        mapping=json.loads(self.resources.read_text(encoding='utf-8'))
        if mapping['containers']:
            for key in HOSTS:
                container=self.container(key)
                assert container['State']['Running'] and container['Id']==next(c['id'] for c in mapping['containers'] if c['service']==key)
            return
        image=json.loads(execute(['docker','image','inspect','argus-plan9-web:b02a-20261027-v2']))[0]
        deadline=int(datetime.fromisoformat(json.loads((self.out/'run.json').read_text(encoding='utf-8'))['created_at']).timestamp())+86400
        self.envfile.write_text('PLAN9_RUN='+self.id+'\nPLAN9_HOST_IMAGE='+image['Id']+'\nPLAN9_TRAFFIC_DEADLINE='+str(deadline)+'\n',encoding='utf-8',newline='\n')
        # Render to JSON in memory and retain only the non-sensitive topology check.
        rendered=json.loads(self.compose('config','--format','json'))
        assert set(rendered['services'])==set(HOSTS)
        write_json(self.out/'compose-validation.json',{'services':list(HOSTS),'image_id':image['Id'],'run':self.id})
        self.compose('up','--detach','--no-build','--wait','--wait-timeout','120',timeout=180)
        for key in HOSTS:
            self.shell(key, 'systemctl is-active plan9-backend plan9-proxy-backend plan9-web-traffic\ncurl -fsS http://127.0.0.1:9000/health >/dev/null\ncurl -fsS http://127.0.0.1:9001/health >/dev/null\n')
        credentials=json.loads((self.out/'private/host-credentials.json').read_text(encoding='utf-8'))
        for key in HOSTS:
            container=self.container(key)
            execute(['docker','exec','-i',container['Id'],'chpasswd'],data='root:'+credentials[key]['password']+'\n')
            identity=self.shell(key,'cat /etc/machine-id\nssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub\n')
            mapping['containers'].append({'id':container['Id'],'service':key,'image_id':image['Id'],'identity':identity})
        assert mapping['containers'][0]['identity'].splitlines()[0]!=mapping['containers'][1]['identity'].splitlines()[0]
        write_json(self.resources,mapping)
        packages=self.shell('web-a',"dpkg-query -W -f='${Package} ${Version}\\n' nginx apache2 haproxy\n")
        lock={'image_id':image['Id'],'architecture':image['Architecture'],'packages':packages.splitlines(),
              'base_image_id':'sha256:752ef62afe9edd62067f6c1617fb79aebbbde7c5951fe633925a284de3b33ad7',
              'source_hashes':{str(p.relative_to(HERE)):hashlib.sha256(p.read_bytes()).hexdigest() for p in HERE.rglob('*') if p.is_file() and '__pycache__' not in p.parts}}
        write_json(self.out/'image-lock.json',lock)
        write_json(HERE/'images.lock.json',lock)
        self.event('infrastructure_started',hosts=2,web_targets=6,image_id=image['Id'])

    def load(self, key, integration, seconds=90):
        ports={'nginx':8081,'apache':8082,'haproxy':8083}
        assert integration in ports and 10<=seconds<=120
        self.shell(key,f'systemd-run --unit=plan9-load-{integration} --collect --property=CPUQuota=50% --property=MemoryMax=128M --property=RuntimeMaxSec={seconds+25} /usr/local/bin/plan9-web-lab hold --port {ports[integration]} --seconds {seconds}\n')
        self.event('connection_load_started',host=key,integration=integration,seconds=seconds,connections=12)

    def fault(self, key, recover=False):
        verb='start' if recover else 'stop'
        self.shell(key,f'systemctl {verb} plan9-proxy-backend.service\n')
        self.event('haproxy_backend_recovered' if recover else 'haproxy_backend_stopped',host=key)


if __name__=='__main__':
    parser=argparse.ArgumentParser()
    parser.add_argument('action',choices=['prepare','up','status','load','fault','recover'])
    parser.add_argument('--run',default='p9-b02a-20261027-01')
    parser.add_argument('--host',choices=list(HOSTS),default='web-a')
    parser.add_argument('--integration',choices=['nginx','apache','haproxy'],default='nginx')
    args=parser.parse_args();run=Run(args.run)
    if args.action in ('prepare','up'):getattr(run,args.action)()
    elif args.action=='load':run.load(args.host,args.integration)
    elif args.action in ('fault','recover'):run.fault(args.host,args.action=='recover')
    else:print(run.shell(args.host,'systemctl is-active ssh nginx apache2 haproxy plan9-backend plan9-proxy-backend\n'))
