"""Record the bounded server fix and synchronize the next installer input."""
import hashlib
import json
import shutil
import requests
import yaml
from manage import Run,ROOT,execute,now

TAG='local-20261115-b11c-proof-r2'
DIGEST='sha256:d2094c8b39a842b728fb28b1d79a150ad5b5ec1c7002a525b29e47d3cd6f8f65'

def record(run):
    session=requests.Session();session.trust_env=False
    images=json.loads((run.out.parent.parent/'argus-account-reset-20261115/release-images.json').read_text())
    accept='application/vnd.oci.image.index.v1+json,application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.v2+json'
    aliases=[]
    for item in images:
        name=item['name'];digest=DIGEST if name=='argus-backend' else item['digest']
        base='http://127.0.0.1:5001/v2/argus/'+name+'/manifests/'
        response=session.get(base+digest,headers={'Accept':accept},timeout=30);response.raise_for_status()
        assert 'sha256:'+hashlib.sha256(response.content).hexdigest()==digest
        target=session.put(base+TAG,data=response.content,headers={'Content-Type':response.headers['Content-Type']},timeout=30);target.raise_for_status()
        assert target.headers['Docker-Content-Digest']==digest
        aliases.append({'name':name,'tag':TAG,'digest':digest,'binary_change':'argus-server and argus-worker' if name=='argus-backend' else 'unchanged'})
    cfg=yaml.safe_load((ROOT/'deploy/.cache/argus-local-formal.yaml').read_text());cfg['spec']['images']['tag']=TAG
    path=ROOT/'deploy/.cache/argus-b11c-proof-20261115.yaml';path.write_text(yaml.safe_dump(cfg,sort_keys=False),encoding='utf-8')
    shutil.copy2(path,ROOT/'deploy/.cache/argus-local-formal.yaml')
    before=json.loads((run.out/'platform-before.json').read_text());after=run.platform();assert before['pvc']==after['pvc']
    changes={}
    for name,old in before['deployments'].items():
        current=after['deployments'][name];assert current['uid']==old['uid']
        if old!=current:
            assert len(old['images'])==len(current['images'])
            assert all(new==previous or ('argus-backend:' in previous and '@'+DIGEST in new)
                for previous,new in zip(old['images'],current['images']))
            assert old['generation']<current['generation']<=old['generation']+2
            changes[name]={'before':old,'after':current}
    run.save('platform-after-fix',after)
    run.save('server-fix-release',{'deployed_at':now(),'backend_digest':DIGEST,'aliases':aliases,'deployment_changes':changes,
        'all_pvc_uids_preserved':True,'all_deployment_uids_preserved':True,'schema_migrations_added':False,
        'binary_sha256':hashlib.sha256((run.out/'backend-context/argus-server').read_bytes()).hexdigest(),
        'worker_binary_sha256':hashlib.sha256((run.out/'backend-context/argus-worker').read_bytes()).hexdigest(),
        'source_sha256':{str(path.relative_to(ROOT)):hashlib.sha256(path.read_bytes()).hexdigest() for path in [ROOT/'internal/connector/host_reconnect.go',ROOT/'internal/resource/connection_proof.go',ROOT/'internal/resource/service.go']}})
    print('Bounded server proof fix deployed; six formal volumes and all workload identities retained; installer aliases synchronized',flush=True)

if __name__=='__main__':record(Run())
