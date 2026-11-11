"""Check exact deployed session UI, live B10a health and review metadata."""
from datetime import datetime,timezone
import hashlib,json,re
from run import Run,ROOT,execute
r=Run();a=r.api();out=r.out/'session-repair'
read=lambda file:json.loads(file.read_text(encoding='utf-8'))
browser=read(r.out/'session-repair-browser.json');assert browser['passed'] and browser['cases']==6
health=read(r.out/'session-repair-health-v3.log');assert health['passed'] and all(x['status']=='pass' for x in health['checks'])
for name,needle in [('session-repair-ui-all.log','157 passed'),('session-repair-log-tests-final.log','4 passed'),('session-repair-mock-browser-v3.log','8 passed')]:
 assert needle in (r.out/name).read_text(encoding='utf-8'),name
for name in ('session-repair-types-v3-final.log','session-repair-eslint-v3.log','session-repair-format-final.log'):
 text=(r.out/name).read_text(encoding='utf-8')
 assert 'error TS' not in text and '[warn]' not in text and not re.search(r'\d+ problems? \(',text),name
source=read(out/'source.json')
for name,digest in source['overlays'].items():
 assert hashlib.sha256((ROOT/name).read_bytes()).hexdigest()==digest
 assert hashlib.sha256((out/'web-source'/name).read_bytes()).hexdigest()==digest
 assert len((ROOT/name).read_text(encoding='utf-8').splitlines())<=2000
release=read(out/'release.json');before=read(r.out/'session-repair-platform-before.json');after=r.platform()
assert after['pvc']==before['pvc'] and len(after['pvc'])==6
for name,value in before['deployments'].items():
 if name!='argus-web':assert after['deployments'][name]==value,name
assert after['deployments']['argus-web']['images'][0]==release['image']
rows=a.call('GET','/monitoring/builtins?target_type=kubernetes_cluster&target_id='+r.cluster_id)
enabled=[x for x in rows if x.get('collection',{}).get('desired_enabled')]
assert len(enabled)==3 and all(x['collection']['health']=='healthy' and x['collection']['freshness']=='fresh' for x in enabled)
current=a.call('GET','/alerts/instances?limit=100')
assert not any(x['lifecycle']=='normal' and x['health']=='no_data' and not x.get('started_at') for x in current['items'])
assert read(r.out/'collector-terminal-diagnosis.json')['native_admin_exec_exit']!=0
report={'passed':True,'run_id':r.id,'recorded_at':datetime.now(timezone.utc).isoformat(),'human_acceptance':'pending','release':release,'web_source_overlays_verified':len(source['overlays']),'healthy_native_collections':3,'real_browser_cases':6,'mock_browser_cases':8,'shared_ui_tests':157,'log_lifecycle_tests':4,'strict_tls':True,'normal_login':True,'real_log_requests':True,'actual_pty_and_reconnect':True,'terminal_reported_target':'argus-telemetry/argus-otelcol-agent','reported_terminal_causes':['native_exec_permission_denied','distroless_image_has_no_shell'],'native_access_unchanged':True,'untriggered_no_data_current_rows':0,'formal_health_checks':len(health['checks']),'formal_pvcs_preserved':6,'other_deployments_unchanged':True,'leftover_test_sessions':read(r.out/'session-repair-leftover-cleanup.json')}
r.save('session-feedback-final',report);r.save('session-repair-platform-after',after)
progress=read(ROOT/'docs/plan9/progress.json')
progress['documentation']['pod_log_terminal_feedback']='16-b10a-pod-logs-and-terminal-20261111.md'
feedback={'status':'fixed_deployed_waiting_user_review','record':'../../artifacts/plan9/'+r.id+'/session-feedback-final.json','real_browser_cases':6,'web_image':release['image'],'human_acceptance':'pending'}
progress['current_runtime_validation']['pod_log_terminal_feedback']=feedback
batch=next(x for x in progress['batches'] if x['id']=='B10');child=next(x for x in batch['sub_batches'] if x['id']=='B10a')
for ident in ('B10a-L01','B10a-T01'):
 if ident not in child['fixed_pending_user_review']:child['fixed_pending_user_review'].append(ident)
child['session_feedback']=feedback
(ROOT/'docs/plan9/progress.json').write_text(json.dumps(progress,indent=2)+'\n',encoding='utf-8')
print(json.dumps({k:report[k] for k in ('passed','real_browser_cases','mock_browser_cases','shared_ui_tests','log_lifecycle_tests','formal_health_checks','formal_pvcs_preserved')},indent=2),flush=True)
