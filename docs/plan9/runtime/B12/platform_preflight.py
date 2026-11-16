"""Read platform-owned Workspace prerequisites with the independent admin login."""
import json
import os
import requests
from manage import Run

run=Run();session=requests.Session();session.trust_env=False;session.verify=str(os.environ['ARGUS_PLAN9_CA_BUNDLE'])
session.headers['Origin']='https://platform.argus.dev'
login=session.post('https://platform.argus.dev/api/v1/platform/auth/login',json={'username':'admin','password':os.environ['ARGUS_PLAN9_PASSWORD']},timeout=30)
assert login.status_code==200 and login.json()['status']=='authenticated'
result={}
for kind in ['backends','images','profiles']:
    response=session.get('https://platform.argus.dev/api/v1/platform/sandbox/'+kind,timeout=30);assert response.status_code==200
    raw=response.json();rows=raw['items'] if isinstance(raw,dict) else raw
    result[kind]=[{k:v for k,v in row.items() if k in ['id','name','status','task_kind','enabled','image','image_ref','image_id','backend_id','cpu_millis','memory_mib','ttl_seconds']} for row in rows]
run.save('workspace-platform-preflight',result);print(json.dumps(result,indent=2))
