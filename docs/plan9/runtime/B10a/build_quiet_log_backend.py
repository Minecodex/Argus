"""Freeze Schema52 and overlay only the retained-log read fix."""
import hashlib,json,os,shutil,subprocess
from infrastructure import ROOT
from run import Run
r=Run();out=r.out/'quiet-pod-repair';out.mkdir(exist_ok=True);source=out/'backend-source';context=out/'backend-image'
if not source.exists():shutil.copytree(r.out/'backend-source-v4',source)
files=['internal/kubernetespodlogs/read.go','internal/kubernetespodlogs/initial_tail.go','internal/kubernetesworkbench/log_sessions.go','api/openapi/components/kubernetes-log-sessions.yaml','internal/gen/openapi/kubernetesapi/types.gen.go']
for name in files:shutil.copy2(ROOT/name,source/name)
# Patch only this schema in the frozen bundles; preserve unrelated Schema52 contracts.
import yaml
for name in ('api/openapi/generated/argus.bundle.json','api/openapi/generated/kubernetesapi.bundle.yaml'):
 path=source/name
 data=json.loads(path.read_text(encoding='utf-8')) if path.suffix=='.json' else yaml.safe_load(path.read_text(encoding='utf-8'))
 field=data['components']['schemas']['KubernetesLogSessionInput']['properties']['since_seconds'];field['minimum']=0;field['description']='Initial lookback in seconds. Zero reads the latest retained tail without a time filter; byte and row limits still apply.'
 path.write_text(json.dumps(data,indent=2)+'\n' if path.suffix=='.json' else yaml.safe_dump(data,sort_keys=False),encoding='utf-8')
context.mkdir(exist_ok=True);env=os.environ.copy();env.update(CGO_ENABLED='0',GOOS='linux',GOARCH='amd64')
for name in ('argus-server','argus-worker','argus-connector-gateway','argus-connector'):
 subprocess.run(['go','build','-trimpath','-ldflags=-s -w','-o',str(context/name),'./cmd/'+name],cwd=source,env=env,check=True)
base=json.loads((r.out/'backend-image-v4.json').read_text(encoding='utf-8'))['containerimage.digest']
(context/'Dockerfile').write_text('FROM localhost:5001/argus/argus-backend:local-20261111-b10a-lifecycle-v4@'+base+'\nCOPY argus-server argus-worker argus-connector-gateway argus-connector /usr/local/bin/\n',encoding='utf-8')
subprocess.run(['docker','buildx','build','--platform','linux/amd64','-t','localhost:5001/argus/argus-backend:local-20261111-b10a-quiet-logs-v1','--metadata-file',str(out/'backend-image.json'),'--push',str(context)],check=True)
(out/'backend-source.json').write_text(json.dumps({'postgres_schema':52,'base':'backend-source-v4','overlays':{name:hashlib.sha256((ROOT/name).read_bytes()).hexdigest() for name in files}},indent=2),encoding='utf-8')
print('Bounded native-tail backend compiled without formal migrations',flush=True)
