"""Real RabbitMQ management provisioning and confirmed AMQP traffic."""
import argparse
import base64
from datetime import datetime,timezone
import json
from pathlib import Path
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
import pika

CREDENTIALS=json.loads(Path('/var/lib/plan9/private/rabbitmq.json').read_text())
QUEUE='jobs'
QUEUE_ARGUMENTS={'x-max-length':500,'x-max-length-bytes':1048576,'x-overflow':'reject-publish','x-message-ttl':900000}

def now():return datetime.now(timezone.utc).isoformat()
def http(method,path,body=None,role='monitor',allowed=(200,201,204)):
    assert path.startswith('/api/')
    request=urllib.request.Request('http://rabbitmq:15672'+path,data=json.dumps(body).encode() if body is not None else None,method=method)
    request.add_header('Authorization','Basic '+base64.b64encode((role+':'+CREDENTIALS[role]).encode()).decode())
    request.add_header('Content-Type','application/json')
    opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        with opener.open(request,timeout=10) as response:status,raw=response.status,response.read(4194304)
    except urllib.error.HTTPError as error:status,raw=error.code,error.read(4096)
    assert status in allowed,(method,path,status)
    return json.loads(raw) if raw and status<400 else {'status':status}

def connection(side):
    assert side in ['work','control']
    return pika.BlockingConnection(pika.ConnectionParameters(host='rabbitmq',port=5672,virtual_host='p9-'+side,credentials=pika.PlainCredentials(side,CREDENTIALS[side]),heartbeat=30,blocked_connection_timeout=30,connection_attempts=5,retry_delay=2,socket_timeout=5))

def stats(role,side,**values):
    p=Path('/var/lib/plan9/'+role+'-'+side+'.json');tmp=p.with_suffix('.tmp');tmp.write_text(json.dumps({'observed_at':now(),**values}));tmp.replace(p)

def initialize(side,count):
    for vhost in ['p9-work','p9-control']:http('PUT','/api/vhosts/'+vhost,{},'bootstrap')
    for role in ['monitor','work','control']:
        http('PUT','/api/users/'+role,{'password':CREDENTIALS[role],'tags':'monitoring' if role=='monitor' else ''},'bootstrap')
    for name in ['work','control']:
        http('PUT','/api/permissions/p9-'+name+'/monitor',{'configure':'^$','write':'^$','read':'^$'},'bootstrap')
        http('PUT','/api/permissions/p9-'+name+'/'+name,{'configure':'^jobs$','write':'^(jobs|amq.default|)$','read':'^jobs$'},'bootstrap')
        c=connection(name)
        try:c.channel().queue_declare(queue=QUEUE,durable=True,arguments=QUEUE_ARGUMENTS)
        finally:c.close()
        burst(name,4);assert drain(name,4)['acknowledged']==4
    return {'observed_at':now(),'amqp_confirmed_messages_each':4,'manual_acknowledged_each':4,'permissions':permissions(side,count),'native':native(side,count)}

def permissions(side,count):
    denied=http('PUT','/api/queues/p9-work/forbidden-monitor-create',{'durable':True,'arguments':{}},allowed=(401,403))
    denied_read=False
    try:
        c=pika.BlockingConnection(pika.ConnectionParameters(host='rabbitmq',virtual_host='p9-control',credentials=pika.PlainCredentials('work',CREDENTIALS['work']),connection_attempts=1,socket_timeout=5))
    except pika.exceptions.ProbableAccessDeniedError:denied_read=True
    else:c.close()
    assert denied_read
    return {'monitor_queue_creation_denied':denied['status'],'work_to_control_vhost_denied':denied_read}

def native(side,count):
    queues=http('GET','/api/queues');nodes=http('GET','/api/nodes');overview=http('GET','/api/overview')
    return {'observed_at':now(),'version':overview['rabbitmq_version'],'queues':[{k:q.get(k) for k in ['name','vhost','messages','messages_ready','messages_unacknowledged','consumers','message_stats','arguments']} for q in queues],
            'nodes':[{k:n.get(k) for k in ['name','running','mem_used','disk_free','fd_used','uptime']} for n in nodes]}

def publish(channel,side,stage):
    identifier=uuid.uuid4().hex
    body=json.dumps({'id':identifier,'vhost':'p9-'+side,'stage':stage,'sent_at':now(),'payload':'real AMQP message'}).encode()
    channel.basic_publish(exchange='',routing_key=QUEUE,body=body,properties=pika.BasicProperties(message_id=identifier,content_type='application/json',delivery_mode=2),mandatory=True)
    return identifier

def burst(side,count):
    assert 1<=count<=250
    c=connection(side);ch=c.channel();ch.confirm_delivery();ids=[]
    try:
        for _ in range(count):ids.append(publish(ch,side,'burst'))
    finally:c.close()
    return {'observed_at':now(),'confirmed':count,'first_message_id':ids[0],'last_message_id':ids[-1],'vhost':'p9-'+side}

def drain(side,count):
    c=connection(side);ch=c.channel();acked=0
    try:
        for _ in range(500):
            method,properties,body=ch.basic_get(QUEUE,auto_ack=False)
            if method is None:break
            value=json.loads(body);assert value['id']==properties.message_id and value['vhost']=='p9-'+side
            ch.basic_ack(method.delivery_tag);acked+=1
    finally:c.close()
    return {'observed_at':now(),'acknowledged':acked}

def producer(side,count):
    c=connection(side);ch=c.channel();ch.confirm_delivery();sent=0;start=time.monotonic()
    try:
        while time.monotonic()-start<28800:
            identifier=publish(ch,side,'background');sent+=1;stats('producer',side,confirmed=sent,last_message_id=identifier)
            c.sleep(2)
    finally:c.close()
    return {'confirmed':sent}

def consumer(side,count):
    c=connection(side);ch=c.channel();ch.basic_qos(prefetch_count=10);pending=[];received=0;acked=0;last=None;start=time.monotonic()
    def held():
        p=Path('/var/lib/plan9/hold-'+side)
        return p.exists() and time.time()<float(p.read_text())
    def acknowledge(tag):
        nonlocal acked
        ch.basic_ack(tag);acked+=1
    def receive(channel,method,properties,body):
        nonlocal received,last
        value=json.loads(body);assert value['id']==properties.message_id and value['vhost']=='p9-'+side
        received+=1;last=value['id']
        if held():pending.append(method.delivery_tag)
        else:acknowledge(method.delivery_tag)
    ch.basic_consume(QUEUE,receive,auto_ack=False,consumer_tag='plan9-'+side)
    try:
        while time.monotonic()-start<28800:
            c.process_data_events(time_limit=1)
            if not held():
                while pending:acknowledge(pending.pop(0))
            stats('consumer',side,received=received,acknowledged=acked,unacknowledged=len(pending),last_message_id=last)
    finally:c.close()
    return {'received':received,'acknowledged':acked}

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['initialize','native','burst','drain','permissions','producer','consumer']);p.add_argument('side',choices=['work','control']);p.add_argument('--count',type=int,default=1);a=p.parse_args();print(json.dumps(globals()[a.action](a.side,a.count)),flush=True)
