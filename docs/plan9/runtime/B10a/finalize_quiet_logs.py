"""Finalize the quiet-agent regression against the exact published images."""
from datetime import datetime,timezone
import hashlib,json,re
from run import Run,ROOT
r=Run();a=r.api();out=r.out/'quiet-pod-repair';read=lambda p:json.loads(p.read_text(encoding='utf-8'))
browser=read(r.out/'quiet-pod-browser.json');assert browser['passed'] and browser['cases']==5
health=read(r.out/'quiet-logs-health.log');assert health['passed'] and all(x['status']=='pass' for x in health['checks'])
assert read(r.out/'latest-tail-postgres.json')['passed']
assert '8 passed' in (r.out/'quiet-logs-mock-browser.log').read_text(encoding='utf-8')
assert '158 passed' in (r.out/'quiet-logs-ui-tests.log').read_text(encoding='utf-8')
assert '4 passed' in (r.out/'quiet-logs-dialog-tests.log').read_text(encoding='utf-8')
sources={'web':read(out/'source.json'),'backend':read(out/'backend-source.json')}
for kind,source in sources.items():
 folder='web-source' if kind=='web' else 'backend-source'
 for name,digest in source['overlays'].items():
  assert hashlib.sha256((ROOT/name).read_bytes()).hexdigest()==digest,(kind,name)
  assert hashlib.sha256((out/folder/name).read_bytes()).hexdigest()==digest,(kind,name)
web=read(out/'release.json');backend=read(r.out/'quiet-pod-backend-release.json')
expected={name:row['image'] for name,row in backend.items()};expected['argus-web']=web['image']
before=read(r.out/'quiet-pod-platform-before-backend.json');after=r.platform();assert before['pvc']==after['pvc'] and len(after['pvc'])==6
for name,row in before['deployments'].items():
 if name in expected:assert after['deployments'][name]['images'][0]==expected[name],name
 else:assert after['deployments'][name]==row,name
builtins=a.call('GET','/monitoring/builtins?target_type=kubernetes_cluster&target_id='+r.cluster_id)
enabled=[x for x in builtins if x.get('collection',{}).get('desired_enabled')]
assert len(enabled)==3 and all(x['collection']['health']=='healthy' and x['collection']['freshness']=='fresh' for x in enabled)
current=a.call('GET','/alerts/instances?limit=100')
assert not any(x['lifecycle']=='normal' and x['health']=='no_data' and not x.get('started_at') for x in current['items'])
proof={'passed':True,'run_id':r.id,'recorded_at':datetime.now(timezone.utc).isoformat(),'human_acceptance':'pending','real_browser_cases':5,'mock_browser_cases':8,'shared_ui_tests':158,'log_lifecycle_tests':4,'postgres_gate':True,'quiet_agent_real_retained_history':True,'default_since_seconds':0,'default_tail_lines':100,'bounded_native_tail_and_boundary_watermark':True,'toolbar_and_footer_geometry_verified':True,'business_pty_and_reconnect':True,'native_exec_permissions_unchanged':True,'debug_container_added':False,'formal_health_checks':len(health['checks']),'formal_pvcs_preserved':6,'healthy_native_collections':3,'source_overlays_verified':{kind:len(value['overlays']) for kind,value in sources.items()},'backend_postgres_schema':sources['backend']['postgres_schema'],'images':expected}
r.save('quiet-pod-final',proof);r.save('quiet-pod-platform-after',after)
progress=read(ROOT/'docs/plan9/progress.json')
feedback={'status':'fixed_deployed_waiting_user_review','record':'../../artifacts/plan9/'+r.id+'/quiet-pod-final.json','real_browser_cases':5,'human_acceptance':'pending'}
progress['documentation']['quiet_pod_logs_and_actions']='17-b10a-quiet-pod-logs-and-actions-20261111.md'
progress['current_runtime_validation']['quiet_pod_followup']=feedback
batch=next(x for x in progress['batches'] if x['id']=='B10');child=next(x for x in batch['sub_batches'] if x['id']=='B10a');child['quiet_pod_followup']=feedback
for issue in ('B10a-L02','B10a-UI03'):
 if issue not in child['fixed_pending_user_review']:child['fixed_pending_user_review'].append(issue)
(ROOT/'docs/plan9/progress.json').write_text(json.dumps(progress,indent=2)+'\n',encoding='utf-8')
print(json.dumps({k:proof[k] for k in ('passed','real_browser_cases','quiet_agent_real_retained_history','formal_health_checks','formal_pvcs_preserved')},indent=2),flush=True)
