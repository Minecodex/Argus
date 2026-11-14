"""Normal-login, strict-TLS browser acceptance against the actual deployment."""
import json
import os
import subprocess
from infrastructure import Run, ROOT

run = Run()
env = dict(os.environ)
env.update(ARGUS_PLAN9_B10C_E2E='1',ARGUS_PLAN9_B10C_MAP=str(run.resources),
           ARGUS_E2E_EXTERNAL='1',ARGUS_E2E_ENTERPRISE_ORIGIN='https://argus.dev',
           ARGUS_E2E_HOST_RESOLVER='MAP argus.dev 127.0.0.1',
           NODE_EXTRA_CA_CERTS=str(ROOT/'artifacts/plan9/reset-20261026/trust-bundle.pem'),
           ARGUS_E2E_ARTIFACTS=str(run.out/'browser'))
with (run.out/'browser.log').open('wb') as log:
    subprocess.run(['node',str(ROOT/'node_modules/@playwright/test/cli.js'),'test','e2e/plan9-b10c-real.spec.ts',
                    '--project=chromium','--workers=1','--retries=0'],cwd=ROOT/'web/apps/enterprise',
                   env=env,stdout=log,stderr=subprocess.STDOUT,check=True,timeout=1200)
run.save('browser-review',{'passed':True,'cases':5,'normal_login':True,'real_backend':True,
                           'strict_tls':True,'themes':['light','dark'],'locales':['zh-CN','en-US'],
                           'real_namespace_scope_denial_and_recovery':True,'type_filter_and_url_context_preserved':True,
                           'table_header_and_sticky_action_colors':True,'table_surface_viewports':[1280,1440,1920]})
print('Five actual B10c browser and session cases passed',flush=True)
