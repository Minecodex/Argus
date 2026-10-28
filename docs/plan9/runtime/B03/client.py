"""Actual SQL clients for the four owned scenario databases, never telemetry."""
import argparse
from datetime import datetime,timezone
import json
from pathlib import Path
import threading
import time
import pymysql
import psycopg2

ALLOWED=['mysql-a','mysql-b','postgres-a','postgres-b']
SECRETS=json.loads(Path('/root/plan9-databases.json').read_text())

def connect(key,role='app',database='plan9'):
    assert key in ALLOWED
    password=SECRETS[key][role]
    if key.startswith('mysql'):
        return pymysql.connect(host=key,user=role,password=password,database=database,port=3306,connect_timeout=5,read_timeout=35,write_timeout=5,autocommit=True)
    c=psycopg2.connect(host=key,user='postgres' if role=='root' else role,password=password,dbname=database,port=5432,connect_timeout=5,application_name='plan9-b03-'+role)
    c.autocommit=True
    return c

def sql(c,statement,parameters=None):
    with c.cursor() as cur:
        cur.execute(statement,parameters)
        return cur.fetchall() if cur.description else []

def initialize(key):
    c=connect(key,'root',None if key.startswith('mysql') else 'plan9')
    try:
        if key.startswith('mysql'):
            sql(c,'CREATE DATABASE IF NOT EXISTS plan9')
            for role in ['monitor','app']:
                sql(c,"CREATE USER IF NOT EXISTS %s@'%%' IDENTIFIED BY %s",(role,SECRETS[key][role]))
            sql(c,"GRANT PROCESS, REPLICATION CLIENT ON *.* TO 'monitor'@'%'")
            sql(c,"GRANT SELECT ON performance_schema.* TO 'monitor'@'%'")
            sql(c,"GRANT SELECT,INSERT,UPDATE,DELETE,CREATE ON plan9.* TO 'app'@'%'")
        else:
            for role in ['monitor','app']:
                if not sql(c,'SELECT 1 FROM pg_roles WHERE rolname=%s',(role,)):
                    sql(c,'CREATE ROLE '+role+' LOGIN PASSWORD %s',(SECRETS[key][role],))
            sql(c,'GRANT pg_monitor TO monitor')
            sql(c,'REVOKE CREATE ON SCHEMA public FROM PUBLIC')
            sql(c,'GRANT CONNECT ON DATABASE plan9 TO monitor,app')
            sql(c,'GRANT USAGE,CREATE ON SCHEMA public TO app')
    finally:c.close()
    c=connect(key)
    try:
        sql(c,'CREATE TABLE IF NOT EXISTS samples(id INTEGER PRIMARY KEY, counter INTEGER NOT NULL, payload VARCHAR(128) NOT NULL)')
        count=200 if key.endswith('-a') else 50
        for i in range(count):
            suffix=' ON DUPLICATE KEY UPDATE id=id' if key.startswith('mysql') else ' ON CONFLICT(id) DO NOTHING'
            sql(c,'INSERT INTO samples VALUES (%s,0,%s)'+suffix,(i,'actual-plan9-'+key))
        sql(c,'ANALYZE TABLE samples' if key.startswith('mysql') else 'ANALYZE samples')
    finally:c.close()
    return verify(key)

def verify(key):
    c=connect(key)
    try:
        rows=sql(c,'SELECT count(*),sum(counter) FROM samples')[0]
        version=sql(c,'SELECT version()')[0][0]
    finally:c.close()
    monitor=connect(key,'monitor',None if key.startswith('mysql') else 'plan9')
    denied=[]
    try:
        for statement in ['SELECT * FROM plan9.samples' if key.startswith('mysql') else 'SELECT * FROM samples',
                          'CREATE TABLE plan9.monitor_forbidden(id INT)' if key.startswith('mysql') else 'CREATE TABLE monitor_forbidden(id INT)']:
            try:sql(monitor,statement)
            except (pymysql.MySQLError,psycopg2.Error):denied.append(True)
            else:raise RuntimeError('Monitoring identity unexpectedly accessed business data')
        native=sql(monitor,"SHOW GLOBAL STATUS WHERE Variable_name IN ('Threads_connected','Threads_running','Innodb_row_lock_current_waits','Com_commit')") if key.startswith('mysql') else sql(monitor,"SELECT state,count(*) FROM pg_stat_activity WHERE datname='plan9' GROUP BY state")
    finally:monitor.close()
    return {'target':key,'observed_at':datetime.now(timezone.utc).isoformat(),'version':version,'row_count':rows[0],'updates':int(rows[1]),'monitor_business_read_and_create_denied':len(denied)==2,'native_counters':native}

def background(key,seconds):
    deadline=time.monotonic()+seconds;operations=0;c=connect(key)
    try:
        while time.monotonic()<deadline:
            sql(c,'BEGIN')
            sql(c,'UPDATE samples SET counter=counter+1 WHERE id=%s',(operations%40,))
            sql(c,'SELECT count(*) FROM samples WHERE counter>=0')
            sql(c,'ROLLBACK' if operations%7==0 else 'COMMIT')
            operations+=1
            time.sleep(2 if key.endswith('-a') else 7)
    finally:c.close()
    return {'operations':operations,'seconds_budget':seconds}

def connections(key,seconds):
    assert seconds<=180
    connections=[];stop=Path('/var/lib/plan9/stop-connections-'+key);stop.unlink(missing_ok=True)
    try:
        for _ in range(10):connections.append(connect(key))
        Path('/var/lib/plan9/connections-'+key+'.json').write_text(json.dumps({'started_at':datetime.now(timezone.utc).isoformat(),'connections':len(connections),'maximum_seconds':seconds}))
        deadline=time.monotonic()+seconds
        while time.monotonic()<deadline and not stop.exists():time.sleep(1)
    finally:
        for c in connections:c.close()
    return {'closed_connections':len(connections)}

def lock(key,seconds):
    assert seconds<=30
    blocker=connect(key);waiter=connect(key);result={}
    def wait_for_lock():
        started=time.monotonic()
        try:
            sql(waiter,'BEGIN');sql(waiter,'UPDATE samples SET counter=counter+1 WHERE id=150');sql(waiter,'ROLLBACK')
            result['wait_seconds']=round(time.monotonic()-started,2)
        except Exception as error:result['error_type']=type(error).__name__
    try:
        sql(blocker,'BEGIN');sql(blocker,'UPDATE samples SET counter=counter+1 WHERE id=150')
        if key.startswith('mysql'):sql(waiter,'SET innodb_lock_wait_timeout=35')
        else:sql(waiter,"SET lock_timeout='35s'")
        thread=threading.Thread(target=wait_for_lock);thread.start();time.sleep(seconds)
        native=connect(key,'monitor',None if key.startswith('mysql') else 'plan9')
        try:
            result['blocked_native']=sql(native,"SHOW GLOBAL STATUS LIKE 'Innodb_row_lock_current_waits'") if key.startswith('mysql') else sql(native,"SELECT count(*) FROM pg_stat_activity WHERE datname='plan9' AND wait_event_type='Lock'")
        finally:native.close()
        sql(blocker,'ROLLBACK');thread.join(timeout=10)
        assert not thread.is_alive() and 'error_type' not in result
    finally:blocker.close();waiter.close()
    return result

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['init','verify','background','connections','lock','recover']);p.add_argument('key',choices=ALLOWED);p.add_argument('--seconds',type=int,default=180);a=p.parse_args()
    if a.action=='recover':Path('/var/lib/plan9/stop-connections-'+a.key).touch();result={'recovery_requested':True}
    elif a.action=='init':result=initialize(a.key)
    elif a.action=='verify':result=verify(a.key)
    else:result=globals()[a.action](a.key,a.seconds)
    print(json.dumps(result,default=str),flush=True)
