"""Managed and batch-scoped dashboards backed by real B09a native sources."""
import copy
from datetime import datetime,timedelta,timezone
import json

from manage import APP_NS,ROOT,write_json


def execute_board(batch,ident,resource,kind,name):
    end=datetime.now(timezone.utc)
    value=batch.api.call('POST','/dashboards/'+ident+'/execute',{
        'from':(end-timedelta(minutes=15)).isoformat(),'to':end.isoformat(),
        'resource_mode':'explicit','resource_types':[kind],'resource_ids':[resource]})
    value.pop('context_token',None)
    write_json(batch.run.out/(name+'-dashboard.json'),value)
    return [{'id':panel['id'],'status':panel['status']} for panel in value['panels']]


def publish(batch,key,name,spec,kind,resource,description='Real B09a observations'):
    state=batch.map['dashboards'].setdefault(key,{})
    if not state.get('draft_id'):
        checked=batch.api.call('POST','/dashboard-spec/validate',spec)
        assert checked['valid'],checked
        draft=batch.api.call('POST','/dashboard-drafts',{'name':name,'description':description,
            'spec':spec,'proposed_bindings':[{'resource_type':kind,'resource_id':resource}]},
            key='draft-'+key,expected=201)
        state['draft_id']=draft['id']
        batch.save()
    if not state.get('id'):
        base='/dashboard-drafts/'+state['draft_id']
        row=batch.api.call('GET',base)
        if not row.get('dashboard_id'):
            batch.api.confirm(batch.api.call('POST',base+'/preview',{'expected_version':row['draft_version']},
                key='publish-'+key,expected=201))
            row=batch.api.call('GET',base)
        state.update(id=row['dashboard_id'],name=name)
        batch.save()
    return state['id']


def specification(application=False):
    original=json.loads((ROOT/'api/contracts/dashboard-templates.json').read_text(encoding='utf-8'))[0]['spec']
    spec=copy.deepcopy(original)
    spec.update(default_time_range={'kind':'relative','seconds':900},panels=[],variables=[])
    if application:
        panel=copy.deepcopy(original['panels'][0])
        panel.update(id='logs',title='容器 stdout / stderr',type='logs',signal='logs',unit='',authoring_mode='builder',
            applicable_resource_types=['application'],source_binding={'source_type':'filelog','capability_version':'v1'},
            layout={'x':0,'y':0,'w':12,'h':75,'min_w':4,'min_h':20})
        panel['targets'][0].update(language='kql',query_mode='instant',
            source_definition={'builder':{'operation':'records','filters':[],'group_by':[],'limit':40}})
        spec['panels']=[panel]
        return spec
    select='{resource_k8s_namespace_name="'+APP_NS+'"}'
    panels=[
        ('available','Deployment 可用副本 / Available replicas','sum by (resource_k8s_deployment_name) (k8s_deployment_available'+select+')','k8s_cluster',''),
        ('desired','Deployment 期望副本 / Desired replicas','sum by (resource_k8s_deployment_name) (k8s_deployment_desired'+select+')','k8s_cluster',''),
        ('restarts','容器重启 / Container restarts','max by (resource_k8s_pod_name, resource_k8s_container_name) (k8s_container_restarts'+select+')','k8s_cluster',''),
        ('cpu','容器 CPU / Container CPU','sum by (resource_k8s_pod_name, resource_k8s_container_name) (container_cpu_usage'+select+')','kubeletstats','cores'),
        ('memory','Pod 内存 / Pod memory','max by (resource_k8s_pod_name) (k8s_pod_memory_usage'+select+')','kubeletstats','bytes'),
    ]
    for index,(key,title,expression,source,unit) in enumerate(panels):
        panel=copy.deepcopy(original['panels'][0])
        panel.update(id=key,title=title,type='timeseries',signal='metrics',authoring_mode='dsl',unit=unit,applicable_resource_types=['kubernetes_cluster','kubernetes_namespace'],
            source_binding={'source_type':source,'capability_version':'v1'},
            layout={'x':index%2*6,'y':index//2*36,'w':6,'h':36,'min_w':4,'min_h':20})
        panel['targets'][0].update(source_definition={'dsl':{'expression':expression}},query_mode='range')
        spec['panels'].append(panel)
    return spec


def dashboards(batch):
    boards=batch.api.call('GET','/dashboards')
    reports={}
    for key in ('kubernetes.cluster','kubernetes.nodes','collector.overview'):
        row=next(row for row in boards if row.get('managed_template_key')==key)
        state=batch.map['dashboards'].setdefault(key,{'id':row['id'],'name':row['name']})
        batch.save()
        path='/kubernetes/clusters/'+batch.cluster+'/dashboard-bindings'
        bound=batch.api.call('GET',path)
        if not any(x['dashboard']['id']==row['id'] for x in bound['items']):
            batch.api.confirm(batch.api.call('POST',path+'/preview',{
                'operation':'attach','dashboard_id':row['id'],'expected_dashboard_version':row['version'],
                'expected_resource_version':bound['resource_version']},key='bind-'+key,expected=201))
        reports[key]=execute_board(batch,row['id'],batch.cluster,'kubernetes_cluster',key)
    ident=publish(batch,'workloads','B09a 工作负载 / Workloads',specification(),
                  'kubernetes_cluster',batch.cluster)
    reports['workloads']=execute_board(batch,ident,batch.cluster,'kubernetes_cluster','workloads')
    reports['namespace']=execute_board(batch,ident,batch.namespace,'kubernetes_namespace','namespace')
    ident=publish(batch,'logs','B09a 容器日志 / Container logs',specification(True),
                  'application',batch.map['application']['id'])
    reports['logs']=execute_board(batch,ident,batch.map['application']['id'],'application','logs')
    write_json(batch.run.out/'dashboard-checks.json',reports)
    print('Dashboard panel states:',reports,flush=True)
    assert all(panel['status']=='success' for rows in reports.values() for panel in rows),reports
