"""Archive B10b evidence and retire its owned resources before starting B10c."""
import json
import os
import subprocess
import sys

from infrastructure import ROOT, PREDECESSOR, execute


def execute_stage(run, _action):
    old = ROOT/'artifacts/plan9'/PREDECESSOR
    a = run.api()
    before = run.platform()
    mapping = json.loads((old/'resource-map.json').read_text(encoding='utf-8'))
    cluster = a.call('GET','/enterprise/kubernetes-clusters/'+mapping['cluster']['id'])
    assert cluster['labels']['plan9_run'] == PREDECESSOR
    native = json.loads(execute(['docker','inspect',PREDECESSOR+'-control-plane']))[0]
    proof = json.loads((old/'cluster-infrastructure.json').read_text(encoding='utf-8'))
    assert native['Id'] == proof['container_id'] and native['Config']['Labels']['io.x-k8s.kind.cluster'] == PREDECESSOR
    # Stop evaluations before deleting consumers, while retaining alert history.
    policies = []
    for row in mapping['alert_policies'].values():
        current = a.call('GET','/alerts/configurations/'+row['id'],expected=[200,404])
        if current.get('code'):
            continue
        assert current['name'].startswith('B10b ')
        policies.append({'id':current['id'],'version':current['version'],'name':current['name']})
        a.confirm(a.call('POST','/alerts/configurations/preview',{'kind':'policy','operation':'delete',
                        'id':current['id'],'expected_version':current['version']},key='retire-B10b-policy-'+row['id'],expected=202))
    run.save('predecessor-alerts-retired', {'policies':policies,'history_preserved':True})
    stored = a.call('GET','/enterprise/auth/session')
    # The existing B10b retirement process runs in its own import namespace.
    (old/'private/enterprise-session.json').write_text(run.session.read_text(encoding='utf-8'),encoding='utf-8')
    with (run.out/'predecessor-retire.log').open('wb') as log:
        subprocess.run([sys.executable,str(ROOT/'docs/plan9/runtime/B10b/run.py'),'retire'],cwd=ROOT,
                       stdout=log,stderr=subprocess.STDOUT,check=True,timeout=1200)
    assert run.platform() == before
    cleanup = json.loads((old/'cleanup-verification.json').read_text(encoding='utf-8'))
    assert cleanup['status'] == 'destroyed'
    assert not execute(['docker','container','ls','-aq','--filter','label=io.x-k8s.kind.cluster='+PREDECESSOR])
    run.save('predecessor-cleanup-verification', cleanup)
    print('B10b consumers and CSI volumes retired; formal services and six PVCs preserved',flush=True)
