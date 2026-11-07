"""Normal API entry points for B08. The session path must point to private storage."""
import argparse
import json

from manage import Run, execute, now, write_json

from common.api import Api
from common.retirement import retire
from dashboard import dashboards
from logs import Logs
from scenario import configure_alert, exercise_alert, restart_and_pause, rotation, verify_redaction


def enroll(logs):
    logs.onboard()
    logs.collectors()
    logs.builtins(selected=['host_metrics','host_systemd_metrics','collector_metrics'])
    logs.setup()


def cleanup(logs):
    run,api=logs.run,logs.api
    checkpoint = run.out/'cleanup.json'
    if checkpoint.exists():
        state = json.loads(checkpoint.read_text(encoding='utf-8'))
        if state['status']=='destroyed':
            return
        if 'compose-destroyed' in state['steps']:
            retire(run,api,execute,write_json,now)
            return
    run.shell('collector','systemctl stop plan9-b08-load.service\n')
    for key,state in logs.map['log_rules'].items():
        row=logs.collection(key)
        assert row['target']['id']==state['resource'] and row['collector_id']==logs.collector
        if row['status']=='active':
            logs.command({'kind':'collection','operation':'archive','id':row['id'],
                          'expected_version':row['version'],'collection':{}},'archive-'+key)
        logs.converge()
    for state in logs.map['dashboards'].values():
        row=api.call('GET','/dashboards/'+state['id'])['dashboard']
        if row['lifecycle']=='active':
            api.confirm(api.call('POST','/dashboards/'+row['id']+'/actions/preview',{
                'id':row['id'],'operation':'archive','expected_version':row['version'],
                'name':row['name'],'description':row.get('description',''),'sort_order':row.get('sort_order',0)},
                key='retire-board-'+row['id'],expected=201))
    row=api.call('GET','/applications/'+logs.app)
    if row['status']=='active':
        logs.command({'kind':'application','operation':'archive','id':row['id'],'expected_version':row['version'],
            'application':{k:row[k] for k in ('name','environment','description','owner_name') if k in row}},'retire-app')
    state=retire(run,api,execute,write_json,now)
    assert state['status']=='destroyed'
    rows=api.call('GET','/monitoring/collections?limit=100')['items']
    assert not any(c['id'] in {entry['id'] for entry in logs.map['log_rules'].values()} for c in rows)


if __name__=='__main__':
    actions={'enroll':enroll,'setup':Logs.setup,'rotation':rotation,'lifecycle':restart_and_pause,
             'alerts':configure_alert,'exercise':exercise_alert,'dashboards':dashboards,'redaction':verify_redaction,'retire':cleanup}
    p=argparse.ArgumentParser()
    p.add_argument('action',choices=actions)
    p.add_argument('--session',required=True)
    p.add_argument('--ca',required=True)
    args=p.parse_args()
    run=Run()
    api=Api(run,args.session,args.ca)
    api.call('GET','/enterprise/auth/session')
    actions[args.action](Logs(run,api))
