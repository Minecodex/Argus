"""Resume an existing B11c for user review without recreating identities or evidence."""
import argparse
from datetime import datetime,timezone
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess

from delivery_watch import watch
from integration import Batch
from lifecycle import delivery,fresh,no_loss
from manage import Run,ROOT,execute,now,write_json
from monitoring import dashboards
from network import probes
from run import browser


def prepare(review_id):
    assert re.fullmatch(r'recheck-\d{8}-\d{2}',review_id)
    run=Run();original=run.out
    assert json.loads((original/'checks.json').read_text())['status']=='ready_for_review'
    target=original/'rechecks'/review_id
    assert target.resolve().is_relative_to(original.resolve()) and not target.exists(),'Use a new recheck ID; retain previous results'
    target.mkdir(parents=True);private=target/'private';private.mkdir()
    execute(['icacls',str(private),'/inheritance:r','/grant:r',execute(['whoami'])+':(OI)(CI)F','*S-1-5-18:(OI)(CI)F'])
    # Compose/project and private login remain bound to the original owned run.
    # Only nonsecret mapping/spec copies and new evidence go to the recheck folder.
    for name in ['resource-map.json','host-specs.json','network-endpoints.json']:
        shutil.copy2(original/name,target/name)
    run.out=target;run.resources=target/'resource-map.json'
    api=run.api();api.login();batch=Batch(run,api)
    before=run.platform();run.save('platform-before',before)
    hosts={}
    for key,state in batch.map['hosts'].items():
        container=run.container(key);assert container['State']['Health']['Status']=='healthy'
        host=api.call('GET','/enterprise/hosts/'+state['host_id'])
        assert host['labels']['plan9_run']==run.id and host['connection_status']=='online'
        assert host['control_path']==('executor_tunnel' if key=='bastion' else 'bastion_relay')
        collector=batch.converge(key)
        assert collector['id']==state['collector_id'] and collector['route']['transport']==('executor_tunnel' if key=='bastion' else 'bastion_tunnel')
        hosts[key]={'host_id':host['id'],'resource_version':host['resource_version'],'collector_id':collector['id'],
            'container_id':container['Id'],'image_id':container['Image']}
    grant=api.call('GET','/enterprise/remote-access-grants/'+batch.map['remote']['grant_id'])
    assert grant['status']=='enabled' and datetime.fromisoformat(grant['valid_until'].replace('Z','+00:00'))>datetime.now(timezone.utc)
    policies=api.call('GET','/alerts/configurations?kind=policy')
    for selected in batch.map['alert_policies'].values():
        policy=next(row for row in policies if row['id']==selected['id'])
        assert policy['spec']['enabled'] and not policy['spec'].get('channel_ids') and not policy['spec'].get('route_ids')
    for key in ['bastion','member']:assert not any(row['connected'] for row in probes(run,key))
    baseline=delivery(run);previous=json.loads((original/'delivery-watch.json').read_text())['absolute_delivery']
    no_loss(previous,baseline)
    run.save('overnight-delivery',{'passed':True,'before':previous,'after':baseline,'new_failures':0,'counters_reset':False})
    run.save('preflight',{'passed':True,'at':now(),'original_run':str(original),'hosts':hosts,'grant_valid_until':grant['valid_until'],
        'both_platform_egress_paths_still_blocked':True,'new_global_permissions':0,'new_human_accounts':0})
    run.save('recheck',{'id':review_id,'run_id':run.id,'status':'validating','started_at':now(),
        'authorization':'用户明确要求执行 B11c；复用仍健康的既有场景并恢复有界负载、复查真实页面',
        'historical_mutation_tests_not_replayed':True,'human_acceptance':'pending'})
    return run,batch,original


def workload(batch):
    run=batch.run;before={key:run.shell(key,'systemctl is-active plan9-b11c-traffic.service || true\n') for key in batch.map['hosts']}
    run.workload()
    native={}
    for key in batch.map['hosts']:
        assert run.shell(key,'systemctl is-active plan9-b11c-traffic.service\n')=='active'
        native[key]={'previous_state':before[key],
            'runtime':run.shell(key,'systemctl show plan9-b11c-traffic.service -p ActiveState -p ExecMainStartTimestamp -p RuntimeMaxUSec -p MemoryMax -p CPUQuotaPerSecUSec\n'),
            'actual_http_response':json.loads(run.shell(key,'curl -fsS http://127.0.0.1:18083/recheck\n'))}
        assert native[key]['actual_http_response']['peer']==key
    run.save('native-workload',native)
    run.save('native-collections',fresh(batch,now()))


def health(run):
    binary=Path('C:/Users/23574/.codex/artifacts/argus-account-reset-20261115/argusctl.exe')
    with (run.out/'formal-health.json').open('wb') as out,(run.out/'private/formal-health.log').open('wb') as err:
        subprocess.run([str(binary),'verify','--config',str(ROOT/'deploy/.cache/argus-local-formal.yaml'),
            '--output','json','--artifacts',str(run.out/'private/formal-health')],cwd=ROOT,stdout=out,stderr=err,check=True,timeout=360)
    report=json.loads((run.out/'formal-health.json').read_text(encoding='utf-8-sig'))
    assert report['passed'] and len(report['checks'])==20 and all(row['status']=='pass' for row in report['checks'])
    print('Fresh formal health: 20/20 passed',flush=True)


def verify(batch,original):
    run=batch.run;api=batch.api;total=0
    for key in batch.map['hosts']:
        for board in batch.map['dashboards']:
            view=json.loads((run.out/(key+'-'+board+'-dashboard.json')).read_text())
            assert all(panel['status']=='success' and all(target['status']=='success' and target['data'] for target in panel['targets']) for panel in view['panels'])
            total+=len(view['panels'])
    assert total==40
    sessions=[]
    for proof_path in (run.out/'browser').glob('*/terminal-evidence.json'):
        proof=json.loads(proof_path.read_text());session=api.call('GET','/enterprise/remote-access-sessions/'+proof['session_id'])
        assert session['status']=='terminated' and session['recording_mode']==session['command_audit_mode']=='required'
        recording=api.call('GET','/enterprise/remote-access-recordings/'+session['recording_id'])
        assert recording['status']=='available' and recording['encrypted'] and recording['final_hash'] and recording['event_count']>0
        events=api.call('GET','/enterprise/remote-access-recordings/'+recording['id']+'/events')
        assert 'B11c-proof-end' in ''.join(event.get('data','') for event in events['events'])
        sessions.append({'session_id':session['id'],'recording_id':recording['id'],'host_id':session['host_id'],'protocol':session['protocol']})
    assert len(sessions)==4
    host_ids={row['host_id'] for row in batch.map['hosts'].values()}
    assert not any(row['host_id'] in host_ids for row in api.call('GET','/enterprise/remote-access-sessions?scope=active')['items'])
    current=api.call('GET','/alerts/instances')['items'];policy_ids={row['id'] for row in batch.map['alert_policies'].values()}
    assert not any(row['policy_id'] in policy_ids for row in current)
    gaps=[row for row in current if set(row.get('resource_ids',[]))&host_ids]
    assert len(gaps)==2 and all(row['state']['kind']=='collector_delivery_gap' for row in gaps)
    historical=json.loads((original/'delivery-maintenance-gap.json').read_text())['delivery_after_recovery']
    for key,state in batch.map['hosts'].items():
        gap=next(row for row in gaps if state['host_id'] in row['resource_ids'])
        assert gap['state']['delivery_gap']['terminal_failures']==historical[key]['terminal_failures']
    assert run.platform()==json.loads((run.out/'platform-before.json').read_text())
    final_sources=fresh(batch,now());run.save('native-collections-final',final_sources)
    run.save('sessions-verified',sessions);run.save('historical-delivery-gaps',gaps)
    report={'run_id':run.id,'review_id':run.out.name,'status':'ready_for_review','verified_at':now(),'human_acceptance':'pending',
        'actual_hosts':2,'same_host_collector_container_identities':True,'native_healthy_fresh_collections':8,
        'resource_scoped_dashboard_views':8,'nonempty_panels':40,'fresh_browser_cases':4,'fresh_recordings':4,
        'formal_health_checks':20,'formal_volumes_and_deployments_unchanged':True,
        'overnight_new_failures':0,'watch_seconds':120,'watch_new_failures':0,'retained_historical_delivery_gaps':2,
        'historical_mutation_tests_reference':str(original/'checks.json'),'historical_mutation_tests_replayed_today':False,
        'real_http_traffic_resumed':True,'traffic_runtime_max_seconds':28800,'next_batch_started':False}
    assert json.loads((run.out/'delivery-watch.json').read_text())['passed']
    run.save('checks',report)
    return report


def publish(run,report,original):
    state=json.loads((original/'run.json').read_text());state.update(status='ready_for_review',latest_recheck=report)
    write_json(original/'run.json',state)
    progress_path=ROOT/'docs/plan9/progress.json';progress=json.loads(progress_path.read_text(encoding='utf-8'))
    progress['current_runtime_validation']['latest_recheck']=report
    item=next(child for parent in progress['batches'] if parent['id']=='B11' for child in parent['sub_batches'] if child['id']=='B11c')
    item['latest_recheck']={'verified_at':report['verified_at'],'record':str(run.out/'checks.json'),'status':'passed','human_acceptance':'pending'}
    write_json(progress_path,progress)
    previous=json.loads((run.out/'recheck.json').read_text());previous.update(status='ready_for_review',verified_at=report['verified_at']);run.save('recheck',previous)
    run.save('source-state',{'head':execute(['git','rev-parse','HEAD']),'uncommitted_changes_preserved':True,
        'files':{str(path.relative_to(ROOT)):hashlib.sha256(path.read_bytes()).hexdigest() for path in Path(__file__).parent.iterdir() if path.is_file()}})
    review_date=datetime.strptime(run.out.name.split('-')[1],'%Y%m%d').strftime('%Y-%m-%d')
    notice=(f'## 最新执行复查（{review_date}）\n\n'
        '按“执行 B11c”复用原两台主机，重新开启最长八小时的真实 HTTP 低负载；当前八项采集新鲜、八个视图共 40 面板正常。'
        '本次四条真实 Shell/SSH、加密录像和中英/明暗/三种桌面宽度检查通过，正式健康 20/20。'
        '与上一轮持久计数比较以及本次连续 120 秒观察均无新增丢弃；两条历史缺口诊断继续保留。\n\n'
        '11 月 15 日的凭据轮换、自然过期、撤权和故障/恢复记录继续作为原始历史证据，未冒充今天重放。'
        f'新证据：`{run.out}`；四条新会话可在历史会话中按本次时间查看。环境继续保留，B12 未启动，人工验收仍待用户确认。\n\n')
    review_path=ROOT/'docs/plan9/batches/B11c-review.md';text=review_path.read_text(encoding='utf-8')
    if '## 最新执行复查' in text:
        text=re.sub(r'## 最新执行复查[^\n]*\n.*?(?=## 从这里查看)',notice,text,count=1,flags=re.S)
    else:text=text.replace('## 从这里查看',notice+'## 从这里查看',1)
    review_path.write_text(text,encoding='utf-8')
    (run.out/'handoff.md').write_text(text,encoding='utf-8')
    print('B11c resumed and freshly verified; original environment/evidence retained, human review pending',flush=True)


if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('--review-id',required=True);args=parser.parse_args()
    run,batch,original=prepare(args.review_id)
    workload(batch);dashboards(batch);browser(run);health(run);watch(batch)
    publish(run,verify(batch,original),original)
