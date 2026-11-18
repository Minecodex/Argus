"""Produce real authorized and control CSV rows before the member's revoke."""
from urllib.parse import urlencode
from manage import Run, now
from integration import Batch
from B13b.scenario import condition, fault, wait_instances


def collect(batch):
    run, api = batch.run, batch.api
    if (run.out / 'export-baseline.json').exists():
        return
    policy_id = batch.map['alert_policies']['service']['id']
    owned = {x['host_id'] for x in batch.map['hosts'].values()}
    with fault(batch, ['orders', 'reports'], 'history', duration=120):
        page = wait_instances(batch, lambda rows: len(condition(rows, ('firing',))) == 2,
                              ['service'], timeout=80, name='history-firing')
        active = condition(page, ('firing',))
        assert {x['resource_ids'][0] for x in active} == owned
        ids = {x['id'] for x in active}
    rows = wait_instances(batch, lambda rows: ids <= {x['id'] for x in rows if x['lifecycle'] == 'resolved'},
                          ['service'], timeout=90, history=True, name='history-recovered')
    events = {x['resource_ids'][0]: x['id'] for x in rows if x['id'] in ids}
    export = api.call('GET', '/alerts/history/export?' + urlencode({'policy_id': policy_id}))
    assert 'csv' in export['content_type'] and all(ident in export['content'] for ident in owned)
    run.save('export-baseline', {'at': now(), 'policy_id': policy_id, 'alert_ids_by_host': events, 'rows': len(events),
                                 'real_protocol': True, 'source': 'Recovered service failure alerts'})
    print('Two actual recovered alerts now provide the scoped CSV positive and negative controls', flush=True)


if __name__ == '__main__':
    run = Run()
    api = run.api()
    api.login()
    collect(Batch(run, api))
