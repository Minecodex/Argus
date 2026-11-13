"""Reusable normal workbench API with native UID/version observations."""
import json,time
from datetime import datetime
from urllib.parse import urlencode
class Workbench:
 def namespace(self,api,name=None):
  name=name or self.app_namespace
  page=api.call('GET','/enterprise/kubernetes-clusters/'+self.cluster_id+'/inventory?type=namespace&limit=100')
  row=next(x for x in page['items'] if x['name']==name)
  return page['namespace_references'][row['uid']]
 def inventory(self,api,kind,namespace=True):
  query={'type':kind,'limit':100}
  if namespace:query.update(namespace_mode='explicit',namespace_ids=self.namespace(api))
  return api.call('GET','/enterprise/kubernetes-clusters/'+self.cluster_id+'/inventory?'+urlencode(query))
 def mutate(self,api,body,label,wait=True):
  path='/enterprise/kubernetes-clusters/'+self.cluster_id
  if body['type']!='namespace':body={**body,'namespace_id':self.namespace(api)}
  preview=api.call('POST',path+'/inventory/actions/preview-mutation',body,key=label,expected=201)
  result=api.confirm(preview,wait=False);ident=(result.get('resource_ref') or {}).get('resource_id')
  if not ident:
   def matches(row):
    return row['type']==body['type'] and row['name']==body['name'] and row['operation']==body['operation'] and abs((datetime.fromisoformat(row['created_at'].replace('Z','+00:00'))-datetime.fromisoformat(preview['created_at'].replace('Z','+00:00'))).total_seconds())<1
   rows=api.wait(path+'/operations?limit=200',lambda page:any(matches(x) for x in page['items']),timeout=60,label=label+' dispatched')['items']
   found=[x['id'] for x in rows if matches(x)];assert len(found)==1,'Ambiguous operation reference'
   ident=found[0]
   result['resource_ref']={'resource_id':ident,'resource_type':'kubernetes_operation'}
  if ident and wait:
   def complete(value):
    if value['status'] in ('failed','conflict','cancelled','result_unknown','expired'):raise RuntimeError(label+': '+value['status']+' '+str(value.get('error_code')))
    return value['status'] in ('succeeded','applied')
   self.save(label+'-operation',api.wait(path+'/operations/'+ident,complete,timeout=180,label=label))
  self.event('workbench_operation_verified' if wait else 'workbench_operation_submitted',label=label,operation_id=ident,object_type=body['type'],object_name=body['name'])
  return result
 def await_operation(self,api,result,label):
  ident=result['resource_ref']['resource_id']
  def complete(value):
   if value['status'] in ('failed','conflict','cancelled','result_unknown','expired'):raise RuntimeError(label+': '+str(value))
   return value['status'] in ('succeeded','applied')
  value=api.wait('/enterprise/kubernetes-clusters/'+self.cluster_id+'/operations/'+ident,complete,timeout=240,label=label)
  self.save(label+'-operation',value);return value
 def change(self,api,kind,name,operation,**values):
  native=self.get(kind,name,'-n',self.app_namespace)
  return self.mutate(api,{'operation':operation,'type':kind,'name':name,'uid':native['metadata']['uid'],'expected_resource_version':native['metadata']['resourceVersion'],**values},operation+'-'+name+'-'+native['metadata']['resourceVersion'])
