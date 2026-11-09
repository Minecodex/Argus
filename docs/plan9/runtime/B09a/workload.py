"""Real bounded HTTP, CPU, disk and stdout/stderr workload inside Kubernetes."""
import argparse
from datetime import datetime,timezone
import hashlib
from http.server import BaseHTTPRequestHandler,HTTPServer
import json
import os
from pathlib import Path
import signal
import sys
import time
import urllib.error
from urllib.parse import parse_qs,urlencode,urlsplit
from urllib.request import urlopen

POD=os.environ.get('POD_NAME','manual')
UID=os.environ.get('POD_UID','manual')
RUN=os.environ.get('PLAN9_RUN','p9-b09a-20261108-01')
DATA=Path('/data')
DATA.mkdir(exist_ok=True)


def observe(label, status=200):
    deadline=time.perf_counter()+.02
    value=b'actual-cpu-work'
    while time.perf_counter()<deadline:
        value=hashlib.sha256(value).digest()
    content=(RUN+' '+label+' '+POD).encode()*512
    path=DATA/'receipt.bin'
    path.write_bytes(content)
    disk=hashlib.sha256(path.read_bytes()).hexdigest()
    record={'message':RUN+'-'+label,'timestamp':datetime.now(timezone.utc).isoformat(),
            'pod':POD,'pod_uid':UID,'status':status,'bytes':len(content),'disk_sha256':disk}
    print(json.dumps(record),flush=True)
    if status>=500:
        print(json.dumps({**record,'message':record['message']+'-stderr','stream':'stderr'}),file=sys.stderr,flush=True)
    return record


class Handler(BaseHTTPRequestHandler):
    def log_message(self,*args):
        return

    def do_GET(self):
        request=urlsplit(self.path)
        if request.path=='/health':
            body={'status':'ok'}
            status=200
        else:
            label=parse_qs(request.query).get('label',['request'])[0][:80]
            status=503 if request.path=='/error' else 200
            body=observe(label,status)
        data=json.dumps(body).encode()
        self.send_response(status)
        self.send_header('Content-Type','application/json')
        self.send_header('Content-Length',str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def request(label):
    results=[]
    for service in ('orders','ledger'):
        path='/error' if 'error' in label and service=='orders' else '/orders'
        url='http://'+service+':8080'+path+'?'+urlencode({'label':label})
        try:
            response=urlopen(url,timeout=5)
        except urllib.error.HTTPError as error:
            response=error
        with response:
            result=json.load(response)
            assert result['status']==response.status
            results.append({'service':service,**result})
    return results


if __name__=='__main__':
    parser=argparse.ArgumentParser()
    parser.add_argument('action',choices=['serve','load','daemon','job','request'])
    parser.add_argument('--label',default='manual')
    args=parser.parse_args()
    if args.action=='serve':
        HTTPServer(('0.0.0.0',8080),Handler).serve_forever()
    elif args.action=='request':
        print(json.dumps(request(args.label)))
    elif args.action=='job':
        observe('completed-job')
    else:
        interval=5 if args.action=='load' else 30
        for index in range(8*3600//interval):
            if args.action=='load':
                try:
                    rows=request('live-error' if index%8==0 else 'live')
                    print(json.dumps({'requests':rows,'sequence':index}),flush=True)
                except (urllib.error.URLError,TimeoutError) as error:
                    print(type(error).__name__,file=sys.stderr,flush=True)
            else:
                observe('node-heartbeat')
            time.sleep(interval)
        signal.pause()
