"""Actual HTTP activity and a bounded, reversible systemd failure fixture."""
import argparse
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
import json
import os
from pathlib import Path
import socket
import time
from urllib.request import urlopen

ROOT=Path('/var/lib/plan9/b11c')
class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        data={'run':os.environ['PLAN9_RUN'],'peer':os.environ['PLAN9_PEER'],'hostname':socket.gethostname(),'pid':os.getpid(),'at':time.time(),'path':self.path}
        self.send_response(200);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(json.dumps(data).encode())
        print(json.dumps(data),flush=True)
def serve():
    ROOT.mkdir(parents=True,exist_ok=True)
    if (ROOT/'fail-start').exists():print('B11c controlled service startup failure',flush=True);raise SystemExit(23)
    ThreadingHTTPServer(('0.0.0.0',18083),Handler).serve_forever()
def traffic():
    deadline=time.monotonic()+8*3600
    while time.monotonic()<deadline:
        try:
            with urlopen('http://127.0.0.1:18083/review',timeout=4) as response:response.read(4096)
        except OSError as error:print(type(error).__name__,flush=True)
        time.sleep(15)
if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['serve','traffic']);args=parser.parse_args();globals()[args.action]()
