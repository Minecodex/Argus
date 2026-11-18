"""Publish the verified template fix, retaining formal data and every other image."""
import hashlib
import json
from pathlib import Path
import os
import shutil
import subprocess
import sys
import requests
import yaml
from manage import Run, ROOT, execute, now, write_json

TAG = os.environ.get('ARGUS_PLAN9_RELEASE_TAG', 'local-20261117-b13a-template-publish-v1')


def load_aliases(tag, names):
    cli = ['kubectl', '--context', 'docker-desktop', '-n', 'kube-system']
    rows = json.loads(execute([*cli, 'get', 'pods', '-l', 'app.kubernetes.io/name=argus-image-loader-argus', '-o', 'json']))['items']
    loader = next(x for x in rows if x['status']['phase'] == 'Running' and not x['metadata'].get('deletionTimestamp'))
    assert loader['metadata']['labels']['argus.io/release-id'] == 'argus'
    registry = requests.Session()
    registry.trust_env = False
    for name in names:
        commands = [*cli, 'exec', loader['metadata']['name'], '-c', 'loader', '--', '/host/ctr', '--address', '/run/containerd/containerd.sock', '--namespace', 'k8s.io', 'images']
        ref = 'host.docker.internal:5001/argus/' + name + ':' + tag
        execute([*commands, 'pull', '--plain-http', '--platform', 'linux/amd64', ref], timeout=90)
        manifest = registry.get('http://127.0.0.1:5001/v2/argus/' + name + '/manifests/' + tag,
                                headers={'Accept': 'application/vnd.oci.image.index.v1+json,application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.v2+json'}, timeout=30)
        manifest.raise_for_status()
        digest = 'sha256:' + hashlib.sha256(manifest.content).hexdigest()
        assert digest == manifest.headers['Docker-Content-Digest']
        execute([*commands, 'tag', '--force', ref, ref + '@' + digest])
        execute([*commands, 'tag', '--force', ref, ref.rsplit(':', 1)[0] + '@' + digest])


def deploy(web_publisher=None):
    run = Run()
    before_path = run.out / 'template-release-before.json'
    before = json.loads(before_path.read_text(encoding='utf-8')) if before_path.exists() else run.platform()
    if not before_path.exists():
        run.save('template-release-before', before)
    cfg_path = ROOT / 'deploy/.cache/argus-local-formal.yaml'
    cfg = yaml.safe_load(cfg_path.read_text(encoding='utf-8'))
    previous_tag = cfg['spec']['images']['tag']
    shutil.copy2(cfg_path, run.private / 'prior-install-config.yaml')
    # Existing release helper freezes source, builds Server/Worker, pushes,
    # loads the exact digest into containerd and checks both running images.
    sys.path.append(str(ROOT / 'docs/plan9/runtime/B10c'))
    from B10c.publish_backend import execute_stage
    os.environ['ARGUS_PLAN9_BACKEND_TAG'] = TAG
    os.environ['ARGUS_PLAN9_GOCACHE'] = 'D:/gocache/go-build'
    execute_stage(run, 'publish')
    image = run.platform()['deployments']['argus-server']['images'][0]
    digest = image.split('@')[1]
    web_image = None
    if web_publisher:
        os.environ['ARGUS_PLAN9_WEB_TAG'] = TAG
        web_publisher(run, TAG)
        web_image = run.platform()['deployments']['argus-web']['images'][0]
    session = requests.Session()
    session.trust_env = False
    aliases = []
    accept = 'application/vnd.oci.image.index.v1+json,application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.v2+json'
    for name in ['argus-web', 'minio', 'argus-workspace', 'argus-workspace-egress', 'argus-remote-runtime', 'argus-install-tools']:
        if web_publisher and name == 'argus-web':
            continue
        base = 'http://127.0.0.1:5001/v2/argus/' + name + '/manifests/'
        response = session.get(base + previous_tag, headers={'Accept': accept}, timeout=30)
        response.raise_for_status()
        content_digest = 'sha256:' + hashlib.sha256(response.content).hexdigest()
        result = session.put(base + TAG, data=response.content, headers={'Content-Type': response.headers['Content-Type']}, timeout=30)
        result.raise_for_status()
        assert result.headers['Docker-Content-Digest'] == content_digest
        aliases.append({'name': name, 'digest': content_digest})
    load_aliases(TAG, ['argus-backend', *(['argus-web'] if web_publisher else []), *[x['name'] for x in aliases]])
    cli = ['kubectl', '--context', 'docker-desktop', '-n', 'argus-system']
    migration = json.loads(execute([*cli, 'get', 'job', 'argus-postgresql-migration', '-o', 'json']))
    assert migration['status']['succeeded'] == 1 and migration['metadata']['annotations']['meta.helm.sh/release-name'] == 'argus-platform'
    write_json(run.private / 'completed-migration-before-recreate.json', migration)
    execute([*cli, 'delete', 'job', 'argus-postgresql-migration', '--wait=true'])
    image_settings = ['--set-string', 'images.backend=' + image]
    if web_image:
        image_settings += ['--set-string', 'images.web=' + web_image]
    with (run.out / 'template-rollout.log').open('wb') as log:
        subprocess.run(['helm', 'upgrade', 'argus-platform', 'deploy/helm/argus-platform', '--kube-context', 'docker-desktop',
                        '--namespace', 'argus-system', '--reuse-values', *image_settings,
                        '--wait', '--timeout', '180s'], cwd=ROOT, stdout=log, stderr=subprocess.STDOUT, check=True, timeout=210)
    after = run.platform()
    assert before['pvc'] == after['pvc']
    for name, prior in before['deployments'].items():
        assert prior['uid'] == after['deployments'][name]['uid']
        current_images = after['deployments'][name]['images']
        if name == 'argus-web' and web_image:
            assert current_images == [web_image]
            continue
        assert all(old == new or ('/argus-backend:' in old and new == image)
                   for old, new in zip(prior['images'], current_images)), name
    cfg['spec']['images']['tag'] = TAG
    cfg_path.write_text(yaml.safe_dump(cfg, sort_keys=False), encoding='utf-8')
    run.save('template-release', {'at': now(), 'backend_image': image, 'web_image': web_image, 'digest': digest, 'other_image_aliases': aliases,
                                 'formal_pvcs_and_deployment_ids_preserved': True, 'schema_migration_added': False,
                                 'helm_values_synchronized': True})
    print('Exact release images deployed with formal data preserved', flush=True)


if __name__ == '__main__':
    deploy()
