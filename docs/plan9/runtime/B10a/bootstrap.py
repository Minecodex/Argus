"""Only the owned lab namespaces get object write/exec/forward permissions."""
from infrastructure import NAMESPACE
def workbench_rbac(run):
 labels={'argus.plan9/run':run.id}
 subject={'kind':'ServiceAccount','name':'argus-kubernetes-connector','namespace':'plan9-access'}
 rules=[
  {'apiGroups':[''],'resources':['pods','services','configmaps','secrets'],'verbs':['get','list','watch','create','update','patch','delete']},
  {'apiGroups':[''],'resources':['pods/log'],'verbs':['get']},
  {'apiGroups':[''],'resources':['pods/exec','pods/portforward'],'verbs':['create','get']},
  {'apiGroups':['apps'],'resources':['deployments','replicasets','statefulsets','daemonsets','deployments/scale','statefulsets/scale'],'verbs':['get','list','watch','create','update','patch','delete']},
  {'apiGroups':['networking.k8s.io'],'resources':['ingresses'],'verbs':['get','list','watch','create','update','patch','delete']},
 ]
 run.apply([
  {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'Role','metadata':{'name':'b10a-workbench','namespace':NAMESPACE,'labels':labels},'rules':rules},
  {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'RoleBinding','metadata':{'name':'b10a-workbench','namespace':NAMESPACE,'labels':labels},'roleRef':{'apiGroup':'rbac.authorization.k8s.io','kind':'Role','name':'b10a-workbench'},'subjects':[subject]},
  {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'ClusterRole','metadata':{'name':'b10a-namespace-lifecycle','labels':labels},'rules':[
   {'apiGroups':[''],'resources':['namespaces'],'verbs':['create']},
   {'apiGroups':[''],'resources':['namespaces'],'resourceNames':['b10a-transient'],'verbs':['get','delete','patch','update']}]},
  {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'ClusterRoleBinding','metadata':{'name':'b10a-namespace-lifecycle','labels':labels},'roleRef':{'apiGroup':'rbac.authorization.k8s.io','kind':'ClusterRole','name':'b10a-namespace-lifecycle'},'subjects':[subject]},
 ])
 actor='system:serviceaccount:plan9-access:argus-kubernetes-connector'
 checks={}
 for label,args,want in [
  ('owned_patch',['patch','deployments','-n',NAMESPACE],True),
  ('owned_exec',['create','pods/exec','-n',NAMESPACE],True),
  ('system_patch',['patch','deployments','-n','kube-system'],False),
  ('rbac_escalation',['create','clusterrolebindings'],False),
 ]:
  value=run.kubectl('auth','can-i',*args,'--as='+actor) if want else None
  if want:assert value.strip()=='yes';checks[label]=True
  else:
   import subprocess
   p=subprocess.run(['docker','exec',run.id+'-control-plane','kubectl','--kubeconfig=/etc/kubernetes/admin.conf','auth','can-i',*args,'--as='+actor],capture_output=True,text=True)
   assert p.returncode==1 and p.stdout.strip()=='no';checks[label]=False
 from B09a.manage import write_json
 write_json(run.out/'workbench-rbac-verification.json',checks)
