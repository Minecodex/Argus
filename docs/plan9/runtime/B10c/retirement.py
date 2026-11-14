"""Retire only the active run, preserving product history and borrowed caches."""
import json
import sys

from infrastructure import ROOT, NAMESPACE, execute
from sources import compose


def execute_stage(run, _action):
    before = run.platform()
    api = run.api()
    mapping = json.loads(run.resources.read_text(encoding='utf-8'))
    assert mapping['run_id']==run.id
    namespace = run.get('namespace',NAMESPACE)
    assert namespace['metadata']['uid']==mapping['namespaces'][NAMESPACE]['uid']
    assert namespace['metadata']['labels']['argus.plan9/run']==run.id
    for row in mapping['alert_policies'].values():
        current = api.call('GET','/alerts/configurations/'+row['id'],expected=[200,404])
        if not current.get('code'):
            api.confirm(api.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'delete',
                                'id':current['id'],'expected_version':current['version']},key='retire-policy-'+row['id'],expected=202))
    from helm import releases, preview, confirm
    for release in releases(run,api):
        assert release['name'].startswith('b10c-')
        if release['status']!='uninstalled':
            confirm(run,api,preview(run,api,release['name'],'uninstall'),'retire-'+release['name'])
    for kind in ('deployment','job','cronjob','pdb','service','configmap','rolebinding','role','serviceaccount'):
        for item in run.get(kind,'-n',NAMESPACE)['items']:
            if item['metadata']['name'] in ('default','kube-root-ca.crt') or (kind in ('role','rolebinding') and item['metadata']['name']=='b10c-workbench'):
                continue
            assert item['metadata'].get('labels',{}).get('argus.plan9/run')==run.id
            run.change(api,kind,item['metadata']['name'],'delete')
    cr = mapping.get('custom_resource')
    if cr and run.kubectl('get','checks.b10c.argus.test','-n',NAMESPACE,'-o','name'):
        item = run.get('checks.b10c.argus.test',cr['name'],'-n',NAMESPACE)
        run.mutate(api,{'type':cr['type'],'name':cr['name'],'uid':item['metadata']['uid'],
                       'expected_resource_version':item['metadata']['resourceVersion'],'operation':'delete'},'retire-custom-instance')
    if cr:
        actual = run.get('crd','checks.b10c.argus.test')
        assert actual['metadata']['uid']==cr['crd_uid'] and actual['metadata']['labels']['argus.plan9/run']==run.id
        run.kubectl('delete','crd','checks.b10c.argus.test','--wait=true')
    sys.path.insert(0,str(ROOT/'docs/plan9/runtime/B09a'))
    from B09a.integration import Batch
    from B09a.retire import retire
    retire(Batch(run,api))
    ids = execute(['docker','container','ls','-aq','--filter','label=com.docker.compose.project='+run.id]).splitlines()
    for ident in ids:
        container = json.loads(execute(['docker','inspect',ident]))[0]
        assert container['Config']['Labels']['argus.plan9/run']==run.id
    for name in execute(['docker','volume','ls','-q','--filter','label=com.docker.compose.project='+run.id]).splitlines():
        volume = json.loads(execute(['docker','volume','inspect',name]))[0]
        assert volume['Labels']['argus.plan9/run']==run.id
    # Docker Compose only removes this project's labelled containers/volume;
    # the external shared Kind network and image registry remain borrowed.
    compose(run,'down','--volumes','--remove-orphans')
    assert not execute(['docker','container','ls','-aq','--filter','label=argus.plan9/run='+run.id])
    assert run.platform()==before
    run.save('cleanup-verification',{'run_id':run.id,'status':'destroyed','formal_resources_preserved':True,
                                      'history_preserved':True,'borrowed_image_cache_retained':True})
