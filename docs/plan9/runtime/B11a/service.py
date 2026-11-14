"""Actual bounded host HTTP traffic and native systemd fault fixture."""
import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import time
from urllib.request import urlopen

ROOT = Path('/var/lib/plan9/b11a')


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        body = json.dumps({'run':os.environ.get('PLAN9_RUN'),'pid':os.getpid(),'path':self.path,
                           'at':time.time()}).encode()
        self.send_response(200);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(body)
        print(body.decode(),flush=True)


def serve():
    ROOT.mkdir(parents=True,exist_ok=True)
    if (ROOT/'fail-start').exists():
        print('B11a controlled service startup failure',flush=True)
        raise SystemExit(23)
    ThreadingHTTPServer(('0.0.0.0',18081),Handler).serve_forever()


def traffic():
    deadline=time.monotonic()+8*3600
    while time.monotonic()<deadline:
        try:
            with urlopen('http://127.0.0.1:18081/review',timeout=4) as response:response.read(4096)
        except OSError as error:print(type(error).__name__,flush=True)
        time.sleep(15)


if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['serve','traffic']);args=p.parse_args()
    globals()[args.action]()
