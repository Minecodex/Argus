"""Publish the tested retirement dependency fix without replacing formal data."""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import requests
import yaml
from manage import Run,ROOT,execute,now,write_json

TAG='local-20261116-b11c-retirement'
DIGEST='sha256:352e8e18d8cbc582cd981670084a2b0657e073fd7d42e847826d56d7e78b7301'
OUT=Path('C:/Users/23574/.codex/artifacts/argus-b11c-retirement-20261116')

def deploy():
    run=Run();before=run.platform();write_json(OUT/'platform-before.json',before)
    private=OUT/'private';private.mkdir(exist_ok=True)
    execute(['icacls',str(private),'/inheritance:r','/grant:r',execute(['whoami'])+':(OI)(CI)F','*S-1-5-18:(OI)(CI)F'])
    cfg=yaml.safe_load((ROOT/'deploy/.cache/argus-local-formal.yaml').read_text())
    old=cfg['spec']['images']['tag'];assert old=='local-20261115-b11c-proof-r2'
    shutil.copy2(ROOT/'deploy/.cache/argus-local-formal.yaml',private/'prior-install-config.yaml')
    session=requests.Session();session.trust_env=False;aliases=[]
    accept='application/vnd.oci.image.index.v1+json,application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.v2+json'
    for name in ['argus-backend','argus-web','minio','argus-workspace','argus-workspace-egress','argus-remote-runtime','argus-install-tools']:
        base='http://127.0.0.1:5001/v2/argus/'+name+'/manifests/'
        response=session.get(base+(DIGEST if name=='argus-backend' else old),headers={'Accept':accept},timeout=30);response.raise_for_status()
        digest='sha256:'+hashlib.sha256(response.content).hexdigest()
        result=session.put(base+TAG,data=response.content,headers={'Content-Type':response.headers['Content-Type']},timeout=30);result.raise_for_status()
        assert result.headers['Docker-Content-Digest']==digest
        aliases.append({'name':name,'digest':digest,'tag':TAG})
    cli=['kubectl','--context','docker-desktop']
    pods=json.loads(execute([*cli,'-n','kube-system','get','pods','-l','app.kubernetes.io/name=argus-image-loader-argus','-o','json']))['items']
    loader=next(row for row in pods if row['status']['phase']=='Running' and not row['metadata'].get('deletionTimestamp'))
    assert loader['metadata']['labels']['argus.io/release-id']=='argus'
    ctr=[*cli,'-n','kube-system','exec',loader['metadata']['name'],'-c','loader','--','/host/ctr','--address','/run/containerd/containerd.sock','--namespace','k8s.io','images']
    ref='host.docker.internal:5001/argus/argus-backend:'+TAG
    execute([*ctr,'pull','--plain-http','--platform','linux/amd64',ref])
    for alias in [ref+'@'+DIGEST,'host.docker.internal:5001/argus/argus-backend@'+DIGEST]:execute([*ctr,'tag',ref,alias])
    for item in aliases:
        if item['name']!='argus-backend':execute([*ctr,'tag','host.docker.internal:5001/argus/'+item['name']+':'+old,'host.docker.internal:5001/argus/'+item['name']+':'+TAG])
    migration=json.loads(execute([*cli,'-n','argus-system','get','job','argus-postgresql-migration','-o','json']))
    assert migration['metadata']['annotations']['meta.helm.sh/release-name']=='argus-platform' and migration['status']['succeeded']==1
    write_json(private/'completed-migration-before-recreate.json',migration)
    execute([*cli,'-n','argus-system','delete','job','argus-postgresql-migration','--wait=true'])
    with (OUT/'rollout.log').open('wb') as log:
        subprocess.run(['helm','upgrade','argus-platform','deploy/helm/argus-platform','--kube-context','docker-desktop','--namespace','argus-system','--reuse-values',
            '--set-string','images.backend='+ref+'@'+DIGEST,'--wait','--timeout','180s'],cwd=ROOT,stdout=log,stderr=subprocess.STDOUT,check=True,timeout=210)
    after=run.platform();assert before['pvc']==after['pvc'];changes={}
    for name,prior in before['deployments'].items():
        current=after['deployments'][name];assert current['uid']==prior['uid']
        assert all(a==b or ('argus-backend:' in a and '@'+DIGEST in b) for a,b in zip(prior['images'],current['images']))
        if prior!=current:changes[name]={'before':prior,'after':current}
    cfg['spec']['images']['tag']=TAG
    path=ROOT/'deploy/.cache/argus-b11c-retirement-20261116.yaml';path.write_text(yaml.safe_dump(cfg,sort_keys=False),encoding='utf-8');shutil.copy2(path,ROOT/'deploy/.cache/argus-local-formal.yaml')
    write_json(OUT/'release.json',{'at':now(),'passed':True,'backend_image':ref+'@'+DIGEST,'aliases':aliases,'changes':changes,
        'all_formal_pvc_and_deployment_uids_preserved':True,'schema_migrations_added':False,
        'query_sha256':hashlib.sha256((ROOT/'internal/storage/postgres/queries/host_removal_operations.sql').read_bytes()).hexdigest()})
    print('Tested retirement fix deployed; formal volumes/identities preserved; no schema migration added',flush=True)

if __name__=='__main__':deploy()
