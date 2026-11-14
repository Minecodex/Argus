"""Explicit workload RBAC and node writes restricted to one owned worker."""
import subprocess
from infrastructure import NAMESPACE


def install(run):
    labels = {'argus.plan9/run':run.id}
    actor = {'kind':'ServiceAccount','name':'argus-kubernetes-connector','namespace':'plan9-access'}
    verbs = ['get','list','watch','create','patch','update','delete']
    role = {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'Role',
            'metadata':{'name':'b10c-workbench','namespace':NAMESPACE,'labels':labels},'rules':[
                {'apiGroups':[''],'resources':['pods','services','configmaps','secrets','serviceaccounts'],'verbs':verbs},
                {'apiGroups':[''],'resources':['pods/log'],'verbs':['get']},
                {'apiGroups':[''],'resources':['pods/exec','pods/portforward','pods/eviction'],'verbs':['get','create']},
                {'apiGroups':['apps'],'resources':['deployments','deployments/scale','replicasets'],'verbs':verbs},
                {'apiGroups':['batch'],'resources':['jobs','cronjobs'],'verbs':verbs},
                {'apiGroups':['policy'],'resources':['poddisruptionbudgets'],'verbs':verbs},
                {'apiGroups':['rbac.authorization.k8s.io'],'resources':['roles','rolebindings'],'verbs':verbs},
                {'apiGroups':['b10c.argus.test'],'resources':['checks'],'verbs':verbs}]}
    worker = run.id+'-worker'
    run.apply([role,
        {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'RoleBinding','metadata':{'name':'b10c-workbench','namespace':NAMESPACE,'labels':labels},
         'roleRef':{'apiGroup':'rbac.authorization.k8s.io','kind':'Role','name':'b10c-workbench'},'subjects':[actor]},
        {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'ClusterRole','metadata':{'name':'b10c-owned-worker','labels':labels},'rules':[
            {'apiGroups':[''],'resources':['nodes'],'resourceNames':[worker],'verbs':['get','patch','update']},
            {'apiGroups':['apiextensions.k8s.io'],'resources':['customresourcedefinitions'],'verbs':['get','list','watch']}]},
        {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'ClusterRoleBinding','metadata':{'name':'b10c-owned-worker','labels':labels},
         'roleRef':{'apiGroup':'rbac.authorization.k8s.io','kind':'ClusterRole','name':'b10c-owned-worker'},'subjects':[actor]}])
    checks = []
    for args, expected in [(['patch','nodes/'+worker],True), (['patch','nodes/'+run.id+'-control-plane'],False),
                           (['patch','deployments','-n','kube-system'],False), (['create','roles','-n',NAMESPACE],True)]:
        checked = subprocess.run(['docker','exec',run.id+'-control-plane','kubectl','--kubeconfig=/etc/kubernetes/admin.conf',
                                  'auth','can-i',*args,'--as=system:serviceaccount:plan9-access:argus-kubernetes-connector'],capture_output=True,text=True,encoding='utf-8',timeout=20)
        assert checked.returncode in (0,1)
        value = checked.stdout.strip()
        assert value == ('yes' if expected else 'no'), args
        checks.append({'action':args,'allowed':expected})
    run.save('native-permission-boundaries', checks)
