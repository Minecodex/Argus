"""Close only leftover browser-test logs owned by this normal HTTP session."""
import json,uuid
from run import Run,execute
r=Run();a=r.api();http=uuid.UUID(a.call('GET','/enterprise/auth/session')['session']['id'])
sql="SELECT id FROM kubernetes_log_sessions WHERE cluster_id='"+str(uuid.UUID(r.cluster_id))+"' AND http_session_id='"+str(http)+"' AND status IN ('pending','active')"
ids=execute(['kubectl','--context','docker-desktop','-n','argus-system','exec','argus-postgresql-0','--','psql','-U','argus','-d','argus','-X','-A','-t','-c',sql]).splitlines()
closed=[]
for ident in ids:
 ident=str(uuid.UUID(ident))
 value=a.call('POST','/enterprise/kubernetes-clusters/'+r.cluster_id+'/log-sessions/'+ident+'/close')
 assert value['status']=='closed';closed.append(ident)
assert not execute(['kubectl','--context','docker-desktop','-n','argus-system','exec','argus-postgresql-0','--','psql','-U','argus','-d','argus','-X','-A','-t','-c',sql]).strip()
r.save('session-repair-leftover-cleanup',{'closed_owned_test_log_sessions':closed,'normal_close_api':True,'scope':'exact_test_http_session_and_cluster','business_database_writes':False})
print('Closed only leftover sessions belonging to this test HTTP login:',len(closed),flush=True)
