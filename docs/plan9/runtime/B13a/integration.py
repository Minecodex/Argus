"""Normal enrollment and domain associations, with one identity for shared Redis."""
import argparse
import json
from manage import Run
from common.hosts import HostEnrollment


class Batch(HostEnrollment):
    NAME = 'B13a'

    def __init__(self, run, api):
        super().__init__(run, api, json.loads((run.out / 'host-specs.json').read_text(encoding='utf-8')), self.NAME.lower())

    def converge(self, key):
        return self.api.wait('/enterprise/hosts/' + self.map['hosts'][key]['host_id'] + '/collector',
                             lambda x: x['status'] == 'converged' and x['desired_revision'] == x['effective_revision'],
                             timeout=180, label=key + ' collector convergence')

    def commit(self, body, key):
        return self.api.confirm(self.api.call('POST', '/monitoring/commands/preview', body, key=key, expected=201))['resource_ref']['resource_id']

    def targets(self):
        credentials = json.loads((self.run.private / 'redis-credentials.json').read_text(encoding='utf-8'))
        for engine, host, endpoint, name in [('redis', 'orders', 'redis:6379', self.NAME + '-Shared-Redis'),
                                             ('memcached', 'reports', 'memcached:11211', self.NAME + '-Reports-Memcached')]:
            self.converge(host)
            collector = self.map['hosts'][host]['collector_id']
            if engine not in self.map['targets']:
                rows = [x for x in self.api.call('GET', '/monitoring/collections?search=' + name)['items'] if x['name'] == name]
                assert len(rows) <= 1
                if not rows:
                    body = {'kind': 'collection', 'operation': 'create', 'collection': {
                        'name': name, 'integration': engine, 'collector_id': collector, 'target_name': name,
                        'target_identity': {'endpoint': endpoint}, 'enabled': True,
                        'credential_update': 'replace' if engine == 'redis' else 'remove',
                        'config': {'interval': '10s', 'timeout': '5s', 'tls': {'enabled': False}}}}
                    if engine == 'redis':
                        body['credentials'] = {'username': 'monitor', 'password': credentials['monitor']}
                    self.commit(body, 'collection-' + engine)
                    rows = [x for x in self.api.call('GET', '/monitoring/collections?search=' + name)['items'] if x['name'] == name]
                assert len(rows) == 1
                row = rows[0]
                self.map['targets'][engine] = {'name': name, 'engine': engine, 'host': host, 'collector_id': collector,
                                               'target_id': row['target']['id'], 'collection_id': row['id']}
                self.save()
            self.converge(host)
            row = self.api.wait('/monitoring/collections/' + self.map['targets'][engine]['collection_id'],
                                lambda x: x['apply_status'] == 'effective' and x['health'] == 'healthy' and x['freshness'] == 'fresh',
                                timeout=180, label=name + ' native metrics')
            self.run.save(engine + '-collection', row)
            print(name + ': real native metrics fresh', flush=True)

    def applications(self):
        for key, environment in [('orders', 'production'), ('reports', 'staging')]:
            name = self.NAME + ' ' + key.title()
            if key not in self.map['applications']:
                rows = [x for x in self.api.call('GET', '/applications?search=' + self.NAME)['items'] if x['name'] == name]
                assert len(rows) <= 1
                ident = rows[0]['id'] if rows else self.commit({'kind': 'application', 'operation': 'create', 'application': {
                    'name': name, 'environment': environment, 'owner_name': 'Plan9',
                    'description': 'Actual bounded HTTP requests on ' + key + ' host; shared Redis dependency'}}, 'application-' + key)
                self.map['applications'][key] = {'id': ident, 'name': name, 'environment': environment, 'bindings': {}}
                self.save()
            app = self.map['applications'][key]
            targets = [('host', key, self.map['hosts'][key]['host_id']), ('monitoring_target', 'redis', self.map['targets']['redis']['target_id'])]
            if key == 'reports':
                targets.append(('monitoring_target', 'memcached', self.map['targets']['memcached']['target_id']))
            for kind, resource, ident in targets:
                if resource in app['bindings']:
                    continue
                app['bindings'][resource] = self.commit({'kind': 'binding', 'operation': 'create', 'binding': {
                    'application_id': app['id'], 'target': {'kind': kind, 'id': ident}}}, key + '-binding-' + resource)
                self.save()
            detail = self.api.call('GET', '/applications/' + app['id'])
            self.run.save(key + '-application', detail)
        assert self.map['applications']['orders']['bindings']['redis'] != self.map['applications']['reports']['bindings']['redis']
        print('Two real applications in distinct environments linked to one shared Redis identity', flush=True)

    def dashboards(self):
        from dashboards import defaults
        defaults(self)


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['onboard', 'collectors', 'builtins', 'targets', 'applications', 'dashboards'])
    run = Run()
    api = run.api()
    api.login()
    batch = Batch(run, api)
    action = parser.parse_args().action
    if action == 'collectors':
        batch.collectors('0.3.3-b11a-20261113')
    else:
        getattr(batch, action)()
