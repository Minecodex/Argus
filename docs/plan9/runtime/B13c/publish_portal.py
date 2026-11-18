"""Build only the changed enterprise portal and retain the pinned Web base."""
import hashlib
import json
import os
import subprocess
from manage import ROOT, execute, write_json
from B13a.release import load_aliases


def fingerprints():
    files = execute(['git', 'ls-files', '-co', '--exclude-standard', '--', 'web', 'pnpm-lock.yaml', 'pnpm-workspace.yaml',
                     'package.json', 'tsconfig.base.json', 'internal/telemetry/queryengine/skywalking/schema/trace.graphql']).splitlines()
    return {name: hashlib.sha256((ROOT / name).read_bytes()).hexdigest() for name in files if (ROOT / name).is_file()}


def publish(run, tag):
    root = run.out / ('enterprise-static-' + tag)
    root.mkdir(exist_ok=True)
    before = run.platform()
    live = before['deployments']['argus-web']['images'][0]
    source = root / 'source.json'
    previous = json.loads(source.read_text(encoding='utf-8'))['base'] if source.exists() else live
    assert previous.startswith('host.docker.internal:5001/argus/argus-web:') and '@sha256:' in previous
    metadata = root / 'image.json'
    if not metadata.exists():
        manifests = fingerprints()
        dist = root / 'dist'
        env = dict(os.environ, VITE_API_MODE='real', VITE_API_BASE_URL='/', VITE_TEMPLATE_ORIGIN='', VITE_PLATFORM_URL='', VITE_DIRECT_EGRESS_ADDRESSES='')
        web = ROOT / 'web/apps/enterprise'
        vite = ROOT / 'node_modules/vite/bin/vite.js'
        with (root / 'build.log').open('wb') as log:
            subprocess.run(['node', str(vite), 'build', '--outDir', str(dist), '--emptyOutDir'], cwd=web, env=env,
                           stdout=log, stderr=subprocess.STDOUT, check=True, timeout=180)
        assert manifests == fingerprints(), 'Enterprise source changed during the frozen build'
        assert (dist / 'index.html').is_file() and (dist / 'assets').is_dir()
        write_json(source, {'files': manifests, 'base': previous, 'api_mode': 'real', 'runtime_origin_config_preserved': True})
        base = previous.replace('host.docker.internal:5001', 'localhost:5001')
        # Copy the new manifest and hashed assets into the unchanged base.
        # No privilege/user switch or runtime configuration change is needed.
        (root / 'Dockerfile').write_text('FROM ' + base + '\nCOPY dist/ /srv/enterprise/\n', encoding='utf-8')
        with (root / 'image-build.log').open('wb') as log:
            subprocess.run(['docker', 'buildx', 'build', '--platform', 'linux/amd64', '-t', 'localhost:5001/argus/argus-web:' + tag,
                            '--metadata-file', str(metadata), '--push', str(root)], cwd=ROOT,
                           stdout=log, stderr=subprocess.STDOUT, check=True, timeout=180)
    digest = json.loads(metadata.read_text(encoding='utf-8'))['containerimage.digest']
    load_aliases(tag, ['argus-web'])
    ref = 'host.docker.internal:5001/argus/argus-web:' + tag + '@' + digest
    assert live in [previous, ref], 'Live Web image changed outside this release'
    cli = ['kubectl', '--context', 'docker-desktop', '-n', 'argus-system']
    deployment = json.loads(execute([*cli, 'get', 'deployment', 'argus-web', '-o', 'json']))
    assert deployment['metadata']['uid'] == before['deployments']['argus-web']['uid']
    if live != ref:
        patch = [{'op': 'test', 'path': '/metadata/resourceVersion', 'value': deployment['metadata']['resourceVersion']},
                 {'op': 'test', 'path': '/spec/template/spec/containers/0/image', 'value': previous},
                 {'op': 'replace', 'path': '/spec/template/spec/containers/0/image', 'value': ref}]
        path = root / 'patch.json'
        write_json(path, patch)
        execute([*cli, 'patch', 'deployment', 'argus-web', '--type=json', '--patch-file', str(path)])
    execute([*cli, 'rollout', 'status', 'deployment/argus-web', '--timeout=120s'], timeout=130)
    pods = json.loads(execute([*cli, 'get', 'pods', '-l', 'app.kubernetes.io/name=argus-web', '-o', 'json']))['items']
    current = [x['status']['containerStatuses'][0] for x in pods if not x['metadata'].get('deletionTimestamp')]
    assert current and all(x['ready'] and x['imageID'].endswith(digest) for x in current)
    after = run.platform()
    assert before['pvc'] == after['pvc']
    assert all(before['deployments'][k] == after['deployments'][k] for k in before['deployments'] if k != 'argus-web')
    run.save('web-release', {'image': ref, 'previous': previous, 'six_formal_pvcs_preserved': True, 'frozen_source_verified': True,
                             'enterprise_assets_only': True, 'other_portal_assets_inherited_from_base': True})
    print('Exact enterprise portal release ready on the retained Web image', flush=True)
