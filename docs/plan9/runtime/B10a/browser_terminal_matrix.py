"""Normal-session real UI checks for every supported Pod and expanded options."""
import os,shutil,subprocess,sys,json
from run import Run,ROOT
terminals_only='--terminals-only' in sys.argv
r=Run();r.api().call('GET','/enterprise/auth/session');out=r.out/'terminal-options-audit';out.mkdir(exist_ok=True)
env=os.environ.copy();env.update(ARGUS_E2E_EXTERNAL='1',ARGUS_E2E_ENTERPRISE_ORIGIN='https://argus.dev',ARGUS_PLAN9_B10A_E2E='1',ARGUS_PLAN9_B10A_CLUSTER=r.cluster_id,ARGUS_PLAN9_SESSION_PATH=str(r.session),ARGUS_PLAN9_POD_AUDIT_FILE=str(r.out/'pod-terminal-inventory.json'),ARGUS_E2E_ARTIFACTS=str(out/('terminal-probe' if terminals_only else 'real-browser')),NODE_EXTRA_CA_CERTS=str(ROOT/'artifacts/plan9/reset-20261026/trust-bundle.pem'))
env.pop('ARGUS_E2E_HOST_RESOLVER',None)
logfile=out/('terminal-probe.log' if terminals_only else 'real-browser.log')
with logfile.open('wb') as log:
 p=subprocess.run([shutil.which('pnpm.cmd'),'exec','playwright','test','plan9-b10a-terminal-matrix-real.spec.ts','--workers=1','--retries=0']+(['--grep','actual terminal'] if terminals_only else []),cwd=ROOT/'web/apps/enterprise',env=env,stdout=log,stderr=subprocess.STDOUT,timeout=600)
content=logfile.read_text(encoding='utf-8');cases=4 if terminals_only else 8;passed=p.returncode==0 and (str(cases)+' passed') in content and 'skipped' not in content
r.save('pod-terminal-platform-proof' if terminals_only else 'terminal-options-browser',{'passed':passed,'cases':cases,'terminal_pods':4,'expanded_options_cases':0 if terminals_only else 4,'normal_login':True,'strict_tls':True,'mock':False,'retries':0})
sys.stdout.reconfigure(encoding='utf-8',errors='replace');print(content[-5000:],flush=True);assert passed
