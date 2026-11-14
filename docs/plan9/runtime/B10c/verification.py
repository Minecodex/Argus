"""Read-only final preservation and exact deployed-source verification."""
from datetime import datetime, timezone
import hashlib
import json

from infrastructure import NAMESPACE, ROOT, execute


def execute_stage(run, _action):
    before = json.loads((run.out/'platform-before.json').read_text(encoding='utf-8'))
    after = run.platform()
    assert before['pvc']==after['pvc'] and len(after['pvc'])==6
    allowed = {'argus-web','argus-server','argus-worker'}
    for name, item in before['deployments'].items():
        assert after['deployments'][name]['uid']==item['uid'], name
        if name not in allowed:
            assert after['deployments'][name]==item, name
    web = json.loads((run.out/'web-release.json').read_text())
    expected = {'argus-web':web['image']}
    backend = json.loads((run.out/'backend-custom-preview-release.json').read_text())
    expected.update({key:value['image'] for key,value in backend['components'].items()})
    images = {}
    for name, image in expected.items():
        assert after['deployments'][name]['images']==[image], name
        pods = json.loads(execute(['kubectl','--context','docker-desktop','-n','argus-system',
                                  'get','pods','-l','app.kubernetes.io/name='+name,'-o','json']))['items']
        pods = [p for p in pods if not p['metadata'].get('deletionTimestamp')]
        assert pods and all(p['status'].get('containerStatuses') and
                            all(s['ready'] and s['imageID'].endswith(image.split('@')[1])
                                for s in p['status']['containerStatuses']) for p in pods), name
        images[name] = {'image':image,'pod_uids':[p['metadata']['uid'] for p in pods]}
    nodes = run.get('nodes')['items']
    assert len(nodes)==3 and all(any(c['type']=='Ready' and c['status']=='True'
                                   for c in n['status']['conditions']) for n in nodes)
    worker = next(n for n in nodes if n['metadata']['name']==run.id+'-worker')
    assert not worker['spec'].get('unschedulable')
    assert worker['metadata']['labels']['argus.plan9/maintenance']=='verified'
    assert not any(t.get('key')=='argus.plan9/maintenance' for t in worker['spec'].get('taints',[]))
    assert run.get('pdb','b10c-maintenance-budget','-n',NAMESPACE)['spec']['minAvailable']==2
    workloads = {}
    for name in ('b10c-repo','b10c-oci','b10c-maintenance','b10c-activity'):
        row = run.get('deployment',name,'-n',NAMESPACE)
        assert row['status'].get('availableReplicas')==row['spec']['replicas']
        workloads[name] = {'desired':row['spec']['replicas'],'available':row['status']['availableReplicas']}
    api = run.api()
    current = api.call('GET','/alerts/instances?limit=100')['items']
    assert not any(run.cluster_id in row.get('resource_ids',[]) for row in current)
    sources = {}
    for row in api.call('GET','/monitoring/builtins?target_type=kubernetes_cluster&target_id='+run.cluster_id):
        collection = row.get('collection') or {}
        if collection.get('desired_enabled'):
            assert collection['health']=='healthy' and collection['freshness']=='fresh'
            sources[row['definition']['key']] = {'freshness':collection['freshness'],
                                                 'last_sample_at':collection['last_sample_at']}
    assert len(sources)==3
    manifests = {}
    web_folder = web.get('build_directory','web-main')
    assert (run.out/web_folder).resolve().is_relative_to(run.out.resolve())
    backend_folder = backend.get('build_directory','backend-custom-preview-v1')
    assert (run.out/backend_folder).resolve().is_relative_to(run.out.resolve())
    for folder, exclude_e2e in [(backend_folder,False),(web_folder,True)]:
        files = json.loads((run.out/folder/'source.json').read_text())['files']
        changed, production = [], []
        for name, digest in files.items():
            if hashlib.sha256((ROOT/name).read_bytes()).hexdigest()!=digest:
                changed.append(name)
            if not (exclude_e2e and '/e2e/' in name):
                production.append(name)
        assert not any(name in production for name in changed), (folder,changed)
        manifests[folder] = {'production_files_match':True,'production_files':len(production),
                             'changed_test_only_files':changed}
    expiry = json.loads((run.out/'credential-expiry.json').read_text())['expires_at']
    run.save('final-preservation',{
        'passed':True,'verified_at':datetime.now(timezone.utc).isoformat(),
        'formal_pvcs':after['pvc'],'formal_pvcs_preserved':6,
        'only_intended_deployments_updated':sorted(allowed),
        'unchanged_deployments':sorted(set(before['deployments'])-allowed),'exact_ready_images':images,
        'native_ready_nodes':3,'workloads':workloads,'worker_uncordoned':True,
        'temporary_maintenance_taint_removed':True,'pdb_min_available_restored':2,
        'sources':sources,'own_current_alerts':0,'frozen_production_source':manifests,
        'credential_expires_utc':datetime.fromtimestamp(expiry,timezone.utc).isoformat()})
    print('Exact images/source, owned final state and six formal PVC identities verified',flush=True)
