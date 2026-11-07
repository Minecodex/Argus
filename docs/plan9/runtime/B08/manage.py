"""Owned Compose deployment for Plan9 B08."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import sys

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[3]
sys.path.insert(0, str(HERE.parent))
from B01.manage import Run as OwnedRun, execute, now, write_json

RUN_ID = 'p9-b08-20261105-01'
PREDECESSOR = 'p9-b07c-20261104-01'
HOSTS = {'collector': {'letter': 'L', 'ssh_port': 22344, 'name': 'B08-Linux-Logs'}}
BASE = 'argus-plan9-host:b01-20261026'
IMAGE = 'argus-plan9-logs:b08-20261105'


class Run(OwnedRun):
    def __init__(self):
        self.id = RUN_ID
        self.out = ROOT / 'artifacts/plan9' / self.id
        self.envfile = self.out / 'compose.env'
        self.resources = self.out / 'resource-map.json'

    def compose(self, *args, timeout=120):
        plugin = Path('C:/Program Files/Docker/Docker/resources/cli-plugins/docker-compose.exe')
        command = [str(plugin)] if os.name == 'nt' and plugin.is_file() else ['docker', 'compose']
        return execute([*command, '--env-file', str(self.envfile), '-f', str(HERE/'compose.yaml'),
                        '--project-name', self.id, *args], timeout=timeout)

    def container(self, host):
        assert host in HOSTS
        cid = self.compose('ps', '--all', '--quiet', host)
        assert cid, 'owned container does not exist'
        value = json.loads(execute(['docker', 'inspect', cid]))[0]
        labels = value['Config']['Labels']
        assert labels['io.argus.plan9.run'] == self.id and labels['io.argus.plan9.batch'] == 'B08'
        assert value['HostConfig']['CgroupnsMode'] == 'private'
        return value

    def platform(self):
        def resources(kind):
            return json.loads(execute(['kubectl', '--context', 'docker-desktop', 'get', kind, '-n', 'argus-system', '-o', 'json']))['items']
        return {
            'deployments': {r['metadata']['name']: [r['metadata']['uid'], r['metadata']['generation'],
                [c['image'] for c in r['spec']['template']['spec']['containers']]] for r in resources('deployment')},
            'volumes': {r['metadata']['name']: [r['metadata']['uid'], r['spec']['volumeName']] for r in resources('pvc')},
        }

    def prepare(self):
        private = self.out / 'private'
        private.mkdir(parents=True, exist_ok=True)
        if os.name == 'nt':
            execute(['icacls', str(private), '/inheritance:r', '/grant:r',
                     execute(['whoami']) + ':(OI)(CI)F', '*S-1-5-18:(OI)(CI)F'])
        else:
            private.chmod(0o700)
        creds = private / 'host-credentials.json'
        if not creds.exists():
            write_json(creds, {'collector': {'username': 'root', 'password': secrets.token_urlsafe(30)}})
        if not self.resources.exists():
            write_json(self.resources, {'run_id': self.id, 'hosts': {}, 'targets': {}, 'log_rules': {},
                                       'dashboards': {}, 'alert_policies': {}, 'containers': []})
        if not (self.out/'run.json').exists():
            write_json(self.out/'run.json', {'run_id': self.id, 'batch': 'B08', 'status': 'preparing',
                'authorization': 'user_requested_batch_transition', 'created_at': now(),
                'limits': {'cpus': 1.5, 'memory_mib': 1280, 'background_hours': 8, 'fault_seconds': 100}})
            write_json(self.out/'platform-before.json', self.platform())

    def build(self):
        base = json.loads(execute(['docker','image','inspect',BASE]))[0]
        execute(['docker', 'build', '--build-arg', 'PLAN9_BASE='+BASE, '-t', IMAGE, str(HERE)], timeout=180)
        image = json.loads(execute(['docker','image','inspect',IMAGE]))[0]
        write_json(self.out/'image-lock.json', {'base_id': base['Id'], 'image_id': image['Id'],
            'architecture': image['Architecture'], 'source_hashes': {p.name: hashlib.sha256(p.read_bytes()).hexdigest()
            for p in HERE.iterdir() if p.is_file()}})
        self.envfile.write_text(f'PLAN9_RUN={self.id}\nPLAN9_HOST_IMAGE={image["Id"]}\n', encoding='utf-8')
        self.compose('config', '--quiet')

    def up(self):
        old = self.out.parent / PREDECESSOR
        assert json.loads((old/'cleanup.json').read_text(encoding='utf-8'))['status']=='destroyed'
        assert (old/'cleanup-verification.json').is_file()
        m = json.loads(self.resources.read_text(encoding='utf-8'))
        if m['containers']:
            assert self.container('collector')['Id'] == m['containers'][0]['id']
            return
        self.compose('up','--detach','--no-build','--wait','--wait-timeout','120',timeout=180)
        c = self.container('collector')
        secret = json.loads((self.out/'private/host-credentials.json').read_text(encoding='utf-8'))
        execute(['docker','exec','-i',c['Id'],'chpasswd'],data='root:'+secret['collector']['password']+'\n')
        m['containers'] = [{'service':'collector','id':c['Id'],'image_id':c['Image']}]
        write_json(self.resources,m)
        self.event('infrastructure_started',service='collector')

    def request(self, label, stream='rename', error=False):
        assert re.fullmatch(r'[a-z][a-z0-9-]{0,50}', label) and stream in ('rename','copy')
        script = f'python3 /usr/local/lib/plan9-b08.py request --label {label} --stream {stream}'
        return json.loads(self.shell('collector',script+(' --error' if error else '')+'\n'))

    def workload(self):
        self.shell('collector','systemctl start plan9-b08-load.service\nsystemctl is-active --quiet plan9-b08-load.service\n')


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['prepare','build','up','workload'])
    getattr(Run(),parser.parse_args().action)()
