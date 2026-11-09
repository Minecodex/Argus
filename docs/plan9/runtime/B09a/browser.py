"""B09a browser acceptance with a real enterprise session and verified TLS."""
import os
import subprocess

from manage import ROOT,Run,now,write_json

run=Run()
base=ROOT/'artifacts/plan9/reset-20261026'
env=dict(os.environ,ARGUS_E2E_EXTERNAL='1',ARGUS_PLAN9_B09A_E2E='1',
    ARGUS_E2E_ENTERPRISE_ORIGIN='https://argus.dev',ARGUS_E2E_PLATFORM_ORIGIN='https://platform.argus.dev',
    ARGUS_E2E_TEMPLATE_ORIGIN='https://cards.argus.dev',
    NODE_EXTRA_CA_CERTS=str(base/'trust-bundle.pem'),
    ARGUS_PLAN9_SESSION_PATH=str(base/'private/enterprise-session.json'),
    ARGUS_PLAN9_RESOURCE_MAP=str(run.resources),ARGUS_E2E_ARTIFACTS=str(run.out/'browser-review'))
env.pop('ARGUS_E2E_HOST_RESOLVER',None)
with (run.out/'browser-review.log').open('wb') as log:
    result=subprocess.run(['node',str(ROOT/'node_modules/@playwright/test/cli.js'),'test',
        'e2e/plan9-b09a-real.spec.ts','--workers=1','--reporter=list','--max-failures=1'],
        cwd=ROOT/'web/apps/enterprise',env=env,stdout=log,stderr=subprocess.STDOUT,timeout=780)
write_json(run.out/'browser-review.json',{'exit_code':result.returncode,'completed_at':now(),
    'target':'deployed_real_api','verified_tls':True,'mock_data':False})
print('B09a browser result',result.returncode)
raise SystemExit(result.returncode)
