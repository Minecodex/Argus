"""Resource-bounded Kubernetes business objects for B09a."""
from manage import APP_IMAGE,APP_NS,RUN


def metadata(name):
    return {'name':name,'namespace':APP_NS,'labels':{'argus.plan9/run':RUN,'app':name}}


def pod(name,action='serve',persistent=False):
    container={'name':'http','image':APP_IMAGE,'imagePullPolicy':'Never',
        'args':[action],'resources':{'requests':{'cpu':'20m','memory':'32Mi'},
            'limits':{'cpu':'200m','memory':'96Mi'}},
        'env':[{'name':'PLAN9_RUN','value':RUN},
               {'name':'POD_NAME','valueFrom':{'fieldRef':{'fieldPath':'metadata.name'}}},
               {'name':'POD_UID','valueFrom':{'fieldRef':{'fieldPath':'metadata.uid'}}}],
        'volumeMounts':[{'name':'data','mountPath':'/data'}]}
    if action=='serve':
        container.update(ports=[{'name':'http','containerPort':8080}],
            readinessProbe={'httpGet':{'path':'/health','port':8080},'initialDelaySeconds':1,'periodSeconds':3})
    spec={'automountServiceAccountToken':False,
          'securityContext':{'runAsUser':10001,'runAsNonRoot':True,'fsGroup':10001},
          'containers':[container]}
    if not persistent:
        spec['volumes']=[{'name':'data','emptyDir':{'sizeLimit':'16Mi'}}]
    return {'metadata':{'labels':{'app':name,'argus.plan9/run':RUN}},'spec':spec}


def objects():
    out=[{'apiVersion':'v1','kind':'Namespace','metadata':{'name':APP_NS,'labels':{'argus.plan9/run':RUN}}}]
    for name in ('orders','ledger'):
        out.append({'apiVersion':'v1','kind':'Service','metadata':metadata(name),
            'spec':{'selector':{'app':name},'ports':[{'port':8080,'targetPort':8080}]}})
    for name,replicas,action in [('orders',2,'serve'),('driver',1,'load')]:
        out.append({'apiVersion':'apps/v1','kind':'Deployment','metadata':metadata(name),
            'spec':{'replicas':replicas,'selector':{'matchLabels':{'app':name}},'template':pod(name,action)}})
    out.append({'apiVersion':'apps/v1','kind':'StatefulSet','metadata':metadata('ledger'),
        'spec':{'replicas':1,'serviceName':'ledger','selector':{'matchLabels':{'app':'ledger'}},
            'template':pod('ledger',persistent=True),'volumeClaimTemplates':[{'metadata':{'name':'data',
                'labels':{'argus.plan9/run':RUN}},'spec':{'accessModes':['ReadWriteOnce'],
                'resources':{'requests':{'storage':'64Mi'}}}}]}})
    out.append({'apiVersion':'apps/v1','kind':'DaemonSet','metadata':metadata('node-agent'),
        'spec':{'selector':{'matchLabels':{'app':'node-agent'}},'template':pod('node-agent','daemon')}})
    job=pod('receipt','job')
    job['spec']['restartPolicy']='Never'
    out.append({'apiVersion':'batch/v1','kind':'Job','metadata':metadata('receipt'),
        'spec':{'backoffLimit':0,'activeDeadlineSeconds':90,'template':job}})
    return out
