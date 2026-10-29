"""Real, bounded SQL Server transactions through TDS; no telemetry injection."""
import argparse
import json
import os
from pathlib import Path
import signal
import threading
import time
os.environ['FREETDSCONF']='/etc/plan9-freetds.conf'
import pymssql

PRIVATE=Path('/var/lib/plan9/private/sqlserver.json')
STOP=threading.Event()
for sig in [signal.SIGTERM,signal.SIGINT]:signal.signal(sig,lambda *_:STOP.set())

def connect(side,role='app'):
    credentials=json.loads(PRIVATE.read_text())
    return pymssql.connect(server='sqlserver-'+side,port=1433,user=role,password=credentials[side][role],database='plan9' if role=='app' else 'master',login_timeout=10,timeout=35,autocommit=True,tds_version='7.4',appname='plan9-'+role)

def rows(connection,sql,args=None):
    cursor=connection.cursor(as_dict=True)
    try:
        cursor.execute(sql,args)
        return list(cursor) if cursor.description else []
    finally:cursor.close()

def native(side):
    with connect(side,'monitor') as c:
        version=rows(c,"SELECT CONVERT(varchar(100),SERVERPROPERTY('ProductVersion')) AS version,CONVERT(varchar(100),SERVERPROPERTY('Edition')) AS edition")[0]
        counters=rows(c,"SELECT RTRIM(object_name) AS object_name,RTRIM(counter_name) AS name,RTRIM(instance_name) AS instance_name,cntr_value AS value,cntr_type AS type FROM sys.dm_os_performance_counters WHERE counter_name IN ('User Connections','Batch Requests/sec','SQL Compilations/sec','Lock Waits/sec','Buffer cache hit ratio','Buffer cache hit ratio base','Page life expectancy')")
        sessions=rows(c,"SELECT session_id,program_name,status FROM sys.dm_exec_sessions WHERE is_user_process=1")
        blocked=rows(c,"SELECT session_id,blocking_session_id,wait_type,wait_time FROM sys.dm_exec_requests WHERE blocking_session_id>0")
    with connect(side) as c:table=rows(c,'SELECT COUNT(*) AS rows, SUM(counter) AS counter_total FROM dbo.samples')[0]
    return {'version':version,'counters':counters,'sessions':sessions,'blocked':blocked,'table':table}

def denied(connection,sql):
    try:rows(connection,sql)
    except pymssql.Error as exc:
        code=int(exc.args[0]);assert code in [229,262,297,916,15151,15247],str(code)
        return code
    raise AssertionError('Least-privilege boundary unexpectedly allowed the request')

def verify(side):
    with connect(side) as c:
        before=rows(c,'SELECT counter FROM dbo.samples WHERE id=149')[0]['counter']
        rows(c,'BEGIN TRANSACTION');rows(c,'UPDATE dbo.samples SET counter=counter+1 WHERE id=149');rows(c,'ROLLBACK TRANSACTION')
        assert rows(c,'SELECT counter FROM dbo.samples WHERE id=149')[0]['counter']==before
        rows(c,'BEGIN TRANSACTION');rows(c,'INSERT INTO dbo.samples(id,counter,payload) VALUES(900001,1,%s)',('transaction proof',));rows(c,'COMMIT TRANSACTION')
        assert rows(c,'SELECT counter FROM dbo.samples WHERE id=900001')[0]['counter']==1
        rows(c,'DELETE FROM dbo.samples WHERE id=900001')
        privilege=rows(c,"SELECT HAS_PERMS_BY_NAME(NULL,NULL,'CREATE ANY DATABASE') AS create_database")[0]
        assert privilege['create_database']==0
    with connect(side,'monitor') as c:permission_error=denied(c,'SELECT TOP(1) * FROM plan9.dbo.samples')
    n=native(side);assert n['table']['rows']==(200 if side=='a' else 80)
    return {'rows':n['table']['rows'],'commit_verified':True,'rollback_verified':True,'monitor_business_read_denied':permission_error,'app_create_database_permission':0,'native':n}

def initialize(side):
    credentials=json.loads(PRIVATE.read_text())[side]
    with connect(side,'sa') as c:
        rows(c,"IF DB_ID('plan9') IS NULL CREATE DATABASE plan9")
        rows(c,'ALTER DATABASE plan9 SET RECOVERY SIMPLE')
        for role in ['monitor','app']:
            if not rows(c,'SELECT name FROM sys.server_principals WHERE name=%s',(role,)):
                rows(c,'CREATE LOGIN ['+role+"] WITH PASSWORD=%s, CHECK_POLICY=ON, DEFAULT_DATABASE=["+('master' if role=='monitor' else 'plan9')+']',(credentials[role],))
        rows(c,'GRANT VIEW ANY DATABASE TO [monitor]');rows(c,'GRANT VIEW SERVER PERFORMANCE STATE TO [monitor]')
        rows(c,'USE plan9')
        rows(c,"IF OBJECT_ID('dbo.samples') IS NULL CREATE TABLE dbo.samples(id int NOT NULL PRIMARY KEY,counter bigint NOT NULL,payload nvarchar(512) NOT NULL)")
        rows(c,"IF USER_ID('app') IS NULL CREATE USER [app] FOR LOGIN [app]")
        rows(c,'GRANT SELECT,INSERT,UPDATE,DELETE ON dbo.samples TO [app]')
    with connect(side) as c:
        # B uses the same reserved transaction and lock row IDs with fewer rows.
        ids=list(range(200)) if side=='a' else list(range(78))+[149,150]
        for ident in ids:rows(c,'IF NOT EXISTS(SELECT 1 FROM dbo.samples WHERE id=%s) INSERT INTO dbo.samples(id,counter,payload) VALUES(%s,0,%s)',(ident,ident,'plan9-'+side+'-'+('x'*120)))
    return verify(side)

def background(side):
    end=time.monotonic()+28800;iteration=0
    with connect(side) as c:
        while time.monotonic()<end and not STOP.is_set():
            ident=iteration%40
            rows(c,'BEGIN TRANSACTION');rows(c,'UPDATE dbo.samples SET counter=counter+1 WHERE id=%s',(ident,));rows(c,'SELECT counter FROM dbo.samples WHERE id=%s',(ident,));rows(c,'ROLLBACK TRANSACTION' if iteration%5==0 else 'COMMIT TRANSACTION')
            iteration+=1;STOP.wait(2 if side=='a' else 7)

def save_pressure(kind,data):
    path=Path('/var/lib/plan9/pressure-'+kind+'.json');temporary=path.with_suffix('.tmp');temporary.write_text(json.dumps(data));temporary.replace(path)

def pressure(side,kind):
    end=time.monotonic()+180;marker=Path('/var/lib/plan9/stop-sql-'+kind);clients=[];count=0;state={'kind':kind,'ready':False,'recovered':False,'requests':0,'maximum_seconds':180}
    try:
        clients=[connect(side) for _ in range(12 if kind=='connections' else 1)]
        state.update(ready=True,connections=len(clients));save_pressure(kind,state)
        while time.monotonic()<end and not STOP.is_set() and not marker.exists() and count<2000:
            for c in clients:rows(c,'SELECT COUNT(*) AS rows FROM dbo.samples');count+=1
            state['requests']=count
            if count%20==0:save_pressure(kind,state)
            STOP.wait(1 if kind=='connections' else .05)
    finally:
        for c in clients:c.close()
        state.update(ready=False,recovered=True,requests=count);save_pressure(kind,state)

def lock(side):
    blocker=connect(side);waiter=connect(side);out={};done=threading.Event()
    def wait_for_lock():
        began=time.monotonic()
        try:
            rows(waiter,'SET LOCK_TIMEOUT 30000');rows(waiter,'BEGIN TRANSACTION');rows(waiter,'UPDATE dbo.samples SET counter=counter+1 WHERE id=150');out['wait_seconds']=round(time.monotonic()-began,3)
        except Exception as exc:out['error_type']=type(exc).__name__
        finally:
            rows(waiter,'IF @@TRANCOUNT>0 ROLLBACK TRANSACTION');waiter.close();done.set()
    try:
        rows(blocker,'BEGIN TRANSACTION');rows(blocker,'UPDATE dbo.samples SET counter=counter+1 WHERE id=150')
        thread=threading.Thread(target=wait_for_lock);thread.start();time.sleep(2)
        with connect(side,'monitor') as c:out['observed_blocked_requests']=rows(c,"SELECT session_id,blocking_session_id,wait_type,wait_time FROM sys.dm_exec_requests WHERE blocking_session_id>0")
        assert any(r['wait_type'].startswith('LCK_M_') for r in out['observed_blocked_requests']);time.sleep(18)
    finally:rows(blocker,'IF @@TRANCOUNT>0 ROLLBACK TRANSACTION');blocker.close()
    assert done.wait(15) and 'error_type' not in out and out['wait_seconds']>=19
    out['released']=True;out['rows_after']=native(side)['table']['rows'];return out

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['initialize','verify','native','background','pressure','lock']);parser.add_argument('side',choices=['a','b']);parser.add_argument('--kind',choices=['connections','batches']);args=parser.parse_args()
    if args.action=='pressure':pressure(args.side,args.kind)
    else:
        result=globals()[args.action](args.side)
        if result is not None:print(json.dumps(result))
