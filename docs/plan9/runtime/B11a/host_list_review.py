"""Publish and verify the B11a host-list follow-up without changing telemetry."""
import argparse
import hashlib
import json
import os
import subprocess
import sys
from manage import Run,ROOT,execute,now


def publish(run):
    sys.path.insert(0,str(ROOT/'docs/plan9/runtime/B10c'))
    from B10c.publish_web import execute_stage
    os.environ.setdefault('ARGUS_PLAN9_WEB_TAG','local-20261113-b11a-host-list-v1')
    execute_stage(run,'publish-web')


def browser(run):
    out=run.out/'browser';env=dict(os.environ)
    env.update(ARGUS_HOST_LIST_TRENDS_REAL='1',ARGUS_PLAN9_B11A_MAP=str(run.resources),
        ARGUS_E2E_EXTERNAL='1',ARGUS_E2E_ENTERPRISE_ORIGIN='https://argus.dev',
        ARGUS_E2E_HOST_RESOLVER='MAP argus.dev 127.0.0.1',ARGUS_E2E_ARTIFACTS=str(out),
        NODE_EXTRA_CA_CERTS=str(ROOT/'artifacts/plan9/reset-20261026/trust-bundle.pem'))
    run.save('browser-review',{'passed':False,'status':'running'})
    with (run.out/'browser.log').open('wb') as log:
        subprocess.run(['node',str(ROOT/'node_modules/@playwright/test/cli.js'),'test','e2e/host-list-trends-real.spec.ts',
            '--project=chromium','--workers=1','--retries=0'],cwd=ROOT/'web/apps/enterprise',env=env,
            stdout=log,stderr=subprocess.STDOUT,check=True,timeout=1000)
    run.save('browser-review',{'passed':True,'cases':6,'normal_login':True,'strict_tls':True,
        'locales':['zh-CN','en-US'],'themes':['light','dark'],'widths':[1280,1440,1920],
        'independent_delayed_cpu':True,'real_metric_failure_recovery':True,'filter_cancellation':True})


def verify(run):
    before=json.loads((run.out/'web-platform-before.json').read_text(encoding='utf-8'))
    after=run.platform();release=json.loads((run.out/'web-release.json').read_text(encoding='utf-8'))
    assert before['pvc']==after['pvc'] and len(after['pvc'])==6
    for key,value in before['deployments'].items():
        assert value['uid']==after['deployments'][key]['uid']
        if key!='argus-web':assert value==after['deployments'][key]
    assert after['deployments']['argus-web']['images']==[release['image']]
    pods=json.loads(execute(['kubectl','--context','docker-desktop','-n','argus-system','get','pods','-l','app.kubernetes.io/name=argus-web','-o','json']))['items']
    pods=[p for p in pods if not p['metadata'].get('deletionTimestamp')]
    assert pods and all(p['status'].get('containerStatuses') and all(s['ready'] and s['imageID'].endswith(release['image'].split('@')[1]) for s in p['status']['containerStatuses']) for p in pods)
    manifest=json.loads((run.out/release['build_directory']/'source.json').read_text())['files']
    assert all(hashlib.sha256((ROOT/file).read_bytes()).hexdigest()==digest for file,digest in manifest.items() if '/e2e/' not in file and '.test.' not in file)
    review=json.loads((run.out/'browser-review.json').read_text());assert review['passed']
    api=run.api();api.login()
    try:
        host_id=json.loads(run.resources.read_text())['hosts']['direct']['host_id']
        host=api.call('GET','/enterprise/hosts/'+host_id)
        assert host['status']=='active' and host['control_path']=='direct'
        collector=api.call('GET','/enterprise/hosts/'+host_id+'/collector')
        assert collector['status']=='converged' and collector['desired_revision']==collector['effective_revision']
        previous=json.loads((run.out.parent/'native-collections-final.json').read_text(encoding='utf-8'))
        native=api.call('GET','/monitoring/builtins?target_type=host&target_id='+host_id)
        configuration=lambda rows:{row['definition']['key']:{key:row['collection'][key] for key in ['id','config','desired_enabled']} for row in rows if row.get('collection') and row['collection']['desired_enabled']}
        assert configuration(previous)==configuration(native)
        assert all(row['collection']['health']=='healthy' and row['collection']['freshness']=='fresh' for row in native if row.get('collection') and row['collection']['desired_enabled'])
        run.save('native-collections-preserved',native)
        delivery=json.loads(run.shell('direct','cat /var/lib/argus-otelcol/identity/delivery-state.json\n'))['report']
        assert delivery['terminal_failures']['metric_points']==1999 and delivery['enqueue_failures']['metric_points']==delivery['receiver_refusals']['metric_points']==0
        run.save('delivery-preserved',delivery)
    finally:
        api.call('DELETE','/enterprise/auth/session',expected=[204,401,403])
        api.session_path.write_text(json.dumps({'origin':'https://argus.dev','csrf':'','user':{},'cookies':[]})+'\n',encoding='utf-8')
    run.save('final-review',{'passed':True,'verified_at':now(),'image':release['image'],'browser':review,
        'formal_pvcs_preserved':6,'only_web_deployment_changed':True,'deployed_production_source_matches':True,
        'collector_configuration_unchanged':True,'historical_delivery_gap_preserved':1999})
    state=json.loads((run.out.parent/'run.json').read_text(encoding='utf-8'))
    state['host_list_followup']={'record':str(run.out/'final-review.json'),'image':release['image'],'status':'verified_pending_user_review'}
    (run.out.parent/'run.json').write_text(json.dumps(state,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print('Host-list actual deployment/page checks and six formal PVC identities passed',flush=True)


if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['publish','browser','verify']);args=parser.parse_args()
    run=Run();run.out=run.out/'host-list-trends-v1';run.out.mkdir(exist_ok=True)
    globals()[args.action](run)
