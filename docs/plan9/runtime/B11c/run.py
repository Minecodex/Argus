"""Explicit stages for the owned, normally authorized B11c run."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
from manage import Run,ROOT,write_json,now

STAGES={
    'prepare':('manage.py','prepare'),'build':('manage.py','build'),'up':('manage.py','up'),
    'network':('network.py','configure'),'isolation':('network.py','restrict'),'publish':('publication.py',),
    **{key:('integration.py',key) for key in ['bastion','member','collectors','builtins']},
    'workload':('manage.py','workload'),'remote':('remote.py',),
    **{key:('monitoring.py',key) for key in ['dashboards','configure_alerts','exercise_alert','exercise_data_pause']},
    **{key:('lifecycle.py',key) for key in ['expiry_prepare','expiry_check','rotate','rotate_after_fix']},
    'connection':('connection_exercise.py',),'delivery_watch':('delivery_watch.py',),'verify':('verification.py',),
    'retire':('retirement.py',),
}

def browser(run,revocation=False):
    label='revocation-browser-r3' if revocation else 'browser'
    env={**os.environ,'ARGUS_PLAN9_B11C_REAL':'1','ARGUS_PLAN9_B11C_MAP':str(run.resources),
        'ARGUS_E2E_EXTERNAL':'1','ARGUS_E2E_ENTERPRISE_ORIGIN':'https://argus.dev',
        'ARGUS_E2E_HOST_RESOLVER':'MAP argus.dev 127.0.0.1','ARGUS_E2E_ARTIFACTS':str(run.out/label),
        'NODE_EXTRA_CA_CERTS':str(ROOT/'deploy/.cache/argus-trust-bundle.pem')}
    args=['node','node_modules/@playwright/test/cli.js','test','--config=web/apps/enterprise/playwright.config.ts',
        'web/apps/enterprise/e2e/plan9-b11c-real.spec.ts','--project=chromium','--workers=1','--retries=0',
        '--grep' if revocation else '--grep-invert','access revocation']
    with (run.out/(label+'.log')).open('wb') as log:subprocess.run(args,cwd=ROOT,env=env,stdout=log,stderr=subprocess.STDOUT,check=True,timeout=800)
    if not revocation:
        evidence=list((run.out/label).glob('*/terminal-evidence.json'));assert len(evidence)==4
        revocation_result=run.out/'revocation-browser-r3/.last-run.json'
        revocation_cases=1 if revocation_result.exists() and json.loads(revocation_result.read_text())['status']=='passed' else 0
        write_json(run.out/'browser-review.json',{'passed':True,'cases':4,'revocation_cases':revocation_cases,'verified_at':now(),
            'normal_login':True,'strict_tls':True,'languages':['zh-CN','en-US'],'themes':['light','dark'],
            'desktop_widths':[1280,1440,1920],'mock':False,'artifact_directory':str(run.out/label)})
    print(label+' actual browser validation passed',flush=True)

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('stage',choices=[*STAGES,'browser','revocation']);args=parser.parse_args()
    if args.stage in ['browser','revocation']:browser(Run(),args.stage=='revocation')
    else:subprocess.run([sys.executable,*[str(Path(__file__).parent/STAGES[args.stage][0]),*STAGES[args.stage][1:]]],cwd=ROOT,check=True)
