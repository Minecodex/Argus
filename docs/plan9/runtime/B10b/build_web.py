"""Publish the shared node CA form on the deployed frozen Web baseline."""
import hashlib,json,shutil,subprocess
from run import Run,ROOT
r=Run();out=r.out/'node-ca-web';out.mkdir(exist_ok=True);source=out/'web-source'
if not source.exists():shutil.copytree(ROOT/'artifacts/plan9/p9-b10a-20261111-01/terminal-options-audit/web-source',source,ignore=shutil.ignore_patterns('node_modules','dist','*.tsbuildinfo','test-results'))
files=['web/apps/enterprise/src/components/monitoring/component-trust-dialog.tsx','web/apps/enterprise/src/i18n/builtin-collections.ts','web/packages/api-client/src/generated/builtin-collections.ts']
for name in files:shutil.copy2(ROOT/name,source/name)
(out/'source.json').write_text(json.dumps({'baseline_image':'host.docker.internal:5001/argus/argus-web:local-20261111-b10a-log-options-v1@sha256:a786bf0430b3aac4e46db3e2ee9ba774818287eea50c4c4b309e14c10235744f','overlays':{name:hashlib.sha256((ROOT/name).read_bytes()).hexdigest()for name in files}},indent=2),encoding='utf-8')
subprocess.run(['docker','buildx','build','--platform','linux/amd64','-f',str(source/'deploy/docker/web.Dockerfile'),'-t','localhost:5001/argus/argus-web:local-20261112-b10b-node-ca-v1','--metadata-file',str(out/'web-image.json'),'--push',str(source)],check=True)
