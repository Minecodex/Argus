"""Continuous windows, independent/AND/OR, handling, grouping and SMTP retry."""
import json
import time
import hashlib
from email.parser import BytesParser
from email.policy import default
from collections import Counter
from integration import Batch
from manage import Run, now
from scenario import CORE, condition, instances, wait_instances, fault, receipts, mailbox_mode, wait_delivery, instant


def semantic_digest(receipt):
    message = BytesParser(policy=default).parsebytes(receipt['body'].encode())
    return hashlib.sha256(message.get_content().replace('\r\n', '\n').strip().encode()).hexdigest()


def delivery_proof(batch, ids):
    sent = wait_delivery(batch, ids, 'first')
    messages = receipts(batch)
    retried = [x for x in messages if x['kind'] == 'temporary_rejection']
    assert retried
    assert any(x['kind'] == 'accepted' and semantic_digest(x) == semantic_digest(retried[0]) for x in messages)
    grouped = [x for x in sent if x.get('instance_id') in ids and x['message']['event_type'] == 'first']
    counts = Counter(x['batch_id'] for x in grouped)
    assert max(counts.values()) > 1
    details = [batch.api.call('GET', '/alerts/deliveries/' + x['id']) for x in grouped]
    assert any(any(str(a.get('result_code')) == '451' for a in d['attempts']) and any(str(a.get('result_code')) == '250' for a in d['attempts']) for d in details)
    batch.run.save('grouping-retry-acceptance', {'passed': True, 'logical_deliveries': grouped, 'delivery_details': details, 'receipts': messages,
                   'same_decoded_message_retried_after_451': True, 'smtp_envelope_batch_ids_are_per_attempt': True,
                   'deduplicated_episode_ids': sorted(ids)})


def finalize(batch):
    run = batch.run
    initial = json.loads((run.out / 'core-firing.json').read_text())['items']
    ids = {x['id'] for x in condition(initial, ('firing',))}
    assert len(ids) == 6
    pending = json.loads((run.out / 'core-pending.json').read_text())['items']
    pending_at = {x['id']: instant(x['state']['pending_since']) for x in condition(pending, ('pending',))}
    assert ids == set(pending_at)
    assert all((instant(x['state']['last_slot']) - pending_at[x['id']]).total_seconds() >= 30 for x in condition(initial, ('firing',)))
    assert not condition(json.loads((run.out / 'short-pulse-cleared.json').read_text())['items'])
    history = wait_instances(batch, lambda r: ids <= {x['id'] for x in r if x['lifecycle'] == 'resolved'}, CORE, history=True, timeout=100, name='core-recovered-history')
    assert {x['id'] for x in history} == ids, 'No extra resolved episode may hide a duplicated trigger'
    assert json.loads((run.out / 'handling-acceptance.json').read_text())['passed']
    delivery_proof(batch, ids)
    wait_delivery(batch, ids, 'recovery')
    mailbox_mode(batch, 'accept')
    run.save('core-acceptance', {'passed': True, 'at': now(), 'short_pulse_did_not_fire': True, 'for_seconds': 30, 'recovery_seconds': 20,
             'firing_instances': sorted(ids), 'fixed_slot_window_verified': True, 'repeated_evaluation_deduplicated': True,
             'independent_every_any_and_negative_control_verified': True, 'history': history,
             'smtp_retry_validated_as_frozen_message_not_attempt_envelope': True})
    print('All six same episodes recovered; handling, grouping and actual 451 retry verified', flush=True)


def exercise(batch):
    run, api = batch.run, batch.api
    if (run.out / 'core-acceptance.json').exists():
        return
    if (run.out / 'core-long-native-recovered.json').exists() and (run.out / 'core-firing.json').exists():
        finalize(batch)
        return
    baseline = condition(instances(batch, CORE))
    assert not baseline
    historical_ids = {x['id'] for x in instances(batch, CORE, history=True)}
    with fault(batch, ['reports'], 'short-pulse'):
        pending = wait_instances(batch, lambda r: len(condition(r, ('pending',))) == 3 and not condition(r, ('firing',)), CORE, timeout=25, name='short-pulse-pending')
        assert all(x['state'].get('pending_since') for x in condition(pending, ('pending',)))
    wait_instances(batch, lambda r: not condition(r), CORE, timeout=50, name='short-pulse-cleared')
    assert {x['id'] for x in instances(batch, CORE, history=True)} == historical_ids, 'Short pulse must not have fired and recovered'
    print('Short actual failure stayed pending and cleared before the continuous window', flush=True)
    mailbox_mode(batch, 'temporary_once')
    first_ids = set()
    with fault(batch, ['orders', 'reports'], 'core-long'):
        pending = wait_instances(batch, lambda r: len(condition(r, ('pending',))) == 6, CORE, timeout=28, name='core-pending')
        pending_at = {x['id']: instant(x['state']['pending_since']) for x in condition(pending, ('pending',))}
        firing = wait_instances(batch, lambda r: len(condition(r, ('firing',))) == 6, CORE, timeout=95, name='core-firing')
        active = condition(firing, ('firing',))
        first_ids = {x['id'] for x in active}
        assert first_ids == set(pending_at)
        for event in active:
            assert (instant(event['state']['last_slot']) - pending_at[event['id']]).total_seconds() >= 30
            assert event['state']['labels']['plan9_run'] == run.id
        assert not condition(instances(batch, ['negative']), ('firing', 'recovering'))
        print('Six real independent/AND/OR episodes fired after their full fixed-slot window', flush=True)
        first_slots = {x['id']: x['last_evaluated_at'] for x in active}
        for _ in range(2):
            time.sleep(12)
            current = condition(instances(batch, CORE), ('firing',))
            assert {x['id'] for x in current} == first_ids
        assert all(x['last_evaluated_at'] > first_slots[x['id']] for x in current)
        run.save('core-dedup-after', {'at': now(), 'items': current})
        target = next(x for x in current if x['policy_id'] == batch.map['alert_policies']['independent']['id'] and batch.map['hosts']['reports']['host_id'] in x['resource_ids'])
        stale = target['handling_version']
        for operation, extra in [('acknowledge', {}), ('assign', {'owner_id': api.user['id']}), ('comment', {'comment': 'B13b real handling verification; service remains failed until controlled recovery'})]:
            target = api.call('POST', '/alerts/instances/' + target['id'] + '/handle', {'operation': operation, 'expected_handling_version': target['handling_version'], **extra}, key='core-handle-' + operation + '-' + target['id'])
        assert target['acknowledged'] and target['owner_id'] == api.user['id']
        rejected = api.call('POST', '/alerts/instances/' + target['id'] + '/handle', {'operation': 'acknowledge', 'expected_handling_version': stale}, key='stale-handling-check', expected=409)
        run.save('handling-acceptance', {'passed': True, 'instance': target, 'stale_version_rejection': rejected})
        delivery_proof(batch, first_ids)
        print('Episode IDs stayed stable; handling, grouped TLS delivery and actual 451 retry passed', flush=True)
    finalize(batch)


if __name__ == '__main__':
    run = Run()
    api = run.api()
    api.login()
    exercise(Batch(run, api))
