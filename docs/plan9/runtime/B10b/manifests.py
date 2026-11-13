"""Owned public objects submitted through the platform workbench."""
import json,time
from infrastructure import IMAGE,NAMESPACE
def meta(r,name):return {'name':name,'namespace':NAMESPACE,'labels':{'argus.plan9/run':r.id,'app':name}}
def pod(r,name,mode='serve',claim=None,allowed=False):
 c={'name':'main','image':IMAGE,'imagePullPolicy':'Never','args':[mode],
 'resources':{'requests':{'cpu':'20m','memory':'20Mi'},'limits':{'cpu':'180m','memory':'64Mi'}},
 'env':[{'name':'PLAN9_RUN','value':r.id},{'name':'POD_NAME','valueFrom':{'fieldRef':{'fieldPath':'metadata.name'}}},{'name':'POD_UID','valueFrom':{'fieldRef':{'fieldPath':'metadata.uid'}}}],
 'volumeMounts':[{'name':'data','mountPath':'/data'}]}
 labels=meta(r,name)['labels']
 if allowed:labels['b10b-access']='allowed'
 if mode=='serve':c.update(ports=[{'containerPort':8080,'name':'http'}],readinessProbe={'httpGet':{'path':'/health','port':8080},'periodSeconds':3})
 if mode=='client':
  f=r.out/'activity-deadline.json'
  if not f.exists():r.save('activity-deadline',{'epoch':time.time()+8*3600,'hours':8})
  c['env'].append({'name':'PLAN9_DEADLINE','value':str(json.loads(f.read_text(encoding='utf-8'))['epoch'])})
 return {'metadata':{'labels':labels},'spec':{'automountServiceAccountToken':False,'terminationGracePeriodSeconds':3,'securityContext':{'runAsNonRoot':True,'runAsUser':10001,'fsGroup':10001},'containers':[c],'volumes':[{'name':'data',**({'persistentVolumeClaim':{'claimName':claim}} if claim else {'emptyDir':{'sizeLimit':'4Mi'}})}]}}
def deployment(r,name,claim=None,mode='serve',allowed=False):
 return {'apiVersion':'apps/v1','kind':'Deployment','metadata':meta(r,name),'spec':{'replicas':1,'selector':{'matchLabels':{'app':name}},'template':pod(r,name,mode,claim,allowed)}}
def pvc(r,name,size='64Mi'):
 return {'apiVersion':'v1','kind':'PersistentVolumeClaim','metadata':meta(r,name),'spec':{'storageClassName':'b10b-expandable','accessModes':['ReadWriteOnce'],'resources':{'requests':{'storage':size}}}}
def job(r,name,delay=2):
 template=pod(r,name,'job','b10b-data');template['spec']['restartPolicy']='Never'
 template['spec']['containers'][0]['env'].append({'name':'JOB_DELAY','value':str(delay)})
 return {'apiVersion':'batch/v1','kind':'Job','metadata':meta(r,name),'spec':{'backoffLimit':0,'activeDeadlineSeconds':180,'template':template}}
def cronjob(r):
 spec=job(r,'b10b-cron')['spec']
 return {'apiVersion':'batch/v1','kind':'CronJob','metadata':meta(r,'b10b-cron'),'spec':{'schedule':'*/2 * * * *','suspend':False,'concurrencyPolicy':'Forbid','successfulJobsHistoryLimit':2,'failedJobsHistoryLimit':1,'jobTemplate':{'spec':spec}}}
def hpa(r):
 return {'apiVersion':'autoscaling/v2','kind':'HorizontalPodAutoscaler','metadata':meta(r,'b10b-autoscale'),'spec':{'scaleTargetRef':{'apiVersion':'apps/v1','kind':'Deployment','name':'b10b-autoscale'},'minReplicas':1,'maxReplicas':3,'metrics':[{'type':'Resource','resource':{'name':'cpu','target':{'type':'Utilization','averageUtilization':50}}}],'behavior':{'scaleDown':{'stabilizationWindowSeconds':30,'policies':[{'type':'Pods','value':2,'periodSeconds':15}]}}}}
def policy(r):
 return {'apiVersion':'networking.k8s.io/v1','kind':'NetworkPolicy','metadata':meta(r,'b10b-storage-allow-client'),'spec':{'podSelector':{'matchLabels':{'app':'b10b-storage'}},'policyTypes':['Ingress'],'ingress':[{'from':[{'podSelector':{'matchLabels':{'b10b-access':'allowed'}}}],'ports':[{'protocol':'TCP','port':8080}]}]}}
