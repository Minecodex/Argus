"""Construct an amd64 OCI archive from verified cached content; fetch missing public blobs."""
import hashlib,io,json,subprocess,tarfile
from concurrent.futures import ThreadPoolExecutor
import requests
from run import Run,write_json
r=Run();out=r.out
index_digest='sha256:594ceea76b01c592858f803f9ff4d2cb40542cae2060410b2c95f75907d659e1'
manifest_digest='sha256:de8fd8f1def14ed36f59c9838bd3c920ecb01754072e3e9f10832d62ef876cb8'
tag='registry.k8s.io/ingress-nginx/controller:b10a-v1.15.1'
target='registry.k8s.io/ingress-nginx/controller@'+manifest_digest
prefix=['kubectl','--context','docker-desktop','-n','kube-system','exec','argus-image-loader-argus-xmwz5','-c','loader','--','/host/ctr','--address','/run/containerd/containerd.sock','--namespace','k8s.io','content','get']
cache=out/'public-image-content';cache.mkdir(exist_ok=True);downloaded=[]
def get(digest):
 file=cache/digest.split(':')[1]
 if file.exists() and hashlib.sha256(file.read_bytes()).hexdigest()==digest.split(':')[1]:return file
 with file.open('wb') as stream:result=subprocess.run([*prefix,digest],stdout=stream,stderr=subprocess.PIPE,timeout=60)
 if result.returncode:
  last=None
  for attempt in range(4):
   try:
    response=requests.get('https://registry.k8s.io/v2/ingress-nginx/controller/blobs/'+digest,timeout=(10,45));response.raise_for_status();file.write_bytes(response.content)
    if hashlib.sha256(response.content).hexdigest()!=digest.split(':')[1]:raise RuntimeError('Public blob checksum mismatch')
    downloaded.append(digest);print('Recovered public blob',digest,'bytes',len(response.content),flush=True);break
   except (requests.RequestException,RuntimeError) as error:last=error
  else:raise RuntimeError('Pinned public blob unavailable after four bounded attempts: '+digest) from last
 assert hashlib.sha256(file.read_bytes()).hexdigest()==digest.split(':')[1]
 return file
index=json.loads(get(index_digest).read_text(encoding="utf-8"));assert any(x['digest']==manifest_digest and x.get('platform',{}).get('architecture')=='amd64' for x in index['manifests'])
manifest=get(manifest_digest);metadata=json.loads(manifest.read_text(encoding="utf-8"))
descriptors=[metadata['config'],*metadata['layers']]
with ThreadPoolExecutor(max_workers=3) as pool:
 paths=list(pool.map(lambda x:get(x['digest']),descriptors))
files=[(manifest_digest,manifest)]+[(x['digest'],path) for x,path in zip(descriptors,paths)]
archive=out/'ingress-amd64.tar'
with tarfile.open(archive,'w') as tar:
 for name,data in [('oci-layout',b'{"imageLayoutVersion":"1.0.0"}'),('index.json',json.dumps({'schemaVersion':2,'manifests':[{'mediaType':'application/vnd.oci.image.manifest.v1+json','digest':manifest_digest,'size':manifest.stat().st_size,'annotations':{'org.opencontainers.image.ref.name':tag}}]}).encode())]:
  entry=tarfile.TarInfo(name);entry.size=len(data);tar.addfile(entry,io.BytesIO(data))
 for digest,file in files:tar.add(file,arcname='blobs/sha256/'+digest.split(':')[1])
subprocess.run([str(r.kind),'load','image-archive',str(archive),'--name',r.id],check=True,timeout=180)
subprocess.run(['docker','exec',r.id+'-control-plane','ctr','--namespace','k8s.io','images','tag','--force',tag,target],check=True)
write_json(out/'ingress-image-transfer.json',{'image':target,'parent_index':index_digest,'amd64_manifest':manifest_digest,'all_content_digests_verified':True,'downloaded_missing_public_blobs':downloaded,'bytes':archive.stat().st_size})
print('Official amd64 image verified and loaded; missing blobs fetched',len(downloaded),flush=True)
