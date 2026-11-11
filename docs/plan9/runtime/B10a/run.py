"""Normal platform client and UID/version-fenced B10a object mutations."""
import argparse,hashlib,json,os,sys,time
from pathlib import Path
from urllib.parse import urlencode
from infrastructure import Infrastructure,ROOT,RUN,NAMESPACE,execute,write_json
from common.api import Api
class Run(Infrastructure):
 @property
 def session(self):return self.private/'enterprise-session.json'
 @property
 def cluster_id(self):return json.loads(self.resources.read_text(encoding="utf-8"))['cluster']['id']
 def api(self):return Api(self,self.session,ROOT/'artifacts/plan9/reset-20261026/trust-bundle.pem')
 def save(self,name,value):write_json(self.out/(name+'.json'),value)
 def namespace(self,api):
  page=api.call('GET','/enterprise/kubernetes-clusters/'+self.cluster_id+'/inventory?type=namespace&limit=100')
  row=next(x for x in page['items'] if x['name']==NAMESPACE)
  return page['namespace_references'][row['uid']]
 def inventory(self,api,kind,namespace=True):
  query={'type':kind,'limit':100}
  if namespace:query.update(namespace_mode='explicit',namespace_ids=self.namespace(api))
  return api.call('GET','/enterprise/kubernetes-clusters/'+self.cluster_id+'/inventory?'+urlencode(query))
 def mutate(self,api,body,label):
  path='/enterprise/kubernetes-clusters/'+self.cluster_id
  if body['type']!='namespace':body={**body,'namespace_id':self.namespace(api)}
  preview=api.call('POST',path+'/inventory/actions/preview-mutation',body,key=label,expected=201)
  public=json.dumps(preview,sort_keys=True)
  for canary in getattr(self,'secret_canaries',[]):assert canary not in public,'Protected Secret escaped into mutation preview'
  result=api.confirm(preview)
  operation_id=(result.get('resource_ref') or {}).get('resource_id')
  if operation_id:
   def done(x):
    if x['status'] in ('failed','conflict','cancelled','result_unknown','expired'):raise RuntimeError('B10a mutation '+label+': '+x['status']+' '+str(x.get('error_code')))
    return x['status'] in ('succeeded','applied')
   outcome=api.wait(path+'/operations/'+operation_id,done,timeout=180,label=label)
   self.save(label+'-operation',outcome)
  self.event('workbench_operation_verified',label=label,operation_id=operation_id,object_type=body['type'],object_name=body['name'])
  return result
 def change(self,api,kind,name,operation,**values):
  native=self.get(kind,name,'-n',NAMESPACE)
  body={'operation':operation,'type':kind,'name':name,'uid':native['metadata']['uid'],'expected_resource_version':native['metadata']['resourceVersion'],**values}
  return self.mutate(api,body,operation+'-'+name+'-'+native['metadata']['resourceVersion'])
if __name__=='__main__':
 parser=argparse.ArgumentParser();parser.add_argument('action',choices=['prepare','cluster','images','bootstrap','foundation','objects','exercise','monitoring','sessions','verify','retire']);args=parser.parse_args();r=Run()
 if args.action in ('prepare','cluster','images','bootstrap'):getattr(r,args.action)()
 elif args.action=='foundation':r.foundation(str(r.session))
 elif args.action=='retire':
  from retirement import retire
  retire(r,r.api())
 else:
  from workflow import actions
  actions[args.action](r,r.api())
