"""Idempotent retirement of owned B09a product resources and the Kind cluster."""
import json

from integration import Batch
from manage import RUN,execute,now,write_json


def retire(batch):
    run,api=batch.run,batch.api
    file=run.out/'cleanup.json'
    state=json.loads(file.read_text(encoding='utf-8')) if file.exists() else {
        'run_id':RUN,'status':'destroying','started_at':now(),'steps':[],'history_preserved':True}
    if state['status']=='destroyed':
        return
    def stage(name,action):
        if name in state['steps']:
            return
        action()
        state['steps'].append(name)
        write_json(file,state)
    def policies():
        for row in batch.map['alert_policies'].values():
            current=api.call('GET','/alerts/configurations/'+row['id'],expected=[200,404])
            if not current.get('code'):
                api.confirm(api.call('POST','/alerts/configurations/preview',{'kind':'policy',
                    'operation':'delete','id':current['id'],'expected_version':current['version']},
                    key='retire-policy-'+row['id'],expected=202))
    stage('policies-retired',policies)
    def rules():
        for row in batch.map['log_rules'].values():
            current=api.call('GET','/monitoring/collections/'+row['id'])
            assert current['collector_id']==batch.collector
            if current['status']=='active':
                batch.command({'kind':'collection','operation':'archive','id':row['id'],
                    'expected_version':current['version'],'collection':{}},'archive-rule-'+row['id'])
                batch.converge()
    stage('log-rules-archived',rules)
    def builtins():
        base='/monitoring/builtins?target_type=kubernetes_cluster&target_id='+batch.cluster
        for row in api.call('GET',base):
            collection=row.get('collection')
            if collection and collection['desired_enabled']:
                api.confirm(api.call('POST','/monitoring/builtins/preview',{
                    'target':{'kind':'kubernetes_cluster','id':batch.cluster},'key':row['definition']['key'],
                    'enabled':False,'expected_version':collection['version'],'config':collection['config']},
                    key='retire-builtin-'+collection['id'],expected=201))
                batch.converge()
    stage('builtins-stopped',builtins)
    def dashboards():
        for row in batch.map['dashboards'].values():
            if not row.get('draft_id'):
                continue
            current=api.call('GET','/dashboards/'+row['id'])['dashboard']
            if current['lifecycle']=='active':
                api.confirm(api.call('POST','/dashboards/'+row['id']+'/actions/preview',{
                    'id':row['id'],'operation':'archive','expected_version':current['version'],
                    'name':current['name'],'description':current.get('description',''),
                    'sort_order':current.get('sort_order',0)},key='archive-board-'+row['id'],expected=201))
    stage('custom-dashboards-archived',dashboards)
    def application():
        app=batch.map['application']
        if not app.get('id'):
            return
        current=api.call('GET','/applications/'+app['id'])
        if current['status']=='active':
            batch.command({'kind':'application','operation':'archive','id':app['id'],
                'expected_version':current['version'],'application':{
                    k:current[k] for k in ('name','environment','description','owner_name') if k in current}},
                'archive-application')
    stage('application-archived',application)
    def uninstall():
        base='/enterprise/kubernetes-clusters/'+batch.cluster+'/collector'
        current=api.call('GET',base)
        if current['status']!='uninstalled':
            api.confirm(api.call('POST',base+'/actions/preview-uninstall',{
                'distribution_version_id':current['distribution_version_id'],'profile_ids':[],
                'route_kind':'direct_argus','transport':'direct','expected_version':current['version']},
                key='uninstall-collector-'+str(current['version']),expected=201))
            api.wait(base,lambda row:row['status']=='uninstalled',timeout=180,label='Collector uninstalled')
        for kind,name in [('deployment','argus-otelcol-gateway'),('daemonset','argus-otelcol-agent'),
                          ('pvc','argus-otelcol-gateway-state')]:
            assert not run.kubectl('get',kind,name,'-n','argus-telemetry','--ignore-not-found','-o','name')
    stage('collector-uninstalled',uninstall)
    def cluster():
        base='/enterprise/kubernetes-clusters/'+batch.cluster
        current=api.call('GET',base,expected=[200,404])
        if not current.get('code'):
            assert current['labels']['plan9_run']==RUN
            api.confirm(api.call('POST',base+'/actions/preview-delete',
                {'expected_version':current['resource_version']},key='delete-cluster',expected=201))
    stage('cluster-retired',cluster)
    def credentials():
        account=batch.map['cluster']
        rows=api.call('GET','/enterprise/credentials')['items']
        for prior in account.get('retired_credentials',[]):
            old=next(row for row in rows if row['id']==prior['credential_id'])
            if old['status']=='active':
                api.call('PUT','/enterprise/credentials/'+old['id'],{'status':'disabled','expected_version':old['version']})
            secret=api.call('GET','/enterprise/secrets/'+prior['secret_id'])
            if secret['status']=='active':
                api.call('DELETE','/enterprise/secrets/'+secret['id']+'?expected_version='+str(secret['version']),expected=204)
        row=next(row for row in rows if row['id']==account['credential_id'])
        if row['status']=='active':
            api.call('PUT','/enterprise/credentials/'+row['id'],
                     {'status':'disabled','expected_version':row['version']})
        secret=api.call('GET','/enterprise/secrets/'+account['secret_id'])
        if secret['status']=='active':
            api.call('DELETE','/enterprise/secrets/'+secret['id']+'?expected_version='+str(secret['version']),expected=204)
        if batch.map.get('evaluator'):
            path='/enterprise/data-authorizations/role/'+batch.map['evaluator']['role_id']
            grants=api.call('GET',path+'?resource_type=kubernetes_cluster')
            if any(row['direct'] and row['resource_id']==batch.cluster for row in grants['items']):
                api.call('POST',path,{'resource_type':'kubernetes_cluster','resource_ids':[batch.cluster],
                    'remove':True,'expected_version':grants['authorization_version']},expected=204)
    stage('credentials-and-grants-revoked',credentials)
    def telemetry_endpoint():
        endpoint=batch.map.get('telemetry_network_endpoint')
        if not endpoint:
            return
        row=execute(['kubectl','--context','docker-desktop','get','svc',endpoint['name'],
            '-n',endpoint['namespace'],'--ignore-not-found','-o','json'])
        if row:
            current=json.loads(row)
            assert current['metadata']['uid']==endpoint['uid']
            assert current['metadata']['labels']['argus.plan9/run']==RUN
            execute(['kubectl','--context','docker-desktop','delete','svc',endpoint['name'],
                '-n',endpoint['namespace'],'--wait=true'])
    stage('owned-telemetry-endpoint-deleted',telemetry_endpoint)
    def destroy():
        original=json.loads((run.out/'cluster-infrastructure.json').read_text(encoding='utf-8'))
        values=json.loads(execute(['docker','inspect',RUN+'-control-plane']))
        assert len(values)==1 and values[0]['Id']==original['container_id']
        assert values[0]['Config']['Labels']['io.x-k8s.kind.cluster']==RUN
        execute([str(run.kind),'delete','cluster','--name',RUN,'--kubeconfig',str(run.kubeconfig)],timeout=150)
        assert not execute(['docker','container','ls','-aq','--filter','label=io.x-k8s.kind.cluster='+RUN])
    stage('owned-kind-cluster-deleted',destroy)
    state.update(status='destroyed',finished_at=now(),owned_containers_remaining=0)
    write_json(file,state)
    print('B09a product resources retired and owned Kind cluster destroyed',flush=True)
