"""Disposable owned PostgreSQL regression tests; formal DB/PVCs are never used."""
import json
import os
import secrets
import subprocess
import time
from manage import Run, ROOT, execute, now


def main(packages=None):
    run = Run()
    name = run.id + '-sql-tests'
    assert not execute(['docker', 'ps', '-aq', '--filter', 'name=^/' + name + '$'])
    password = secrets.token_hex(24)
    envfile = run.private / 'postgres-tests.env'
    envfile.write_text('POSTGRES_PASSWORD=' + password + '\nPOSTGRES_DB=argus_test\nPGDATA=/var/lib/postgresql/data\n', encoding='utf-8')
    image = json.loads(execute(['docker', 'image', 'inspect', 'postgres:18.6-alpine']))[0]['Id']
    ident = execute(['docker', 'run', '-d', '--name', name, '--label', 'io.argus.plan9.run=' + run.id,
                     '--label', 'io.argus.plan9.purpose=disposable-sql-tests', '--cpus=1', '--memory=512m', '--pids-limit=128',
                     '--env-file', str(envfile), '--tmpfs', '/var/lib/postgresql:rw,size=512m', '-p', '127.0.0.1:22493:5432', image])
    succeeded = False
    try:
        for _ in range(30):
            try:
                execute(['docker', 'exec', ident, 'pg_isready', '-U', 'postgres', '-d', 'argus_test'])
                break
            except RuntimeError:
                time.sleep(2)
        else:
            raise RuntimeError('Disposable PostgreSQL did not start')
        env = dict(os.environ)
        address = 'postgres://postgres:' + password + '@127.0.0.1:22493/argus_test?sslmode=disable'
        env.update(ARGUS_MONITORING_TEST_DATABASE_URL=address, ARGUS_DASHBOARD_TEST_DATABASE_URL=address, ARGUS_ALERT_TEST_DATABASE_URL=address,
                   GOTMPDIR=str(run.out / 'go-tmp'))
        (run.out / 'go-tmp').mkdir(exist_ok=True)
        with (run.out / 'postgres-tests.log').open('wb') as log:
            packages = packages or ['./internal/monitoring', './internal/dashboard']
            subprocess.run(['go', 'test', '-p', '1', *packages, '-count=1'], cwd=ROOT, env=env,
                           stdout=log, stderr=subprocess.STDOUT, check=True, timeout=600)
        succeeded = True
    finally:
        row = json.loads(execute(['docker', 'inspect', ident]))[0]
        assert row['Config']['Labels']['io.argus.plan9.run'] == run.id and row['Config']['Labels']['io.argus.plan9.purpose'] == 'disposable-sql-tests'
        execute(['docker', 'rm', '-f', ident])
        assert not execute(['docker', 'ps', '-aq', '--filter', 'name=^/' + name + '$'])
        run.save('postgres-tests', {'at': now(), 'passed': succeeded, 'ephemeral_container_removed': True, 'formal_database_used': False,
                                    'packages': packages, 'log': 'postgres-tests.log'})
    print('PostgreSQL tests passed for ' + ', '.join(packages) + '; disposable database cleaned', flush=True)


if __name__ == '__main__':
    main()
