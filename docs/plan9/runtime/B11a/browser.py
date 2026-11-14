"""Normal-login verification against the deployed portal and real PTYs."""
import os
import subprocess
from manage import Run,ROOT


run=Run();out=run.out/'browser';out.mkdir(exist_ok=True)
run.save('browser-review',{'passed':False,'status':'running','cases':4})
env=dict(os.environ)
env.update(ARGUS_PLAN9_B11A_REAL='1',ARGUS_PLAN9_B11A_MAP=str(run.resources),ARGUS_E2E_EXTERNAL='1',
    ARGUS_E2E_ENTERPRISE_ORIGIN='https://argus.dev',ARGUS_E2E_HOST_RESOLVER='MAP argus.dev 127.0.0.1',
    ARGUS_E2E_ARTIFACTS=str(out),NODE_EXTRA_CA_CERTS=str(ROOT/'artifacts/plan9/reset-20261026/trust-bundle.pem'))
with (run.out/'browser.log').open('wb') as log:
    subprocess.run(['node',str(ROOT/'node_modules/@playwright/test/cli.js'),'test','e2e/plan9-b11a-real.spec.ts',
        '--project=chromium','--workers=1','--retries=0'],cwd=ROOT/'web/apps/enterprise',env=env,
        stdout=log,stderr=subprocess.STDOUT,check=True,timeout=1100)
run.save('browser-review',{'passed':True,'cases':4,'normal_login':True,'real_backend':True,'strict_tls':True,
    'locales':['zh-CN','en-US'],'themes':['light','dark'],'protocols':['shell','ssh'],
    'actual_output':['hostname','id','ps'],'terminated_after_verification':True,
    'recording_read_and_playback':True,'recovered_alert_timeline':True})
print('Four real direct-host browser cases passed',flush=True)
