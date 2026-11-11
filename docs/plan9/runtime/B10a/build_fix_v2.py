"""Publish only the shared Kubernetes reference projection fix onto the live Schema52 baseline."""
import hashlib,json,os,shutil,subprocess
from pathlib import Path
from infrastructure import ROOT
OUT=ROOT/'artifacts/plan9/p9-b10a-20261111-01';source=OUT/'backend-source-v2';context=OUT/'backend-image-v2'
if not source.exists():shutil.copytree(OUT/'backend-source',source)
changed=['internal/kubernetesinventory/manifest_projection.go','internal/kubernetesinventory/projection.go','internal/kubernetesworkbench/mutation.go']
for name in changed:shutil.copy2(ROOT/name,source/name)
context.mkdir(exist_ok=True);env=os.environ.copy();env.update(CGO_ENABLED='0',GOOS='linux',GOARCH='amd64')
for name in ['argus-server','argus-worker']:
 subprocess.run(['go','build','-trimpath','-ldflags=-s -w','-o',str(context/name),'./cmd/'+name],cwd=source,env=env,check=True)
(context/'Dockerfile').write_text('FROM localhost:5001/argus/argus-backend:local-20261110-b09b-repairs-v5@sha256:48dd9340cabbbbdcbc79fdb577cd8f9d4db149bb88199203d15ca0a2d3d053c3\nCOPY argus-server argus-worker /usr/local/bin/\n')
subprocess.run(['docker','buildx','build','--platform','linux/amd64','-t','localhost:5001/argus/argus-backend:local-20261111-b10a-secret-reference-v2','--metadata-file',str(OUT/'backend-image-v2.json'),'--push',str(context)],check=True)
(OUT/'backend-source-v2.json').write_text(json.dumps({'base':'b09b-repairs-20261110/backend-source-v5','postgres_schema':52,'overlays':{name:hashlib.sha256((ROOT/name).read_bytes()).hexdigest() for name in changed}},indent=2))
