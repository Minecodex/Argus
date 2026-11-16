"""Native expiry and credential-version checks; no database or clock changes."""
from datetime import datetime,timezone
from email.utils import parsedate_to_datetime
import argparse
import json
import secrets
import time
import uuid
from integration import Batch
from manage import Run,execute,now,write_json

KEYS=['bastion','member']

def delivery(run):
    return {key:json.loads(run.shell(key,'cat /var/lib/argus-otelcol/identity/delivery-state.json\n'))['report'] for key in KEYS}

def no_loss(before,after):
    for key in KEYS:
        assert before[key]['instance_id']==after[key]['instance_id'],'Delivery counters must never be reset for a passing check'
        for field in ['terminal_failures','enqueue_failures','receiver_refusals']:
            assert before[key][field]==after[key][field],(key,field,before[key][field],after[key][field])

def fresh(batch,after):
    result={}
    for key in KEYS:
        hid=batch.map['hosts'][key]['host_id']
        batch.api.wait('/enterprise/hosts/'+hid,lambda row:row['connection_status']=='online',timeout=160,label=key+' control online')
        def valid(rows):
            active=[row['collection'] for row in rows if (row.get('collection') or {}).get('desired_enabled')]
            return len(active)==4 and all(row['health']=='healthy' and row['freshness']=='fresh' and row.get('last_sample_at','')>after for row in active)
        result[key]=batch.api.wait('/monitoring/builtins?target_type=host&target_id='+hid,valid,timeout=160,label=key+' fresh native samples')
    return result

def expiry_prepare(batch):
    run=batch.run
    if (run.out/'expiry-preview.json').exists():return
    hid=batch.map['hosts']['member']['host_id']
    item=next(row for row in batch.api.call('GET','/monitoring/builtins?target_type=host&target_id='+hid) if row['definition']['key']=='host_metrics')
    collection=item['collection'];assert collection['desired_enabled']
    preview=batch.api.call('POST','/monitoring/builtins/preview',{'target':{'kind':'host','id':hid},'key':'host_metrics',
        'enabled':False,'expected_version':collection['version'],'config':collection['config']},key='natural-expiry-preview',expected=201)
    run.save('expiry-preview',{'preview':preview,'collection_id':collection['id'],'collection_version':collection['version'],
        'host_id':hid,'issued_at':now(),'intentionally_unconfirmed':True,'clock_and_ttl_unchanged':True})
    print('Real pending disable preview issued; collection remains enabled. Expires at '+preview['expires_at'],flush=True)

def expiry_check(batch):
    run=batch.run
    if (run.out/'expiry-refusal.json').exists():return
    saved=json.loads((run.out/'expiry-preview.json').read_text());ref=saved['preview']['action_ref']
    response=batch.api.http.get(batch.api.origin+'/api/v1/enterprise/pending-actions/'+ref,timeout=30)
    assert response.status_code==200
    server=parsedate_to_datetime(response.headers['Date']);expires=datetime.fromisoformat(saved['preview']['expires_at'].replace('Z','+00:00'))
    if server<=expires:raise RuntimeError('Native preview has not expired; remaining seconds '+str(int((expires-server).total_seconds())))
    pending=response.json()
    rejected=batch.api.call('POST','/enterprise/pending-actions/'+ref+'/confirm',key='expired-refusal-'+ref,expected=[409,410])
    assert rejected['code']=='PENDING_ACTION_INVALIDATED',rejected
    current=next(row['collection'] for row in batch.api.call('GET','/monitoring/builtins?target_type=host&target_id='+saved['host_id']) if row['id']==saved['collection_id'])
    assert current['desired_enabled'] and current['version']==saved['collection_version']
    run.save('expiry-refusal',{'passed':True,'action_ref':ref,'expires_at':expires.isoformat(),'server_date':server.isoformat(),
        'pending_status':pending['status'],'error_code':rejected['code'],'no_execution':True,'collection_still_enabled':True,
        'version_unchanged':True,'checked_at':now(),'clock_and_ttl_unchanged':True})
    print('Naturally expired preview refused; original collection still enabled and unchanged',flush=True)

def rotate(batch):
    run=batch.run;api=batch.api
    if (run.out/'credential-rotation.json').exists():return
    state=batch.map['hosts']['bastion'];path='/enterprise/hosts/'+state['host_id']
    before_host=api.call('GET',path);before_collector=batch.converge('bastion');before=delivery(run)
    secret=api.call('GET','/enterprise/secrets/'+state['secret_id'])
    credential=next(row for row in api.call('GET','/enterprise/credentials')['items'] if row['id']==state['credential_id'])
    journal=run.private/'rotation.json'
    assert not journal.exists(),'Resume the existing private rotation journal; never repeat a partial rotation blindly'
    old=api.call('POST',path+'/actions/preview-reconnect',{'expected_version':before_host['resource_version']},key='v1-preview-'+uuid.uuid4().hex,expected=201)
    record={'old_preview':old,'old_secret_version':secret['version'],'old_credential_version':credential['version'],
        'password':secrets.token_urlsafe(32),'stage':'prepared','started_at':now()};write_json(journal,record)
    execute(['docker','exec','-i',run.container('bastion')['Id'],'chpasswd'],data='root:'+record['password']+'\n')
    record['stage']='os_password_changed';write_json(journal,record)
    rotated=api.call('POST','/enterprise/secrets/'+state['secret_id']+'/rotate',{'value':record['password'],'expected_version':secret['version']},key='native-root-rotation')
    record['stage']='secret_rotated';write_json(journal,record)
    private=json.loads((run.private/'host-credentials.json').read_text());private['bastion']['password']=record['password'];write_json(run.private/'host-credentials.json',private)
    latest=next(row for row in api.call('GET','/enterprise/credentials')['items'] if row['id']==state['credential_id'])
    assert latest['version']==credential['version']+1 and rotated['version']==secret['version']+1
    rejected=api.call('POST','/enterprise/pending-actions/'+old['action_ref']+'/confirm',key='v1-preview-refusal',expected=[409,410])
    assert rejected['code']=='PENDING_ACTION_INVALIDATED',rejected
    # Observe leased SSH transports draining after normal secret rotation revokes them.
    deadline=time.monotonic()+100
    while time.monotonic()<deadline:
        listeners=run.shell('bastion','ss -H -lntp\n')
        if not any(':'+str(port)+' ' in listeners for port in [18443,19443]):break
        time.sleep(2)
    else:raise RuntimeError('Previous control SSH lease still listening after rotation')
    fresh_preview=api.call('POST',path+'/actions/preview-reconnect',{'expected_version':before_host['resource_version']},key='v2-preview-'+uuid.uuid4().hex,expected=201)
    confirmed=api.confirm(fresh_preview)
    samples=fresh(batch,now());host=api.call('GET',path);collector=batch.converge('bastion');after=delivery(run);no_loss(before,after)
    assert host['id']==before_host['id'] and host['connector_id']==before_host['connector_id'] and host['pinned_host_key']==before_host['pinned_host_key']
    assert host['resource_version']==before_host['resource_version']+1 and collector['id']==before_collector['id']
    record['stage']='verified';write_json(journal,record)
    run.save('credential-rotation',{'passed':True,'started_at':record['started_at'],'verified_at':now(),
        'old_preview_ref':old['action_ref'],'old_preview_error':rejected['code'],'fresh_preview_ref':fresh_preview['action_ref'],
        'secret_version_before':secret['version'],'secret_version_after':rotated['version'],
        'credential_version_before':credential['version'],'credential_version_after':latest['version'],
        'host_identity_preserved':True,'collector_identity_preserved':True,'connection':confirmed,'fresh_sources':samples,
        'delivery_before':before,'delivery_after':after,'no_new_delivery_loss':True})
    print('Actual root password/secret rotated; stale preview refused; current-version SSH/control/data restored with unchanged identities and no new delivery loss',flush=True)

def rotate_resume(batch):
    run=batch.run;api=batch.api
    if (run.out/'credential-rotation.json').exists():return
    record=json.loads((run.private/'rotation.json').read_text());assert record['stage']=='secret_rotated'
    state=batch.map['hosts']['bastion'];path='/enterprise/hosts/'+state['host_id']
    original=json.loads((run.out/'bastion-online.json').read_text())['host'];host=api.call('GET',path)
    credential=next(row for row in api.call('GET','/enterprise/credentials')['items'] if row['id']==state['credential_id'])
    secret=api.call('GET','/enterprise/secrets/'+state['secret_id'])
    assert credential['version']==record['old_credential_version']+1 and secret['version']==record['old_secret_version']+1
    before=delivery(run)
    old=api.call('GET','/enterprise/pending-actions/'+record['old_preview']['action_ref'])
    rejected=api.call('POST','/enterprise/pending-actions/'+record['old_preview']['action_ref']+'/confirm',key='v1-preview-refusal-recheck',expected=[409,410])
    assert rejected['code']=='PENDING_ACTION_INVALIDATED',rejected
    preview=api.call('POST',path+'/actions/preview-reconnect',{'expected_version':host['resource_version']},key='v2-resume-'+uuid.uuid4().hex,expected=201)
    result=api.confirm(preview);samples=fresh(batch,now());after=delivery(run);no_loss(before,after)
    current=api.call('GET',path)
    assert current['id']==original['id'] and current['connector_id']==original['connector_id'] and current['pinned_host_key']==original['pinned_host_key']
    assert current['resource_version']==host['resource_version']+1
    assert batch.converge('bastion')['id']==state['collector_id']
    record['stage']='verified';write_json(run.private/'rotation.json',record)
    run.save('credential-rotation',{'passed':True,'started_at':record['started_at'],'verified_at':now(),'resumed_without_repeating_rotation':True,
        'old_preview_ref':record['old_preview']['action_ref'],'old_preview_status':old['status'],'old_preview_error':rejected['code'],
        'fresh_preview_ref':preview['action_ref'],'secret_version_before':record['old_secret_version'],'secret_version_after':secret['version'],
        'credential_version_before':record['old_credential_version'],'credential_version_after':credential['version'],
        'host_identity_preserved':True,'collector_identity_preserved':True,'connection':result,'fresh_sources':samples,
        'delivery_before_resume':before,'delivery_after':after,'no_new_delivery_loss':True})
    print('Persisted v2 rotation verified: stale v1 preview invalidated, fresh v2 reconnect succeeded, original identities and data preserved',flush=True)

def rotate_after_fix(batch):
    run=batch.run
    if (run.out/'credential-rotation.json').exists():return
    original=run.private/'rotation.json';archived=run.private/'rotation-before-worker-fix.json'
    if not archived.exists():
        record=json.loads(original.read_text());assert record['stage']=='secret_rotated'
        samples=fresh(batch,now());snapshot=delivery(run)
        run.save('delivery-maintenance-gap',{'status':'fixed_historical_gap','issue':'B11c-D01',
            'cause':'Reconnect proof rejected after jsonb normalization; first deployment updated only server, leaving the old worker validator. Repair outage exceeded the native 300-second retry budget.',
            'interrupted_after':'2026-11-15T04:45:34Z','fresh_again_at':now(),'retry_limit_seconds':300,
            'delivery_after_recovery':snapshot,'fresh_sources':samples,'counters_reset':False,
            'earlier_failed_execution':'01a1dd87-143d-7cc4-a7d5-b67acd9e8387','server_worker_fix':json.loads((run.out/'server-fix-release.json').read_text())['backend_digest']})
        original.rename(archived)
    rotate(batch)

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['expiry_prepare','expiry_check','rotate','rotate_resume','rotate_after_fix']);args=parser.parse_args()
    run=Run();api=run.api();api.login();globals()[args.action](Batch(run,api))
