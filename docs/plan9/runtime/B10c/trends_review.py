"""Owned, real-process fixtures and API checks for progressive list metrics."""
import argparse
import json
import time
from urllib.parse import urlencode
from infrastructure import Run, NAMESPACE


def fixtures(run, api):
    path = run.out/'resource-trends-v1/fixtures.json'
    if path.exists():
        previous = json.loads(path.read_text(encoding='utf-8'))
        assert previous['run_id']==run.id and previous['status']=='destroyed', 'Inspect active fixture identities before recreating'
        archive = path.parent/('fixtures-retired-'+str(int(time.time()))+'.json')
        archive.write_text(json.dumps(previous,indent=2)+'\n',encoding='utf-8')
    rows = []
    code = "import time, pathlib; pathlib.Path('/tmp/argus-trends.bin').write_bytes(b'x'*1048576); print('real resource trends fixture', flush=True); deadline=time.monotonic()+180; exec('while time.monotonic()<deadline:\\n start=time.monotonic()\\n while time.monotonic()-start<0.01: pass\\n time.sleep(0.09)'); time.sleep(8*3600)"
    for variant in ('unlimited','request-only','partial'):
        name = 'b10c-trends-'+variant
        container = {'name':'app','image':run.image,'imagePullPolicy':'Never','command':['python3','-u','-c',code]}
        if variant!='unlimited':
            container['resources'] = {'requests':{'cpu':'5m','memory':'16Mi'}}
        if variant=='partial':
            container['resources']['limits'] = {'cpu':'100m','memory':'64Mi'}
        containers = [container]
        if variant=='partial':
            containers.append({'name':'without-resources','image':run.image,'imagePullPolicy':'Never',
                               'command':['python3','-u','-c',"import time; print('unbounded side container',flush=True); time.sleep(8*3600)"]})
        value = {'apiVersion':'v1','kind':'Pod','metadata':{'name':name,'namespace':NAMESPACE,
                 'labels':{'argus.plan9/run':run.id,'argus.plan9/test':'resource-trends'}},
                 'spec':{'restartPolicy':'Never','automountServiceAccountToken':False,
                         'nodeSelector':{'argus.plan9/role':'worker'},'containers':containers}}
        run.mutate(api,{'type':'pod','name':name,'operation':'create','object':value},'create-'+name)
        run.kubectl('wait','--for=condition=Ready','pod/'+name,'-n',NAMESPACE,'--timeout=90s')
        pod = run.get('pod',name,'-n',NAMESPACE)
        rows.append({'name':name,'uid':pod['metadata']['uid'],'variant':variant})
        path.write_text(json.dumps({'run_id':run.id,'namespace':NAMESPACE,'status':'running','pods':rows},indent=2)+'\n',encoding='utf-8')
    print('Three real-process fixtures created through normal previews and confirmations',flush=True)


def check(run, api):
    root = run.out/'resource-trends-v1'
    mapping = json.loads(run.resources.read_text(encoding='utf-8'))
    ns = mapping['namespaces'][NAMESPACE]['id']
    report = []
    for kind in ('deployment','pod','namespace','node','replicaset','daemonset'):
        namespaced = kind in ('deployment','pod','replicaset')
        query = {'type':kind,'namespace_mode':'explicit' if namespaced else 'all_authorized','limit':30}
        if namespaced: query['namespace_ids'] = ns
        base = '/enterprise/kubernetes-clusters/'+run.cluster_id+'/inventory'
        page = api.call('GET',base+'?'+urlencode(query))
        prepared = api.call('POST',base+'/metrics/prepare',{'list':{'type':kind,'namespaces':{
            'mode':query['namespace_mode'],'ids':[ns] if namespaced else []},'limit':30},'uids':[o['uid'] for o in page['items']]})
        for metric in ('cpu','memory','storage'):
            response = api.call('POST',base+'/metrics/read',{'token':prepared['token'],'metric':metric})
            (root/(kind+'-'+metric+'.json')).write_text(json.dumps(response,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
            ready = [v for v in response['items'] if any(t['state']=='ready' and any(p['value'] is not None for p in t['points']) for t in v['trends'])]
            assert ready,(kind,metric,response)
            report.append({'type':kind,'metric':metric,'ready_rows':len(ready),'rows':len(response['items'])})
    (root/'native-checks.json').write_text(json.dumps({'passed':True,'checks':report},indent=2)+'\n',encoding='utf-8')
    print('Eighteen real current-page metric batches returned actual samples',flush=True)


def cleanup(run, api):
    path = run.out/'resource-trends-v1/fixtures.json'
    saved = json.loads(path.read_text(encoding='utf-8'))
    assert saved['run_id']==run.id and saved['namespace']==NAMESPACE
    for row in saved['pods']:
        value = run.get('pod',row['name'],'-n',NAMESPACE)
        assert value['metadata']['uid']==row['uid'] and value['metadata']['labels']['argus.plan9/test']=='resource-trends'
        run.change(api,'pod',row['name'],'delete')
        run.kubectl('wait','--for=delete','pod/'+row['name'],'-n',NAMESPACE,'--timeout=90s')
    saved.update(status='destroyed',cleaned_at=time.time())
    path.write_text(json.dumps(saved,indent=2)+'\n',encoding='utf-8')
    print('Only the three UID-verified temporary test Pods were removed',flush=True)


if __name__=='__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('action',choices=('fixtures','check','cleanup'))
    args = parser.parse_args()
    run = Run();api = run.api();api.login()
    globals()[args.action](run,api)
