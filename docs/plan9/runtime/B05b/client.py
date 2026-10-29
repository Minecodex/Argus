"""Actual Kafka production, processing and synchronous consumer commits."""
import argparse
from datetime import datetime,timezone
import json
from pathlib import Path
import signal
import time
import uuid
from confluent_kafka import Consumer,Producer,TopicPartition,KafkaException,KafkaError
from confluent_kafka.admin import AdminClient,NewTopic

CREDS=json.loads(Path('/var/lib/plan9/private/kafka.json').read_text())
STOP=False
def stop(*_):
    global STOP
    STOP=True
signal.signal(signal.SIGTERM,stop)
signal.signal(signal.SIGINT,stop)
def now():return datetime.now(timezone.utc).isoformat()
def config(role):
    return {'bootstrap.servers':'kafka:9092','security.protocol':'SASL_PLAINTEXT','sasl.mechanism':'PLAIN','sasl.username':role,'sasl.password':CREDS[role],'socket.timeout.ms':10000}
def reader(side,role=None):
    return Consumer({**config(role or side),'group.id':'p9-'+side+'-consumer','enable.auto.commit':False,'enable.auto.offset.store':False,'auto.offset.reset':'earliest','session.timeout.ms':10000,'queued.max.messages.kbytes':1024,'fetch.message.max.bytes':1048576})
def writer(side):
    return Producer({**config(side),'acks':'all','enable.idempotence':False,'retries':0,'message.timeout.ms':10000,'queue.buffering.max.messages':500,'queue.buffering.max.kbytes':1024})
def record(role,side,**values):
    p=Path('/var/lib/plan9/'+role+'-'+side+'.json');tmp=p.with_suffix('.tmp');tmp.write_text(json.dumps({'observed_at':now(),**values}));tmp.replace(p)
def publish(p,side,index,stage):
    identifier=uuid.uuid4().hex;outcome=[]
    def delivered(error,message):
        if error:outcome.append({'error':str(error)})
        else:outcome.append({'topic':message.topic(),'partition':message.partition(),'offset':message.offset(),'id':identifier})
    body=json.dumps({'id':identifier,'topic':'p9-'+side,'stage':stage,'sent_at':now()}).encode()
    p.produce('p9-'+side,key=identifier,value=body,partition=index%3,on_delivery=delivered)
    assert p.flush(12)==0 and len(outcome)==1 and 'error' not in outcome[0],outcome
    return outcome[0]
def offsets(side,role='monitor'):
    c=reader(side,role)
    try:
        parts=c.committed([TopicPartition('p9-'+side,i) for i in range(3)],timeout=10);rows=[]
        for p in parts:
            low,high=c.get_watermark_offsets(TopicPartition(p.topic,p.partition),timeout=10,cached=False)
            assert p.offset>=0 and high>=p.offset
            rows.append({'topic':p.topic,'partition':p.partition,'committed':p.offset,'log_start':low,'log_end':high,'lag':high-p.offset})
        return rows
    finally:c.close()
def native(side,count):
    admin=AdminClient(config('monitor'));metadata=admin.list_topics(timeout=10);groups=[]
    for name in ['work','control']:
        gid='p9-'+name+'-consumer';desc=next(g for g in admin.list_groups(group=gid,timeout=10) if g.id==gid)
        groups.append({'group':gid,'members':len(desc.members),'state':str(desc.state),'partitions':offsets(name)})
    return {'observed_at':now(),'broker_ids':list(metadata.brokers),'topics':{name:len(metadata.topics[name].partitions) for name in ['p9-work','p9-control']},'groups':groups}
def burst(side,count):
    assert 1<=count<=240
    p=writer(side);delivered=[publish(p,side,i,'burst') for i in range(count)]
    return {'observed_at':now(),'delivered':count,'partitions':{str(i):len([x for x in delivered if x['partition']==i]) for i in range(3)},'first':delivered[0],'last':delivered[-1]}
def process(c,msg,side):
    if msg.error():raise KafkaException(msg.error())
    body=json.loads(msg.value());assert body['id']==msg.key().decode() and body['topic']=='p9-'+side and msg.topic()=='p9-'+side
    committed=c.commit(message=msg,asynchronous=False)
    assert any(p.partition==msg.partition() and p.offset==msg.offset()+1 for p in committed)
    return {'id':body['id'],'partition':msg.partition(),'processed_offset':msg.offset(),'committed_offset':msg.offset()+1}
def drain(side,count):
    c=reader(side);c.subscribe(['p9-'+side]);processed=0;last_message=time.monotonic();deadline=time.monotonic()+50
    try:
        while time.monotonic()<deadline:
            msg=c.poll(1)
            if msg is not None:process(c,msg,side);processed+=1;last_message=time.monotonic()
            elif c.assignment() and time.monotonic()-last_message>3:break
    finally:c.close()
    return {'observed_at':now(),'processed_and_committed':processed}
def permissions(side,count):
    admin=AdminClient(config('monitor'));denied=False
    try:admin.create_topics([NewTopic('p9-forbidden-create',1,1)],request_timeout=5)['p9-forbidden-create'].result(10)
    except KafkaException as error:denied=error.args[0].code() in [KafkaError.TOPIC_AUTHORIZATION_FAILED,KafkaError.CLUSTER_AUTHORIZATION_FAILED]
    assert denied and 'p9-forbidden-create' not in admin.list_topics(timeout=10).topics
    c=reader('work');cross=False
    try:
        # ListOffsets may intentionally hide an unauthorized topic as unknown.
        # A real subscription exposes the authoritative authorization denial.
        c.subscribe(['p9-control']);deadline=time.monotonic()+20
        while time.monotonic()<deadline:
            msg=c.poll(1)
            if msg is None:continue
            assert msg.error(),'work account unexpectedly consumed the control topic'
            cross=msg.error().code()==KafkaError.TOPIC_AUTHORIZATION_FAILED
            break
    finally:c.close()
    assert cross
    return {'monitor_create_topic_denied':True,'work_control_topic_access_denied':True}
def initialize(side,count):
    report={}
    for name in ['work','control']:
        sent=burst(name,3);consumed=drain(name,3);assert consumed['processed_and_committed']>=3
        observed=offsets(name);assert all(x['committed']>=1 and x['lag']==0 for x in observed)
        report[name]={'publish':sent,'consume':consumed,'broker_offsets':observed}
    return {'observed_at':now(),'groups':report,'permissions':permissions(side,count),'native':native(side,count),'offsets_reset_or_injected':False}
def producer(side,count):
    p=writer(side);start=time.monotonic();sent=0
    while not STOP and time.monotonic()-start<28800:
        if sent%10==0:
            lag=sum(x['lag'] for x in offsets(side,side))
            if lag>=1000:record('producer',side,delivered=sent,stopped_reason='backlog_limit',lag=lag);break
        last=publish(p,side,sent,'background');sent+=1;record('producer',side,delivered=sent,last=last)
        for _ in range(20):
            if STOP:break
            time.sleep(.1)
    return {'delivered':sent}
def consumer(side,count):
    c=reader(side);c.subscribe(['p9-'+side]);start=time.monotonic();processed=0;paused=False;pending=[];last=None
    try:
        while not STOP and time.monotonic()-start<28800:
            marker=Path('/var/lib/plan9/pause-'+side);want=marker.exists() and time.time()<float(marker.read_text())
            assignment=c.assignment()
            if assignment and want!=paused:
                (c.pause if want else c.resume)(assignment);paused=want
            msg=c.poll(.5)
            if msg is not None:
                if msg.error():raise KafkaException(msg.error())
                pending.append(msg)
            if not want:
                while pending:last=process(c,pending.pop(0),side);processed+=1
            record('consumer',side,processed_and_committed=processed,paused=paused,assigned_partitions=len(c.assignment()),buffered_uncommitted=len(pending),last=last)
    finally:c.close()
    return {'processed_and_committed':processed}
if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['initialize','native','burst','drain','permissions','producer','consumer']);p.add_argument('side',choices=['work','control']);p.add_argument('--count',type=int,default=1);a=p.parse_args();print(json.dumps(globals()[a.action](a.side,a.count)),flush=True)
