"""Bounded real RESP and Memcached text-protocol workloads."""
import argparse
from datetime import datetime,timezone
import json
from pathlib import Path
import socket
import time

class Redis:
    def __init__(self,role='app'):
        self.sock=socket.create_connection(('redis',6379),5);self.sock.settimeout(5);self.reader=self.sock.makefile('rb')
        credentials=json.loads(Path('/root/plan9-redis.json').read_text())
        self.command('AUTH',role,credentials[role])
    def response(self):
        line=self.reader.readline();kind=line[:1];data=line[1:-2]
        if kind==b'-':raise RuntimeError(data.decode())
        if kind==b'+':return data.decode()
        if kind==b':':return int(data)
        if kind==b'$':
            size=int(data)
            if size<0:return None
            assert size<1048576
            value=self.reader.read(size);assert self.reader.read(2)==b'\r\n';return value
        if kind==b'*':return [self.response() for _ in range(int(data))]
        raise RuntimeError('Invalid RESP response')
    def command(self,*args):
        values=[x if isinstance(x,bytes) else str(x).encode() for x in args]
        self.sock.sendall(b'*'+str(len(values)).encode()+b'\r\n'+b''.join(b'$'+str(len(x)).encode()+b'\r\n'+x+b'\r\n' for x in values))
        return self.response()
    def put(self,key,value,ttl):assert self.command('SET',key,value,'EX',ttl)=='OK'
    def get(self,key):return self.command('GET',key)
    def delete(self,key):self.command('DEL',key)
    def close(self):self.reader.close();self.sock.close()

class Memcached:
    def __init__(self):
        self.sock=socket.create_connection(('memcached',11211),5);self.sock.settimeout(5);self.reader=self.sock.makefile('rb')
    def put(self,key,value,ttl):
        self.sock.sendall(f'set {key} 0 {ttl} {len(value)}\r\n'.encode()+value+b'\r\n');assert self.reader.readline()==b'STORED\r\n'
    def get(self,key):
        self.sock.sendall(f'get {key}\r\n'.encode());line=self.reader.readline()
        if line==b'END\r\n':return None
        fields=line.split();assert fields[0]==b'VALUE';size=int(fields[3]);assert size<131072
        result=self.reader.read(size);assert self.reader.read(2)==b'\r\n';assert self.reader.readline()==b'END\r\n';return result
    def delete(self,key):
        self.sock.sendall(f'delete {key}\r\n'.encode());assert self.reader.readline() in [b'DELETED\r\n',b'NOT_FOUND\r\n']
    def stats(self):
        self.sock.sendall(b'stats\r\n');result={}
        while True:
            line=self.reader.readline()
            if line==b'END\r\n':return result
            assert line.startswith(b'STAT ')
            _,key,value=line.decode().strip().split(' ',2);result[key]=int(value) if value.isdigit() else value
    def close(self):self.reader.close();self.sock.close()

def connect(engine):
    assert engine in ['redis','memcached']
    return Redis() if engine=='redis' else Memcached()

def native(engine):
    c=Redis('monitor') if engine=='redis' else Memcached()
    try:
        if engine=='redis':
            info=c.command('INFO','ALL').decode();result={}
            keys=['redis_version','used_memory','used_memory_rss','maxmemory','evicted_keys','expired_keys','keyspace_hits','keyspace_misses','connected_clients','total_commands_processed']
            for line in info.splitlines():
                if ':' in line:
                    key,value=line.split(':',1)
                    if key in keys:result[key]=int(value) if value.isdigit() else value
        else:
            all_stats=c.stats();result={k:all_stats[k] for k in ['version','bytes','limit_maxbytes','curr_items','curr_connections','get_hits','get_misses','evictions','cmd_get','cmd_set']}
        return {'engine':engine,'observed_at':datetime.now(timezone.utc).isoformat(),'stats':result}
    finally:c.close()

def verify(engine):
    before=native(engine);c=connect(engine)
    try:
        c.put('p9:verify',b'actual-cache-value',2)
        assert c.get('p9:verify')==b'actual-cache-value'
        c.delete('p9:missing');assert c.get('p9:missing') is None
        time.sleep(3);assert c.get('p9:verify') is None
    finally:c.close()
    denied=None
    if engine=='redis':
        monitor=Redis('monitor')
        try:
            try:monitor.get('p9:verify')
            except RuntimeError as error:denied='NOPERM' in str(error)
            else:denied=False
            assert denied
        finally:monitor.close()
    return {'before':before,'after':native(engine),'actual_hit':True,'actual_miss':True,'ttl_expired':True,'monitor_key_read_denied':denied}

def pressure(engine):
    before=native(engine);c=connect(engine)
    try:
        for i in range(1024):c.put('p9:pressure:'+str(i),b'x'*32768,120)
    finally:c.close()
    after=native(engine);field='evicted_keys' if engine=='redis' else 'evictions'
    assert after['stats'][field]>before['stats'][field]
    return {'started_at':before['observed_at'],'finished_at':after['observed_at'],'sent_keys':1024,'sent_bytes':33554432,'ttl_seconds':120,'before':before,'after':after,'actual_evictions':after['stats'][field]-before['stats'][field]}

def recover(engine):
    c=connect(engine)
    try:
        for i in range(1024):c.delete('p9:pressure:'+str(i))
    finally:c.close()
    return native(engine)

def background(engine):
    c=connect(engine);start=time.monotonic();operations=0
    try:
        while time.monotonic()-start<28800:
            key='p9:live:'+str(operations%32);value=b'bounded-real-request'*16
            c.put(key,value,12);assert c.get(key)==value
            assert c.get('p9:known-missing') is None
            operations+=1;time.sleep(2)
    finally:c.close()
    return {'operations':operations,'maximum_seconds':28800}

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['verify','native','pressure','recover','background']);p.add_argument('engine',choices=['redis','memcached']);a=p.parse_args();print(json.dumps(globals()[a.action](a.engine)),flush=True)
