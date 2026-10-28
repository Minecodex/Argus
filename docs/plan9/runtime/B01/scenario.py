"""Actual dashboards, scoped alert evaluator, faults and evidence for B01."""
from datetime import datetime, timedelta, timezone
import json
import time

from manage import write_json


class Scenario:
    def __init__(self, run, api):
        self.run, self.api = run, api
        self.map = json.loads(run.resources.read_text(encoding='utf-8'))

    def save(self):
        write_json(self.run.resources, self.map)

    def query(self, host, expression):
        now = datetime.now(timezone.utc)
        return self.api.call('POST', '/enterprise/metrics/query', {
            'query': expression, 'resource_ids': [self.map['hosts'][host]['host_id']],
            'time_range': {'from': (now - timedelta(minutes=5)).isoformat(), 'to': now.isoformat()},
            'budget': {'max_scan_bytes': 268435456, 'max_rows': 1000, 'max_samples': 5000000,
                       'max_series': 100000, 'timeout_ms': 10000, 'max_result_bytes': 8388608}})

    def dashboards(self):
        wanted = ['host.overview', 'host.processes', 'host.systemd', 'collector.overview']
        boards = self.api.call('GET', '/dashboards')
        selected = {x['managed_template_key']: x for x in boards if x.get('managed_template_key') in wanted}
        assert set(selected) == set(wanted), 'required managed dashboards not discoverable'
        self.map['dashboards'] = {key: {'id': value['id'], 'name': value['name'],
                                       'template_version': value.get('managed_template_version')}
                                  for key, value in selected.items()}
        self.save()
        reports, issues = [], []
        now = datetime.now(timezone.utc)
        for host, state in self.map['hosts'].items():
            hid = state['host_id']
            for key, board in selected.items():
                bindings = self.api.call('GET', '/hosts/' + hid + '/dashboard-bindings')
                if not any(x['dashboard']['id'] == board['id'] for x in bindings['items']):
                    preview = self.api.call('POST', '/hosts/' + hid + '/dashboard-bindings/preview', {
                        'operation': 'attach', 'dashboard_id': board['id'],
                        'expected_dashboard_version': board['version'],
                        'expected_resource_version': bindings['resource_version']},
                        key='bind-' + host + '-' + key, expected=201)
                    self.api.confirm(preview)
                execution = self.api.call('POST', '/dashboards/' + board['id'] + '/execute', {
                    'from': (now - timedelta(minutes=15)).isoformat(), 'to': now.isoformat(),
                    'resource_mode': 'explicit', 'resource_types': ['host'], 'resource_ids': [hid]})
                evidence = {k: v for k, v in execution.items() if k != 'context_token'}
                write_json(self.run.out / (host + '-' + key + '-execution.json'), evidence)
                panels = execution['panels']
                reports.append({'host': host, 'template': key, 'dashboard_id': board['id'],
                                'panels': [{'id': x['id'], 'status': x['status']} for x in panels]})
                if any(x['status'] not in ('success', 'empty', 'no_data') for x in panels):
                    issues.append({'host': host, 'template': key,
                                   'panels': [{'id': x['id'], 'status': x['status'],
                                               'codes': [t.get('code') for t in x['targets'] if t.get('code')]}
                                              for x in panels if x['status'] != 'success']})
            print(host + ': dashboards queried and linked', flush=True)
        write_json(self.run.out / 'dashboard-query-checks.json', reports)
        write_json(self.run.out / 'dashboard-known-issues.json', issues)
        # Check the actual native resource labels keep process/service series distinct.
        for key, expression, label in [
            ('process-groups', 'sum by (resource_process_executable_name) (process_memory_usage)', 'resource_process_executable_name'),
            ('service-groups', 'sum by (resource_systemd_unit_name) (rate(systemd_service_cpu_time[5m])) / 1000000', 'resource_systemd_unit_name')]:
            result = self.query('host-a', expression)
            rows = result['data']['result']
            assert len(rows) > 1 and all(x['metric'].get(label) for x in rows), key
            write_json(self.run.out / (key + '-verified.json'), result)
        return {'dashboards': len(selected), 'resource_executions': len(reports),
                'all_panels_complete': not issues, 'known_issue_views': len(issues)}

    def config(self, kind, name, spec):
        rows = self.api.call('GET', '/alerts/configurations?kind=' + kind)
        matches = [x for x in rows if x['name'] == name]
        if matches:
            assert len(matches) == 1
            return matches[0]
        preview = self.api.call('POST', '/alerts/configurations/preview', {
            'kind': kind, 'operation': 'create', 'name': name, 'spec': spec},
            key='alert-' + kind + '-' + name, expected=202)
        self.api.confirm(preview)
        rows = self.api.call('GET', '/alerts/configurations?kind=' + kind)
        return next(x for x in rows if x['name'] == name)

    def alerts(self):
        evaluator = self.map.setdefault('evaluator', {})
        if not evaluator.get('service_account_id'):
            row = self.api.call('POST', '/enterprise/service-accounts', {
                'name': 'B01 metric evaluator', 'description': 'Only B01 test hosts; no tools or API key',
                'allowed_tool_ids': []}, key='evaluator', expected=201)
            evaluator['service_account_id'] = row['id']
            self.save()
        if not evaluator.get('role_id'):
            row = self.api.call('POST', '/enterprise/roles', {
                'name': 'B01 metric reader', 'description': 'Read only B01 host observations',
                'permissions': ['host.read']}, key='evaluator-role', expected=201)
            evaluator['role_id'] = row['id']
            self.save()
        if not evaluator.get('role_binding_id'):
            row = self.api.call('POST', '/enterprise/role-bindings', {
                'subject_type': 'service_account', 'subject_id': evaluator['service_account_id'],
                'role_id': evaluator['role_id']}, key='evaluator-binding', expected=201)
            evaluator['role_binding_id'] = row['id']
            self.save()
        grants = '/enterprise/data-authorizations/role/' + evaluator['role_id']
        current = self.api.call('GET', grants + '?resource_type=host')
        host_ids = [x['host_id'] for x in self.map['hosts'].values()]
        missing = [hid for hid in host_ids if not any(x['resource_id'] == hid and x['direct'] for x in current['items'])]
        if missing:
            self.api.call('POST', grants, {'resource_type': 'host', 'resource_ids': missing,
                                          'remove': False, 'expected_version': current['authorization_version']}, expected=204)
        row = self.config('settings', 'B01 background evaluator', {'evaluator_id': evaluator['service_account_id']})
        evaluator['settings_id'] = row['id']
        self.save()
        service = 'max by (argus_resource_id) (systemd_unit_state{resource_systemd_unit_name="plan9-web.service",systemd_unit_active_state="failed"})'
        cpu = '100 * (1 - avg by (argus_resource_id) (system_cpu_utilization{state="idle"}))'
        observations = []
        for _ in range(3):
            time.sleep(8)
            baseline = self.query('host-a', cpu)
            values = baseline['data']['result']
            assert len(values) == 1
            observations.append(float(values[0]['value'][1]))
        baseline_cpu = sorted(observations)[1]
        assert 0 <= baseline_cpu < 60, 'host too busy for bounded CPU test'
        threshold = round(baseline_cpu + 2, 2)
        write_json(self.run.out / 'cpu-baseline-before-policy.json', baseline)
        write_json(self.run.out / 'cpu-baseline-observations.json', observations)
        definitions = [
            ('service', 'B01 test service failed', service, 0.5, 0, '', 'warning'),
            ('cpu', 'B01 bounded CPU load', cpu, threshold, round(baseline_cpu + 1, 2), '%', 'warning')]
        self.map['alert_policies'] = self.map.get('alert_policies', {})
        for key, name, expression, trigger, recovery, unit, severity in definitions:
            spec = {'description': 'Plan9 B01 test policy; thresholds fixed before real fault/load',
                    'enabled': True, 'resources': {'mode': 'explicit', 'types': ['host'],
                                                  'ids': [self.map['hosts']['host-a']['host_id']], 'matchers': []},
                    'rules': [{'name': name, 'expression': expression, 'query_mode': 'numeric',
                               'operator': '>', 'threshold': trigger, 'recovery_threshold': recovery,
                               'severity': severity, 'unit': unit}],
                    'mode': 'independent', 'match_by': ['argus_resource_id'], 'interval_seconds': 10,
                    'for_seconds': 20, 'recovery_seconds': 20, 'no_data_behavior': 'report',
                    'error_behavior': 'keep', 'channel_ids': [], 'route_ids': [],
                    'labels': {'plan9_batch': 'b01', 'plan9_run': self.run.id}}
            trial = self.api.call('POST', '/alerts/trial', spec)
            write_json(self.run.out / (key + '-alert-trial.json'), trial)
            assert trial['health'] == 'normal', key + ': evaluation trial is not healthy'
            row = self.config('policy', name, spec)
            self.map['alert_policies'][key] = {'id': row['id'], 'name': name,
                                             'expression': expression, 'threshold': trigger,
                                             'recovery_threshold': recovery}
            self.save()
        self.run.event('alert_policies_created_before_faults', baseline_cpu=baseline_cpu,
                       cpu_threshold=threshold, service_threshold=0.5)
        return {'evaluator_host_grants': 2, 'policies': 2, 'external_channels': 0}

    def instances(self, key):
        return self.api.call('GET', '/alerts/instances?policy_id=' + self.map['alert_policies'][key]['id'])

    def wait_alert(self, key, lifecycle, timeout=240, instance_ids=None):
        endpoint = '/alerts/history' if lifecycle == 'resolved' else '/alerts/instances'
        path = endpoint + '?policy_id=' + self.map['alert_policies'][key]['id']
        return self.api.wait(path, lambda v: any(x['lifecycle'] == lifecycle and x.get('health') == 'normal'
                                                and (not instance_ids or x['id'] in instance_ids)
                                                for x in v['items']), timeout=timeout, label=key + ' alert ' + lifecycle)

    def exercise(self):
        prior = self.run.out / 'service-firing.json'
        if prior.exists():
            failed = json.loads(prior.read_text(encoding='utf-8'))
        else:
            self.run.fault('host-a', False)
            failed = self.wait_alert('service', 'firing')
            write_json(prior, failed)
            self.run.fault('host-a', True)
        ids = {x['id'] for x in failed['items'] if x['lifecycle'] == 'firing'}
        recovered = self.wait_alert('service', 'resolved', instance_ids=ids)
        write_json(self.run.out / 'service-recovered.json', recovered)
        self.run.workload('host-a', 'cpu')
        loaded = self.wait_alert('cpu', 'firing', timeout=150)
        write_json(self.run.out / 'cpu-firing.json', loaded)
        ids = {x['id'] for x in loaded['items'] if x['lifecycle'] == 'firing'}
        recovered = self.wait_alert('cpu', 'resolved', timeout=240, instance_ids=ids)
        write_json(self.run.out / 'cpu-recovered.json', recovered)
        for mode in ('memory', 'disk', 'network'):
            self.run.workload('host-a', mode, seconds=60)
            print(mode + ': real bounded workload started', flush=True)
            for i in range(13):
                time.sleep(5)
                if i == 6:
                    query = {'memory': 'max(process_memory_usage{resource_process_executable_name="plan9-workload"})',
                             'disk': 'sum(rate(process_disk_io[1m]))',
                             'network': 'sum by (direction) (rate(system_network_io[1m]))'}[mode]
                    write_json(self.run.out / (mode + '-during-load.json'), self.query('host-a', query))
                    native = self.run.shell('host-a',
                        'ps -eo pid,comm,rss | head -25\n'
                        'stat -c %s /var/lib/plan9/bounded-64m.bin 2>/dev/null || true\n'
                        'cat /sys/class/net/eth0/statistics/rx_bytes /sys/class/net/eth0/statistics/tx_bytes\n')
                    (self.run.out / (mode + '-native-observation.txt')).write_text(native, encoding='utf-8')
            status = self.run.shell('host-a', 'systemctl is-active plan9-web\n')
            assert status == 'active'
            self.run.event('bounded_load_completed', mode=mode)
        return {'service_and_cpu_alerts_triggered_and_recovered': True,
                'bounded_real_loads': ['cpu', 'memory', 'disk', 'network']}

    def builtin_rows(self, host):
        return self.api.call('GET', '/monitoring/builtins?target_type=host&target_id=' +
                             self.map['hosts'][host]['host_id'])

    def change_builtin(self, host, key, enabled):
        item = next(x for x in self.builtin_rows(host) if x['definition']['key'] == key)
        current = item['collection']
        if current['desired_enabled'] == enabled:
            return
        preview = self.api.call('POST', '/monitoring/builtins/preview', {
            'target': {'kind': 'host', 'id': self.map['hosts'][host]['host_id']},
            'key': key, 'enabled': enabled, 'expected_version': current['version'],
            'config': current['config']}, key=f'lifecycle-{host}-{key}-{current["version"]}-{enabled}', expected=201)
        self.api.confirm(preview)
        self.run.event('builtin_changed', host=host, key=key, enabled=enabled)

    def lifecycle(self):
        keys = ['host_metrics', 'host_process_metrics', 'host_systemd_metrics', 'collector_metrics']
        path = '/monitoring/builtins?target_type=host&target_id=' + self.map['hosts']['host-a']['host_id']
        def applied(rows, expected):
            selected = [x['collection'] for x in rows if x['definition']['key'] in keys]
            return all(x['desired_enabled'] == expected[x['integration']] and
                       x['apply_status'] == 'effective' and x['desired_revision'] == x['effective_revision']
                       for x in selected)
        active = dict.fromkeys(keys, True)
        self.change_builtin('host-a', 'host_metrics', False)
        stopped = {**active, 'host_metrics': False}
        self.api.wait(path, lambda rows: applied(rows, stopped), label='Single builtin stop')
        time.sleep(25)
        write_json(self.run.out / 'single-stop-host-a.json', self.builtin_rows('host-a'))
        peer = self.builtin_rows('host-b')
        assert all(x['collection']['freshness'] == 'fresh' for x in peer if x.get('collection'))
        write_json(self.run.out / 'single-stop-host-b-unaffected.json', peer)
        self.change_builtin('host-a', 'host_metrics', True)
        self.api.wait(path, lambda rows: applied(rows, active), label='Single builtin restore')
        for key in keys:
            self.change_builtin('host-a', key, False)
        self.api.wait(path, lambda rows: applied(rows, dict.fromkeys(keys, False)), label='All metrics stop')
        time.sleep(25)
        control = self.api.call('GET', '/enterprise/hosts/' + self.map['hosts']['host-a']['host_id'])
        assert control['connection_status'] == 'online'
        service = self.run.shell('host-a', 'systemctl is-active argus-connector argus-otelcol\n')
        assert service.splitlines() == ['active', 'active']
        write_json(self.run.out / 'all-metrics-off-control-online.json', {
            'host_id': control['id'], 'connection_status': control['connection_status'],
            'services': service.splitlines(), 'builtins': self.builtin_rows('host-a'),
            'service_alerts': self.instances('service'), 'cpu_alerts': self.instances('cpu')})
        for key in keys:
            self.change_builtin('host-a', key, True)
        def fresh(rows):
            return applied(rows, active) and all(x['collection']['freshness'] == 'fresh' and
                x['collection']['health'] == 'healthy' for x in rows if x['definition']['key'] in keys)
        self.api.wait(path, fresh, timeout=180, label='All metrics restore')
        write_json(self.run.out / 'all-metrics-restored.json', self.builtin_rows('host-a'))
        return {'single_stop_peer_unaffected': True, 'all_off_control_online': True,
                'all_four_restored_with_fresh_data': True}

    def retain(self):
        self.run.fault('host-a', False)
        value = self.wait_alert('service', 'firing')
        write_json(self.run.out / 'review-current-service-firing.json', value)
        self.run.event('review_fault_retained', host='host-a', service='plan9-web.service',
                       policy_id=self.map['alert_policies']['service']['id'])
        return {'review_fault': 'host-a plan9-web failed; no resource pressure', 'environment_retained': True}
