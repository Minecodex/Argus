"""Rebuild the deployed frozen Web source with only the options-grid stylesheet changed."""
import hashlib,json,shutil,subprocess
from run import Run,ROOT
r=Run();out=r.out/'terminal-options-audit';out.mkdir(exist_ok=True)
before=r.platform();r.save('terminal-options-platform-before',before)
expected=json.loads((r.out/'quiet-pod-repair/release.json').read_text(encoding='utf-8'))['image']
assert before['deployments']['argus-web']['images'][0]==expected
source=out/'web-source';shutil.copytree(r.out/'quiet-pod-repair/web-source',source,dirs_exist_ok=True,ignore=shutil.ignore_patterns('node_modules','dist','*.tsbuildinfo','test-results'))
file='web/apps/enterprise/src/styles/kubernetes-workbench.css';shutil.copy2(ROOT/file,source/file)
(out/'source.json').write_text(json.dumps({'baseline_web_image':expected,'base_source':'quiet-pod-repair/web-source','overlays':{file:hashlib.sha256((ROOT/file).read_bytes()).hexdigest()}},indent=2),encoding='utf-8')
subprocess.run(['docker','buildx','build','--platform','linux/amd64','-f',str(source/'deploy/docker/web.Dockerfile'),'-t','localhost:5001/argus/argus-web:local-20261111-b10a-log-options-v1','--metadata-file',str(out/'web-image.json'),'--push',str(source)],check=True)
print('Frozen Web rebuilt with the options-grid CSS change only',flush=True)
