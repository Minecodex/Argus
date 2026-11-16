"""Run the actual current-portal model workflow without exposing provider keys."""
import argparse
import os
import subprocess
from manage import Run,ROOT

def execute_stage(run,stage):
    env={**os.environ,'ARGUS_PLAN9_B12_REAL':'1','ARGUS_PLAN9_B12_MAP':str(run.resources),'ARGUS_PLAN9_B12_OUT':str(run.out),
        'ARGUS_PLAN9_B12_STAGE':stage,'ARGUS_E2E_EXTERNAL':'1','ARGUS_E2E_ENTERPRISE_ORIGIN':'https://argus.dev',
        'ARGUS_E2E_HOST_RESOLVER':'MAP argus.dev 127.0.0.1','ARGUS_E2E_ARTIFACTS':str(run.out/('browser-'+stage)),
        'NODE_EXTRA_CA_CERTS':str(ROOT/'deploy/.cache/argus-trust-bundle.pem')}
    with (run.out/('browser-'+stage+'.log')).open('wb') as log:
        subprocess.run(['node','node_modules/@playwright/test/cli.js','test','--config=web/apps/enterprise/playwright.config.ts',
            'web/apps/enterprise/e2e/plan9-b12-real.spec.ts','--project=chromium','--workers=1','--retries=0'],cwd=ROOT,env=env,stdout=log,stderr=subprocess.STDOUT,check=True,timeout=780)
    print('Actual B12 '+stage+' browser and model stage passed',flush=True)

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('stage',choices=['observe','analysis','analysis_review','metrics','repair_flow','repair_review']);args=parser.parse_args();execute_stage(Run(),args.stage)
