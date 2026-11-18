"""Bounded real Collector delivery observation, preserving lifetime counters."""
import json
import time


def watch(batch, seconds=120):
    run, api = batch.run, batch.api
    from B01.manage import now
    active_hosts = [key for key in batch.map['hosts'] if key not in batch.map.get('retired_hosts', [])]
    assert active_hosts
    def reports():
        return {key: json.loads(run.shell(key, 'cat /var/lib/argus-otelcol/identity/delivery-state.json\n'))['report'] for key in active_hosts}
    before = reports()
    run.save('delivery-watch-baseline', {'at': now(), 'reports': before})
    snapshots = [before]
    started = now()
    assert seconds % 30 == 0
    for index in range(seconds // 30):
        time.sleep(30)
        current = reports()
        for key, row in current.items():
            assert row['instance_id'] == before[key]['instance_id'] and row['sequence'] > before[key]['sequence']
            for field in ['terminal_failures', 'receiver_refusals', 'enqueue_failures']:
                assert row[field] == before[key][field], (key, field, before[key][field], row[field])
            data = api.call('GET', '/monitoring/builtins?target_type=host&target_id=' + batch.map['hosts'][key]['host_id'])
            active = [x['collection'] for x in data if (x.get('collection') or {}).get('desired_enabled')]
            assert len(active) == 4 and all(x['health'] == 'healthy' and x['freshness'] == 'fresh' for x in active)
        for target in batch.map.get('targets', {}).values():
            row = api.call('GET', '/monitoring/collections/' + target['collection_id'])
            if row['status'] == 'active':
                assert row['desired_enabled'] and row['health'] == 'healthy' and row['freshness'] == 'fresh'
        snapshots.append(current)
        print('Active Collector deliveries stable: ' + str(index + 1) + '/' + str(seconds // 30), flush=True)
    run.save('delivery-watch', {'passed': True, 'started_at': started, 'ended_at': now(), 'seconds': seconds, 'observations': snapshots,
                               'terminal_failure_increase': 0, 'receiver_refusal_increase': 0, 'enqueue_failure_increase': 0,
                               'historical_baseline_retained': before, 'active_hosts': active_hosts, 'counter_reset': False})
