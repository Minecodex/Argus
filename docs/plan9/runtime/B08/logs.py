"""Provision and prove the native Filelog and Journald rules through Argus."""
from datetime import datetime, timedelta, timezone
import json
import time

from manage import write_json
from common.hosts import HostEnrollment

SERVICE = 'plan9-b08.service'
DIR = '/var/lib/plan9/b08'
FAILED = ('max by (argus_resource_id) (systemd_unit_state{'
          'resource_systemd_unit_name="plan9-b08.service",systemd_unit_active_state="failed"})')
BUDGET = {'max_scan_bytes': 268435456, 'max_rows': 5000, 'max_samples': 1000000,
          'max_series': 10000, 'timeout_ms': 10000, 'max_result_bytes': 8388608}


def query_scope(resource, seconds=900):
    end = datetime.now(timezone.utc)
    return {'resource_ids': [resource], 'time_range': {
        'from': (end - timedelta(seconds=seconds)).isoformat(), 'to': end.isoformat()}, 'budget': BUDGET}


class Logs(HostEnrollment):
    def __init__(self, run, api):
        from manage import HOSTS
        super().__init__(run, api, HOSTS, 'b08')

    @property
    def host(self):
        return self.map['hosts']['collector']['host_id']

    @property
    def collector(self):
        return self.map['hosts']['collector']['collector_id']

    @property
    def app(self):
        return self.map['application']['id']

    def command(self, body, key):
        preview = self.api.call('POST', '/monitoring/commands/preview', body, key=key, expected=201)
        return self.api.confirm(preview)['resource_ref']['resource_id']

    def converge(self):
        return self.api.wait('/enterprise/hosts/' + self.host + '/collector',
            lambda row: row['status'] == 'converged' and row['desired_revision'] == row['effective_revision'],
            timeout=180, label='Collector configuration')

    def setup(self):
        state = self.map.setdefault('application', {})
        if 'id' not in state:
            name = 'B08 Linux File Logs'
            state.update(id=self.command({'kind': 'application', 'operation': 'create', 'application': {
                'name': name, 'environment': 'testing', 'description': 'Real B08 HTTP service file logs'}}, 'app'), name=name)
            self.save()
        if 'service_id' not in state:
            state['service_id'] = self.command({'kind': 'service', 'operation': 'create', 'service': {
                'application_id': self.app, 'service_namespace': 'plan9-b08', 'service_name': 'orders-log'}}, 'service')
            self.save()
        specs = {
            'json': ('B08 Application JSON', 'filelog', self.app, 'application', {
                'scope': 'business', 'include': [DIR + '/json/*.log'], 'format': 'json', 'start_at': 'beginning'}),
            'stack': ('B08 Application Multiline', 'filelog', self.app, 'application', {
                'scope': 'business', 'include': [DIR + '/stack/*.log'], 'multiline_start': '^ERROR ', 'start_at': 'beginning'}),
            'journal': ('B08 Host Journald', 'journald', self.host, 'host', {
                'scope': 'system', 'units': [SERVICE], 'start_at': 'beginning'}),
        }
        for key, (name, integration, ident, kind, config) in specs.items():
            self.converge()
            state = self.map['log_rules'].setdefault(key, {})
            if 'id' not in state:
                body = {'name': name, 'integration': integration, 'target': {'kind': kind, 'id': ident},
                        'collector_id': self.collector, 'enabled': True, 'credential_update': 'keep', 'config': config}
                if kind == 'application':
                    body.update(application_id=ident, service_id=self.map['application']['service_id'])
                state.update(id=self.command({'kind': 'collection', 'operation': 'create', 'collection': body},
                                              'rule-'+key), name=name, resource=ident)
                self.save()
            self.effective(key, True)
            print(name + ' effective', flush=True)

    def collection(self, key):
        return self.api.call('GET','/monitoring/collections/'+self.map['log_rules'][key]['id'])

    def effective(self, key, enabled):
        def done(row):
            if row['apply_status'] == 'failed':
                raise RuntimeError(f"{key}: {row['error_code']}")
            return (row['desired_enabled'] == enabled and row['apply_status'] == 'effective'
                    and row['effective_revision'] == row['desired_revision'])
        return self.api.wait('/monitoring/collections/' + self.map['log_rules'][key]['id'], done,
                             timeout=150, label=key+' rule effective')

    def enable(self, key, enabled):
        row = self.collection(key)
        if row['desired_enabled'] != enabled:
            self.converge()
            self.command({'kind': 'collection', 'operation': 'set_enabled', 'id': row['id'],
                          'expected_version': row['version'], 'collection': {'enabled': enabled}},
                         f'{key}-{enabled}-{row["version"]}')
        return self.effective(key, enabled)

    def query(self, marker, resource):
        data = self.api.call('POST','/enterprise/logs/query',{
            **query_scope(resource), 'query':'body : '+json.dumps(marker)})
        if data.get('partial') or data.get('status') in ('failed','partial','error'):
            raise RuntimeError('Log query incomplete')
        return data['data']

    def wait_records(self, marker, resource, count=1, timeout=45):
        deadline = time.monotonic()+timeout
        while True:
            rows = self.query(marker,resource)
            if len(rows) == count:
                assert all(row['resource_id']==resource and marker in str(row['body']) for row in rows), rows
                return rows
            if len(rows) > count:
                raise RuntimeError(f'{marker}: duplicate or unexpected records ({len(rows)} vs {count})')
            if time.monotonic() >= deadline:
                raise RuntimeError(f'{marker}: only {len(rows)} of {count} log records arrived')
            time.sleep(2)

    def checked_request(self, label, stream='rename', error=False):
        receipt = self.run.request(label,stream,error)
        assert receipt['status'] == (503 if error else 200)
        rows = self.wait_records(receipt['message'], self.app, 2 if error else 1)
        def decoded(body):
            if isinstance(body, dict):
                return body
            try:
                return json.loads(body)
            except (TypeError, ValueError):
                return None
        semantic = [item for row in rows if isinstance((item:=decoded(row['body'])),dict)]
        assert len(semantic)==1
        assert all(semantic[0][field]==receipt[field] for field in ('message','sequence','run_id','timestamp','status'))
        if error:
            assert any('at orders.validate_request' in str(row['body']) and
                       'caused by controlled dependency refusal' in str(row['body']) for row in rows)
        journal_marker = receipt['message']+'-journal'
        journal = self.wait_records(journal_marker,self.host)
        assert not self.query(journal_marker,self.app)
        assert all(row['source_id'] not in {x['source_id'] for x in journal} for row in rows)
        proof = {'http':receipt,'file_records':rows,'journal_records':journal}
        write_json(self.run.out/(label+'.json'), proof)
        print(label + ': actual HTTP, file and journal records received', flush=True)
        return receipt
