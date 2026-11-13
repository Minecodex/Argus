"""Bounded native guard for a retained CronJob; exact owned UID only."""
import json,time
from run import Run,execute,NAMESPACE
def install(r):
 cron=r.get('cronjob','b10b-cron','-n',NAMESPACE)
 assert cron['metadata']['labels']['argus.plan9/run']==r.id
 deadline=json.loads((r.out/'activity-deadline.json').read_text(encoding='utf-8'))['epoch'];unit='b10b-deadline-'+r.id
 patch=json.dumps([{'op':'test','path':'/metadata/uid','value':cron['metadata']['uid']},{'op':'test','path':'/spec/suspend','value':False},{'op':'replace','path':'/spec/suspend','value':True}])
 execute(['docker','exec',r.id+'-control-plane','systemd-run','--unit='+unit,'--on-active='+str(max(1,int(deadline-time.time())))+'s','kubectl','--kubeconfig=/etc/kubernetes/admin.conf','patch','cronjob','b10b-cron','-n',NAMESPACE,'--type=json','-p',patch])
 r.save('cron-deadline',{'epoch':deadline,'uid':cron['metadata']['uid'],'unit':unit})
 print('Owned CronJob stops scheduling at the HTTP activity deadline',flush=True)
if __name__=='__main__':install(Run())
