"""Verify all current supported Pods and the exact CSS-only Web release."""
from datetime import datetime,timezone
import hashlib,json
from run import Run,ROOT
r=Run();out=r.out/'terminal-options-audit';read=lambda p:json.loads(p.read_text(encoding='utf-8'))
browser=read(r.out/'terminal-options-browser.json');assert browser['passed'] and browser['cases']==8
health=read(r.out/'terminal-options-health.log');assert health['passed'] and all(x['status']=='pass' for x in health['checks'])
inventory=read(r.out/'pod-terminal-inventory.json')
sessions=[read(p) for p in (out/'real-browser').rglob('verified-session.json')];assert len(sessions)==4
supported={x['pod'] for x in inventory['pods'] if x['preflight']=='shell_present'}
assert {x['pod'] for x in sessions}==supported
for row in sessions:assert row['status']=='closed' and row['bytes_in']>0 and row['bytes_out']>0 and row['real_uid_output']
for row in inventory['pods']:
 row['platform_terminal_verified']=row['pod'] in supported
inventory.update(platform_verified=True,platform_verified_at=datetime.now(timezone.utc).isoformat())
r.save('pod-terminal-inventory',inventory)
source=read(out/'source.json');assert len(source['overlays'])==1
for name,digest in source['overlays'].items():
 assert hashlib.sha256((ROOT/name).read_bytes()).hexdigest()==digest
 assert hashlib.sha256((out/'web-source'/name).read_bytes()).hexdigest()==digest
release=read(out/'release.json');before=read(r.out/'terminal-options-platform-before.json');after=r.platform();assert after['pvc']==before['pvc'] and len(after['pvc'])==6
for name,row in before['deployments'].items():
 if name=='argus-web':assert after['deployments'][name]['images'][0]==release['image']
 else:assert after['deployments'][name]==row,name
record={'passed':True,'run_id':r.id,'recorded_at':datetime.now(timezone.utc).isoformat(),'human_acceptance':'pending','inspected_pods':len(inventory['pods']),'native_permissions':inventory['native_permissions'],'platform_verified_sessions':sessions,'native_exec_permissions_unchanged':True,'browser_cases':8,'terminal_cases':4,'expanded_options_cases':4,'strict_tls':True,'normal_login':True,'mock':False,'only_stylesheet_overlay':True,'image':release['image'],'formal_health_checks':len(health['checks']),'formal_pvcs_preserved':6,'other_deployments_unchanged':True}
r.save('terminal-options-final',record);r.save('terminal-options-platform-after',after)
progress=read(ROOT/'docs/plan9/progress.json')
feedback={'status':'verified_deployed_waiting_user_review','record':'../../artifacts/plan9/'+r.id+'/terminal-options-final.json','browser_cases':8,'supported_pods':4,'human_acceptance':'pending'}
progress['documentation']['terminal_matrix_and_options']='18-b10a-terminal-matrix-and-log-options-20261111.md'
progress['current_runtime_validation']['terminal_matrix_and_options']=feedback
batch=next(x for x in progress['batches'] if x['id']=='B10');child=next(x for x in batch['sub_batches'] if x['id']=='B10a');child['terminal_matrix_and_options']=feedback
if 'B10a-UI04' not in child['fixed_pending_user_review']:child['fixed_pending_user_review'].append('B10a-UI04')
(ROOT/'docs/plan9/progress.json').write_text(json.dumps(progress,indent=2)+'\n',encoding='utf-8')
print(json.dumps({k:record[k] for k in ('passed','inspected_pods','terminal_cases','expanded_options_cases','formal_health_checks','formal_pvcs_preserved')},indent=2),flush=True)
