"""Normal-API CR/RBAC operations and real, PDB-controlled node eviction."""
import json
import time
from urllib.parse import urlencode

from infrastructure import HERE, NAMESPACE


def wait(run, test, label, seconds=120):
    end = time.monotonic()+seconds
    while time.monotonic() < end:
        value = test()
        if value:
            return value
        time.sleep(2)
    raise RuntimeError(label+' did not converge')


def obj(run, kind, name, spec=None, version='v1'):
    value = {'apiVersion':version,'kind':kind,'metadata':{'name':name,'namespace':NAMESPACE,
                                                      'labels':{'argus.plan9/run':run.id}}}
    if spec is not None:
        value['spec'] = spec
    return value


def create(run, api, key, value):
    return run.mutate(api,{'type':key,'name':value['metadata']['name'],'operation':'create','object':value},
                      'create-'+key.replace('/','_')+'-'+value['metadata']['name'])


def base_objects(run, api):
    ready = []
    for kind,name in [('configmap','b10c-runtime'),('deployment','b10c-maintenance'),('deployment','b10c-activity'),('pdb','b10c-maintenance-budget')]:
        raw = run.kubectl('get',kind,name,'-n',NAMESPACE,'--ignore-not-found','-o','json')
        if raw:
            existing = json.loads(raw)
            assert existing['metadata']['labels']['argus.plan9/run']==run.id
            ready.append((kind,name))
    if len(ready)==4:
        for name in ('b10c-maintenance','b10c-activity'):
            run.kubectl('rollout','status','deployment/'+name,'-n',NAMESPACE,'--timeout=90s')
        return
    assert not ready, 'Inspect partial base-object creation before continuing'
    code = obj(run,'ConfigMap','b10c-runtime')
    code['data'] = {'service.py':(HERE/'service.py').read_text(encoding='utf-8')}
    create(run,api,'configmap',code)
    for name, client in [('b10c-maintenance',False),('b10c-activity',True)]:
        pod = {'automountServiceAccountToken':False,'nodeSelector':{'argus.plan9/role':'worker'},
               'volumes':[{'name':'code','configMap':{'name':'b10c-runtime'}}],
               'affinity':{'podAntiAffinity':{'preferredDuringSchedulingIgnoredDuringExecution':[{
                   'weight':100,'podAffinityTerm':{'topologyKey':'kubernetes.io/hostname','labelSelector':{'matchLabels':{'app':name}}}}]}},
               'containers':[{'name':'app','image':run.image,'imagePullPolicy':'Never',
                              'command':['python3','-u','/batch/service.py',*(['client'] if client else [])],
                              'volumeMounts':[{'name':'code','mountPath':'/batch','readOnly':True}],
                              'env':[{'name':'PLAN9_RUN','value':run.id},
                                     {'name':'PLAN9_DEADLINE','value':str(int(time.time()+8*3600))},
                                     {'name':'POD_NAME','valueFrom':{'fieldRef':{'fieldPath':'metadata.name'}}}],
                              'resources':{'requests':{'cpu':'10m','memory':'24Mi'},'limits':{'cpu':'150m','memory':'96Mi'}}}]}
        if not client:
            pod['containers'][0]['readinessProbe'] = {'httpGet':{'path':'/ready','port':8080},'periodSeconds':3}
        create(run,api,'deployment',obj(run,'Deployment',name,{'replicas':1 if client else 2,
                    'selector':{'matchLabels':{'app':name}},'template':{'metadata':{'labels':{'app':name,'argus.plan9/run':run.id}},'spec':pod}},'apps/v1'))
        run.kubectl('rollout','status','deployment/'+name,'-n',NAMESPACE,'--timeout=90s')
    create(run,api,'pdb',obj(run,'PodDisruptionBudget','b10c-maintenance-budget',
                           {'minAvailable':2,'selector':{'matchLabels':{'app':'b10c-maintenance'}}},'policy/v1'))


def extensions(run, api):
    base_objects(run,api)
    crd = {'apiVersion':'apiextensions.k8s.io/v1','kind':'CustomResourceDefinition',
           'metadata':{'name':'checks.b10c.argus.test','labels':{'argus.plan9/run':run.id}},
           'spec':{'group':'b10c.argus.test','scope':'Namespaced','names':{'plural':'checks','singular':'check','kind':'Plan9Check'},
                   'versions':[{'name':'v1','served':True,'storage':True,'schema':{'openAPIV3Schema':{'type':'object','properties':{
                       'spec':{'type':'object','properties':{'message':{'type':'string'},'replicas':{'type':'integer','minimum':1,'maximum':3}},'required':['message']}}}}}]}}
    # CRD is an infrastructure prerequisite; its instances use platform actions.
    run.apply([crd])
    run.kubectl('wait','--for=condition=Established','crd/checks.b10c.argus.test','--timeout=45s')
    native = run.get('crd','checks.b10c.argus.test')
    key = 'custom/b10c.argus.test/v1/checks@'+native['metadata']['uid']
    value = obj(run,'Plan9Check','b10c-check',{'message':'Created through the normal Argus workbench','replicas':1},'b10c.argus.test/v1')
    create(run,api,key,value)
    actual = run.get('checks.b10c.argus.test','b10c-check','-n',NAMESPACE)
    run.mutate(api,{'type':key,'name':'b10c-check','uid':actual['metadata']['uid'],
                    'expected_resource_version':actual['metadata']['resourceVersion'],'operation':'patch',
                    'object':{'spec':{'message':'Updated through the normal Argus workbench','replicas':2}}},'patch-owned-custom-resource')
    assert run.get('checks.b10c.argus.test','b10c-check','-n',NAMESPACE)['spec']['replicas'] == 2
    scratch = obj(run,'Plan9Check','b10c-scratch',{'message':'Owned lifecycle check'},'b10c.argus.test/v1')
    create(run,api,key,scratch)
    actual = run.get('checks.b10c.argus.test','b10c-scratch','-n',NAMESPACE)
    run.mutate(api,{'type':key,'name':'b10c-scratch','uid':actual['metadata']['uid'],
                    'expected_resource_version':actual['metadata']['resourceVersion'],'operation':'delete'},'delete-owned-custom-resource')
    role = obj(run,'Role','b10c-observer',version='rbac.authorization.k8s.io/v1')
    role['rules'] = [{'apiGroups':[''],'resources':['pods'],'verbs':['get','list']}]
    create(run,api,'role',role)
    account = obj(run,'ServiceAccount','b10c-reader')
    account['automountServiceAccountToken'] = False
    create(run,api,'serviceaccount',account)
    binding = obj(run,'RoleBinding','b10c-reader',version='rbac.authorization.k8s.io/v1')
    binding.update(roleRef={'apiGroup':'rbac.authorization.k8s.io','kind':'Role','name':'b10c-observer'},
                   subjects=[{'kind':'ServiceAccount','name':'b10c-reader','namespace':NAMESPACE}])
    create(run,api,'rolebinding',binding)
    allowed = run.kubectl('auth','can-i','list','pods','-n',NAMESPACE,'--as=system:serviceaccount:'+NAMESPACE+':b10c-reader')
    assert allowed == 'yes'
    run.change(api,'role','b10c-observer','patch',object={'rules':[{'apiGroups':[''],'resources':['pods'],'verbs':['get']}]})
    # A real denied authorization check returns nonzero, so capture it explicitly.
    import subprocess
    checked = subprocess.run(['docker','exec',run.id+'-control-plane','kubectl','--kubeconfig=/etc/kubernetes/admin.conf',
                              'auth','can-i','list','pods','-n',NAMESPACE,'--as=system:serviceaccount:'+NAMESPACE+':b10c-reader'],
                             capture_output=True,text=True,encoding='utf-8',timeout=20)
    assert checked.stdout.strip() == 'no'
    run.change(api,'role','b10c-observer','patch',object={'rules':role['rules']})
    denied = []
    for body in [
        {'type':'role','name':'b10c-forbidden','operation':'create','object':{**role,'metadata':{**role['metadata'],'name':'b10c-forbidden'},'rules':[{'apiGroups':['*'],'resources':['*'],'verbs':['*']}]}},
        {'type':'node','name':run.id+'-control-plane','operation':'cordon',
         'uid':run.get('node',run.id+'-control-plane')['metadata']['uid'],
         'expected_resource_version':run.get('node',run.id+'-control-plane')['metadata']['resourceVersion']},
    ]:
        if body['type']=='role':
            body['namespace_id'] = run.namespace(api)
        response = api.call('POST','/enterprise/kubernetes-clusters/'+run.cluster_id+'/inventory/actions/preview-mutation',body,
                            key='expected-denial-'+body['name'],expected=[400,403,409,422])
        assert response.get('code')
        denied.append({'object':body['name'],'code':response['code']})
    mapping = json.loads(run.resources.read_text(encoding='utf-8'))
    mapping.update(custom_resource={'type':key,'crd_uid':native['metadata']['uid'],'name':'b10c-check'},
                   maintenance_node=run.id+'-worker')
    run.save('resource-map',mapping)
    run.save('extension-rbac-exercise',{'custom_resource':mapping['custom_resource'],'reader_list_allowed_then_revoked_then_restored':True,'expected_denials':denied})
    print('Actual custom-resource CRUD, namespace RBAC edits and permission denials verified',flush=True)


def nodes(run, api):
    name = run.id+'-worker'
    pod_uids = {p['metadata']['uid'] for p in run.get('pods','-n',NAMESPACE)['items'] if p['spec'].get('nodeName')==name and p.get('status',{}).get('phase')=='Running'}
    assert pod_uids
    wait(run,lambda:run.get('pdb','b10c-maintenance-budget','-n',NAMESPACE).get('status',{}).get('currentHealthy')==2,'PDB healthy')
    run.change(api,'node',name,'cordon')
    assert run.get('node',name)['spec']['unschedulable']
    run.change(api,'node',name,'uncordon')
    node = run.get('node',name)
    operation = run.mutate(api,{'type':'node','name':name,'uid':node['metadata']['uid'],
                                'expected_resource_version':node['metadata']['resourceVersion'],'operation':'drain'},'drain-owned-worker',wait=False)
    def blocked():
        state = api.call('GET','/enterprise/kubernetes-clusters/'+run.cluster_id+'/operations/'+operation)
        if state['status'] in ('failed','conflict','cancelled','result_unknown','expired'):
            run.save('drain-blocked-unexpected',state)
            raise RuntimeError('Drain failed before the expected PDB wait')
        pending = run.get('pdb','b10c-maintenance-budget','-n',NAMESPACE)['status']
        still = run.get('pods','-n',NAMESPACE,'-l','app=b10c-maintenance')['items']
        return state if 'blocked' in json.dumps(state).lower() and run.get('node',name)['spec'].get('unschedulable') and pending.get('disruptionsAllowed')==0 and any(p['spec'].get('nodeName')==name for p in still) else None
    held = wait(run,blocked,'Real PDB blocks eviction',60)
    run.save('drain-pdb-held',held)
    run.change(api,'pdb','b10c-maintenance-budget','patch',object={'spec':{'minAvailable':1}})
    completed = run.wait_operation(api,operation,'drain-after-budget-recovery',timeout=240)
    current = run.get('pods','-n',NAMESPACE)['items']
    assert not any(p['metadata']['uid'] in pod_uids for p in current)
    for deployment in ('b10c-repo','b10c-oci','b10c-maintenance','b10c-activity'):
        run.kubectl('rollout','status','deployment/'+deployment,'-n',NAMESPACE,'--timeout=90s')
    run.change(api,'node',name,'uncordon')
    run.change(api,'node',name,'patch',object={'metadata':{'labels':{'argus.plan9/maintenance':'verified'}}})
    original = run.get('node',name)['spec'].get('taints',[])
    run.change(api,'node',name,'patch',object={'spec':{'taints':original+[{'key':'argus.plan9/acceptance','value':'owned','effect':'NoSchedule'}]}})
    assert any(t['key']=='argus.plan9/acceptance' for t in run.get('node',name)['spec']['taints'])
    run.change(api,'node',name,'patch',object={'spec':{'taints':original}})
    run.change(api,'pdb','b10c-maintenance-budget','patch',object={'spec':{'minAvailable':2}})
    run.save('node-maintenance-exercise',{'node':name,'old_pod_uids':sorted(pod_uids),
                                          'drain_operation':completed,'pdb_block_and_recovery':True,
                                          'uncordoned':not run.get('node',name)['spec'].get('unschedulable',False),
                                          'label_retained':True,'temporary_taint_removed':True})
    print('Actual cordon/PDB block/eviction/reschedule/uncordon and label/taint operations verified',flush=True)


def execute_stage(run, action):
    return {'extensions':extensions,'nodes':nodes}[action](run,run.api())
