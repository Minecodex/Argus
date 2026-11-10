"""Explicit preparation of metrics endpoints on the owned lab node only."""
import copy
import hashlib
import json
import subprocess
import time

import yaml

from run import execute

COMPONENTS = ('kube-scheduler', 'kube-controller-manager', 'etcd')
REMOTE = '/etc/kubernetes/argus-plan9-b09b-metrics'


def digest(value):
    # Windows-created review files may use CRLF; YAML identity compares LF bytes.
    return hashlib.sha256(value.replace('\r\n', '\n').encode()).hexdigest()


def updated_manifest(original, name):
    """Keep authentication and all unrelated fields; change only lab listeners/TLS."""
    value = yaml.safe_load(original)
    container = value['spec']['containers'][0]
    assert container['name'] == name
    command = container['command']
    old = '--listen-metrics-urls=http://127.0.0.1:2381' if name == 'etcd' else '--bind-address=127.0.0.1'
    new = '--listen-metrics-urls=http://0.0.0.0:2381' if name == 'etcd' else '--bind-address=0.0.0.0'
    assert command.count(old) == 1, 'Unexpected original metrics listener'
    command[command.index(old)] = new
    if name != 'etcd':
        assert not any(c.startswith(('--tls-cert-file=', '--tls-private-key-file=')) for c in command)
        command.extend(['--tls-cert-file=/argus-plan9-b09b-metrics/' + name + '.crt',
                        '--tls-private-key-file=/argus-plan9-b09b-metrics/' + name + '.key'])
        container.setdefault('volumeMounts', []).append({'mountPath': '/argus-plan9-b09b-metrics',
                                                        'name': 'argus-plan9-b09b-metrics', 'readOnly': True})
        value['spec'].setdefault('volumes', []).append({'name': 'argus-plan9-b09b-metrics',
            'hostPath': {'path': REMOTE, 'type': 'Directory'}})
    return yaml.safe_dump(value, sort_keys=False)


def assert_owner(run):
    node = json.loads(execute(['docker', 'inspect', run.node]))[0]
    saved = json.loads((run.infra.out / 'cluster-infrastructure.json').read_text(encoding='utf-8'))
    assert node['Id'] == saved['container_id'], 'Dedicated node identity changed'
    assert node['Config']['Labels']['io.x-k8s.kind.cluster'] == run.infra.id
    return node['Id']


def prepare(run, restore=False):
    ident = assert_owner(run)
    plans = []
    for name in COMPONENTS:
        path = '/etc/kubernetes/manifests/' + name + '.yaml'
        current = execute(['docker', 'exec', ident, 'cat', path]) + '\n'
        baseline = run.out / (name + '.original.yaml')
        if not baseline.exists():
            assert not restore, 'No original manifest recorded'
            baseline.write_bytes(current.replace('\r\n', '\n').encode())
        original = baseline.read_text(encoding='utf-8')
        updated = updated_manifest(original, name)
        assert digest(current) in (digest(original), digest(updated)), 'Component changed outside this run'
        applied = run.out / (name + '.applied.yaml')
        applied.write_bytes(updated.encode())
        plans.append({'name': name, 'path': path, 'original_sha256': digest(original),
                      'applied_sha256': digest(updated), 'current_sha256': digest(current)})
    run.save('endpoint-plan', {'node_id': ident, 'hash_format': 'sha256_normalized_lf',
                              'plans': plans, 'restore': restore, 'status': 'prepared'})
    if not restore:
        execute(['docker', 'exec', ident, 'mkdir', '-p', REMOTE])
        for name in COMPONENTS[:2]:
            # No private key crosses the node boundary. Public certs use its existing cluster CA.
            base = REMOTE + '/' + name
            exists = execute(['docker', 'exec', ident, 'sh', '-c', 'test ! -f "$1" || printf present', 'check', base + '.crt'])
            if not exists:
                extension = 'subjectAltName=DNS:' + name + '\nextendedKeyUsage=serverAuth\nkeyUsage=digitalSignature,keyEncipherment\nbasicConstraints=critical,CA:FALSE\n'
                execute(['docker', 'exec', '-i', ident, 'sh', '-c', 'cat > "$1"', 'write', base + '.ext'], data=extension)
                execute(['docker', 'exec', ident, 'openssl', 'req', '-new', '-newkey', 'rsa:2048', '-nodes',
                         '-keyout', base + '.key', '-out', base + '.csr', '-subj', '/CN=' + name])
                execute(['docker', 'exec', ident, 'openssl', 'x509', '-req', '-in', base + '.csr',
                         '-CA', '/etc/kubernetes/pki/ca.crt', '-CAkey', '/etc/kubernetes/pki/ca.key',
                         '-CAcreateserial', '-out', base + '.crt', '-days', '7', '-extfile', base + '.ext'])
                execute(['docker', 'exec', ident, 'chmod', '600', base + '.key'])
    assert_owner(run)
    for plan in plans:
        source = run.out / (plan['name'] + ('.original.yaml' if restore else '.applied.yaml'))
        desired_hash = plan['original_sha256'] if restore else plan['applied_sha256']
        if plan['current_sha256'] == desired_hash:
            continue
        staged = REMOTE + '/' + plan['name'] + '.yaml'
        execute(['docker', 'cp', str(source), ident + ':' + staged])
        execute(['docker', 'exec', ident, 'mv', staged, plan['path']])
    deadline = time.monotonic() + 300
    while time.monotonic() < deadline:
        try:
            pods = json.loads(run.infra.kubectl('get', 'pods', '-n', 'kube-system',
                                               '--request-timeout=5s', '-o', 'json', timeout=10))['items']
            ready = True
            for name in COMPONENTS:
                pod = next(p for p in pods if p['metadata']['name'] == name + '-' + run.node)
                command = pod['spec']['containers'][0]['command']
                expected = '--listen-metrics-urls=http://' + ('127.0.0.1' if restore else '0.0.0.0') + ':2381' if name == 'etcd' else '--bind-address=' + ('127.0.0.1' if restore else '0.0.0.0')
                ready &= expected in command and any(c['type'] == 'Ready' and c['status'] == 'True' for c in pod.get('status', {}).get('conditions', []))
            if ready:
                break
        except (RuntimeError, StopIteration, subprocess.TimeoutExpired):
            pass
        time.sleep(2)
    else:
        raise RuntimeError('Owned components did not become ready; original manifests preserved')
    run.save('endpoint-preparation', {'node_id': ident, 'plans': plans, 'status': 'restored' if restore else 'ready'})
    if not restore:
        proxy_rbac(run)
    run.event('component_endpoints_restored' if restore else 'component_endpoints_ready')
    print('Owned component metrics listeners and TLS: ' + ('restored' if restore else 'ready'), flush=True)


def proxy_rbac(run):
    labels = {'argus.plan9/run': run.id}
    role = {'apiVersion': 'rbac.authorization.k8s.io/v1', 'kind': 'Role',
            'metadata': {'name': 'plan9-b09b-component-proxy', 'namespace': 'kube-system', 'labels': labels},
            'rules': [{'apiGroups': [''], 'resources': ['pods/proxy'], 'verbs': ['get']}]}
    binding = {'apiVersion': 'rbac.authorization.k8s.io/v1', 'kind': 'RoleBinding',
            'metadata': copy.deepcopy(role['metadata']),
            'roleRef': {'apiGroup': 'rbac.authorization.k8s.io', 'kind': 'Role', 'name': role['metadata']['name']},
            'subjects': [{'kind': 'ServiceAccount', 'name': 'argus-otelcol', 'namespace': 'argus-telemetry'}]}
    actual = run.infra.get('deployment', 'argus-otelcol-gateway', '-n', 'argus-telemetry')
    account = actual['spec']['template']['spec']['serviceAccountName']
    binding['subjects'][0]['name'] = account
    for value in (role, binding):
        existing = run.infra.kubectl('get', value['kind'], value['metadata']['name'], '-n', 'kube-system', '--ignore-not-found', '-o', 'json')
        if existing:
            assert json.loads(existing)['metadata']['labels']['argus.plan9/run'] == run.id
    run.infra.apply([role, binding])
    evidence = run.infra.get('role,rolebinding', '-n', 'kube-system', '-l', 'argus.plan9/run=' + run.id)
    run.save('component-proxy-rbac', evidence)
