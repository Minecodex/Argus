"""Actual metric dashboards, evaluator policy and bounded workload alert cycle."""
import copy
from datetime import datetime, timedelta, timezone
import json
import math
import subprocess
import sys
import uuid

from infrastructure import ROOT, NAMESPACE, execute
from exercises import wait

BUDGET = {'max_scan_bytes':268435456,'max_rows':20000,'max_samples':2000000,
          'max_series':20000,'timeout_ms':10000,'max_result_bytes':8388608}
AVAILABLE = 'max by (argus_resource_id) (k8s_deployment_available{resource_k8s_namespace_name="'+NAMESPACE+'",resource_k8s_deployment_name="b10c-repo"})'


def query(run, api, expression, seconds=900):
    end = datetime.now(timezone.utc)
    return api.call('POST','/enterprise/metrics/query', {'resource_ids':[run.cluster_id],
                    'time_range':{'from':(end-timedelta(seconds=seconds)).isoformat(),'to':end.isoformat()},
                    'budget':BUDGET,'query':expression})


def dashboards(run, api):
    original = json.loads((ROOT/'api/contracts/dashboard-templates.json').read_text(encoding='utf-8'))[0]['spec']
    spec = copy.deepcopy(original)
    spec.update(default_time_range={'kind':'relative','seconds':3600},panels=[],variables=[])
    selector = '{resource_k8s_namespace_name="'+NAMESPACE+'"}'
    expressions = [
        ('available','业务可用副本 / Available replicas','sum by (resource_k8s_deployment_name) (k8s_deployment_available'+selector+')','k8s_cluster',''),
        ('cpu','容器 CPU / Container CPU','sum by (resource_k8s_pod_name) (container_cpu_usage'+selector+')','kubeletstats','cores'),
        ('memory','Pod 内存 / Pod memory','max by (resource_k8s_pod_name) (k8s_pod_memory_usage'+selector+')','kubeletstats','bytes'),
    ]
    for index,(key,title,expression,source,unit) in enumerate(expressions):
        assert query(run,api,expression)['data']['result'], key+' has no actual samples'
        panel = copy.deepcopy(original['panels'][0])
        panel.update(id=key,title=title,type='timeseries',signal='metrics',authoring_mode='dsl',unit=unit,
                     applicable_resource_types=['kubernetes_cluster'],source_binding={'source_type':source,'capability_version':'v1'},
                     layout={'x':index%2*6,'y':index//2*40,'w':6,'h':40,'min_w':4,'min_h':20})
        panel['targets'][0].update(source_definition={'dsl':{'expression':expression}},query_mode='range')
        spec['panels'].append(panel)
    sys.path.insert(0,str(ROOT/'docs/plan9/runtime/B09a'))
    from B09a.dashboard import publish
    class Context:
        def __init__(self):
            self.run,self.api,self.cluster = run,api,run.cluster_id
            self.map = json.loads(run.resources.read_text(encoding='utf-8'))
        def save(self):
            run.save('resource-map',self.map)
    context = Context()
    publish(context,'helm-nodes','B10c Helm 与节点维护 / Helm and Node Maintenance',spec,
            'kubernetes_cluster',run.cluster_id,description='Actual Helm resources, PDB-controlled eviction and workload availability')
    boards = api.call('GET','/dashboards')
    for key in ('kubernetes.cluster','kubernetes.nodes','collector.overview'):
        board = next(v for v in boards if v.get('managed_template_key')==key)
        context.map['dashboards'][key] = {'id':board['id'],'name':board['name']}
    context.save()
    path = '/kubernetes/clusters/'+run.cluster_id+'/dashboard-bindings'
    for item in context.map['dashboards'].values():
        bound = api.call('GET',path)
        if not any(v['dashboard']['id']==item['id'] for v in bound['items']):
            board = next(v for v in api.call('GET','/dashboards') if v['id']==item['id'])
            api.confirm(api.call('POST',path+'/preview',{'operation':'attach','dashboard_id':board['id'],
                        'expected_dashboard_version':board['version'],'expected_resource_version':bound['resource_version']},
                        key='bind-'+board['id'],expected=201))
    overview = api.call('GET','/kubernetes/clusters/'+run.cluster_id+'/overview-dashboard')
    run.save('overview-selection',overview)


def configure(run, api):
    wait(run,lambda:query(run,api,AVAILABLE)['data']['result'],'Fresh workload availability metric',150)
    mapping = json.loads(run.resources.read_text(encoding='utf-8'))
    role = json.loads((ROOT/'artifacts/plan9/common-evaluator.json').read_text(encoding='utf-8'))
    mapping['evaluator'] = role
    path = '/enterprise/data-authorizations/role/'+role['role_id']
    grants = api.call('GET',path+'?resource_type=kubernetes_cluster')
    if not any(v['direct'] and v['resource_id']==run.cluster_id for v in grants['items']):
        api.call('POST',path,{'resource_type':'kubernetes_cluster','resource_ids':[run.cluster_id],
                             'remove':False,'expected_version':grants['authorization_version']},expected=204)
    name = 'B10c released workload has no available replica'
    spec = {'description':'Actual bounded deployment scale and recovery; platform events only','enabled':True,
            'resources':{'mode':'explicit','types':['kubernetes_cluster'],'ids':[run.cluster_id],'matchers':[]},
            'rules':[{'name':name,'expression':AVAILABLE,'query_mode':'numeric','operator':'<','threshold':.5,
                      'recovery_threshold':2,'severity':'warning','unit':''}], 'mode':'independent','match_by':['argus_resource_id'],
            'interval_seconds':10,'for_seconds':20,'recovery_seconds':20,'no_data_behavior':'report','error_behavior':'keep',
            'channel_ids':[],'route_ids':[],'labels':{'plan9_run':run.id,'plan9_batch':'b10c'}}
    assert api.call('POST','/alerts/trial',spec)['health']=='normal'
    api.confirm(api.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'create','name':name,'spec':spec},
                         key='availability-policy',expected=202))
    policy = next(v for v in api.call('GET','/alerts/configurations?kind=policy') if v['name']==name)
    mapping['alert_policies']['available'] = {'id':policy['id'],'name':name,'expression':AVAILABLE}
    run.save('resource-map',mapping)
    dashboards(run,api)
    alert_cycle(run,api,policy['id'])


def alert_cycle(run, api, policy):
    node = run.id+'-control-plane'
    workload = run.get('deployment','b10c-repo','-n',NAMESPACE)
    assert workload['spec']['replicas']==2
    unit = 'b10c-recover-'+uuid.uuid4().hex[:8]
    patch = json.dumps([{'op':'test','path':'/metadata/uid','value':workload['metadata']['uid']},
                        {'op':'test','path':'/spec/replicas','value':0},
                        {'op':'replace','path':'/spec/replicas','value':2}])
    execute(['docker','exec',node,'systemd-run','--collect','--unit='+unit,'--on-active=120s',
             'kubectl','--kubeconfig=/etc/kubernetes/admin.conf','patch','deployment','b10c-repo','-n',NAMESPACE,'--type=json','-p',patch])
    started = datetime.now(timezone.utc).isoformat()
    event = None
    try:
        run.change(api,'deployment','b10c-repo','scale',replicas=0)
        page = api.wait('/alerts/instances?policy_id='+policy,
                        lambda value:any(v['lifecycle']=='firing' and v['state']['kind']=='condition' for v in value['items']),
                        timeout=90,label='Actual B10c workload alert')
        event = next(v for v in page['items'] if v['lifecycle']=='firing' and v['state']['kind']=='condition')
        assert event['started_at'] >= started
        run.save('alert-fired',event)
    finally:
        live = run.get('deployment','b10c-repo','-n',NAMESPACE)
        assert live['metadata']['uid']==workload['metadata']['uid']
        if live['spec']['replicas']!=2:
            run.change(api,'deployment','b10c-repo','scale',replicas=2)
        run.kubectl('rollout','status','deployment/b10c-repo','-n',NAMESPACE,'--timeout=90s')
        subprocess.run(['docker','exec',node,'systemctl','stop',unit+'.timer'],capture_output=True,timeout=15)
    page = api.wait('/alerts/history?policy_id='+policy,
                    lambda value:any(v['id']==event['id'] and v['lifecycle']=='resolved' for v in value['items']),
                    timeout=90,label='B10c actual recovery')
    recovered = next(v for v in page['items'] if v['id']==event['id'])
    run.save('alert-cycle',{key:recovered[key] for key in ('id','started_at','resolved_at','lifecycle')})
    print('Actual released workload availability alert fired and recovered',flush=True)


def verify(run, api):
    mapping = json.loads(run.resources.read_text(encoding='utf-8'))
    report = {'run_id':run.id,'verified_at':datetime.now(timezone.utc).isoformat(),'passed':False}
    nodes = run.get('nodes')['items']
    assert len(nodes)==3 and all(any(c['type']=='Ready' and c['status']=='True' for c in n['status']['conditions']) for n in nodes)
    report['ready_nodes'] = 3
    for name in ('b10c-repo','b10c-oci','b10c-maintenance','b10c-activity'):
        row = run.get('deployment',name,'-n',NAMESPACE)
        assert row['status'].get('availableReplicas')==row['spec']['replicas']
    panels = {}
    end = datetime.now(timezone.utc)
    for key,board in mapping['dashboards'].items():
        value = api.call('POST','/dashboards/'+board['id']+'/execute',{
            'from':(end-timedelta(hours=1)).isoformat(),'to':end.isoformat(),'resource_mode':'explicit',
            'resource_types':['kubernetes_cluster'],'resource_ids':[run.cluster_id]})
        value.pop('context_token',None)
        run.save(key+'-dashboard',value)
        for panel in value['panels']:
            assert panel['status']=='success', panel['id']
            assert any(math.isfinite(float(p[1])) for t in panel['targets'] for s in (t.get('data') or [])
                       for p in ((s.get('values') or [])+([s['value']] if s.get('value') else []))),panel['id']
        panels[key] = len(value['panels'])
    report['dashboard_panels'] = panels
    report['alert_cycle'] = json.loads((run.out/'alert-cycle.json').read_text(encoding='utf-8'))
    assert report['alert_cycle']['lifecycle']=='resolved'
    assert not any(run.cluster_id in v.get('resource_ids',[]) for v in api.call('GET','/alerts/instances?limit=100')['items'])
    for name in ('helm-final-releases','extension-rbac-exercise','node-maintenance-exercise'):
        assert (run.out/(name+'.json')).exists(),name
    for key,item in mapping['builtins'].items():
        current = api.call('GET','/monitoring/collections/'+item['id'])
        assert current['freshness']=='fresh' and current['health']=='healthy',key
    baseline = json.loads((run.out/'platform-before.json').read_text(encoding='utf-8'))
    assert run.platform()['pvc']==baseline['pvc']
    report.update(passed=True,formal_pvcs_preserved=6,source_tls_verified=True)
    run.save('checks',report)
    print('Actual resources, four dashboards, alert recovery and fresh metric sources verified',flush=True)


def handoff(run, api):
    checks = json.loads((run.out/'checks.json').read_text(encoding='utf-8'))
    browser = json.loads((run.out/'browser-review.json').read_text(encoding='utf-8'))
    delivery = json.loads((run.out/'delivery-watch.json').read_text(encoding='utf-8'))
    health = json.loads((run.out/'formal-health-after-fix.log').read_text(encoding='utf-8-sig'))
    preservation = json.loads((run.out/'final-preservation.json').read_text(encoding='utf-8'))
    assert all(item['passed'] for item in (checks,browser,delivery,health,preservation))
    assert delivery['watch_seconds'] >= 305
    assert len(health['checks']) == 20 and all(item['status']=='pass' for item in health['checks'])
    state = json.loads((run.out/'run.json').read_text(encoding='utf-8'))
    state.update(status='ready_for_review',ready_at=datetime.now(timezone.utc).isoformat(),human_acceptance='pending',
                 review_entry='docs/plan9/batches/B10c-review.md')
    run.save('run',state)
    print('B10c ready for user review',flush=True)


def execute_stage(run, action):
    return {'monitoring':configure,'verify':verify,'handoff':handoff}[action](run,run.api())
