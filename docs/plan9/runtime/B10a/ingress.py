"""Dedicated, namespace-watching ingress prerequisite; business Ingress is created by Argus."""
import json,subprocess
from infrastructure import NAMESPACE,execute,write_json
IMAGE='registry.k8s.io/ingress-nginx/controller@sha256:de8fd8f1def14ed36f59c9838bd3c920ecb01754072e3e9f10832d62ef876cb8'
def install(r):
 transfer=json.loads((r.out/'ingress-image-transfer.json').read_text(encoding='utf-8'))
 assert transfer['image']==IMAGE and transfer['all_content_digests_verified']
 assert IMAGE in execute(['docker','exec',r.id+'-control-plane','ctr','--namespace','k8s.io','images','ls','-q'])
 ns='b10a-infra';labels={'argus.plan9/run':r.id}
 subject={'kind':'ServiceAccount','name':'b10a-ingress','namespace':ns}
 r.apply([
  {'apiVersion':'v1','kind':'Namespace','metadata':{'name':ns,'labels':labels}},
  {'apiVersion':'v1','kind':'ServiceAccount','metadata':{'name':'b10a-ingress','namespace':ns,'labels':labels}},
  {'apiVersion':'v1','kind':'ConfigMap','metadata':{'name':'b10a-ingress','namespace':ns,'labels':labels},'data':{'allow-snippet-annotations':'false'}},
  {'apiVersion':'networking.k8s.io/v1','kind':'IngressClass','metadata':{'name':'b10a-nginx','labels':labels},'spec':{'controller':'k8s.io/b10a-nginx'}},
  {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'ClusterRole','metadata':{'name':'b10a-ingress-reader','labels':labels},'rules':[
   {'apiGroups':[''],'resources':['nodes','namespaces'],'verbs':['get','list','watch']},
   {'apiGroups':['networking.k8s.io'],'resources':['ingressclasses'],'verbs':['get','list','watch']}]},
  {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'ClusterRoleBinding','metadata':{'name':'b10a-ingress-reader','labels':labels},'roleRef':{'apiGroup':'rbac.authorization.k8s.io','kind':'ClusterRole','name':'b10a-ingress-reader'},'subjects':[subject]},
 ])
 for target in [ns,NAMESPACE]:
  r.apply([
   {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'Role','metadata':{'name':'b10a-ingress','namespace':target,'labels':labels},'rules':[
    {'apiGroups':[''],'resources':['pods','services','endpoints','secrets','configmaps'],'verbs':['get','list','watch']},
    {'apiGroups':[''],'resources':['events'],'verbs':['create','patch']},
    {'apiGroups':['discovery.k8s.io'],'resources':['endpointslices'],'verbs':['get','list','watch']},
    {'apiGroups':['networking.k8s.io'],'resources':['ingresses'],'verbs':['get','list','watch']},
    {'apiGroups':['networking.k8s.io'],'resources':['ingresses/status'],'verbs':['update','patch']},
    {'apiGroups':['coordination.k8s.io'],'resources':['leases'],'verbs':['create','get','update','patch']}]},
   {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'RoleBinding','metadata':{'name':'b10a-ingress','namespace':target,'labels':labels},'roleRef':{'apiGroup':'rbac.authorization.k8s.io','kind':'Role','name':'b10a-ingress'},'subjects':[subject]}])
 r.apply([
  {'apiVersion':'v1','kind':'Service','metadata':{'name':'b10a-ingress','namespace':ns,'labels':labels},'spec':{'selector':{'app':'b10a-ingress'},'ports':[{'name':'http','port':80,'targetPort':80}]}},
  {'apiVersion':'apps/v1','kind':'Deployment','metadata':{'name':'b10a-ingress','namespace':ns,'labels':labels},'spec':{'replicas':1,'selector':{'matchLabels':{'app':'b10a-ingress'}},'template':{'metadata':{'labels':{**labels,'app':'b10a-ingress'}},'spec':{'serviceAccountName':'b10a-ingress','containers':[{
   'name':'controller','image':IMAGE,'imagePullPolicy':'Never',
   'args':['/nginx-ingress-controller','--controller-class=k8s.io/b10a-nginx','--ingress-class=b10a-nginx','--election-id=b10a-ingress','--watch-namespace='+NAMESPACE,'--configmap='+ns+'/b10a-ingress'],
   'env':[{'name':'POD_NAMESPACE','valueFrom':{'fieldRef':{'fieldPath':'metadata.namespace'}}},{'name':'POD_NAME','valueFrom':{'fieldRef':{'fieldPath':'metadata.name'}}}],
   'resources':{'requests':{'cpu':'50m','memory':'96Mi'},'limits':{'cpu':'300m','memory':'256Mi'}},
   'ports':[{'name':'http','containerPort':80}],'readinessProbe':{'httpGet':{'path':'/healthz','port':10254},'periodSeconds':5},
   'securityContext':{'runAsUser':101,'runAsNonRoot':True,'allowPrivilegeEscalation':False,'capabilities':{'drop':['ALL'],'add':['NET_BIND_SERVICE']}}}]}}}}
 ])
 r.kubectl('rollout','status','deployment/b10a-ingress','-n',ns,'--timeout=120s',timeout=130)
 write_json(r.out/'ingress-controller.json',{'image':IMAGE,'namespace':ns,'uid':r.get('deployment','b10a-ingress','-n',ns)['metadata']['uid'],'watch_namespace':NAMESPACE})
 print('Owned namespace-scoped ingress controller ready',flush=True)
def verify(r):
 code="import json,urllib.request;r=urllib.request.Request('http://b10a-ingress.b10a-infra/?label=real-ingress',headers={'Host':'b10a.test'});print(urllib.request.urlopen(r,timeout=5).read().decode())"
 data=json.loads(r.kubectl('exec','-n',NAMESPACE,'deployment/b10a-api','--','python3','-c',code))
 assert data['run']==r.id and data['pod'].startswith('b10a-api-')
 write_json(r.out/'ingress-http-receipt.json',data)
 return data
if __name__=='__main__':
 from run import Run
 install(Run())
