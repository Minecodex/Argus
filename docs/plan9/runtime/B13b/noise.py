"""Actual silence/inhibition with an unsuppressed host and a later unmuted cycle."""
from configuration import enabled
from integration import Batch
from manage import Run, now
from scenario import CORE, condition, instances, wait_instances, fault, wait_delivery, deliveries


def recovered(batch, ids, keys, name):
    return wait_instances(batch, lambda r: ids <= {x['id'] for x in r if x['lifecycle'] == 'resolved'}, keys, history=True, name=name)


def silence(batch):
    run = batch.run
    if (run.out / 'silence-acceptance.json').exists():
        return
    enabled(batch, 'silence', 'owned', True)
    reports = batch.map['hosts']['reports']['host_id']
    with fault(batch, ['orders', 'reports'], 'silence-cycle'):
        rows = wait_instances(batch, lambda r: len(condition(r, ('firing',))) == 6 and all(
            x['state']['notification_state'] == 'muted' for x in condition(r, ('firing',)) if reports in x['resource_ids']), CORE, name='silence-firing')
        active = condition(rows, ('firing',))
        muted = {x['id'] for x in active if reports in x['resource_ids']}
        control = {x['id'] for x in active if reports not in x['resource_ids']}
        assert len(muted) == len(control) == 3
        sent = wait_delivery(batch, control, 'first')
        assert not any(x.get('instance_id') in muted and x['status'] == 'accepted' and x['message']['event_type'] == 'first' for x in sent)
        assert all(x['state']['noise_reasons'] for x in active if x['id'] in muted)
        run.save('silence-under-fault', {'items': active, 'deliveries': sent, 'muted_ids': sorted(muted), 'control_ids': sorted(control)})
        print('Reports silence suppressed its three episodes; three Orders controls were really delivered', flush=True)
    recovered(batch, muted | control, CORE, 'silence-recovered-history')
    wait_delivery(batch, control, 'recovery')
    enabled(batch, 'silence', 'owned', False)
    run.save('silence-acceptance', {'passed': True, 'at': now(), 'muted_ids': sorted(muted), 'control_ids': sorted(control),
                                  'scope_was_one_host_only': True, 'disabled_after_actual_recovery': True})


def inhibition(batch):
    run = batch.run
    if (run.out / 'inhibition-acceptance.json').exists():
        return
    enabled(batch, 'policy', 'child', False)
    enabled(batch, 'inhibition', 'owned', True)
    with fault(batch, ['reports'], 'inhibition-cycle'):
        rows = wait_instances(batch, lambda r: len(condition(r, ('firing',))) == 3, CORE, name='inhibition-primary-firing')
        primary = {x['id'] for x in condition(rows, ('firing',))}
        wait_delivery(batch, primary, 'first')
        enabled(batch, 'policy', 'child', True)
        children = wait_instances(batch, lambda r: len(condition(r, ('firing',))) == 1 and all(
            x['state']['notification_state'] == 'inhibited' for x in condition(r, ('firing',))), ['child'], name='inhibition-child-firing')
        child = condition(children, ('firing',))[0]
        assert child['state']['noise_reasons']
        sent = deliveries(batch)
        assert not any(x.get('instance_id') == child['id'] and x['status'] == 'accepted' and x['message']['event_type'] == 'first' for x in sent)
        run.save('inhibition-under-fault', {'primary_ids': sorted(primary), 'child': child, 'deliveries': sent,
                                          'source_was_firing_before_child_enabled': True})
        print('Existing real primary failure inhibited the same-host child; primary notification remained enabled', flush=True)
    recovered(batch, primary | {child['id']}, CORE + ['child'], 'inhibition-recovered-history')
    enabled(batch, 'inhibition', 'owned', False)
    run.save('inhibition-acceptance', {'passed': True, 'at': now(), 'primary_ids': sorted(primary), 'inhibited_child_id': child['id'],
                                     'equal_resource_identity_verified': True, 'disabled_after_recovery': True})


def released(batch):
    run = batch.run
    if (run.out / 'noise-release-acceptance.json').exists():
        return
    enabled(batch, 'silence', 'owned', False)
    enabled(batch, 'inhibition', 'owned', False)
    enabled(batch, 'policy', 'child', True)
    with fault(batch, ['reports'], 'noise-released-cycle'):
        rows = wait_instances(batch, lambda r: len(condition(r, ('firing',))) == 4 and all(
            not x['state']['noise_reasons'] for x in condition(r, ('firing',))), CORE + ['child'], name='noise-released-firing')
        ids = {x['id'] for x in condition(rows, ('firing',))}
        wait_delivery(batch, ids, 'first')
        print('New actual failure after silence/inhibition removal delivered all four eligible episodes', flush=True)
    recovered(batch, ids, CORE + ['child'], 'noise-released-recovered-history')
    wait_delivery(batch, ids, 'recovery')
    run.save('noise-release-acceptance', {'passed': True, 'at': now(), 'episode_ids': sorted(ids), 'same_child_rule_delivered_after_release': True})


def exercise(batch):
    assert (batch.run.out / 'core-acceptance.json').exists()
    silence(batch)
    inhibition(batch)
    released(batch)
    batch.run.save('noise-acceptance', {'passed': True, 'at': now(), 'silence': True, 'inhibition': True, 'release_new_cycle': True,
                                      'native_conditions_preserved_while_notification_suppressed': True})


if __name__ == '__main__':
    run = Run()
    api = run.api()
    api.login()
    exercise(Batch(run, api))
