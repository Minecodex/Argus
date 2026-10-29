"""Bounded native Oracle sessions and transactions; never inject telemetry."""
import argparse
import json
from pathlib import Path
import signal
import threading
import time
import oracledb

PRIVATE=Path('/var/lib/plan9/private/oracle.json')
STOP=threading.Event()
for sig in [signal.SIGTERM,signal.SIGINT]:signal.signal(sig,lambda *_:STOP.set())

def connect(side,role='app'):
    credentials=json.loads(PRIVATE.read_text())
    connection=oracledb.connect(user='PLAN9_APP' if role=='app' else 'MONITOR',password=credentials[side][role],dsn='oracle-'+side+':1521/FREEPDB1',tcp_connect_timeout=10)
    connection.call_timeout=15000;connection.module='plan9-'+role
    return connection

def rows(c,sql,args=None):
    with c.cursor() as cursor:
        cursor.execute(sql,args or [])
        if cursor.description:
            names=[d[0].lower() for d in cursor.description]
            return [dict(zip(names,row)) for row in cursor]
        return []

def native(side):
    with connect(side,'monitor') as c:
        version=c.version
        counters=rows(c,"SELECT name,value FROM v$sysstat WHERE name IN ('user commits','user rollbacks','CPU used by this session','session pga memory')")
        observed=time.monotonic()
        sessions=rows(c,"SELECT status,type,COUNT(*) AS count FROM v$session GROUP BY status,type")
        clients=rows(c,"SELECT sid,serial#,username,status,module FROM v$session WHERE username='PLAN9_APP'")
        spaces=rows(c,'SELECT tablespace_name,used_space,tablespace_size,used_percent FROM dba_tablespace_usage_metrics')
    with connect(side) as c:table=rows(c,'SELECT COUNT(*) AS row_count,SUM(counter) AS counter_total FROM samples')[0]
    table['rows']=table.pop('row_count')
    return {'version':version,'counter_observed_monotonic':observed,'counters':counters,'sessions':sessions,'app_sessions':clients,'tablespaces':spaces,'table':table}

def denied(c,sql):
    try:rows(c,sql)
    except oracledb.DatabaseError as exc:
        code=exc.args[0].code;assert code in [942,1031],str(code);return code
    raise AssertionError('Least-privilege boundary unexpectedly allowed the operation')

def verify(side):
    with connect(side) as c:
        before=rows(c,'SELECT counter FROM samples WHERE id=149')[0]['counter']
        rows(c,'UPDATE samples SET counter=counter+1 WHERE id=149');c.rollback()
        assert rows(c,'SELECT counter FROM samples WHERE id=149')[0]['counter']==before
        rows(c,'INSERT INTO samples(id,counter,payload) VALUES(900001,1,:payload)',{'payload':'commit proof'});c.commit()
        with connect(side) as reader:assert rows(reader,'SELECT counter FROM samples WHERE id=900001')[0]['counter']==1
        rows(c,'DELETE FROM samples WHERE id=900001');c.commit()
        app_denied=denied(c,'CREATE TABLE plan9_forbidden(id NUMBER)')
    with connect(side,'monitor') as c:
        read_denied=denied(c,'SELECT * FROM PLAN9_APP.samples WHERE ROWNUM=1')
        write_denied=denied(c,'CREATE TABLE plan9_forbidden(id NUMBER)')
    n=native(side);assert n['table']['rows']==(200 if side=='a' else 80)
    return {'rows':n['table']['rows'],'commit_visible_in_second_session':True,'rollback_verified':True,'app_create_table_denied':app_denied,'monitor_business_read_denied':read_denied,'monitor_create_table_denied':write_denied,'native':n}

def initialize(side):
    with connect(side) as c:
        ids=list(range(200)) if side=='a' else list(range(79))+[149]
        for ident in ids:rows(c,'MERGE INTO samples t USING (SELECT :id AS id FROM dual) s ON (t.id=s.id) WHEN NOT MATCHED THEN INSERT(id,counter,payload) VALUES(s.id,0,:payload)',{'id':ident,'payload':'p9-'+side+'-'+('x'*120)})
        c.commit()
    return verify(side)

def background(side):
    end=time.monotonic()+28800;i=0
    with connect(side) as c:
        while time.monotonic()<end and not STOP.is_set():
            rows(c,'UPDATE samples SET counter=counter+1 WHERE id=:id',{'id':i%40});rows(c,'SELECT counter FROM samples WHERE id=:id',{'id':i%40})
            if i%5==0:c.rollback()
            else:c.commit()
            i+=1;STOP.wait(2 if side=='a' else 7)

def save_pressure(kind,state):
    p=Path('/var/lib/plan9/pressure-'+kind+'.json');tmp=p.with_suffix('.tmp');tmp.write_text(json.dumps(state));tmp.replace(p)

def pressure(side,kind):
    clients=[];end=time.monotonic()+180;marker=Path('/var/lib/plan9/stop-oracle-'+kind);count=0;state={'kind':kind,'ready':False,'recovered':False,'operations':0,'maximum_seconds':180}
    try:
        clients=[connect(side) for _ in range(12 if kind=='sessions' else 1)]
        state.update(ready=True,connections=len(clients));save_pressure(kind,state)
        while time.monotonic()<end and not STOP.is_set() and not marker.exists() and count<2000:
            if kind=='sessions':
                for c in clients:rows(c,'SELECT 1 FROM dual')
                count+=1;STOP.wait(5)
            else:
                rows(clients[0],'UPDATE samples SET counter=counter+1 WHERE id=100');clients[0].commit();count+=1;STOP.wait(.05)
            state['operations']=count
            if count%10==0:save_pressure(kind,state)
    finally:
        for c in clients:c.close()
        state.update(ready=False,recovered=True,operations=count);save_pressure(kind,state)

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['initialize','verify','native','background','pressure']);p.add_argument('side',choices=['a','b']);p.add_argument('--kind',choices=['sessions','commits']);a=p.parse_args()
    if a.action=='pressure':pressure(a.side,a.kind)
    else:
        result=globals()[a.action](a.side)
        if result is not None:print(json.dumps(result))
