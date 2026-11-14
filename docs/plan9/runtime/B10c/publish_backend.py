"""Patch Server/Worker from frozen source; no protocol or schema migration."""
from datetime import datetime, timezone
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
    tag = os.environ.get('ARGUS_PLAN9_BACKEND_TAG','local-20261113-b10c-custom-preview-v1')
    assert re.fullmatch(r'[a-z0-9][a-z0-9_.-]{0,127}',tag)
    out = run.out/('backend-custom-preview-v1' if tag=='local-20261113-b10c-custom-preview-v1' else 'backend-'+tag)
    out.mkdir(exist_ok=True)
    source, context = out/'source', out/'context'
    before = run.platform()
    base_image = before['deployments']['argus-server']['images'][0]
    if not (out/'image.json').exists():
        assert not source.exists(), 'Inspect a partial build before changing its frozen directory'
        source.mkdir()
        context.mkdir()
        for name in ('cmd','internal','api','migrations'):
            shutil.copytree(ROOT/name,source/name)
        for name in ('go.mod','go.sum'):
            shutil.copy2(ROOT/name,source/name)
        files = {str(p.relative_to(source)).replace('\\','/'):hashlib.sha256(p.read_bytes()).hexdigest()
                 for p in source.rglob('*') if p.is_file()}
        (out/'source.json').write_text(json.dumps({'head':execute(['git','rev-parse','HEAD']),'files':files},indent=2),encoding='utf-8')
        env = dict(os.environ)
        env.update(CGO_ENABLED='0',GOOS='linux',GOARCH='amd64',
                   GOCACHE=os.environ.get('ARGUS_PLAN9_GOCACHE',str(run.out/'go-cache')),GOTMPDIR=str(run.out/'go-tmp'))
        Path(env['GOTMPDIR']).mkdir(exist_ok=True)
        flags = '-s -w -X github.com/kakj-go/Argus/internal/buildinfo.Version='+tag+' -X github.com/kakj-go/Argus/internal/buildinfo.Commit='+execute(['git','rev-parse','--short','HEAD'])+'-b10c-worktree -X github.com/kakj-go/Argus/internal/buildinfo.Date='+datetime.now(timezone.utc).isoformat()
        with (out/'compile.log').open('wb') as log:
            for binary in ('argus-server','argus-worker'):
                subprocess.run(['go','build','-trimpath','-ldflags='+flags,'-o',str(context/binary),'./cmd/'+binary],
                               cwd=source,env=env,stdout=log,stderr=subprocess.STDOUT,check=True,timeout=900)
        (context/'Dockerfile').write_text('FROM '+base_image.replace('host.docker.internal:5001','localhost:5001')+'\nCOPY argus-server argus-worker /usr/local/bin/\n',encoding='utf-8')
        with (out/'build.log').open('wb') as log:
            subprocess.run(['docker','buildx','build','--platform','linux/amd64','-t','localhost:5001/argus/argus-backend:'+tag,
                            '--metadata-file',str(out/'image.json'),'--push',str(context)],stdout=log,stderr=subprocess.STDOUT,check=True,timeout=600)
    files = json.loads((out/'source.json').read_text(encoding='utf-8'))['files']
    assert all(hashlib.sha256((ROOT/name).read_bytes()).hexdigest()==digest for name,digest in files.items())
    digest = json.loads((out/'image.json').read_text(encoding='utf-8'))['containerimage.digest']
    ref = 'host.docker.internal:5001/argus/argus-backend:'+tag+'@'+digest
    loaders = json.loads(execute(['kubectl','--context','docker-desktop','-n','kube-system','get','pods',
                                  '-l','app.kubernetes.io/name=argus-image-loader-argus','-o','json']))['items']
    for pod in loaders:
        if pod['metadata'].get('deletionTimestamp'):
            continue
        assert pod['metadata']['labels']['argus.io/release-id']=='argus'
        cmd = ['kubectl','--context','docker-desktop','-n','kube-system','exec',pod['metadata']['name'],'-c','loader','--',
               '/host/ctr','--address','/run/containerd/containerd.sock','--namespace','k8s.io','images']
        execute([*cmd,'pull','--plain-http',ref],timeout=180)
        execute([*cmd,'tag','--force',ref,'host.docker.internal:5001/argus/argus-backend@'+digest])
    base = ['kubectl','--context','docker-desktop','-n','argus-system']
    proof = {}
    for name in ('argus-server','argus-worker'):
        item = json.loads(execute([*base,'get','deployment',name,'-o','json']))
        old = item['spec']['template']['spec']['containers'][0]['image']
        assert item['metadata']['uid']==before['deployments'][name]['uid']
        patch = [{'op':'test','path':'/metadata/uid','value':item['metadata']['uid']},
                 {'op':'test','path':'/metadata/resourceVersion','value':item['metadata']['resourceVersion']},
                 {'op':'test','path':'/spec/template/spec/containers/0/image','value':old},
                 {'op':'replace','path':'/spec/template/spec/containers/0/image','value':ref}]
        file = out/(name+'-patch.json')
        file.write_text(json.dumps(patch),encoding='utf-8')
        execute([*base,'patch','deployment',name,'--type=json','--patch-file',str(file)])
        execute([*base,'rollout','status','deployment/'+name,'--timeout=180s'],timeout=190)
        for _ in range(30):
            pods = [p for p in json.loads(execute([*base,'get','pods','-l','app.kubernetes.io/name='+name,'-o','json']))['items']
                    if not p['metadata'].get('deletionTimestamp')]
            if pods and all(p['status'].get('containerStatuses') and all(s['ready'] and s['imageID'].endswith(digest) for s in p['status']['containerStatuses']) for p in pods):
                break
            time.sleep(2)
        else:
            raise RuntimeError('Exact new backend image not observed')
        proof[name] = {'image':ref,'previous':old,'pod_ids':[p['metadata']['uid'] for p in pods]}
    after = run.platform()
    assert before['pvc']==after['pvc']
    for name,item in before['deployments'].items():
        if name not in proof:
            assert after['deployments'][name]==item,name
    run.save('backend-custom-preview-release',{'components':proof,'six_formal_pvcs_preserved':True,
                                               'other_deployments_preserved':True,'schema_migration':False,
                                               'build_directory':out.relative_to(run.out).as_posix()})
    print('Custom preview transaction fix deployed to exact Server/Worker image',flush=True)
