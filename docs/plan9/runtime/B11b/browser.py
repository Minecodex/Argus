"""Real portal acceptance through valid normal login and actual PTY output."""
import os
import subprocess
from manage import Run,ROOT

run=Run();env=dict(os.environ);out=run.out/'browser';out.mkdir(exist_ok=True)
env.update(ARGUS_PLAN9_B11B_REAL='1',ARGUS_PLAN9_B11B_MAP=str(run.resources),ARGUS_E2E_EXTERNAL='1',
    ARGUS_E2E_ENTERPRISE_ORIGIN='https://argus.dev',ARGUS_E2E_HOST_RESOLVER='MAP argus.dev 127.0.0.1',
    ARGUS_E2E_ARTIFACTS=str(out),NODE_EXTRA_CA_CERTS=str(ROOT/'artifacts/plan9/reset-20261026/trust-bundle.pem'))
run.save('browser-review',{'passed':False,'status':'running'})
with (run.out/'browser.log').open('wb') as log:
    subprocess.run(['node',str(ROOT/'node_modules/@playwright/test/cli.js'),'test','e2e/plan9-b11b-real.spec.ts',
        '--project=chromium','--workers=1','--retries=0'],cwd=ROOT/'web/apps/enterprise',env=env,stdout=log,stderr=subprocess.STDOUT,check=True,timeout=900)
run.save('browser-review',{'passed':True,'cases':4,'normal_login':True,'strict_tls':True,'real_backend':True,
    'hosts':['bastion','member'],'protocols':['shell','ssh'],'locales':['zh-CN','en-US'],'themes':['light','dark'],
    'widths':[1280,1440,1920],'dock_positions':['left','right','bottom'],'keyboard_resize':True,
    'actual_output':['hostname','id','ps'],'recording_replay':True,'alert_timeline':True})
print('Four actual Bastion/member PTY, dashboard, recording and layout browser cases passed',flush=True)
