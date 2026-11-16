"""Verify both original normal portal logins without emitting private values."""
import os
import requests
from manage import Run

run=Run();api=run.api();api.login()
assert api.user['id']=='01a177ec-f249-7cf9-9cb7-77681ac02cd2'
session=requests.Session();session.trust_env=False;session.verify=api.http.verify
response=session.post('https://platform.argus.dev/api/v1/platform/auth/login',
    json={'username':'admin','password':os.environ['ARGUS_PLAN9_PASSWORD']},timeout=30)
assert response.status_code==200 and response.json()['status']=='authenticated','Original platform login failed'
user=response.json()['authenticated_session']['user'];assert user['id']=='01a177ec-ee29-7270-b5a8-9c9b9276259e'
run.save('normal-original-logins',{'passed':True,'enterprise_user_id':api.user['id'],'platform_user_id':user['id'],
    'normal_login':True,'strict_tls':True,'original_passwords':True,'credentials_logged':False})
print('Original independent enterprise/platform users logged in normally with their retained passwords',flush=True)
