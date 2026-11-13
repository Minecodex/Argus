"""Remove completed duplicate UI fixture Jobs through normal platform mutations."""
import re
from run import Run,NAMESPACE
r=Run();a=r.api();pending=[];removed=[]
for job in r.get('jobs','-n',NAMESPACE)['items']:
 name=job['metadata']['name']
 if not re.fullmatch(r'b10b-ui-(zh|en)-(light|dark)-\d{6}',name):continue
 assert job['spec']['template']['metadata']['labels']['argus.plan9/run']==r.id
 assert job.get('status',{}).get('succeeded')==1
 result=r.mutate(a,{'operation':'delete','type':'job','name':name,'uid':job['metadata']['uid'],'expected_resource_version':job['metadata']['resourceVersion']},'delete-duplicate-ui-'+job['metadata']['uid'],wait=False)
 pending.append((result,name));removed.append({'name':name,'uid':job['metadata']['uid']})
for result,name in pending:r.await_operation(a,result,'ui-fixture-removed-'+name)
r.save('duplicate-browser-fixtures-removed',{'jobs':removed,'normal_platform_api':True,'historical_operations_preserved':True})
print('Completed duplicate UI fixtures removed; final browser run will retain four new Jobs',flush=True)
