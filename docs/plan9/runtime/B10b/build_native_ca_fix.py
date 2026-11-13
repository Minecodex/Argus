"""Freeze deployed Schema52 and overlay node public-CA support only."""
import hashlib,json,os,shutil,subprocess
from run import Run,ROOT
r=Run();out=r.out/'native-ca-file-fix';out.mkdir(exist_ok=True);source=out/'backend-source';context=out/'backend-image'
if not source.exists():shutil.copytree(ROOT/'artifacts/plan9/p9-b10a-20261111-01/quiet-pod-repair/backend-source',source)
files=['internal/builtincollection/catalog.go','internal/otelcol/configbundle/builtins.go','internal/kubernetesworkbench/worker.go','internal/kubernetesworkbench/superseded_observation.go','internal/builtincollection/config.go','internal/otelcol/configbundle/kubelet_trust.go','internal/collectormanager/kubernetes.go']
for name in files:shutil.copy2(ROOT/name,source/name)
context.mkdir(exist_ok=True);env=os.environ.copy();env.update(CGO_ENABLED='0',GOOS='linux',GOARCH='amd64')
for name in ('argus-server','argus-worker','argus-connector-gateway','argus-connector'):
 subprocess.run(['go','build','-trimpath','-ldflags=-s -w','-o',str(context/name),'./cmd/'+name],cwd=source,env=env,check=True)
base='localhost:5001/argus/argus-backend:local-20261111-b10a-quiet-logs-v1@sha256:e69ac4b3601cdcd06b1a84b76dde4111cd19ee9459a00b7491bf2de6b33dd1e9'
(context/'Dockerfile').write_text('FROM '+base+'\nCOPY argus-server argus-worker argus-connector-gateway argus-connector /usr/local/bin/\n',encoding='utf-8')
subprocess.run(['docker','buildx','build','--platform','linux/amd64','-t','localhost:5001/argus/argus-backend:local-20261112-b10b-native-ca-v3','--metadata-file',str(out/'backend-image.json'),'--push',str(context)],check=True)
(out/'backend-source.json').write_text(json.dumps({'postgres_schema':52,'base':base,'overlays':{name:hashlib.sha256((ROOT/name).read_bytes()).hexdigest() for name in files}},indent=2),encoding='utf-8')
print('Schema52 backend with node public CA built',flush=True)
