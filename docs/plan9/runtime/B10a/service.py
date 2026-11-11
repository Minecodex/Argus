"""Real bounded HTTP/file/log workload for workbench operations."""
from datetime import datetime,timezone
from http.server import BaseHTTPRequestHandler,HTTPServer
from pathlib import Path
import hashlib,json,os,sys,time
from urllib.request import urlopen,Request
from urllib.parse import urlsplit,parse_qs
ROOT=Path('/data');ROOT.mkdir(exist_ok=True)
def state(label):
 token=Path('/secret/demo-token')
 result={'run':os.environ['PLAN9_RUN'],'pod':os.environ.get('POD_NAME','manual'),'pod_uid':os.environ.get('POD_UID','manual'),'version':os.environ.get('VERSION','v1'),'mode':Path('/config/mode').read_text(encoding="utf-8").strip() if Path('/config/mode').exists() else 'unset','secret_present':token.exists(),'marker':label[:80],'time':datetime.now(timezone.utc).isoformat()}
 (ROOT/'last-request.json').write_text(json.dumps(result))
 print(json.dumps(result),flush=True)
 return result
class Handler(BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def do_GET(self):
  label=parse_qs(urlsplit(self.path).query).get('label',['request'])[0]
  data=json.dumps({'status':'ok'} if self.path=='/health' else state(label)).encode()
  self.send_response(200);self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(data)));self.end_headers();self.wfile.write(data)
if __name__=='__main__':
 if len(sys.argv)==1 or sys.argv[1]=='serve':HTTPServer(('0.0.0.0',8080),Handler).serve_forever()
 elif sys.argv[1]=='daemon':
  while True:state('node-observation');time.sleep(30)
 elif sys.argv[1]=='load':
  deadline=float(os.environ['PLAN9_DEADLINE'])
  while time.time()<deadline:
   for name in ('b10a-api','b10a-ledger'):
    try:
     with urlopen('http://'+name+':8080/?label=bounded-load',timeout=3) as response:print(json.dumps({'target':name,'status':response.status}),flush=True)
    except Exception as error:print(json.dumps({'target':name,'error_type':type(error).__name__}),flush=True)
   time.sleep(15)
  print('Bounded workload ended; environment retained',flush=True)
  while True:time.sleep(300)
