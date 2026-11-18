"""Owned B13a two-host, two-cache dashboard acceptance infrastructure."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import sys

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[3]
sys.path.insert(0, str(HERE.parent))
from B01.manage import Run as OwnedRun, execute, now, write_json
from B09a.manage import Run as PlatformRun
from common.api import Api

RUN = 'p9-b13a-20261117-01'
BASE = 'argus-plan9-host:b11c-base-20261115'
SERVICES = ['orders', 'reports', 'redis', 'memcached']


class Run(OwnedRun):
    RUN_ID = RUN
    BATCH = 'B13a'
    PACKAGE = HERE
    BUILD_CONTEXT = None
    HOST_TAG = 'argus-plan9-dashboard:b13a-20261117'
    PREDECESSOR = 'p9-b12-20261116-01'
    AUTHORIZATION = '继续下一轮 B13a'

    def __init__(self):
        self.id = self.RUN_ID
        self.out = Path(os.environ.get('ARGUS_PLAN9_ARTIFACT_ROOT', 'C:/Users/23574/.codex/artifacts/argus-plan9')) / self.id
        self.private = self.out / 'private'
        self.resources = self.out / 'resource-map.json'
        self.envfile = self.out / 'compose.env'

    def save(self, name, value):
        write_json(self.out / (name + '.json'), value)

    def platform(self):
        return PlatformRun().platform()

    def api(self):
        return Api(self, self.private / 'enterprise-session.json', ROOT / 'deploy/.cache/argus-trust-bundle.pem',
                   {'enterprise_username': os.environ.get('ARGUS_PLAN9_USERNAME', 'kakj'), 'password': os.environ['ARGUS_PLAN9_PASSWORD']})

    def compose(self, *args, timeout=120):
        return execute(['C:/Program Files/Docker/Docker/resources/cli-plugins/docker-compose.exe', '--env-file', str(self.envfile),
                        '-f', str(self.PACKAGE / 'compose.yaml'), '--project-name', self.id, *args], timeout=timeout)

    def container(self, key='orders'):
        assert key in SERVICES
        ident = self.compose('ps', '--all', '--quiet', key)
        assert ident
        row = json.loads(execute(['docker', 'inspect', ident]))[0]
        assert row['Config']['Labels']['io.argus.plan9.run'] == self.id
        prior = next((x for x in json.loads(self.resources.read_text(encoding='utf-8'))['containers'] if x['service'] == key), None)
        if prior:
            assert row['Id'] == prior['id']
        return row

    def prepare(self):
        if self.resources.exists():
            assert json.loads(self.resources.read_text(encoding='utf-8'))['run_id'] == self.id, 'Refuse to write another run evidence directory'
        retired = self.out / 'cleanup-verification.json'
        if retired.exists() and json.loads(retired.read_text(encoding='utf-8'))['status'] == 'destroyed':
            raise RuntimeError('Retired run is immutable; prepare a fresh run ID')
        self.private.mkdir(parents=True, exist_ok=True)
        execute(['icacls', str(self.private), '/inheritance:r', '/grant:r', execute(['whoami']) + ':(OI)(CI)F', '*S-1-5-18:(OI)(CI)F'])
        if not self.resources.exists():
            self.save('resource-map', {'run_id': self.id, 'hosts': {}, 'containers': [], 'targets': {}, 'dashboards': {},
                                      'applications': {}, 'alert_policies': {}, 'remote': {}})
            write_json(self.private / 'host-credentials.json', {key: {'username': 'root', 'password': secrets.token_urlsafe(32)} for key in ['orders', 'reports']})
            write_json(self.private / 'redis-credentials.json', {role: secrets.token_urlsafe(32) for role in ['app', 'monitor']})
            self.save('run', {'run_id': self.id, 'batch': self.BATCH, 'status': 'preparing', 'created_at': now(),
                             'authorization': self.AUTHORIZATION, 'human_acceptance': 'pending', 'shared_kernel_scope': True,
                             'limits': {'cpus': 3, 'memory_mib': 2368, 'traffic_seconds': 28800, 'fault_seconds': 120},
                             'real_model_calls': 0, 'direct_database_writes': False})
        if not (self.private / 'enterprise-session.json').exists():
            write_json(self.private / 'enterprise-session.json', {'origin': 'https://argus.dev', 'csrf': '', 'user': {}, 'cookies': []})
        credentials = json.loads((self.private / 'redis-credentials.json').read_text(encoding='utf-8'))
        acl = 'user default off\n'
        for role, commands in [('monitor', '+info +config|get +ping'), ('app', '~p9:' + self.BATCH.lower() + ':* +get +set +del +ping')]:
            acl += 'user ' + role + ' on #' + hashlib.sha256(credentials[role].encode()).hexdigest() + ' ' + commands + '\n'
        (self.private / 'users.acl').write_text(acl, encoding='utf-8')
        (self.private / 'redis.conf').write_text('bind 0.0.0.0\nprotected-mode yes\nport 6379\naclfile /run/secrets/users.acl\nmaxmemory 16mb\nmaxmemory-policy allkeys-lru\nsave ""\nappendonly no\n', encoding='utf-8')
        for key in ['orders', 'reports']:
            (self.private / (key + '.env')).write_text('PLAN9_APP=' + key + '\n', encoding='utf-8')
        api = self.api()
        api.login()
        self.save('platform-before', self.platform())
        print(self.BATCH + ' private state and formal deployment baseline prepared', flush=True)

    def build(self):
        base = json.loads(execute(['docker', 'image', 'inspect', BASE]))[0]
        assert base['Architecture'] == 'amd64'
        host = self.HOST_TAG
        context = self.BUILD_CONTEXT or self.PACKAGE
        execute(['docker', 'build', '--platform', 'linux/amd64', '--build-arg', 'PLAN9_BASE=' + BASE,
                 '-f', str(self.PACKAGE / 'Dockerfile'), '-t', host, str(context)], timeout=240)
        # Reuse already verified versions; pull only missing artifacts, pin every ID.
        refs = {'host': host, 'redis': 'redis:8.10.0-alpine', 'memcached': 'memcached:1.6-alpine'}
        images = {}
        for key, ref in refs.items():
            try:
                row = json.loads(execute(['docker', 'image', 'inspect', ref]))[0]
            except RuntimeError:
                execute(['docker', 'pull', ref], timeout=180)
                row = json.loads(execute(['docker', 'image', 'inspect', ref]))[0]
            assert row['Architecture'] == 'amd64'
            images[key] = {'id': row['Id'], 'repo_digests': row.get('RepoDigests', [])}
        assert json.loads(execute(['docker', 'image', 'inspect', BASE]))[0]['Id'] == base['Id']
        values = {'PLAN9_RUN': self.id, 'PLAN9_PRIVATE': self.private.as_posix(), **{'PLAN9_' + key.upper() + '_IMAGE': row['id'] for key, row in images.items()}}
        self.envfile.write_text(''.join(k + '=' + v + '\n' for k, v in values.items()), encoding='utf-8')
        self.compose('config', '--quiet')
        self.save('image-lock', {'base_id': base['Id'], 'images': images, 'source': {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in self.PACKAGE.iterdir() if p.is_file()}})
        print('Images pinned and Compose validated', flush=True)

    def up(self):
        assert json.loads((self.out.parent / self.PREDECESSOR / 'cleanup-verification.json').read_text(encoding='utf-8'))['status'] == 'destroyed'
        mapping = json.loads(self.resources.read_text(encoding='utf-8'))
        assert not mapping['containers'], 'Inspect existing infrastructure before rerunning up'
        self.compose('up', '-d', '--no-build', '--wait', '--wait-timeout', '120', timeout=180)
        passwords = json.loads((self.private / 'host-credentials.json').read_text(encoding='utf-8'))
        specs = {}
        for key in SERVICES:
            row = self.container(key)
            mapping['containers'].append({'service': key, 'id': row['Id'], 'image_id': row['Image']})
            self.save('resource-map', mapping)
            if key in passwords:
                execute(['docker', 'exec', '-i', row['Id'], 'chpasswd'], data='root:' + passwords[key]['password'] + '\n')
                specs[key] = {'letter': 'A' if key == 'orders' else 'B', 'name': self.BATCH + '-' + key.title() + '-Host', 'address': row['NetworkSettings']['Networks']['kind']['IPAddress'],
                              'ssh_port': 22, 'control_path': 'executor_tunnel', 'collector_transport': 'direct'}
        self.save('host-specs', specs)
        assert self.platform() == json.loads((self.out / 'platform-before.json').read_text(encoding='utf-8'))
        print('Two owned systemd hosts and two native caches healthy', flush=True)

    def workload(self):
        evidence = {}
        prefix = 'plan9-' + self.BATCH.lower()
        for key in ['orders', 'reports']:
            if self.shell(key, 'systemctl is-active ' + prefix + '-traffic.service || true\n') != 'active':
                self.shell(key, 'systemd-run --collect --unit=' + prefix + '-traffic --property=RuntimeMaxSec=28800 --property=MemoryMax=64M --property=CPUQuota=5% /usr/bin/python3 /usr/local/lib/' + prefix + '/service.py traffic\n')
            evidence[key] = json.loads(self.shell(key, 'curl -fsS http://127.0.0.1:18085/request\n'))
        self.save('native-workloads', {'at': now(), 'applications': evidence, 'shared_redis_identity': 'redis:6379', 'fabricated_telemetry': False})
        print('Eight-hour bounded HTTP requests exercise shared Redis and Reports Memcached', flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['prepare', 'build', 'up', 'workload'])
    getattr(Run(), parser.parse_args().action)()
