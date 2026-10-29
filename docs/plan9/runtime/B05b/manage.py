"""Owned KRaft Kafka and actual protocol clients. No Argus storage writes."""
import argparse
import base64
import json
import os
from pathlib import Path
import re
import secrets
import sys
import uuid
HERE=Path(__file__).resolve().parent
ROOT=HERE.parents[3]
sys.path.insert(0,str(HERE.parent))
from B01.manage import execute,write_json,now
HOSTS={'collector':{'letter':'C','ssh_port':22281,'name':'B05b-Kafka-Collector'}}
SERVICES=['collector','kafka']

class Run:
    def __init__(self,run_id='p9-b05b-20261028-01'):
        if not re.fullmatch(r'p9-b05b-\d{8}-\d{2}',run_id):raise ValueError('invalid B05b run')
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
        assert c['Config']['Labels'].get('io.argus.plan9.run')==self.id and c['Config']['Labels'].get('io.argus.plan9.batch')=='B05b'
        return c
    def shell(self,key,script,timeout=120):return execute(['docker','exec','-i',self.container(key)['Id'],'/bin/sh','-se'],data=script,timeout=timeout)
    def prepare(self):
        self.out.mkdir(parents=True,exist_ok=True);private=self.out/'private';private.mkdir(exist_ok=True)
        if os.name=='nt':execute(['icacls',str(private),'/inheritance:r','/grant:r',execute(['whoami'])+':(OI)(CI)F','*S-1-5-18:(OI)(CI)F'])
        else:private.chmod(0o700)
        if not (private/'host-credentials.json').exists():write_json(private/'host-credentials.json',{'collector':{'username':'root','password':secrets.token_urlsafe(30)}})
        if not (private/'kafka-credentials.json').exists():write_json(private/'kafka-credentials.json',{role:secrets.token_urlsafe(30) for role in ['monitor','work','control']})
        credentials=json.loads((private/'kafka-credentials.json').read_text(encoding='utf-8'))
        props='''process.roles=broker,controller
node.id=1
controller.listener.names=CONTROLLER
controller.quorum.bootstrap.servers=127.0.0.1:19093
listeners=INTERNAL://127.0.0.1:19092,EXTERNAL://0.0.0.0:9092,CONTROLLER://127.0.0.1:19093
advertised.listeners=INTERNAL://127.0.0.1:19092,EXTERNAL://kafka:9092
listener.security.protocol.map=INTERNAL:PLAINTEXT,EXTERNAL:SASL_PLAINTEXT,CONTROLLER:PLAINTEXT
inter.broker.listener.name=INTERNAL
sasl.enabled.mechanisms=PLAIN
authorizer.class.name=org.apache.kafka.metadata.authorizer.StandardAuthorizer
super.users=User:ANONYMOUS
allow.everyone.if.no.acl.found=false
auto.create.topics.enable=false
log.dirs=/tmp/kafka-data
num.partitions=3
offsets.topic.num.partitions=3
offsets.topic.replication.factor=1
transaction.state.log.replication.factor=1
transaction.state.log.min.isr=1
group.initial.rebalance.delay.ms=0
log.segment.bytes=1048576
log.retention.bytes=4194304
log.retention.ms=3600000
log.retention.check.interval.ms=60000
'''
        props+='listener.name.external.plain.sasl.jaas.config=org.apache.kafka.common.security.plain.PlainLoginModule required '+' '.join('user_'+role+'="'+value+'"' for role,value in credentials.items())+';\n'
        (private/'server.properties').write_text(props,encoding='utf-8',newline='\n')
        if not self.resources.exists():write_json(self.resources,{'run_id':self.id,'hosts':{},'targets':{},'dashboards':{},'alert_policies':{},'containers':[],'cluster_id':base64.urlsafe_b64encode(uuid.uuid4().bytes).decode().rstrip('=')})
        if not (self.out/'run.json').exists():write_json(self.out/'run.json',{'run_id':self.id,'batch':'B05b','created_at':now(),'status':'preparing','authorization':'下一批','budget':{'cpus':2.5,'memory_mib':2048,'brokers':1,'topics':2,'partitions_per_topic':3,'replication_factor':1,'retention_bytes_per_partition':4194304,'burst_max_messages':240,'producer_lag_ceiling':1000,'automatic_recovery_seconds':180,'background_hours_max':8}})
    def up(self):
        assert json.loads((ROOT/'artifacts/plan9/p9-b05a-20261028-01/cleanup.json').read_text(encoding='utf-8'))['status']=='destroyed'
        mapping=json.loads(self.resources.read_text(encoding='utf-8'))
        if mapping['containers']:
            for item in mapping['containers']:assert self.container(item['service'])['Id']==item['id']
            return
        images={key:json.loads(execute(['docker','image','inspect',tag]))[0] for key,tag in {'HOST':'argus-plan9-kafka:b05b-20261028','KAFKA':'apache/kafka:4.2.1'}.items()}
        self.envfile.write_text('PLAN9_RUN='+self.id+'\nPLAN9_PRIVATE='+(self.out/'private').as_posix()+'\nPLAN9_CLUSTER_ID='+mapping['cluster_id']+'\n'+''.join('PLAN9_'+key+'_IMAGE='+v['Id']+'\n' for key,v in images.items()),encoding='utf-8',newline='\n')
        assert set(json.loads(self.compose('config','--format','json'))['services'])==set(SERVICES)
        self.compose('up','--detach','--no-build','--wait','--wait-timeout','180',timeout=240)
        password=json.loads((self.out/'private/host-credentials.json').read_text(encoding='utf-8'))['collector']['password']
        execute(['docker','exec','-i',self.container('collector')['Id'],'chpasswd'],data='root:'+password+'\n')
        self.shell('collector','install -d -m 700 /var/lib/plan9/private\n')
        execute(['docker','exec','-i',self.container('collector')['Id'],'python3','-c',"import sys,pathlib;p=pathlib.Path('/var/lib/plan9/private/kafka.json');p.write_bytes(sys.stdin.buffer.read());p.chmod(0o600)"],data=(self.out/'private/kafka-credentials.json').read_text(encoding='utf-8'))
        for key in SERVICES:
            c=self.container(key);mapping['containers'].append({'service':key,'id':c['Id'],'image_id':c['Image']})
        write_json(self.resources,mapping);lock={'images':{key:{'id':v['Id'],'digests':v.get('RepoDigests',[]),'architecture':v['Architecture']} for key,v in images.items()}}
        write_json(self.out/'image-lock.json',lock);write_json(HERE/'images.lock.json',lock);self.event('infrastructure_started')
    def cli(self,script,*args):
        assert script in ['kafka-topics.sh','kafka-acls.sh','kafka-consumer-groups.sh']
        return execute(['docker','exec',self.container('kafka')['Id'],'/opt/kafka/bin/'+script,'--bootstrap-server','127.0.0.1:19092',*args],timeout=90)
    def initialize(self):
        for side in ['work','control']:
            self.cli('kafka-topics.sh','--create','--if-not-exists','--topic','p9-'+side,'--partitions','3','--replication-factor','1')
        self.cli('kafka-acls.sh','--add','--allow-principal','User:monitor','--operation','Describe','--cluster')
        self.cli('kafka-acls.sh','--add','--allow-principal','User:monitor','--operation','Describe','--operation','DescribeConfigs','--topic','p9-','--resource-pattern-type','prefixed')
        self.cli('kafka-acls.sh','--add','--allow-principal','User:monitor','--operation','Describe','--group','p9-','--resource-pattern-type','prefixed')
        for side in ['work','control']:
            self.cli('kafka-acls.sh','--add','--allow-principal','User:'+side,'--operation','Read','--operation','Write','--operation','Describe','--topic','p9-'+side)
            self.cli('kafka-acls.sh','--add','--allow-principal','User:'+side,'--operation','Read','--operation','Describe','--group','p9-'+side+'-consumer')
        report=self.client('initialize');write_json(self.out/'protocol-preflight.json',report);print('Actual records consumed and offsets committed for both groups',flush=True)
    def client(self,action,side='work',count=1):
        assert action in ['initialize','native','burst','drain','permissions'] and side in ['work','control'] and 1<=count<=240
        return json.loads(self.shell('collector',f'python3 /usr/local/lib/plan9-kafka-client.py {action} {side} --count {count}\n',timeout=120))
    def workload(self):
        self.shell('collector','systemctl start plan9-kafka-consumer@work plan9-kafka-consumer@control plan9-kafka-producer@work plan9-kafka-producer@control\n');self.event('bounded_workload_started',maximum_seconds=28800)
    def fault(self,kind,recover=False):
        assert kind in ['lag','members']
        if kind=='lag':
            self.shell('collector','rm -f /var/lib/plan9/pause-work\n' if recover else "python3 -c \"import time,pathlib;pathlib.Path('/var/lib/plan9/pause-work').write_text(str(time.time()+180))\"\n")
        else:
            self.shell('collector','systemctl stop plan9-kafka-recovery.timer 2>/dev/null || true\n'+('systemctl start plan9-kafka-consumer@work plan9-kafka-producer@work\n' if recover else 'systemctl stop plan9-kafka-producer@work plan9-kafka-consumer@work\nsystemd-run --unit=plan9-kafka-recovery --on-active=180s /bin/systemctl start plan9-kafka-consumer@work plan9-kafka-producer@work\n'))
        self.event('recovered' if recover else 'fault',scenario=kind,automatic_recovery_seconds=180)

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['prepare','up','initialize','workload']);a=p.parse_args();getattr(Run(),a.action)()
