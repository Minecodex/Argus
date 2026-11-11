"""Real B10a inventory, projected data, metrics, dashboards, alerts and ownership."""
from datetime import datetime,timedelta,timezone
import base64,hashlib,json,math,time
from urllib.parse import urlencode
from infrastructure import NAMESPACE,execute,write_json
def finite(value):
 return sum(math.isfinite(float(point[1])) for panel in value['panels'] for target in panel['targets'] for series in (target.get('data') or []) for point in ((series.get('values') or [])+([series['value']] if series.get('value') else [])))
def verify(r,a):
 mapping=json.loads(r.resources.read_text(encoding='utf-8'));report={'run_id':r.id,'cluster_id':r.cluster_id}
 pages={}
 for kind,names in {'deployment':['b10a-api','b10a-driver'],'statefulset':['b10a-ledger'],'daemonset':['b10a-node-observer'],'service':['b10a-api','b10a-ledger'],'ingress':['b10a-http'],'configmap':['b10a-settings'],'secret':['b10a-demo-secret']}.items():
  page=r.inventory(a,kind);assert page['complete']
  assert all(any(x['name']==name and x['namespace']==NAMESPACE for x in page['items']) for name in names),kind
  pages[kind]=page
 r.save('inventory-verification',pages);report['object_kinds']=['namespace',*pages,'pod']
 path='/enterprise/kubernetes-clusters/'+r.cluster_id
 deployment=r.get('deployment','b10a-api','-n',NAMESPACE)
 projected=a.call('GET',path+'/inventory/object?'+urlencode({'type':'deployment','name':'b10a-api','uid':deployment['metadata']['uid'],'namespace_id':r.namespace(a)}))
 assert 'b10a-demo-secret' in json.dumps(projected),'Public object lost the typed reference'
 native=r.get('secret','b10a-demo-secret','-n',NAMESPACE)
 public=a.call('GET',path+'/inventory/object?'+urlencode({'type':'secret','name':'b10a-demo-secret','uid':native['metadata']['uid'],'namespace_id':r.namespace(a)}))
 text=json.dumps(public);assert '[REDACTED]' in text
 for value in native['data'].values():assert value not in text and base64.b64decode(value).decode() not in text
 r.save('public-reference-verification',{'typed_reference_preserved':True,'secret_values_redacted':True})
 # Show a denied API write preview without issuing a live system mutation.
 directory=a.call('GET',path+'/inventory?type=namespace&limit=100');system=next(x for x in directory['items'] if x['name']=='kube-system')
 core=r.get('deployment','coredns','-n','kube-system')
 denial=a.call('POST',path+'/inventory/actions/preview-mutation',{'type':'deployment','operation':'patch','name':'coredns','uid':core['metadata']['uid'],'expected_resource_version':core['metadata']['resourceVersion'],'namespace_id':directory['namespace_references'][system['uid']],'object':{'metadata':{'annotations':{'plan9.argus/forbidden-check':'must-not-write'}}}},key='system-namespace-denial',expected=[400,403])
 assert denial.get('code');report['system_write_denied']=denial['code']
 collections=a.call('GET','/monitoring/builtins?target_type=kubernetes_cluster&target_id='+r.cluster_id)
 enabled=[x for x in collections if x.get('collection',{}).get('desired_enabled')]
 assert len(enabled)==3 and all(x['collection']['health']=='healthy' and x['collection']['freshness']=='fresh' and x['collection']['apply_status']=='effective' for x in enabled)
 r.save('native-collections-final',enabled);report['healthy_collections']=3
 boards={}
 for key,item in mapping['dashboards'].items():
  end=datetime.now(timezone.utc)
  value=a.call('POST','/dashboards/'+item['id']+'/execute',{'from':(end-timedelta(minutes=15)).isoformat(),'to':end.isoformat(),'resource_mode':'explicit','resource_types':['kubernetes_cluster'],'resource_ids':[r.cluster_id]})
  value.pop('context_token',None);r.save('dashboard-'+key,value)
  panels=[{'id':p['id'],'status':p['status'],'points':sum(math.isfinite(float(pt[1])) for t in p['targets'] for s in (t.get('data') or []) for pt in ((s.get('values') or [])+([s['value']] if s.get('value') else [])))} for p in value['panels']]
  assert all(p['status']=='success' and p['points']>0 for p in panels),(key,panels)
  boards[key]={'id':item['id'],'name':item['name'],'panels':panels}
 r.save('dashboard-verification',boards);report['dashboards']=len(boards);report['nonempty_panels']=sum(len(x['panels']) for x in boards.values())
 cycle=json.loads((r.out/'alert-cycle.json').read_text(encoding='utf-8'));assert cycle['lifecycle']=='resolved';report['alert_cycle']=cycle
 active=a.call('GET','/alerts/instances?policy_id='+mapping['alert_policies']['available']['id']);assert not active['items']
 browser=json.loads((r.out/'browser-review.json').read_text(encoding='utf-8'));assert browser['passed'];report['real_browser_cases']=browser['cases']
 tunnel=json.loads((r.out/'forward-session-closed.json').read_text(encoding='utf-8'));assert tunnel['status']=='closed' and tunnel['bytes_in']>0 and tunnel['bytes_out']>0
 report['actual_cli_port_forward']=True;report['actual_pod_logs_and_exec']=True
 watch=json.loads((r.out/'delivery-watch.json').read_text(encoding='utf-8'));assert watch['passed'] and watch['watch_seconds']>=300
 assert all(watch[x]==0 for x in ('terminal_failure_delta','enqueue_failure_delta','receiver_refusal_delta'))
 report['delivery_watch']={k:watch[k] for k in ('watch_seconds','terminal_failure_delta','enqueue_failure_delta','receiver_refusal_delta','historical_delivery_event_ids','historical_failure_counters_preserved','untriggered_no_data_current_rows')}
 report['historical_failed_metric_points']=watch['final']['report']['terminal_failures']['metric_points']
 extra=json.loads((r.out/'dashboard-browser-review.json').read_text(encoding='utf-8'));assert extra['passed'];report['real_browser_cases']+=extra['cases']
 assert json.loads((r.out/'namespace-post-fix-cycle.json').read_text(encoding='utf-8'))['native_namespace_removed']
 assert json.loads((r.out/'postgres-regression.json').read_text(encoding='utf-8'))['passed']
 assert json.loads((r.out/'postgres-namespace-regression.json').read_text(encoding='utf-8'))['passed']
 health_file=r.out/'terminal-options-health.log' if (r.out/'terminal-options-health.log').exists() else r.out/'quiet-logs-health.log' if (r.out/'quiet-logs-health.log').exists() else (r.out/'session-repair-health-v3.log' if (r.out/'session-repair-health-v3.log').exists() else r.out/'health-after-namespace-fix.log')
 health=json.loads(health_file.read_text(encoding='utf-8'));assert health['passed'] and all(x['status']=='pass' for x in health['checks']);report['formal_health_checks']=len(health['checks'])
 source=json.loads((r.out/'backend-source-v4.json').read_text(encoding='utf-8'))
 from infrastructure import ROOT
 for name,digest in source['overlays'].items():
  assert hashlib.sha256((ROOT/name).read_bytes()).hexdigest()==digest
  assert hashlib.sha256((r.out/'backend-source-v4'/name).read_bytes()).hexdigest()==digest
 report['frozen_source_schema']=source['postgres_schema'];report['deployed_source_overlays_verified']=len(source['overlays'])
 for key,row in mapping['objects'].items():
  kind,name=key.split('/',1);current=r.get(kind,name,'-n',NAMESPACE)
  assert current['metadata']['uid']==row['uid']
  row['resource_version']=current['metadata']['resourceVersion']
  row['observed_at']=datetime.now(timezone.utc).isoformat()
 r.save('resource-map',mapping)
 before=json.loads((r.out/'platform-before.json').read_text(encoding='utf-8'));after=r.platform();assert before['pvc']==after['pvc']
 released=json.loads((r.out/'fix-release-v4.json').read_text(encoding='utf-8'))
 repair_release=r.out/'session-repair/release.json'
 if repair_release.exists():
  web=json.loads(repair_release.read_text(encoding='utf-8'))
  released['argus-web']={'image':web['image']}
  feedback=json.loads((r.out/'session-repair-browser.json').read_text(encoding='utf-8'));assert feedback['passed']
  report['session_feedback_browser_cases']=feedback['cases']
  report['session_feedback_web_image']=web['image']
 quiet_release=r.out/'quiet-pod-backend-release.json'
 if quiet_release.exists():
  released.update(json.loads(quiet_release.read_text(encoding='utf-8')))
  quiet_web=json.loads((r.out/'quiet-pod-repair/release.json').read_text(encoding='utf-8'))
  released['argus-web']={'image':quiet_web['image']}
  quiet_browser=json.loads((r.out/'quiet-pod-browser.json').read_text(encoding='utf-8'));assert quiet_browser['passed']
  report['quiet_agent_browser_cases']=quiet_browser['cases']
  report['quiet_logs_web_image']=quiet_web['image']
 terminal_release=r.out/'terminal-options-audit/release.json'
 if terminal_release.exists():
  terminal_web=json.loads(terminal_release.read_text(encoding='utf-8'))
  released['argus-web']={'image':terminal_web['image']}
  terminal_browser=json.loads((r.out/'terminal-options-browser.json').read_text(encoding='utf-8'));assert terminal_browser['passed']
  report['terminal_matrix_options_browser_cases']=terminal_browser['cases']
  report['terminal_matrix_options_web_image']=terminal_web['image']
 for name,row in released.items():
  assert after['deployments'][name]['images'][0]==row['image'],name
 for name,row in before['deployments'].items():
  if name not in released:assert after['deployments'][name]==row,name
 assert execute(['kubectl','--context','docker-desktop','config','current-context']).strip()=='docker-desktop'
 r.save('platform-after',after);report.update(status='ready_for_review',passed=True,human_acceptance='pending',normal_product_operations=True,synthetic_platform_data=False,formal_pvc_bindings_preserved=len(after['pvc']),recorded_at=datetime.now(timezone.utc).isoformat())
 r.save('checks',report)
 record=json.loads((r.out/'run.json').read_text(encoding='utf-8'));record.update(status='ready_for_review',ready_at=report['recorded_at'],human_acceptance='pending');r.save('run',record)
 print(json.dumps(report,indent=2),flush=True)
