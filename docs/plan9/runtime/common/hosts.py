"""Shared normal Host/Connector/Collector enrollment for Plan9 batches."""
import json

def write_json(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')

class HostEnrollment:
    def __init__(self, run, api, host_specs, batch):
        self.run, self.api = run, api
        self.host_specs, self.batch = host_specs, batch
        self.map = json.loads(run.resources.read_text(encoding='utf-8'))

    def save(self):
        write_json(self.run.resources, self.map)

    def onboard(self):
        private = json.loads((self.run.out / 'private/host-credentials.json').read_text(encoding='utf-8'))
        for host, metadata in self.host_specs.items():
            letter, port = metadata['letter'], metadata['ssh_port']
            state = self.map['hosts'].setdefault(host, {'name': metadata['name']})
            if state.get('create_action_ref') and not state.get('host_id'):
                matches = [x for x in self.api.call('GET', '/enterprise/hosts')['items']
                           if x['name'] == state['name'] and x.get('labels', {}).get('plan9_run') == self.run.id]
                if matches:
                    assert len(matches) == 1
                    previous = matches[0]
                    assert previous['onboarding']['pending_action_ref'] == state['create_action_ref']
                    self.api.wait('/enterprise/executions/' + previous['onboarding']['execution_id'],
                                  lambda x: x['status'] == 'succeeded', label='Prior installation reconciliation')
                    state['host_id'] = previous['id']
                    self.save()
            if not state.get('secret_id'):
                secret = self.api.call('POST', '/enterprise/secrets', {
                    'name': self.run.id + '-' + letter + '-ssh', 'type': 'ssh_password',
                    'value': private[host]['password']}, key='secret-' + host, expected=201)
                state['secret_id'] = secret['id']
                self.save()
            if not state.get('credential_id'):
                credential = self.api.call('POST', '/enterprise/credentials', {
                    'name': self.run.id + '-' + letter + '-ssh', 'protocol': 'ssh', 'username': 'root',
                    'secret_id': state['secret_id']}, key='credential-' + host, expected=201)
                state['credential_id'] = credential['id']
                self.save()
            if not state.get('host_id'):
                connection = self.api.call('POST', '/enterprise/hosts/connection-tests', {
                    'address': 'host.docker.internal', 'port': port, 'platform': 'linux',
                    'ssh_path': 'direct_executor', 'onboarding_control_path': 'executor_tunnel',
                    'credential_id': state['credential_id'], 'username': 'root'},
                    key='connection-' + host, expected=202)
                state['connection_test_id'] = connection['id']
                self.save()
                def successful(value):
                    if value['status'] in ('failed', 'result_unknown', 'expired'):
                        write_json(self.run.out / (host + '-connection-failed.json'), value)
                        raise RuntimeError(host + ' connection test: ' + str(value.get('error_code')))
                    return value['status'] == 'succeeded'
                connection = self.api.wait('/enterprise/connection-tests/' + connection['id'], successful,
                                           label='SSH and callback preflight')
                write_json(self.run.out / (host + '-connection.json'), connection)
                preview = self.api.call('POST', '/enterprise/hosts/actions/preview-create', {
                    'name': state['name'], 'address': 'host.docker.internal', 'port': port,
                    'platform': 'linux', 'role': 'managed_host', 'control_path': 'executor_tunnel',
                    'install_method': 'ssh', 'ssh_path': 'direct_executor',
                    'credential_id': state['credential_id'], 'username': 'root',
                    'connection_test_id': state['connection_test_id'], 'environment': 'development',
                    'labels': {'plan9_run': self.run.id, 'plan9_batch': self.batch, 'node': letter.lower()}},
                    key='host-' + host, expected=201)
                state['create_action_ref'] = preview['action_ref']
                self.save()
                result = self.api.confirm(preview)
                reference = result.get('resource_ref', {})
                hid = reference.get('resource_id')
                if not hid:
                    matches = [x for x in self.api.call('GET', '/enterprise/hosts')['items']
                               if x['name'] == state['name'] and x.get('labels', {}).get('plan9_run') == self.run.id]
                    assert len(matches) == 1
                    hid = matches[0]['id']
                state['host_id'] = hid
                self.save()
            current = self.api.wait('/enterprise/hosts/' + state['host_id'],
                                    lambda x: x.get('connection_status') == 'online', label='Host registration')
            write_json(self.run.out / (host + '-online.json'), current)
            print(host + ': normal SSH installation online', flush=True)
        return {'online_hosts': len(self.map['hosts'])}

    def collectors(self):
        distributions = self.api.call('GET', '/enterprise/telemetry/distributions')
        distribution = next(x for x in distributions if x['support_status'] == 'supported' and
                            any(a['platform'] == 'linux_amd64' for a in x['artifacts']))
        for host, state in self.map['hosts'].items():
            hid = state['host_id']
            base = '/enterprise/hosts/' + hid + '/collector'
            current = self.api.call('GET', base, expected=[200, 204, 404])
            if not current or current.get('code'):
                preview = self.api.call('POST', base + '/actions/preview-install', {
                    'distribution_version_id': distribution['id'], 'profile_ids': [],
                    'route_kind': 'direct_argus', 'transport': 'executor_tunnel'},
                    key='collector-install-' + host, expected=201)
                state['collector_install_action_ref'] = preview['action_ref']
                self.save()
                self.api.confirm(preview)
            current = self.api.wait(base, lambda x: x.get('status') in ('converged', 'ready'),
                                    timeout=360, label='Collector install')
            state['collector_id'] = current.get('id') or current.get('collector_id')
            write_json(self.run.out / (host + '-collector.json'), current)
            self.save()
            print(host + ': Collector installed', flush=True)
        return {'collector_hosts': len(self.map['hosts'])}

    def builtins(self, enabled=True, selected=None):
        selected = selected or ['host_metrics', 'host_process_metrics', 'host_systemd_metrics', 'collector_metrics']
        for host, state in self.map['hosts'].items():
            hid = state['host_id']
            endpoint = '/monitoring/builtins?target_type=host&target_id=' + hid
            for key in selected:
                rows = self.api.call('GET', endpoint)
                item = next(x for x in rows if x['definition']['key'] == key)
                if not item['available']:
                    raise RuntimeError(host + ': unavailable builtin ' + key + ': ' + item.get('reason', ''))
                collection = item.get('collection') or {}
                if bool(collection.get('desired_enabled', False)) == enabled and collection:
                    continue
                preview = self.api.call('POST', '/monitoring/builtins/preview', {
                    'target': {'kind': 'host', 'id': hid}, 'key': key, 'enabled': enabled,
                    'expected_version': collection.get('version', 0), 'config': {'interval': '10s'}},
                    key=f'{host}-{key}-{enabled}-{collection.get("version",0)}', expected=201)
                self.api.confirm(preview)
            state['builtins'] = self.api.call('GET', endpoint)
            self.save()
            print(host + ': fixed metrics configured', flush=True)
        return {'configured_hosts': len(self.map['hosts']), 'keys': selected}
