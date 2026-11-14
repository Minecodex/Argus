"""Publish a signed, immutable linux/amd64 release through the normal catalog job."""
import base64
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tarfile
import time

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat

from manage import Run,ROOT,execute,write_json

VERSION='0.3.3-b11a-20261113'


def main():
    run=Run();out=run.out/'collector-release';out.mkdir(exist_ok=True)
    job=json.loads(execute(['kubectl','--context','docker-desktop','-n','argus-system','get','job','argus-telemetry-catalog-sync','-o','json']))
    env={e['name']:e['value'] for e in job['spec']['template']['spec']['containers'][0]['env'] if 'value' in e}
    material=json.loads((ROOT/'deploy/.keys/otelcol-signing-key.json').read_text(encoding='utf-8'))
    private=base64.b64decode(material['private_key']+'===');key=Ed25519PrivateKey.from_private_bytes(private[:32])
    public=base64.b64encode(key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw)).decode().rstrip('=')
    assert material['key_id']==env['ARGUS_OTELCOL_SIGNING_KEY_ID'] and public==env['ARGUS_OTELCOL_SIGNING_PUBLIC_KEY']
    image=json.loads((run.out/'collector-current-image.json').read_text())
    if not (out/'argus-otelcol').exists():
        ident=execute(['docker','create','--label','io.argus.plan9.run='+run.id,'--label','io.argus.plan9.task=artifact-export',image['image_id']])
        try:
            value=json.loads(execute(['docker','inspect',ident]))[0]
            assert value['Config']['Labels']['io.argus.plan9.run']==run.id and value['Image']==image['image_id']
            execute(['docker','cp',ident+':/usr/local/bin/argus-otelcol',str(out/'argus-otelcol')],timeout=90)
        finally:execute(['docker','rm',ident])
    execute(['docker','cp',str(out/'argus-otelcol'),run.container()['Id']+':/var/lib/plan9/b11a/diagnostic/current-collector'],timeout=90)
    validation=run.shell('direct',"chmod 700 /var/lib/plan9/b11a/diagnostic/current-collector\n/var/lib/plan9/b11a/diagnostic/current-collector validate --config=/var/lib/argus-otelcol/01a1d539-c80a-7341-a10a-2ce6cf4ad8b2/collector-config.yaml --config=/var/lib/plan9/b11a/diagnostic/empty-private.json\n")
    (out/'actual-candidate-validation.log').write_text(validation,encoding='utf-8')
    archive=out/'linux-amd64.tar.gz'
    if not archive.exists():
        with tarfile.open(archive,'w:gz',compresslevel=6) as tar:
            data=(out/'argus-otelcol').read_bytes();info=tarfile.TarInfo('argus-otelcol');info.size=len(data);info.mode=0o755;info.mtime=0;tar.addfile(info,io.BytesIO(data))
    digest=hashlib.sha256(archive.read_bytes()).hexdigest();signature=base64.b64encode(key.sign(bytes.fromhex(digest))).decode().rstrip('=')
    object_key='argus-otelcol/'+VERSION+'/linux-amd64.tar.gz'
    release={'version':VERSION,'platform':'linux_amd64','uri':'https://artifacts.argus.dev/argus-collector-artifacts/'+object_key,
             'sha256':digest,'signature':signature,'byte_size':archive.stat().st_size,'signing_key_id':material['key_id'],
             'source_image':image,'other_architectures_unchanged':True}
    write_json(out/'release.json',release)
    creds=json.loads(execute(['kubectl','--context','docker-desktop','-n','argus-system','get','secret','argus-data-credentials','-o','json']))['data']
    forward=subprocess.Popen(['kubectl','--context','docker-desktop','-n','argus-system','port-forward','service/argus-minio','23401:9000','--address=127.0.0.1'],stdout=(out/'port-forward.log').open('wb'),stderr=subprocess.STDOUT,creationflags=getattr(subprocess,'CREATE_NO_WINDOW',0))
    request=run.private/'artifact-publication.json'
    try:
        for _ in range(40):
            try:
                with socket.create_connection(('127.0.0.1',23401),timeout=1):break
            except OSError:time.sleep(.5)
        payload={'Endpoint':'127.0.0.1:23401','Access':base64.b64decode(creds['minio-root-user']).decode(),'Secret':base64.b64decode(creds['minio-root-password']).decode(),'Path':str(archive),'Key':object_key}
        write_json(request,payload)
        if not (out/'uploaded.json').exists():
            execute(['go','run','./docs/plan9/runtime/B11a/publishartifact',str(request)],timeout=180)
            write_json(out/'uploaded.json',{'object_key':object_key,'sha256':digest})
    finally:
        forward.terminate();forward.wait(timeout=15);request.write_text('{}\n',encoding='utf-8')
    context=run.out/'catalog-context';context.mkdir(exist_ok=True)
    backend=json.loads(execute(['kubectl','--context','docker-desktop','-n','argus-system','get','deployment','argus-server','-o','json']))['spec']['template']['spec']['containers'][0]['image']
    (context/'Dockerfile').write_text('FROM '+backend.replace('host.docker.internal:5001','localhost:5001')+'\nCOPY argus-telemetry-catalog-sync /usr/local/bin/argus-telemetry-catalog-sync\n')
    tag='localhost:5001/argus/argus-backend:local-20261113-b11a-catalog'
    if not (out/'catalog-image.json').exists():
        with (out/'catalog-build.log').open('wb') as log:
            subprocess.run(['docker','buildx','build','--platform','linux/amd64','-t',tag,'--push','--metadata-file',str(out/'catalog-image.json'),str(context)],stdout=log,stderr=subprocess.STDOUT,check=True,timeout=180)
    image_digest=json.loads((out/'catalog-image.json').read_text())['containerimage.digest']
    ref=tag.replace('localhost:5001','host.docker.internal:5001')+'@'+image_digest
    loaders=json.loads(execute(['kubectl','--context','docker-desktop','-n','kube-system','get','pods','-l','app.kubernetes.io/name=argus-image-loader-argus','-o','json']))['items']
    for pod in loaders:
        if pod['metadata'].get('deletionTimestamp'):continue
        command=['kubectl','--context','docker-desktop','-n','kube-system','exec',pod['metadata']['name'],'-c','loader','--','/host/ctr','--address','/run/containerd/containerd.sock','--namespace','k8s.io','images']
        execute([*command,'pull','--plain-http',ref],timeout=180);execute([*command,'tag','--force',ref,ref.split(':local-')[0]+'@'+image_digest])
    template=job['spec']['template']['spec'];container=template['containers'][0];container['image']=ref
    env.update(ARGUS_OTELCOL_VERSION=VERSION,ARGUS_OTELCOL_PLATFORMS='linux_amd64',ARGUS_OTELCOL_LINUX_AMD64_URI=release['uri'],ARGUS_OTELCOL_LINUX_AMD64_SHA256=digest,ARGUS_OTELCOL_LINUX_AMD64_SIGNATURE=signature,ARGUS_OTELCOL_LINUX_AMD64_BYTE_SIZE=str(release['byte_size']))
    container['env']=[{'name':name,'value':value} for name,value in env.items()]
    value={'apiVersion':'batch/v1','kind':'Job','metadata':{'name':'plan9-b11a-catalog','namespace':'argus-system','labels':{'argus.plan9/run':run.id}},'spec':{'backoffLimit':0,'template':{'metadata':{'labels':{'argus.plan9/run':run.id}},'spec':template}}}
    execute(['kubectl','--context','docker-desktop','create','-f','-'],data=json.dumps(value))
    execute(['kubectl','--context','docker-desktop','-n','argus-system','wait','--for=condition=complete','job/plan9-b11a-catalog','--timeout=120s'],timeout=130)
    (out/'catalog-sync.log').write_text(execute(['kubectl','--context','docker-desktop','-n','argus-system','logs','job/plan9-b11a-catalog']),encoding='utf-8')
    print('Signed current linux/amd64 artifact and scoped product catalog release published',flush=True)


if __name__=='__main__':main()
