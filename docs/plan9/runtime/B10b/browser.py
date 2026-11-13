"""Real deployed desktop browser acceptance, normal login and strict TLS."""
import os,shutil,subprocess,sys
from run import Run,ROOT,NAMESPACE
from workflow import wait_native
r=Run();a=r.api();a.call('GET','/enterprise/auth/session')
env=os.environ.copy();env.update(ARGUS_E2E_EXTERNAL='1',ARGUS_E2E_ENTERPRISE_ORIGIN='https://argus.dev',ARGUS_PLAN9_B10B_E2E='1',ARGUS_PLAN9_B10B_CLUSTER=r.cluster_id,ARGUS_PLAN9_B10B_RESOURCE_MAP=str(r.resources),ARGUS_PLAN9_SESSION_PATH=str(r.session),ARGUS_E2E_ARTIFACTS=str(r.out/'real-browser'),NODE_EXTRA_CA_CERTS=str(ROOT/'artifacts/plan9/reset-20261026/trust-bundle.pem'))
env.pop('ARGUS_E2E_HOST_RESOLVER',None)
wait_native(r,lambda:not r.get('cronjob','b10b-cron','-n',NAMESPACE).get('status',{}).get('active'),'Scheduled Job completes before browser source snapshot',60)
was_paused=r.get('cronjob','b10b-cron','-n',NAMESPACE)['spec'].get('suspend',False)
if not was_paused:r.change(a,'cronjob','b10b-cron','patch',object={'spec':{'suspend':True}})
try:
 with (r.out/'real-browser.log').open('wb') as log:
  p=subprocess.run([shutil.which('pnpm.cmd'),'exec','playwright','test','plan9-b10b-real.spec.ts','plan9-b10b-dashboards-real.spec.ts','--workers=1','--retries=0'],cwd=ROOT/'web/apps/enterprise',env=env,stdout=log,stderr=subprocess.STDOUT,timeout=1000)
finally:
 if not was_paused:r.change(a,'cronjob','b10b-cron','patch',object={'spec':{'suspend':False}})
content=(r.out/'real-browser.log').read_text(encoding='utf-8');passed=p.returncode==0 and '6 passed' in content and 'skipped' not in content
r.save('browser-review',{'passed':passed,'cases':6,'strict_tls':True,'normal_login':True,'mock':False,'retries':0,'manual_runs_while_schedule_paused':True,'original_schedule_restored':not was_paused,'coverage':['zh/en','light/dark','1280/1920 desktop','seven object types','real CronJob manual run while suspended','actual Job stdout','actual mounted storage shell','all bound dashboard canvases','current alerts','native node public CA dialog']})
sys.stdout.reconfigure(encoding='utf-8',errors='replace');print(content[-6000:],flush=True);assert passed
