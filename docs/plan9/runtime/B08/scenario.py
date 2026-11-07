"""B08 log continuity, service fault and recovery through real observations."""
import json
import secrets
import time
import uuid

from logs import DIR, FAILED, SERVICE, query_scope
from manage import now, write_json


def rotation(logs):
    run = logs.run
    for mode in ('rename', 'copy'):
        prefix = f'{mode}-before'
        before = [logs.checked_request(f'{prefix}-{i}',stream=mode) for i in range(1,7)]
        path = f'{DIR}/json/{mode}.log'
        original = run.shell('collector',f'stat -c %i {path}\n').strip()
        if mode == 'rename':
            run.shell('collector',f'mv {path} {path}.1\n')
        else:
            run.shell('collector',f'cp {path} {path}.1\ntruncate -s 0 {path}\n')
        after = run.request(f'{mode}-after',stream=mode)
        new = run.shell('collector',f'stat -c %i {path}\n').strip()
        assert (original==new) == (mode=='copy')
        for item in [*before,after]:
            logs.wait_records(item['message'],logs.app)
        logs.wait_records(after['message']+'-journal',logs.host)
        assert after['sequence']==before[-1]['sequence']+1
        write_json(run.out/(mode+'-rotation.json'),{'before':before,'after':after,'old_inode':original,
                   'new_inode':new,'rotation': 'copytruncate' if mode=='copy' else 'rename_create',
                   'exactly_once_records':len(before)+1})
        print(mode + ' rotation verified with contiguous sequence',flush=True)


def restart_and_pause(logs):
    run = logs.run
    old = run.shell('collector','systemctl show -p InvocationID --value argus-otelcol.service\n').strip()
    logs.checked_request('before-collector-restart')
    run.shell('collector','systemctl restart argus-otelcol.service\n')
    new = run.shell('collector','systemctl show -p InvocationID --value argus-otelcol.service\n').strip()
    assert len(old)==len(new)==32 and old!=new
    logs.converge()
    resumed = logs.checked_request('after-collector-restart')
    original = json.loads((run.out/'before-collector-restart.json').read_text(encoding='utf-8'))['http']
    logs.wait_records(original['message'],logs.app)
    logs.wait_records(original['message']+'-journal',logs.host)
    disabled = logs.enable('json',False)
    receipt = None
    try:
        receipt = run.request('during-json-pause',error=True)
        logs.wait_records(receipt['message']+'-journal',logs.host)
        rows = logs.wait_records(receipt['message'],logs.app)
        assert 'at orders.validate_request' in str(rows[0]['body'])
        assert logs.collection('stack')['desired_enabled'] and logs.collection('journal')['desired_enabled']
        time.sleep(3)
        assert len(logs.query(receipt['message'],logs.app))==1
    finally:
        enabled = logs.enable('json',True)
    logs.wait_records(receipt['message'],logs.app,count=2)
    tail = logs.checked_request('after-json-resume')
    write_json(run.out/'lifecycle-checks.json',{'collector_invocation_changed':True,
        'resumed_request':resumed,'disabled_rule':disabled,'enabled_rule':enabled,
        'stack_and_journal_continued_during_pause':True,'buffered_file_record_arrived_once':True,'new_request':tail})
    print('Collector restart and independent JSON pause/recovery verified',flush=True)


def service_metric(logs):
    return logs.api.call('POST','/enterprise/metrics/query',{**query_scope(logs.host), 'query':FAILED})


def configure_alert(logs):
    base = logs.run.out.parent/'common-evaluator.json'
    evaluator = json.loads(base.read_text(encoding='utf-8'))
    logs.map['evaluator'] = evaluator
    logs.save()
    path = '/enterprise/data-authorizations/role/'+evaluator['role_id']
    grants = logs.api.call('GET',path+'?resource_type=host')
    if not any(x['direct'] and x['resource_id']==logs.host for x in grants['items']):
        logs.api.call('POST',path,{'resource_type':'host','resource_ids':[logs.host],
            'remove':False,'expected_version':grants['authorization_version']},expected=204)
    name = 'B08 HTTP log service failed'
    rows = logs.api.call('GET','/alerts/configurations?kind=policy')
    row = next((r for r in rows if r['name']==name),None)
    if row is None:
        deadline = time.monotonic()+80
        while True:
            metric = service_metric(logs)
            values = metric['data']['result']
            if len(values)==1:
                assert float(values[0]['value'][1])==0
                break
            if time.monotonic()>deadline:
                raise RuntimeError('Systemd failed-state series unavailable')
            time.sleep(3)
        spec = {'description':'Actual failed state of the B08 systemd HTTP service','enabled':True,
            'resources':{'mode':'explicit','types':['host'],'ids':[logs.host],'matchers':[]},
            'rules':[{'name':name,'expression':FAILED,'query_mode':'numeric','operator':'>','threshold':.5,
                      'recovery_threshold':0,'severity':'warning','unit':''}],
            'mode':'independent','match_by':['argus_resource_id'],'interval_seconds':10,'for_seconds':20,
            'recovery_seconds':20,'no_data_behavior':'report','error_behavior':'keep',
            'channel_ids':[],'route_ids':[],'labels':{'plan9_batch':'b08','plan9_run':logs.run.id}}
        trial = logs.api.call('POST','/alerts/trial',spec)
        assert trial['health']=='normal'
        write_json(logs.run.out/'alert-baseline.json',{'metric':metric,'trial':trial,'spec':spec})
        preview = logs.api.call('POST','/alerts/configurations/preview',
            {'kind':'policy','operation':'create','name':name,'spec':spec},key='service-failed',expected=202)
        logs.api.confirm(preview)
        row = next(r for r in logs.api.call('GET','/alerts/configurations?kind=policy') if r['name']==name)
    logs.map['alert_policies']['service'] = {'id':row['id'],'name':name,'expression':FAILED}
    logs.save()
    print('Real systemd alert configured',flush=True)


def exercise_alert(logs):
    run = logs.run
    file = run.out/'alert-cycle.json'
    if file.exists():
        return
    policy = logs.map['alert_policies']['service']['id']
    started = now()
    recover = f'rm -f {DIR}/fail-start; systemctl reset-failed {SERVICE} || true; systemctl start {SERVICE}'
    firing = None
    try:
        run.shell('collector',f"systemd-run --unit=plan9-b08-auto-recover --on-active=100s /bin/sh -ec '{recover}'\n"
                  f"systemctl stop {SERVICE}\ntouch {DIR}/fail-start\nsystemctl start {SERVICE} || true\n"
                  f"for i in 1 2 3 4 5; do systemctl is-failed --quiet {SERVICE} && exit 0; sleep 1; done\nexit 1\n")
        report = logs.api.wait('/alerts/instances?policy_id='+policy,lambda value:any(
            row['lifecycle']=='firing' and row['state']['kind']=='condition' and row['started_at']>=started[:19]
            for row in value['items']),timeout=90,label='Systemd fault alert')
        firing = next(row for row in report['items'] if row['lifecycle']=='firing' and row['state']['kind']=='condition')
        assert float(service_metric(logs)['data']['result'][0]['value'][1])==1
        logs.wait_records('B08 controlled service startup failure',logs.host)
        write_json(run.out/'alert-firing.json',firing)
        print('Systemd failure triggered real alert and native journal message',flush=True)
    finally:
        run.shell('collector',f'systemctl stop plan9-b08-auto-recover.timer || true\n{recover}\n')
    recovered = logs.api.wait('/alerts/history?policy_id='+policy,lambda value:any(
        row['id']==firing['id'] and row['lifecycle']=='resolved' for row in value['items']),
        timeout=90,label='Systemd alert recovery')
    event = next(row for row in recovered['items'] if row['id']==firing['id'])
    assert float(service_metric(logs)['data']['result'][0]['value'][1])==0
    logs.checked_request('after-service-recovery')
    write_json(file,{'id':event['id'],'started_at':event['started_at'],'resolved_at':event['resolved_at'],
                     'lifecycle':event['lifecycle']})
    print('Actual alert resolved after the service recovered',flush=True)

def verify_redaction(logs):
    run = logs.run
    marker = 'b08-redaction-' + uuid.uuid4().hex
    canary = secrets.token_hex(24)
    record = json.dumps({'message':marker,'authorization':'Bearer '+canary})
    run.shell('collector', "printf '%s\\n' '" + record + "' >> " + DIR + "/json/redaction.log\n")
    rows = logs.wait_records(marker,logs.app)
    body = json.loads(rows[0]['body'])
    assert body['message']==marker and canary not in json.dumps(rows)
    write_json(run.out/'redaction-check.json',{'checked_at':now(),'marker':marker,
        'field':'authorization','value_removed':True,'source_id':rows[0]['source_id']})
    print('Native file log redaction verified',flush=True)
