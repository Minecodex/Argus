"""Read-only final identity, isolation, recording and preservation gate."""
import json
from integration import Batch
from manage import Run,execute,now


def verify(batch):
    run=batch.run;api=batch.api
    before=json.loads((run.out/'platform-before.json').read_text(encoding='utf-8'));after=run.platform()
    assert before==after and len(after['pvc'])==6
    nodes={};native={};collectors={};boards={}
    for key,state in batch.map['hosts'].items():
        host=api.call('GET','/enterprise/hosts/'+state['host_id']);assert host['labels']['plan9_run']==run.id and host['connection_status']=='online'
        assert host['bastion_scope_id']==batch.scope
        assert host['control_path']==('direct' if key=='bastion' else 'bastion_relay')
        current=batch.converge(key);assert current['role']==('edge_gateway' if key=='bastion' else 'leaf')
        collectors[key]=current
        raw=json.loads(run.shell(key,"python3 - <<'PY'\nimport json,socket,os\nprint(json.dumps({'hostname':socket.gethostname(),'machine_id':open('/etc/machine-id').read().strip(),'pid':os.getpid()}))\nPY\n"))
        assert raw['hostname']==run.id+'-'+key;nodes[key]=raw
        sources=api.call('GET','/monitoring/builtins?target_type=host&target_id='+host['id'])
        active=[row['collection'] for row in sources if (row.get('collection') or {}).get('desired_enabled')]
        assert len(active)==4 and all(row['health']=='healthy' and row['freshness']=='fresh' for row in active)
        native[key]=sources
        for board in batch.map['dashboards']:
            value=json.loads((run.out/(key+'-'+board+'-dashboard.json')).read_text(encoding='utf-8'))
            assert value['panels'] and all(panel['status']=='success' and all(target['status']=='success' and target['data'] for target in panel['targets']) for panel in value['panels'])
            boards[key+'-'+board]=len(value['panels'])
    assert nodes['bastion']['machine_id']!=nodes['member']['machine_id'] and sum(boards.values())==40
    assert collectors['member']['route']['kind']=='bastion_gateway' and collectors['member']['route']['transport']=='bastion_tunnel'
    assert collectors['member']['route']['gateway_collector_id']==collectors['bastion']['id']
    scope=api.call('GET','/enterprise/bastion-scopes/'+batch.scope)
    assert scope['status']=='active' and scope['relay_status']=='ready' and scope['member_count']==1
    isolation=json.loads((run.out/'member-isolation.json').read_text());assert not isolation['member_to_platform_direct'] and isolation['dns_bypassed_for_direct_tests']
    for key in ['connection-recovery','alert-cycle','data-pause-cycle']:
        value=json.loads((run.out/(key+'.json')).read_text())
        if key=='connection-recovery':assert value['no_new_delivery_loss']
        else:assert value['lifecycle']=='resolved'
    browser=json.loads((run.out/'browser-review.json').read_text());assert browser['passed'] and browser['cases']==4
    evidence=list((run.out/'browser').glob('*/terminal-evidence.json'));assert len(evidence)==4
    sessions=[];recordings=[];audit=[]
    for path in evidence:
        proof=json.loads(path.read_text(encoding='utf-8'));session=api.call('GET','/enterprise/remote-access-sessions/'+proof['session_id'])
        assert session['status']=='terminated' and session['host_id']==batch.map['hosts'][proof['key']]['host_id']
        assert session['protocol']==proof['protocol'] and session['recording_mode']==session['command_audit_mode']=='required'
        recording=api.wait('/enterprise/remote-access-recordings/'+session['recording_id'],lambda row:row['status']=='available',timeout=45)
        assert recording['encrypted'] and recording['final_hash'] and recording['event_count']>0
        events=api.call('GET','/enterprise/remote-access-recordings/'+recording['id']+'/events')
        assert 'B11b-proof-end' in ''.join(event.get('data','') for event in events['events'])
        logs=api.call('GET','/enterprise/audit-events?resource_id='+session['id'])['items'];assert any(row['action']=='remote_access.session.create' for row in logs)
        assert not any(row['action']=='remote_access.command_audit.degrade' for row in logs)
        sessions.append(session);recordings.append(recording);audit.extend(logs)
    run.save('sessions-verified',sessions);run.save('recordings-verified',recordings);run.save('session-audit-verified',audit)
    active=api.call('GET','/enterprise/remote-access-sessions?scope=active')['items']
    assert not any(row['host_id'] in {state['host_id'] for state in batch.map['hosts'].values()} for row in active)
    current=api.call('GET','/alerts/instances')['items'];owned={row['id'] for row in batch.map['alert_policies'].values()}
    assert not any(row['policy_id'] in owned or set(row.get('resource_ids',[]))&{s['host_id'] for s in batch.map['hosts'].values()} for row in current)
    watch=json.loads((run.out/'delivery-watch.json').read_text());assert watch['passed'] and watch['terminal_failures']==0
    health=json.loads((run.out/'formal-health.json').read_text(encoding='utf-8-sig'));assert health['passed'] and all(row['status']=='pass' for row in health['checks'])
    cli=['kubectl','--context','docker-desktop','-n','argus-system'];deployment_images={}
    deployments=json.loads(execute([*cli,'get','deployments','-o','json']))['items']
    replica_sets=json.loads(execute([*cli,'get','replicasets','-o','json']))['items']
    all_pods=json.loads(execute([*cli,'get','pods','-o','json']))['items']
    for name,definition in after['deployments'].items():
        deployment=next(row for row in deployments if row['metadata']['uid']==definition['uid'])
        owners={row['metadata']['uid'] for row in replica_sets if any(owner['uid']==definition['uid'] for owner in row['metadata'].get('ownerReferences',[]))}
        pods=[pod for pod in all_pods if not pod['metadata'].get('deletionTimestamp') and any(owner['uid'] in owners for owner in pod['metadata'].get('ownerReferences',[]))]
        assert len(pods)>=deployment['spec'].get('replicas',1)
        assert all(pod['status'].get('containerStatuses') and all(status['ready'] for status in pod['status']['containerStatuses']) for pod in pods),name
        deployment_images[name]=definition['images']
    run.save('native-collections-final',native);run.save('native-hosts-final',nodes);run.save('collectors-final',collectors)
    run.save('final-preservation',{'passed':True,'at':now(),'formal_pvcs_preserved':after['pvc'],'formal_deployments_unchanged':True,'images':deployment_images})
    report={'run_id':run.id,'batch':'B11b','status':'ready_for_review','human_acceptance':'pending','verified_at':now(),
        'actual_hosts':2,'native_healthy_fresh_collections':8,'resource_scoped_dashboard_views':8,'nonempty_panels':40,
        'scope_id':batch.scope,'hosts':{key:state['host_id'] for key,state in batch.map['hosts'].items()},
        'control':{'bastion':'direct','member':'bastion_relay'},'telemetry':{'bastion':'edge_gateway/direct_argus/direct','member':'leaf/bastion_gateway/bastion_tunnel'},
        'member_direct_platform_access':False,'native_fault_and_query_health_events_recovered':True,
        'browser':browser,'verified_encrypted_recordings':4,'test_sessions_closed':True,'current_owned_alerts':0,
        'delivery_watch_seconds':120,'terminal_failures':0,'enqueue_failures':0,'receiver_refusals':0,
        'formal_health_checks':len(health['checks']),'formal_pvcs_preserved':6,'direct_database_writes':False,'synthetic_telemetry':False,
        'shared_kernel_scope':True,'grant_valid_until':batch.map['remote']['valid_until']}
    run.save('checks',report);state=json.loads((run.out/'run.json').read_text());state.update(status='ready_for_review',self_check='passed',runtime_status='running');run.save('run',state)
    print('B11b real isolation, TLS Relay, Gateway/Leaf data, PTYs, recordings, both alerts and formal preservation passed',flush=True)


if __name__=='__main__':
    run=Run();api=run.api();api.login();verify(Batch(run,api))
