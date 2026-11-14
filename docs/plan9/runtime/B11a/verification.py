"""Read-only final proof, artifact identity and preservation checks."""
import hashlib
import json
from integration import Batch
from manage import Run,execute,now


def verify(batch):
    run=batch.run;api=batch.api
    before=json.loads((run.out/'platform-before.json').read_text(encoding='utf-8'))
    after=run.platform()
    assert before['pvc']==after['pvc'],'Formal PVC identities changed'
    for name,value in before['deployments'].items():
        if name=='argus-web' and after['deployments'][name]!=value:
            followup=json.loads((run.out/'host-list-trends-v1/final-review.json').read_text(encoding='utf-8'))
            assert followup['passed'] and after['deployments'][name]['uid']==value['uid']
            assert after['deployments'][name]['images']==[followup['image']]
        else:assert after['deployments'][name]==value,'Unexpected deployment change: '+name
    assert len(after['pvc'])==6
    host=api.call('GET','/enterprise/hosts/'+batch.host)
    assert host['status']=='active' and host['control_path']=='direct' and not host.get('bastion_scope_id')
    collector=batch.converge()
    assert collector['route']['kind']=='direct_argus' and collector['route']['transport']=='direct'
    definitions=api.call('GET','/monitoring/builtins?target_type=host&target_id='+batch.host)
    sources={}
    for item in definitions:
        row=item.get('collection') or {}
        if row.get('desired_enabled'):
            assert row['health']=='healthy' and row['freshness']=='fresh',row
            sources[item['definition']['key']]={'freshness':row['freshness'],'last_sample_at':row['last_sample_at']}
    assert len(sources)==4
    run.save('native-collections-final',definitions)
    boards={}
    for key in batch.map['dashboards']:
        value=json.loads((run.out/(key+'-dashboard.json')).read_text(encoding='utf-8'))
        for panel in value['panels']:
            assert panel['status']=='success' and panel['targets']
            assert all(target['status']=='success' and len(target['data'])>0 for target in panel['targets'])
        boards[key]=len(value['panels'])
    assert sum(boards.values())==20
    previous=json.loads((run.out/'distribution-catalog.json').read_text(encoding='utf-8'))
    published=api.call('GET','/enterprise/telemetry/distributions')
    byid={row['id']:row for row in published}
    assert all(byid[row['id']]==row for row in previous),'Existing distribution metadata was changed'
    added=[row for row in published if row['id'] not in {row['id'] for row in previous}]
    assert len(added)==1 and added[0]['name']=='argus-otelcol-linux-amd64'
    binary_hash=run.shell('direct','sha256sum /usr/local/bin/argus-otelcol\n').split()[0]
    assert binary_hash==hashlib.sha256((run.out/'collector-release/argus-otelcol').read_bytes()).hexdigest()
    sessions=api.call('GET','/enterprise/remote-access-sessions?host_id='+batch.host)['items']
    assert sessions and all(row['status']=='terminated' for row in sessions)
    browser=json.loads((run.out/'browser-review.json').read_text(encoding='utf-8'));assert browser['passed']
    proofs=list((run.out/'browser').glob('*/terminal-evidence.json'))
    assert len(proofs)==4
    recordings=[];audits=[]
    for path in proofs:
        proof=json.loads(path.read_text(encoding='utf-8'))
        session=next(row for row in sessions if row['id']==proof['session_id'])
        assert session['recording_mode']==session['command_audit_mode']=='required'
        recording=api.wait('/enterprise/remote-access-recordings/'+session['recording_id'],
            lambda row:row['status']=='available',timeout=45,label='Encrypted recording completion')
        assert recording['encrypted'] and recording['event_count']>0 and recording['final_hash']
        page=api.call('GET','/enterprise/remote-access-recordings/'+recording['id']+'/events')
        assert 'B11a-proof-end' in ''.join(event.get('data','') for event in page['events'])
        events=api.call('GET','/enterprise/audit-events?resource_id='+session['id'])['items']
        assert any(event['action']=='remote_access.session.create' for event in events)
        assert not any(event['action']=='remote_access.command_audit.degrade' for event in events)
        recordings.append(recording);audits.extend(events)
    run.save('recordings-final',recordings);run.save('session-audit-final',audits)
    current=[item for item in api.call('GET','/alerts/instances')['items'] if batch.host in item.get('resource_ids',[])]
    gaps=[item for item in current if item['state']['kind']=='collector_delivery_gap']
    assert not any(item['state']['kind']!='collector_delivery_gap' for item in current)
    assert len(gaps)==1 and gaps[0]['state']['delivery_gap']['terminal_failures']['metric_points']==1999
    run.save('historical-delivery-gaps-preserved',gaps)
    watch=json.loads((run.out/'delivery-watch.json').read_text(encoding='utf-8'))
    assert watch['passed'] and watch['new_terminal_failures']==0
    alert=json.loads((run.out/'alert-cycle.json').read_text(encoding='utf-8'));assert alert['lifecycle']=='resolved'
    health=json.loads((run.out/'formal-health.json').read_text(encoding='utf-8-sig'))
    assert health['passed'] and all(item['status']=='pass' for item in health['checks'])
    snapshot=json.loads(run.shell('direct',"python3 - <<'PY'\nimport json,urllib.request\nfrom datetime import datetime,timezone\ntext=urllib.request.urlopen('http://127.0.0.1:8888/metrics',timeout=4).read().decode()\nkeys=('otelcol_exporter_send_failed_metric_points','otelcol_exporter_enqueue_failed_metric_points','otelcol_receiver_refused_metric_points')\nrows=[line for line in text.splitlines() if not line.startswith('#') and line.startswith(keys)]\nprint(json.dumps({'at':datetime.now(timezone.utc).isoformat(),'failure_counters':rows}))\nPY\n"))
    assert snapshot['failure_counters'] and all(float(row.split()[-1])==0 for row in snapshot['failure_counters'])
    run.save('delivery-final',snapshot)
    run.save('final-preservation',{'passed':True,'verified_at':now(),'formal_pvc_identities_preserved':6,
        'formal_deployments_unchanged':after['deployments'],'existing_distributions_preserved':len(previous),
        'new_signed_amd64_distribution':added[0]['id'],'installed_binary_sha256':binary_hash})
    report={'run_id':run.id,'status':'ready_for_review','verified_at':now(),'human_acceptance':'pending',
        'direct_database_writes':False,'synthetic_telemetry':False,'host_id':batch.host,'collector_id':collector['id'],
        'control_path':'direct','collector_transport':'direct','healthy_fresh_sources':sources,
        'dashboards':boards,'nonempty_panels':20,'browser':browser,'verified_recordings':len(recordings),
        'all_test_sessions_closed':True,'alert_cycle':alert,'current_owned_alerts':len(current),
        'current_owned_condition_or_query_health_alerts':0,'historical_delivery_gaps':[row['id'] for row in gaps],
        'delivery_watch':watch,'known_issues':['B11a-D01: setup SOURCE_REJECTED, 1999 historical lost metric points; retained without acknowledgement'],
        'formal_health_checks':len(health['checks']),'formal_pvcs_preserved':6,'shared_kernel_scope':True,
        'grant_valid_until':batch.map['remote']['valid_until']}
    run.save('checks',report)
    state=json.loads((run.out/'run.json').read_text(encoding='utf-8'))
    state.update(status='ready_for_review',self_check='passed',runtime_status='running',human_acceptance='pending')
    run.save('run',state)
    print('B11a real direct connection, recordings, monitoring, alert and preservation verified',flush=True)


if __name__=='__main__':
    run=Run();api=run.api();api.login();verify(Batch(run,api))
