"""Disposable PG domain regression; never writes formal Argus business data."""
import json,os,secrets,subprocess,time
from run import Run
r=Run();name='p9-b10a-namespace-postgres';password=secrets.token_hex(24);envfile=r.private/'postgres.env'
envfile.write_text('POSTGRES_USER=regression\nPOSTGRES_DB=regression\nPOSTGRES_PASSWORD='+password+'\n')
assert subprocess.run(['docker','inspect',name],capture_output=True).returncode!=0
try:
 subprocess.run(['docker','run','-d','--name',name,'--label','argus.plan9.test='+r.id,'--env-file',str(envfile),'--memory','512m','--cpus','0.5','--publish','127.0.0.1:22449:5432','--tmpfs','/var/lib/postgresql/data:rw,size=384m','postgres:17-alpine'],check=True,capture_output=True)
 for _ in range(40):
  if subprocess.run(['docker','exec',name,'pg_isready','-U','regression'],capture_output=True).returncode==0:break
  time.sleep(.5)
 env=os.environ.copy();env['ARGUS_MONITORING_TEST_DATABASE_URL']='postgresql://regression:'+password+'@127.0.0.1:22449/regression?sslmode=disable'
 with (r.out/'postgres-namespace-regressions.log').open('wb') as log:
  p=subprocess.run(['go','test','./internal/telemetry','-run','Test(PostgresMetricNamespaceUIDCannotFollowRecreatedName|MixedNamespaceAttributionRetainsClusterDataWithoutGrantingNamespace|DirectNamespaceAttributionRetainsClusterDataWithoutGrantingNamespace)$','-count=1','-v'],env=env,stdout=log,stderr=subprocess.STDOUT,timeout=300)
 r.save('postgres-namespace-regression',{'passed':p.returncode==0,'disposable_database':True})
 assert p.returncode==0
finally:
 row=json.loads(subprocess.check_output(['docker','inspect',name]))[0]
 assert row['Config']['Labels']['argus.plan9.test']==r.id
 subprocess.run(['docker','rm','-f',name],check=True,capture_output=True)
