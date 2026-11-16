"""Republish the verified native amd64 binary via Artifact Store and catalog CLI."""
import base64
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import socket
import time
import urllib3
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
from manage import Run,ROOT,execute

def publish(run):
    out=run.out/'collector-release';out.mkdir(exist_ok=True)
    source=run.out.parent/'p9-b11a-20261113-01'
    release=json.loads((source/'collector-release/release.json').read_text(encoding='utf-8'))
    archive=source/'collector-release/linux-amd64.tar.gz';raw=archive.read_bytes()
    material=json.loads((ROOT/'deploy/.keys/otelcol-signing-key.json').read_text(encoding='utf-8'))
    digest=hashlib.sha256(raw).digest()
    assert digest.hex()==release['sha256'] and len(raw)==release['byte_size'] and material['key_id']==release['signing_key_id']
    Ed25519PublicKey.from_public_bytes(base64.b64decode(material['public_key']+'===')).verify(base64.b64decode(release['signature']+'==='),digest)
    job=json.loads(execute(['kubectl','--context','docker-desktop','-n','argus-system','get','job','argus-telemetry-catalog-sync','-o','json']))
    template=job['spec']['template']['spec'];container=template['containers'][0]
    env={row['name']:row.get('value') for row in container['env']}
    assert env['ARGUS_OTELCOL_SIGNING_KEY_ID']==material['key_id'] and env['ARGUS_OTELCOL_SIGNING_PUBLIC_KEY']==material['public_key']
    key=release['uri'].split('/argus-collector-artifacts/',1)[1]
    # Bulk exec stdin on this Windows/kubectl pair truncated a 47MB stream.
    # Use the normal S3 PUT interface over a loopback-only TCP port-forward.
    credentials=json.loads(execute(['kubectl','--context','docker-desktop','-n','argus-system','get','secret','argus-data-credentials','-o','json']))['data']
    access=base64.b64decode(credentials['minio-root-user']).decode()
    password=base64.b64decode(credentials['minio-root-password']).decode()
    with (out/'upload-forward.log').open('wb') as log:
        forward=subprocess.Popen(['kubectl','--context','docker-desktop','-n','argus-system','port-forward','service/argus-minio','23411:9000','--address=127.0.0.1'],
            cwd=ROOT,stdout=log,stderr=subprocess.STDOUT,creationflags=getattr(subprocess,'CREATE_NO_WINDOW',0))
        try:
            for _ in range(60):
                if forward.poll() is not None:raise RuntimeError('Scoped Artifact Store port-forward failed')
                try:
                    with socket.create_connection(('127.0.0.1',23411),timeout=1):break
                except OSError:time.sleep(.5)
            else:raise RuntimeError('Artifact Store loopback endpoint timed out')
            settings={'aws-sigv4':'aws:amz:us-east-1:s3','user':access+':'+password,
                      'url':'http://127.0.0.1:23411/argus-collector-artifacts/'+key,'upload-file':str(archive).replace('\\','/'),
                      'header':'x-amz-content-sha256: '+release['sha256'],'noproxy':'*','max-time':'120','write-out':'%{http_code}'}
            config='silent\nshow-error\nfail\n'+''.join(name+' = '+json.dumps(value)+'\n' for name,value in settings.items())
            status=execute(['curl.exe','--config','-'],data=config,timeout=140)
            assert status=='200','Artifact Store PUT did not complete'
        finally:
            forward.terminate();forward.wait(timeout=15)
    pool=urllib3.PoolManager(cert_reqs='CERT_REQUIRED',ca_certs=str(ROOT/'deploy/.cache/argus-trust-bundle.pem'),assert_hostname='artifacts.argus.dev',server_hostname='artifacts.argus.dev')
    response=pool.request('GET','https://127.0.0.1/argus-collector-artifacts/'+key,headers={'Host':'artifacts.argus.dev'},timeout=90)
    assert response.status==200 and hashlib.sha256(response.data).hexdigest()==release['sha256']
    context=run.out/'catalog-context';context.mkdir(exist_ok=True)
    binary=source/'catalog-context/argus-telemetry-catalog-sync'
    assert binary.is_file() and binary.stat().st_size>1000000
    shutil.copy2(binary,context/binary.name)
    backend=json.loads(execute(['kubectl','--context','docker-desktop','-n','argus-system','get','deployment','argus-server','-o','json']))['spec']['template']['spec']['containers'][0]['image']
    (context/'Dockerfile').write_text('FROM '+backend.replace('host.docker.internal:5001','localhost:5001')+'\nCOPY argus-telemetry-catalog-sync /usr/local/bin/argus-telemetry-catalog-sync\n',encoding='utf-8')
    tag='localhost:5001/argus/argus-backend:local-20261115-b11c-catalog'
    if not (out/'catalog-image.json').exists():
        with (out/'catalog-build.log').open('wb') as log:
            subprocess.run(['docker','buildx','build','--platform','linux/amd64','-t',tag,'--push','--metadata-file',str(out/'catalog-image.json'),str(context)],cwd=ROOT,stdout=log,stderr=subprocess.STDOUT,check=True,timeout=300)
    image_digest=json.loads((out/'catalog-image.json').read_text())['containerimage.digest']
    ref=tag.replace('localhost:5001','host.docker.internal:5001')+'@'+image_digest
    loaders=json.loads(execute(['kubectl','--context','docker-desktop','-n','kube-system','get','pods','-l','app.kubernetes.io/name=argus-image-loader-argus','-o','json']))['items']
    for pod in loaders:
        if pod['metadata'].get('deletionTimestamp'):continue
        assert pod['metadata']['labels']['argus.io/release-id']=='argus'
        cli=['kubectl','--context','docker-desktop','-n','kube-system','exec',pod['metadata']['name'],'-c','loader','--','/host/ctr','--address','/run/containerd/containerd.sock','--namespace','k8s.io','images']
        execute([*cli,'pull','--plain-http',ref],timeout=180)
        execute([*cli,'tag','--force',ref,ref.split(':local-')[0]+'@'+image_digest])
    container['image']=ref
    updates={'ARGUS_OTELCOL_VERSION':release['version'],'ARGUS_OTELCOL_PLATFORMS':'linux_amd64',
             'ARGUS_OTELCOL_LINUX_AMD64_URI':release['uri'],'ARGUS_OTELCOL_LINUX_AMD64_SHA256':release['sha256'],
             'ARGUS_OTELCOL_LINUX_AMD64_SIGNATURE':release['signature'],'ARGUS_OTELCOL_LINUX_AMD64_BYTE_SIZE':str(release['byte_size'])}
    container['env']=[row for row in container['env'] if row['name'] not in updates]+[{'name':name,'value':value} for name,value in updates.items()]
    name='plan9-b11c-catalog'
    desired={'apiVersion':'batch/v1','kind':'Job','metadata':{'name':name,'namespace':'argus-system','labels':{'argus.plan9/run':run.id}},
             'spec':{'backoffLimit':0,'template':{'metadata':{'labels':{'argus.plan9/run':run.id}},'spec':template}}}
    current=execute(['kubectl','--context','docker-desktop','-n','argus-system','get','job',name,'--ignore-not-found','-o','json'])
    if not current:execute(['kubectl','--context','docker-desktop','create','-f','-'],data=json.dumps(desired))
    else:assert json.loads(current)['metadata']['labels']['argus.plan9/run']==run.id
    execute(['kubectl','--context','docker-desktop','-n','argus-system','wait','--for=condition=complete','job/'+name,'--timeout=120s'],timeout=130)
    owned=json.loads(execute(['kubectl','--context','docker-desktop','-n','argus-system','get','job',name,'-o','json']))
    assert owned['metadata']['labels']['argus.plan9/run']==run.id
    mapping=json.loads(run.resources.read_text());mapping['catalog_job']={'name':name,'namespace':'argus-system','uid':owned['metadata']['uid']};run.save('resource-map',mapping)
    run.save('collector-release',{'version':release['version'],'sha256':release['sha256'],'signature_verified':True,'byte_size':release['byte_size'],
        'original_signed_build_reused':True,'platform':'linux_amd64','operator_catalog_cli':True,'catalog_binary_sha256':hashlib.sha256(binary.read_bytes()).hexdigest(),
        'catalog_job':name,'catalog_image':ref,'other_architectures_unchanged':True})
    api=run.api();api.login()
    row=next(item for item in api.call('GET','/enterprise/telemetry/distributions') if item['version']==release['version'] and item['name']=='argus-otelcol-linux-amd64')
    assert row['support_status']=='supported' and [item['platform'] for item in row['artifacts']]==['linux_amd64']
    run.save('collector-distribution',row)
    print('Signed current amd64 Collector and native filtered catalog published; existing formal workloads unchanged',flush=True)

if __name__=='__main__':publish(Run())
