"""Issue bounded real HTTP requests and retain correlation IDs, never OTLP fixtures."""
import argparse
import json
import signal
import socket
import threading
import time
import urllib.request
import urllib.error
import uuid
STOP=threading.Event()
for sig in [signal.SIGTERM,signal.SIGINT]:signal.signal(sig,lambda *_:STOP.set())
HTTP=urllib.request.build_opener(urllib.request.ProxyHandler({}))
batch = socket.gethostname().partition('-')[2].partition('-')[0]
REQUEST_PREFIX = batch if batch in {'b07a', 'b07b', 'b07c'} else 'b07'

def request(label='background',sequence=0):
    identifier=f'{REQUEST_PREFIX}-{label}-{uuid.uuid4().hex}'
    req=urllib.request.Request('http://127.0.0.1:18080/orders?item='+str(sequence%64),headers={'X-Request-ID':identifier})
    start=time.monotonic()
    try:response=HTTP.open(req,timeout=6)
    except urllib.error.HTTPError as error:response=error
    with response:body=json.load(response);status=response.status
    return {'request_id':identifier,'status':status,'duration_seconds':time.monotonic()-start,'received_at':time.time(),'response':body}
def background():
    count=0;end=time.monotonic()+28800
    while time.monotonic()<end and count<60000 and not STOP.is_set():
        try:
            value=request(sequence=count)
            if count%20==0:print(json.dumps(value),flush=True)
        except Exception as exc:print(json.dumps({'request_error':type(exc).__name__}),flush=True)
        count+=1;STOP.wait(.5)
if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['request','background']);p.add_argument('--label',default='manual');a=p.parse_args()
    if a.action=='background':background()
    else:print(json.dumps(request(a.label)))
