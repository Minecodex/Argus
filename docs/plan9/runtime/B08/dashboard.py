"""Publish and verify the two resource-scoped B08 dashboards."""
import copy
from datetime import datetime, timedelta, timezone
import json

from logs import FAILED
from manage import ROOT, write_json


def specification(kind):
    template = json.loads((ROOT/'api/contracts/dashboard-templates.json').read_text(encoding='utf-8'))[0]['spec']
    base = copy.deepcopy(template)
    base.update(default_time_range={'kind':'relative','seconds':900},panels=[],variables=[])
    panel = copy.deepcopy(template['panels'][0])
    is_app = kind == 'application'
    panel.update(id='logs',title='应用文件日志 / Application file logs' if is_app else '主机 Journald / Host journal',
        type='logs',signal='logs',unit='',authoring_mode='builder',applicable_resource_types=[kind],
        source_binding={'source_type':'filelog' if is_app else 'journald','capability_version':'v1'},
        layout={'x':0,'y':0,'w':12,'h':58,'min_w':4,'min_h':20})
    panel['targets'][0].update(language='kql',query_mode='instant',
        source_definition={'builder':{'operation':'records','filters':[],'group_by':[],'limit':30}})
    base['panels'].append(panel)
    if not is_app:
        service = copy.deepcopy(template['panels'][0])
        service.update(id='failed',title='服务失败状态 / Service failed state',type='timeseries',unit='',
            applicable_resource_types=['host'],source_binding={'source_type':'host_systemd','capability_version':'v1'},
            layout={'x':0,'y':58,'w':12,'h':35,'min_w':4,'min_h':20})
        service['targets'][0].update(source_definition={'dsl':{'expression':FAILED}},query_mode='range')
        base['panels'].append(service)
    return base


def dashboards(logs):
    result = {}
    for kind, ident, name in [('application',logs.app,'B08 应用文件日志 / Application file logs'),
                               ('host',logs.host,'B08 主机 Journald / Host journal')]:
        spec = specification(kind)
        state = logs.map['dashboards'].setdefault(kind,{})
        if not state.get('draft_id'):
            report = logs.api.call('POST','/dashboard-spec/validate',spec)
            assert report['valid'],report
            draft = logs.api.call('POST','/dashboard-drafts',{'name':name,
                'description':'B08 actual logs from the enrolled Linux HTTP service',
                'spec':spec,'proposed_bindings':[{'resource_type':kind,'resource_id':ident}]},
                key=kind+'-board',expected=201)
            state['draft_id'] = draft['id']
            logs.save()
        if not state.get('id'):
            path = '/dashboard-drafts/'+state['draft_id']
            draft = logs.api.call('GET',path)
            if not draft.get('dashboard_id'):
                logs.api.confirm(logs.api.call('POST',path+'/preview',{'expected_version':draft['draft_version']},
                                               key='publish-'+kind+'-board',expected=201))
                draft = logs.api.call('GET',path)
            state.update(id=draft['dashboard_id'],name=name)
            logs.save()
        end = datetime.now(timezone.utc)
        board = logs.api.call('POST','/dashboards/'+state['id']+'/execute',{
            'from':(end-timedelta(minutes=15)).isoformat(),'to':end.isoformat(),
            'resource_mode':'explicit','resource_types':[kind],'resource_ids':[ident]})
        board.pop('context_token',None)
        write_json(logs.run.out/(kind+'-dashboard.json'),board)
        result[kind] = {panel['id']:panel['status'] for panel in board['panels']}
    write_json(logs.run.out/'dashboard-checks.json',result)
    assert result == {'application':{'logs':'success'},'host':{'logs':'success','failed':'success'}},result
    print('Actual application and host dashboards passed:',result,flush=True)
