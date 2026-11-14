"""Real normal-API Helm repository/OCI lifecycle with immutable previews."""
import json
import time
from urllib.parse import urlencode

import yaml
from infrastructure import NAMESPACE
from sources import compose, source_private


def releases(run, api):
    query = urlencode({'namespace_id':run.namespace(api)})
    return api.call('GET','/enterprise/kubernetes-clusters/'+run.cluster_id+'/helm/releases?'+query)['items']


def current(run, api, name):
    return next((item for item in releases(run,api) if item['name']==name), None)


def preview(run, api, name, operation, source='repository', version='0.1.0', **extra):
    live = current(run,api,name)
    body = {'namespace_id':run.namespace(api),'operation':operation,'name':name,
            'expected_revision':live['revision'] if live else 0,'timeout_seconds':90,
            'reason':'B10c actual '+source+' '+operation, **extra}
    if operation in ('install','upgrade'):
        endpoints = json.loads((run.out/'helm-sources.json').read_text(encoding='utf-8'))
        account = json.loads((source_private(run)/'source-account.json').read_text(encoding='utf-8'))
        body.update(source={'kind':source,'url':endpoints['repository' if source=='repository' else 'oci'],'version':version},
                    source_credentials={**account,'ca_pem':(source_private(run)/'source-ca.pem').read_text(encoding='utf-8')})
        if source == 'repository':
            body['source']['chart'] = 'b10c-web'
    return api.call('POST','/enterprise/kubernetes-clusters/'+run.cluster_id+'/helm/actions/preview',
                    body,key='helm-'+name+'-'+operation+'-'+str(body['expected_revision'])+'-'+version,expected=201)


def confirm(run, api, action, label, expected=('succeeded','applied')):
    submitted = api.confirm(action,wait=False)
    run.save(label+'-submitted',submitted)
    operation = run.operation_ref(api,submitted)
    return run.wait_operation(api,operation,label,timeout=240,expected=expected)


def lifecycle(run, api, name, source, keep=True):
    history = []
    for operation, version, extra in [('install','0.1.0',{}),('upgrade','0.2.0',{}),
                                       ('rollback','0.1.0',{'rollback_revision':1})]:
        action = preview(run,api,name,operation,source,version,**extra)
        history.append(confirm(run,api,action,'helm-'+name+'-'+operation))
        live = current(run,api,name)
        assert live and live['status']=='deployed' and live['chart_version']==version
        run.kubectl('rollout','status','deployment/'+name,'-n',NAMESPACE,'--timeout=90s')
    if not keep:
        history.append(confirm(run,api,preview(run,api,name,'uninstall',source),'helm-'+name+'-uninstall'))
        assert current(run,api,name) is None or current(run,api,name)['status']=='uninstalled'
        for kind, target in [('deployment',name),('service',name),('configmap',name+'-code')]:
            assert not run.kubectl('get',kind,target,'-n',NAMESPACE,'--ignore-not-found','-o','name')
        if run.kubectl('get','job',name+'-hook','-n',NAMESPACE,'--ignore-not-found','-o','name'):
            run.change(api,'job',name+'-hook','delete')
    run.save('helm-'+source+'-lifecycle', {'release':name,'operations':history,'kept':keep})


def execute_stage(run, _action):
    api = run.api()
    if (run.out/'helm-final-releases.json').exists():
        assert all(current(run,api,name)['status']=='deployed' for name in ('b10c-repo','b10c-oci'))
        return
    completed = run.out/'helm-repo-rollback-operation.json'
    if completed.exists() and (run.out/'source-offline-execution-proof.json').exists():
        prior = json.loads(completed.read_text(encoding='utf-8'))
        live = current(run,api,'b10c-repo')
        assert prior['status']=='succeeded' and live['status']=='deployed'
        assert live['revision']==prior['result']['release']['revision'] and live['chart_version']=='0.1.0'
        finish_oci(run,api)
        return
    existing = current(run,api,'b10c-repo')
    if not existing:
        confirm(run,api,preview(run,api,'b10c-repo','install'),'helm-repository-first-install')
    else:
        assert existing['status']=='deployed' and existing['chart_version']=='0.1.0'
        prior = json.loads((run.out/'helm-repository-offline-confirm-submitted.json').read_text(encoding='utf-8'))
        run.wait_operation(api,run.operation_ref(api,prior),'repository-first-confirm-observed')
    action = preview(run,api,'b10c-offline-check','install')
    # Real upstream sources become unavailable after preview. The approved
    # operation must use its already downloaded, verified and sealed Chart.
    compose(run,'stop','charts','registry')
    try:
        result = confirm(run,api,action,'helm-repository-offline-confirm-complete')
        assert current(run,api,'b10c-offline-check')['status']=='deployed'
        run.save('source-offline-execution-proof',{'operation_id':result['id'],'finished_at':result['finished_at'],
                                                 'sources_stayed_off_until_operation_complete':True})
    finally:
        compose(run,'up','-d','--pull','never')
    confirm(run,api,preview(run,api,'b10c-offline-check','uninstall'),'helm-offline-scratch-uninstall')
    if run.kubectl('get','job','b10c-offline-check-hook','-n',NAMESPACE,'--ignore-not-found','-o','name'):
        run.change(api,'job','b10c-offline-check-hook','delete')
    assert current(run,api,'b10c-repo')['status']=='deployed'
    confirm(run,api,preview(run,api,'b10c-repo','upgrade',version='0.2.0'),'helm-repo-upgrade')
    confirm(run,api,preview(run,api,'b10c-repo','rollback',rollback_revision=1),'helm-repo-rollback')
    assert current(run,api,'b10c-repo')['chart_version']=='0.1.0'
    finish_oci(run,api)


def finish_oci(run, api):
    lifecycle(run,api,'b10c-oci-lifecycle','oci',keep=False)
    confirm(run,api,preview(run,api,'b10c-oci','install','oci','0.2.0'),'helm-oci-retained')
    for name in ('b10c-repo','b10c-oci'):
        run.kubectl('rollout','status','deployment/'+name,'-n',NAMESPACE,'--timeout=90s')
        logs = run.kubectl('logs','-n',NAMESPACE,'job/'+name+'-hook')
        assert 'B10C_HOOK_OK' in logs
        run.save(name+'-hook-receipt', {'stdout':logs,'observed':True})
    mapping = json.loads(run.resources.read_text(encoding='utf-8'))
    mapping['helm_releases'] = {name:current(run,api,name) for name in ('b10c-repo','b10c-oci')}
    run.save('resource-map',mapping)
    run.save('helm-final-releases',releases(run,api))
    print('Repository/OCI install, upgrade, rollback, uninstall, real hooks and offline-confirm verified',flush=True)
