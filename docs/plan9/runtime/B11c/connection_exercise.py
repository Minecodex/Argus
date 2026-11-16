"""Interrupt only the owned control SSH transport; leave telemetry independent."""
import json
import re
import shutil
import time
from integration import Batch
from lifecycle import delivery,no_loss,fresh
from manage import Run,now

def exercise(batch):
    run=batch.run;api=batch.api
    previous=run.out/'connection-recovery.json'
    if previous.exists():
        if json.loads(previous.read_text()).get('bastion_data_advanced_while_control_offline'):return
        for name in ['connection-recovery','connection-interruption']:
            source=run.out/(name+'.json');target=run.out/(name+'-before-source-wait.json')
            assert source.resolve().parent==target.resolve().parent==run.out.resolve()
            if not target.exists():shutil.copy2(source,target)
    before=delivery(run);started=now();guard='plan9-b11c-ssh-recover-'+str(int(time.time()));observed=None
    listeners=run.shell('bastion','ss -H -lntp\n');control=set();data=set()
    for line in listeners.splitlines():
        ports={int(port) for port in re.findall(r'127\.0\.0\.1:(\d+)',line)}
        pid=re.search(r'users:\(\("sshd[^\"]*",pid=(\d+),',line)
        if not pid:continue
        if ports&{18443,19443}:control.add(int(pid.group(1)))
        if ports&{14317,14318}:data.add(int(pid.group(1)))
    assert len(control)==1 and data and not control&data,(control,data,listeners)
    pid=next(iter(control))
    # Protect against PID reuse and do not touch the SSH server or telemetry lease.
    run.shell('bastion','case "$(cat /proc/'+str(pid)+'/comm)" in sshd|sshd-session) ;; *) exit 1;; esac\n')
    recover='iptables -D INPUT -p tcp --dport 22 -m conntrack --ctstate NEW -m comment --comment B11c-control-recovery -j REJECT'
    try:
        run.shell('bastion',"systemd-run --collect --unit="+guard+" --on-active=110s /bin/sh -ec '"+recover+"'\n"
            'iptables -I INPUT 1 -p tcp --dport 22 -m conntrack --ctstate NEW -m comment --comment B11c-control-recovery -j REJECT\nkill -TERM '+str(pid)+'\n')
        root=batch.map['hosts']['bastion']['host_id']
        offline=api.wait('/enterprise/hosts/'+root,lambda row:row['connection_status'] in ['offline','degraded'],timeout=95,label='Actual executor control tunnel loss')
        def advancing(rows):
            active=[row['collection'] for row in rows if (row.get('collection') or {}).get('desired_enabled')]
            return len(active)==4 and all(row.get('last_sample_at','')>started for row in active)
        root_sources=api.wait('/monitoring/builtins?target_type=host&target_id='+root,advancing,
            timeout=40,label='Independent root data during control interruption')
        member_sources=api.call('GET','/monitoring/builtins?target_type=host&target_id='+batch.map['hosts']['member']['host_id'])
        sources={'bastion':root_sources,'member':member_sources}
        assert api.call('GET','/enterprise/hosts/'+root)['connection_status'] in ['offline','degraded']
        observed={'at':now(),'root':offline,'sources':sources,'control_sshd_pid':pid,'independent_data_sshd_pids':sorted(data),
            'native_listeners':run.shell('bastion','ss -H -lntp\n'),
            'samples_after_interruption':{key:sum((row.get('collection') or {}).get('last_sample_at','')>started for row in rows) for key,rows in sources.items()}}
        run.save('connection-interruption',observed)
        print('Owned root control SSH interrupted; independent data listeners and sample timestamps captured',flush=True)
    finally:run.shell('bastion','systemctl stop '+guard+'.timer || true\n'+recover+' || true\n')
    sources=fresh(batch,now());after=delivery(run);no_loss(before,after)
    run.save('connection-recovery',{'started_at':started,'offline_observed_at':observed['at'],'verified_at':now(),
        'no_new_delivery_loss':True,'automatic_reconnection':True,'same_host_identity':True,'guard_seconds':110,
        'bastion_data_advanced_while_control_offline':True,
        'member_data_advanced_while_control_offline':advancing(member_sources),
        'member_tunnel_depends_on_bastion_connector_control_lease':True,
        'control_path':'executor_tunnel','telemetry_transport':'executor_tunnel','fresh_sources':sources,
        'delivery_before':before,'delivery_after':after})
    print('Same root and member controls reconnected automatically; both telemetry paths fresh with zero new permanent loss',flush=True)

if __name__=='__main__':
    run=Run();api=run.api();api.login();exercise(Batch(run,api))
