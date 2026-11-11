"""Public manifests; protected Secret values are supplied separately at preview."""
from infrastructure import IMAGE,NAMESPACE
def meta(run,name):return {'name':name,'namespace':NAMESPACE,'labels':{'argus.plan9/run':run.id,'app':name}}
def template(run,name,action='serve'):
 c={'name':'http','image':IMAGE,'imagePullPolicy':'Never','args':[action],
  'resources':{'requests':{'cpu':'15m','memory':'24Mi'},'limits':{'cpu':'150m','memory':'80Mi'}},
  'env':[{'name':'PLAN9_RUN','value':run.id},{'name':'VERSION','value':'v1'},
   {'name':'POD_NAME','valueFrom':{'fieldRef':{'fieldPath':'metadata.name'}}},
   {'name':'POD_UID','valueFrom':{'fieldRef':{'fieldPath':'metadata.uid'}}}],
  'volumeMounts':[{'name':'data','mountPath':'/data'},{'name':'config','mountPath':'/config','readOnly':True},{'name':'secret','mountPath':'/secret','readOnly':True}]}
 if action=='serve':c.update(ports=[{'name':'http','containerPort':8080}],readinessProbe={'httpGet':{'path':'/health','port':8080},'periodSeconds':3,'initialDelaySeconds':1})
 spec={'automountServiceAccountToken':False,'securityContext':{'runAsNonRoot':True,'runAsUser':10001,'fsGroup':10001},'containers':[c],
  'volumes':[{'name':'data','emptyDir':{'sizeLimit':'8Mi'}},{'name':'config','configMap':{'name':'b10a-settings'}},{'name':'secret','secret':{'secretName':'b10a-demo-secret'}}]}
 return {'metadata':{'labels':meta(run,name)['labels']},'spec':spec}
def objects(run):
 cm={'apiVersion':'v1','kind':'ConfigMap','metadata':meta(run,'b10a-settings'),'data':{'mode':'baseline','message':'B10a real workbench'}}
 secret={'apiVersion':'v1','kind':'Secret','metadata':meta(run,'b10a-demo-secret'),'type':'Opaque','data':{'demo-token':'[REDACTED]'}}
 result=[('configmap',cm),('secret',secret)]
 for name,headless in [('b10a-api',False),('b10a-ledger',True)]:
  spec={'selector':{'app':name},'ports':[{'name':'http','port':8080,'targetPort':8080}]}
  if headless:spec['clusterIP']='None'
  result.append(('service',{'apiVersion':'v1','kind':'Service','metadata':meta(run,name),'spec':spec}))
 for kind,name in [('Deployment','b10a-api'),('StatefulSet','b10a-ledger'),('DaemonSet','b10a-node-observer')]:
  spec={'selector':{'matchLabels':{'app':name}},'template':template(run,name,'daemon' if kind=='DaemonSet' else 'serve')}
  if kind!='DaemonSet':spec['replicas']=1
  if kind=='StatefulSet':spec['serviceName']='b10a-ledger'
  result.append((kind.lower(),{'apiVersion':'apps/v1','kind':kind,'metadata':meta(run,name),'spec':spec}))
 result.append(('ingress',{'apiVersion':'networking.k8s.io/v1','kind':'Ingress','metadata':meta(run,'b10a-http'),'spec':{'ingressClassName':'b10a-nginx','rules':[{'host':'b10a.test','http':{'paths':[{'path':'/','pathType':'Prefix','backend':{'service':{'name':'b10a-api','port':{'number':8080}}}}]}}]}}))
 return result
