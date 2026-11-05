"""Publish a batch-owned real-data dashboard through the normal draft workflow."""
from datetime import datetime,timedelta,timezone
import copy
import json
from manage import ROOT,write_json

METRICS=[
 ('requests','请求速率 / Request rate','sum by (resource_service_name) (rate(plan9_http_requests_total[1m]))','reqps','timeseries'),
 ('errors','错误率 / Error ratio','sum by (resource_service_name) (rate(plan9_http_errors_total[1m])) / sum by (resource_service_name) (rate(plan9_http_requests_total[1m]))','percent_ratio','timeseries'),
 ('latency','平均耗时 / Mean duration','sum by (resource_service_name) (rate(plan9_http_duration_seconds_sum[1m])) / sum by (resource_service_name) (rate(plan9_http_duration_seconds_count[1m]))','s','timeseries'),
 ('p95','P95 耗时 / P95 duration','histogram_quantile(0.95, sum by (resource_service_name, le) (rate(plan9_http_duration_seconds_bucket[1m])))','s','timeseries'),
 ('active','处理中的请求 / Active requests','plan9_http_active_requests','','timeseries'),
 ('uptime','服务运行时长 / Service uptime','plan9_service_uptime_seconds','s','stat')]

def specification(trace_source='otlp'):
    base=json.loads((ROOT/'api/contracts/dashboard-templates.json').read_text(encoding='utf-8'))[0]['spec'];base=copy.deepcopy(base);base.update(default_time_range={'kind':'relative','seconds':900},panels=[],variables=[])
    template=json.loads((ROOT/'api/contracts/dashboard-templates.json').read_text(encoding='utf-8'))[0]['spec']['panels'][0]
    for index,(key,title,expression,unit,kind) in enumerate(METRICS):
        panel=copy.deepcopy(template);panel.update(id=key,title=title,type=kind,unit=unit,applicable_resource_types=['application'],source_binding={'source_type':'otlp','capability_version':'v1'},layout={'x':index%2*6,'y':index//2*36,'w':6,'h':36,'min_w':2,'min_h':20});panel['targets'][0].update(source_definition={'dsl':{'expression':expression}},query_mode='instant' if kind=='stat' else 'range');base['panels'].append(panel)
    for index,(key,title,kind,operation,signal) in enumerate([('services','服务概览 / Services','apm_services','apm_services','traces'),('topology','服务依赖 / Service topology','apm_topology','apm_topology','traces'),('traces','实际调用链 / Real traces','trace_list','list','traces'),('logs','业务日志 / Business logs','logs','records','logs')],start=6):
        panel=copy.deepcopy(template);panel.update(id=key,title=title,type=kind,signal=signal,unit='',authoring_mode='builder',applicable_resource_types=['application'],source_binding={'source_type':'otlp','capability_version':'v1'},layout={'x':index%2*6,'y':108+(index-6)//2*46,'w':6,'h':46,'min_w':2,'min_h':20})
        if signal == 'traces':
            panel['source_binding']['source_type'] = trace_source
        panel['targets'][0].update(language='skywalking_graphql' if signal=='traces' else 'kql',source_definition={'builder':{'operation':operation,'filters':[],'group_by':[],'limit':20}},query_mode='instant');base['panels'].append(panel)
    return base

def dashboards(platform):
    platform.dashboards();name=f'{platform.run.batch} {platform.run.protocol} 应用 / {platform.run.protocol} application';app=platform.map['applications']['orders'];state=platform.map.setdefault('batch_dashboard',{})
    if 'draft_id' not in state:
        description = f'Actual gateway -> worker -> Redis requests. Trace source: {platform.run.trace_source}. Metrics and logs: OTLP.'
        draft=platform.api.call('POST','/dashboard-drafts',{'name':name,'description':description,'spec':specification(platform.run.trace_source),'proposed_bindings':[{'resource_type':'application','resource_id':app['id']}]},key='batch-dashboard-draft',expected=201)
        state['draft_id']=draft['id'];platform.save()
    if 'id' not in state:
        draft=platform.api.call('GET','/dashboard-drafts/'+state['draft_id'])
        if not draft.get('dashboard_id'):
            spec=specification(platform.run.trace_source)
            if draft['spec']!=spec:
                draft=platform.api.call('PATCH','/dashboard-drafts/'+state['draft_id'],{'name':draft['name'],'description':draft['description'],'spec':spec,'proposed_bindings':[{'resource_type':'application','resource_id':app['id']}],'expected_version':draft['draft_version']})
            report=platform.api.call('POST','/dashboard-spec/validate',spec);write_json(platform.run.out/'dashboard-static-validation.json',report);assert report['valid']
            platform.api.confirm(platform.api.call('POST','/dashboard-drafts/'+state['draft_id']+'/preview',{'expected_version':draft['draft_version']},key='publish-dashboard',expected=201));draft=platform.api.call('GET','/dashboard-drafts/'+state['draft_id'])
        state.update(id=draft['dashboard_id'],name=name);platform.save()
    end=datetime.now(timezone.utc);result=platform.api.call('POST','/dashboards/'+state['id']+'/execute',{'from':(end-timedelta(minutes=15)).isoformat(),'to':end.isoformat(),'resource_mode':'explicit','resource_types':['application'],'resource_ids':[app['id']]});result.pop('context_token',None);write_json(platform.run.out/'application-dashboard.json',result);summary=[{'id':p['id'],'status':p['status']} for p in result['panels']];write_json(platform.run.out/'application-dashboard-checks.json',summary);print('Application dashboard:',summary,flush=True);assert len(summary)==10 and all(p['status']=='success' for p in summary)
