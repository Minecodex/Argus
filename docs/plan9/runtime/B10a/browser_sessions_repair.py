"""Fresh normal login cookies, strict TLS and actual deployed sessions only."""
import os,shutil,subprocess,sys
from run import Run,ROOT
r=Run();out=r.out/'session-repair';out.mkdir(exist_ok=True)
# Real HTTP requests create enough recent output to test actual mouse-wheel scrolling.
code="import urllib.request;[urllib.request.urlopen('http://b10a-api:8080/?label=session-scroll-baseline-'+str(i),timeout=3).read() for i in range(110)]"
r.kubectl('exec','-n','b10a-workloads','deployment/b10a-api','--','python3','-c',code,timeout=30)
env=os.environ.copy();env.update(ARGUS_E2E_EXTERNAL='1',ARGUS_E2E_ENTERPRISE_ORIGIN='https://argus.dev',ARGUS_PLAN9_B10A_E2E='1',ARGUS_PLAN9_B10A_CLUSTER=r.cluster_id,ARGUS_PLAN9_SESSION_PATH=str(r.session),ARGUS_E2E_ARTIFACTS=str(out/'real-browser'),NODE_EXTRA_CA_CERTS=str(ROOT/'artifacts/plan9/reset-20261026/trust-bundle.pem'))
env.pop('ARGUS_E2E_HOST_RESOLVER',None)
with (out/'real-browser.log').open('wb') as log:
 p=subprocess.run([shutil.which('pnpm.cmd'),'exec','playwright','test','plan9-b10a-session-repair-real.spec.ts','--workers=1','--retries=0'],cwd=ROOT/'web/apps/enterprise',env=env,stdout=log,stderr=subprocess.STDOUT,timeout=600)
content=(out/'real-browser.log').read_text(encoding='utf-8')
passed=p.returncode==0 and '6 passed' in content and 'skipped' not in content
r.save('session-repair-browser',{'passed':passed,'cases':6,'strict_tls':True,'normal_login':True,'mock':False,'retries':0})
sys.stdout.reconfigure(encoding='utf-8',errors='replace');print(content[-5000:],flush=True);assert passed
