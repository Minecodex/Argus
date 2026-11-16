"""A real, bounded HTTP/Redis service for B12 model/tool diagnosis."""
import argparse
from datetime import datetime,timezone
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
import json
import os
from pathlib import Path
import socket
import time
from urllib.request import urlopen
from redis_client import Redis

STATE=Path('/var/lib/plan9/b12');COUNTER=0

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        global COUNTER
        client=None
        try:
            client=Redis();assert client.command('PING')=='PONG'
            if self.path.startswith('/health'):result={'healthy':True,'redis':'PONG'}
            else:
                COUNTER+=1;key='p9:b12:order:'+str(COUNTER%64);cached=client.command('GET',key)
                hit=cached is not None
                if cached is None:
                    cached=json.dumps({'order':COUNTER%64,'quantity':1+COUNTER%4,'at':datetime.now(timezone.utc).isoformat()}).encode()
                    assert client.command('SET',key,cached,'EX',600)=='OK'
                assert client.command('GET',key)==cached
                result={'hostname':socket.gethostname(),'run':os.environ['PLAN9_RUN'],'request':COUNTER,
                    'cache_hit':hit,'cached_order':json.loads(cached),'at':datetime.now(timezone.utc).isoformat()}
                print(json.dumps(result),flush=True)
            raw=json.dumps(result).encode();self.send_response(200);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(raw)
        except (OSError,RuntimeError,AssertionError):
            self.send_response(503);self.end_headers();self.wfile.write(b'cache unavailable')
        finally:
            if client:client.close()

def serve():
    STATE.mkdir(parents=True,exist_ok=True)
    if (STATE/'fail-start').exists():print('B12 controlled startup failure',flush=True);raise SystemExit(23)
    ThreadingHTTPServer(('0.0.0.0',18084),Handler).serve_forever()

def traffic():
    deadline=time.monotonic()+28800
    while time.monotonic()<deadline:
        try:
            with urlopen('http://127.0.0.1:18084/orders',timeout=5) as response:response.read(4096)
        except OSError as error:print(type(error).__name__,flush=True)
        time.sleep(5)

def native():
    client=Redis('monitor')
    try:
        lines=client.command('INFO','ALL').decode().splitlines();fields=['redis_version','used_memory','maxmemory','connected_clients','total_commands_processed','keyspace_hits','keyspace_misses','evicted_keys']
        data={line.split(':',1)[0]:line.split(':',1)[1] for line in lines if ':' in line and line.split(':',1)[0] in fields}
        print(json.dumps({'at':datetime.now(timezone.utc).isoformat(),'stats':data}),flush=True)
    finally:client.close()

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['serve','traffic','native']);args=parser.parse_args();globals()[args.action]()
