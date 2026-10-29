"""Actual MongoDB wire-protocol CRUD and bounded independent client pressure."""
import argparse
from datetime import datetime,timezone
import json
from pathlib import Path
import signal
import time
import uuid
from pymongo import MongoClient,UpdateOne
from pymongo.errors import OperationFailure

CREDS=json.loads(Path('/var/lib/plan9/private/mongodb.json').read_text())
STOP=False
def stop(*_):
    global STOP
    STOP=True
signal.signal(signal.SIGTERM,stop)
signal.signal(signal.SIGINT,stop)
def now():return datetime.now(timezone.utc).isoformat()
def connect(side,role='app'):
    assert side in ['a','b'] and role in ['bootstrap','monitor','app']
    c=MongoClient('mongodb-'+side,27017,username=role,password=CREDS[side][role],authSource='admin',directConnection=True,maxPoolSize=1,minPoolSize=1,serverSelectionTimeoutMS=10000,connectTimeoutMS=5000,socketTimeoutMS=10000,appname='plan9-b06a-'+role)
    c.admin.command('ping');return c
def record(name,**values):
    p=Path('/var/lib/plan9/'+name+'.json');tmp=p.with_suffix('.tmp');tmp.write_text(json.dumps({'observed_at':now(),**values}));tmp.replace(p)
def native(side,kind=None):
    c=connect(side,'monitor')
    try:
        status=c.admin.command('serverStatus');db=c.plan9.command('dbStats',scale=1)
        return {'observed_at':now(),'side':side,'version':status['version'],'connections':status['connections'],'operations':status['opcounters'],'memory_mib':status['mem'],'wired_tiger_cache':{k:v for k,v in status.get('wiredTiger',{}).get('cache',{}).items() if k in ['bytes currently in the cache','maximum bytes configured','pages read into cache','pages written from cache']},'database':{k:db[k] for k in ['db','collections','objects','dataSize','storageSize','indexes','indexSize']}}
    finally:c.close()
def verify(side,kind=None):
    c=connect(side);expected=200 if side=='a' else 80
    try:
        count=c.plan9.samples.count_documents({});assert count==expected
        updates=list(c.plan9.samples.aggregate([{'$group':{'_id':None,'updates':{'$sum':'$counter'}}}]))[0]['updates']
        denied_cross=False
        try:c.admin['system.users'].find_one({})
        except OperationFailure as error:denied_cross=error.code==13
        assert denied_cross
    finally:c.close()
    monitor=connect(side,'monitor');denied=[]
    try:
        for operation in [lambda:monitor.plan9.samples.find_one({}),lambda:monitor.plan9.samples.insert_one({'_id':'forbidden-monitor-write'})]:
            try:operation()
            except OperationFailure as error:denied.append(error.code==13)
            else:denied.append(False)
        assert denied==[True,True]
    finally:monitor.close()
    return {'observed_at':now(),'side':side,'documents':count,'actual_counter_updates':updates,'monitor_read_and_write_denied':True,'app_admin_read_denied':True,'native':native(side)}
def initialize(side,kind=None):
    c=connect(side,'bootstrap')
    try:
        for role,roles in [('monitor',[{'role':'clusterMonitor','db':'admin'}]),('app',[{'role':'readWrite','db':'plan9'}])]:
            if not c.admin.command('usersInfo',role)['users']:c.admin.command('createUser',role,pwd=CREDS[side][role],roles=roles)
    finally:c.close()
    c=connect(side);temporary='protocol-'+uuid.uuid4().hex
    try:
        count=200 if side=='a' else 80
        c.plan9.samples.bulk_write([UpdateOne({'_id':i},{'$setOnInsert':{'counter':0,'bucket':i%8,'payload':'actual-mongodb-'+side+'-'+'x'*128}},upsert=True) for i in range(count)],ordered=True)
        c.plan9.samples.create_index('bucket')
        assert c.plan9.samples.insert_one({'_id':temporary,'value':1}).acknowledged
        assert c.plan9.samples.find_one({'_id':temporary})['value']==1
        changed=c.plan9.samples.update_one({'_id':temporary},{'$inc':{'value':1}});assert changed.modified_count==1
        assert c.plan9.samples.find_one({'_id':temporary})['value']==2
        assert c.plan9.samples.delete_one({'_id':temporary}).deleted_count==1
        assert c.plan9.samples.find_one({'_id':temporary}) is None
    finally:c.plan9.samples.delete_one({'_id':temporary});c.close()
    return {'actual_insert_find_update_delete':True,**verify(side)}
def background(side,kind=None):
    c=connect(side);start=time.monotonic();operations=0
    try:
        while not STOP and time.monotonic()-start<28800:
            key=operations%40;result=c.plan9.samples.update_one({'_id':key},{'$inc':{'counter':1}});assert result.modified_count==1
            doc=c.plan9.samples.find_one({'_id':key});assert doc['counter']>0 and doc['payload'].startswith('actual-mongodb-'+side)
            operations+=1;record('background-'+side,operations=operations)
            for _ in range(20 if side=='a' else 70):
                if STOP:break
                time.sleep(.1)
    finally:c.close()
    return {'completed_updates_and_reads':operations}
def pressure(side,kind):
    assert side=='a' and kind in ['connections','operations'];clients=[];operations=0;start=time.monotonic();marker=Path('/var/lib/plan9/stop-mongo-'+kind)
    try:
        for _ in range(12 if kind=='connections' else 1):
            c=connect(side);assert c.plan9.samples.find_one({'_id':0}) is not None;clients.append(c)
        record('pressure-'+kind,ready=True,opened_clients=len(clients),operations=0,max_seconds=180,max_operations=2000)
        while not STOP and time.monotonic()-start<180 and not marker.exists():
            if kind=='operations':
                if operations>=2000:break
                result=clients[0].plan9.samples.update_one({'_id':operations%40},{'$inc':{'counter':1}});assert result.modified_count==1;operations+=1
                if operations%20==0:record('pressure-'+kind,ready=True,opened_clients=1,operations=operations,max_seconds=180,max_operations=2000)
                time.sleep(.05)
            else:time.sleep(.25)
    finally:
        for c in clients:c.close()
    record('pressure-'+kind,ready=False,closed_clients=len(clients),completed_operations=operations,elapsed_seconds=round(time.monotonic()-start,3))
    return {'closed_clients':len(clients),'operations':operations}
if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['initialize','native','verify','background','pressure']);p.add_argument('side',choices=['a','b']);p.add_argument('--kind',choices=['connections','operations']);a=p.parse_args();print(json.dumps(globals()[a.action](a.side,a.kind)),flush=True)
