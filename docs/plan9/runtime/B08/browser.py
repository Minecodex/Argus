"""Run B08 browser acceptance against the real deployed Argus portal."""
import os
import subprocess

from manage import ROOT, Run, now, write_json

run = Run()
session = ROOT/'artifacts/plan9/reset-20261026'
env = dict(os.environ, ARGUS_E2E_EXTERNAL='1', ARGUS_PLAN9_B08_E2E='1',
    ARGUS_E2E_ENTERPRISE_ORIGIN='https://argus.dev',
    ARGUS_E2E_PLATFORM_ORIGIN='https://platform.argus.dev',
    ARGUS_E2E_TEMPLATE_ORIGIN='https://cards.argus.dev',
    NODE_EXTRA_CA_CERTS=str(session/'trust-bundle.pem'),
    ARGUS_PLAN9_SESSION_PATH=str(session/'private/enterprise-session.json'),
    ARGUS_PLAN9_RESOURCE_MAP=str(run.resources),
    ARGUS_E2E_ARTIFACTS=str(run.out/'browser-review'))
env.pop('ARGUS_E2E_HOST_RESOLVER',None)
with (run.out/'browser-review.log').open('wb') as output:
    result = subprocess.run(['node',str(ROOT/'node_modules/@playwright/test/cli.js'),'test',
        'e2e/plan9-b08-real.spec.ts','--workers=1','--reporter=list','--max-failures=1'],
        cwd=ROOT/'web/apps/enterprise',env=env,stdout=output,stderr=subprocess.STDOUT,timeout=480)
write_json(run.out/'browser-review.json',{'exit_code':result.returncode,'completed_at':now(),
    'target':'formal_real_api','tls_verification':True,'mock_data':False})
print('browser exit code:',result.returncode)
raise SystemExit(result.returncode)
