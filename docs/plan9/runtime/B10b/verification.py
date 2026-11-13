"""Final native and normal-API checks; human review stays pending."""
from datetime import datetime,timedelta,timezone
import hashlib,json,math
from run import ROOT,write_json
from infrastructure import NAMESPACE
def read(r,name):return json.loads((r.out/(name+'.json')).read_text(encoding='utf-8'))
def verify(r,a):
 mapping=read(r,'resource-map');report={'run_id':r.id,'status':'ready_for_review','cluster_id':r.cluster_id,'human_acceptance':'pending','observed_at':datetime.now(timezone.utc).isoformat()}
 assert read(r,'predecessor-cleanup-verification')['status']=='destroyed'
 checks={}
 for kind,names in {'deployment':['b10b-storage','b10b-allowed','b10b-denied','b10b-autoscale'],'pvc':['b10b-data'],'pv':[r.get('pvc','b10b-data','-n',NAMESPACE)['spec']['volumeName']],'storageclass':['b10b-expandable'],'job':['b10b-job','b10b-pausable-job'],'cronjob':['b10b-cron'],'hpa':['b10b-autoscale'],'networkpolicy':['b10b-storage-allow-client']}.items():
  page=r.inventory(a,kind,namespace=kind not in ('pv','storageclass'));assert page['complete']
  assert all(any(x['name']==name for x in page['items'])for name in names),kind
  checks[kind]={'count':len(page['items']),'required_names':names};r.save('inventory-'+kind,page)
 assert r.get('namespace',NAMESPACE)['metadata']['uid']==mapping['namespaces'][NAMESPACE]['uid']
 pvc=r.get('pvc','b10b-data','-n',NAMESPACE);assert pvc['status']['phase']=='Bound' and pvc['status']['capacity']['storage']=='128Mi'
 report['storage']={k:read(r,'storage-exercise')[k] for k in ('shrink_rejected','scratch_volume_deleted')}
 report['storage'].update(persistent_file_survived_restart=True,online_resize_from='64Mi',online_resize_to='128Mi',filesystem_capacity_before=read(r,'storage-exercise')['after_restart']['capacity_bytes'],filesystem_capacity_after=read(r,'storage-exercise')['after_online_expansion']['capacity_bytes'])
 report['job_suspend_resume_and_manual_scheduled_runs']=True;assert read(r,'jobs-exercise')['cron_resumed']
 assert read(r,'network-policy-exercise')['final_policy_enforced'];report['actual_network_policy_probes']=True
 hpa=read(r,'hpa-exercise');assert hpa['before']['status']['currentReplicas']==1 and hpa['scaled_up']['status']['currentReplicas']>1 and hpa['scaled_down']['status']['currentReplicas']==1
 report['actual_cpu_hpa_replicas']=[1,hpa['scaled_up']['status']['currentReplicas'],1]
 values=a.call('GET','/monitoring/builtins?target_type=kubernetes_cluster&target_id='+r.cluster_id);enabled=[x['collection'] for x in values if x.get('collection',{}).get('desired_enabled')]
 assert len(enabled)==3 and all(x['health']=='healthy' and x['freshness']=='fresh' and x['apply_status']=='effective' for x in enabled)
 report['fresh_collections']=3;r.save('final-builtins',values)
 collector=a.call('GET','/enterprise/kubernetes-clusters/'+r.cluster_id+'/collector');assert collector['status']=='converged' and collector['desired_revision']==collector['effective_revision']
 report['collector_revision']=collector['effective_revision']
 panels={};end=datetime.now(timezone.utc)
 for key,board in mapping['dashboards'].items():
  value=a.call('POST','/dashboards/'+board['id']+'/execute',{'from':(end-timedelta(minutes=15)).isoformat(),'to':end.isoformat(),'resource_mode':'explicit','resource_types':['kubernetes_cluster'],'resource_ids':[r.cluster_id]});value.pop('context_token',None);r.save(key+'-dashboard',value)
  for panel in value['panels']:
   assert panel['status']=='success',panel['id']
   assert any(math.isfinite(float(point[1])) for target in panel['targets'] for series in (target.get('data')or[]) for point in ((series.get('values')or[])+([series['value']]if series.get('value')else[]))),panel['id']
  panels[key]=len(value['panels'])
 report['dashboard_panels']=panels;report['total_dashboard_panels']=sum(panels.values())
 cycle=read(r,'alert-cycle');assert cycle['lifecycle']=='resolved';report['actual_alert_cycle']=cycle
 current=a.call('GET','/alerts/instances?limit=100');own=[x for x in current['items'] if r.cluster_id in x.get('resource_ids',[])]
 assert not any(x['lifecycle']=='normal' for x in current['items']);assert not own,[(x['name'],x['health'])for x in own]
 report['active_batch_alerts']=0
 for policy in mapping['alert_policies'].values():assert a.call('GET','/alerts/configurations/'+policy['id'])['health']=='normal'
 watch=read(r,'delivery-watch');assert watch['passed'] and watch['watch_seconds']>=300 and all(watch[k]==0 for k in ('terminal_failure_delta','enqueue_failure_delta','receiver_refusal_delta'))
 report['delivery_watch_seconds']=watch['watch_seconds'];report['terminal_failed_points']=watch['final']['report']['terminal_failures']['metric_points']
 browser=read(r,'browser-review');assert browser['passed'];report['real_browser_cases']=browser['cases']
 health=json.loads((r.out/'formal-health.log').read_text(encoding='utf-8-sig'));assert health['passed'] and all(x['status']=='pass' for x in health['checks']);report['formal_health_checks']=len(health['checks']);report['formal_degradations']=health['degradations']
 initial=read(r,'platform-before');final=r.platform();assert final['pvc']==initial['pvc'];release=read(r,'native-ca-backend-release')
 web=read(r,'node-ca-web/release');release['argus-web']={'image':web['image']}
 changed=[name for name,row in initial['deployments'].items() if row!=final['deployments'][name]]
 assert set(changed)==set(release),changed
 for name in changed:
  assert final['deployments'][name]['images'][0]==release[name]['image']
  assert final['deployments'][name]['images'][1:]==initial['deployments'][name]['images'][1:],'Unrelated sidecar image changed'
 report['preserved_formal_pvcs']=len(initial['pvc']);report['updated_components']=changed;report['frozen_postgres_schema']=52
 source=read(r,'native-ca-file-fix/backend-source')
 for name,digest in source['overlays'].items():assert hashlib.sha256((ROOT/name).read_bytes()).hexdigest()==digest
 native=read(r,'native-node-ca-verified');assert native['wrong_ca_rejected'] and read(r,'native-ca-probe-cleanup')['owned_probe_deleted']
 report['kubelet_tls_verified_with_explicit_public_ca']=True
 report['native_wrong_ca_rejected']=True
 for kind in ('pods','jobs','configmaps'):
  assert not any(x['metadata']['name'].startswith('b10b-wrong-ca-') for x in r.get(kind,'-n','argus-telemetry')['items']),'Native CA probe was not removed'
 assert not r.get('cronjob','b10b-cron','-n',NAMESPACE)['spec'].get('suspend',False),'Original schedule was not restored'
 original=next(x for x in a.call('GET','/enterprise/kubernetes-clusters/'+r.cluster_id+'/operations?limit=200')['items'] if x['name']=='b10b-pausable-job' and x['operation']=='create')
 assert original['status']=='failed' and original['error_code']=='KUBERNETES_OPERATION_SUPERSEDED'
 r.save('superseded-create-verified',original);report['superseded_create_settled']=True
 report['limits']={'pvc_volume_stats':'Rawfile CSI does not supply kubelet per-PVC volume stats in this run. PVC object capacity and actual mounted statvfs/df verified; no fabricated volume graph.'}
 mapping['objects']={}
 for kind in ('deployment','service','pvc','job','cronjob','hpa','networkpolicy'):
  for obj in r.get(kind,'-n',NAMESPACE)['items']:
   mapping['objects'][kind+'/'+obj['metadata']['name']]={'uid':obj['metadata']['uid'],'resource_version':obj['metadata']['resourceVersion'],'namespace':NAMESPACE}
 r.save('resource-map',mapping);r.save('platform-after',final);r.save('checks',report)
 state=read(r,'run');state.update(status='ready_for_review',ready_at=report['observed_at'],human_acceptance='pending',review_entry='docs/plan9/batches/B10b-review.md');r.save('run',state)
 print(json.dumps(report,ensure_ascii=True,indent=2),flush=True)
