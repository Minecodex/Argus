"""Guarded real faults and bounded normal-API observations shared by B13b cases."""
from contextlib import contextmanager
from datetime import datetime
import json
import time
from manage import now

CORE = ['independent', 'every', 'any']
SERVICE = 'plan9-b13b.service'


def instant(value):
    return datetime.fromisoformat(value.replace('Z', '+00:00'))


def instances(batch, keys=None, history=False):
    response = batch.api.call('GET', '/alerts/' + ('history' if history else 'instances') + '?limit=200')
    assert not response.get('next_cursor'), 'Do not truncate batch alert evidence'
    ids = {batch.map['alert_policies'][key]['id'] for key in (keys or batch.map['alert_policies'])}
    rows = [x for x in response['items'] if x['policy_id'] in ids]
    owned = {x['host_id'] for x in batch.map['hosts'].values()}
    assert all(set(x['resource_ids']) <= owned for x in rows)
    return rows


def condition(rows, states=('pending', 'firing', 'recovering')):
    return [x for x in rows if x['state']['kind'] == 'condition' and x['lifecycle'] in states]


def wait_instances(batch, predicate, keys=None, timeout=110, history=False, name='episodes'):
    deadline = time.monotonic() + timeout
    latest = []
    while time.monotonic() < deadline:
        latest = instances(batch, keys, history)
        if predicate(latest):
            batch.run.save(name, {'at': now(), 'items': latest})
            return latest
        time.sleep(2)
    batch.run.save(name + '-timeout-' + str(int(time.time())), {'at': now(), 'items': latest})
    raise RuntimeError(name + ' did not reach its bounded real state')


def receipts(batch):
    text = batch.run.shell('reports', 'if [ -f /var/lib/plan9/mail/receipts.jsonl ]; then cat /var/lib/plan9/mail/receipts.jsonl; fi\n')
    return [json.loads(line) for line in text.splitlines() if line]


def mailbox_mode(batch, mode):
    assert mode in ['accept', 'temporary_once', 'permanent']
    value = json.dumps({'mode': mode, 'remaining': 1})
    batch.run.shell('reports', "install -d -m 700 /var/lib/plan9/mail\nprintf '%s\\n' '" + value + "' > /var/lib/plan9/mail/mode.json\n")


def recover(batch, hosts):
    service = 'plan9-' + batch.run.BATCH.lower() + '.service'
    for key in hosts:
        batch.run.shell(key, 'systemctl reset-failed ' + service + ' || true\nsystemctl start ' + service + '\n')
        result = json.loads(batch.run.shell(key, 'curl -fsS --retry 10 --retry-connrefused --retry-delay 1 --max-time 3 http://127.0.0.1:18085/health\n'))
        assert result['redis_endpoint'] == 'redis:6379'


@contextmanager
def fault(batch, hosts, name, duration=240):
    prefix = 'plan9-' + batch.run.BATCH.lower()
    service = prefix + '.service'
    guard = prefix + '-recover-' + str(time.time_ns())
    started = now()
    armed = []
    try:
        for key in hosts:
            batch.run.shell(key, 'systemd-run --collect --unit=' + guard + ' --on-active=' + str(duration) + "s /bin/sh -ec 'systemctl reset-failed " + service + ' || true; systemctl start ' + service + "'\n"
                            'systemctl kill --kill-whom=main --signal=SIGKILL ' + service + '\n'
                            'for i in 1 2 3 4 5; do systemctl is-failed --quiet ' + service + ' && exit 0; sleep 1; done; exit 1\n')
            armed.append(key)
        batch.run.save(name + '-fault', {'started_at': started, 'hosts': hosts, 'guard': guard, 'guard_seconds': duration, 'injection': 'Owned HTTP service main process SIGKILL; metrics/mail/caches retained'})
        yield started
    finally:
        recover(batch, armed)
        for key in armed:
            batch.run.shell(key, 'systemctl stop ' + guard + '.timer || true\n')
        batch.run.save(name + '-native-recovered', {'at': now(), 'hosts': armed, 'guard_stopped': True})


def deliveries(batch):
    cid = batch.map['notification']['channel_id']
    rows = batch.api.call('GET', '/alerts/deliveries?channel_id=' + cid + '&limit=200')
    assert not rows.get('next_cursor')
    return rows['items']


def wait_delivery(batch, ids, event_type, timeout=120):
    deadline = time.monotonic() + timeout
    latest = []
    while time.monotonic() < deadline:
        latest = deliveries(batch)
        accepted = {x['instance_id'] for x in latest if x['status'] == 'accepted' and x['message']['event_type'] == event_type}
        if ids <= accepted:
            batch.run.save('deliveries-' + event_type, {'at': now(), 'items': latest})
            return latest
        time.sleep(2)
    batch.run.save('deliveries-' + event_type + '-timeout', {'at': now(), 'items': latest})
    raise RuntimeError('Actual SMTP deliveries did not complete: ' + event_type)
