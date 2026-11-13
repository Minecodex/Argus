"""B10b object operations stay in its workload namespace; storage globals read only."""
import subprocess
from infrastructure import NAMESPACE
def workbench_rbac(r):
 labels={'argus.plan9/run':r.id};actor={'kind':'ServiceAccount','name':'argus-kubernetes-connector','namespace':'plan9-access'}
 verbs=['get','list','watch','create','update','patch','delete']
 objects=[
 {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'Role','metadata':{'name':'b10b-workbench','namespace':NAMESPACE,'labels':labels},'rules':[
  {'apiGroups':[''],'resources':['pods','services','configmaps','persistentvolumeclaims'],'verbs':verbs},
  {'apiGroups':[''],'resources':['pods/log'],'verbs':['get']},
  {'apiGroups':[''],'resources':['pods/exec','pods/portforward'],'verbs':['create','get']},
  {'apiGroups':['apps'],'resources':['deployments','deployments/scale','replicasets'],'verbs':verbs},
  {'apiGroups':['batch'],'resources':['jobs','cronjobs'],'verbs':verbs},
  {'apiGroups':['autoscaling'],'resources':['horizontalpodautoscalers'],'verbs':verbs},
  {'apiGroups':['networking.k8s.io'],'resources':['networkpolicies'],'verbs':verbs}]},
 {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'RoleBinding','metadata':{'name':'b10b-workbench','namespace':NAMESPACE,'labels':labels},'roleRef':{'apiGroup':'rbac.authorization.k8s.io','kind':'Role','name':'b10b-workbench'},'subjects':[actor]},
 {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'ClusterRole','metadata':{'name':'b10b-storage-view','labels':labels},'rules':[
  {'apiGroups':[''],'resources':['persistentvolumes'],'verbs':['get','list','watch']},
  {'apiGroups':['storage.k8s.io'],'resources':['storageclasses','csidrivers','csinodes','volumeattachments','csistoragecapacities'],'verbs':['get','list','watch']}]},
 {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'ClusterRoleBinding','metadata':{'name':'b10b-storage-view','labels':labels},'roleRef':{'apiGroup':'rbac.authorization.k8s.io','kind':'ClusterRole','name':'b10b-storage-view'},'subjects':[actor]}]
 r.apply(objects)
 checks={}
 for label,args,want in [('pvc_write',['patch','persistentvolumeclaims','-n',NAMESPACE],True),('job_create',['create','jobs','-n',NAMESPACE],True),('hpa_write',['patch','horizontalpodautoscalers','-n',NAMESPACE],True),('network_policy',['create','networkpolicies','-n',NAMESPACE],True),('system_write',['patch','deployments','-n','kube-system'],False),('storageclass_write',['create','storageclasses'],False)]:
  p=subprocess.run(['docker','exec',r.id+'-control-plane','kubectl','--kubeconfig=/etc/kubernetes/admin.conf','auth','can-i',*args,'--as=system:serviceaccount:plan9-access:argus-kubernetes-connector'],capture_output=True,text=True,timeout=15)
  assert p.stdout.strip()==('yes' if want else 'no'),label
  checks[label]=want
 r.save('workbench-rbac-verification',checks)
 print('B10b scoped mutation and read-only storage privileges ready',flush=True)
