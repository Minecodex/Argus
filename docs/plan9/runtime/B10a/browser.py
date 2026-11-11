"""Launch only the real B10a portal suite, using strict TLS and normal cookies."""
import os,shutil,subprocess,sys
sys.stdout.reconfigure(encoding="utf-8",errors="replace")
from run import Run,ROOT
r=Run();env=os.environ.copy();env.update(ARGUS_E2E_EXTERNAL='1',ARGUS_E2E_ENTERPRISE_ORIGIN='https://argus.dev',ARGUS_PLAN9_B10A_E2E='1',ARGUS_PLAN9_B10A_CLUSTER=r.cluster_id,ARGUS_PLAN9_SESSION_PATH=str(r.session),ARGUS_E2E_ARTIFACTS=str(r.out/'real-browser'),NODE_EXTRA_CA_CERTS=str(ROOT/'artifacts/plan9/reset-20261026/trust-bundle.pem'))
env.pop('ARGUS_E2E_HOST_RESOLVER',None)
with (r.out/'real-browser.log').open('wb') as log:
 p=subprocess.run([shutil.which('pnpm.cmd'),'exec','playwright','test','plan9-b10a-real.spec.ts','--workers=1','--retries=0'],cwd=ROOT/'web/apps/enterprise',env=env,stdout=log,stderr=subprocess.STDOUT,timeout=900)
content=(r.out/'real-browser.log').read_text(encoding='utf-8')
passed=p.returncode==0 and '5 passed' in content and 'skipped' not in content
r.save('browser-review',{'passed':passed,'cases':5,'strict_tls':True,'normal_session':True,'mock':False,'scope':['zh/en','light/dark','1280/1920 desktop','actual object inventory','Secret redaction','actual ConfigMap preview and create/update','actual Pod logs and exec output']})
print(content[-5000:],flush=True)
assert passed
