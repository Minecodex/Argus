"""Actual HTTP requests, readiness failures and bounded client activity."""
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
import sys
import time
from urllib.request import urlopen


def record(kind, **fields):
    value = {'run':os.environ['PLAN9_RUN'],'pod':os.environ.get('POD_NAME'),
             'time':datetime.now(timezone.utc).isoformat(),'kind':kind,**fields}
    print(json.dumps(value), flush=True)
    return value


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_args):
        pass

    def do_GET(self):
        failed = os.environ.get('PLAN9_FAULT') == '1' and time.time() < float(os.environ.get('FAULT_UNTIL','0'))
        status = 503 if failed and self.path != '/health' else 200
        value = record('http-request',path=self.path,status=status,version=os.environ.get('CHART_VERSION','maintenance'))
        raw = json.dumps(value).encode()
        self.send_response(status)
        self.send_header('Content-Type','application/json')
        self.send_header('Content-Length',str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)


if __name__ == '__main__':
    if len(sys.argv) > 1 and sys.argv[1] == 'client':
        deadline = float(os.environ['PLAN9_DEADLINE'])
        while time.time() < deadline:
            for target in ('b10c-repo','b10c-oci'):
                try:
                    with urlopen('http://'+target+':8080/',timeout=3) as response:
                        record('actual-client-response',target=target,status=response.status)
                except Exception as error:
                    record('actual-client-failure',target=target,error=type(error).__name__)
            time.sleep(15)
        record('activity-budget-expired')
        while True:
            time.sleep(3600)
    else:
        ThreadingHTTPServer(('0.0.0.0',8080),Handler).serve_forever()
