import json
from run import Run,ROOT,execute
run=Run();OUT=run.out/'terminal-options-audit'
before=run.platform()
run.save('terminal-options-platform-before-web',before)
expected=json.loads((run.out/'quiet-pod-repair/release.json').read_text(encoding='utf-8'))['image']
assert before['deployments']['argus-web']['images'][0]==expected

digest = json.loads((OUT / 'web-image.json').read_text(encoding='utf-8'))['containerimage.digest']
image = 'host.docker.internal:5001/argus/argus-web:local-20261111-b10a-log-options-v1@' + digest
loaders = json.loads(execute(['kubectl', '--context', 'docker-desktop', '-n', 'kube-system', 'get', 'pods',
    '-l', 'app.kubernetes.io/name=argus-image-loader-argus', '-o', 'json']))['items']
loaders = [p for p in loaders if not p['metadata'].get('deletionTimestamp') and p['status']['phase'] == 'Running']
assert loaders and all(p['metadata']['labels']['argus.io/release-id'] == 'argus' for p in loaders)
for pod in loaders:
    command = ['kubectl', '--context', 'docker-desktop', '-n', 'kube-system', 'exec', pod['metadata']['name'],
        '-c', 'loader', '--', '/host/ctr', '--address', '/run/containerd/containerd.sock', '--namespace', 'k8s.io', 'images']
    (OUT / 'image-load.log').write_text(execute(command + ['pull', '--plain-http', image], timeout=120), encoding='utf-8')
    execute(command + ['tag', '--force', image, 'host.docker.internal:5001/argus/argus-web@' + digest])
base = ['kubectl', '--context', 'docker-desktop', '-n', 'argus-system']
current = json.loads(execute(base + ['get', 'deployment', 'argus-web', '-o', 'json']))
old = current['spec']['template']['spec']['containers'][0]['image']
assert current['metadata']['uid'] == before['deployments']['argus-web']['uid']
assert old in (before['deployments']['argus-web']['images'][0], image)
if old != image:
    patch = [{'op': 'test', 'path': '/metadata/uid', 'value': current['metadata']['uid']},
        {'op': 'test', 'path': '/metadata/resourceVersion', 'value': current['metadata']['resourceVersion']},
        {'op': 'test', 'path': '/spec/template/spec/containers/0/image', 'value': old},
        {'op': 'replace', 'path': '/spec/template/spec/containers/0/image', 'value': image}]
    path = OUT / 'web-patch.json'
    path.write_text(json.dumps(patch), encoding='utf-8')
    execute(base + ['patch', 'deployment', 'argus-web', '--type=json', '--patch-file', str(path)])
execute(base + ['rollout', 'status', 'deployment/argus-web', '--timeout=120s'], timeout=130)
pods = json.loads(execute(base + ['get', 'pods', '-l', 'app.kubernetes.io/name=argus-web', '-o', 'json']))['items']
actual = []
for pod in pods:
    if pod['metadata'].get('deletionTimestamp'):
        continue
    status = pod['status']['containerStatuses'][0]
    assert status['ready'] and pod['spec']['containers'][0]['image'] == image
    assert status['imageID'].endswith(digest)
    actual.append({'pod': pod['metadata']['name'], 'uid': pod['metadata']['uid'], 'image_id': status['imageID']})
assert actual
after = run.platform()
assert after['pvc'] == before['pvc']
for name, row in before['deployments'].items():
    if name != 'argus-web':
        assert after['deployments'][name] == row, name
(OUT / 'platform-after.json').write_text(json.dumps(after, indent=2), encoding='utf-8')
(OUT / 'release.json').write_text(json.dumps({'image': image, 'previous': old, 'pods': actual,
    'only_web_changed': True, 'preserved_pvcs': len(after['pvc'])}, indent=2), encoding='utf-8')
print('Real API Web rollout and exact imageID verified; all other deployments and six PVCs preserved')
