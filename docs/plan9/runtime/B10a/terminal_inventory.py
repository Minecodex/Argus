"""Read actual Pod states, namespace exec grants and supported container shells."""
from datetime import datetime,timezone
import json,subprocess
from run import Run,NAMESPACE
r=Run();a=r.api();a.call('GET','/enterprise/auth/session')
r.kubectl('get','namespace',NAMESPACE,'-o','name') # Owned cluster guard.
pods=r.get('pods','--all-namespaces')['items']
actor='system:serviceaccount:plan9-access:argus-kubernetes-connector'
permissions={}
for namespace in sorted({p['metadata']['namespace'] for p in pods}):
 result=subprocess.run(['docker','exec',r.id+'-control-plane','kubectl','--kubeconfig=/etc/kubernetes/admin.conf','auth','can-i','create','pods/exec','-n',namespace,'--as='+actor],capture_output=True,text=True,timeout=15)
 assert result.returncode in (0,1) and result.stdout.strip() in ('yes','no')
 permissions[namespace]=result.stdout.strip()=='yes'
rows=[]
for pod in pods:
 namespace=pod['metadata']['namespace'];phase=pod['status']['phase']
 for container in pod['spec']['containers']:
  row={'namespace':namespace,'pod':pod['metadata']['name'],'pod_uid':pod['metadata']['uid'],'container':container['name'],'phase':phase,'image':container['image'],'native_exec_allowed':permissions[namespace]}
  if phase!='Running':row['preflight']='not_running'
  elif not permissions[namespace]:row['preflight']='native_permission_denied'
  else:
   code='for shell in sh bash powershell cmd; do command -v "$shell" || true; done'
   value=r.kubectl('exec','-n',namespace,pod['metadata']['name'],'-c',container['name'],'--','/bin/sh','-c',code,timeout=15)
   row['shell_paths']=value.strip().splitlines();row['preflight']='shell_present'
  rows.append(row)
report={'run_id':r.id,'recorded_at':datetime.now(timezone.utc).isoformat(),'cluster_id':r.cluster_id,'native_service_account':actor,'native_permissions':permissions,'pods':rows,'platform_verified':False}
r.save('pod-terminal-inventory',report)
print(json.dumps({'pod_count':len(pods),'permissions':permissions,'shell_candidates':[x for x in rows if x['preflight']=='shell_present']},indent=2),flush=True)
