"""Owned B01 infrastructure and real workload lifecycle (no Argus DB access)."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import subprocess
import sys

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[3]


def execute(argv, *, data=None, timeout=120):
    if argv[0] == 'docker' and os.name == 'nt':
        desktop = Path(os.environ.get('ProgramFiles', 'C:/Program Files')) / 'Docker/Docker/resources/bin/docker.exe'
        if desktop.is_file():
            argv = [str(desktop), *argv[1:]]
    result = subprocess.run(argv, input=data.encode('utf-8') if data is not None else None,
                            capture_output=True, cwd=REPO, timeout=timeout)
    if result.returncode:
        # Callers using stdin may supply private material; never print their output.
        if data is not None:
            raise RuntimeError(f'{argv[0]} failed, private output suppressed (exit {result.returncode})')
        raise RuntimeError((result.stderr[-1800:] or result.stdout[-1800:]).decode('utf-8', errors='replace'))
    return result.stdout.decode('utf-8', errors='replace').strip()


def now():
    return datetime.now(timezone.utc).isoformat()


def write_json(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


class Run:
    def __init__(self, run_id):
        if not re.fullmatch(r'p9-b01-\d{8}-\d{2}', run_id):
            raise ValueError('invalid B01 run ID')
        self.id = run_id
        self.out = REPO / 'artifacts' / 'plan9' / run_id
        self.envfile = self.out / 'compose.env'
        self.resources = self.out / 'resource-map.json'

    def compose(self, *args, timeout=120):
        plugin = Path('C:/Program Files/Docker/Docker/resources/cli-plugins/docker-compose.exe')
        command = [str(plugin)] if os.name == 'nt' and plugin.is_file() else ['docker', 'compose']
        return execute([*command, '--env-file', str(self.envfile),
                        '--file', str(HERE / 'compose.yaml'), '--project-name', self.id, *args],
                       timeout=timeout)

    def event(self, kind, **fields):
        with (self.out / 'timeline.jsonl').open('a', encoding='utf-8') as stream:
            stream.write(json.dumps({'time': now(), 'event': kind, **fields}, ensure_ascii=False) + '\n')

    def prepare(self):
        self.out.mkdir(parents=True, exist_ok=True)
        private = self.out / 'private'
        private.mkdir(exist_ok=True)
        if os.name == 'nt':
            account = execute(['whoami'])
            execute(['icacls', str(private), '/inheritance:r', '/grant:r',
                     account + ':(OI)(CI)F', '*S-1-5-18:(OI)(CI)F'])
        else:
            private.chmod(0o700)
        if not (self.out / 'run.json').exists():
            state = {'run_id': self.id, 'batch': 'B01', 'created_at': now(),
                     'authorization': 'User: 部署 B01', 'status': 'preparing',
                     'review_status': 'not_reviewed', 'self_check': 'not_run',
                     'runtime_status': 'not_created', 'direct_database_writes': False,
                     'synthetic_telemetry': False, 'shared_kernel_scope': True,
                     'limits': {'each_host_cpus': 1.5, 'each_host_memory_mib': 1280,
                                'single_load_seconds': 120, 'max_test_file_mib': 64}}
            write_json(self.out / 'run.json', state)
            pods = json.loads(execute(['kubectl', '--context', 'docker-desktop', 'get', 'pods',
                                      '-n', 'argus-system', '-o', 'json']))
            write_json(self.out / 'platform-before.json', [
                {'name': p['metadata']['name'], 'uid': p['metadata']['uid'],
                 'phase': p['status']['phase'],
                 'images': [c['image'] for c in p['spec']['containers']]}
                for p in pods['items']])
            pvcs = json.loads(execute(['kubectl', '--context', 'docker-desktop', 'get', 'pvc',
                                      '-A', '-o', 'json']))
            write_json(self.out / 'volumes-before.json', [
                {'namespace': p['metadata']['namespace'], 'name': p['metadata']['name'],
                 'uid': p['metadata']['uid'], 'volume': p['spec'].get('volumeName'),
                 'phase': p['status']['phase']} for p in pvcs['items']])
            write_json(private / 'host-credentials.json', {
                host: {'username': 'root', 'password': secrets.token_urlsafe(30)}
                for host in ('host-a', 'host-b')})
            write_json(self.resources, {'run_id': self.id, 'containers': [], 'hosts': {},
                                       'borrowed': ['existing formal Argus platform'],
                                       'platform_objects': [], 'policies': []})
        if not self.envfile.exists():
            self.envfile.write_text(f'PLAN9_RUN={self.id}\nPLAN9_HOST_IMAGE=argus-plan9-host:b01-20261026\n'
                                    'PLAN9_SSH_A=22221\nPLAN9_SSH_B=22222\n', encoding='utf-8')
        self.compose('config', '--quiet')
        current = json.loads((self.out / 'run.json').read_text(encoding='utf-8'))
        if current.get('runtime_status') == 'running':
            return {'run_id': self.id, 'prepared': True, 'existing_run_preserved': True}
        progress_path = REPO / 'docs/plan9/progress.json'
        progress = json.loads(progress_path.read_text(encoding='utf-8'))
        progress['current_run_id'] = self.id
        progress['status'] = 'b01_preparing'
        first = progress['batches'][0]
        first.update(status='preparing', package_status='in_progress')
        if self.id not in first['runs']:
            first['runs'].append(self.id)
        if not any(r.get('run_id') == self.id for r in progress['runs']):
            progress['runs'].append({'run_id': self.id, 'batch': 'B01',
                                    'record': '../../artifacts/plan9/' + self.id + '/run.json'})
        write_json(progress_path, progress)
        self.event('prepared')
        return {'run_id': self.id, 'prepared': True}

    def container(self, host):
        if host not in ('host-a', 'host-b'):
            raise ValueError('unknown owned host')
        cid = self.compose('ps', '--all', '--quiet', host)
        if not cid:
            raise RuntimeError('owned container does not exist')
        value = json.loads(execute(['docker', 'inspect', cid]))[0]
        if value['Config']['Labels'].get('io.argus.plan9.run') != self.id:
            raise RuntimeError('container ownership mismatch')
        return value

    def shell(self, host, script, timeout=120):
        cid = self.container(host)['Id']
        return execute(['docker', 'exec', '-i', cid, '/bin/bash', '-se'], data=script, timeout=timeout)

    def up(self):
        existing = json.loads(self.resources.read_text(encoding='utf-8'))
        if any(x.get('host_id') for x in existing['hosts'].values()):
            for host in ('host-a', 'host-b'):
                if not self.container(host)['State']['Running']:
                    raise RuntimeError('registered host is stopped; recover the existing container')
            return {'running_hosts': 2, 'existing_registration_preserved': True}
        inspection = json.loads(execute(['docker', 'image', 'inspect', 'argus-plan9-host:b01-20261026']))[0]
        image_id = inspection['Id']
        source_hashes = {str(p.relative_to(HERE)): hashlib.sha256(p.read_bytes()).hexdigest()
                         for p in HERE.rglob('*') if p.is_file() and '__pycache__' not in p.parts}
        write_json(self.out / 'image-lock.json', {'image_id': image_id, 'architecture': inspection['Architecture'],
                                               'base': 'debian@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad',
                                               'source_hashes': source_hashes})
        text = self.envfile.read_text(encoding='utf-8')
        text = re.sub(r'^PLAN9_HOST_IMAGE=.*$', 'PLAN9_HOST_IMAGE=' + image_id, text, flags=re.M)
        self.envfile.write_text(text, encoding='utf-8')
        self.compose('up', '--detach', '--no-build', '--wait', '--wait-timeout', '120', timeout=180)
        creds = json.loads((self.out / 'private/host-credentials.json').read_text(encoding='utf-8'))
        mapping = json.loads(self.resources.read_text(encoding='utf-8'))
        mapping['containers'] = []
        for host in ('host-a', 'host-b'):
            value = self.container(host)
            if value['HostConfig']['CgroupnsMode'] != 'private':
                raise RuntimeError('private cgroup required')
            execute(['docker', 'exec', '-i', value['Id'], 'chpasswd'],
                    data='root:' + creds[host]['password'] + '\n')
            identity = self.shell(host, 'cat /etc/machine-id\nssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub\n')
            mapping['containers'].append({'service': host, 'id': value['Id'], 'name': value['Name'],
                                          'image_id': value['Image'], 'identity': identity,
                                          'networks': value['NetworkSettings']['Networks']})
        if mapping['containers'][0]['identity'].splitlines()[0] == mapping['containers'][1]['identity'].splitlines()[0]:
            raise RuntimeError('machine IDs are not independent')
        write_json(self.resources, mapping)
        state = json.loads((self.out / 'run.json').read_text(encoding='utf-8'))
        state.update(status='deploying', runtime_status='running', infrastructure_started_at=now())
        write_json(self.out / 'run.json', state)
        self.event('infrastructure_started', image_id=image_id, hosts=2)
        return {'healthy_hosts': 2, 'image_id': image_id}

    def workload(self, host, mode, seconds=120):
        if mode not in ('cpu', 'memory', 'disk', 'network'):
            raise ValueError('unsupported workload')
        if not 10 <= seconds <= 120:
            raise ValueError('workload exceeds the B01 duration budget')
        script = ('systemd-run --unit=plan9-load-' + mode + ' --collect '
                  '--property=CPUQuota=80% --property=MemoryMax=256M --property=RuntimeMaxSec=' + str(seconds + 30) + ' '
                  '--property=EnvironmentFile=/etc/plan9.env '
                  '/usr/local/bin/plan9-workload ' + mode + ' --seconds ' + str(seconds) + '\n')
        self.shell(host, script)
        self.event('load_started', host=host, mode=mode, bounded_seconds=seconds)
        return {'load_started': mode, 'host': host, 'max_seconds': seconds + 30}

    def fault(self, host, recover=False):
        script = ('rm -f /var/lib/plan9/service-fault\nsystemctl reset-failed plan9-web\n'
                  'systemctl restart plan9-web\nsystemctl is-active plan9-web\n') if recover else (
                  'touch /var/lib/plan9/service-fault\nsystemctl restart plan9-web || true\n'
                  'sleep 1\nsystemctl is-failed plan9-web\n')
        output = self.shell(host, script)
        self.event('service_recovered' if recover else 'service_failed', host=host, observed=output)
        return {'host': host, 'observed': output}


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['prepare', 'up', 'status', 'load', 'fault', 'recover'])
    parser.add_argument('--run', default='p9-b01-20261026-01')
    parser.add_argument('--host', default='host-a', choices=['host-a', 'host-b'])
    parser.add_argument('--mode', choices=['cpu', 'memory', 'disk', 'network'])
    args = parser.parse_args()
    run = Run(args.run)
    if args.action == 'prepare':
        result = run.prepare()
    elif args.action == 'up':
        result = run.up()
    elif args.action == 'status':
        result = {h: run.shell(h, 'systemctl is-active ssh dbus plan9-web\ncat /proc/meminfo | head -3\n')
                  for h in ('host-a', 'host-b')}
    elif args.action == 'load':
        result = run.workload(args.host, args.mode)
    else:
        result = run.fault(args.host, args.action == 'recover')
    print(json.dumps(result, ensure_ascii=False))
