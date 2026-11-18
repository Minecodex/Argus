"""Publish real-data boards and execute one scope resolver across both apps."""
import copy
from datetime import datetime, timedelta, timezone
import json
from manage import ROOT, Run
from common.monitoring import MonitoringBatch


class HostView:
    def __init__(self, batch, key):
        self.batch, self.key, self.api, self.map, self.run = batch, key, batch.api, batch.map, self

    @property
    def host(self):
        return self.map['hosts'][self.key]['host_id']

    def save(self, *args):
        if args:
            self.batch.run.save(self.key + '-' + args[0], args[1])
        else:
            self.batch.save()


def defaults(batch):
    from B11a.monitoring import dashboards as host_defaults
    for key in ['orders', 'reports']:
        host_defaults(HostView(batch, key))
    monitoring = MonitoringBatch(batch.run, batch.api, ['redis', 'memcached'])
    monitoring.dashboards()
    batch.map = monitoring.map
    batch.save()


def specification():
    templates = json.loads((ROOT / 'api/contracts/dashboard-templates.json').read_text(encoding='utf-8'))
    spec = copy.deepcopy(templates[0]['spec'])
    template = spec['panels'][0]
    spec.update(default_time_range={'kind': 'relative', 'seconds': 900}, default_refresh_seconds=30, variables=[], panels=[])
    metrics = [
        ('cpu', '主机 CPU / Host CPU', 'host', 'hostmetrics', '1 - avg by (argus_resource_id) (system_cpu_utilization{state="idle"})', 'percent_ratio', 'timeseries'),
        ('memory', '主机内存 / Host memory', 'host', 'hostmetrics', 'system_memory_usage{state="$state"}', 'bytes', 'timeseries'),
        ('redis-memory', '共享 Redis 内存 / Shared Redis memory', 'monitoring_target', 'redis', 'redis_memory_used', 'bytes', 'timeseries'),
        ('redis-commands', '共享 Redis 命令 / Shared Redis commands', 'monitoring_target', 'redis', 'rate(redis_commands_processed[1m])', 'ops/s', 'timeseries'),
        ('memcached-memory', 'Reports 缓存 / Reports cache', 'monitoring_target', 'memcached', 'memcached_bytes', 'bytes', 'timeseries'),
        ('redis-targets', '唯一 Redis 目标 / Unique Redis targets', 'monitoring_target', 'redis', 'count(redis_uptime)', '', 'stat'),
    ]
    for i, (key, title, kind, source, expression, unit, chart) in enumerate(metrics):
        panel = copy.deepcopy(template)
        panel.update(id=key, title=title, description='Actual owned sources; shared Redis is not attributable exclusively to either application.',
                     type=chart, unit=unit, applicable_resource_types=[kind], source_binding={'source_type': source, 'capability_version': 'v1'},
                     layout={'x': i % 2 * 6, 'y': i // 2 * 36, 'w': 6, 'h': 36, 'min_w': 2, 'min_h': 20})
        panel['targets'][0].update(source_definition={'dsl': {'expression': expression}}, query_mode='instant' if chart == 'stat' else 'range')
        if key == 'memory':
            panel['local_filters'] = [{'id': 'state', 'label': '内存状态 / Memory state', 'kind': 'text', 'default': {'all': False, 'values': ['used']}}]
            panel['targets'][0]['parameter_bindings'] = [{'parameter': 'state', 'local_parameter': 'state'}]
        spec['panels'].append(panel)
    return spec


def publish(batch):
    api = batch.api
    name = batch.NAME
    state = batch.map.setdefault('batch_dashboard', {})
    bindings = [{'resource_type': 'application', 'resource_id': app['id']} for app in batch.map['applications'].values()]
    spec = specification()
    validation = api.call('POST', '/dashboard-spec/validate', spec)
    batch.run.save('dashboard-static-validation', validation)
    assert validation['valid'], validation
    if 'draft_id' not in state:
        draft = api.call('POST', '/dashboard-drafts', {'name': name + ' 范围与共享依赖 / Scope and shared dependencies',
                         'description': 'Orders production + Reports staging; actual Host/Redis/Memcached. Shared Redis counted once.',
                         'spec': spec, 'proposed_bindings': bindings}, key='combined-board', expected=201)
        state['draft_id'] = draft['id']
        batch.save()
    if 'id' not in state:
        draft = api.call('GET', '/dashboard-drafts/' + state['draft_id'])
        if not draft.get('dashboard_id'):
            api.confirm(api.call('POST', '/dashboard-drafts/' + state['draft_id'] + '/preview', {'expected_version': draft['draft_version']}, key='publish-combined', expected=201))
            draft = api.call('GET', '/dashboard-drafts/' + state['draft_id'])
        state.update(id=draft['dashboard_id'], name=draft['name'])
        batch.save()
    # A managed-board edit creates a personal draft; it must never mutate the template.
    personal = batch.map.setdefault('personal_dashboard', {})
    original_id = batch.map['dashboards']['redis']['id']
    detail = api.call('GET', '/dashboards/' + original_id)
    original = {**detail['dashboard'], 'spec': detail['revision']['spec']}
    if 'draft_id' in personal and 'id' not in personal:
        previous = api.call('GET', '/dashboard-drafts/' + personal['draft_id'])
        if not api.call('POST', '/dashboard-spec/validate', previous['spec'])['valid']:
            # This batch's failed draft contains only this harness's input.
            # Preserve the regression evidence, discard it through the normal
            # draft API, then test a fresh copy of the newly fixed template.
            assert previous['name'] == name + ' Redis 个人副本 / Personal copy'
            batch.run.save('invalid-personal-draft-before-fix', previous)
            api.call('DELETE', '/dashboard-drafts/' + previous['id'], {'expected_version': previous['draft_version']}, expected=204)
            personal.clear()
            personal['discarded_invalid_draft_id'] = previous['id']
            batch.save()
    if 'draft_id' not in personal:
        draft = api.call('POST', '/dashboard-drafts', {'dashboard_id': original_id, 'name': original['name'],
                         'description': original['description'], 'spec': original['spec'], 'proposed_bindings': []}, key='managed-personal-copy', expected=201)
        assert not draft.get('dashboard_id') and draft['source_template_key'] == 'redis.overview'
        personal.update(draft_id=draft['id'], source_id=original_id, source_revision=original['active_revision_id'])
        batch.save()
    if 'id' not in personal:
        draft = api.call('GET', '/dashboard-drafts/' + personal['draft_id'])
        if not draft.get('dashboard_id'):
            edited = copy.deepcopy(draft['spec'])
            edited['panels'][0]['title'] = name + ' Redis 运行时长 / Redis uptime'
            draft = api.call('PATCH', '/dashboard-drafts/' + draft['id'], {'name': name + ' Redis 个人副本 / Personal copy',
                             'description': 'Independent editable copy of managed Redis overview; native current scope.', 'spec': edited,
                             'proposed_bindings': [{'resource_type': 'monitoring_target', 'resource_id': batch.map['targets']['redis']['target_id']}],
                             'expected_version': draft['draft_version']})
            end = datetime.now(timezone.utc)
            parameters = {'from': (end - timedelta(minutes=15)).isoformat(), 'to': end.isoformat(),
                          'resource_mode': 'explicit', 'resource_types': ['monitoring_target'],
                          'resource_ids': [batch.map['targets']['redis']['target_id']]}
            api.confirm(api.call('POST', '/dashboard-drafts/' + draft['id'] + '/preview', {'expected_version': draft['draft_version'],
                            'parameters': parameters}, key='publish-personal', expected=201))
            draft = api.call('GET', '/dashboard-drafts/' + draft['id'])
        personal.update(id=draft['dashboard_id'], name=draft['name'])
        batch.save()
    assert api.call('GET', '/dashboards/' + original_id)['dashboard']['active_revision_id'] == personal['source_revision']
    batch.run.save('personal-copy-check', {'source_id': original_id, 'source_revision': personal['source_revision'], 'copy_id': personal['id'],
                                          'independent_identity': personal['id'] != original_id, 'template_unchanged': True, 'edited_and_published': True})
    print('Combined scope board and edited managed-template personal copy published normally', flush=True)


def execute(batch, api=None, name='combined', **filters):
    end = datetime.now(timezone.utc)
    body = {'from': (end - timedelta(minutes=15)).isoformat(), 'to': end.isoformat(), 'resource_mode': 'all_authorized',
            'resource_types': ['host', 'monitoring_target'], 'application_mode': 'explicit',
            'application_ids': [x['id'] for x in batch.map['applications'].values()], **filters}
    result = (api or batch.api).call('POST', '/dashboards/' + batch.map['batch_dashboard']['id'] + '/execute', body)
    result.pop('context_token', None)
    batch.run.save(name + '-execution', {'input': body, 'result': result})
    return result


if __name__ == '__main__':
    from integration import Batch
    import argparse
    p = argparse.ArgumentParser()
    p.add_argument('action', choices=['defaults', 'publish', 'execute'])
    run = Run()
    api = run.api()
    api.login()
    globals()[p.parse_args().action](Batch(run, api))
