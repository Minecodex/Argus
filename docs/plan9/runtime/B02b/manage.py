"""Owned prober and real protocol endpoints for Plan9 B02b."""
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
from pki import generate

HOSTS={'probe':{'letter':'P','ssh_port':22241,'name':'B02b-Probe'}}
SERVICES=['probe','target-a','target-b']


class Run:
    def __init__(self,run_id='p9-b02b-20261027-01'):
        if not re.fullmatch(r'p9-b02b-\d{8}-\d{2}',run_id):raise ValueError('invalid B02b run')
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
        result=json.loads(execute(['docker','inspect',cid]))[0]
        assert result['Config']['Labels'].get('io.argus.plan9.run')==self.id
        assert result['Config']['Labels'].get('io.argus.plan9.batch')=='B02b'
        return result

    def shell(self,key,script,timeout=120):
        return execute(['docker','exec','-i',self.container(key)['Id'],'/bin/bash','-se'],data=script,timeout=timeout)

    def prepare(self):
        self.out.mkdir(parents=True,exist_ok=True);private=self.out/'private';private.mkdir(exist_ok=True)
        if os.name=='nt':execute(['icacls',str(private),'/inheritance:r','/grant:r',execute(['whoami'])+':(OI)(CI)F','*S-1-5-18:(OI)(CI)F'])
        else:private.chmod(0o700)
        generate(private/'pki')
        if not (private/'host-credentials.json').exists():write_json(private/'host-credentials.json',{'probe':{'username':'root','password':secrets.token_urlsafe(30)}})
        if not self.resources.exists():write_json(self.resources,{'run_id':self.id,'hosts':{},'targets':{},'dashboards':{},'alert_policies':{},'containers':[]})
        if not (self.out/'run.json').exists():write_json(self.out/'run.json',{'run_id':self.id,'batch':'B02b','created_at':now(),'status':'preparing','authorization':'没啥问题切换到 B02b 把','budget':{'cpus':2.5,'memory_mib':1792,'fault_max_seconds':180}})
        write_json(self.out/'certificate-manifest.json',json.loads((private/'pki/manifest.json').read_text(encoding='utf-8')))

    def up(self):
        assert json.loads((ROOT/'artifacts/plan9/p9-b02a-20261027-01/cleanup.json').read_text(encoding='utf-8'))['status']=='destroyed'
        mapping=json.loads(self.resources.read_text(encoding='utf-8'))
        if mapping['containers']:
            for item in mapping['containers']:assert self.container(item['service'])['Id']==item['id']
            return
        image=json.loads(execute(['docker','image','inspect','argus-plan9-probes:b02b-20261027']))[0]
        self.envfile.write_text('PLAN9_RUN='+self.id+'\nPLAN9_IMAGE='+image['Id']+'\nPLAN9_PKI_DIR='+(self.out/'private/pki').as_posix()+'\n',encoding='utf-8',newline='\n')
        rendered=json.loads(self.compose('config','--format','json'));assert set(rendered['services'])==set(SERVICES)
        self.compose('up','--detach','--no-build','--wait','--wait-timeout','120',timeout=180)
        password=json.loads((self.out/'private/host-credentials.json').read_text(encoding='utf-8'))['probe']['password']
        execute(['docker','exec','-i',self.container('probe')['Id'],'chpasswd'],data='root:'+password+'\n')
        for key in SERVICES:
            c=self.container(key);mapping['containers'].append({'service':key,'id':c['Id'],'image_id':c['Image'],
                'ip':c['NetworkSettings']['Networks'][self.id+'_lab']['IPAddress']})
        write_json(self.resources,mapping)
        for key in ['target-a','target-b']:
            ip=next(x['ip'] for x in mapping['containers'] if x['service']==key)
            self.shell(key,"printf '%s app.b02b.test\\n' "+ip+" > /var/lib/plan9/dns-hosts\nsupervisorctl -c /etc/plan9-supervisord.conf signal HUP dns\n")
        self.preflight()
        lock={'image_id':image['Id'],'architecture':image['Architecture'],
              'base_image_id':'sha256:752ef62afe9edd62067f6c1617fb79aebbbde7c5951fe633925a284de3b33ad7',
              'packages':self.shell('probe',"dpkg-query -W -f='${Package} ${Version}\\n' supervisor dnsmasq-base openssl iptables\n").splitlines(),
              'source_hashes':{str(p.relative_to(HERE)):hashlib.sha256(p.read_bytes()).hexdigest() for p in HERE.rglob('*') if p.is_file() and '__pycache__' not in p.parts and p.name!='images.lock.json'}}
        write_json(self.out/'image-lock.json',lock);write_json(HERE/'images.lock.json',lock)
        self.event('infrastructure_started',services=SERVICES,image_id=image['Id'])

    def preflight(self):
        output=self.shell('probe',"curl -fsS http://target-a:8080/health\ncurl -fsS https://target-b:8443/health\ndig +short @target-a -p 5353 app.b02b.test A\ndig +short @target-b -p 5353 app.b02b.test A\nping -c 2 -W 2 target-a\nping -c 2 -W 2 target-b\nopenssl s_client -brief -connect target-a:8443 -servername target-a -verify_hostname target-a -verify_return_error -CAfile /pki/ca.crt </dev/null 2>&1\n",timeout=30)
        write_json(self.out/'probe-network-preflight.json',{'observed_at':now(),'output':output,'tls_verification_disabled':False})

    def fault(self,kind,recover=False,retain=False):
        assert kind in ['http','tcp','tls','dns','icmp','tls_expired']
        if kind=='http':script='rm -f /var/lib/plan9/http-fault\n' if recover else 'touch /var/lib/plan9/http-fault\n'
        elif kind=='tcp':script=('supervisorctl -c /etc/plan9-supervisord.conf status tcp | grep -q RUNNING || supervisorctl -c /etc/plan9-supervisord.conf start tcp\n') if recover else 'supervisorctl -c /etc/plan9-supervisord.conf stop tcp\n'
        elif kind in ['tls','tls_expired']:
            value='normal' if recover else ('expired' if kind=='tls_expired' else 'short')
            script="printf '%s\\n' "+value+" > /var/lib/plan9/tls-variant\nsupervisorctl -c /etc/plan9-supervisord.conf restart tls\n"
        elif kind=='dns':
            ip=self.container('target-a')['NetworkSettings']['Networks'][self.id+'_lab']['IPAddress']
            script=("printf '%s app.b02b.test\\n' "+ip+" > /var/lib/plan9/dns-hosts\n" if recover else ': > /var/lib/plan9/dns-hosts\n')+'supervisorctl -c /etc/plan9-supervisord.conf signal HUP dns\n'
        else:
            rule='INPUT -p icmp --icmp-type echo-request -j DROP'
            script=('if iptables -C '+rule+' 2>/dev/null; then iptables -D '+rule+'; fi\n') if recover else ('iptables -C '+rule+' 2>/dev/null || iptables -I '+rule+'\n')
        family='tls' if kind=='tls_expired' else kind
        marker='/var/lib/plan9/fault-guard-'+family
        self.shell('target-a','rm -f '+marker+'\n'+script)
        if not recover and not retain:
            token=secrets.token_hex(12)
            self.shell('target-a',"printf '%s\\n' "+token+' > '+marker+'\n')
            ip=self.container('target-a')['NetworkSettings']['Networks'][self.id+'_lab']['IPAddress']
            recovery={
                'http':'rm -f /var/lib/plan9/http-fault',
                'tcp':'supervisorctl -c /etc/plan9-supervisord.conf start tcp',
                'tls':"printf 'normal\\n' > /var/lib/plan9/tls-variant; supervisorctl -c /etc/plan9-supervisord.conf restart tls",
                'dns':"printf '%s app.b02b.test\\n' "+ip+' > /var/lib/plan9/dns-hosts; supervisorctl -c /etc/plan9-supervisord.conf signal HUP dns',
                'icmp':'iptables -D INPUT -p icmp --icmp-type echo-request -j DROP',
            }[family]
            guard='sleep 180\nif [ "$(cat '+marker+' 2>/dev/null)" = "'+token+'" ]; then '+recovery+'; rm -f '+marker+'; fi\n'
            execute(['docker','exec','-d',self.container('target-a')['Id'],'/bin/sh','-c',guard])
        self.event('probe_recovered' if recover else 'probe_fault',probe=kind,target='target-a',retained=retain,automatic_recovery_seconds=None if recover or retain else 180)


if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['prepare','up','preflight','fault','recover']);p.add_argument('--kind',choices=['http','tcp','tls','dns','icmp','tls_expired'],default='http');args=p.parse_args();run=Run()
    if args.action in ('fault','recover'):run.fault(args.kind,args.action=='recover')
    else:getattr(run,args.action)()
