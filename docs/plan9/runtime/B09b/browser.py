"""Strict-TLS deployed desktop acceptance using the normal enterprise session."""
import os
import shutil
import subprocess
import sys

from run import ROOT, Run


def main():
    sys.stdout.reconfigure(encoding='utf-8')
    run = Run()
    env = os.environ.copy()
    env.update(ARGUS_E2E_EXTERNAL='1', ARGUS_E2E_ENTERPRISE_ORIGIN='https://argus.dev',
        ARGUS_PLAN9_B09B_E2E='1', ARGUS_PLAN9_CLUSTER_ID=run.cluster, ARGUS_PLAN9_SESSION_PATH=str(run.session),
        ARGUS_PLAN9_COMPONENT_BOARDS=str(run.out / 'dashboard-verification.json'),
        ARGUS_E2E_ARTIFACTS=str(run.out / 'real-browser-final'), NODE_EXTRA_CA_CERTS=str(run.ca))
    # No host-resolver override: browser and API requests keep certificate validation enabled.
    env.pop('ARGUS_E2E_HOST_RESOLVER', None)
    pnpm = shutil.which('pnpm.cmd' if os.name == 'nt' else 'pnpm')
    assert pnpm, 'pnpm is required for real browser acceptance'
    log = run.out / 'real-browser-final.log'
    with log.open('wb') as stream:
        result = subprocess.run([pnpm, 'exec', 'playwright', 'test', 'plan9-b09b-real.spec.ts',
            '--workers=1', '--retries=0'], cwd=ROOT / 'web/apps/enterprise', env=env,
            stdout=stream, stderr=subprocess.STDOUT, timeout=900)
    content = log.read_text(encoding='utf-8')
    passed = result.returncode == 0 and '5 passed' in content and 'skipped' not in content
    run.save('browser-review', {'passed': passed, 'cases': 5, 'origin': 'https://argus.dev',
        'strict_tls': True, 'normal_session': True, 'mock': False,
        'coverage': ['zh/en', 'light/dark', '1280/1920 desktop', 'five component switches',
                     'five default dashboards and 30 panels', 'cluster dashboard bindings',
                     'CoreDNS independent stop, no new samples, other collections continue, resume']})
    print(content[-4500:], flush=True)
    assert passed, 'Deployed browser acceptance did not pass without skips'


if __name__ == '__main__':
    main()
