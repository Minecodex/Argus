"""Real bounded file, HTTP, Job and CPU workload; no synthetic telemetry."""
from datetime import datetime,timezone
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit,parse_qs
from urllib.request import urlopen
import hashlib,json,os,sys,threading,time
DATA=Path('/data');DATA.mkdir(exist_ok=True)
cpu_until=0.0
def emit(kind,**values):
 row={'run':os.environ['PLAN9_RUN'],'pod':os.environ.get('POD_NAME'),'pod_uid':os.environ.get('POD_UID'),'time':datetime.now(timezone.utc).isoformat(),'kind':kind,**values}
 print(json.dumps(row),flush=True);return row
def capacity():
 s=os.statvfs(DATA);return s.f_blocks*s.f_frsize
def burn():
 while time.monotonic()<cpu_until:hashlib.sha256(b'B10b real CPU work'*4096).hexdigest()
class Handler(BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def do_GET(self):
  global cpu_until
  path=urlsplit(self.path);q=parse_qs(path.query)
  if path.path=='/health':row={'status':'ok'}
  elif path.path=='/cpu':
   seconds=min(150,max(0,int(q.get('seconds',['0'])[0])));was=cpu_until>time.monotonic();cpu_until=time.monotonic()+seconds
   if seconds and not was:threading.Thread(target=burn,daemon=True).start()
   row=emit('cpu-load',seconds=seconds)
  elif path.path=='/write':
   marker=q.get('marker',['request'])[0][:100];target=DATA/'receipt.json';target.write_text(json.dumps(emit('file-write',marker=marker)),encoding='utf-8')
   with target.open('rb') as f:os.fsync(f.fileno())
   row=json.loads(target.read_text(encoding='utf-8'));row.update(capacity_bytes=capacity())
  else:
   row=emit('file-read',capacity_bytes=capacity(),receipt=json.loads((DATA/'receipt.json').read_text(encoding='utf-8')) if (DATA/'receipt.json').exists() else None)
  raw=json.dumps(row).encode();self.send_response(200);self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(raw)));self.end_headers();self.wfile.write(raw)
if __name__=='__main__':
 mode=sys.argv[1] if len(sys.argv)>1 else 'serve'
 if mode=='serve':ThreadingHTTPServer(('0.0.0.0',8080),Handler).serve_forever()
 elif mode=='job':
  time.sleep(min(60,max(0,int(os.environ.get('JOB_DELAY','2')))))
  row=emit('job-completed',capacity_bytes=capacity())
  with (DATA/'jobs.jsonl').open('a',encoding='utf-8') as f:f.write(json.dumps(row)+'\n');f.flush();os.fsync(f.fileno())
 elif mode=='client':
  deadline=float(os.environ.get('PLAN9_DEADLINE',str(time.time()+8*3600)))
  while True:
   if time.time()<deadline:
    try:
     with urlopen('http://b10b-storage:8080/write?marker=bounded-background',timeout=3) as response:emit('http-activity',status=response.status)
    except Exception as e:emit('http-activity-failed',error_type=type(e).__name__)
   time.sleep(30 if time.time()<deadline else 300)
 else:raise ValueError(mode)
