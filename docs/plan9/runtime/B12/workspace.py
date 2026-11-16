"""Create the missing platform-owned offline Workspace prerequisites normally."""
import base64
import hashlib
import json
import os
import requests
import yaml
from manage import Run,ROOT,execute,now

def configure(run,api):
    session=requests.Session();session.trust_env=False;session.verify=api.http.verify;session.headers['Origin']='https://platform.argus.dev'
    login=session.post('https://platform.argus.dev/api/v1/platform/auth/login',json={'username':'admin','password':os.environ['ARGUS_PLAN9_PASSWORD']},timeout=30)
    assert login.status_code==200 and login.json()['status']=='authenticated'
    session.headers['X-CSRF-Token']=login.json()['authenticated_session']['csrf_token']
    def call(method,path,body=None,missing=False):
        key='p9-b12-platform-'+hashlib.sha256((path+json.dumps(body,sort_keys=True)).encode()).hexdigest()
        response=session.request(method,'https://platform.argus.dev/api/v1'+path,json=body,headers={'Idempotency-Key':key},timeout=90)
        if missing and response.status_code==404:return None
        assert response.status_code in [200,201],(path,response.status_code,response.json().get('code'))
        return response.json()
    backends=call('GET','/platform/sandbox/backends')
    backend=next((row for row in backends if row['name']=='Local OpenSandbox'),None)
    if backend is None:
        secret=json.loads(execute(['kubectl','--context','docker-desktop','-n','argus-sandbox','get','secret','argus-generated-secrets','-o','json']))['data']
        key=base64.b64decode(secret['opensandbox-api-key']).decode()
        backend=call('POST','/platform/sandbox/backends',{'name':'Local OpenSandbox','endpoint':'http://opensandbox-server.argus-sandbox.svc',
            'api_key':key,'status':'enabled','expected_version':0})
    backend=call('POST','/platform/sandbox/backends/'+backend['id']+'/test')
    assert backend['health_status']=='healthy'
    cfg=yaml.safe_load((ROOT/'deploy/.cache/argus-local-formal.yaml').read_text());tag=cfg['spec']['images']['tag']
    ref='host.docker.internal:5001/argus/argus-workspace:'+tag
    public=requests.Session();public.trust_env=False
    manifest=public.get('http://127.0.0.1:5001/v2/argus/argus-workspace/manifests/'+tag,
        headers={'Accept':'application/vnd.oci.image.index.v1+json,application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.v2+json'},timeout=30);manifest.raise_for_status()
    digest='sha256:'+hashlib.sha256(manifest.content).hexdigest()
    images=call('GET','/platform/sandbox/images');image=next((row for row in images if row['name']=='Local offline analysis'),None)
    if image is None:image=call('POST','/platform/sandbox/images',{'name':'Local offline analysis','backend_id':backend['id'],'image_ref':ref,'digest':digest,'status':'enabled','expected_version':0})
    profiles=call('GET','/platform/sandbox/profiles');profile=next((row for row in profiles if row['name']=='Local offline Workspace'),None)
    if profile is None:profile=call('POST','/platform/sandbox/profiles',{'name':'Local offline Workspace','backend_id':backend['id'],'image_id':image['id'],
        'task_kinds':['agent_workspace'],'cpu_millis':500,'memory_mib':512,'timeout_seconds':600,'network_mode':'none','status':'enabled','expected_version':0})
    current=api.call('GET','/enterprise/auth/session');enterprise=current['session']['enterprise_id']
    quota=call('GET','/platform/sandbox/enterprise-quotas/'+enterprise,missing=True)
    if not quota or quota['max_concurrent_sessions']==0 or quota['monthly_session_seconds']==0:
        quota=call('PUT','/platform/sandbox/enterprise-quotas/'+enterprise,{'max_concurrent_sessions':1,'monthly_session_seconds':3600,'expected_version':quota['version'] if quota else 0})
    run.save('workspace-platform-configured',{'at':now(),'backend':backend,'image':image,'profile':profile,'quota':quota,
        'platform_admin_separate_from_enterprise_user':True,'normal_api':True,'network_mode':'none','shared_runtime_retained':True})
    print('Native offline Workspace backend/image/profile ready with 500m CPU, 512 MiB and finite enterprise quota',flush=True)

if __name__=='__main__':
    run=Run();api=run.api();api.login();configure(run,api)
