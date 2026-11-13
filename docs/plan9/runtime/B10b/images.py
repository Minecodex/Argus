"""Content-addressed public images imported into the exact owned Kind node."""
import hashlib,json,subprocess,time
from infrastructure import execute,write_json
def stage(r,reference):
 file=r.out/'images.lock.json'
 state=json.loads(file.read_text(encoding='utf-8')) if file.exists() else {}
 node=r.id+'-control-plane'
 r.kubectl('get','nodes','-o','name')
 if reference in state:
  desired=state[reference]['pinned']
  if desired in execute(['docker','exec',node,'ctr','-n','k8s.io','images','ls','-q']).splitlines():return desired
 for attempt in range(3):
  p=subprocess.run(['docker','pull','--platform','linux/amd64',reference],capture_output=True,timeout=180)
  if p.returncode==0:break
 else:raise RuntimeError('Public image pull failed: '+reference+' '+p.stderr.decode(errors='replace')[-600:])
 row=json.loads(execute(['docker','image','inspect',reference]))[0]
 repo=reference.split('@',1)[0].rsplit(':',1)[0]
 digest=next((x for x in row.get('RepoDigests',[]) if x.split('@')[0]==repo),None)
 if not digest and row.get('RepoDigests'):digest=row['RepoDigests'][0]
 assert digest and '@sha256:' in digest
 name=hashlib.sha256(reference.encode()).hexdigest()[:16]+'.tar'
 archive=r.out/name
 execute(['docker','image','save','--platform','linux/amd64','-o',str(archive),reference],timeout=180)
 target='/var/tmp/'+r.id+'-'+name
 execute(['docker','cp',str(archive),node+':'+target],timeout=180)
 execute(['docker','exec',node,'ctr','-n','k8s.io','images','import','--platform','linux/amd64',target],timeout=180)
 listing=execute(['docker','exec',node,'ctr','-n','k8s.io','images','ls','-q']).splitlines()
 source=reference if reference in listing else next((x for x in listing if x.split('@')[0]==repo),None)
 assert source,(reference,'imported tag missing')
 if digest not in listing:execute(['docker','exec',node,'ctr','-n','k8s.io','images','tag','--force',source,digest])
 state[reference]={'pinned':digest,'docker_id':row['Id'],'archive_sha256':hashlib.sha256(archive.read_bytes()).hexdigest(),'platform':'linux/amd64'}
 write_json(file,state);print('Pinned native image ready: '+reference,flush=True)
 return digest
