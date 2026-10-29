"""Actual ZooKeeper sessions, digest ACLs, watches and ephemeral znodes."""
import argparse
from datetime import datetime,timezone
import json
from pathlib import Path
import signal
import socket
import threading
import time
import uuid
from kazoo.client import KazooClient
from kazoo.exceptions import NoAuthError,NodeExistsError
from kazoo.security import make_digest_acl

CREDS=json.loads(Path('/var/lib/plan9/private/zookeeper.json').read_text())
STOP=False
def stop(*_):
    global STOP
    STOP=True
signal.signal(signal.SIGTERM,stop)
signal.signal(signal.SIGINT,stop)
def now():return datetime.now(timezone.utc).isoformat()
def connect(side,authenticated=True):
    assert side in ['a','b'];creds=CREDS[side]
    auth={'auth_data':[('digest',creds['username']+':'+creds['password'])],'default_acl':[make_digest_acl(creds['username'],creds['password'],all=True)]} if authenticated else {}
    c=KazooClient(hosts='zookeeper-'+side+':2181',timeout=8,**auth);c.start(timeout=12);return c
def close(c):
    if c is not None:c.stop();c.close()
def four_letter(side,command):
    assert side in ['a','b'] and command in ['ruok','mntr','srvr','stat']
    with socket.create_connection(('zookeeper-'+side,2181),5) as sock:
        sock.settimeout(5);sock.sendall(command.encode());result=b''
        while True:
            data=sock.recv(8192)
            if not data:break
            result+=data;assert len(result)<1048576
    return result.decode()
def record(name,**values):
    p=Path('/var/lib/plan9/'+name+'.json');tmp=p.with_suffix('.tmp');tmp.write_text(json.dumps({'observed_at':now(),**values}));tmp.replace(p)
def native(side,kind=None):
    stats={}
    for line in four_letter(side,'mntr').splitlines():
        fields=line.split('\t',1)
        if len(fields)!=2:continue
        key,value=fields
        try:value=int(value)
        except ValueError:
            try:value=float(value)
            except ValueError:pass
        stats[key]=value
    assert stats['zk_server_state']=='standalone'
    c=connect(side)
    try:children={name:len(c.get_children('/plan9/'+name)) for name in ['connections','bulk']}
    finally:close(c)
    return {'observed_at':now(),'side':side,'stats':stats,'owned_ephemeral_children':children}
def initialize(side,kind=None):
    c=connect(side);observer=connect(side);changed=threading.Event();events=[]
    def watch(event):events.append({'type':event.type,'path':event.path});changed.set()
    try:
        for path in ['/plan9','/plan9/connections','/plan9/bulk']:c.ensure_path(path)
        for path in ['/plan9/value','/plan9/anchor']:
            try:c.create(path,b'initial')
            except NodeExistsError:pass
        observer.get('/plan9/value',watch=watch)
        value=('actual-update-'+uuid.uuid4().hex).encode();c.set('/plan9/value',value)
        assert c.get('/plan9/value')[0]==value and changed.wait(5)
        assert events[0]['type']=='CHANGED' and events[0]['path']=='/plan9/value'
        anonymous=connect(side,False);denied=False
        try:
            try:anonymous.get('/plan9/value')
            except NoAuthError:denied=True
            assert denied
        finally:close(anonymous)
        path='/plan9/protocol-ephemeral-'+uuid.uuid4().hex;c.create(path,b'actual-session',ephemeral=True)
        data,stat=observer.get(path);assert data==b'actual-session' and stat.ephemeralOwner==c.client_id[0]
        close(c);c=None;deadline=time.monotonic()+15
        while observer.exists(path):
            if time.monotonic()>deadline:raise RuntimeError('Server did not remove closed-session ephemeral node')
            time.sleep(.2)
    finally:close(c);close(observer)
    assert four_letter(side,'ruok').strip()=='imok'
    return {'observed_at':now(),'side':side,'actual_create_read_update':True,'watch_events':events,'anonymous_read_denied':denied,'ephemeral_owner_verified':True,'ephemeral_deleted_by_session_close':True,'native':native(side)}
def background(side,kind=None):
    c=connect(side);path='/plan9/live-'+uuid.uuid4().hex;c.create(path,b'live-session',ephemeral=True);c.exists('/plan9/anchor',watch=lambda _:None);start=time.monotonic();operations=0
    try:
        while not STOP and time.monotonic()-start<28800:
            value=json.dumps({'side':side,'counter':operations,'time':now()}).encode();assert len(value)<=1024
            c.set('/plan9/value',value);assert c.get('/plan9/value')[0]==value
            assert c.exists(path).ephemeralOwner==c.client_id[0]
            operations+=1;record('background-'+side,operations=operations,connected=c.connected)
            for _ in range(20):
                if STOP:break
                time.sleep(.1)
    finally:close(c)
    return {'completed_operations':operations}
def pressure(side,kind):
    assert side=='a' and kind in ['connections','nodes'];clients=[];paths=[];deadline=time.monotonic()+180
    try:
        if kind=='connections':
            for i in range(12):
                c=connect(side);clients.append(c);path='/plan9/connections/session-'+uuid.uuid4().hex;c.create(path,b'actual-session',ephemeral=True);paths.append(path);c.exists('/plan9/anchor',watch=lambda _:None)
        else:
            c=connect(side);clients.append(c)
            for i in range(80):
                path='/plan9/bulk/node-'+uuid.uuid4().hex;c.create(path,b'bounded-ephemeral-value',ephemeral=True);paths.append(path)
                _,stat=c.get(path,watch=lambda _:None);assert stat.ephemeralOwner==c.client_id[0]
        record('pressure-'+kind,ready=True,opened_sessions=len(clients),created_ephemeral_nodes=len(paths),max_seconds=180)
        while not STOP and time.monotonic()<deadline and not Path('/var/lib/plan9/stop-'+kind).exists():time.sleep(.25)
    finally:
        for c in clients:close(c)
    record('pressure-'+kind,ready=False,closed_sessions=len(clients),ephemeral_nodes_released=len(paths))
    return {'closed_sessions':len(clients),'server_session_cleanup_expected':True}
def verify_released(side,kind=None):
    c=connect(side);deadline=time.monotonic()+15
    try:
        while True:
            counts={name:len(c.get_children('/plan9/'+name)) for name in ['connections','bulk']}
            if not any(counts.values()):break
            if time.monotonic()>deadline:raise RuntimeError('Pressure ephemeral nodes remain: '+str(counts))
            time.sleep(.5)
    finally:close(c)
    return {'observed_at':now(),'server_removed_ephemeral_nodes':True,'remaining':counts,'native':native(side)}
if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['initialize','native','background','pressure','verify_released']);p.add_argument('side',choices=['a','b']);p.add_argument('--kind',choices=['connections','nodes']);a=p.parse_args();print(json.dumps(globals()[a.action](a.side,a.kind)),flush=True)
