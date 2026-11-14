"""Publish current Web source with exact-image and persistent-identity checks."""
import hashlib
import json
import os
import re
from pathlib import Path
import shutil
import subprocess
import time

from infrastructure import ROOT, execute


def execute_stage(run, _action):
    tag = os.environ.get('ARGUS_PLAN9_WEB_TAG','local-20261113-b10c-v1')
    assert re.fullmatch(r'[a-z0-9][a-z0-9_.-]{0,127}',tag)
    out = run.out/('web-main' if tag=='local-20261113-b10c-v1' else 'web-'+tag)
    out.mkdir(exist_ok=True)
    source = out/'source'
    if not (out/'image.json').exists():
        assert not source.exists(), 'Inspect the previous build before retrying with a new build directory'
        source.mkdir()
        ignore = shutil.ignore_patterns('node_modules','dist','test-results','playwright-report','*.tsbuildinfo','__pycache__')
        for name in ('web','scripts','deploy/docker'):
            shutil.copytree(ROOT/name,source/name,ignore=ignore)
        for name in ('package.json','pnpm-lock.yaml','pnpm-workspace.yaml','tsconfig.base.json','.dockerignore'):
            shutil.copy2(ROOT/name,source/name)
        schema = Path('internal/telemetry/queryengine/skywalking/schema/trace.graphql')
        (source/schema).parent.mkdir(parents=True,exist_ok=True)
        shutil.copy2(ROOT/schema,source/schema)
        hashes = {str(p.relative_to(source)).replace('\\','/'):hashlib.sha256(p.read_bytes()).hexdigest()
                  for p in source.rglob('*') if p.is_file()}
        (out/'source.json').write_text(json.dumps({'head':execute(['git','rev-parse','HEAD']),'files':hashes},indent=2),encoding='utf-8')
        with (out/'build.log').open('wb') as log:
            subprocess.run(['docker','buildx','build','--platform','linux/amd64','-f',str(source/'deploy/docker/web.Dockerfile'),
                            '-t','localhost:5001/argus/argus-web:'+tag,'--metadata-file',str(out/'image.json'),'--push',str(source)],
                           stdout=log,stderr=subprocess.STDOUT,check=True,timeout=900)
    manifest = json.loads((out/'source.json').read_text(encoding='utf-8'))['files']
    assert all(hashlib.sha256((ROOT/name).read_bytes()).hexdigest()==digest for name,digest in manifest.items())
    digest = json.loads((out/'image.json').read_text(encoding='utf-8'))['containerimage.digest']
    ref = 'host.docker.internal:5001/argus/argus-web:'+tag+'@'+digest
    before = run.platform()
    run.save('web-platform-before',before)
    loaders = json.loads(execute(['kubectl','--context','docker-desktop','-n','kube-system','get','pods',
                                  '-l','app.kubernetes.io/name=argus-image-loader-argus','-o','json']))['items']
    assert loaders
    for pod in loaders:
        if pod['metadata'].get('deletionTimestamp'):
            continue
        assert pod['metadata']['labels']['argus.io/release-id']=='argus'
        cmd = ['kubectl','--context','docker-desktop','-n','kube-system','exec',pod['metadata']['name'],'-c','loader',
               '--','/host/ctr','--address','/run/containerd/containerd.sock','--namespace','k8s.io','images']
        execute([*cmd,'pull','--plain-http',ref],timeout=180)
        execute([*cmd,'tag','--force',ref,'host.docker.internal:5001/argus/argus-web@'+digest])
    base = ['kubectl','--context','docker-desktop','-n','argus-system']
    deployment = json.loads(execute([*base,'get','deployment','argus-web','-o','json']))
    old = deployment['spec']['template']['spec']['containers'][0]['image']
    patch = [{'op':'test','path':'/metadata/uid','value':deployment['metadata']['uid']},
             {'op':'test','path':'/metadata/resourceVersion','value':deployment['metadata']['resourceVersion']},
             {'op':'test','path':'/spec/template/spec/containers/0/image','value':old},
             {'op':'replace','path':'/spec/template/spec/containers/0/image','value':ref}]
    path = out/'release-patch.json'
    path.write_text(json.dumps(patch),encoding='utf-8')
    execute([*base,'patch','deployment','argus-web','--type=json','--patch-file',str(path)])
    execute([*base,'rollout','status','deployment/argus-web','--timeout=180s'],timeout=190)
    for _ in range(30):
        pods = [p for p in json.loads(execute([*base,'get','pods','-l','app.kubernetes.io/name=argus-web','-o','json']))['items']
                if not p['metadata'].get('deletionTimestamp')]
        if pods and all(all(s['ready'] and s['imageID'].endswith(digest) for s in p['status'].get('containerStatuses',[])) for p in pods):
            break
        time.sleep(2)
    else:
        raise RuntimeError('Exact new Web image not observed')
    after = run.platform()
    assert before['pvc']==after['pvc']
    for name,value in before['deployments'].items():
        if name!='argus-web':
            assert after['deployments'][name]==value,name
    run.save('web-release',{'image':ref,'previous':old,'six_formal_pvcs_preserved':True,
                            'other_deployments_preserved':True,'frozen_source_verified':True,
                            'build_directory':out.relative_to(run.out).as_posix()})
    print('Current Web source published; exact image ready and formal persistent identities preserved',flush=True)
