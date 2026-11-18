"""Owned direct TLS ingress exposure; name resolution changes only on lab hosts."""
import json
import time
from manage import Run, execute


def service(namespace, name):
    return json.loads(execute(['kubectl', '--context', 'docker-desktop', '-n', namespace, 'get', 'service', name, '-o', 'json']))


def configure(run):
    name = 'plan9-b13a-telemetry'
    mapping = json.loads(run.resources.read_text(encoding='utf-8'))
    original = service('argus-system', 'argus-telemetry-ingest')
    current = execute(['kubectl', '--context', 'docker-desktop', '-n', 'argus-system', 'get', 'service', name, '--ignore-not-found', '-o', 'json'])
    if current:
        assert json.loads(current)['metadata']['labels']['argus.plan9/run'] == run.id
    desired = {'apiVersion': 'v1', 'kind': 'Service', 'metadata': {'name': name, 'namespace': 'argus-system', 'labels': {'argus.plan9/run': run.id}},
               'spec': {'type': 'LoadBalancer', 'selector': original['spec']['selector'], 'ports': [{'name': 'grpc', 'port': 4317, 'targetPort': 4317}, {'name': 'https', 'port': 4318, 'targetPort': 4318}]}}
    execute(['kubectl', '--context', 'docker-desktop', 'apply', '-f', '-'], data=json.dumps(desired))
    for _ in range(40):
        value = service('argus-system', name)
        addresses = value.get('status', {}).get('loadBalancer', {}).get('ingress', [])
        if addresses and addresses[0].get('ip'):
            break
        time.sleep(1)
    else:
        raise RuntimeError('Owned telemetry ingress unavailable')
    entries = {service('argus-ingress', 'argus-ingress')['status']['loadBalancer']['ingress'][0]['ip']: ['argus.dev', 'artifacts.argus.dev'],
               service('argus-system', 'argus-connector-gateway-public')['status']['loadBalancer']['ingress'][0]['ip']: ['connector.argus.dev'],
               addresses[0]['ip']: ['argus-telemetry-ingest.argus-system.svc', 'argus-telemetry-ingest.argus-system.svc.cluster.local']}
    script = "python3 - <<'PY'\nfrom pathlib import Path\nentries=" + repr(entries) + "\np=Path('/etc/hosts');names={n for values in entries.values() for n in values}\nlines=[line for line in p.read_text().splitlines() if not names.intersection(line.split()[1:])]\nlines.extend(address+' '+' '.join(values) for address,values in entries.items())\np.write_text('\\n'.join(lines)+'\\n')\nPY\n"
    for key in ['orders', 'reports']:
        run.shell(key, script)
    mapping['telemetry_network_endpoint'] = {'name': name, 'namespace': 'argus-system', 'uid': value['metadata']['uid'], 'ip': addresses[0]['ip']}
    run.save('resource-map', mapping)
    run.save('network-endpoints', {'entries': entries, 'end_to_end_tls': True, 'formal_services_unchanged': True,
                                 'connector_control': 'normal executor_tunnel; telemetry direct'})
    print('Native TLS ingress mapped only on the two owned hosts', flush=True)


if __name__ == '__main__':
    configure(Run())
