"""Owned RabbitMQ and actual AMQP clients, with bounded faults and recovery."""
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
HOSTS={'collector':{'letter':'C','ssh_port':22271,'name':'B05a-MQ-Collector'}}
SERVICES=['collector','rabbitmq']

class Run:
    def __init__(self,run_id='p9-b05a-20261028-01'):
        if not re.fullmatch(r'p9-b05a-\d{8}-\d{2}',run_id):raise ValueError('invalid B05a run')
        self.id=run_id;self.out=ROOT/'artifacts/plan9'/run_id
        self.resources=self.out/'resource-map.json';self.envfile=self.out/'compose.env'
    def event(self,kind,**fields):
        with (self.out/'timeline.jsonl').open('a',encoding='utf-8') as stream:stream.write(json.dumps({'time':now(),'event':kind,**fields})+'\n')
    def compose(self,*args,timeout=120):
        plugin=Path('C:/Program Files/Docker/Docker/resources/cli-plugins/docker-compose.exe')
        command=[str(plugin)] if os.name=='nt' and plugin.is_file() else ['docker','compose']
        return execute([*command,'--env-file',str(self.envfile),'--file',str(HERE/'compose.yaml'),'--project-name',self.id,*args],timeout=timeout)
    def container(self,key):
        assert key in SERVICES
        cid=self.compose('ps','--all','--quiet',key)
        if not cid:raise RuntimeError('Owned container missing: '+key)
        c=json.loads(execute(['docker','inspect',cid]))[0]
        assert c['Config']['Labels'].get('io.argus.plan9.run')==self.id and c['Config']['Labels'].get('io.argus.plan9.batch')=='B05a'
        return c
    def shell(self,key,script,timeout=120):return execute(['docker','exec','-i',self.container(key)['Id'],'/bin/sh','-se'],data=script,timeout=timeout)
    def prepare(self):
        self.out.mkdir(parents=True,exist_ok=True);private=self.out/'private';private.mkdir(exist_ok=True)
        if os.name=='nt':execute(['icacls',str(private),'/inheritance:r','/grant:r',execute(['whoami'])+':(OI)(CI)F','*S-1-5-18:(OI)(CI)F'])
        else:private.chmod(0o700)
        if not (private/'host-credentials.json').exists():write_json(private/'host-credentials.json',{'collector':{'username':'root','password':secrets.token_urlsafe(30)}})
        if not (private/'rabbitmq-credentials.json').exists():write_json(private/'rabbitmq-credentials.json',{role:secrets.token_urlsafe(30) for role in ['bootstrap','monitor','work','control']})
        credentials=json.loads((private/'rabbitmq-credentials.json').read_text(encoding='utf-8'))
        (private/'rabbitmq.conf').write_text('default_user = bootstrap\ndefault_pass = '+credentials['bootstrap']+'\ndefault_vhost = p9-work\nlisteners.tcp.default = 5672\nmanagement.tcp.port = 15672\nvm_memory_high_watermark.absolute = 256MiB\ndisk_free_limit.absolute = 64MB\ncollect_statistics_interval = 5000\n',encoding='utf-8',newline='\n')
        if not self.resources.exists():write_json(self.resources,{'run_id':self.id,'hosts':{},'targets':{},'dashboards':{},'alert_policies':{},'containers':[]})
        if not (self.out/'run.json').exists():write_json(self.out/'run.json',{'run_id':self.id,'batch':'B05a','created_at':now(),'status':'preparing','authorization':'B04 没啥问题，清理后部署 B05a，把 RabbitMQ 的真实场景、监控、仪表盘和告警都接好，我再去平台看。','budget':{'cpus':2.5,'memory_mib':2048,'vhosts':2,'queues':2,'queue_messages_max':500,'queue_bytes_max':1048576,'burst_messages_max':250,'hold_prefetch':10,'automatic_recovery_seconds':180,'background_hours_max':8}})
    def up(self):
        assert json.loads((ROOT/'artifacts/plan9/p9-b04-20261027-01/cleanup.json').read_text(encoding='utf-8'))['status']=='destroyed'
        mapping=json.loads(self.resources.read_text(encoding='utf-8'))
        if mapping['containers']:
            for item in mapping['containers']:assert self.container(item['service'])['Id']==item['id']
            return
        images={key:json.loads(execute(['docker','image','inspect',tag]))[0] for key,tag in {'HOST':'argus-plan9-rabbitmq:b05a-20261028','RABBITMQ':'rabbitmq:4-management-alpine'}.items()}
        self.envfile.write_text('PLAN9_RUN='+self.id+'\nPLAN9_PRIVATE='+(self.out/'private').as_posix()+'\n'+''.join('PLAN9_'+key+'_IMAGE='+v['Id']+'\n' for key,v in images.items()),encoding='utf-8',newline='\n')
        assert set(json.loads(self.compose('config','--format','json'))['services'])==set(SERVICES)
        self.compose('up','--detach','--no-build','--wait','--wait-timeout','180',timeout=240)
        password=json.loads((self.out/'private/host-credentials.json').read_text(encoding='utf-8'))['collector']['password']
        execute(['docker','exec','-i',self.container('collector')['Id'],'chpasswd'],data='root:'+password+'\n')
        self.shell('collector','install -d -m 700 /var/lib/plan9/private\n')
        execute(['docker','exec','-i',self.container('collector')['Id'],'python3','-c',"import sys,pathlib;p=pathlib.Path('/var/lib/plan9/private/rabbitmq.json');p.write_bytes(sys.stdin.buffer.read());p.chmod(0o600)"],data=(self.out/'private/rabbitmq-credentials.json').read_text(encoding='utf-8'))
        for key in SERVICES:
            c=self.container(key);mapping['containers'].append({'service':key,'id':c['Id'],'image_id':c['Image']})
        write_json(self.resources,mapping);lock={'images':{key:{'id':v['Id'],'digests':v.get('RepoDigests',[]),'architecture':v['Architecture']} for key,v in images.items()}}
        write_json(self.out/'image-lock.json',lock);write_json(HERE/'images.lock.json',lock);self.event('infrastructure_started')
    def client(self,action,side='work',count=1):
        assert action in ['initialize','native','burst','drain','permissions'] and side in ['work','control'] and 1<=count<=250
        return json.loads(self.shell('collector',f'python3 /usr/local/lib/plan9-mq-client.py {action} {side} --count {count}\n',timeout=90))
    def initialize(self):write_json(self.out/'protocol-preflight.json',self.client('initialize'));print('RabbitMQ actual management/AMQP initialization complete',flush=True)
    def workload(self):
        self.shell('collector','systemctl start plan9-mq-consumer@work plan9-mq-consumer@control plan9-mq-producer@work plan9-mq-producer@control\n')
        self.event('bounded_workload_started',maximum_seconds=28800)
    def fault(self,kind,recover=False):
        assert kind in ['backlog','unacked']
        if kind=='backlog':
            self.shell('collector','systemctl stop plan9-mq-recovery.timer 2>/dev/null || true\n'+('systemctl start plan9-mq-consumer@work\n' if recover else 'systemctl stop plan9-mq-consumer@work\nsystemd-run --unit=plan9-mq-recovery --on-active=180s /bin/systemctl start plan9-mq-consumer@work\n'))
        else:
            self.shell('collector','rm -f /var/lib/plan9/hold-work\n' if recover else "python3 -c \"import time,pathlib;pathlib.Path('/var/lib/plan9/hold-work').write_text(str(time.time()+180))\"\n")
        self.event('recovered' if recover else 'fault',scenario=kind,automatic_recovery_seconds=180)

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['prepare','up','initialize','workload']);a=p.parse_args();getattr(Run(),a.action)()
