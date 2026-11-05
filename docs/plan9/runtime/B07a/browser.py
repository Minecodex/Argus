import argparse
from datetime import datetime, timezone
import yaml
import json
import os
from pathlib import Path
import subprocess

from manage import ROOT, RUNS, Run

parser = argparse.ArgumentParser()
parser.add_argument('--batch', choices=RUNS, default='B07a')
parser.add_argument('--smoke', action='store_true')
args = parser.parse_args()
run = Run(batch=args.batch)
RUN = run.out
mapping = json.loads((RUN / 'resource-map.json').read_text(encoding='utf-8'))
assert mapping.get('batch_dashboard',{}).get('id') and mapping['targets'] and all(t['engine'] in mapping['dashboards'] for t in mapping['targets'].values()), 'Complete dashboard binding before browser verification'
name = 'browser-smoke' if args.smoke else 'browser-review'
env = os.environ.copy()
env['ARGUS_PLAN9_RUNTIME_ROOT'] = str(ROOT)
env['NODE_EXTRA_CA_CERTS'] = str(ROOT / 'artifacts/plan9/reset-20261026/trust-bundle.pem')
env.pop('ARGUS_PLAN9_B07A_E2E', None)
env.pop('ARGUS_PLAN9_B07B_E2E', None)
env.pop('ARGUS_PLAN9_B07C_E2E', None)
env['ARGUS_PLAN9_' + args.batch.upper() + '_E2E'] = '1'
env.update(ARGUS_E2E_EXTERNAL='1',
           ARGUS_E2E_ENTERPRISE_ORIGIN='https://argus.dev',
           ARGUS_E2E_PLATFORM_ORIGIN='https://platform.argus.dev',
           ARGUS_E2E_TEMPLATE_ORIGIN='https://cards.argus.dev',
           ARGUS_PLAN9_RESOURCE_MAP=str(RUN / 'resource-map.json'),
           ARGUS_PLAN9_RETIRED_MAP=str(run.out.parent / run.predecessor / 'resource-map.json'),
           ARGUS_PLAN9_SESSION_PATH=str(ROOT / 'artifacts/plan9/reset-20261026/private/enterprise-session.json'),
           ARGUS_E2E_ARTIFACTS=str(RUN / name))
env.pop('ARGUS_E2E_HOST_RESOLVER', None)
argv = ['node', str(ROOT / 'node_modules/@playwright/test/cli.js'), 'test',
        'e2e/plan9-b07a-real.spec.ts', '--workers=1', '--reporter=list','--max-failures=1']
if args.smoke:
    argv += ['--grep', 'fits 1280']
with (RUN / (name + '.log')).open('wb') as stream:
    p = subprocess.run(argv, cwd=Path(os.environ.get('ARGUS_PLAN9_BUILD_ROOT', str(ROOT))) / 'web/apps/enterprise', env=env,
                       stdout=stream, stderr=subprocess.STDOUT, timeout=900)
(RUN / (name + '.json')).write_text(json.dumps({'exit_code': p.returncode,
    'target': 'deployed_formal_real_api', 'tls_verification': True,
    'finished_at':datetime.now(timezone.utc).isoformat(),
    'deployed_version':yaml.safe_load((ROOT/'deploy/.cache/argus-local-formal.yaml').read_text(encoding='utf-8-sig'))['spec']['images']['tag'],
    'session': 'normal_enterprise_login_cookie', 'mock_data': False,
    'trace_and_video': 'off_no_session_recording'}, indent=2) + '\n', encoding='utf-8')
print(name + ' exit code:', p.returncode)
raise SystemExit(p.returncode)
