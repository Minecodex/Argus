"""Owned B09a/B09b lab: renew real Collector leaves across their 15m overlap.

Run after the normal Collector upgrade. The temporary early-renewal setting is
never a product default. Private keys stay on the lab node. --restore recovers
the original configuration if an earlier invocation was interrupted.
"""
import argparse
import base64
from datetime import datetime, timezone
import json
import ipaddress
from pathlib import Path
import re
import subprocess
import sys
import time
from urllib.parse import urlsplit

from cryptography import x509
import yaml

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[2]
sys.path.insert(0, str(HERE / 'B09b'))
from run import Run, execute
from environment import assert_owner
from verification import METRICS, query
from common.api import Api

NAMESPACE = 'argus-telemetry'


def now():
    return datetime.now(timezone.utc)


class Review:
    def __init__(self, output, session):
        self.run = Run()
        self.node = assert_owner(self.run)
        self.out = output.resolve()
        self.out.mkdir(parents=True, exist_ok=True)
        self.run.id = 'collector-identity-renewal-' + self.out.name
        self.run.out = self.out
        self.api = Api(self.run, session, self.run.ca)
        self.state_path = self.out / 'identity-test-state.json'

    def save(self, name, data):
        (self.out / (name + '.json')).write_text(json.dumps(data, indent=2), encoding='utf-8')

    def node_command(self, *args, data=None, timeout=30):
        return execute(['docker', 'exec', '-i', self.node, *args], data=data, timeout=timeout)

    def pods(self):
        return [p for p in self.run.infra.get('pods', '-n', NAMESPACE)['items']
                if p['metadata']['name'].startswith(('argus-otelcol-agent-', 'argus-otelcol-gateway-')) and not p['metadata'].get('deletionTimestamp')]

    def identities(self):
        rows = []
        for pod in self.pods():
            statuses = pod.get('status', {}).get('containerStatuses', [])
            if not statuses or not statuses[0].get('ready'):
                continue
            cid = statuses[0]['containerID'].split('://', 1)[1]
            runtime = json.loads(self.node_command('crictl', 'inspect', cid))
            path = next(m['source'] for m in runtime['info']['runtimeSpec']['mounts']
                        if m['destination'] == '/var/lib/argus-otelcol/identity')
            assert path.startswith(('/var/local-path-provisioner/', '/var/lib/kubelet/pods/')) and '..' not in path
            certificates = {}
            for name in ('client', 'server'):
                certificate = x509.load_pem_x509_certificate(self.node_command('cat', path + '/' + name + '.pem').encode())
                certificates[name] = {'serial': format(certificate.serial_number, 'x'),
                    'not_before': certificate.not_valid_before_utc.isoformat(),
                    'not_after': certificate.not_valid_after_utc.isoformat()}
            rows.append({'name': pod['metadata']['name'], 'uid': pod['metadata']['uid'],
                'role': 'gateway' if 'gateway' in pod['metadata']['name'] else 'agent',
                'path': path, 'pid': runtime['info']['pid'], 'ip': pod['status']['podIP'], 'certificates': certificates})
        return rows

    def exporter_metrics(self, gateway):
        raw = self.node_command('nsenter', '-t', str(gateway['pid']), '-n', 'curl', '--noproxy', '*',
            '--fail', '--silent', '--max-time', '5', 'http://127.0.0.1:8888/metrics')
        wanted = ('otelcol_exporter_send_failed_', 'otelcol_exporter_sent_', 'otelcol_exporter_enqueue_failed_',
                  'otelcol_exporter_queue_size', 'otelcol_exporter_in_flight_requests')
        result = {}
        for line in raw.splitlines():
            if line.startswith(wanted) and 'exporter="otlp/argus"' in line:
                key, value = line.rsplit(' ', 1)
                result[key] = float(value)
        assert any('otelcol_exporter_sent_metric_points' in key for key in result)
        return result

    def wait_exporter_settled(self, gateway, timeout=420):
        deadline = time.monotonic() + timeout
        empty = 0
        started = time.monotonic()
        while time.monotonic() < deadline:
            metrics = self.exporter_metrics(gateway)
            pending = sum(v for k, v in metrics.items() if 'queue_size' in k or 'in_flight_requests' in k)
            empty = empty + 1 if pending == 0 else 0
            if empty >= 3:
                return metrics, round(time.monotonic() - started, 2)
            time.sleep(5)
        raise RuntimeError('Persistent backlog or in-flight retries did not settle before continuity measurement')

    def snapshot(self):
        rows = self.run.builtins(self.api)
        statuses = {r['definition']['key']: {k: r['collection'][k] for k in
            ('health', 'freshness', 'last_sample_at', 'error_code')} for r in rows}
        return {'time': now().isoformat(), 'collections': statuses, 'identities': self.identities()}

    @staticmethod
    def healthy(snapshot):
        return len(snapshot['collections']) == 8 and all(r['health'] == 'healthy' and r['freshness'] == 'fresh'
            and not r['error_code'] for r in snapshot['collections'].values())

    def wait_healthy(self, after, timeout=150):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            snapshot = self.snapshot()
            if self.healthy(snapshot) and all(datetime.fromisoformat(r['last_sample_at'].replace('Z', '+00:00')) > after
                    for r in snapshot['collections'].values()):
                return snapshot
            time.sleep(3)
        raise RuntimeError('Eight real post-change sample streams did not become fresh')

    def patch_config(self, state, old, new):
        patch = [{'op': 'test', 'path': '/metadata/uid', 'value': state['config_uid']},
                 {'op': 'test', 'path': '/data/gateway.yaml', 'value': old},
                 {'op': 'replace', 'path': '/data/gateway.yaml', 'value': new}]
        self.run.infra.kubectl('patch', 'configmap', state['config_name'], '-n', NAMESPACE,
            '--type=json', '--patch-file=/dev/stdin', data=json.dumps(patch))

    def restart_gateway(self, state):
        deployment = self.run.infra.get('deployment', 'argus-otelcol-gateway', '-n', NAMESPACE)
        assert deployment['metadata']['uid'] == state['deployment_uid']
        assert deployment['spec']['replicas'] == 1 and deployment['spec']['strategy']['type'] == 'Recreate'
        pods = [p for p in self.pods() if 'gateway' in p['metadata']['name']]
        assert len(pods) == 1
        pod = pods[0]
        assert pod['spec']['containers'][0]['image'] == state['image']
        assert any(v.get('configMap', {}).get('name') == state['config_name'] for v in pod['spec']['volumes'])
        self.run.infra.kubectl('delete', 'pod', pod['metadata']['name'], '-n', NAMESPACE, '--wait=true', timeout=100)
        self.run.infra.kubectl('rollout', 'status', 'deployment/argus-otelcol-gateway', '-n', NAMESPACE,
            '--timeout=120s', timeout=130)

    def probe_identity(self, gateway, endpoint, certificate_path=None):
        parsed = urlsplit(endpoint)
        assert parsed.scheme == 'https' and parsed.hostname and not parsed.username
        # The owned lab's cross-cluster service mapping is in CoreDNS. The node's
        # host resolver can return a Windows proxy Fake-IP; connect to the same
        # explicit mapping as the Collector while keeping original SNI/verification.
        corefile = self.run.infra.get('configmap', 'coredns', '-n', 'kube-system')['data']['Corefile']
        address = None
        for line in corefile.splitlines():
            fields = line.split()
            if parsed.hostname in fields[1:]:
                try:
                    address = str(ipaddress.ip_address(fields[0]))
                    break
                except ValueError:
                    pass
        assert address, 'Endpoint must have an explicit owned-lab DNS mapping'
        path = gateway['path']
        state = json.loads(self.node_command('cat', path + '/trust-bundle.json'))
        body = json.dumps(state)
        request = ('POST ' + parsed.path + ' HTTP/1.1\r\nHost: ' + parsed.netloc + '\r\n'
            'Content-Type: application/json\r\nConnection: close\r\nContent-Length: ' + str(len(body.encode())) + '\r\n\r\n' + body)
        command = ['docker', 'exec', '-i', self.node, 'openssl', 's_client', '-quiet', '-verify_return_error',
            '-verify_hostname', parsed.hostname, '-servername', parsed.hostname, '-alpn', 'http/1.1',
            # Separate the verify store: OpenSSL must send only the client leaf,
            # matching Collector, rather than auto-appending the trusted root.
            '-connect', address + ':' + str(parsed.port or 443), '-verifyCAfile', path + '/ca.pem',
            '-no-CAfile', '-no-CApath', '-no-CAstore',
            '-cert', certificate_path or path + '/client.pem', '-key', path + '/client-key.pem']
        # Binary stdin preserves HTTP CRLF when orchestrating from Windows.
        response = subprocess.run(command, input=request.encode(), capture_output=True, timeout=20)
        stdout, stderr = response.stdout.decode(errors='replace'), response.stderr.decode(errors='replace')
        (self.out / 'identity-probe-last.log').write_text(stdout[:2048] + '\n' + stderr[-512:], encoding='utf-8')
        match = re.search(r'HTTP/1\.[01] (\d{3})', stdout)
        if not match:
            raise RuntimeError('Strict TLS identity probe returned no HTTP response: ' + stderr[-500:])
        return int(match[1])

    def probe_gateway_server(self, gateway, collector_id):
        path = gateway['path']
        name = 'collector-' + collector_id + '.argus.telemetry'
        response = subprocess.run(['docker', 'exec', '-i', self.node, 'openssl', 's_client', '-showcerts',
            '-verify_return_error', '-verify_hostname', name, '-servername', name,
            '-connect', gateway['ip'] + ':4317', '-verifyCAfile', path + '/ca.pem',
            '-no-CAfile', '-no-CApath', '-no-CAstore',
            '-cert', path + '/client.pem', '-key', path + '/client-key.pem'],
            input='', text=True, capture_output=True, timeout=15)
        match = re.search(r'-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----', response.stdout, re.S)
        if not match or 'Verification: OK' not in response.stdout:
            raise RuntimeError('Gateway did not present a valid server leaf')
        return format(x509.load_pem_x509_certificate(match[0].encode()).serial_number, 'x')

    def restore(self):
        state = json.loads(self.state_path.read_text(encoding='utf-8'))
        if state.get('restored'):
            return
        current = self.run.infra.get('configmap', state['config_name'], '-n', NAMESPACE)
        assert current['metadata']['uid'] == state['config_uid']
        actual = current['data']['gateway.yaml']
        assert actual in (state['original_yaml'], state['test_yaml']), 'Config changed outside this review'
        if actual != state['original_yaml']:
            self.patch_config(state, actual, state['original_yaml'])
        started = now()
        self.restart_gateway(state)
        snapshot = self.wait_healthy(started)
        state.update(restored=True, restored_at=now().isoformat())
        self.save('identity-test-state', state)
        self.save('identity-restored-samples', snapshot)
        print('Original 8h/5m renewal settings restored; eight new streams healthy', flush=True)

    def exercise(self, minutes):
        assert minutes >= 17, 'Must cross the actual 15 minute registry overlap'
        if self.state_path.exists():
            raise RuntimeError('Existing review state: inspect or restore it before starting another run')
        initial = self.snapshot()
        assert self.healthy(initial)
        gateway = next(p for p in initial['identities'] if p['role'] == 'gateway')
        pod = next(p for p in self.pods() if p['metadata']['uid'] == gateway['uid'])
        config_name = next(v['configMap']['name'] for v in pod['spec']['volumes'] if v['name'] == 'config')
        config = self.run.infra.get('configmap', config_name, '-n', NAMESPACE)
        original = config['data']['gateway.yaml']
        rendered = yaml.safe_load(original)
        agent = yaml.safe_load(config['data']['agent.yaml'])
        assert agent['extensions']['argus_identity']['follow_bootstrap_identity'] is True
        identity = rendered['extensions']['argus_identity']
        endpoint, collector_id = identity['trust_bundle_endpoint'], identity['collector_id']
        assert identity['rotate_before'] == '8h' and identity['check_interval'] == '5m'
        assert self.probe_identity(gateway, endpoint) in (200, 204)
        identity['rotate_before'], identity['check_interval'] = '23h59m', '10s'
        temporary = yaml.safe_dump(rendered, sort_keys=False)
        deployment = self.run.infra.get('deployment', 'argus-otelcol-gateway', '-n', NAMESPACE)
        state = {'started_at': now().isoformat(), 'config_name': config_name, 'config_uid': config['metadata']['uid'],
            'deployment_uid': deployment['metadata']['uid'], 'image': pod['spec']['containers'][0]['image'],
            'original_yaml': original, 'test_yaml': temporary, 'restored': False}
        self.save('identity-test-state', state)
        remote = '/tmp/argus-collector-identity-' + state['config_uid'] + '.pem'
        old_pem = self.node_command('cat', gateway['path'] + '/client.pem')
        assert x509.load_pem_x509_certificate(old_pem.encode()).not_valid_after_utc.timestamp() > now().timestamp() + minutes * 60 + 180
        self.node_command('sh', '-c', 'umask 077; cat > "$1"', 'public-leaf', remote, data=old_pem)
        old_serial = gateway['certificates']['client']['serial']
        report = {'started_at': state['started_at'], 'collector_id': collector_id, 'old_serial': old_serial,
            'old_identity_before': 'accepted', 'private_keys_left_on_node': True, 'new_gateway_serials': [],
            'sample_checks': 0, 'failed_sample_checks': 0, 'server_leaf_reloaded': False}
        self.save('identity-renewal-review', report)
        try:
            self.patch_config(state, original, temporary)
            self.restart_gateway(state)
            steady = self.wait_healthy(now())
            startup = next(p for p in steady['identities'] if p['role'] == 'gateway')
            startup_server = startup['certificates']['server']['serial']
            native_before, warmup_seconds = self.wait_exporter_settled(startup)
            steady = self.wait_healthy(now())
            report['warmup_seconds'] = warmup_seconds
            report['steady_started_at'] = steady['time']
            report['native_exporter_before'] = native_before
            report['startup_client_serial'] = startup['certificates']['client']['serial']
            report['startup_server_serial'] = startup_server
            startup_remote = remote + '.startup.pem'
            startup_pem = self.node_command('cat', startup['path'] + '/client.pem')
            self.node_command('sh', '-c', 'umask 077; cat > "$1"', 'public-leaf', startup_remote, data=startup_pem)
            started = time.monotonic()
            last_summary = -1
            while time.monotonic() - started < minutes * 60:
                snapshot = self.snapshot()
                report['sample_checks'] += 1
                if not self.healthy(snapshot):
                    report['failed_sample_checks'] += 1
                    self.save('unhealthy-renewal-snapshot', snapshot)
                    raise RuntimeError('A stream stopped during steady identity renewal')
                current = next(p for p in snapshot['identities'] if p['role'] == 'gateway')
                serial = current['certificates']['client']['serial']
                if serial not in report['new_gateway_serials']:
                    assert serial != old_serial
                    report['new_gateway_serials'].append(serial)
                elapsed = time.monotonic() - started
                snapshot['elapsed_seconds'] = round(elapsed, 2)
                with (self.out / 'identity-continuity.jsonl').open('a', encoding='utf-8') as stream:
                    stream.write(json.dumps(snapshot) + '\n')
                if elapsed > 90 and not report['server_leaf_reloaded']:
                    served = self.probe_gateway_server(current, collector_id)
                    report['server_leaf_reloaded'] = served != startup_server
                    report['served_server_serial'] = served
                minute = int(elapsed // 60)
                if minute != last_summary:
                    print('Renewal minute ' + str(minute) + ': eight streams fresh; ' +
                        str(len(report['new_gateway_serials'])) + ' Gateway generations observed', flush=True)
                    last_summary = minute
                self.save('identity-renewal-review', report)
                time.sleep(15)
            report['steady_seconds'] = round(time.monotonic() - started, 2)
            assert len(report['new_gateway_serials']) >= 2 and report['server_leaf_reloaded']
            assert current['uid'] == startup['uid'], 'Gateway unexpectedly restarted during the continuous test'
            native_after = self.exporter_metrics(current)
            report['native_exporter_after'] = native_after
            self.save('identity-renewal-review', report)
            logs = self.run.infra.kubectl('logs', current['name'], '-n', NAMESPACE, '--since-time=' + steady['time'])
            (self.out / 'gateway-continuous-renewal.log').write_text(logs, encoding='utf-8')
            for key, value in native_after.items():
                if 'send_failed_' in key or 'enqueue_failed_' in key:
                    assert value == native_before.get(key, 0), 'Exporter dropped or failed telemetry during renewal'
            assert 'Exporting failed. Dropping data.' not in logs
            assert 'telemetry downstream identity rejected' not in logs
            assert 'telemetry identity or payload rejected' not in logs
            report['old_identity_after_http_status'] = self.probe_identity(current, endpoint, remote)
            assert report['old_identity_after_http_status'] == 401
            report['startup_identity_after_http_status'] = self.probe_identity(current, endpoint, startup_remote)
            assert report['startup_identity_after_http_status'] == 401
            assert self.probe_identity(current, endpoint) in (200, 204)
            rows = self.run.builtins(self.api)
            report['target_metrics'] = {}
            for key, metric in METRICS.items():
                collection = next(r['collection'] for r in rows if r['definition']['key'] == key)
                result = query(self.run, self.api, metric + '{argus_collection_id="' + collection['id'] + '"}')
                assert result['data']['result']
                report['target_metrics'][key] = len(result['data']['result'])
            report['passed_overlap'] = True
            self.save('identity-renewal-review', report)
        finally:
            # Capture failures before deleting the Pod during restoration.
            try:
                actual = next(p for p in self.identities() if p['role'] == 'gateway')
                logs = self.run.infra.kubectl('logs', actual['name'], '-n', NAMESPACE, '--since-time=' + state['started_at'])
                (self.out / 'gateway-whole-renewal.log').write_text(logs, encoding='utf-8')
                self.save('exporter-before-restore', self.exporter_metrics(actual))
            except (RuntimeError, StopIteration, subprocess.SubprocessError):
                pass
            self.restore()
            # Only a public leaf was staged, under the recorded owned path.
            self.node_command('rm', '-f', '--', remote, remote + '.startup.pem')
        # Agents follow within 1m plus kubelet projection delay. Check a common
        # stable final serial after returning to the product rotation interval.
        deadline = time.monotonic() + 150
        while time.monotonic() < deadline:
            identities = self.identities()
            clients = {p['certificates']['client']['serial'] for p in identities}
            servers = {p['certificates']['server']['serial'] for p in identities}
            if len(identities) >= 2 and len(clients) == len(servers) == 1:
                break
            time.sleep(5)
        else:
            raise RuntimeError('Agents did not adopt the final Gateway identity')
        secret = self.run.infra.get('secret', 'argus-otelcol-identity', '-n', NAMESPACE)
        mirrored = x509.load_pem_x509_certificate(base64.b64decode(secret['data']['client.pem']))
        assert format(mirrored.serial_number, 'x') == next(iter(clients))
        report.update(passed=True, final_client_serial=next(iter(clients)), final_agents=len(identities)-1,
            original_configuration_restored=True, completed_at=now().isoformat())
        self.save('identity-renewal-review', report)
        print('Actual 15m overlap crossed; retired identity rejected, all Agents and eight streams remain healthy', flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--session', type=Path, required=True)
    parser.add_argument('--restore', action='store_true')
    parser.add_argument('--minutes', type=int, default=17)
    args = parser.parse_args()
    review = Review(args.output, args.session)
    if args.restore:
        review.restore()
    else:
        review.exercise(args.minutes)


if __name__ == '__main__':
    main()
