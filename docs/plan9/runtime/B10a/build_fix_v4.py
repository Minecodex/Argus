"""Schema52 frozen release: complete native Namespace attribution without migrating formal data."""
import hashlib,json,os,shutil,subprocess
from infrastructure import ROOT
OUT=ROOT/'artifacts/plan9/p9-b10a-20261111-01';source=OUT/'backend-source-v4';context=OUT/'backend-image-v4'
if not source.exists():shutil.copytree(OUT/'backend-source-v3',source)
name='internal/telemetry/namespace_metric_identity.go';shutil.copy2(ROOT/name,source/name)
context.mkdir(exist_ok=True);env=os.environ.copy();env.update(CGO_ENABLED='0',GOOS='linux',GOARCH='amd64')
subprocess.run(['go','build','-trimpath','-ldflags=-s -w','-o',str(context/'argus-telemetry'),'./cmd/argus-telemetry'],cwd=source,env=env,check=True)
base=json.loads((OUT/'backend-image-v3.json').read_text(encoding='utf-8'))['containerimage.digest']
(context/'Dockerfile').write_text('FROM localhost:5001/argus/argus-backend:local-20261111-b10a-lifecycle-v3@'+base+'\nCOPY argus-telemetry /usr/local/bin/\n',encoding='utf-8')
subprocess.run(['docker','buildx','build','--platform','linux/amd64','-t','localhost:5001/argus/argus-backend:local-20261111-b10a-lifecycle-v4','--metadata-file',str(OUT/'backend-image-v4.json'),'--push',str(context)],check=True)
record=json.loads((OUT/'backend-source-v3.json').read_text(encoding='utf-8'))
record['overlays'][name]=hashlib.sha256((ROOT/name).read_bytes()).hexdigest()
(OUT/'backend-source-v4.json').write_text(json.dumps(record,indent=2),encoding='utf-8')
