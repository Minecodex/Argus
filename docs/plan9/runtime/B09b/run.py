"""Normal API and ownership boundary for the B09b component-metrics review."""
import argparse
from datetime import datetime, timezone
import getpass
import json
import os
from pathlib import Path
import shutil
import sys

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[3]
sys.path.insert(0, str(HERE.parent))
from B09a.manage import Run as WorkloadRun, execute, write_json
from common.api import Api

RUN = 'p9-b09b-20261109-01'
KEYS = (
    'kubernetes_apiserver_metrics', 'kubernetes_coredns_metrics',
    'kubernetes_scheduler_metrics', 'kubernetes_controller_metrics',
    'kubernetes_etcd_metrics',
)
TEMPLATES = dict(zip(KEYS, (
    'kubernetes.apiserver', 'kubernetes.coredns', 'kubernetes.scheduler',
    'kubernetes.controller_manager', 'kubernetes.etcd',
)))


class Run:
    def __init__(self):
        self.id = os.environ.get('ARGUS_PLAN9_B09B_RUN', RUN)
        if self.id != RUN:
            from infrastructure import Infrastructure
            self.infra = Infrastructure(self.id)
        else:
            self.infra = WorkloadRun()
        self.out = ROOT / 'artifacts/plan9' / self.id
        self.private = self.out / 'private'
        self.private.mkdir(parents=True, exist_ok=True)
        if os.name == 'nt':
            execute(['icacls', str(self.private), '/inheritance:r', '/grant:r',
                     execute(['whoami']) + ':(OI)(CI)F', '*S-1-5-18:(OI)(CI)F'])
        else:
            self.private.chmod(0o700)
        self.mapping = json.loads(self.infra.resources.read_text(encoding='utf-8'))
        self.cluster = self.mapping['cluster']['id']
        self.session = self.private / 'enterprise-session.json'
        self.ca = ROOT / 'artifacts/plan9/reset-20261026/trust-bundle.pem'
        self.node = self.infra.id + '-control-plane'

    def event(self, kind, **values):
        with (self.out / 'timeline.jsonl').open('a', encoding='utf-8') as stream:
            stream.write(json.dumps({'time': datetime.now(timezone.utc).isoformat(),
                                     'event': kind, **values}) + '\n')

    def api(self, login=False):
        if not self.session.exists():
            previous = ROOT / 'artifacts/kubernetes-resource-recovery-20261109/private/enterprise-session.json'
            shutil.copy2(previous, self.session)
        api = Api(self, self.session, self.ca)
        if login:
            api.credentials = {'enterprise_username': 'kakj',
                               'password': getpass.getpass('Enterprise administrator password: ')}
            api.login()
            api.credentials = None
        return api

    def save(self, name, value):
        write_json(self.out / (name + '.json'), value)

    def builtins(self, api):
        return api.call('GET', '/monitoring/builtins?target_type=kubernetes_cluster&target_id=' + self.cluster)

    def inspect(self, api):
        rows = self.builtins(api)
        self.save('initial-builtins', rows)
        self.save('platform-before', self.infra.platform())
        self.save('collector-before', api.call('GET', '/enterprise/kubernetes-clusters/' + self.cluster + '/collector'))
        self.save('infrastructure', {'run_id': self.id, 'infrastructure_run': self.infra.id,
            'cluster_id': self.cluster,
            'node': json.loads(execute(['docker', 'inspect', self.node]))[0]['Id'],
            'scope': ('fresh_independent_cluster' if self.infra.id==self.id else
                      'extend_existing_owned_cluster_preserve_workloads_and_resource_identity')})
        print(json.dumps([{'key': r['definition']['key'], 'available': r['available'],
                           'collection': r.get('collection')} for r in rows]), flush=True)

    def converge(self, api):
        path = '/enterprise/kubernetes-clusters/' + self.cluster + '/collector'
        def ready(value):
            if value['status'] in ('failed', 'result_unknown', 'uninstalled'):
                raise RuntimeError('Collector rollout: ' + value['status'])
            return value['status'] == 'converged' and value['desired_revision'] == value['effective_revision']
        return api.wait(path, ready, timeout=240, label='Component Collector configuration')

    def change(self, api, key, enabled, config=None):
        self.converge(api)
        row = next(r for r in self.builtins(api) if r['definition']['key'] == key)
        assert row['available'], (key, row.get('reason'))
        current = row.get('collection') or {}
        body = {'target': {'kind': 'kubernetes_cluster', 'id': self.cluster},
                'key': key, 'enabled': enabled, 'expected_version': current.get('version', 0)}
        if config is not None:
            body['config'] = config
        if current.get('desired_enabled') != enabled or config is not None and current.get('config') != config:
            api.confirm(api.call('POST', '/monitoring/builtins/preview', body,
                                 key='component-' + key + '-' + str(current.get('version', 0)), expected=201))
            self.converge(api)
        row = next(r for r in self.builtins(api) if r['definition']['key'] == key)
        self.event('component_configured', key=key, enabled=enabled,
                   revision=row['collection']['effective_revision'])
        return row['collection']

    def enable(self, api):
        for key in KEYS:
            value = self.change(api, key, True, {'interval': '10s'})
            print(key + ': ' + value['apply_status'] + ' / ' + value['health'], flush=True)
        self.save('components-enabled', self.builtins(api))


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['inspect', 'login', 'enable', 'prepare', 'restore', 'activity', 'verify', 'exercise', 'availability', 'permissions', 'alerts', 'retire', 'final'])
    args = parser.parse_args()
    run = Run()
    api = run.api(login=args.action == 'login')
    if args.action in ('prepare', 'restore'):
        from environment import prepare
        prepare(run, restore=args.action == 'restore')
    elif args.action == 'enable':
        run.enable(api)
    elif args.action == 'activity':
        from workloads import deploy
        deploy(run)
    elif args.action == 'verify':
        from verification import fresh, dashboards
        fresh(run, api)
        dashboards(run, api)
    elif args.action == 'exercise':
        from exercise import exercise
        exercise(run, api)
    elif args.action == 'availability':
        from exercise import availability
        availability(run, api)
    elif args.action == 'final':
        from verification import final_review
        final_review(run, api)
    elif args.action == 'retire':
        from retirement import retire
        retire(run, api)
    elif args.action == 'alerts':
        from alerts import configure
        configure(run, api)
    elif args.action == 'permissions':
        from verification import permissions
        permissions(run)
    else:
        run.inspect(api)
