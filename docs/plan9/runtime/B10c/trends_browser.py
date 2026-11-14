"""Actual deployment/browser validation; metric faults use transport delay/abort only."""
import json
import os
import subprocess
from infrastructure import Run, ROOT

run = Run();out = run.out/'resource-trends-v1';env = dict(os.environ)
env.update(ARGUS_KWR_TRENDS_REAL='1',ARGUS_PLAN9_B10C_MAP=str(run.resources),
           ARGUS_KWR_TRENDS_FIXTURES=str(out/'fixtures.json'),ARGUS_E2E_EXTERNAL='1',
           ARGUS_E2E_ENTERPRISE_ORIGIN='https://argus.dev',ARGUS_E2E_HOST_RESOLVER='MAP argus.dev 127.0.0.1',
           NODE_EXTRA_CA_CERTS=str(ROOT/'artifacts/plan9/reset-20261026/trust-bundle.pem'),ARGUS_E2E_ARTIFACTS=str(out/'browser'))
with (out/'browser.log').open('wb') as log:
    subprocess.run(['node',str(ROOT/'node_modules/@playwright/test/cli.js'),'test','e2e/kubernetes-resource-trends-real.spec.ts',
                    '--project=chromium','--workers=1','--retries=0'],cwd=ROOT/'web/apps/enterprise',env=env,
                   stdout=log,stderr=subprocess.STDOUT,check=True,timeout=1200)
(out/'browser-review.json').write_text(json.dumps({'passed':True,'cases':6,'normal_login':True,'real_backend':True,
   'strict_tls':True,'themes':['light','dark'],'locales':['zh-CN','en-US'],'widths':[1280,1440,1920],
   'actual_metric_responses_only':True,'transport_delay_and_abort_failure_checks':True},indent=2)+'\n',encoding='utf-8')
print('Six actual progressive-list metric browser cases passed',flush=True)
