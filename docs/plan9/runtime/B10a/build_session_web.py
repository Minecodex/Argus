"""Overlay only the seven session UI files onto the currently deployed Web source."""
import hashlib,json,shutil,subprocess
from run import Run,ROOT
r=Run();out=r.out/'session-repair';out.mkdir(exist_ok=True)
before=r.platform();r.save('session-repair-platform-before',before)
assert before['deployments']['argus-web']['images'][0]==json.loads((out/'release-v2.json').read_text(encoding='utf-8'))['image']
base=ROOT/'artifacts/chart-tooltip-20261110/source';source=out/'web-source';source.mkdir(exist_ok=True)
ignore=shutil.ignore_patterns('node_modules','dist','*.tsbuildinfo','test-results')
for name in ('web','scripts','deploy','internal/telemetry/queryengine/skywalking/schema'):
 shutil.copytree(base/name,source/name,dirs_exist_ok=True,ignore=ignore)
for name in ('package.json','pnpm-lock.yaml','pnpm-workspace.yaml','tsconfig.base.json','.dockerignore'):
 if (base/name).exists():shutil.copy2(base/name,source/name)
files=['web/apps/enterprise/src/components/kubernetes/log-session-dialog.tsx','web/apps/enterprise/src/components/kubernetes/use-pod-log-session.ts','web/apps/enterprise/src/components/kubernetes/stream-session-dialog.tsx','web/apps/enterprise/src/i18n/kubernetes-log-session.ts','web/apps/enterprise/src/i18n/kubernetes-stream.ts','web/apps/enterprise/src/styles/kubernetes-workbench.css','web/packages/ui/src/log-viewer.tsx']
for name in files:shutil.copy2(ROOT/name,source/name)
(out/'source.json').write_text(json.dumps({'baseline_web_image':before['deployments']['argus-web']['images'][0],'base_source':'artifacts/chart-tooltip-20261110/source','overlays':{name:hashlib.sha256((ROOT/name).read_bytes()).hexdigest() for name in files}},indent=2),encoding='utf-8')
subprocess.run(['docker','buildx','build','--platform','linux/amd64','-f',str(source/'deploy/docker/web.Dockerfile'),'-t','localhost:5001/argus/argus-web:local-20261111-b10a-sessions-v3','--metadata-file',str(out/'web-image.json'),'--push',str(source)],check=True)
print('Frozen Web baseline built with session UI overlays only',flush=True)
